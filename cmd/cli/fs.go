package main

import (
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
)

// resetDir removes any existing destination directory and recreates it.
func resetDir(
	dir string,
	mode os.FileMode,
) error {
	_, err := os.Stat(dir)
	if err == nil {
		if err := os.RemoveAll(dir); err != nil {
			return err
		}
	} else if !os.IsNotExist(err) {
		return err
	}

	return os.MkdirAll(dir, mode)
}

// copyDir copies the contents of src into dst.
func copyDir(
	src string,
	dst string,
) error {
	log.Printf("[copyDir] copying dir from %s to %s", src, dst)

	srcInfo, err := os.Stat(src)
	if err != nil {
		return fmt.Errorf("[copyDir] failed to stat source dir: %w", err)
	}
	if !srcInfo.IsDir() {
		return fmt.Errorf("[copyDir] source is not a directory: %s", src)
	}

	err = resetDir(dst, srcInfo.Mode())
	if err != nil {
		return fmt.Errorf("[copyDir] failed to prepare destination dir: %w", err)
	}

	return filepath.Walk(src, func(currentPath string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relPath, err := filepath.Rel(src, currentPath)
		if err != nil {
			return err
		}
		if relPath == "." {
			return nil
		}

		targetPath := filepath.Join(dst, relPath)
		if info.IsDir() {
			return os.MkdirAll(targetPath, info.Mode())
		}
		if info.Mode()&os.ModeSymlink != 0 {
			linkTarget, err := os.Readlink(currentPath)
			if err != nil {
				return err
			}
			return os.Symlink(linkTarget, targetPath)
		}

		return copyFile(currentPath, targetPath, info.Mode())
	})
}

// copyFile copies a file from src to dst with the provided mode.
func copyFile(
	src string,
	dst string,
	mode os.FileMode,
) (err error) {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	srcInfo, err := in.Stat()
	if err != nil {
		return fmt.Errorf("failed to stat source file: %w", err)
	}

	out, err := os.OpenFile(dst, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, mode)
	if err != nil {
		return err
	}
	defer func() {
		if closeErr := out.Close(); closeErr != nil && err == nil {
			err = closeErr
		}
	}()

	written, err := io.Copy(out, in)
	if err != nil {
		return err
	}

	if written != srcInfo.Size() {
		return fmt.Errorf("incomplete copy: wrote %d bytes, expected %d bytes", written, srcInfo.Size())
	}

	if err := out.Sync(); err != nil {
		return err
	}

	return nil
}
