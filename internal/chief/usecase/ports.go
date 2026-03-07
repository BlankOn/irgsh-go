package usecase

// Ports (interfaces) consumed by the chief usecase layer.

import (
	"github.com/blankon/irgsh-go/internal/monitoring"
)

// TaskQueue abstracts the distributed task queue (machinery).
type TaskQueue interface {
	// SendBuildChain queues a build -> repo task chain.
	SendBuildChain(taskUUID string, payload []byte) error
	// SendISOTask queues a single ISO build task.
	SendISOTask(taskUUID string, payload []byte) error
	// GetTaskState returns the current state string for a task.
	// taskName is "build", "repo", or "iso".
	GetTaskState(taskName, taskUUID string) string
}

// GPGVerifier handles GPG key listing and signature verification.
type GPGVerifier interface {
	ListKeysWithColons() (string, error)
	ListKeys() (string, error)
	VerifySignedSubmission(submissionPath string) error
	VerifyFile(filePath string) error
}

// FileStorage manages the on-disk layout for submissions, artifacts, and logs.
type FileStorage interface {
	ArtifactsDir() string
	LogsDir() string
	SubmissionsDir() string
	EnsureDir(path string) error
	SubmissionTarballPath(taskUUID string) string
	SubmissionDirPath(taskUUID string) string
	SubmissionSignaturePath(taskUUID string) string
	ExtractSubmission(taskUUID string) error
	CopyFileWithSudo(src, dst string) error
	CopyDirWithSudo(src, dst string) error
	ChownWithSudo(path string) error
	ChownRecursiveWithSudo(path string) error
}

// JobStore tracks package build job state for the dashboard and status queries.
type JobStore interface {
	RecordJob(job monitoring.JobInfo) error
	GetRecentJobs(limit int) ([]*monitoring.JobInfo, error)
	GetJob(taskUUID string) (*monitoring.JobInfo, error)
	UpdateJobState(taskUUID string, state string) error
	UpdateJobStages(taskUUID, buildState, repoState, currentStage string) error
}

// ISOJobStore tracks ISO build job state.
type ISOJobStore interface {
	RecordISOJob(job monitoring.ISOJobInfo) error
	GetRecentISOJobs(limit int) ([]*monitoring.ISOJobInfo, error)
}

// InstanceRegistry manages worker instance tracking and dashboard summaries.
type InstanceRegistry interface {
	ListInstances(instanceType monitoring.InstanceType, status monitoring.InstanceStatus) ([]*monitoring.InstanceInfo, error)
	GetSummary() (monitoring.InstanceSummary, error)
}
