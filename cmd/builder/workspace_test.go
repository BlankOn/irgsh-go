package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func validBuildPayload(t *testing.T) string {
	t.Helper()
	fields := map[string]any{
		"taskUUID": "job-123", "dist": "verbeek", "packageName": "hello",
		"packageVersion": "1.0", "maintainer": "Test Maintainer", "component": "main",
		"packageExtendedVersion": "1", "packageUrl": "https://example.test/package",
		"sourceUrl": "https://example.test/source", "isExperimental": true,
		"forceVersion": true, "packageBranch": "main", "sourceBranch": "stable",
	}
	data, err := json.Marshal(fields)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestDecodeBuildSubmission(t *testing.T) {
	valid := validBuildPayload(t)
	tests := []struct {
		name       string
		payload    string
		servedDist string
		wantError  bool
	}{
		{"valid current payload", valid, "verbeek", false},
		{"malformed JSON", "{", "verbeek", true},
		{"missing taskUUID", `{"dist":"verbeek","packageName":"hello","packageVersion":"1.0","maintainer":"Test","component":"main"}`, "verbeek", true},
		{"unsafe taskUUID", `{"taskUUID":"../job","dist":"verbeek","packageName":"hello","packageVersion":"1.0","maintainer":"Test","component":"main"}`, "verbeek", true},
		{"missing dist", `{"taskUUID":"job-123","packageName":"hello","packageVersion":"1.0","maintainer":"Test","component":"main"}`, "verbeek", true},
		{"unsafe dist", valid, "../verbeek", true},
		{"wrong worker dist", valid, "other", true},
		{"missing packageName", `{"taskUUID":"job-123","dist":"verbeek","packageVersion":"1.0","maintainer":"Test","component":"main"}`, "verbeek", true},
		{"missing packageVersion", `{"taskUUID":"job-123","dist":"verbeek","packageName":"hello","maintainer":"Test","component":"main"}`, "verbeek", true},
		{"missing maintainer", `{"taskUUID":"job-123","dist":"verbeek","packageName":"hello","packageVersion":"1.0","component":"main"}`, "verbeek", true},
		{"missing component", `{"taskUUID":"job-123","dist":"verbeek","packageName":"hello","packageVersion":"1.0","maintainer":"Test"}`, "verbeek", true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := decodeBuildSubmission(test.payload, test.servedDist)
			if (err != nil) != test.wantError {
				t.Fatalf("decode error = %v, wantError %v", err, test.wantError)
			}
			if !test.wantError && (got.TaskUUID != "job-123" || got.Dist != "verbeek" || got.PackageExtendedVersion != "1" || !got.IsExperimental || !got.ForceVersion) {
				t.Fatalf("decoded payload lost fields: %+v", got)
			}
		})
	}
}

func TestNewBuildJobUsesAbsoluteConfinedPaths(t *testing.T) {
	workdir := t.TempDir()
	job, err := newBuildJob(workdir, buildSubmission{TaskUUID: "job-123"})
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{
		"root":      filepath.Join(workdir, "jobs", "job-123"),
		"artifacts": filepath.Join(workdir, "artifacts", "job-123"),
		"archive":   filepath.Join(workdir, "artifacts", "job-123.tar.gz"),
		"log":       filepath.Join(workdir, "artifacts", "job-123", "build.log"),
	}
	got := map[string]string{"root": job.Root, "artifacts": job.Artifacts, "archive": job.Archive, "log": job.Log}
	for key, path := range want {
		if got[key] != path || !filepath.IsAbs(got[key]) {
			t.Errorf("%s = %q, want absolute %q", key, got[key], path)
		}
	}
	attempt, err := job.newAttempt(1)
	if err != nil {
		t.Fatal(err)
	}
	wantAttempt := map[string]string{
		"root":   filepath.Join(workdir, "jobs", "job-123", "1"),
		"input":  filepath.Join(workdir, "jobs", "job-123", "1", "input"),
		"result": filepath.Join(workdir, "jobs", "job-123", "1", "result"),
		"temp":   filepath.Join(workdir, "jobs", "job-123", "1", "tmp"),
		"base":   filepath.Join(workdir, "jobs", "job-123", "1", "base.tar"),
	}
	gotAttempt := map[string]string{"root": attempt.Root, "input": attempt.Input, "result": attempt.Result, "temp": attempt.Temp, "base": attempt.Base}
	for key, path := range wantAttempt {
		if gotAttempt[key] != path {
			t.Errorf("%s = %q, want %q", key, gotAttempt[key], path)
		}
	}
	for _, path := range []string{attempt.Input, attempt.Result, attempt.Temp} {
		if info, err := os.Stat(path); err != nil || !info.IsDir() {
			t.Errorf("expected directory %q: %v", path, err)
		}
	}
}

