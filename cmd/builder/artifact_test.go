package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/blankon/irgsh-go/internal/config"
)

func artifactFixture(t *testing.T) (buildJob, attemptPaths, sourceSet) {
	t.Helper()
	job, err := newBuildJob(t.TempDir(), buildSubmission{TaskUUID: "job-123"})
	if err != nil {
		t.Fatal(err)
	}
	attempt, err := job.newAttempt(1)
	if err != nil {
		t.Fatal(err)
	}
	root, _, _ := sourceFixture(t, "hello_1.0.orig.tar.xz", []byte("source bytes"), "signed")
	source, err := prepareSource(context.Background(), root, attempt.Input)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(job.Artifacts, 0755); err != nil {
		t.Fatal(err)
	}
	writeArtifactFixture(t, job.Log, "build log")
	writeArtifactFixture(t, filepath.Join(attempt.Result, "hello_1.0_amd64.deb"), "binary bytes")
	writeArtifactFixture(t, filepath.Join(attempt.Result, "hello_1.0_amd64.buildinfo"), "build info")
	return job, attempt, source
}

func writeArtifactFixture(t *testing.T, filename, content string) {
	t.Helper()
	if err := os.WriteFile(filename, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
}

func TestCollectArtifactsAcceptsDebUdebDdebBuildinfoAndChanges(t *testing.T) {
	job, attempt, source := artifactFixture(t)
	for _, name := range []string{"hello_1.0_amd64.udeb", "hello_1.0_amd64.ddeb", "hello_1.0_amd64.changes", "ignored.txt"} {
		writeArtifactFixture(t, filepath.Join(attempt.Result, name), name)
	}
	if err := os.Mkdir(filepath.Join(attempt.Result, "nested"), 0755); err != nil {
		t.Fatal(err)
	}
	writeArtifactFixture(t, filepath.Join(attempt.Result, "nested", "ignored.deb"), "nested binary")
	names, err := collectArtifacts(context.Background(), job, attempt, source)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"hello_1.0.dsc", "hello_1.0.orig.tar.xz", "hello_1.0_amd64.buildinfo", "hello_1.0_amd64.changes", "hello_1.0_amd64.ddeb", "hello_1.0_amd64.deb", "hello_1.0_amd64.udeb"}
	if !reflect.DeepEqual(names, want) {
		t.Fatalf("artifacts = %v, want %v", names, want)
	}
	for _, name := range names {
		info, err := os.Stat(filepath.Join(job.Artifacts, name))
		if err != nil || !info.Mode().IsRegular() || info.Mode().Perm() != 0644 {
			t.Fatalf("artifact %s: %v, %v", name, info, err)
		}
	}
	content, err := os.ReadFile(filepath.Join(job.Artifacts, "hello_1.0.orig.tar.xz"))
	if err != nil || string(content) != "source bytes" {
		t.Fatalf("source artifact = %q, %v", content, err)
	}
}

func TestCollectArtifactsRequiresBinary(t *testing.T) {
	job, attempt, source := artifactFixture(t)
	if err := os.Remove(filepath.Join(attempt.Result, "hello_1.0_amd64.deb")); err != nil {
		t.Fatal(err)
	}
	if _, err := collectArtifacts(context.Background(), job, attempt, source); err == nil {
		t.Fatal("accepted output without a binary")
	}
}

func TestCollectArtifactsRequiresBuildinfo(t *testing.T) {
	job, attempt, source := artifactFixture(t)
	if err := os.Remove(filepath.Join(attempt.Result, "hello_1.0_amd64.buildinfo")); err != nil {
		t.Fatal(err)
	}
	if _, err := collectArtifacts(context.Background(), job, attempt, source); err == nil {
		t.Fatal("accepted output without buildinfo")
	}
}

func TestCollectArtifactsRejectsSymlink(t *testing.T) {
	for _, name := range []string{"linked.deb", "hello_1.0_amd64.build"} {
		t.Run(name, func(t *testing.T) {
			job, attempt, source := artifactFixture(t)
			if err := os.Symlink(job.Log, filepath.Join(attempt.Result, name)); err != nil {
				t.Fatal(err)
			}
			if _, err := collectArtifacts(context.Background(), job, attempt, source); err == nil {
				t.Fatal("accepted a symlink artifact")
			}
		})
	}
}

