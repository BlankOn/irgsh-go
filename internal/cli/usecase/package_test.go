package usecase_test

import (
	"context"
	"errors"
	"testing"

	"github.com/blankon/irgsh-go/internal/cli/domain"
	"github.com/blankon/irgsh-go/internal/cli/usecase"
	"github.com/blankon/irgsh-go/pkg/httputil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
		PackageURL: "https://git.example.com/pkg",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "connect to chief")
}

func TestSubmitPackage_EmptyPackageURL(t *testing.T) {
	svc := usecase.NewCLIUsecase(
		&mockConfigStore{config: domain.Config{ChiefAddress: "http://chief", MaintainerSigningKey: "KEY"}},
		nil, nil, nil, nil, nil, nil, nil, nil, nil, "",
	)
	_, err := svc.SubmitPackage(context.Background(), domain.SubmitParams{
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
