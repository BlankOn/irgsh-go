package monitoring

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

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

// GetJobStateFromMachinery queries machinery backend for actual task state
func GetJobStateFromMachinery(ctx context.Context, client *redis.Client, taskUUID string) string {
	// Machinery stores task states with multiple possible key patterns, try them all
	possibleKeys := []string{
		"machinery_task_state_" + taskUUID,
		taskUUID,
		"machinery:task:state:" + taskUUID,
	}

	for _, stateKey := range possibleKeys {
		data, err := client.Get(ctx, stateKey).Result()
		if err != nil {
			continue // Try next key pattern
		}

		// Parse the state JSON
		var stateData map[string]interface{}
		if err := json.Unmarshal([]byte(data), &stateData); err != nil {
			continue
		}

		// Try to extract state from various possible field names
		for _, fieldName := range []string{"State", "state", "status"} {
			if state, ok := stateData[fieldName].(string); ok && state != "" {
				return state
			}
		}
	}

	// If no machinery state found, return UNKNOWN (will use cached state)
	return "UNKNOWN"
}

// GetJobStagesFromMachinery queries both build and repo task states
func GetJobStagesFromMachinery(ctx context.Context, client *redis.Client, taskUUID string) (buildState, repoState, currentStage string) {
	// Query build task state
	buildState = getTaskStateFromMachinery(ctx, client, taskUUID, "build")

	// Query repo task state
	repoState = getTaskStateFromMachinery(ctx, client, taskUUID, "repo")

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

// getTaskStateFromMachinery queries a specific task state (build or repo)
func getTaskStateFromMachinery(ctx context.Context, client *redis.Client, taskUUID, taskName string) string {
	// Try different key patterns that machinery might use
	possibleKeys := []string{
		"machinery_task_state_" + taskUUID + "_" + taskName,
		"machinery_task_state_" + taskUUID,
		taskUUID + "_" + taskName,
		taskUUID,
	}

	for _, stateKey := range possibleKeys {
		data, err := client.Get(ctx, stateKey).Result()
		if err != nil {
			continue
		}

		var stateData map[string]interface{}
		if err := json.Unmarshal([]byte(data), &stateData); err != nil {
			continue
		}

		// Check if this state belongs to the task we're looking for
		if taskNameField, ok := stateData["TaskName"].(string); ok {
			if taskNameField != taskName {
				continue // Wrong task
			}
		}

		// Extract state
		for _, fieldName := range []string{"State", "state", "status"} {
			if state, ok := stateData[fieldName].(string); ok && state != "" {
				return state
			}
		}
	}

	return "PENDING"
}
