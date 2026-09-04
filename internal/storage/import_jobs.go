package storage

import (
	"database/sql"
	"fmt"
	"time"
)

// ImportJobInfo contains metadata about a package import job: pulling an
// already built package out of an external Debian repository and injecting it
// into ours.
type ImportJobInfo struct {
	TaskUUID   string `json:"task_uuid"`
	SourceURL  string `json:"source_url"`
	Dist       string `json:"dist"`        // source suite, e.g. "sid"
	TargetDist string `json:"target_dist"` // our distribution the packages are injected into, e.g. "verbeek"
	Packages   string `json:"packages"`    // comma separated package names
	Component  string `json:"component"`
	// Maintainer is the identity of whoever triggered the import.
	Maintainer     string    `json:"maintainer"`
	IsExperimental bool      `json:"is_experimental"`
	SubmittedAt    time.Time `json:"submitted_at"`
	State          string    `json:"state"` // PENDING, STARTED, SUCCESS, FAILURE
}

// ImportJobStore handles import job persistence in SQLite
type ImportJobStore struct {
	db            *DB
	maxImportJobs int
}

// NewImportJobStore creates a new import job store
func NewImportJobStore(db *DB, maxImportJobs int) *ImportJobStore {
	if maxImportJobs <= 0 {
		maxImportJobs = 200 // Default maximum import jobs
	}
	return &ImportJobStore{
		db:            db,
		maxImportJobs: maxImportJobs,
	}
}

// RecordImportJob stores import job metadata in SQLite
func (s *ImportJobStore) RecordImportJob(job ImportJobInfo) error {
	query := `
		INSERT INTO import_jobs (task_uuid, source_url, dist, target_dist, packages, component, maintainer, is_experimental, submitted_at, state)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(task_uuid) DO UPDATE SET
			source_url = excluded.source_url,
			dist = excluded.dist,
			target_dist = excluded.target_dist,
			packages = excluded.packages,
			component = excluded.component,
			maintainer = excluded.maintainer,
			is_experimental = excluded.is_experimental,
			state = excluded.state,
			updated_at = CURRENT_TIMESTAMP
	`

	_, err := s.db.Exec(query, job.TaskUUID, job.SourceURL, job.Dist, job.TargetDist, job.Packages,
		job.Component, job.Maintainer, job.IsExperimental, job.SubmittedAt, job.State)
	if err != nil {
		return fmt.Errorf("failed to record import job: %w", err)
	}

	// Cleanup old jobs if exceeding max
	if err := s.cleanupOldJobs(); err != nil {
		// Log but don't fail
		fmt.Printf("Warning: failed to cleanup old import jobs: %v\n", err)
	}

	return nil
}

// GetImportJob retrieves an import job by UUID
func (s *ImportJobStore) GetImportJob(taskUUID string) (*ImportJobInfo, error) {
	query := `
		SELECT task_uuid, source_url, dist, target_dist, packages, component, maintainer, is_experimental, submitted_at, state
		FROM import_jobs
		WHERE task_uuid = ?
	`

	var job ImportJobInfo
	err := s.db.QueryRow(query, taskUUID).Scan(
		&job.TaskUUID, &job.SourceURL, &job.Dist, &job.TargetDist, &job.Packages,
		&job.Component, &job.Maintainer, &job.IsExperimental, &job.SubmittedAt, &job.State,
	)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("import job not found: %s", taskUUID)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get import job: %w", err)
	}

	return &job, nil
}

// GetRecentImportJobs retrieves the N most recent import jobs
func (s *ImportJobStore) GetRecentImportJobs(limit int) ([]*ImportJobInfo, error) {
	if limit <= 0 {
		limit = 10
	}

	query := `
		SELECT task_uuid, source_url, dist, target_dist, packages, component, maintainer, is_experimental, submitted_at, state
		FROM import_jobs
		ORDER BY submitted_at DESC
		LIMIT ?
	`

	rows, err := s.db.Query(query, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to list import jobs: %w", err)
	}
	defer rows.Close()

	var jobs []*ImportJobInfo
	for rows.Next() {
		var job ImportJobInfo
		err := rows.Scan(&job.TaskUUID, &job.SourceURL, &job.Dist, &job.TargetDist, &job.Packages,
			&job.Component, &job.Maintainer, &job.IsExperimental, &job.SubmittedAt, &job.State)
		if err != nil {
			return nil, fmt.Errorf("failed to scan import job: %w", err)
		}
		jobs = append(jobs, &job)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating import jobs: %w", err)
	}

	return jobs, nil
}

// UpdateImportJobState updates the state of an import job.
// Terminal states (SUCCESS, DONE, FAILURE, FAILED) are never overwritten.
func (s *ImportJobStore) UpdateImportJobState(taskUUID, state string) error {
	query := `
		UPDATE import_jobs
		SET state = ?, updated_at = CURRENT_TIMESTAMP
		WHERE task_uuid = ?
		AND state NOT IN ('SUCCESS', 'DONE', 'FAILURE', 'FAILED')
	`

	_, err := s.db.Exec(query, state, taskUUID)
	if err != nil {
		return fmt.Errorf("failed to update import job state: %w", err)
	}

	return nil
}

// cleanupOldJobs removes old import jobs exceeding the maximum count
func (s *ImportJobStore) cleanupOldJobs() error {
	query := `
		DELETE FROM import_jobs
		WHERE id NOT IN (
			SELECT id FROM import_jobs
			ORDER BY submitted_at DESC
			LIMIT ?
		)
	`

	_, err := s.db.Exec(query, s.maxImportJobs)
	return err
}
