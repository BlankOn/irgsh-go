package usecase_test

import (
	"context"
	"errors"
	"testing"

	"github.com/blankon/irgsh-go/internal/cli/domain"
	"github.com/blankon/irgsh-go/internal/cli/usecase"
	"github.com/blankon/irgsh-go/pkg/httputil"
	"github.com/stretchr/testify/assert"
)

func TestSubmitISO_Success(t *testing.T) {
	svc := usecase.NewCLIUsecase(
		&mockConfigStore{config: domain.Config{ChiefAddress: "http://chief", MaintainerSigningKey: "KEY"}},
		&mockPipelineStore{},
		&mockChiefAPI{isoResp: domain.SubmitResponse{PipelineID: "iso-123"}},
		nil, nil, nil, nil, nil, nil, nil, "",
	)
	resp, err := svc.SubmitISO(context.Background(), "http://repo.git", "main")
	assert.NoError(t, err)
	assert.Equal(t, "iso-123", resp.PipelineID)
}

func TestSubmitISO_ConfigMissing(t *testing.T) {
	svc := usecase.NewCLIUsecase(
		&mockConfigStore{err: errors.New("no config")},
		nil, nil, nil, nil, nil, nil, nil, nil, nil, "",
	)
	_, err := svc.SubmitISO(context.Background(), "http://repo.git", "main")
	assert.ErrorIs(t, err, usecase.ErrConfigMissing)
}

func TestSubmitISO_EmptyURL(t *testing.T) {
	svc := usecase.NewCLIUsecase(
		&mockConfigStore{config: domain.Config{ChiefAddress: "http://chief", MaintainerSigningKey: "KEY"}},
		nil, nil, nil, nil, nil, nil, nil, nil, nil, "",
	)
	_, err := svc.SubmitISO(context.Background(), "", "main")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "lb-url")
}

func TestSubmitISO_EmptyBranch(t *testing.T) {
	svc := usecase.NewCLIUsecase(
		&mockConfigStore{config: domain.Config{ChiefAddress: "http://chief", MaintainerSigningKey: "KEY"}},
		nil, nil, nil, nil, nil, nil, nil, nil, nil, "",
	)
	_, err := svc.SubmitISO(context.Background(), "http://repo.git", "")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "lb-branch")
}

func TestISOStatus_Success(t *testing.T) {
	svc := usecase.NewCLIUsecase(
		&mockConfigStore{config: domain.Config{ChiefAddress: "http://chief", MaintainerSigningKey: "KEY"}},
		&mockPipelineStore{},
		&mockChiefAPI{isoStatus: domain.ISOStatus{PipelineID: "iso-123", JobStatus: "DONE", ISOStatus: "SUCCESS"}},
		nil, nil, nil, nil, nil, nil, nil, "",
	)
	status, err := svc.ISOStatus(context.Background(), "iso-123")
	assert.NoError(t, err)
	assert.Equal(t, "DONE", status.JobStatus)
}

func TestISOStatus_LoadFromStore(t *testing.T) {
	svc := usecase.NewCLIUsecase(
		&mockConfigStore{config: domain.Config{ChiefAddress: "http://chief", MaintainerSigningKey: "KEY"}},
		&mockPipelineStore{isoID: "stored-iso"},
		&mockChiefAPI{isoStatus: domain.ISOStatus{PipelineID: "stored-iso", JobStatus: "BUILDING"}},
		nil, nil, nil, nil, nil, nil, nil, "",
	)
	status, err := svc.ISOStatus(context.Background(), "")
	assert.NoError(t, err)
	assert.Equal(t, "BUILDING", status.JobStatus)
}

func TestISOStatus_PipelineIDMissing(t *testing.T) {
	svc := usecase.NewCLIUsecase(
		&mockConfigStore{config: domain.Config{ChiefAddress: "http://chief", MaintainerSigningKey: "KEY"}},
		&mockPipelineStore{},
		nil, nil, nil, nil, nil, nil, nil, nil, "",
	)
	_, err := svc.ISOStatus(context.Background(), "")
	assert.ErrorIs(t, err, usecase.ErrPipelineIDMissing)
}

func TestISOLog_Success(t *testing.T) {
	svc := usecase.NewCLIUsecase(
		&mockConfigStore{config: domain.Config{ChiefAddress: "http://chief", MaintainerSigningKey: "KEY"}},
		&mockPipelineStore{},
		&mockChiefAPI{fetchLogResp: "ISO build log content"},
		nil, nil, nil, nil, nil, nil, nil, "",
	)
	logResult, err := svc.ISOLog(context.Background(), "iso-123")
	assert.NoError(t, err)
	assert.Equal(t, "ISO build log content", logResult)
}

func TestISOLog_NotFound(t *testing.T) {
	svc := usecase.NewCLIUsecase(
		&mockConfigStore{config: domain.Config{ChiefAddress: "http://chief", MaintainerSigningKey: "KEY"}},
		&mockPipelineStore{},
		&mockChiefAPI{fetchLogErr: httputil.HTTPStatusError{StatusCode: 404}},
		nil, nil, nil, nil, nil, nil, nil, "",
	)
	_, err := svc.ISOLog(context.Background(), "iso-123")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "ISO log is not found")
}