func TestCollectArtifactsRejectsSourceChecksumChange(t *testing.T) {
	job, attempt, source := artifactFixture(t)
	filename := filepath.Join(attempt.Input, "hello_1.0.orig.tar.xz")
	if err := os.Chmod(filename, 0644); err != nil {
		t.Fatal(err)
	}
	writeArtifactFixture(t, filename, "source Bytes")
	if _, err := collectArtifacts(context.Background(), job, attempt, source); err == nil {
		t.Fatal("accepted changed source bytes")
	}
}

func TestCollectArtifactsRejectsNameCollision(t *testing.T) {
	job, attempt, _ := artifactFixture(t)
	root, input, _ := sourceFixture(t, "hello_1.0_amd64.deb", []byte("source bytes"), "signed")
	source, err := prepareSource(context.Background(), root, input)
	if err != nil {
		t.Fatal(err)
	}
	attempt.Input = input
	if _, err := collectArtifacts(context.Background(), job, attempt, source); err == nil {
		t.Fatal("accepted colliding source and binary names")
	}
}

func TestCollectArtifactsRemovesStaleRootArtifacts(t *testing.T) {
	job, attempt, source := artifactFixture(t)
	writeArtifactFixture(t, filepath.Join(job.Artifacts, "stale.deb"), "stale")
	if err := os.Mkdir(filepath.Join(job.Artifacts, "stale"), 0755); err != nil {
		t.Fatal(err)
	}
	writeArtifactFixture(t, filepath.Join(job.Artifacts, "stale", "old.deb"), "stale")
	if _, err := collectArtifacts(context.Background(), job, attempt, source); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"stale.deb", "stale"} {
		if _, err := os.Stat(filepath.Join(job.Artifacts, name)); !os.IsNotExist(err) {
			t.Fatalf("stale artifact %s remains: %v", name, err)
		}
	}
	content, err := os.ReadFile(job.Log)
	if err != nil || string(content) != "build log" {
		t.Fatalf("build log = %q, %v", content, err)
	}
}

func readArtifactArchive(t *testing.T, filename string) map[string]string {
	t.Helper()
	file, err := os.Open(filename)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	gzipReader, err := gzip.NewReader(file)
	if err != nil {
		t.Fatal(err)
	}
	defer gzipReader.Close()
	reader := tar.NewReader(gzipReader)
	members := make(map[string]string)
	for {
		header, err := reader.Next()
		if err == io.EOF {
			return members
		}
		if err != nil {
			t.Fatal(err)
		}
		if header.Typeflag != tar.TypeReg || header.Mode != 0644 {
			t.Fatalf("unexpected artifact header: %+v", header)
		}
		if _, exists := members[header.Name]; exists {
			t.Fatalf("duplicate archive member %q", header.Name)
		}
		content, err := io.ReadAll(reader)
		if err != nil {
			t.Fatal(err)
		}
		members[header.Name] = string(content)
	}
}

func TestWriteArtifactArchiveHasOneJobDirectory(t *testing.T) {
	job, attempt, source := artifactFixture(t)
	names, err := collectArtifacts(context.Background(), job, attempt, source)
	if err != nil {
		t.Fatal(err)
	}
	if err := writeArtifactArchive(context.Background(), job, names); err != nil {
		t.Fatal(err)
	}
	members := readArtifactArchive(t, job.Archive)
	want := map[string]string{
		"job-123/hello_1.0.dsc":             "",
		"job-123/hello_1.0.orig.tar.xz":     "source bytes",
		"job-123/hello_1.0_amd64.deb":       "binary bytes",
		"job-123/hello_1.0_amd64.buildinfo": "build info",
	}
	dsc, err := os.ReadFile(source.DSC)
	if err != nil {
		t.Fatal(err)
	}
	want["job-123/hello_1.0.dsc"] = string(dsc)
	if !reflect.DeepEqual(members, want) {
		t.Fatalf("archive = %v, want %v", members, want)
	}
	info, err := os.Stat(job.Archive)
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatalf("archive mode: %v, %v", info, err)
	}
}

