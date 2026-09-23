package usecase

import (
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/blankon/irgsh-go/internal/chief/domain"
	chiefrepository "github.com/blankon/irgsh-go/internal/chief/repository"
	"github.com/blankon/irgsh-go/internal/monitoring"
	"github.com/blankon/irgsh-go/pkg/httputil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestSubmissionService(tq TaskQueue, fs FileStorage, gpg GPGVerifier, js JobStore, iso ISOJobStore) *SubmissionService {
	return NewSubmissionService(tq, fs, gpg, js, iso, nil)
}

func newTestImportService(tq TaskQueue, store ImportJobStore) *SubmissionService {
	return NewSubmissionService(tq, &mockFileStorage{}, &mockGPGVerifier{}, nil, nil, store)
}

func TestSubmitPackage_ValidationErrors(t *testing.T) {
	svc := newTestSubmissionService(&mockTaskQueue{}, &mockFileStorage{submissionsDir: t.TempDir()}, &mockGPGVerifier{}, nil, nil)

	tests := []struct {
		name       string
		submission domain.Submission
		wantMsg    string
	}{
		{
			"missing dist",
			domain.Submission{MaintainerFingerprint: "ABC123", PackageName: "pkg", Tarball: "tarball"},
			"dist is required",
		},
		{
			"unsafe dist",
			domain.Submission{Dist: "verbeek; rm -rf /", MaintainerFingerprint: "ABC123", PackageName: "pkg", Tarball: "tarball"},
			"unsupported characters",
		},
		{
			"invalid fingerprint",
			domain.Submission{Dist: "verbeek", MaintainerFingerprint: "../bad", PackageName: "pkg", Tarball: "tarball"},
			"invalid maintainer fingerprint",
		},
		{
			"invalid package name",
			domain.Submission{Dist: "verbeek", MaintainerFingerprint: "ABC123", PackageName: "bad/name", Tarball: "tarball"},
			"invalid package name",
		},
		{
			"invalid tarball",
			domain.Submission{Dist: "verbeek", MaintainerFingerprint: "ABC123", PackageName: "pkg", Tarball: "bad/tarball"},
			"invalid tarball identifier",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := svc.SubmitPackage(tt.submission)
			require.Error(t, err)
			var httpErr httputil.HTTPError
			require.True(t, errors.As(err, &httpErr))
			assert.Equal(t, http.StatusBadRequest, httpErr.Code)
			assert.Contains(t, httpErr.Message, tt.wantMsg)
		})
	}
}

func TestSubmitPackage_GPGFailure(t *testing.T) {
	tmpDir := t.TempDir()

	// Create the source tarball and token files that MoveFile expects
	tarballName := "test-tarball"
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, tarballName+".tar.gz"), []byte("data"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, tarballName+".token"), []byte("sig"), 0644))

	gpg := &mockGPGVerifier{
		verifySignedSubmissionFn: func(submissionPath string) error {
			return errors.New("bad signature")
		},
	}
	storage := &mockFileStorage{
		submissionsDir: tmpDir,
		submissionTarballPathFn: func(taskUUID string) string {
			return filepath.Join(tmpDir, taskUUID+".tar.gz")
		},
		submissionDirPathFn: func(taskUUID string) string {
			return filepath.Join(tmpDir, taskUUID)
		},
		submissionSignaturePathFn: func(taskUUID string) string {
			return filepath.Join(tmpDir, taskUUID+".sig")
		},
	}

	svc := newTestSubmissionService(&mockTaskQueue{}, storage, gpg, nil, nil)

	sub := domain.Submission{
		Dist:                  "verbeek",
		MaintainerFingerprint: "ABCDEF1234567890",
		PackageName:           "testpkg",
		Tarball:               tarballName,
	}
	_, err := svc.SubmitPackage(sub)
	require.Error(t, err)
	var httpErr httputil.HTTPError
	require.True(t, errors.As(err, &httpErr))
	assert.Equal(t, http.StatusUnauthorized, httpErr.Code)
}

