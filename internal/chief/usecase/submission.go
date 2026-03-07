package usecase

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/blankon/irgsh-go/internal/chief/domain"
	"github.com/blankon/irgsh-go/internal/monitoring"
	"github.com/blankon/irgsh-go/pkg/httputil"
	"github.com/blankon/irgsh-go/pkg/systemutil"
)

// SubmissionService handles package submission, retry, and ISO build workflows.
type SubmissionService struct {
	taskQueue TaskQueue
	storage   FileStorage
	gpg       GPGVerifier
	jobStore  JobStore
	isoStore  ISOJobStore
}

func NewSubmissionService(
	taskQueue TaskQueue,
	storage FileStorage,
	gpg GPGVerifier,
	jobStore JobStore,
	isoStore ISOJobStore,
) *SubmissionService {
	return &SubmissionService{
		taskQueue: taskQueue,
		storage:   storage,
		gpg:       gpg,
		jobStore:  jobStore,
		isoStore:  isoStore,
	}
}

func (ss *SubmissionService) SubmitPackage(submission domain.Submission) (domain.SubmitPayloadResponse, error) {
	if !domain.SafeIDPattern.MatchString(submission.MaintainerFingerprint) {
		return domain.SubmitPayloadResponse{}, httputil.NewHTTPError(http.StatusBadRequest, "invalid maintainer fingerprint")
	}
	if !domain.SafeIDPattern.MatchString(submission.PackageName) {
		return domain.SubmitPayloadResponse{}, httputil.NewHTTPError(http.StatusBadRequest, "invalid package name")
	}
	if !domain.SafeIDPattern.MatchString(submission.Tarball) {
		return domain.SubmitPayloadResponse{}, httputil.NewHTTPError(http.StatusBadRequest, "invalid tarball identifier")
	}

	submission.Timestamp = time.Now()
	submission.TaskUUID = submission.Timestamp.Format("2006-01-02-150405") + "_" + uuid.New().String() + "_" + submission.MaintainerFingerprint + "_" + submission.PackageName

	if err := ss.storage.EnsureDir(filepath.Join(ss.storage.SubmissionsDir(), submission.TaskUUID)); err != nil {
		log.Println(err)
		return domain.SubmitPayloadResponse{}, httputil.NewHTTPError(http.StatusInternalServerError, "500")
	}

	src := filepath.Join(ss.storage.SubmissionsDir(), submission.Tarball+".tar.gz")
	path := ss.storage.SubmissionTarballPath(submission.TaskUUID)
	if err := systemutil.MoveFile(src, path); err != nil {
		log.Println(err)
		return domain.SubmitPayloadResponse{}, httputil.NewHTTPError(http.StatusInternalServerError, "500")
	}

	if err := ss.storage.ExtractSubmission(submission.TaskUUID); err != nil {
		log.Println(err)
		return domain.SubmitPayloadResponse{}, httputil.NewHTTPError(http.StatusInternalServerError, "500")
	}

	src = filepath.Join(ss.storage.SubmissionsDir(), submission.Tarball+".token")
	path = ss.storage.SubmissionSignaturePath(submission.TaskUUID)
	if err := systemutil.MoveFile(src, path); err != nil {
		log.Println(err)
		return domain.SubmitPayloadResponse{}, httputil.NewHTTPError(http.StatusInternalServerError, "500")
	}

	if err := ss.gpg.VerifySignedSubmission(ss.storage.SubmissionDirPath(submission.TaskUUID)); err != nil {
		log.Println(err)
		return domain.SubmitPayloadResponse{}, httputil.NewHTTPError(http.StatusUnauthorized, "401 Unauthorized")
	}

	jsonStr, err := json.Marshal(submission)
	if err != nil {
		log.Println(err)
		return domain.SubmitPayloadResponse{}, httputil.NewHTTPError(http.StatusBadRequest, "400")
	}

	if err := ss.taskQueue.SendBuildChain(submission.TaskUUID, jsonStr); err != nil {
		log.Printf("Could not send build chain: %v\n", err)
		return domain.SubmitPayloadResponse{}, httputil.NewHTTPError(http.StatusInternalServerError, "500")
	}

	if ss.jobStore != nil {
		job := monitoring.JobInfo{
			TaskUUID:       submission.TaskUUID,
			PackageName:    submission.PackageName,
			PackageVersion: submission.PackageVersion,
			Maintainer:     submission.Maintainer,
			Component:      submission.Component,
			IsExperimental: submission.IsExperimental,
			SubmittedAt:    submission.Timestamp,
			State:          "PENDING",
			PackageURL:     submission.PackageURL,
			SourceURL:      submission.SourceURL,
			PackageBranch:  submission.PackageBranch,
			SourceBranch:   submission.SourceBranch,
		}
		if err := ss.jobStore.RecordJob(job); err != nil {
			log.Printf("Failed to record job: %v\n", err)
		}
	}

	return domain.SubmitPayloadResponse{PipelineID: submission.TaskUUID}, nil
}