func TestWriteArtifactArchiveExcludesBuildLogAndScratch(t *testing.T) {
	job, attempt, source := artifactFixture(t)
	names, err := collectArtifacts(context.Background(), job, attempt, source)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"jobs", "input", "result", "tmp"} {
		if err := os.Mkdir(filepath.Join(job.Artifacts, name), 0755); err != nil {
			t.Fatal(err)
		}
		writeArtifactFixture(t, filepath.Join(job.Artifacts, name, "ignored.deb"), "scratch")
	}
	if err := writeArtifactArchive(context.Background(), job, names); err != nil {
		t.Fatal(err)
	}
	members := readArtifactArchive(t, job.Archive)
	if len(members) != 4 {
		t.Fatalf("archive has %d members, want 4", len(members))
	}
	for name := range members {
		if !strings.HasPrefix(name, "job-123/") || strings.Count(name, "/") != 1 || filepath.Base(name) == "build.log" {
			t.Fatalf("unexpected archive member %q", name)
		}
	}
}

func TestWriteArtifactArchiveRemovesPartialOutput(t *testing.T) {
	job, _, _ := artifactFixture(t)
	writeArtifactFixture(t, job.Archive, "old archive")
	if err := writeArtifactArchive(context.Background(), job, []string{"missing.deb"}); err == nil {
		t.Fatal("archived a missing file")
	}
	if _, err := os.Stat(job.Archive); !os.IsNotExist(err) {
		t.Fatalf("partial archive remains: %v", err)
	}
}

func TestCollectArtifactsCancelsDuringCopy(t *testing.T) {
	job, attempt, source := artifactFixture(t)
	writeArtifactFixture(t, filepath.Join(attempt.Result, "hello_1.0_amd64.deb"), strings.Repeat("x", 256*1024))
	ctx, written := cancelOnWrite(t, filepath.Join(job.Artifacts, "hello_1.0_amd64.deb"))
	if _, err := collectArtifacts(ctx, job, attempt, source); !errors.Is(err, context.Canceled) {
		t.Fatalf("artifact collection error = %v, want context.Canceled", err)
	}
	if *written <= 0 || *written >= 256*1024 {
		t.Fatalf("artifact copy canceled after %d bytes, want a partial copy", *written)
	}
}

func TestWriteArtifactArchiveCancelsDuringCompression(t *testing.T) {
	job, attempt, source := artifactFixture(t)
	names, err := collectArtifacts(context.Background(), job, attempt, source)
	if err != nil {
		t.Fatal(err)
	}
	ctx, _ := cancelOnWrite(t, job.Archive)
	if err := writeArtifactArchive(ctx, job, names); !errors.Is(err, context.Canceled) {
		t.Fatalf("archive error = %v, want context.Canceled", err)
	}
	if _, err := os.Stat(job.Archive); !os.IsNotExist(err) {
		t.Fatalf("canceled artifact archive remains: %v", err)
	}
}

func TestChiefEndpointReplacesPathAndQuery(t *testing.T) {
	endpoint, err := chiefEndpoint("https://chief.example/old%20path?old=true", "/api/v1/artifact-upload", url.Values{"id": {"job + 123"}})
	if err != nil || endpoint != "https://chief.example/api/v1/artifact-upload?id=job+%2B+123" {
		t.Fatalf("endpoint = %q, %v", endpoint, err)
	}
	for _, address := range []string{"chief.example", "ftp://chief.example", "https:///missing", "://bad"} {
		if endpoint, err := chiefEndpoint(address, "/upload", nil); err == nil {
			t.Fatalf("accepted chief address %q: %q", address, endpoint)
		}
	}
}

func TestUploadFileSendsExpectedMultipartField(t *testing.T) {
	filePath := filepath.Join(t.TempDir(), "package.tar.gz")
	content := "artifact bytes\x00\xff"
	writeArtifactFixture(t, filePath, content)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/artifact-upload" || r.URL.Query().Get("id") != "job + 123" || r.URL.Query().Get("type") != "build" {
			t.Errorf("unexpected upload request: %s %s", r.Method, r.URL)
		}
		reader, err := r.MultipartReader()
		if err != nil {
			t.Error(err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		part, err := reader.NextPart()
		if err != nil {
			t.Error(err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		body, err := io.ReadAll(part)
		if err != nil || part.FormName() != "uploadFile" || part.FileName() != "package.tar.gz" || string(body) != content {
			t.Errorf("multipart file = %q, %q, %q, %v", part.FormName(), part.FileName(), body, err)
		}
		if _, err := reader.NextPart(); err != io.EOF {
			t.Errorf("extra multipart data: %v", err)
		}
		w.WriteHeader(http.StatusCreated)
	}))
	defer server.Close()
	endpoint, err := chiefEndpoint(server.URL, "/api/v1/artifact-upload", url.Values{"id": {"job + 123"}, "type": {"build"}})
	if err != nil {
		t.Fatal(err)
	}
	if err := uploadFile(context.Background(), server.Client(), endpoint, "uploadFile", filePath); err != nil {
		t.Fatal(err)
	}
}