func TestSubmitPackage_QueueFailure(t *testing.T) {
	tmpDir := t.TempDir()
	tarballName := "test-tarball"
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, tarballName+".tar.gz"), []byte("data"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, tarballName+".token"), []byte("sig"), 0644))

	tq := &mockTaskQueue{
		sendBuildChainFn: func(taskUUID, dist string, payload []byte) error {
			return errors.New("queue down")
		},
	}
	storage := &mockFileStorage{
		submissionsDir: tmpDir,
		submissionTarballPathFn: func(taskUUID string) string {
			return filepath.Join(tmpDir, taskUUID+".tar.gz")
		},
		submissionDirPathFn: func(taskUUID string) string {
			return filepath.Join(tmpDir, taskUUID)
		},
		submissionSignaturePathFn: func(taskUUID string) string {
			return filepath.Join(tmpDir, taskUUID+".sig")
		},
	}

	svc := newTestSubmissionService(tq, storage, &mockGPGVerifier{}, nil, nil)

	sub := domain.Submission{
		Dist:                  "verbeek",
		MaintainerFingerprint: "ABCDEF1234567890",
		PackageName:           "testpkg",
		Tarball:               tarballName,
	}
	_, err := svc.SubmitPackage(sub)
	require.Error(t, err)
	var httpErr httputil.HTTPError
	require.True(t, errors.As(err, &httpErr))
	assert.Equal(t, http.StatusInternalServerError, httpErr.Code)
}

func TestSubmitPackage_Success(t *testing.T) {
	tmpDir := t.TempDir()
	tarballName := "test-tarball"
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, tarballName+".tar.gz"), []byte("data"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, tarballName+".token"), []byte("sig"), 0644))

	var recordedJob monitoring.JobInfo
	jobStore := &mockJobStore{
		recordJobFn: func(job monitoring.JobInfo) error {
			recordedJob = job
			return nil
		},
	}

	var queuedUUID string
	tq := &mockTaskQueue{
		sendBuildChainFn: func(taskUUID, dist string, payload []byte) error {
			queuedUUID = taskUUID
			return nil
		},
	}

	storage := &mockFileStorage{
		submissionsDir: tmpDir,
		submissionTarballPathFn: func(taskUUID string) string {
			return filepath.Join(tmpDir, taskUUID+".tar.gz")
		},
		submissionDirPathFn: func(taskUUID string) string {
			return filepath.Join(tmpDir, taskUUID)
		},
		submissionSignaturePathFn: func(taskUUID string) string {
			return filepath.Join(tmpDir, taskUUID+".sig")
		},
	}

	svc := newTestSubmissionService(tq, storage, &mockGPGVerifier{}, jobStore, nil)

	sub := domain.Submission{
		Dist:                  "verbeek",
		MaintainerFingerprint: "ABCDEF1234567890",
		PackageName:           "testpkg",
		PackageVersion:        "1.0",
		Maintainer:            "Test User",
		Tarball:               tarballName,
	}
	resp, err := svc.SubmitPackage(sub)
	require.NoError(t, err)
	assert.NotEmpty(t, resp.PipelineID)
	assert.Equal(t, resp.PipelineID, queuedUUID)
	assert.Equal(t, "testpkg", recordedJob.PackageName)
	assert.Equal(t, "verbeek", recordedJob.Dist)
	assert.Equal(t, "PENDING", recordedJob.State)
}

