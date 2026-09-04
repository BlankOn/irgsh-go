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
	chief := &mockChiefAPI{isoResp: domain.SubmitResponse{PipelineID: "iso-123"}}
	svc := usecase.NewCLIUsecase(
		&mockConfigStore{config: domain.Config{ChiefAddress: "http://chief", MaintainerSigningKey: "KEY"}},
		&mockPipelineStore{},
		chief,
		nil, nil, nil, nil, nil, nil, nil, "",
	)
	resp, err := svc.SubmitISO(context.Background(), "verbeek", "without-praya", false)
	assert.NoError(t, err)
	assert.Equal(t, "iso-123", resp.PipelineID)
	// The live-build repository is the worker's config, not the client's to send.
	assert.Equal(t, domain.ISOSubmission{Dist: "verbeek", Branch: "without-praya"}, chief.isoSubmitted)
}

func TestSubmitISO_NoCacheIsForwarded(t *testing.T) {
	chief := &mockChiefAPI{isoResp: domain.SubmitResponse{PipelineID: "iso-123"}}
	svc := usecase.NewCLIUsecase(
		&mockConfigStore{config: domain.Config{ChiefAddress: "http://chief", MaintainerSigningKey: "KEY"}},
		&mockPipelineStore{},
		chief,
		nil, nil, nil, nil, nil, nil, nil, "",
	)
	_, err := svc.SubmitISO(context.Background(), "verbeek", "without-praya", true)
	assert.NoError(t, err)
	assert.True(t, chief.isoSubmitted.NoCache)
}

func TestSubmitISO_ConfigMissing(t *testing.T) {
	svc := usecase.NewCLIUsecase(
		&mockConfigStore{err: errors.New("no config")},
		nil, nil, nil, nil, nil, nil, nil, nil, nil, "",
	)
	_, err := svc.SubmitISO(context.Background(), "verbeek", "main", false)
	assert.ErrorIs(t, err, usecase.ErrConfigMissing)
}

func TestSubmitISO_EmptyDist(t *testing.T) {
	svc := usecase.NewCLIUsecase(
		&mockConfigStore{config: domain.Config{ChiefAddress: "http://chief", MaintainerSigningKey: "KEY"}},
		nil, nil, nil, nil, nil, nil, nil, nil, nil, "",
	)
	_, err := svc.SubmitISO(context.Background(), "", "main", false)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "--dist")
}

func TestSubmitISO_EmptyBranch(t *testing.T) {
	svc := usecase.NewCLIUsecase(
		&mockConfigStore{config: domain.Config{ChiefAddress: "http://chief", MaintainerSigningKey: "KEY"}},
		nil, nil, nil, nil, nil, nil, nil, nil, nil, "",
	)
	_, err := svc.SubmitISO(context.Background(), "verbeek", "", false)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "--branch")
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