func TestUploadFileRejectsNon2xx(t *testing.T) {
	filePath := filepath.Join(t.TempDir(), "package.tar.gz")
	writeArtifactFixture(t, filePath, "artifact bytes")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = io.WriteString(w, "upload rejected "+strings.Repeat("x", 8192))
	}))
	defer server.Close()
	err := uploadFile(context.Background(), server.Client(), server.URL, "uploadFile", filePath)
	if err == nil || !strings.Contains(err.Error(), "500") || !strings.Contains(err.Error(), "upload rejected") || len(err.Error()) > 4300 {
		t.Fatalf("upload error = %v", err)
	}
}

func TestUploadFileRejectsRedirectWithoutPostingArtifact(t *testing.T) {
	filePath := filepath.Join(t.TempDir(), "package.tar.gz")
	writeArtifactFixture(t, filePath, "artifact bytes")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		if r.URL.Path == "/upload" {
			http.Redirect(w, r, "/login", http.StatusFound)
		}
	}))
	defer server.Close()
	err := uploadFile(context.Background(), server.Client(), server.URL+"/upload", "uploadFile", filePath)
	if err == nil || !strings.Contains(err.Error(), "302") {
		t.Fatalf("redirected upload error = %v", err)
	}
}

func TestUploadFileHonorsCancellation(t *testing.T) {
	filePath := filepath.Join(t.TempDir(), "package.tar.gz")
	writeArtifactFixture(t, filePath, "artifact bytes")
	started := make(chan struct{})
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		close(started)
		<-release
	}))
	defer server.Close()
	defer close(release)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- uploadFile(ctx, server.Client(), server.URL, "uploadFile", filePath) }()
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("upload did not reach server")
	}
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("upload error = %v, want context.Canceled", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("upload ignored cancellation")
	}
}

func TestUploadFilePropagatesFileAndResponseErrors(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		w.Header().Set("Content-Length", "100")
		_, _ = io.WriteString(w, "short")
	}))
	defer server.Close()
	filePath := filepath.Join(t.TempDir(), "package.tar.gz")
	if err := uploadFile(context.Background(), server.Client(), server.URL, "uploadFile", filePath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("missing file error = %v", err)
	}
	writeArtifactFixture(t, filePath, "artifact bytes")
	if err := uploadFile(context.Background(), server.Client(), server.URL, "uploadFile", filePath); !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("response error = %v", err)
	}
}

func buildFlowFixture(t *testing.T) (buildJob, buildSteps) {
	t.Helper()
	job, err := newBuildJob(t.TempDir(), buildSubmission{TaskUUID: "job-123"})
	if err != nil {
		t.Fatal(err)
	}
	root, _, _ := sourceFixture(t, "hello_1.0.orig.tar.xz", []byte("source bytes"), "signed")
	return job, buildSteps{
		Prepare: func(ctx context.Context, got buildJob, attempt attemptPaths) (sourceSet, error) {
			if got != job || attempt.Number != 1 || ctx.Err() != nil {
				t.Fatalf("unexpected preparation input: %+v, %+v, %v", got, attempt, ctx.Err())
			}
			return prepareSource(ctx, root, attempt.Input)
		},
		Build: func(ctx context.Context, got buildJob, attempt attemptPaths, source sourceSet) (attemptPaths, error) {
			if got != job || source.DSC != filepath.Join(attempt.Input, "hello_1.0.dsc") || ctx.Err() != nil {
				t.Fatalf("unexpected backend input: %+v, %+v, %v", got, source, ctx.Err())
			}
			writeArtifactFixture(t, filepath.Join(attempt.Result, "hello_1.0_amd64.deb"), "binary bytes")
			writeArtifactFixture(t, filepath.Join(attempt.Result, "hello_1.0_amd64.buildinfo"), "build info")
			return attempt, nil
		},
		UploadArtifact: func(ctx context.Context, got buildJob) error {
			if got != job || ctx.Err() != nil {
				t.Fatalf("unexpected upload input: %+v, %v", got, ctx.Err())
			}
			members := readArtifactArchive(t, got.Archive)
			if len(members) != 4 || members["job-123/hello_1.0_amd64.deb"] != "binary bytes" {
				t.Fatalf("uploaded archive = %v", members)
			}
			return nil
		},
	}
}