func TestRetryPipeline_ValidationErrors(t *testing.T) {
	svc := newTestSubmissionService(&mockTaskQueue{}, &mockFileStorage{}, &mockGPGVerifier{}, &mockJobStore{}, nil)

	t.Run("invalid pipeline id", func(t *testing.T) {
		_, err := svc.RetryPipeline("bad/id")
		require.Error(t, err)
		var httpErr httputil.HTTPError
		require.True(t, errors.As(err, &httpErr))
		assert.Equal(t, http.StatusBadRequest, httpErr.Code)
	})

	t.Run("nil job store", func(t *testing.T) {
		svc := newTestSubmissionService(&mockTaskQueue{}, &mockFileStorage{}, &mockGPGVerifier{}, nil, nil)
		_, err := svc.RetryPipeline("valid-id")
		require.Error(t, err)
		var httpErr httputil.HTTPError
		require.True(t, errors.As(err, &httpErr))
		assert.Equal(t, http.StatusServiceUnavailable, httpErr.Code)
	})

	t.Run("job not found", func(t *testing.T) {
		js := &mockJobStore{
			getJobFn: func(taskUUID string) (*monitoring.JobInfo, error) {
				return nil, errors.New("not found")
			},
		}
		svc := newTestSubmissionService(&mockTaskQueue{}, &mockFileStorage{}, &mockGPGVerifier{}, js, nil)
		_, err := svc.RetryPipeline("valid-id")
		require.Error(t, err)
		var httpErr httputil.HTTPError
		require.True(t, errors.As(err, &httpErr))
		assert.Equal(t, http.StatusNotFound, httpErr.Code)
	})
}

func TestRetryPipeline_MissingTarball(t *testing.T) {
	js := &mockJobStore{
		getJobFn: func(taskUUID string) (*monitoring.JobInfo, error) {
			return &monitoring.JobInfo{
				TaskUUID:    taskUUID,
				PackageName: "testpkg",
			}, nil
		},
	}
	storage := &mockFileStorage{
		submissionsDir: t.TempDir(), // empty dir, no tarball
	}
	svc := newTestSubmissionService(&mockTaskQueue{}, storage, &mockGPGVerifier{}, js, nil)

	_, err := svc.RetryPipeline("2024-01-01-120000_uuid_FINGERPRINT_pkg")
	require.Error(t, err)
	var httpErr httputil.HTTPError
	require.True(t, errors.As(err, &httpErr))
	assert.Equal(t, http.StatusNotFound, httpErr.Code)
}

func TestRetryPipeline_CopiesOnlyTarballBeforeQueue(t *testing.T) {
	storage := chiefrepository.NewStorage(t.TempDir())
	require.NoError(t, os.Mkdir(storage.SubmissionsDir(), 0755))
	oldID := "2024-01-01-120000_uuid_FINGERPRINT_pkg"
	oldTarball := storage.SubmissionTarballPath(oldID)
	require.NoError(t, os.WriteFile(oldTarball, []byte("signed submission"), 0600))
	require.NoError(t, os.Chmod(oldTarball, 0666))
	require.NoError(t, os.Mkdir(storage.SubmissionDirPath(oldID), 0755))
	require.NoError(t, os.WriteFile(filepath.Join(storage.SubmissionDirPath(oldID), "extracted"), []byte("unused"), 0600))
	job := monitoring.JobInfo{
		TaskUUID: oldID, Dist: "verbeek", PackageName: "pkg", PackageVersion: "1.0",
		PackageURL: "https://example.test/pkg", SourceURL: "https://example.test/source",
		Maintainer: "Tester", Component: "main", IsExperimental: true,
		PackageBranch: "packaging", SourceBranch: "source",
	}
	var queued bool
	var recorded monitoring.JobInfo
	queue := &mockTaskQueue{sendBuildChainFn: func(taskUUID, dist string, payload []byte) error {
		queued = true
		assert.Equal(t, "verbeek", dist)
		copied, err := os.ReadFile(storage.SubmissionTarballPath(taskUUID))
		require.NoError(t, err)
		assert.Equal(t, []byte("signed submission"), copied)
		info, err := os.Stat(storage.SubmissionTarballPath(taskUUID))
		require.NoError(t, err)
		assert.Equal(t, os.FileMode(0666), info.Mode().Perm())
		_, err = os.Stat(storage.SubmissionDirPath(taskUUID))
		assert.True(t, os.IsNotExist(err))
		var submission domain.Submission
		require.NoError(t, json.Unmarshal(payload, &submission))
		assert.Equal(t, taskUUID, submission.TaskUUID)
		assert.Equal(t, dist, submission.Dist)
		assert.Equal(t, job.PackageName, submission.PackageName)
		assert.Equal(t, job.PackageVersion, submission.PackageVersion)
		assert.Equal(t, job.PackageURL, submission.PackageURL)
		assert.Equal(t, job.SourceURL, submission.SourceURL)
		assert.Equal(t, job.Maintainer, submission.Maintainer)
		assert.Equal(t, "FINGERPRINT", submission.MaintainerFingerprint)
		assert.Equal(t, job.Component, submission.Component)
		assert.Equal(t, job.IsExperimental, submission.IsExperimental)
		assert.Equal(t, job.PackageBranch, submission.PackageBranch)
		assert.Equal(t, job.SourceBranch, submission.SourceBranch)
		return nil
	}}
	store := &mockJobStore{
		getJobFn: func(taskUUID string) (*monitoring.JobInfo, error) {
			assert.Equal(t, oldID, taskUUID)
			return &job, nil
		},
		recordJobFn: func(newJob monitoring.JobInfo) error {
			recorded = newJob
			return nil
		},
	}
	svc := newTestSubmissionService(queue, storage, &mockGPGVerifier{}, store, nil)
	response, err := svc.RetryPipeline(oldID)
	require.NoError(t, err)
	assert.True(t, queued)
	assert.NotEqual(t, oldID, response.PipelineID)
	assert.Equal(t, response.PipelineID, recorded.TaskUUID)
	assert.Equal(t, "PENDING", recorded.State)
	assert.Equal(t, job.Dist, recorded.Dist)
	assert.Equal(t, job.PackageName, recorded.PackageName)
}

