package usecase

import (
	"context"
	"errors"
	"fmt"

	"github.com/blankon/irgsh-go/internal/cli/domain"
)

// CancelPipeline stops a queued or running job.
//
// Every kind of job can be cancelled except a package pipeline that has
// already reached its repo stage: that stage drives reprepro, and interrupting
// reprepro can leave the repository database corrupted. Chief refuses those,
// and the refusal is what the maintainer sees.
func (u *CLIUsecase) CancelPipeline(ctx context.Context, pipelineID string) (domain.CancelResponse, error) {
	if _, err := u.config.Load(); err != nil {
		return domain.CancelResponse{}, fmt.Errorf("%w: %w", ErrConfigMissing, err)
	}
	if pipelineID == "" {
		return domain.CancelResponse{}, ErrPipelineIDMissing
	}

	fmt.Println("Cancelling pipeline " + pipelineID + " ...")

	resp, err := u.chief.Cancel(ctx, pipelineID)
	if err != nil {
		return domain.CancelResponse{}, chiefError(err)
	}
	if resp.Error != "" {
		return domain.CancelResponse{}, errors.New(resp.Error)
	}

	message := resp.Message
	if message == "" {
		message = "cancelled"
	}
	fmt.Println("Pipeline " + resp.PipelineID + ": " + message)

	return resp, nil
}