func TestBuildFlowDoesNotRunBackendAfterPreparationFailure(t *testing.T) {
	job, steps := buildFlowFixture(t)
	cause := errors.New("preparation failed")
	steps.Prepare = func(context.Context, buildJob, attemptPaths) (sourceSet, error) {
		return sourceSet{}, cause
	}
	steps.Build = func(context.Context, buildJob, attemptPaths, sourceSet) (attemptPaths, error) {
		t.Fatal("backend ran after preparation failed")
		return attemptPaths{}, nil
	}
	next, err := executeBuild(context.Background(), "unchanged payload", job, steps)
	if next != "" || !errors.Is(err, cause) {
		t.Fatalf("build result = %q, %v", next, err)
	}
}

func TestBuildFlowDoesNotUploadAfterBackendFailure(t *testing.T) {
	job, steps := buildFlowFixture(t)
	cause := errors.New("backend failed")
	steps.Build = func(context.Context, buildJob, attemptPaths, sourceSet) (attemptPaths, error) {
		return attemptPaths{}, cause
	}
	steps.UploadArtifact = func(context.Context, buildJob) error {
		t.Fatal("uploaded after backend failed")
		return nil
	}
	next, err := executeBuild(context.Background(), "unchanged payload", job, steps)
	if next != "" || !errors.Is(err, cause) {
		t.Fatalf("build result = %q, %v", next, err)
	}
}

func TestBuildFlowDoesNotReturnPayloadAfterArtifactUploadFailure(t *testing.T) {
	job, steps := buildFlowFixture(t)
	cause := errors.New("upload failed")
	steps.UploadArtifact = func(context.Context, buildJob) error { return cause }
	next, err := executeBuild(context.Background(), "unchanged payload", job, steps)
	if next != "" || !errors.Is(err, cause) {
		t.Fatalf("build result = %q, %v", next, err)
	}
}

func TestBuildFlowReturnsUnchangedPayloadAfterArtifactUpload(t *testing.T) {
	job, steps := buildFlowFixture(t)
	completed := false
	upload := steps.UploadArtifact
	steps.UploadArtifact = func(ctx context.Context, job buildJob) error {
		if err := upload(ctx, job); err != nil {
			return err
		}
		completed = true
		return nil
	}
	payload := " {\"taskUUID\": \"job-123\", \"unknown\": true} "
	next, err := executeBuild(context.Background(), payload, job, steps)
	if err != nil || next != payload || !completed {
		t.Fatalf("build result = %q, %v; upload completed = %v", next, err, completed)
	}
}

func TestBuildFlowDoesNotCollectAfterCancellation(t *testing.T) {
	job, steps := buildFlowFixture(t)
	if err := os.MkdirAll(job.Artifacts, 0755); err != nil {
		t.Fatal(err)
	}
	stale := filepath.Join(job.Artifacts, "old.deb")
	writeArtifactFixture(t, stale, "old")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	backend := steps.Build
	steps.Build = func(ctx context.Context, job buildJob, attempt attemptPaths, source sourceSet) (attemptPaths, error) {
		result, err := backend(ctx, job, attempt, source)
		cancel()
		return result, err
	}
	steps.UploadArtifact = func(context.Context, buildJob) error {
		t.Fatal("uploaded after cancellation")
		return nil
	}
	next, err := executeBuild(ctx, "payload", job, steps)
	if next != "" || !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled build result = %q, %v", next, err)
	}
	if _, err := os.Stat(stale); err != nil {
		t.Fatalf("collection ran after cancellation: %v", err)
	}
}

