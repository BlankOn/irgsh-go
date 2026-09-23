package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
)

func includeBinaries(repository, artifacts, dist, component string, env []string, logPath string, run func(context.Context, string, []string, []string, string, string, string) (string, error)) error {
	repository, err := filepath.Abs(repository)
	if err != nil {
		return fmt.Errorf("resolve repository: %w", err)
	}
	artifacts, err = filepath.Abs(artifacts)
	if err != nil {
		return fmt.Errorf("resolve artifacts: %w", err)
	}
	entries, err := os.ReadDir(artifacts)
	if err != nil {
		return fmt.Errorf("read binary artifacts: %w", err)
	}
	files := map[string][]string{}
	for _, entry := range entries {
		extension := filepath.Ext(entry.Name())
		if extension != ".deb" && extension != ".udeb" && extension != ".ddeb" {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			return fmt.Errorf("inspect binary artifact %q: %w", entry.Name(), err)
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("binary artifact %q is not a regular file", entry.Name())
		}
		files[extension] = append(files[extension], filepath.Join(artifacts, entry.Name()))
	}
	if len(files) == 0 {
		return fmt.Errorf("no binary artifacts to include")
	}
	if err := os.MkdirAll(repository, 0755); err != nil {
		return fmt.Errorf("create repository directory: %w", err)
	}
	for _, extension := range []string{".deb", ".udeb", ".ddeb"} {
		if len(files[extension]) == 0 {
			continue
		}
		args := []string{"--basedir", repository, "-v", "-v", "-v", "--nothingiserror", "--component", component}
		command := "includedeb"
		if extension == ".udeb" {
			command = "includeudeb"
		}
		if extension == ".ddeb" {
			args = append(args, "--ignore=extension")
		}
		args = append(args, command, dist)
		args = append(args, files[extension]...)
		if _, err := run(context.Background(), "reprepro", args, env, repository, "Injecting "+extension+" files from artifact to the repository", logPath); err != nil {
			return fmt.Errorf("include %s artifacts: %w", extension, err)
		}
	}
	return nil
}
