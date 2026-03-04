package usecase

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/blankon/irgsh-go/internal/cli/entity"
)

func (u *CLIUsecase) SubmitISO(ctx context.Context, repoURL, branch string) (entity.SubmitResponse, error) {
	if _, err := u.Config.Load(); err != nil {
		return entity.SubmitResponse{}, ErrConfigMissing
	}

	if repoURL == "" {
		return entity.SubmitResponse{}, errors.New("--lb-url is required")
	}
	if branch == "" {
		return entity.SubmitResponse{}, errors.New("--lb-branch is required")
	}

	fmt.Printf("Submitting ISO build job...\n")
	fmt.Printf("Repository: %s\n", repoURL)
	fmt.Printf("Branch: %s\n", branch)

	submission := entity.ISOSubmission{
		RepoURL: repoURL,
		Branch:  branch,
	}

	resp, err := u.Chief.SubmitISO(ctx, submission)
	if err != nil {
		return entity.SubmitResponse{}, err
	}
	if resp.Error != "" {
		return entity.SubmitResponse{}, errors.New(resp.Error)
	}

	fmt.Println("ISO build submitted successfully!")
	fmt.Println("Pipeline ID: " + resp.PipelineID)

	if err := u.Pipelines.SaveISOID(resp.PipelineID); err != nil {
		fmt.Printf("warning: failed to save pipeline ID: %v\n", err)
	}

	return resp, nil
}

func (u *CLIUsecase) ISOStatus(ctx context.Context, pipelineID string) (entity.ISOStatus, error) {
	if _, err := u.Config.Load(); err != nil {
		return entity.ISOStatus{}, ErrConfigMissing
	}

	var err error
	if pipelineID == "" {
		pipelineID, err = u.Pipelines.LoadISOID()
		if err != nil || pipelineID == "" {
			return entity.ISOStatus{}, ErrPipelineIDMissing
		}
	}

	fmt.Println("Checking the status of " + pipelineID + " ...")
	return u.Chief.GetISOStatus(ctx, pipelineID)
}

func (u *CLIUsecase) ISOLog(ctx context.Context, pipelineID string) (string, error) {
	if _, err := u.Config.Load(); err != nil {
		return "", ErrConfigMissing
	}

	var err error
	if pipelineID == "" {
		pipelineID, err = u.Pipelines.LoadISOID()
		if err != nil || pipelineID == "" {
			return "", ErrPipelineIDMissing
		}
	}

	fmt.Println("Fetching the logs of " + pipelineID + " ...")

	logResult, err := u.Chief.FetchLog(ctx, pipelineID+".iso.log")
	if err != nil {
		return "", err
	}
	if strings.Contains(logResult, "404 page not found") {
		return "", errors.New("ISO log is not found. The worker/pipeline may have terminated ungracefully")
	}

	return logResult, nil
}
