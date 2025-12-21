package main

import (
	"bytes"
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

	"gopkg.in/src-d/go-git.v4"
	"gopkg.in/src-d/go-git.v4/plumbing"
)

var (
	errCacheUnavailable     = errors.New("cache unavailable")
	errRepoOrBranchNotFound = errors.New("repo or branch not found")
)

// getRemoteHash queries the remote repository for the commit hash at a branch ref.
func getRemoteHash(
	repoUrl string,
	branch string,
) (string, error) {
	log.Printf("[getRemoteHash] getting remote hash for %s branch %s", repoUrl, branch)

	ref := branch
	if !strings.HasPrefix(ref, "refs/") {
		ref = fmt.Sprintf("refs/heads/%s", branch)
	}
	cmd := exec.Command("git", "ls-remote", repoUrl, ref)
	var out bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &stderr
	err := cmd.Run()
	if err != nil {
		err = fmt.Errorf("git ls-remote: %w: %s", err, stderr.String())
		log.Printf("[getRemoteHash] %v", err)
		return "", err
	}
	parts := strings.Fields(out.String())
	if len(parts) > 0 {
		return parts[0], nil
	}
	return "", errRepoOrBranchNotFound
}

// cacheDirExists reports whether the cache directory exists.
func cacheDirExists(
	cacheDir string,
) (bool, error) {
	log.Println("[cacheDirExists] checking if cache dir exists: " + cacheDir)

	info, err := os.Stat(cacheDir)
	if err == nil {
		if !info.IsDir() {
			return false, fmt.Errorf("cache path exists but is not a directory: %s", cacheDir)
		}
		return true, nil
	}

	if os.IsNotExist(err) {
		return false, nil
	}

	return false, err
}

// removeCacheDir deletes the cache directory and its contents.
func removeCacheDir(
	cacheDir string,
) error {
	log.Println("[removeCacheDir] removing cache dir: " + cacheDir)

	err := os.RemoveAll(cacheDir)
	if err != nil {
		log.Printf("[removeCacheDir] failed to remove cache dir: %v", err)
		return err
	}

	return nil
}

// lockCacheDir acquires an exclusive lock for a cache directory.
func lockCacheDir(
	cacheDir string,
) (func() error, error) {
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
		lockFile.Close()
		return nil, err
	}

	log.Println("[lockCacheDir] acquired cache lock: " + cacheDir)

	return func() error {
		log.Println("[lockCacheDir] releasing cache lock: " + cacheDir)
		if err := syscall.Flock(int(lockFile.Fd()), syscall.LOCK_UN); err != nil {
			lockFile.Close()
			return err
		}
		return lockFile.Close()
	}, nil
}

// useCache checks the cache and copies it to targetDir if it is current.
func useCache(
	repoUrl string,
	branch string,
	cacheDir string,
	remoteHash string,
	targetDir string,
) error {
	log.Println("[useCache] checking cache for " + repoUrl)

	exists, err := cacheDirExists(cacheDir)
	if err != nil {
		log.Printf("[useCache] failed to stat cache dir: %v", err)
		return err
	}
	if !exists {
		log.Printf("[useCache] cache dir not found: %s", cacheDir)
		return errCacheUnavailable
	}

	repo, err := git.PlainOpen(cacheDir)
	if err != nil {
		log.Printf("[useCache] failed to open cache: %v", err)
		removeErr := removeCacheDir(cacheDir)
		if removeErr != nil {
			return removeErr
		}
		return errCacheUnavailable
	}

	ref, err := repo.Head()
	if err != nil {
		log.Printf("[useCache] failed to read cache HEAD: %v", err)
		removeErr := removeCacheDir(cacheDir)
		if removeErr != nil {
			return removeErr
		}
		return errCacheUnavailable
	}

	if ref.Hash().String() == remoteHash {
		log.Println("[useCache] cache hit for " + repoUrl)
		err = copyDir(cacheDir, targetDir)
		if err != nil {
			return err
		}
		return nil
	}

	log.Println("[useCache] cache stale, updating...")
	worktree, err := repo.Worktree()
	if err != nil {
		log.Printf("[useCache] failed to get worktree: %v", err)
		removeErr := removeCacheDir(cacheDir)
		if removeErr != nil {
			return removeErr
		}
		return errCacheUnavailable
	}

	err = worktree.Pull(&git.PullOptions{
		RemoteName:    "origin",
		ReferenceName: plumbing.ReferenceName(fmt.Sprintf("refs/heads/%s", branch)),
		SingleBranch:  true,
		Depth:         1,
	})
	if err != nil && err != git.NoErrAlreadyUpToDate {
		log.Printf("[useCache] failed to pull cache: %v", err)
		removeErr := removeCacheDir(cacheDir)
		if removeErr != nil {
			return removeErr
		}
		return errCacheUnavailable
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
		return errCacheUnavailable
	}

	log.Printf("[useCache] using cache at commit %s", ref.Hash().String())

	err = copyDir(cacheDir, targetDir)
	if err != nil {
		return err
	}

	return nil
}

// cloneCache clones the repository into a local cache if it does not exist.
func cloneCache(
	repoUrl string,
	branch string,
	cacheDir string,
) error {
	log.Println("[cloneCache] cloning cache for " + repoUrl)

	exists, err := cacheDirExists(cacheDir)
	if err != nil {
		return err
	}
	if exists {
		log.Println("[cloneCache] cache already exists, skipping clone")
		return nil
	}

	cacheRoot := filepath.Dir(cacheDir)
	err = os.MkdirAll(cacheRoot, 0755)
	if err != nil {
		log.Printf("[cloneCache] failed to create cache root: %v", err)
		return err
	}

	log.Println("[cloneCache] cloning to cache " + repoUrl)
	_, err = git.PlainClone(
		cacheDir,
		false,
		&git.CloneOptions{
			URL:           repoUrl,
			Progress:      os.Stdout,
			SingleBranch:  true,
			Depth:         1,
			ReferenceName: plumbing.ReferenceName(fmt.Sprintf("refs/heads/%s", branch)),
		},
	)
	if err != nil {
		log.Printf("[cloneCache] failed to clone cache: %v", err)
		return err
	}

	return nil
}

// syncRepo keeps targetDir synced with the remote repository using a cache.
func syncRepo(
	repoUrl string,
	branch string,
	homeDir string,
	targetDir string,
) error {
	log.Println("[syncRepo] syncing repo " + repoUrl + " branch " + branch)

	hasher := sha256.New()
	hasher.Write([]byte(repoUrl))
	cacheKey := hex.EncodeToString(hasher.Sum(nil))
	cacheDir := filepath.Join(homeDir, ".irgsh", "cache", cacheKey)

	remoteHash, err := getRemoteHash(repoUrl, branch)
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

	err = useCache(repoUrl, branch, cacheDir, remoteHash, targetDir)
	if err == nil {
		return nil
	}
	if !errors.Is(err, errCacheUnavailable) {
		return err
	}

	log.Println("[syncRepo] cache unavailable, cloning fresh copy")
	err = cloneCache(repoUrl, branch, cacheDir)
	if err != nil {
		return err
	}

	return useCache(repoUrl, branch, cacheDir, remoteHash, targetDir)
}
