package storage

import (
	"database/sql"
	"fmt"
	"time"
)

// ISOJobInfo contains metadata about an ISO build job
type ISOJobInfo struct {
	TaskUUID    string    `json:"task_uuid"`
	RepoURL     string    `json:"repo_url"`
	Branch      string    `json:"branch"`
	SubmittedAt time.Time `json:"submitted_at"`
	State       string    `json:"state"` // PENDING, STARTED, SUCCESS, FAILURE
}

// ISOJobStore handles ISO job persistence in SQLite
type ISOJobStore struct {
	db         *DB
	maxISOJobs int
}

// NewISOJobStore creates a new ISO job store
func NewISOJobStore(db *DB, maxISOJobs int) *ISOJobStore {
	if maxISOJobs <= 0 {
		maxISOJobs = 200 // Default maximum ISO jobs
	}
	return &ISOJobStore{
		db:         db,
		maxISOJobs: maxISOJobs,
	}
}

// RecordISOJob stores ISO job metadata in SQLite
func (s *ISOJobStore) RecordISOJob(job ISOJobInfo) error {
	query := `
		INSERT INTO iso_jobs (task_uuid, repo_url, branch, submitted_at, state)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(task_uuid) DO UPDATE SET
			repo_url = excluded.repo_url,
			branch = excluded.branch,
			state = excluded.state,
			updated_at = CURRENT_TIMESTAMP
	`

	_, err := s.db.Exec(query, job.TaskUUID, job.RepoURL, job.Branch, job.SubmittedAt, job.State)
	if err != nil {
		return fmt.Errorf("failed to record ISO job: %w", err)
	}

	// Cleanup old jobs if exceeding max
	if err := s.cleanupOldJobs(); err != nil {
		// Log but don't fail
		fmt.Printf("Warning: failed to cleanup old ISO jobs: %v\n", err)
	}

	return nil
}

// GetISOJob retrieves an ISO job by UUID
func (s *ISOJobStore) GetISOJob(taskUUID string) (*ISOJobInfo, error) {
	query := `
		SELECT task_uuid, repo_url, branch, submitted_at, state
		FROM iso_jobs
		WHERE task_uuid = ?
	`

	var job ISOJobInfo
	err := s.db.QueryRow(query, taskUUID).Scan(
		&job.TaskUUID, &job.RepoURL, &job.Branch, &job.SubmittedAt, &job.State,
	)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("ISO job not found: %s", taskUUID)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get ISO job: %w", err)
	}

	return &job, nil
}

// GetRecentISOJobs retrieves the N most recent ISO jobs
func (s *ISOJobStore) GetRecentISOJobs(limit int) ([]*ISOJobInfo, error) {
	if limit <= 0 {
		limit = 10
	}

	query := `
		SELECT task_uuid, repo_url, branch, submitted_at, state
		FROM iso_jobs
		ORDER BY submitted_at DESC
		LIMIT ?
	`

	rows, err := s.db.Query(query, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to list ISO jobs: %w", err)
	}
	defer rows.Close()

	var jobs []*ISOJobInfo
	for rows.Next() {
		var job ISOJobInfo
		err := rows.Scan(&job.TaskUUID, &job.RepoURL, &job.Branch, &job.SubmittedAt, &job.State)
		if err != nil {
			return nil, fmt.Errorf("failed to scan ISO job: %w", err)
		}
		jobs = append(jobs, &job)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating ISO jobs: %w", err)
	}

	return jobs, nil
}

// UpdateISOJobState updates the state of an ISO job
func (s *ISOJobStore) UpdateISOJobState(taskUUID, state string) error {
	query := `
		UPDATE iso_jobs
		SET state = ?, updated_at = CURRENT_TIMESTAMP
		WHERE task_uuid = ?
	`

	_, err := s.db.Exec(query, state, taskUUID)
	if err != nil {
		return fmt.Errorf("failed to update ISO job state: %w", err)
	}

	return nil
}

// cleanupOldJobs removes old ISO jobs exceeding the maximum count
func (s *ISOJobStore) cleanupOldJobs() error {
	query := `
		DELETE FROM iso_jobs
		WHERE id NOT IN (
			SELECT id FROM iso_jobs
			ORDER BY submitted_at DESC
			LIMIT ?
		)
	`

	_, err := s.db.Exec(query, s.maxISOJobs)
	return err
}
