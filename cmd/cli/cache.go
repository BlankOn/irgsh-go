package main

// Cache Management Strategy:
//
// This file implements a local git repository cache to speed up repeated builds
// and reduce network traffic. The cache works as follows:
//
// 1. Cache Directory Structure:
//    ~/.irgsh/cache/<cache-key>/
//    where <cache-key> is a SHA256 hash of "repoURL:branch"
//
// 2. Cache Key Generation:
//    The cache key is computed as: SHA256(repoURL + ":" + branch)
//    This ensures that:
//    - Different repositories get separate cache directories
//    - Different branches of the same repository get separate caches
//    - The cache key is filesystem-safe (64 hex characters)
//    - The cache key is collision-resistant
//
// 3. Cache Lifecycle:
//    - First access: Clone repository with depth=1 (shallow clone)
//    - Subsequent access: Pull updates if cache is stale
//    - Cache validation: Compare local HEAD with remote branch hash
//    - Cache invalidation: Remove and recreate if corrupted
//
// 4. Concurrency:
//    File-based locking (flock) prevents concurrent access to the same cache
//    directory, ensuring cache consistency across parallel builds.

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

// getRemoteHash queries the remote repository for the commit hash of the given branch.
// It uses 'git ls-remote' to fetch the commit hash without cloning the repository.
//
// The branch name is automatically prefixed with 'refs/heads/' if it's not already
// a full reference path.
//
// Returns errRepoOrBranchNotFound if the repository or branch cannot be found
// (git exits with code 128 or returns no matching refs).
func getRemoteHash(
	repoURL string,
	branch string,
) (string, error) {
	log.Printf("[getRemoteHash] getting remote hash for %s branch %s", repoURL, branch)

	ref := branch
	if !strings.HasPrefix(ref, "refs/") {
		ref = fmt.Sprintf("refs/heads/%s", branch)
	}
	cmd := exec.Command("git", "ls-remote", repoURL, ref)
	var out bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &stderr
	err := cmd.Run()
	if err != nil {
		err = fmt.Errorf("git ls-remote: %w: %s", err, stderr.String())
		log.Printf("[getRemoteHash] %v", err)
		return "", errRepoOrBranchNotFound
	}
	parts := strings.Fields(out.String())
	if len(parts) > 0 {
		return parts[0], nil
	}
	return "", errRepoOrBranchNotFound
}

// removeCacheDir deletes the cache directory and all its contents.
// It is called when a cache is corrupted or git operations fail.
//
// The caller should hold the cache lock before calling this function.
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

// lockCacheDir acquires an exclusive lock for the cache directory and returns
// an unlock function. It uses flock to prevent concurrent access to the same cache.
//
// The lock file is named <cacheDir>.lock. The function blocks until the lock
// is acquired. The returned unlock function must be called to release the lock:
//
//	unlock, err := lockCacheDir(cacheDir)
//	if err != nil { return err }
//	defer unlock()
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
		closeErr := lockFile.Close()
		if closeErr != nil {
			return nil, fmt.Errorf("failed to acquire lock: %w (close error: %v)", err, closeErr)
		}
		return nil, err
	}

	log.Println("[lockCacheDir] acquired cache lock: " + cacheDir)

	return func() error {
		log.Println("[lockCacheDir] releasing cache lock: " + cacheDir)
		unlockErr := syscall.Flock(int(lockFile.Fd()), syscall.LOCK_UN)
		closeErr := lockFile.Close()

		// Return both errors if they both occurred
		if unlockErr != nil && closeErr != nil {
			return fmt.Errorf("failed to unlock: %w (close error: %v)", unlockErr, closeErr)
		}
		if unlockErr != nil {
			return unlockErr
		}
		return closeErr
	}, nil
}

// useCache validates the cache and copies it to targetDir if current.
// If the cache is stale, it pulls updates before copying.
//
// The function compares the local HEAD commit hash with remoteHash. If they match,
// the cache is copied immediately. If they differ, the cache is updated via
// checkout and pull, then copied.
//
// Returns errCacheUnavailable if any git operation fails (open, checkout, pull),
// causing the cache to be removed. The caller will then clone a fresh copy.
func useCache(
	repoURL string,
	branch string,
	cacheDir string,
	remoteHash string,
	targetDir string,
) error {
	log.Println("[useCache] checking cache for " + repoURL)

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
		log.Println("[useCache] cache hit for " + repoURL)
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
		return errCacheUnavailable
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

// cloneCache creates a shallow clone (depth=1) of the repository in the cache directory.
//
// The function does not check if the cache exists before cloning. If another process
// has already created the cache (git.ErrRepositoryAlreadyExists), this is treated as
// success. This approach avoids TOCTOU race conditions.
//
// Returns errRepoOrBranchNotFound if the repository or branch doesn't exist,
// allowing the caller to implement fallback behavior.
func cloneCache(
	repoURL string,
	branch string,
	cacheDir string,
) error {
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
			return errRepoOrBranchNotFound
		}
		log.Printf("[cloneCache] failed to clone cache: %v", err)
		return err
	}

	return nil
}

// syncRepo synchronizes targetDir with the remote repository using a local cache.
//
// The cache key is a SHA256 hash of "repoURL:branch", ensuring unique, filesystem-safe
// cache directories. For example:
//
//	repoURL: https://github.com/user/repo.git
//	branch:  main
//	cache:   ~/.irgsh/cache/a3b5c7d9.../
//
// The function first attempts to use an existing cache (updating if stale).
// If the cache is unavailable or corrupted, it clones a fresh copy.
func syncRepo(
	repoURL string,
	branch string,
	homeDir string,
	targetDir string,
) error {
	log.Println("[syncRepo] syncing repo " + repoURL + " branch " + branch)

	hasher := sha256.New()
	hasher.Write([]byte(repoURL + ":" + branch))
	cacheKey := hex.EncodeToString(hasher.Sum(nil))
	cacheDir := filepath.Join(homeDir, ".irgsh", "cache", cacheKey)

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
	if !errors.Is(err, errCacheUnavailable) {
		return err
	}

	log.Println("[syncRepo] cache unavailable, cloning fresh copy")
	err = cloneCache(repoURL, branch, cacheDir)
	if err != nil {
		return err
	}

	return useCache(repoURL, branch, cacheDir, remoteHash, targetDir)
}