func (ss *SubmissionService) RetryPipeline(oldTaskUUID string) (domain.SubmitPayloadResponse, error) {
	if !domain.SafeIDPattern.MatchString(oldTaskUUID) {
		return domain.SubmitPayloadResponse{}, httputil.NewHTTPError(http.StatusBadRequest, "invalid pipeline identifier")
	}
	if ss.jobStore == nil {
		return domain.SubmitPayloadResponse{}, httputil.NewHTTPError(http.StatusServiceUnavailable, `{"error": "monitoring is not enabled, retry requires job tracking"}`)
	}

	job, err := ss.jobStore.GetJob(oldTaskUUID)
	if err != nil {
		log.Printf("Job not found for retry: %s: %v\n", oldTaskUUID, err)
		return domain.SubmitPayloadResponse{}, httputil.NewHTTPError(http.StatusNotFound, `{"error": "job not found"}`)
	}

	parts := strings.Split(oldTaskUUID, "_")
	var maintainerFingerprint string
	if len(parts) >= 3 {
		maintainerFingerprint = parts[2]
	}

	newTimestamp := time.Now()
	newTaskUUID := newTimestamp.Format("2006-01-02-150405") + "_" + uuid.New().String() + "_" + maintainerFingerprint + "_" + job.PackageName

	submissionsDir := ss.storage.SubmissionsDir()
	oldTarball := filepath.Join(submissionsDir, oldTaskUUID+".tar.gz")
	newTarball := filepath.Join(submissionsDir, newTaskUUID+".tar.gz")
	oldDir := filepath.Join(submissionsDir, oldTaskUUID)
	newDir := filepath.Join(submissionsDir, newTaskUUID)

	log.Printf("Retry: copying submission files from %s to %s\n", oldTaskUUID, newTaskUUID)

	if _, err := os.Stat(oldTarball); os.IsNotExist(err) {
		log.Printf("Original submission tarball not found: %s\n", oldTarball)
		return domain.SubmitPayloadResponse{}, httputil.NewHTTPError(http.StatusNotFound, `{"error": "original submission tarball not found, cannot retry"}`)
	}

	if err := ss.storage.CopyFileWithSudo(oldTarball, newTarball); err != nil {
		log.Printf("Failed to copy submission tarball: %v\n", err)
		return domain.SubmitPayloadResponse{}, httputil.NewHTTPError(http.StatusInternalServerError, `{"error": "failed to copy submission files for retry"}`)
	}

	if _, err := os.Stat(oldDir); err == nil {
		if err := ss.storage.CopyDirWithSudo(oldDir, newDir); err != nil {
			log.Printf("Failed to copy submission directory: %v\n", err)
			return domain.SubmitPayloadResponse{}, httputil.NewHTTPError(http.StatusInternalServerError, `{"error": "failed to copy submission directory for retry"}`)
		}
	}

	if err := ss.storage.ChownWithSudo(newTarball); err != nil {
		log.Printf("Failed to chown tarball: %v\n", err)
	}

	if err := ss.storage.ChownRecursiveWithSudo(newDir); err != nil {
		log.Printf("Failed to chown submission directory: %v\n", err)
	}

	log.Printf("Retry: submission files copied successfully\n")

	submission := domain.Submission{
		TaskUUID:              newTaskUUID,
		Timestamp:             newTimestamp,
		PackageName:           job.PackageName,
		PackageVersion:        job.PackageVersion,
		PackageURL:            job.PackageURL,
		SourceURL:             job.SourceURL,
		Maintainer:            job.Maintainer,
		MaintainerFingerprint: maintainerFingerprint,
		Component:             job.Component,
		IsExperimental:        job.IsExperimental,
		PackageBranch:         job.PackageBranch,
		SourceBranch:          job.SourceBranch,
	}

	jsonStr, err := json.Marshal(submission)
	if err != nil {
		log.Println(err.Error())
		return domain.SubmitPayloadResponse{}, httputil.NewHTTPError(http.StatusInternalServerError, `{"error": "failed to marshal submission"}`)
	}

	if err := ss.taskQueue.SendBuildChain(submission.TaskUUID, jsonStr); err != nil {
		log.Printf("Could not send retry build chain: %v\n", err)
		return domain.SubmitPayloadResponse{}, httputil.NewHTTPError(http.StatusInternalServerError, `{"error": "failed to queue retry task"}`)
	}

	newJob := monitoring.JobInfo{
		TaskUUID:       newTaskUUID,
		PackageName:    job.PackageName,
		PackageVersion: job.PackageVersion,
		Maintainer:     job.Maintainer,
		Component:      job.Component,
		IsExperimental: job.IsExperimental,
		SubmittedAt:    newTimestamp,
		State:          "PENDING",
		PackageURL:     job.PackageURL,
		SourceURL:      job.SourceURL,
		PackageBranch:  job.PackageBranch,
		SourceBranch:   job.SourceBranch,
	}
	if err := ss.jobStore.RecordJob(newJob); err != nil {
		log.Printf("Failed to record retry job: %v\n", err)
	}

	log.Printf("Job %s retried as new pipeline %s\n", oldTaskUUID, newTaskUUID)

	return domain.SubmitPayloadResponse{PipelineID: newTaskUUID}, nil
}

