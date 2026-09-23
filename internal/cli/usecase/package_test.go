package usecase_test

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/blankon/irgsh-go/internal/cli/domain"
	"github.com/blankon/irgsh-go/internal/cli/repository"
	"github.com/blankon/irgsh-go/internal/cli/usecase"
	"github.com/blankon/irgsh-go/pkg/httputil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type submissionArchiveRepo struct{}

func (submissionArchiveRepo) Sync(_, _, destination string) error {
	if err := os.MkdirAll(filepath.Join(destination, "debian"), 0755); err != nil {
		return err
	}
	return os.Symlink("debian", filepath.Join(destination, "link"))
}

type submissionArchiveDebian struct{ mockDebianPackager }

func (submissionArchiveDebian) BuildSource(directory string) error {
	for _, name := range []string{"hello_1.0-1.dsc", "hello_1.0.orig.tar.xz", "hello_1.0.debian.tar.gz"} {
		if err := os.WriteFile(filepath.Join(filepath.Dir(directory), name), []byte("test source"), 0644); err != nil {
			return err
		}
	}
	return nil
}

type submissionArchiveChief struct {
	mockChiefAPI
	names []string
}

func (chief *submissionArchiveChief) UploadSubmission(_ context.Context, archive, _ string, _ func(int64, int64)) (domain.UploadResponse, error) {
	file, err := os.Open(archive)
	if err != nil {
		return domain.UploadResponse{}, err
	}
	defer file.Close()
	compressed, err := gzip.NewReader(file)
	if err != nil {
		return domain.UploadResponse{}, err
	}
	defer compressed.Close()
	reader := tar.NewReader(compressed)
	for {
		header, err := reader.Next()
		if err == io.EOF {
			return domain.UploadResponse{ID: "test-upload"}, nil
		}
		if err != nil {
			return domain.UploadResponse{}, err
		}
		chief.names = append(chief.names, header.Name)
	}
}

func TestSubmitPackageArchiveOmitsRenamedUnpackedTree(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	binDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(binDir, "dpkg-genchanges"), []byte("#!/bin/sh\nprintf 'source changes\\n'\n"), 0755))
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	chief := &submissionArchiveChief{}
	svc := usecase.NewCLIUsecase(
		&mockConfigStore{config: domain.Config{MaintainerSigningKey: "TEST-KEY"}},
		&mockPipelineStore{}, chief, repository.ShellRunner{}, submissionArchiveRepo{},
		&submissionArchiveDebian{mockDebianPackager{packageName: "hello", version: "1.0", extendedVersion: "1"}},
		&mockGPGSigner{identity: "Test Maintainer"}, nil, nil, nil, "",
	)
	_, err := svc.SubmitPackage(context.Background(), domain.SubmitParams{
		Dist: "verbeek", PackageURL: "https://example.test/package", IsExperimental: true, IgnoreChecks: true, SkipLocalBuild: true,
	})
	require.NoError(t, err)
	sort.Strings(chief.names)
	assert.Equal(t, []string{
		"./", "./hello_1.0.debian.tar.gz", "./signed/", "./signed/hello_1.0-1.dsc", "./signed/hello_1.0-1_source.changes", "./signed/hello_1.0.orig.tar.xz",
	}, chief.names)
}

func TestSubmitPackage_ConfigMissing(t *testing.T) {
	svc := usecase.NewCLIUsecase(
		&mockConfigStore{err: errors.New("no config")},
		nil, nil, nil, nil, nil, nil, nil, nil, nil, "",
	)
	_, err := svc.SubmitPackage(context.Background(), domain.SubmitParams{})
	assert.ErrorIs(t, err, usecase.ErrConfigMissing)
}