func TestRetryPipeline_CopyFailureRemovesTarball(t *testing.T) {
	storage := chiefrepository.NewStorage(t.TempDir())
	require.NoError(t, os.Mkdir(storage.SubmissionsDir(), 0755))
	oldID := "2024-01-01-120000_uuid_FINGERPRINT_pkg"
	require.NoError(t, os.Mkdir(storage.SubmissionTarballPath(oldID), 0755))
	queued := false
	queue := &mockTaskQueue{sendBuildChainFn: func(string, string, []byte) error {
		queued = true
		return nil
	}}
	store := &mockJobStore{getJobFn: func(string) (*monitoring.JobInfo, error) {
		return &monitoring.JobInfo{PackageName: "pkg", Dist: "verbeek"}, nil
	}}
	svc := newTestSubmissionService(queue, storage, &mockGPGVerifier{}, store, nil)
	_, err := svc.RetryPipeline(oldID)
	require.Error(t, err)
	var httpErr httputil.HTTPError
	require.ErrorAs(t, err, &httpErr)
	assert.Equal(t, http.StatusInternalServerError, httpErr.Code)
	assert.False(t, queued)
	entries, err := os.ReadDir(storage.SubmissionsDir())
	require.NoError(t, err)
	require.Len(t, entries, 1)
	assert.Equal(t, oldID+".tar.gz", entries[0].Name())
}

func TestRetryPipeline_QueueFailureKeepsTarball(t *testing.T) {
	storage := chiefrepository.NewStorage(t.TempDir())
	require.NoError(t, os.Mkdir(storage.SubmissionsDir(), 0755))
	oldID := "2024-01-01-120000_uuid_FINGERPRINT_pkg"
	require.NoError(t, os.WriteFile(storage.SubmissionTarballPath(oldID), []byte("signed submission"), 0600))
	var queuedID string
	queue := &mockTaskQueue{sendBuildChainFn: func(taskUUID, _ string, _ []byte) error {
		queuedID = taskUUID
		return errors.New("queue unavailable")
	}}
	store := &mockJobStore{getJobFn: func(string) (*monitoring.JobInfo, error) {
		return &monitoring.JobInfo{PackageName: "pkg", Dist: "verbeek"}, nil
	}}
	svc := newTestSubmissionService(queue, storage, &mockGPGVerifier{}, store, nil)
	_, err := svc.RetryPipeline(oldID)
	require.Error(t, err)
	var httpErr httputil.HTTPError
	require.ErrorAs(t, err, &httpErr)
	assert.Equal(t, http.StatusInternalServerError, httpErr.Code)
	require.NotEmpty(t, queuedID)
	copied, err := os.ReadFile(storage.SubmissionTarballPath(queuedID))
	require.NoError(t, err)
	assert.Equal(t, []byte("signed submission"), copied)
}