func TestNewAttemptRejectsNonPositiveNumber(t *testing.T) {
	job, err := newBuildJob(t.TempDir(), buildSubmission{TaskUUID: "job-123"})
	if err != nil {
		t.Fatal(err)
	}
	for _, number := range []int{0, -1} {
		if _, err := job.newAttempt(number); err == nil {
			t.Errorf("attempt %d accepted", number)
		}
	}
}

func TestDecodeFailureCreatesNoWorkspace(t *testing.T) {
	workdir := t.TempDir()
	if _, err := decodeBuildSubmission(`{"taskUUID":"../escape"}`, "verbeek"); err == nil {
		t.Fatal("invalid submission accepted")
	}
	if entries, err := os.ReadDir(workdir); err != nil || len(entries) != 0 {
		t.Fatalf("workspace changed after decode failure: %v, %v", entries, err)
	}
}

func TestDownloadSubmissionRequiresHTTP200(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/submissions/job-123.tar.gz" {
			t.Errorf("request path = %q", r.URL.Path)
		}
		http.Error(w, "unavailable", http.StatusNotFound)
	}))
	defer server.Close()
	destination := filepath.Join(t.TempDir(), "submission.tar.gz")
	if err := downloadSubmission(context.Background(), server.Client(), server.URL, "job-123", destination); err == nil {
		t.Fatal("non-200 response accepted")
	}
	if _, err := os.Stat(destination); !os.IsNotExist(err) {
		t.Fatalf("partial download remains: %v", err)
	}
}

func TestDownloadSubmissionHonorsCancellation(t *testing.T) {
	started := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(started)
		<-r.Context().Done()
	}))
	defer server.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	destination := filepath.Join(t.TempDir(), "submission.tar.gz")
	done := make(chan error, 1)
	go func() { done <- downloadSubmission(ctx, server.Client(), server.URL, "job-123", destination) }()
	<-started
	cancel()
	if err := <-done; err == nil {
		t.Fatal("canceled download succeeded")
	}
	if _, err := os.Stat(destination); !os.IsNotExist(err) {
		t.Fatalf("partial download remains: %v", err)
	}
}

