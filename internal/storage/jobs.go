package storage

import (
	"database/sql"
	"fmt"
	"time"
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

// JobStore handles job persistence in SQLite
type JobStore struct {
	db      *DB
	maxJobs int
}

// NewJobStore creates a new job store
func NewJobStore(db *DB, maxJobs int) *JobStore {
	if maxJobs <= 0 {
		maxJobs = 1000 // Default maximum jobs
	}
	return &JobStore{
		db:      db,
		maxJobs: maxJobs,
	}
}

// RecordJob stores job metadata in SQLite
func (s *JobStore) RecordJob(job JobInfo) error {
	query := `
		INSERT INTO jobs (
			task_uuid, package_name, package_version, maintainer, component,
			is_experimental, submitted_at, state, current_stage, build_state,
			repo_state, package_url, source_url, package_branch, source_branch
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(task_uuid) DO UPDATE SET
			package_name = excluded.package_name,
			package_version = excluded.package_version,
			maintainer = excluded.maintainer,
			component = excluded.component,
			is_experimental = excluded.is_experimental,
			state = excluded.state,
			current_stage = excluded.current_stage,
			build_state = excluded.build_state,
			repo_state = excluded.repo_state,
			package_url = excluded.package_url,
			source_url = excluded.source_url,
			package_branch = excluded.package_branch,
			source_branch = excluded.source_branch,
			updated_at = CURRENT_TIMESTAMP
	`

	_, err := s.db.Exec(query,
		job.TaskUUID, job.PackageName, job.PackageVersion, job.Maintainer, job.Component,
		job.IsExperimental, job.SubmittedAt, job.State, job.CurrentStage, job.BuildState,
		job.RepoState, job.PackageURL, job.SourceURL, job.PackageBranch, job.SourceBranch,
	)
	if err != nil {
		return fmt.Errorf("failed to record job: %w", err)
	}

	// Cleanup old jobs if exceeding max
	if err := s.cleanupOldJobs(); err != nil {
		// Log but don't fail
		fmt.Printf("Warning: failed to cleanup old jobs: %v\n", err)
	}

	return nil
}

// GetJob retrieves a job by UUID
func (s *JobStore) GetJob(taskUUID string) (*JobInfo, error) {
	query := `
		SELECT task_uuid, package_name, package_version, maintainer, component,
			   is_experimental, submitted_at, state, current_stage, build_state,
			   repo_state, package_url, source_url, package_branch, source_branch
		FROM jobs
		WHERE task_uuid = ?
	`

	var job JobInfo
	err := s.db.QueryRow(query, taskUUID).Scan(
		&job.TaskUUID, &job.PackageName, &job.PackageVersion, &job.Maintainer, &job.Component,
		&job.IsExperimental, &job.SubmittedAt, &job.State, &job.CurrentStage, &job.BuildState,
		&job.RepoState, &job.PackageURL, &job.SourceURL, &job.PackageBranch, &job.SourceBranch,
	)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("job not found: %s", taskUUID)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get job: %w", err)
	}

	return &job, nil
}

// GetRecentJobs retrieves the N most recent jobs
func (s *JobStore) GetRecentJobs(limit int) ([]*JobInfo, error) {
	if limit <= 0 {
		limit = 10
	}

	query := `
		SELECT task_uuid, package_name, package_version, maintainer, component,
			   is_experimental, submitted_at, state, current_stage, build_state,
			   repo_state, package_url, source_url, package_branch, source_branch
		FROM jobs
		ORDER BY submitted_at DESC
		LIMIT ?
	`

	rows, err := s.db.Query(query, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to list jobs: %w", err)
	}
	defer rows.Close()

	var jobs []*JobInfo
	for rows.Next() {
		var job JobInfo
		err := rows.Scan(
			&job.TaskUUID, &job.PackageName, &job.PackageVersion, &job.Maintainer, &job.Component,
			&job.IsExperimental, &job.SubmittedAt, &job.State, &job.CurrentStage, &job.BuildState,
			&job.RepoState, &job.PackageURL, &job.SourceURL, &job.PackageBranch, &job.SourceBranch,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan job: %w", err)
		}
		jobs = append(jobs, &job)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating jobs: %w", err)
	}

	return jobs, nil
}

// IsTerminalState returns true if the state is a final state that should not be overwritten.
func IsTerminalState(state string) bool {
	switch state {
	case "SUCCESS", "DONE", "FAILURE", "FAILED":
		return true
	}
	return false
}

// UpdateJobState updates the state of a job.
// Terminal states (SUCCESS, DONE, FAILURE, FAILED) are never overwritten.
func (s *JobStore) UpdateJobState(taskUUID, state string) error {
	query := `
		UPDATE jobs
		SET state = ?, updated_at = CURRENT_TIMESTAMP
		WHERE task_uuid = ?
		AND state NOT IN ('SUCCESS', 'DONE', 'FAILURE', 'FAILED')
	`

	result, err := s.db.Exec(query, state, taskUUID)
	if err != nil {
		return fmt.Errorf("failed to update job state: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		// Job not found or already in terminal state, both acceptable
		return nil
	}

	return nil
}

// UpdateJobStages updates the build and repo states of a job.
// Jobs already in a terminal state (SUCCESS, DONE, FAILURE, FAILED) are not updated.
func (s *JobStore) UpdateJobStages(taskUUID, buildState, repoState, currentStage string) error {
	query := `
		UPDATE jobs
		SET build_state = ?, repo_state = ?, current_stage = ?, updated_at = CURRENT_TIMESTAMP
		WHERE task_uuid = ?
		AND state NOT IN ('SUCCESS', 'DONE', 'FAILURE', 'FAILED')
	`

	_, err := s.db.Exec(query, buildState, repoState, currentStage, taskUUID)
	if err != nil {
		return fmt.Errorf("failed to update job stages: %w", err)
	}

	return nil
}

// cleanupOldJobs removes old jobs exceeding the maximum count
func (s *JobStore) cleanupOldJobs() error {
	query := `
		DELETE FROM jobs
		WHERE id NOT IN (
			SELECT id FROM jobs
			ORDER BY submitted_at DESC
			LIMIT ?
		)
	`

	_, err := s.db.Exec(query, s.maxJobs)
	return err
}
