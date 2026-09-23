package main

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/blankon/irgsh-go/pkg/systemutil"
)

func collectArtifacts(job buildJob, attempt attemptPaths, source sourceSet) ([]string, error) {
	if err := os.MkdirAll(job.Artifacts, 0755); err != nil {
		return nil, fmt.Errorf("create artifact directory: %w", err)
	}
	entries, err := os.ReadDir(job.Artifacts)
	if err != nil {
		return nil, fmt.Errorf("read artifact directory: %w", err)
	}
	for _, entry := range entries {
		if entry.Name() != "build.log" {
			if err := os.RemoveAll(filepath.Join(job.Artifacts, entry.Name())); err != nil {
				return nil, fmt.Errorf("remove stale artifact %q: %w", entry.Name(), err)
			}
		}
	}
	if err := validateSource(attempt.Input, source); err != nil {
		return nil, fmt.Errorf("validate artifact source: %w", err)
	}
	paths := make(map[string]string)
	for _, name := range source.Files {
		if name == "build.log" {
			return nil, fmt.Errorf("source member collides with build.log")
		}
		paths[name] = filepath.Join(attempt.Input, name)
	}
	entries, err = os.ReadDir(attempt.Result)
	if err != nil {
		return nil, fmt.Errorf("read build result: %w", err)
	}
	hasBinary, hasBuildinfo := false, false
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			return nil, fmt.Errorf("inspect build result %q: %w", entry.Name(), err)
		}
		if !info.Mode().IsRegular() {
			return nil, fmt.Errorf("build result %q is not a regular file", entry.Name())
		}
		name := entry.Name()
		switch filepath.Ext(name) {
		case ".deb", ".udeb", ".ddeb":
			hasBinary = true
		case ".buildinfo":
			hasBuildinfo = true
		case ".changes":
		default:
			continue
		}
		if _, exists := paths[name]; exists {
			return nil, fmt.Errorf("source and build result collide at %q", name)
		}
		paths[name] = filepath.Join(attempt.Result, name)
	}
	if !hasBinary || !hasBuildinfo {
		return nil, fmt.Errorf("build result requires a binary package and .buildinfo")
	}
	names := make([]string, 0, len(paths))
	for name, original := range paths {
		target := filepath.Join(job.Artifacts, name)
		if err := systemutil.CopyFile(original, target, 0644); err != nil {
			return nil, fmt.Errorf("copy artifact %q: %w", name, err)
		}
		if err := os.Chmod(target, 0644); err != nil {
			return nil, fmt.Errorf("set artifact mode %q: %w", name, err)
		}
		names = append(names, name)
	}
	sort.Strings(names)
	return names, nil
}

func writeArtifactArchive(job buildJob, names []string) (err error) {
	if err := os.Remove(job.Archive); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove previous artifact archive: %w", err)
	}
	file, err := os.OpenFile(job.Archive, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return fmt.Errorf("create artifact archive: %w", err)
	}
	compressed := gzip.NewWriter(file)
	writer := tar.NewWriter(compressed)
	defer func() {
		err = errors.Join(err, writer.Close(), compressed.Close())
		if err == nil {
			err = file.Sync()
		}
		err = errors.Join(err, file.Close())
		if err != nil {
			err = errors.Join(err, os.Remove(job.Archive))
		}
	}()
	seen := make(map[string]bool)
	for _, name := range names {
		if name == "." || name == ".." || name == "build.log" || filepath.Base(name) != name || strings.Contains(name, `\`) || seen[name] {
			return fmt.Errorf("invalid artifact name %q", name)
		}
		seen[name] = true
		filename := filepath.Join(job.Artifacts, name)
		info, err := os.Lstat(filename)
		if err != nil {
			return fmt.Errorf("inspect artifact %q: %w", name, err)
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("artifact %q is not a regular file", name)
		}
		if err := writer.WriteHeader(&tar.Header{Name: job.Submission.TaskUUID + "/" + name, Mode: 0644, Size: info.Size(), Typeflag: tar.TypeReg}); err != nil {
			return fmt.Errorf("write artifact header %q: %w", name, err)
		}
		input, err := os.Open(filename)
		if err != nil {
			return fmt.Errorf("open artifact %q: %w", name, err)
		}
		_, copyErr := io.Copy(writer, input)
		if err := errors.Join(copyErr, input.Close()); err != nil {
			return fmt.Errorf("archive artifact %q: %w", name, err)
		}
	}
	return nil
}

func chiefEndpoint(address string, endpointPath string, query url.Values) (string, error) {
	endpoint, err := url.Parse(address)
	if err != nil {
		return "", fmt.Errorf("parse chief address: %w", err)
	}
	if (endpoint.Scheme != "http" && endpoint.Scheme != "https") || endpoint.Host == "" {
		return "", fmt.Errorf("chief address requires an http or https scheme and host")
	}
	endpoint.Path = endpointPath
	endpoint.RawPath = ""
	endpoint.RawQuery = query.Encode()
	return endpoint.String(), nil
}

func uploadFile(ctx context.Context, client *http.Client, endpoint string, field string, filePath string) (err error) {
	file, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("open upload file: %w", err)
	}
	defer func() { err = errors.Join(err, file.Close()) }()
	reader, writer := io.Pipe()
	defer reader.Close()
	multipartWriter := multipart.NewWriter(writer)
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, reader)
	if err != nil {
		_ = writer.Close()
		return fmt.Errorf("create upload request: %w", err)
	}
	request.Header.Set("Content-Type", multipartWriter.FormDataContentType())
	done := make(chan error, 1)
	go func() {
		part, writeErr := multipartWriter.CreateFormFile(field, filepath.Base(filePath))
		if writeErr == nil {
			_, writeErr = io.Copy(part, file)
		}
		writeErr = errors.Join(writeErr, multipartWriter.Close())
		_ = writer.CloseWithError(writeErr)
		done <- writeErr
	}()
	uploadClient := *client
	uploadClient.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	response, requestErr := uploadClient.Do(request)
	_ = reader.CloseWithError(requestErr)
	writeErr := <-done
	if requestErr != nil {
		return fmt.Errorf("upload request: %w", errors.Join(requestErr, writeErr))
	}
	defer func() { err = errors.Join(err, response.Body.Close()) }()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		body, readErr := io.ReadAll(io.LimitReader(response.Body, 4096))
		return errors.Join(fmt.Errorf("upload: HTTP %d: %s", response.StatusCode, strings.TrimSpace(string(body))), writeErr, readErr)
	}
	_, readErr := io.Copy(io.Discard, response.Body)
	if err := errors.Join(writeErr, readErr, ctx.Err()); err != nil {
		return fmt.Errorf("upload file: %w", err)
	}
	return nil
}
