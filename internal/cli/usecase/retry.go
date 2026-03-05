package usecase

import (
	"context"
	"errors"
	"fmt"
	"log"

	"github.com/blankon/irgsh-go/internal/cli/entity"
)

func (u *CLIUsecase) RetryPipeline(ctx context.Context, pipelineID string) (entity.RetryResponse, error) {
	if _, err := u.Config.Load(); err != nil {
		return entity.RetryResponse{}, fmt.Errorf("%w: %v", ErrConfigMissing, err)
	}

	var err error
	if pipelineID == "" {
		pipelineID, err = u.Pipelines.LoadRetryID()
		if err != nil || pipelineID == "" {
			return entity.RetryResponse{}, ErrPipelineIDMissing
		}
	}

	fmt.Println("Retrying pipeline " + pipelineID + " ...")

	resp, err := u.Chief.Retry(ctx, pipelineID)
	if err != nil {
		return entity.RetryResponse{}, err
	}
	if resp.Error != "" {
		return entity.RetryResponse{}, errors.New(resp.Error)
	}

	fmt.Println("Pipeline " + resp.PipelineID + " has been queued for retry")

	if err := u.Pipelines.SaveRetryID(resp.PipelineID); err != nil {
		log.Printf("warning: failed to save retry pipeline ID: %v", err)
	}

	return resp, nil
}
