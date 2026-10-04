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

func TestCancelPipeline_Success(t *testing.T) {
	svc := usecase.NewCLIUsecase(
		&mockConfigStore{config: domain.Config{ChiefAddress: "http://chief", MaintainerSigningKey: "KEY"}},
		&mockPipelineStore{},
		&mockChiefAPI{cancelResp: domain.CancelResponse{
			PipelineID: "2026-09-10-134726_46ef4607_import",
			State:      "CANCELED",
			Message:    "cancellation requested, the worker is stopping the job",
		}},
		nil, nil, nil, nil, nil, nil, nil, "",
	)
	resp, err := svc.CancelPipeline(context.Background(), "2026-09-10-134726_46ef4607_import")
	assert.NoError(t, err)
	assert.Equal(t, "CANCELED", resp.State)
}

func TestCancelPipeline_ConfigMissing(t *testing.T) {
	svc := usecase.NewCLIUsecase(
		&mockConfigStore{err: errors.New("no config")},
		nil, nil, nil, nil, nil, nil, nil, nil, nil, "",
	)
	_, err := svc.CancelPipeline(context.Background(), "some-id")
	assert.ErrorIs(t, err, usecase.ErrConfigMissing)
}

// Unlike retry there is no last-pipeline fallback: cancelling the wrong job is
// not something to guess at.
func TestCancelPipeline_PipelineIDMissing(t *testing.T) {
	svc := usecase.NewCLIUsecase(
		&mockConfigStore{config: domain.Config{ChiefAddress: "http://chief", MaintainerSigningKey: "KEY"}},
		&mockPipelineStore{retryID: "stored-retry"},
		&mockChiefAPI{},
		nil, nil, nil, nil, nil, nil, nil, "",
	)
	_, err := svc.CancelPipeline(context.Background(), "")
	assert.ErrorIs(t, err, usecase.ErrPipelineIDMissing)
}

// Chief refuses a pipeline that has reached its repo stage, and the maintainer
// has to see why.
func TestCancelPipeline_RefusedByChief(t *testing.T) {
	svc := usecase.NewCLIUsecase(
		&mockConfigStore{config: domain.Config{ChiefAddress: "http://chief", MaintainerSigningKey: "KEY"}},
		&mockPipelineStore{},
		&mockChiefAPI{cancelErr: errors.New("interrupting reprepro can corrupt the repository database")},
		nil, nil, nil, nil, nil, nil, nil, "",
	)
	_, err := svc.CancelPipeline(context.Background(), "some-id")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "reprepro")
}

// Chief's refusal arrives as an HTTP status error carrying a JSON body; what
// the maintainer needs to read is the reason inside it.
func TestCancelPipeline_RefusalMessageIsUnwrapped(t *testing.T) {
	svc := usecase.NewCLIUsecase(
		&mockConfigStore{config: domain.Config{ChiefAddress: "http://chief", MaintainerSigningKey: "KEY"}},
		&mockPipelineStore{},
		&mockChiefAPI{cancelErr: httputil.HTTPStatusError{
			StatusCode: 409,
			Body:       `{"error":"job x is already in its repo stage and cannot be cancelled"}`,
		}},
		nil, nil, nil, nil, nil, nil, nil, "",
	)
	_, err := svc.CancelPipeline(context.Background(), "x")
	assert.EqualError(t, err, "job x is already in its repo stage and cannot be cancelled")
}