func TestBuildISO_ValidationErrors(t *testing.T) {
	svc := newTestSubmissionService(&mockTaskQueue{}, &mockFileStorage{}, &mockGPGVerifier{}, nil, nil)

	t.Run("missing dist", func(t *testing.T) {
		_, err := svc.BuildISO(domain.ISOSubmission{Branch: "main"})
		require.Error(t, err)
		var httpErr httputil.HTTPError
		require.True(t, errors.As(err, &httpErr))
		assert.Equal(t, http.StatusBadRequest, httpErr.Code)
		assert.Contains(t, httpErr.Message, "dist")
	})

	t.Run("unsafe dist", func(t *testing.T) {
		_, err := svc.BuildISO(domain.ISOSubmission{Dist: "verbeek; rm -rf /", Branch: "main"})
		require.Error(t, err)
		var httpErr httputil.HTTPError
		require.True(t, errors.As(err, &httpErr))
		assert.Equal(t, http.StatusBadRequest, httpErr.Code)
		assert.Contains(t, httpErr.Message, "unsupported characters")
	})

	t.Run("missing branch", func(t *testing.T) {
		_, err := svc.BuildISO(domain.ISOSubmission{Dist: "verbeek"})
		require.Error(t, err)
		var httpErr httputil.HTTPError
		require.True(t, errors.As(err, &httpErr))
		assert.Equal(t, http.StatusBadRequest, httpErr.Code)
		assert.Contains(t, httpErr.Message, "branch")
	})
}

func TestBuildISO_QueueFailure(t *testing.T) {
	tq := &mockTaskQueue{
		sendISOTaskFn: func(taskUUID, dist string, payload []byte) error {
			return errors.New("queue down")
		},
	}
	svc := newTestSubmissionService(tq, &mockFileStorage{}, &mockGPGVerifier{}, nil, nil)

	_, err := svc.BuildISO(domain.ISOSubmission{
		Dist:   "verbeek",
		Branch: "main",
	})
	require.Error(t, err)
	var httpErr httputil.HTTPError
	require.True(t, errors.As(err, &httpErr))
	assert.Equal(t, http.StatusInternalServerError, httpErr.Code)
}

func TestBuildISO_Success(t *testing.T) {
	var recordedISO monitoring.ISOJobInfo
	isoStore := &mockISOJobStore{
		recordISOJobFn: func(job monitoring.ISOJobInfo) error {
			recordedISO = job
			return nil
		},
	}

	var queuedUUID, queuedDist string
	var queuedPayload []byte
	tq := &mockTaskQueue{
		sendISOTaskFn: func(taskUUID, dist string, payload []byte) error {
			queuedUUID = taskUUID
			queuedDist = dist
			queuedPayload = payload
			return nil
		},
	}

	svc := newTestSubmissionService(tq, &mockFileStorage{}, &mockGPGVerifier{}, nil, isoStore)

	resp, err := svc.BuildISO(domain.ISOSubmission{
		Dist:    "verbeek",
		Branch:  "main",
		NoCache: true,
	})
	require.NoError(t, err)
	assert.NotEmpty(t, resp.PipelineID)
	assert.Contains(t, resp.PipelineID, "_iso")
	assert.Equal(t, resp.PipelineID, queuedUUID)
	assert.Equal(t, "PENDING", recordedISO.State)
	assert.Equal(t, "verbeek", recordedISO.Dist)
	assert.Equal(t, "main", recordedISO.Branch)
	// The live-build repository belongs to the worker's config now, so chief
	// records nothing for it.
	assert.Empty(t, recordedISO.RepoURL)

	// The job must be routed to the target dist's queue, and the worker needs
	// the branch and the cacheless flag off the wire.
	assert.Equal(t, "verbeek", queuedDist)
	assert.Contains(t, string(queuedPayload), `"branch":"main"`)
	assert.Contains(t, string(queuedPayload), `"noCache":true`)
	assert.NotContains(t, string(queuedPayload), "repoUrl")
}