func TestBuildFlowCollectsOnlySuccessfulAttempt(t *testing.T) {
	job, steps := buildFlowFixture(t)
	backend := steps.Build
	steps.Build = func(ctx context.Context, job buildJob, attempt attemptPaths, source sourceSet) (attemptPaths, error) {
		writeArtifactFixture(t, filepath.Join(attempt.Result, "failed.deb"), "failed attempt")
		next, err := job.newAttempt(2)
		if err != nil {
			return next, err
		}
		for _, name := range source.Files {
			if err := os.Link(filepath.Join(attempt.Input, name), filepath.Join(next.Input, name)); err != nil {
				return next, err
			}
		}
		source.DSC = filepath.Join(next.Input, "hello_1.0.dsc")
		result, err := backend(ctx, job, next, source)
		if err != nil {
			return result, err
		}
		return result, os.RemoveAll(attempt.Input)
	}
	if next, err := executeBuild(context.Background(), "payload", job, steps); next != "payload" || err != nil {
		t.Fatalf("retry build result = %q, %v", next, err)
	}
	if _, err := os.Stat(filepath.Join(job.Artifacts, "failed.deb")); !os.IsNotExist(err) {
		t.Fatalf("failed attempt artifact published: %v", err)
	}
}

func TestBuildFlowDoesNotUploadInvalidArtifacts(t *testing.T) {
	job, steps := buildFlowFixture(t)
	backend := steps.Build
	steps.Build = func(ctx context.Context, job buildJob, attempt attemptPaths, source sourceSet) (attemptPaths, error) {
		result, err := backend(ctx, job, attempt, source)
		if err != nil {
			return result, err
		}
		return result, os.Remove(filepath.Join(result.Result, "hello_1.0_amd64.buildinfo"))
	}
	steps.UploadArtifact = func(context.Context, buildJob) error {
		t.Fatal("uploaded invalid artifacts")
		return nil
	}
	if next, err := executeBuild(context.Background(), "payload", job, steps); next != "" || err == nil {
		t.Fatalf("invalid artifact build result = %q, %v", next, err)
	}
}

func TestUploadFinalLogUsesIndependentContext(t *testing.T) {
	job, steps := buildFlowFixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := executeBuild(ctx, "payload", job, steps); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled build result = %v", err)
	}
	var uploadContext context.Context
	before := time.Now()
	uploadFinalLog(job, func(ctx context.Context, got buildJob) error {
		uploadContext = ctx
		if got != job || ctx.Err() != nil {
			t.Fatalf("log upload = %+v, %v", got, ctx.Err())
		}
		deadline, ok := ctx.Deadline()
		if !ok || deadline.Before(before.Add(2*time.Minute)) || deadline.After(time.Now().Add(2*time.Minute)) {
			t.Fatalf("log upload deadline = %v, present = %v", deadline, ok)
		}
		return nil
	})
	if uploadContext == nil || uploadContext.Err() != context.Canceled {
		t.Fatalf("log upload context not released: %v", uploadContext)
	}
}

