package repository

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/blankon/irgsh-go/internal/cli/domain"
	"github.com/blankon/irgsh-go/pkg/systemutil"
	"gopkg.in/src-d/go-git.v4"
	"gopkg.in/src-d/go-git.v4/plumbing"
)

var ErrCacheUnavailable = errors.New("cache unavailable")

// GitRepoSync implements usecase.RepoSync using a local git cache.
type GitRepoSync struct {
	cacheDir string
}

func NewGitRepoSync(cacheDir string) *GitRepoSync {
	return &GitRepoSync{cacheDir: cacheDir}
}

func (g *GitRepoSync) Sync(repoURL, branch, targetDir string) error {
	log.Println("[syncRepo] syncing repo " + repoURL + " branch " + branch)

	hasher := sha256.New()
	hasher.Write([]byte(repoURL + ":" + branch))
	cacheKey := hex.EncodeToString(hasher.Sum(nil))
	cacheDir := filepath.Join(g.cacheDir, cacheKey)

	remoteHash, err := getRemoteHash(repoURL, branch)
	if err != nil {
		log.Printf("[syncRepo] failed to fetch remote hash: %v", err)
		return err
	}

	unlock, err := lockCacheDir(cacheDir)
	if err != nil {
		log.Printf("[syncRepo] failed to lock cache dir: %v", err)
		return err
	}
	defer func() {
		if unlockErr := unlock(); unlockErr != nil {
			log.Printf("[syncRepo] failed to release cache lock: %v", unlockErr)
		}
	}()

	err = useCache(repoURL, branch, cacheDir, remoteHash, targetDir)
	if err == nil {
		return nil
	}
	if !errors.Is(err, ErrCacheUnavailable) {
		return err
	}

	log.Println("[syncRepo] cache unavailable, cloning fresh copy")
	err = cloneCache(repoURL, branch, cacheDir)
	if err != nil {
		return err
	}

	return useCache(repoURL, branch, cacheDir, remoteHash, targetDir)
}

func getRemoteHash(repoURL string, branch string) (string, error) {
	log.Printf("[getRemoteHash] getting remote hash for %s branch %s", repoURL, branch)

	ref := branch
	if !strings.HasPrefix(ref, "refs/") {
		ref = fmt.Sprintf("refs/heads/%s", branch)
	}
	cmd := exec.Command("git", "ls-remote", repoURL, ref)
	var out, stderr strings.Builder
	cmd.Stdout = &out
	cmd.Stderr = &stderr
	err := cmd.Run()
	if err != nil {
		err = fmt.Errorf("git ls-remote: %w: %s", err, stderr.String())
		log.Printf("[getRemoteHash] %v", err)
		return "", domain.ErrRepoOrBranchNotFound
	}
	parts := strings.Fields(out.String())
	if len(parts) > 0 {
		return parts[0], nil
	}
	return "", domain.ErrRepoOrBranchNotFound
}

func removeCacheDir(cacheDir string) error {
	log.Println("[removeCacheDir] removing cache dir: " + cacheDir)
	err := os.RemoveAll(cacheDir)
	if err != nil {
		log.Printf("[removeCacheDir] failed to remove cache dir: %v", err)
		return err
	}
	return nil
}

func lockCacheDir(cacheDir string) (func() error, error) {
	log.Println("[lockCacheDir] acquiring cache lock: " + cacheDir)

	cacheRoot := filepath.Dir(cacheDir)
	err := os.MkdirAll(cacheRoot, 0755)
	if err != nil {
		return nil, err
	}

	lockPath := cacheDir + ".lock"
	lockFile, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}

	if err := syscall.Flock(int(lockFile.Fd()), syscall.LOCK_EX); err != nil {
		closeErr := lockFile.Close()
		if closeErr != nil {
			return nil, fmt.Errorf("failed to acquire lock: %w (close error: %w)", err, closeErr)
		}
		return nil, err
	}

	log.Println("[lockCacheDir] acquired cache lock: " + cacheDir)

	return func() error {
		log.Println("[lockCacheDir] releasing cache lock: " + cacheDir)
		unlockErr := syscall.Flock(int(lockFile.Fd()), syscall.LOCK_UN)
		closeErr := lockFile.Close()
		if unlockErr != nil && closeErr != nil {
			return fmt.Errorf("failed to unlock: %w (close error: %w)", unlockErr, closeErr)
		}
		if unlockErr != nil {
			return unlockErr
		}
		return closeErr
	}, nil
}

