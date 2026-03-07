package usecase

import (
	"context"
	"errors"
	"fmt"
	"log"

	"github.com/blankon/irgsh-go/internal/cli/domain"
)

func (u *CLIUsecase) RetryPipeline(ctx context.Context, pipelineID string) (domain.RetryResponse, error) {
	if _, err := u.config.Load(); err != nil {
		return domain.RetryResponse{}, fmt.Errorf("%w: %w", ErrConfigMissing, err)
	}

	var err error
	if pipelineID == "" {
		pipelineID, err = u.pipelines.LoadRetryID()
		if err != nil || pipelineID == "" {
			return domain.RetryResponse{}, ErrPipelineIDMissing
		}
	}

	fmt.Println("Retrying pipeline " + pipelineID + " ...")

	resp, err := u.chief.Retry(ctx, pipelineID)
	if err != nil {
		return domain.RetryResponse{}, err
	}
	if resp.Error != "" {
		return domain.RetryResponse{}, errors.New(resp.Error)
	}

	fmt.Println("Pipeline " + resp.PipelineID + " has been queued for retry")

	if err := u.pipelines.SaveRetryID(resp.PipelineID); err != nil {
		log.Printf("warning: failed to save retry pipeline ID: %v", err)
	}

	return resp, nil
}
