package usecase

import (
	"context"
	"errors"
	"fmt"

	"github.com/blankon/irgsh-go/internal/cli/entity"
)

func (u *CLIUsecase) RetryPipeline(ctx context.Context, pipelineID string) (entity.RetryResponse, error) {
	if _, err := u.Config.Load(); err != nil {
		return entity.RetryResponse{}, ErrConfigMissing
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
	return resp, nil
}