func useCache(repoURL string, branch string, cacheDir string, remoteHash string, targetDir string) error {
	log.Println("[useCache] checking cache for " + repoURL)

	repo, err := git.PlainOpen(cacheDir)
	if err != nil {
		log.Printf("[useCache] failed to open cache: %v", err)
		removeErr := removeCacheDir(cacheDir)
		if removeErr != nil {
			return removeErr
		}
		return ErrCacheUnavailable
	}

	ref, err := repo.Head()
	if err != nil {
		log.Printf("[useCache] failed to read cache HEAD: %v", err)
		removeErr := removeCacheDir(cacheDir)
		if removeErr != nil {
			return removeErr
		}
		return ErrCacheUnavailable
	}

	if ref.Hash().String() == remoteHash {
		log.Println("[useCache] cache hit for " + repoURL)
		return systemutil.CopyDir(cacheDir, targetDir)
	}

	log.Println("[useCache] cache stale, updating...")
	worktree, err := repo.Worktree()
	if err != nil {
		log.Printf("[useCache] failed to get worktree: %v", err)
		removeErr := removeCacheDir(cacheDir)
		if removeErr != nil {
			return removeErr
		}
		return ErrCacheUnavailable
	}

	branchRefName := plumbing.NewBranchReferenceName(branch)
	err = worktree.Checkout(&git.CheckoutOptions{
		Branch: branchRefName,
		Force:  true,
	})
	if err != nil {
		log.Printf("[useCache] failed to checkout branch %q in cache: %v", branch, err)
		removeErr := removeCacheDir(cacheDir)
		if removeErr != nil {
			return removeErr
		}
		return ErrCacheUnavailable
	}
	err = worktree.Pull(&git.PullOptions{
		RemoteName:    "origin",
		ReferenceName: branchRefName,
		SingleBranch:  true,
	})
	if err != nil && err != git.NoErrAlreadyUpToDate {
		log.Printf("[useCache] failed to pull cache: %v", err)
		removeErr := removeCacheDir(cacheDir)
		if removeErr != nil {
			return removeErr
		}
		return ErrCacheUnavailable
	}

	if err == git.NoErrAlreadyUpToDate {
		log.Println("[useCache] cache already up to date")
	} else {
		log.Println("[useCache] cache updated successfully")
	}

	ref, err = repo.Head()
	if err != nil {
		log.Printf("[useCache] failed to read cache HEAD after pull: %v", err)
		removeErr := removeCacheDir(cacheDir)
		if removeErr != nil {
			return removeErr
		}
		return ErrCacheUnavailable
	}

	log.Printf("[useCache] using cache at commit %s", ref.Hash().String())
	return systemutil.CopyDir(cacheDir, targetDir)
}

func cloneCache(repoURL string, branch string, cacheDir string) error {
	log.Println("[cloneCache] cloning cache for " + repoURL)

	cacheRoot := filepath.Dir(cacheDir)
	err := os.MkdirAll(cacheRoot, 0755)
	if err != nil {
		log.Printf("[cloneCache] failed to create cache root: %v", err)
		return err
	}

	log.Println("[cloneCache] cloning to cache " + repoURL)
	_, err = git.PlainClone(
		cacheDir,
		false,
		&git.CloneOptions{
			URL:           repoURL,
			Progress:      os.Stdout,
			SingleBranch:  true,
			Depth:         1,
			ReferenceName: plumbing.ReferenceName(fmt.Sprintf("refs/heads/%s", branch)),
		},
	)
	if err != nil {
		if err == git.ErrRepositoryAlreadyExists {
			log.Println("[cloneCache] cache already exists (created by another process)")
			return nil
		}
		errMsg := err.Error()
		if strings.Contains(errMsg, "repository not found") ||
			strings.Contains(errMsg, "Repository not found") ||
			strings.Contains(errMsg, "reference not found") ||
			strings.Contains(errMsg, "couldn't find remote ref") {
			log.Printf("[cloneCache] repository or branch not found: %v", err)
			return domain.ErrRepoOrBranchNotFound
		}
		log.Printf("[cloneCache] failed to clone cache: %v", err)
		return err
	}

	return nil
}
