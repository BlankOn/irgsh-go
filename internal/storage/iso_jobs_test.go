package storage

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestISOJobStore_RecordAndGetJob(t *testing.T) {
	db, err := NewDB(":memory:")
	require.NoError(t, err)
	defer db.Close()

	store := NewISOJobStore(db, 100)

	job := ISOJobInfo{
		TaskUUID:    "iso-uuid-123",
		RepoURL:     "https://github.com/test/iso-repo.git",
		Branch:      "main",
		SubmittedAt: time.Now().UTC().Truncate(time.Second),
		State:       "PENDING",
	}

	// Record job
	err = store.RecordISOJob(job)
	require.NoError(t, err)

	// Get job
	retrieved, err := store.GetISOJob("iso-uuid-123")
	require.NoError(t, err)
	assert.Equal(t, job.TaskUUID, retrieved.TaskUUID)
	assert.Equal(t, job.RepoURL, retrieved.RepoURL)
	assert.Equal(t, job.Branch, retrieved.Branch)
	assert.Equal(t, job.State, retrieved.State)
}

func TestISOJobStore_GetJobNotFound(t *testing.T) {
	db, err := NewDB(":memory:")
	require.NoError(t, err)
	defer db.Close()

	store := NewISOJobStore(db, 100)

	_, err = store.GetISOJob("nonexistent-uuid")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "ISO job not found")
}

func TestISOJobStore_UpdateJobState(t *testing.T) {
	db, err := NewDB(":memory:")
	require.NoError(t, err)
	defer db.Close()

	store := NewISOJobStore(db, 100)

	job := ISOJobInfo{
		TaskUUID:    "iso-uuid-456",
		RepoURL:     "https://github.com/test/iso-repo.git",
		Branch:      "main",
		SubmittedAt: time.Now().UTC(),
		State:       "PENDING",
	}

	err = store.RecordISOJob(job)
	require.NoError(t, err)

	// Update state
	err = store.UpdateISOJobState("iso-uuid-456", "SUCCESS")
	require.NoError(t, err)

	// Verify update
	retrieved, err := store.GetISOJob("iso-uuid-456")
	require.NoError(t, err)
	assert.Equal(t, "SUCCESS", retrieved.State)
}

func TestISOJobStore_GetRecentJobs(t *testing.T) {
	db, err := NewDB(":memory:")
	require.NoError(t, err)
	defer db.Close()

	store := NewISOJobStore(db, 100)

	// Create multiple jobs with different timestamps
	baseTime := time.Now().UTC()
	for i := 0; i < 5; i++ {
		job := ISOJobInfo{
			TaskUUID:    "iso-uuid-" + string(rune('a'+i)),
			RepoURL:     "https://github.com/test/iso-" + string(rune('a'+i)) + ".git",
			Branch:      "main",
			SubmittedAt: baseTime.Add(time.Duration(i) * time.Hour),
			State:       "PENDING",
		}
		err = store.RecordISOJob(job)
		require.NoError(t, err)
	}

	// Get recent jobs
	jobs, err := store.GetRecentISOJobs(3)
	require.NoError(t, err)
	assert.Len(t, jobs, 3)

	// Verify order (most recent first)
	assert.Equal(t, "iso-uuid-e", jobs[0].TaskUUID)
	assert.Equal(t, "iso-uuid-d", jobs[1].TaskUUID)
	assert.Equal(t, "iso-uuid-c", jobs[2].TaskUUID)
}

func TestISOJobStore_CleanupOldJobs(t *testing.T) {
	db, err := NewDB(":memory:")
	require.NoError(t, err)
	defer db.Close()

	// Create store with max 3 jobs
	store := NewISOJobStore(db, 3)

	baseTime := time.Now().UTC()
	for i := 0; i < 5; i++ {
		job := ISOJobInfo{
			TaskUUID:    "iso-uuid-" + string(rune('a'+i)),
			RepoURL:     "https://github.com/test/iso-" + string(rune('a'+i)) + ".git",
			Branch:      "main",
			SubmittedAt: baseTime.Add(time.Duration(i) * time.Hour),
			State:       "PENDING",
		}
		err = store.RecordISOJob(job)
		require.NoError(t, err)
	}

	// Get all jobs - should only have 3 (the most recent)
	jobs, err := store.GetRecentISOJobs(10)
	require.NoError(t, err)
	assert.Len(t, jobs, 3)

	// Verify oldest jobs were removed
	_, err = store.GetISOJob("iso-uuid-a")
	assert.Error(t, err)
	_, err = store.GetISOJob("iso-uuid-b")
	assert.Error(t, err)

	// Verify newest jobs remain
	_, err = store.GetISOJob("iso-uuid-e")
	assert.NoError(t, err)
}

func TestISOJobStore_RecordJobUpsert(t *testing.T) {
	db, err := NewDB(":memory:")
	require.NoError(t, err)
	defer db.Close()

	store := NewISOJobStore(db, 100)

	job := ISOJobInfo{
		TaskUUID:    "iso-uuid-upsert",
		RepoURL:     "https://github.com/test/iso-repo.git",
		Branch:      "main",
		SubmittedAt: time.Now().UTC(),
		State:       "PENDING",
	}

	// Record job first time
	err = store.RecordISOJob(job)
	require.NoError(t, err)

	// Update and record again (upsert)
	job.State = "STARTED"
	job.Branch = "develop"
	err = store.RecordISOJob(job)
	require.NoError(t, err)

	// Verify upsert worked
	retrieved, err := store.GetISOJob("iso-uuid-upsert")
	require.NoError(t, err)
	assert.Equal(t, "STARTED", retrieved.State)
	assert.Equal(t, "develop", retrieved.Branch)
}
