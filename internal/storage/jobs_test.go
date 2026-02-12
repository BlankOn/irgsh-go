package storage

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestJobStore_RecordAndGetJob(t *testing.T) {
	db, err := NewDB(":memory:")
	require.NoError(t, err)
	defer db.Close()

	store := NewJobStore(db, 100)

	job := JobInfo{
		TaskUUID:       "test-uuid-123",
		PackageName:    "test-package",
		PackageVersion: "1.0.0",
		Maintainer:     "Test Maintainer <test@example.com>",
		Component:      "main",
		IsExperimental: false,
		SubmittedAt:    time.Now().UTC().Truncate(time.Second),
		State:          "PENDING",
		CurrentStage:   "build",
		BuildState:     "",
		RepoState:      "",
		PackageURL:     "https://github.com/test/package.git",
		SourceURL:      "https://github.com/test/source.git",
		PackageBranch:  "main",
		SourceBranch:   "master",
	}

	// Record job
	err = store.RecordJob(job)
	require.NoError(t, err)

	// Get job
	retrieved, err := store.GetJob("test-uuid-123")
	require.NoError(t, err)
	assert.Equal(t, job.TaskUUID, retrieved.TaskUUID)
	assert.Equal(t, job.PackageName, retrieved.PackageName)
	assert.Equal(t, job.PackageVersion, retrieved.PackageVersion)
	assert.Equal(t, job.Maintainer, retrieved.Maintainer)
	assert.Equal(t, job.Component, retrieved.Component)
	assert.Equal(t, job.IsExperimental, retrieved.IsExperimental)
	assert.Equal(t, job.State, retrieved.State)
	assert.Equal(t, job.CurrentStage, retrieved.CurrentStage)
	assert.Equal(t, job.PackageURL, retrieved.PackageURL)
	assert.Equal(t, job.SourceURL, retrieved.SourceURL)
	assert.Equal(t, job.PackageBranch, retrieved.PackageBranch)
	assert.Equal(t, job.SourceBranch, retrieved.SourceBranch)
}

func TestJobStore_GetJobNotFound(t *testing.T) {
	db, err := NewDB(":memory:")
	require.NoError(t, err)
	defer db.Close()

	store := NewJobStore(db, 100)

	_, err = store.GetJob("nonexistent-uuid")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "job not found")
}

func TestJobStore_UpdateJobState(t *testing.T) {
	db, err := NewDB(":memory:")
	require.NoError(t, err)
	defer db.Close()

	store := NewJobStore(db, 100)

	job := JobInfo{
		TaskUUID:       "test-uuid-456",
		PackageName:    "test-package",
		PackageVersion: "1.0.0",
		Maintainer:     "Test Maintainer",
		Component:      "main",
		SubmittedAt:    time.Now().UTC(),
		State:          "PENDING",
		CurrentStage:   "build",
	}

	err = store.RecordJob(job)
	require.NoError(t, err)

	// Update state
	err = store.UpdateJobState("test-uuid-456", "SUCCESS")
	require.NoError(t, err)

	// Verify update
	retrieved, err := store.GetJob("test-uuid-456")
	require.NoError(t, err)
	assert.Equal(t, "SUCCESS", retrieved.State)
}

func TestJobStore_UpdateJobStages(t *testing.T) {
	db, err := NewDB(":memory:")
	require.NoError(t, err)
	defer db.Close()

	store := NewJobStore(db, 100)

	job := JobInfo{
		TaskUUID:       "test-uuid-789",
		PackageName:    "test-package",
		PackageVersion: "1.0.0",
		Maintainer:     "Test Maintainer",
		Component:      "main",
		SubmittedAt:    time.Now().UTC(),
		State:          "PENDING",
		CurrentStage:   "build",
	}

	err = store.RecordJob(job)
	require.NoError(t, err)

	// Update stages
	err = store.UpdateJobStages("test-uuid-789", "SUCCESS", "PENDING", "repo")
	require.NoError(t, err)

	// Verify update
	retrieved, err := store.GetJob("test-uuid-789")
	require.NoError(t, err)
	assert.Equal(t, "SUCCESS", retrieved.BuildState)
	assert.Equal(t, "PENDING", retrieved.RepoState)
	assert.Equal(t, "repo", retrieved.CurrentStage)
}