func TestSubmitPackage_VersionMismatch(t *testing.T) {
	svc := usecase.NewCLIUsecase(
		&mockConfigStore{config: domain.Config{ChiefAddress: "http://chief", MaintainerSigningKey: "KEY"}},
		nil,
		&mockChiefAPI{version: domain.VersionResponse{Version: "2.0.0"}},
		nil, nil, nil, nil, nil, nil, nil, "1.0.0",
	)
	_, err := svc.SubmitPackage(context.Background(), domain.SubmitParams{
		Dist:       "verbeek",
		PackageURL: "https://git.example.com/pkg",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "version mismatch")
}

func TestSubmitPackage_ChiefConnectError(t *testing.T) {
	svc := usecase.NewCLIUsecase(
		&mockConfigStore{config: domain.Config{ChiefAddress: "http://chief", MaintainerSigningKey: "KEY"}},
		nil,
		&mockChiefAPI{versionErr: errors.New("connection refused")},
		nil, nil, nil, nil, nil, nil, nil, "1.0.0",
	)
	_, err := svc.SubmitPackage(context.Background(), domain.SubmitParams{
		Dist:       "verbeek",
		PackageURL: "https://git.example.com/pkg",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "connect to chief")
}

func TestSubmitPackage_EmptyDist(t *testing.T) {
	svc := usecase.NewCLIUsecase(
		&mockConfigStore{config: domain.Config{ChiefAddress: "http://chief", MaintainerSigningKey: "KEY"}},
		nil, nil, nil, nil, nil, nil, nil, nil, nil, "",
	)
	_, err := svc.SubmitPackage(context.Background(), domain.SubmitParams{
		IgnoreChecks: true,
		PackageURL:   "https://git.example.com/pkg",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--dist is required")
}

func TestSubmitPackage_EmptyDistCheckedBeforeChief(t *testing.T) {
	// A missing --dist must be reported without ever contacting chief, so the
	// mock's failing GetVersion would surface as a different error if the
	// connectivity check ran first.
	svc := usecase.NewCLIUsecase(
		&mockConfigStore{config: domain.Config{ChiefAddress: "http://chief", MaintainerSigningKey: "KEY"}},
		nil,
		&mockChiefAPI{versionErr: errors.New("connection refused")},
		nil, nil, nil, nil, nil, nil, nil, "1.0.0",
	)
	_, err := svc.SubmitPackage(context.Background(), domain.SubmitParams{
		PackageURL: "https://git.example.com/pkg",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--dist is required")
}

func TestSubmitPackage_EmptyPackageURL(t *testing.T) {
	svc := usecase.NewCLIUsecase(
		&mockConfigStore{config: domain.Config{ChiefAddress: "http://chief", MaintainerSigningKey: "KEY"}},
		nil, nil, nil, nil, nil, nil, nil, nil, nil, "",
	)
	_, err := svc.SubmitPackage(context.Background(), domain.SubmitParams{
		Dist:         "verbeek",
		IgnoreChecks: true,
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--package should not be empty")
}

func TestSubmitPackage_InvalidPackageURL(t *testing.T) {
	svc := usecase.NewCLIUsecase(
		&mockConfigStore{config: domain.Config{ChiefAddress: "http://chief", MaintainerSigningKey: "KEY"}},
		nil, nil, nil, nil, nil, nil, nil, nil, nil, "",
	)
	_, err := svc.SubmitPackage(context.Background(), domain.SubmitParams{
		Dist:         "verbeek",
		IgnoreChecks: true,
		PackageURL:   "not-a-url",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--package must be a valid")
}

func TestSubmitPackage_InvalidSourceURL(t *testing.T) {
	svc := usecase.NewCLIUsecase(
		&mockConfigStore{config: domain.Config{ChiefAddress: "http://chief", MaintainerSigningKey: "KEY"}},
		nil, nil, nil, nil, nil, nil, nil, nil, nil, "",
	)
	_, err := svc.SubmitPackage(context.Background(), domain.SubmitParams{
		Dist:         "verbeek",
		IgnoreChecks: true,
		PackageURL:   "https://git.example.com/pkg",
		SourceURL:    "ftp://invalid",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--source must be a valid")
}

func TestSubmitPackage_UserCancelled(t *testing.T) {
	svc := usecase.NewCLIUsecase(
		&mockConfigStore{config: domain.Config{ChiefAddress: "http://chief", MaintainerSigningKey: "KEY"}},
		nil, nil, nil, nil, nil, nil, nil, nil,
		&mockPrompter{confirmed: false},
		"",
	)
	_, err := svc.SubmitPackage(context.Background(), domain.SubmitParams{
		Dist:           "verbeek",
		IgnoreChecks:   true,
		PackageURL:     "https://git.example.com/pkg",
		IsExperimental: false,
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cancelled by user")
}

func TestPackageStatus_Success(t *testing.T) {
	svc := usecase.NewCLIUsecase(
		&mockConfigStore{config: domain.Config{ChiefAddress: "http://chief", MaintainerSigningKey: "KEY"}},
		&mockPipelineStore{},
		&mockChiefAPI{pkgStatus: domain.PackageStatus{
			PipelineID:  "pkg-123",
			JobStatus:   "DONE",
			BuildStatus: "SUCCESS",
			RepoStatus:  "SUCCESS",
			State:       "DONE",
		}},
		nil, nil, nil, nil, nil, nil, nil, "",
	)
	status, err := svc.PackageStatus(context.Background(), "pkg-123")
	assert.NoError(t, err)
	assert.Equal(t, "DONE", status.JobStatus)
	assert.Equal(t, "SUCCESS", status.BuildStatus)
	assert.Equal(t, "SUCCESS", status.RepoStatus)
}

func TestPackageStatus_ConfigMissing(t *testing.T) {
	svc := usecase.NewCLIUsecase(
		&mockConfigStore{err: errors.New("no config")},
		nil, nil, nil, nil, nil, nil, nil, nil, nil, "",
	)
	_, err := svc.PackageStatus(context.Background(), "pkg-123")
	assert.ErrorIs(t, err, usecase.ErrConfigMissing)
}

func TestPackageStatus_LoadFromStore(t *testing.T) {
	svc := usecase.NewCLIUsecase(
		&mockConfigStore{config: domain.Config{ChiefAddress: "http://chief", MaintainerSigningKey: "KEY"}},
		&mockPipelineStore{packageID: "stored-pkg"},
		&mockChiefAPI{pkgStatus: domain.PackageStatus{PipelineID: "stored-pkg", State: "DONE"}},
		nil, nil, nil, nil, nil, nil, nil, "",
	)
	status, err := svc.PackageStatus(context.Background(), "")
	assert.NoError(t, err)
	assert.Equal(t, "DONE", status.State)
}

func TestPackageStatus_PipelineIDMissing(t *testing.T) {
	svc := usecase.NewCLIUsecase(
		&mockConfigStore{config: domain.Config{ChiefAddress: "http://chief", MaintainerSigningKey: "KEY"}},
		&mockPipelineStore{},
		nil, nil, nil, nil, nil, nil, nil, nil, "",
	)
	_, err := svc.PackageStatus(context.Background(), "")
	assert.ErrorIs(t, err, usecase.ErrPipelineIDMissing)
}

func TestPackageLog_Success(t *testing.T) {
	svc := usecase.NewCLIUsecase(
		&mockConfigStore{config: domain.Config{ChiefAddress: "http://chief", MaintainerSigningKey: "KEY"}},
		&mockPipelineStore{},
		&mockChiefAPI{
			pkgStatus:    domain.PackageStatus{State: "DONE"},
			fetchLogResp: "log content",
		},
		nil, nil, nil, nil, nil, nil, nil, "",
	)
	buildLog, repoLog, err := svc.PackageLog(context.Background(), "pkg-123")
	assert.NoError(t, err)
	assert.Equal(t, "log content", buildLog)
	assert.Equal(t, "log content", repoLog)
}

func TestPackageLog_PipelineNotFinished(t *testing.T) {
	svc := usecase.NewCLIUsecase(
		&mockConfigStore{config: domain.Config{ChiefAddress: "http://chief", MaintainerSigningKey: "KEY"}},
		&mockPipelineStore{},
		&mockChiefAPI{pkgStatus: domain.PackageStatus{State: "STARTED"}},
		nil, nil, nil, nil, nil, nil, nil, "",
	)
	_, _, err := svc.PackageLog(context.Background(), "pkg-123")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not finished yet")
}

func TestPackageLog_BuildLogNotFound(t *testing.T) {
	svc := usecase.NewCLIUsecase(
		&mockConfigStore{config: domain.Config{ChiefAddress: "http://chief", MaintainerSigningKey: "KEY"}},
		&mockPipelineStore{},
		&mockChiefAPI{
			pkgStatus:   domain.PackageStatus{State: "DONE"},
			fetchLogErr: httputil.HTTPStatusError{StatusCode: 404},
		},
		nil, nil, nil, nil, nil, nil, nil, "",
	)
	_, _, err := svc.PackageLog(context.Background(), "pkg-123")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "log is not found")
}