func TestBuildUploadsArtifactsThenFinalLogAndCleansOnlyJob(t *testing.T) {
	for _, artifactStatus := range []int{http.StatusCreated, http.StatusInternalServerError} {
		t.Run(fmt.Sprint(artifactStatus), func(t *testing.T) {
			previousConfig := irgshConfig
			t.Cleanup(func() { irgshConfig = previousConfig })
			irgshConfig = config.IrgshConfig{}
			irgshConfig.Builder = baseFixture(t)
			irgshConfig.Builder.Workdir = filepath.Join(t.TempDir(), "builder space")
			irgshConfig.Builder.DistCodename = "verbeek"
			base := oldBaseFixture(t, irgshConfig.Builder)
			job, err := newBuildJob(irgshConfig.Builder.Workdir, buildSubmission{TaskUUID: "job-123"})
			if err != nil {
				t.Fatal(err)
			}
			sibling := filepath.Join(filepath.Dir(job.Root), "other-job")
			if err := os.MkdirAll(sibling, 0755); err != nil {
				t.Fatal(err)
			}
			root, _, _ := sourceFixture(t, "hello_1.0.orig.tar.xz", []byte("source bytes"), "signed")
			var submission bytes.Buffer
			compressed := gzip.NewWriter(&submission)
			writer := tar.NewWriter(compressed)
			for _, name := range []string{"hello_1.0.dsc", "hello_1.0.orig.tar.xz"} {
				content, err := os.ReadFile(filepath.Join(root, "signed", name))
				if err != nil {
					t.Fatal(err)
				}
				if err := writer.WriteHeader(&tar.Header{Name: "signed/" + name, Mode: 0644, Size: int64(len(content)), Typeflag: tar.TypeReg}); err != nil {
					t.Fatal(err)
				}
				if _, err := writer.Write(content); err != nil {
					t.Fatal(err)
				}
			}
			if err := errors.Join(writer.Close(), compressed.Close()); err != nil {
				t.Fatal(err)
			}
			requests := make(chan string, 3)
			logs := make(chan string, 1)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests <- r.URL.Path
				if r.URL.Path == "/submissions/job-123.tar.gz" {
					_, _ = w.Write(submission.Bytes())
					return
				}
				if r.URL.Query().Get("id") != "job-123" {
					t.Errorf("upload query = %s", r.URL)
				}
				file, _, err := r.FormFile("uploadFile")
				if err != nil {
					t.Error(err)
					w.WriteHeader(http.StatusBadRequest)
					return
				}
				defer file.Close()
				defer r.MultipartForm.RemoveAll()
				content, err := io.ReadAll(file)
				if err != nil {
					t.Error(err)
				}
				switch r.URL.Path {
				case "/api/v1/artifact-upload":
					if !bytes.HasPrefix(content, []byte{0x1f, 0x8b}) {
						t.Error("artifact is not gzip")
					}
					w.WriteHeader(artifactStatus)
				case "/api/v1/log-upload":
					if r.URL.Query().Get("type") != "build" {
						t.Errorf("log query = %s", r.URL)
					}
					if _, err := os.Stat(job.Root); err != nil {
						t.Errorf("scratch removed before log upload: %v", err)
					}
					logs <- string(content)
				default:
					t.Errorf("unexpected endpoint %s", r.URL)
					w.WriteHeader(http.StatusNotFound)
				}
			}))
			defer server.Close()
			irgshConfig.Chief.Address = server.URL
			argsPath := nativeCommandFixture(t)
			payload := validBuildPayload(t)
			next, err := Build(payload)
			wantLog := "[ BUILD DONE ]"
			if artifactStatus == http.StatusCreated {
				if err != nil || next != payload {
					t.Fatalf("build result = %q, %v", next, err)
				}
			} else {
				wantLog = "[ BUILD FAILED ]"
				if err == nil || next != "" {
					t.Fatalf("failed upload result = %q, %v", next, err)
				}
			}
			if len(requests) != 3 {
				t.Fatalf("received %d requests, want 3", len(requests))
			}
			for _, want := range []string{"/submissions/job-123.tar.gz", "/api/v1/artifact-upload", "/api/v1/log-upload"} {
				if got := <-requests; got != want {
					t.Errorf("endpoint = %q, want %q", got, want)
				}
			}
			if len(logs) != 1 || !strings.Contains(<-logs, wantLog) {
				t.Fatalf("final log missing %q", wantLog)
			}
			if _, err := os.Stat(job.Root); !os.IsNotExist(err) {
				t.Fatalf("scratch remains: %v", err)
			}
			for _, path := range []string{sibling, job.Log, job.Archive} {
				if _, err := os.Stat(path); err != nil {
					t.Errorf("persistent path removed: %s: %v", path, err)
				}
			}
			args, err := os.ReadFile(argsPath)
			wantArgs, marshalErr := json.Marshal([]any{[]string{"--chroot-mode=unshare", "--chroot=" + filepath.Join(job.Root, "1", "base.tar"), "--dist=verbeek", "--arch=amd64", "--arch-all", "--arch-any", "--no-source", "--enable-network", "--nolog", "--build-dir=" + filepath.Join(job.Root, "1", "result"), filepath.Join(job.Root, "1", "input", "hello_1.0.dsc")}, base.Config, filepath.Join(job.Root, "1", "tmp")})
			if err != nil || marshalErr != nil || string(args) != string(wantArgs) {
				t.Fatalf("sbuild arguments = %q, %v, %v", args, err, marshalErr)
			}
		})
	}
}