func (ss *SubmissionService) BuildISO(submission domain.ISOSubmission) (domain.SubmitPayloadResponse, error) {
	if submission.RepoURL == "" {
		return domain.SubmitPayloadResponse{}, httputil.NewHTTPError(http.StatusBadRequest, "repoUrl is required")
	}
	if submission.Branch == "" {
		return domain.SubmitPayloadResponse{}, httputil.NewHTTPError(http.StatusBadRequest, "branch is required")
	}

	submission.Timestamp = time.Now()
	submission.TaskUUID = submission.Timestamp.Format("2006-01-02-150405") + "_" + uuid.New().String() + "_iso"

	jsonStr, err := json.Marshal(submission)
	if err != nil {
		log.Println(err.Error())
		return domain.SubmitPayloadResponse{}, httputil.NewHTTPError(http.StatusBadRequest, "400")
	}

	if err := ss.taskQueue.SendISOTask(submission.TaskUUID, jsonStr); err != nil {
		log.Printf("Could not send ISO task: %v\n", err)
		return domain.SubmitPayloadResponse{}, httputil.NewHTTPError(http.StatusInternalServerError, "500")
	}

	if ss.isoStore != nil {
		isoJob := monitoring.ISOJobInfo{
			TaskUUID:    submission.TaskUUID,
			RepoURL:     submission.RepoURL,
			Branch:      submission.Branch,
			SubmittedAt: submission.Timestamp,
			State:       "PENDING",
		}
		if err := ss.isoStore.RecordISOJob(isoJob); err != nil {
			log.Printf("Failed to record ISO job: %v\n", err)
		}
	}

	return domain.SubmitPayloadResponse{PipelineID: submission.TaskUUID}, nil
}