func writeTestArchive(t *testing.T, members ...tar.Header) string {
	t.Helper()
	var data bytes.Buffer
	gz := gzip.NewWriter(&data)
	tw := tar.NewWriter(gz)
	for _, header := range members {
		if err := tw.WriteHeader(&header); err != nil {
			t.Fatal(err)
		}
		if header.Typeflag == tar.TypeReg || header.Typeflag == tar.TypeRegA {
			if _, err := tw.Write(bytes.Repeat([]byte("x"), int(header.Size))); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	archive := filepath.Join(t.TempDir(), "submission.tar.gz")
	if err := os.WriteFile(archive, data.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}
	return archive
}

func TestExtractSubmissionAcceptsRegularFilesAndDirectories(t *testing.T) {
	archive := writeTestArchive(t,
		tar.Header{Name: "./", Typeflag: tar.TypeDir},
		tar.Header{Name: "signed/", Typeflag: tar.TypeDir},
		tar.Header{Name: "signed/source.dsc", Typeflag: tar.TypeReg, Size: 3},
	)
	destination := filepath.Join(t.TempDir(), "extract")
	if err := extractSubmission(archive, destination); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(destination, "signed", "source.dsc"))
	if err != nil || string(data) != "xxx" {
		t.Fatalf("extracted file = %q, %v", data, err)
	}
}

func TestExtractSubmissionRejectsAbsolutePath(t *testing.T) {
	outside := filepath.Join(t.TempDir(), "outside")
	assertRejectedArchive(t, tar.Header{Name: outside, Typeflag: tar.TypeReg, Size: 1}, outside)
}

func TestExtractSubmissionRejectsParentTraversal(t *testing.T) {
	assertRejectedArchive(t, tar.Header{Name: "../outside", Typeflag: tar.TypeReg, Size: 1}, "outside")
}

func TestExtractSubmissionRejectsSymlink(t *testing.T) {
	assertRejectedArchive(t, tar.Header{Name: "link", Typeflag: tar.TypeSymlink, Linkname: "../outside"}, "outside")
}

func TestExtractSubmissionRejectsHardLink(t *testing.T) {
	assertRejectedArchive(t, tar.Header{Name: "link", Typeflag: tar.TypeLink, Linkname: "../outside"}, "outside")
}

func TestExtractSubmissionRejectsDuplicateMember(t *testing.T) {
	archive := writeTestArchive(t,
		tar.Header{Name: "same", Typeflag: tar.TypeReg, Size: 1},
		tar.Header{Name: "./same", Typeflag: tar.TypeReg, Size: 1},
	)
	if err := extractSubmission(archive, filepath.Join(t.TempDir(), "extract")); err == nil {
		t.Fatal("duplicate destination accepted")
	}
}

func TestExtractSubmissionRejectsDuplicateRootDirectory(t *testing.T) {
	archive := writeTestArchive(t,
		tar.Header{Name: "./", Typeflag: tar.TypeDir},
		tar.Header{Name: ".", Typeflag: tar.TypeDir},
	)
	if err := extractSubmission(archive, filepath.Join(t.TempDir(), "extract")); err == nil {
		t.Fatal("duplicate root directory accepted")
	}
}

func assertRejectedArchive(t *testing.T, member tar.Header, outsideName string) {
	t.Helper()
	root := t.TempDir()
	archive := writeTestArchive(t, member)
	destination := filepath.Join(root, "extract")
	if err := extractSubmission(archive, destination); err == nil {
		t.Fatalf("member %q accepted", member.Name)
	}
	out := outsideName
	if !filepath.IsAbs(out) {
		out = filepath.Join(root, out)
	}
	if _, err := os.Stat(out); !os.IsNotExist(err) {
		t.Fatalf("outside file %q exists: %v", out, err)
	}
}

func testDSC(name string, size int, hash string) string {
	return fmt.Sprintf("-----BEGIN PGP SIGNED MESSAGE-----\nHash: SHA256\n\nFormat: 3.0 (quilt)\nChecksums-Sha256:\n %s %d %s\n-----BEGIN PGP SIGNATURE-----\ntest-only-signature\n-----END PGP SIGNATURE-----\n", hash, size, name)
}

func sourceFixture(t *testing.T, name string, content []byte, location string) (string, string, string) {
	t.Helper()
	root := t.TempDir()
	signed := filepath.Join(root, "signed")
	if err := os.Mkdir(signed, 0755); err != nil {
		t.Fatal(err)
	}
	hash := fmt.Sprintf("%x", sha256.Sum256(content))
	dsc := filepath.Join(signed, "hello_1.0.dsc")
	if err := os.WriteFile(dsc, []byte(testDSC(name, len(content), hash)), 0600); err != nil {
		t.Fatal(err)
	}
	if location != "" {
		if err := os.WriteFile(filepath.Join(root, location, name), content, 0600); err != nil {
			t.Fatal(err)
		}
	}
	return root, filepath.Join(t.TempDir(), "input"), dsc
}

func TestPrepareSourceAcceptsMemberBesideDSC(t *testing.T) {
	assertPreparedSource(t, "signed")
}

func TestPrepareSourceAcceptsMemberAtArchiveRoot(t *testing.T) {
	assertPreparedSource(t, ".")
}

func assertPreparedSource(t *testing.T, location string) {
	t.Helper()
	root, input, dsc := sourceFixture(t, "hello_1.0.orig.tar.xz", []byte("source bytes"), location)
	source, err := prepareSource(root, input)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"hello_1.0.dsc", "hello_1.0.orig.tar.xz"}
	if source.DSC != filepath.Join(input, want[0]) || !reflect.DeepEqual(source.Files, want) {
		t.Fatalf("source = %+v, want dsc %q and files %v", source, filepath.Join(input, want[0]), want)
	}
	entries, err := os.ReadDir(input)
	if err != nil || len(entries) != 2 {
		t.Fatalf("input files = %v, %v", entries, err)
	}
	for _, name := range want {
		info, err := os.Stat(filepath.Join(input, name))
		if err != nil || !info.Mode().IsRegular() || info.Mode().Perm() != 0444 {
			t.Fatalf("staged %s: %v, %v", name, info, err)
		}
	}
	stagedDSC, err := os.ReadFile(source.DSC)
	if err != nil {
		t.Fatal(err)
	}
	originalDSC, err := os.ReadFile(dsc)
	if err != nil || !bytes.Equal(stagedDSC, originalDSC) {
		t.Fatalf("staged DSC differs: %v", err)
	}
	stagedMember, err := os.ReadFile(filepath.Join(input, want[1]))
	if err != nil || string(stagedMember) != "source bytes" {
		t.Fatalf("staged member = %q, %v", stagedMember, err)
	}
	if err := validateSource(input, source); err != nil {
		t.Fatalf("validate staged source: %v", err)
	}
}

