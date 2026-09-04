package usecase

import (
	"context"
	"errors"
	"fmt"

	"github.com/blankon/irgsh-go/internal/cli/domain"
)

func (u *CLIUsecase) SubmitISO(ctx context.Context, dist, branch string, noCache bool) (domain.SubmitResponse, error) {
	if _, err := u.config.Load(); err != nil {
		return domain.SubmitResponse{}, fmt.Errorf("%w: %w", ErrConfigMissing, err)
	}

	if dist == "" {
		return domain.SubmitResponse{}, errors.New("--dist is required")
	}
	if branch == "" {
		return domain.SubmitResponse{}, errors.New("--branch is required")
	}

	fmt.Printf("Submitting ISO build job...\n")
	fmt.Printf("Distribution: %s\n", dist)
	fmt.Printf("Branch: %s\n", branch)
	if noCache {
		fmt.Println("Cacheless build: the worker will clear cache, chroot, auto and local first")
	}

	submission := domain.ISOSubmission{
		Dist:    dist,
		Branch:  branch,
		NoCache: noCache,
	}

	resp, err := u.chief.SubmitISO(ctx, submission)
	if err != nil {
		return domain.SubmitResponse{}, err
	}
	if resp.Error != "" {
		return domain.SubmitResponse{}, errors.New(resp.Error)
	}

	fmt.Println("ISO build submitted successfully!")
	fmt.Println("Pipeline ID: " + resp.PipelineID)

	if err := u.pipelines.SaveISOID(resp.PipelineID); err != nil {
		fmt.Printf("warning: failed to save pipeline ID: %v\n", err)
	}

	return resp, nil
}

func (u *CLIUsecase) ISOStatus(ctx context.Context, pipelineID string) (domain.ISOStatus, error) {
	if _, err := u.config.Load(); err != nil {
		return domain.ISOStatus{}, fmt.Errorf("%w: %w", ErrConfigMissing, err)
	}

	var err error
	if pipelineID == "" {
		pipelineID, err = u.pipelines.LoadISOID()
		if err != nil || pipelineID == "" {
			return domain.ISOStatus{}, ErrPipelineIDMissing
		}
	}

	fmt.Println("Checking the status of " + pipelineID + " ...")
	return u.chief.GetISOStatus(ctx, pipelineID)
}

func (u *CLIUsecase) ISOLog(ctx context.Context, pipelineID string) (string, error) {
	if _, err := u.config.Load(); err != nil {
		return "", fmt.Errorf("%w: %w", ErrConfigMissing, err)
	}

	var err error
	if pipelineID == "" {
		pipelineID, err = u.pipelines.LoadISOID()
		if err != nil || pipelineID == "" {
			return "", ErrPipelineIDMissing
		}
	}

	fmt.Println("Fetching the logs of " + pipelineID + " ...")

	logResult, err := u.chief.FetchLog(ctx, pipelineID+".iso.log")
	if err != nil {
		if isHTTPNotFound(err) {
			return "", errors.New("ISO log is not found. The worker/pipeline may have terminated ungracefully")
		}
		return "", err
	}

	return logResult, nil
}