func TestJobStore_GetRecentJobs(t *testing.T) {
	db, err := NewDB(":memory:")
	require.NoError(t, err)
	defer db.Close()

	store := NewJobStore(db, 100)

	// Create multiple jobs with different timestamps
	baseTime := time.Now().UTC()
	for i := 0; i < 5; i++ {
		job := JobInfo{
			TaskUUID:       "test-uuid-" + string(rune('a'+i)),
			PackageName:    "package-" + string(rune('a'+i)),
			PackageVersion: "1.0.0",
			Maintainer:     "Test Maintainer",
			Component:      "main",
			SubmittedAt:    baseTime.Add(time.Duration(i) * time.Hour),
			State:          "PENDING",
			CurrentStage:   "build",
		}
		err = store.RecordJob(job)
		require.NoError(t, err)
	}

	// Get recent jobs
	jobs, err := store.GetRecentJobs(3)
	require.NoError(t, err)
	assert.Len(t, jobs, 3)

	// Verify order (most recent first)
	assert.Equal(t, "test-uuid-e", jobs[0].TaskUUID)
	assert.Equal(t, "test-uuid-d", jobs[1].TaskUUID)
	assert.Equal(t, "test-uuid-c", jobs[2].TaskUUID)
}

func TestJobStore_CleanupOldJobs(t *testing.T) {
	db, err := NewDB(":memory:")
	require.NoError(t, err)
	defer db.Close()

	// Create store with max 3 jobs
	store := NewJobStore(db, 3)

	baseTime := time.Now().UTC()
	for i := 0; i < 5; i++ {
		job := JobInfo{
			TaskUUID:       "test-uuid-" + string(rune('a'+i)),
			PackageName:    "package-" + string(rune('a'+i)),
			PackageVersion: "1.0.0",
			Maintainer:     "Test Maintainer",
			Component:      "main",
			SubmittedAt:    baseTime.Add(time.Duration(i) * time.Hour),
			State:          "PENDING",
			CurrentStage:   "build",
		}
		err = store.RecordJob(job)
		require.NoError(t, err)
	}

	// Get all jobs - should only have 3 (the most recent)
	jobs, err := store.GetRecentJobs(10)
	require.NoError(t, err)
	assert.Len(t, jobs, 3)

	// Verify oldest jobs were removed
	_, err = store.GetJob("test-uuid-a")
	assert.Error(t, err)
	_, err = store.GetJob("test-uuid-b")
	assert.Error(t, err)

	// Verify newest jobs remain
	_, err = store.GetJob("test-uuid-e")
	assert.NoError(t, err)
}

func TestJobStore_RecordJobUpsert(t *testing.T) {
	db, err := NewDB(":memory:")
	require.NoError(t, err)
	defer db.Close()

	store := NewJobStore(db, 100)

	job := JobInfo{
		TaskUUID:       "test-uuid-upsert",
		PackageName:    "test-package",
		PackageVersion: "1.0.0",
		Maintainer:     "Test Maintainer",
		Component:      "main",
		SubmittedAt:    time.Now().UTC(),
		State:          "PENDING",
		CurrentStage:   "build",
	}

	// Record job first time
	err = store.RecordJob(job)
	require.NoError(t, err)

	// Update and record again (upsert)
	job.State = "STARTED"
	job.PackageVersion = "2.0.0"
	err = store.RecordJob(job)
	require.NoError(t, err)

	// Verify upsert worked
	retrieved, err := store.GetJob("test-uuid-upsert")
	require.NoError(t, err)
	assert.Equal(t, "STARTED", retrieved.State)
	assert.Equal(t, "2.0.0", retrieved.PackageVersion)
}
