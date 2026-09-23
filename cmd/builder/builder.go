package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"time"

	"github.com/blankon/irgsh-go/internal/logstream"
	"github.com/blankon/irgsh-go/internal/notification"
	"github.com/blankon/irgsh-go/pkg/systemutil"
)

var errCanceled = errors.New("job canceled on request")

type buildSteps struct {
	Prepare        func(context.Context, buildJob, attemptPaths) (sourceSet, error)
	Build          func(context.Context, buildJob, attemptPaths, sourceSet) (attemptPaths, error)
	UploadArtifact func(context.Context, buildJob) error
}

func executeBuild(ctx context.Context, payload string, job buildJob, steps buildSteps) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	attempt, err := job.newAttempt(1)
	if err != nil {
		return "", err
	}
	source, err := steps.Prepare(ctx, job, attempt)
	if err != nil {
		return "", fmt.Errorf("prepare build: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if err := validateSource(ctx, attempt.Input, source); err != nil {
		return "", fmt.Errorf("validate build source: %w", err)
	}
	attempt, err = steps.Build(ctx, job, attempt, source)
	if err != nil {
		return "", fmt.Errorf("build package: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	source.DSC = filepath.Join(attempt.Input, filepath.Base(source.DSC))
	names, err := collectArtifacts(ctx, job, attempt, source)
	if err != nil {
		return "", fmt.Errorf("collect artifacts: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if err := writeArtifactArchive(ctx, job, names); err != nil {
		return "", fmt.Errorf("write artifact archive: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if err := steps.UploadArtifact(ctx, job); err != nil {
		return "", fmt.Errorf("upload artifact: %w", err)
	}
	return payload, nil
}

func uploadFinalLog(job buildJob, upload func(context.Context, buildJob) error) {
	logContext, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	if err := upload(logContext, job); err != nil {
		log.Printf("unable to upload build log: %v", err)
	}
}

func sendBuildNotification(taskUUID, status string, jobInfo notification.JobNotificationInfo) {
	notification.SendJobNotification(
		irgshConfig.Notification.WebhookURL,
		"Build",
		taskUUID,
		status,
		jobInfo,
	)
}

func Build(payload string) (next string, err error) {
	submission, err := decodeBuildSubmission(payload, irgshConfig.Builder.DistCodename)
	if err != nil {
		return "", err
	}
	job, err := newBuildJob(irgshConfig.Builder.Workdir, submission)
	if err != nil {
		return "", err
	}
	if err := systemutil.PrepareLogFile(job.Log); err != nil {
		return "", err
	}
	stopLogStream := logstream.Mirror(logPublisher, submission.TaskUUID, "build", job.Log)
	guard := cancelWatcher.Guard(submission.TaskUUID)
	status := "FAILED"
	defer func() {
		stopLogStream()
		uploadFinalLog(job, func(ctx context.Context, job buildJob) error {
			endpoint, endpointErr := chiefEndpoint(irgshConfig.Chief.Address, "/api/v1/log-upload", url.Values{"id": {submission.TaskUUID}, "type": {"build"}})
			if endpointErr != nil {
				log.Printf("unable to build log upload endpoint: %v", endpointErr)
				return nil
			}
			return uploadFile(ctx, http.DefaultClient, endpoint, "uploadFile", job.Log)
		})
		if cleanupErr := os.RemoveAll(job.Root); cleanupErr != nil {
			log.Printf("unable to remove build scratch: %v", cleanupErr)
		}
		guard.Release()
		sendBuildNotification(submission.TaskUUID, status, notification.JobNotificationInfo{
			PackageName:    submission.PackageName,
			PackageVersion: submission.PackageVersion,
			Maintainer:     submission.Maintainer,
			Dist:           submission.Dist,
			Component:      submission.Component,
			IsExperimental: submission.IsExperimental,
			SourceURL:      submission.SourceURL,
			SourceBranch:   submission.SourceBranch,
			PackageURL:     submission.PackageURL,
			PackageBranch:  submission.PackageBranch,
		})
	}()
	next, err = executeBuild(guard.Context(), payload, job, buildSteps{
		Prepare: func(ctx context.Context, job buildJob, attempt attemptPaths) (sourceSet, error) {
			_ = systemutil.WriteLog(job.Log, "##### Fetching the submission tarball from chief")
			archive := filepath.Join(attempt.Temp, "submission.tar.gz")
			if err := downloadSubmission(ctx, http.DefaultClient, irgshConfig.Chief.Address, submission.TaskUUID, archive); err != nil {
				return sourceSet{}, err
			}
			if err := ctx.Err(); err != nil {
				return sourceSet{}, err
			}
			extracted := filepath.Join(attempt.Temp, "extracted")
			if err := extractSubmission(ctx, archive, extracted); err != nil {
				return sourceSet{}, err
			}
			if err := ctx.Err(); err != nil {
				return sourceSet{}, err
			}
			return prepareSource(ctx, extracted, attempt.Input)
		},
		Build: buildPackage,
		UploadArtifact: func(ctx context.Context, job buildJob) error {
			endpoint, err := chiefEndpoint(irgshConfig.Chief.Address, "/api/v1/artifact-upload", url.Values{"id": {submission.TaskUUID}})
			if err != nil {
				return err
			}
			_ = systemutil.WriteLog(job.Log, "##### Uploading package artifacts to chief")
			return uploadFile(ctx, http.DefaultClient, endpoint, "uploadFile", job.Archive)
		},
	})
	if err != nil {
		if guard.Requested() {
			status = "CANCELED"
			_ = systemutil.WriteLog(job.Log, "[ BUILD CANCELED ] The build was stopped on request")
			return "", errCanceled
		}
		_ = systemutil.WriteLog(job.Log, "[ BUILD FAILED ] "+systemutil.FailureSummary(err))
		return "", err
	}
	status = "SUCCESS"
	_ = systemutil.WriteLog(job.Log, "[ BUILD DONE ]")
	return next, nil
}

func buildPackage(ctx context.Context, job buildJob, attempt attemptPaths, source sourceSet) (attemptPaths, error) {
	architecture, err := nativeArchitecture(ctx, systemutil.CmdExecArgsContext)
	if err != nil {
		return attempt, err
	}
	identity, err := baseIdentity(irgshConfig.Builder, architecture)
	if err != nil {
		return attempt, err
	}
	base, err := builderBasePaths(irgshConfig.Builder.Workdir, identity)
	if err != nil {
		return attempt, err
	}
	return runSbuild(ctx, job, attempt, source, base, architecture, irgshConfig.Builder.Attempts(), systemutil.CmdExecArgsContext)
}
