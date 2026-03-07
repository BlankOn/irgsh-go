package usecase

import (
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/blankon/irgsh-go/internal/chief/domain"
	"github.com/blankon/irgsh-go/internal/monitoring"
	"github.com/blankon/irgsh-go/pkg/httputil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestSubmissionService(tq TaskQueue, fs FileStorage, gpg GPGVerifier, js JobStore, iso ISOJobStore) *SubmissionService {
	return NewSubmissionService(tq, fs, gpg, js, iso)
}

func TestSubmitPackage_ValidationErrors(t *testing.T) {
	svc := newTestSubmissionService(&mockTaskQueue{}, &mockFileStorage{submissionsDir: t.TempDir()}, &mockGPGVerifier{}, nil, nil)

	tests := []struct {
		name       string
		submission domain.Submission
		wantMsg    string
	}{
		{
			"invalid fingerprint",
			domain.Submission{MaintainerFingerprint: "../bad", PackageName: "pkg", Tarball: "tarball"},
			"invalid maintainer fingerprint",
		},
		{
			"invalid package name",
			domain.Submission{MaintainerFingerprint: "ABC123", PackageName: "bad/name", Tarball: "tarball"},
			"invalid package name",
		},
		{
			"invalid tarball",
			domain.Submission{MaintainerFingerprint: "ABC123", PackageName: "pkg", Tarball: "bad/tarball"},
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
		sendBuildChainFn: func(taskUUID string, payload []byte) error {
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
		sendBuildChainFn: func(taskUUID string, payload []byte) error {
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

func TestBuildISO_ValidationErrors(t *testing.T) {
	svc := newTestSubmissionService(&mockTaskQueue{}, &mockFileStorage{}, &mockGPGVerifier{}, nil, nil)

	t.Run("missing repoUrl", func(t *testing.T) {
		_, err := svc.BuildISO(domain.ISOSubmission{Branch: "main"})
		require.Error(t, err)
		var httpErr httputil.HTTPError
		require.True(t, errors.As(err, &httpErr))
		assert.Equal(t, http.StatusBadRequest, httpErr.Code)
		assert.Contains(t, httpErr.Message, "repoUrl")
	})

	t.Run("missing branch", func(t *testing.T) {
		_, err := svc.BuildISO(domain.ISOSubmission{RepoURL: "https://repo.example.com"})
		require.Error(t, err)
		var httpErr httputil.HTTPError
		require.True(t, errors.As(err, &httpErr))
		assert.Equal(t, http.StatusBadRequest, httpErr.Code)
		assert.Contains(t, httpErr.Message, "branch")
	})
}

func TestBuildISO_QueueFailure(t *testing.T) {
	tq := &mockTaskQueue{
		sendISOTaskFn: func(taskUUID string, payload []byte) error {
			return errors.New("queue down")
		},
	}
	svc := newTestSubmissionService(tq, &mockFileStorage{}, &mockGPGVerifier{}, nil, nil)

	_, err := svc.BuildISO(domain.ISOSubmission{
		RepoURL: "https://repo.example.com",
		Branch:  "main",
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

	var queuedUUID string
	tq := &mockTaskQueue{
		sendISOTaskFn: func(taskUUID string, payload []byte) error {
			queuedUUID = taskUUID
			return nil
		},
	}

	svc := newTestSubmissionService(tq, &mockFileStorage{}, &mockGPGVerifier{}, nil, isoStore)

	resp, err := svc.BuildISO(domain.ISOSubmission{
		RepoURL: "https://repo.example.com",
		Branch:  "main",
	})
	require.NoError(t, err)
	assert.NotEmpty(t, resp.PipelineID)
	assert.Contains(t, resp.PipelineID, "_iso")
	assert.Equal(t, resp.PipelineID, queuedUUID)
	assert.Equal(t, "PENDING", recordedISO.State)
	assert.Equal(t, "https://repo.example.com", recordedISO.RepoURL)
}