func TestPrepareSourceRejectsUnsignedDSC(t *testing.T) {
	root, input, dsc := sourceFixture(t, "source.tar.xz", []byte("x"), "signed")
	if err := os.WriteFile(dsc, []byte("Checksums-Sha256:\n"), 0600); err != nil {
		t.Fatal(err)
	}
	assertPrepareRejected(t, root, input)
}

func TestPrepareSourceRejectsNoDSC(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "signed"), 0755); err != nil {
		t.Fatal(err)
	}
	assertPrepareRejected(t, root, filepath.Join(t.TempDir(), "input"))
}

func TestPrepareSourceRejectsMultipleDSC(t *testing.T) {
	root, input, dsc := sourceFixture(t, "source.tar.xz", []byte("x"), "signed")
	data, err := os.ReadFile(dsc)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "signed", "other.dsc"), data, 0600); err != nil {
		t.Fatal(err)
	}
	assertPrepareRejected(t, root, input)
}

func TestPrepareSourceRejectsUnsafeMemberName(t *testing.T) {
	for _, name := range []string{"../source.tar.xz", `..\source.tar.xz`, "nested/source.tar.xz"} {
		t.Run(name, func(t *testing.T) {
			root, input, _ := sourceFixture(t, name, []byte("x"), "")
			assertPrepareRejected(t, root, input)
		})
	}
}

func TestPrepareSourceRejectsMissingMember(t *testing.T) {
	root, input, _ := sourceFixture(t, "source.tar.xz", []byte("x"), "")
	assertPrepareRejected(t, root, input)
}

func TestPrepareSourceRejectsDuplicateMemberLocations(t *testing.T) {
	root, input, _ := sourceFixture(t, "source.tar.xz", []byte("x"), "signed")
	if err := os.WriteFile(filepath.Join(root, "source.tar.xz"), []byte("x"), 0600); err != nil {
		t.Fatal(err)
	}
	assertPrepareRejected(t, root, input)
}

func TestPrepareSourceRejectsWrongSize(t *testing.T) {
	root, input, _ := sourceFixture(t, "source.tar.xz", []byte("x"), "signed")
	if err := os.WriteFile(filepath.Join(root, "signed", "source.tar.xz"), []byte("longer"), 0600); err != nil {
		t.Fatal(err)
	}
	assertPrepareRejected(t, root, input)
}

func TestPrepareSourceRejectsWrongSHA256(t *testing.T) {
	root, input, _ := sourceFixture(t, "source.tar.xz", []byte("x"), "signed")
	if err := os.WriteFile(filepath.Join(root, "signed", "source.tar.xz"), []byte("y"), 0600); err != nil {
		t.Fatal(err)
	}
	assertPrepareRejected(t, root, input)
}

func TestValidateSourceRejectsChangedStagedMember(t *testing.T) {
	root, input, _ := sourceFixture(t, "source.tar.xz", []byte("x"), "signed")
	source, err := prepareSource(root, input)
	if err != nil {
		t.Fatal(err)
	}
	member := filepath.Join(input, "source.tar.xz")
	if err := os.Chmod(member, 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(member, []byte("y"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := validateSource(input, source); err == nil {
		t.Fatal("modified staged source accepted")
	}
}

func assertPrepareRejected(t *testing.T, root string, input string) {
	t.Helper()
	if _, err := prepareSource(root, input); err == nil {
		t.Fatal("unverified source accepted")
	}
	if entries, err := os.ReadDir(input); err == nil && len(entries) != 0 {
		t.Fatalf("rejected source staged %d files", len(entries))
	} else if err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
}
