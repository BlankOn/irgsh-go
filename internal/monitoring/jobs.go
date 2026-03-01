package monitoring

import (
	"fmt"

	"github.com/RichardKnop/machinery/v1/backends/iface"
	"github.com/RichardKnop/machinery/v1/backends/result"
	"github.com/RichardKnop/machinery/v1/tasks"
	"github.com/blankon/irgsh-go/internal/storage"
)

// Note: Job data is now stored in SQLite for persistence.
// Redis is still used for machinery task queue and instance heartbeats.

// JobInfo is an alias to storage.JobInfo for backward compatibility
type JobInfo = storage.JobInfo

// RecordJob stores job metadata in SQLite
func (r *Registry) RecordJob(job JobInfo) error {
	if r.jobStore == nil {
		return fmt.Errorf("job store not initialized")
	}
	return r.jobStore.RecordJob(job)
}

// GetRecentJobs retrieves the N most recent jobs from SQLite
func (r *Registry) GetRecentJobs(limit int) ([]*JobInfo, error) {
	if r.jobStore == nil {
		return nil, fmt.Errorf("job store not initialized")
	}
	return r.jobStore.GetRecentJobs(limit)
}

// GetJob retrieves a job by UUID from SQLite
func (r *Registry) GetJob(taskUUID string) (*JobInfo, error) {
	if r.jobStore == nil {
		return nil, fmt.Errorf("job store not initialized")
	}
	return r.jobStore.GetJob(taskUUID)
}

// UpdateJobState updates the state of a job in SQLite
func (r *Registry) UpdateJobState(taskUUID string, state string) error {
	if r.jobStore == nil {
		return fmt.Errorf("job store not initialized")
	}
	return r.jobStore.UpdateJobState(taskUUID, state)
}

// UpdateJobStages updates the build and repo states of a job in SQLite
func (r *Registry) UpdateJobStages(taskUUID, buildState, repoState, currentStage string) error {
	if r.jobStore == nil {
		return fmt.Errorf("job store not initialized")
	}
	return r.jobStore.UpdateJobStages(taskUUID, buildState, repoState, currentStage)
}

// GetJobStagesFromMachinery queries both build and repo task states using machinery backend
func GetJobStagesFromMachinery(backend iface.Backend, taskUUID string) (buildState, repoState, currentStage string) {
	// Query build task state using machinery API
	buildSignature := tasks.Signature{
		Name: "build",
		UUID: taskUUID,
		Args: []tasks.Arg{
			{
				Type:  "string",
				Value: "xyz",
			},
		},
	}
	buildResult := result.NewAsyncResult(&buildSignature, backend)
	buildResult.Touch()
	buildTaskState := buildResult.GetState()
	buildState = buildTaskState.State

	// Query repo task state using machinery API
	repoSignature := tasks.Signature{
		Name: "repo",
		UUID: taskUUID,
	}
	repoResult := result.NewAsyncResult(&repoSignature, backend)
	repoResult.Touch()
	repoTaskState := repoResult.GetState()
	repoState = repoTaskState.State

	// Determine current stage based on states
	if buildState == "FAILURE" {
		currentStage = "build"
	} else if buildState == "SUCCESS" && repoState == "PENDING" {
		currentStage = "repo"
	} else if buildState == "SUCCESS" && repoState == "STARTED" {
		currentStage = "repo"
	} else if buildState == "SUCCESS" && repoState == "SUCCESS" {
		currentStage = "completed"
	} else if buildState == "SUCCESS" && repoState == "FAILURE" {
		currentStage = "repo"
	} else if buildState == "STARTED" {
		currentStage = "build"
	} else if buildState == "PENDING" {
		currentStage = "build"
	} else {
		currentStage = "build"
	}

	return buildState, repoState, currentStage
}

// UpdateISOJobState updates the state of an ISO job in SQLite
func (r *Registry) UpdateISOJobState(taskUUID string, state string) error {
	if r.isoJobStore == nil {
		return fmt.Errorf("ISO job store not initialized")
	}
	return r.isoJobStore.UpdateISOJobState(taskUUID, state)
}

// ISOJobInfo is an alias to storage.ISOJobInfo for backward compatibility
type ISOJobInfo = storage.ISOJobInfo

// RecordISOJob stores ISO job metadata in SQLite
func (r *Registry) RecordISOJob(job ISOJobInfo) error {
	if r.isoJobStore == nil {
		return fmt.Errorf("ISO job store not initialized")
	}
	return r.isoJobStore.RecordISOJob(job)
}

// GetRecentISOJobs retrieves the N most recent ISO jobs from SQLite
func (r *Registry) GetRecentISOJobs(limit int) ([]*ISOJobInfo, error) {
	if r.isoJobStore == nil {
		return nil, fmt.Errorf("ISO job store not initialized")
	}
	return r.isoJobStore.GetRecentISOJobs(limit)
}

// GetISOJob retrieves an ISO job by UUID from SQLite
func (r *Registry) GetISOJob(taskUUID string) (*ISOJobInfo, error) {
	if r.isoJobStore == nil {
		return nil, fmt.Errorf("ISO job store not initialized")
	}
	return r.isoJobStore.GetISOJob(taskUUID)
}

// GetISOJobStateFromMachinery queries ISO task state using machinery backend
func GetISOJobStateFromMachinery(backend iface.Backend, taskUUID string) string {
	// Query ISO task state using machinery API
	isoSignature := tasks.Signature{
		Name: "iso",
		UUID: taskUUID,
		Args: []tasks.Arg{
			{
				Type:  "string",
				Value: "xyz",
			},
		},
	}
	isoResult := result.NewAsyncResult(&isoSignature, backend)
	isoResult.Touch()
	isoTaskState := isoResult.GetState()
	return isoTaskState.State
}
