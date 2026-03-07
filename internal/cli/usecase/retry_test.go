package usecase_test

import (
	"context"
	"errors"
	"testing"

	"github.com/blankon/irgsh-go/internal/cli/domain"
	"github.com/blankon/irgsh-go/internal/cli/usecase"
	"github.com/stretchr/testify/assert"
)

func TestRetryPipeline_Success(t *testing.T) {
	svc := usecase.NewCLIUsecase(
		&mockConfigStore{config: domain.Config{ChiefAddress: "http://chief", MaintainerSigningKey: "KEY"}},
		&mockPipelineStore{},
		&mockChiefAPI{retryResp: domain.RetryResponse{PipelineID: "retry-456"}},
		nil, nil, nil, nil, nil, nil, nil, "",
	)
	resp, err := svc.RetryPipeline(context.Background(), "old-123")
	assert.NoError(t, err)
	assert.Equal(t, "retry-456", resp.PipelineID)
}

func TestRetryPipeline_ConfigMissing(t *testing.T) {
	svc := usecase.NewCLIUsecase(
		&mockConfigStore{err: errors.New("no config")},
		nil, nil, nil, nil, nil, nil, nil, nil, nil, "",
	)
	_, err := svc.RetryPipeline(context.Background(), "old-123")
	assert.ErrorIs(t, err, usecase.ErrConfigMissing)
}

func TestRetryPipeline_PipelineIDMissing(t *testing.T) {
	svc := usecase.NewCLIUsecase(
		&mockConfigStore{config: domain.Config{ChiefAddress: "http://chief", MaintainerSigningKey: "KEY"}},
		&mockPipelineStore{},
		nil, nil, nil, nil, nil, nil, nil, nil, "",
	)
	_, err := svc.RetryPipeline(context.Background(), "")
	assert.ErrorIs(t, err, usecase.ErrPipelineIDMissing)
}

func TestRetryPipeline_LoadFromStore(t *testing.T) {
	svc := usecase.NewCLIUsecase(
		&mockConfigStore{config: domain.Config{ChiefAddress: "http://chief", MaintainerSigningKey: "KEY"}},
		&mockPipelineStore{retryID: "stored-retry"},
		&mockChiefAPI{retryResp: domain.RetryResponse{PipelineID: "new-retry"}},
		nil, nil, nil, nil, nil, nil, nil, "",
	)
	resp, err := svc.RetryPipeline(context.Background(), "")
	assert.NoError(t, err)
	assert.Equal(t, "new-retry", resp.PipelineID)
}

func TestRetryPipeline_ServerError(t *testing.T) {
	svc := usecase.NewCLIUsecase(
		&mockConfigStore{config: domain.Config{ChiefAddress: "http://chief", MaintainerSigningKey: "KEY"}},
		&mockPipelineStore{},
		&mockChiefAPI{retryResp: domain.RetryResponse{Error: "job not found"}},
		nil, nil, nil, nil, nil, nil, nil, "",
	)
	_, err := svc.RetryPipeline(context.Background(), "old-123")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "job not found")
}
