package monitoring

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/RichardKnop/machinery/v1/backends/iface"
	"github.com/RichardKnop/machinery/v1/backends/result"
	"github.com/RichardKnop/machinery/v1/tasks"
	"github.com/go-redis/redis/v8"
)

const (
	// Redis key for job tracking
	jobsIndexKey    = "irgsh:jobs:index"       // Sorted set of job IDs (sorted by timestamp)
	jobKeyPrefix    = "irgsh:jobs:"            // Job metadata
	jobRetentionTTL = 7 * 24 * time.Hour      // Keep job data for 7 days
	maxJobsInIndex  = 100                      // Keep latest 100 jobs in index
)

// JobInfo contains metadata about a build job
type JobInfo struct {
	TaskUUID       string    `json:"task_uuid"`
	PackageName    string    `json:"package_name"`
	PackageVersion string    `json:"package_version"`
	Maintainer     string    `json:"maintainer"`
	Component      string    `json:"component"`
	IsExperimental bool      `json:"is_experimental"`
	SubmittedAt    time.Time `json:"submitted_at"`
	State          string    `json:"state"`          // PENDING, STARTED, SUCCESS, FAILURE
	CurrentStage   string    `json:"current_stage"`  // build, repo, completed
	BuildState     string    `json:"build_state"`    // State of build task
	RepoState      string    `json:"repo_state"`     // State of repo task
	PackageURL     string    `json:"package_url"`    // Git repository URL for package
	SourceURL      string    `json:"source_url"`     // Git repository URL for source
	PackageBranch  string    `json:"package_branch"` // Branch name for package
	SourceBranch   string    `json:"source_branch"`  // Branch name for source
}

// RecordJob stores job metadata in Redis
func (r *Registry) RecordJob(job JobInfo) error {
	jobKey := jobKeyPrefix + job.TaskUUID

	// Serialize to JSON
	data, err := json.Marshal(job)
	if err != nil {
		return fmt.Errorf("failed to marshal job info: %w", err)
	}

	// Use pipeline for atomic operations
	pipe := r.client.Pipeline()

	// Store job data with 7 day TTL
	pipe.Set(r.ctx, jobKey, data, jobRetentionTTL)

	// Add to sorted set (score = unix timestamp for chronological ordering)
	score := float64(job.SubmittedAt.Unix())
	pipe.ZAdd(r.ctx, jobsIndexKey, &redis.Z{
		Score:  score,
		Member: job.TaskUUID,
	})

	// Keep only the latest N jobs in the index
	pipe.ZRemRangeByRank(r.ctx, jobsIndexKey, 0, -maxJobsInIndex-1)

	_, err = pipe.Exec(r.ctx)
	if err != nil {
		return fmt.Errorf("failed to record job: %w", err)
	}

	return nil
}

// GetRecentJobs retrieves the N most recent jobs
func (r *Registry) GetRecentJobs(limit int) ([]*JobInfo, error) {
	if limit <= 0 {
		limit = 10
	}

	// Get job IDs from sorted set (most recent first)
	jobIDs, err := r.client.ZRevRange(r.ctx, jobsIndexKey, 0, int64(limit-1)).Result()
	if err != nil {
		return nil, fmt.Errorf("failed to list jobs: %w", err)
	}

	jobs := make([]*JobInfo, 0, len(jobIDs))
	for _, id := range jobIDs {
		job, err := r.GetJob(id)
		if err != nil {
			// Job might have expired, skip it
			continue
		}
		jobs = append(jobs, job)
	}

	return jobs, nil
}

// GetJob retrieves a job by UUID
func (r *Registry) GetJob(taskUUID string) (*JobInfo, error) {
	jobKey := jobKeyPrefix + taskUUID

	data, err := r.client.Get(r.ctx, jobKey).Result()
	if err == redis.Nil {
		return nil, fmt.Errorf("job not found: %s", taskUUID)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get job: %w", err)
	}

	var job JobInfo
	if err := json.Unmarshal([]byte(data), &job); err != nil {
		return nil, fmt.Errorf("failed to unmarshal job info: %w", err)
	}

	return &job, nil
}

// UpdateJobState updates the state of a job
func (r *Registry) UpdateJobState(taskUUID string, state string) error {
	jobKey := jobKeyPrefix + taskUUID

	// Get existing job
	data, err := r.client.Get(r.ctx, jobKey).Result()
	if err == redis.Nil {
		// Job not found, might be too old
		return nil
	}
	if err != nil {
		return fmt.Errorf("failed to get job: %w", err)
	}

	var job JobInfo
	if err := json.Unmarshal([]byte(data), &job); err != nil {
		return err
	}

	// Update state
	job.State = state

	// Store updated job
	updatedData, err := json.Marshal(job)
	if err != nil {
		return err
	}

	// Keep existing TTL
	ttl, _ := r.client.TTL(r.ctx, jobKey).Result()
	if ttl < 0 {
		ttl = jobRetentionTTL
	}

	return r.client.Set(r.ctx, jobKey, updatedData, ttl).Err()
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
