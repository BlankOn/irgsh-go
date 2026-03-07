package usecase

import (
	"bytes"
	"compress/gzip"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/blankon/irgsh-go/pkg/httputil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func gzipBytes(t *testing.T, data []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	_, err := gz.Write(data)
	require.NoError(t, err)
	require.NoError(t, gz.Close())
	return buf.Bytes()
}

func TestUploadArtifact_InvalidID(t *testing.T) {
	svc := NewUploadService(&mockFileStorage{artifactsDir: t.TempDir()}, &mockGPGVerifier{})
	err := svc.UploadArtifact("../bad", bytes.NewReader(nil))
	require.Error(t, err)
	var httpErr httputil.HTTPError
	require.True(t, errors.As(err, &httpErr))
	assert.Equal(t, http.StatusBadRequest, httpErr.Code)
}

func TestUploadArtifact_InvalidContentType(t *testing.T) {
	dir := t.TempDir()
	svc := NewUploadService(&mockFileStorage{artifactsDir: dir}, &mockGPGVerifier{})

	// Plain text is not gzip
	err := svc.UploadArtifact("valid-id", bytes.NewReader([]byte("not a gzip file")))
	require.Error(t, err)
	var httpErr httputil.HTTPError
	require.True(t, errors.As(err, &httpErr))
	assert.Equal(t, http.StatusBadRequest, httpErr.Code)

	// File should have been cleaned up
	_, statErr := os.Stat(filepath.Join(dir, "valid-id.tar.gz"))
	assert.True(t, os.IsNotExist(statErr))
}

func TestUploadArtifact_Success(t *testing.T) {
	dir := t.TempDir()
	svc := NewUploadService(&mockFileStorage{artifactsDir: dir}, &mockGPGVerifier{})

	content := gzipBytes(t, []byte("hello world"))
	err := svc.UploadArtifact("my-artifact", bytes.NewReader(content))
	require.NoError(t, err)

	written, err := os.ReadFile(filepath.Join(dir, "my-artifact.tar.gz"))
	require.NoError(t, err)
	assert.Equal(t, content, written)
}

func TestUploadLog_InvalidID(t *testing.T) {
	svc := NewUploadService(&mockFileStorage{logsDir: t.TempDir()}, &mockGPGVerifier{})

	err := svc.UploadLog("../bad", "build", bytes.NewReader([]byte("log data")))
	require.Error(t, err)
	var httpErr httputil.HTTPError
	require.True(t, errors.As(err, &httpErr))
	assert.Equal(t, http.StatusBadRequest, httpErr.Code)
	assert.Contains(t, httpErr.Message, "invalid log id")
}

func TestUploadLog_InvalidLogType(t *testing.T) {
	svc := NewUploadService(&mockFileStorage{logsDir: t.TempDir()}, &mockGPGVerifier{})

	err := svc.UploadLog("valid-id", "../bad", bytes.NewReader([]byte("log data")))
	require.Error(t, err)
	var httpErr httputil.HTTPError
	require.True(t, errors.As(err, &httpErr))
	assert.Equal(t, http.StatusBadRequest, httpErr.Code)
	assert.Contains(t, httpErr.Message, "invalid log type")
}

func TestUploadLog_InvalidContentType(t *testing.T) {
	dir := t.TempDir()
	svc := NewUploadService(&mockFileStorage{logsDir: dir}, &mockGPGVerifier{})

	// gzip content is not text/plain
	content := gzipBytes(t, []byte("binary data"))
	err := svc.UploadLog("valid-id", "build", bytes.NewReader(content))
	require.Error(t, err)
	var httpErr httputil.HTTPError
	require.True(t, errors.As(err, &httpErr))
	assert.Equal(t, http.StatusBadRequest, httpErr.Code)
}

func TestUploadLog_Success(t *testing.T) {
	dir := t.TempDir()
	svc := NewUploadService(&mockFileStorage{logsDir: dir}, &mockGPGVerifier{})

	logContent := "Build started\nBuild completed\n"
	err := svc.UploadLog("my-job", "build", bytes.NewReader([]byte(logContent)))
	require.NoError(t, err)

	written, err := os.ReadFile(filepath.Join(dir, "my-job.build.log"))
	require.NoError(t, err)
	assert.Equal(t, logContent, string(written))
}

func TestUploadSubmission_GPGFailure(t *testing.T) {
	dir := t.TempDir()
	gpg := &mockGPGVerifier{
		verifyFileFn: func(filePath string) error {
			return errors.New("bad signature")
		},
	}
	svc := NewUploadService(&mockFileStorage{submissionsDir: dir}, gpg)

	content := gzipBytes(t, []byte("payload"))
	_, err := svc.UploadSubmission([]byte("token-data"), bytes.NewReader(content))
	require.Error(t, err)
	var httpErr httputil.HTTPError
	require.True(t, errors.As(err, &httpErr))
	assert.Equal(t, http.StatusUnauthorized, httpErr.Code)
}

func TestUploadSubmission_InvalidContentType(t *testing.T) {
	dir := t.TempDir()
	svc := NewUploadService(&mockFileStorage{submissionsDir: dir}, &mockGPGVerifier{})

	_, err := svc.UploadSubmission([]byte("token"), bytes.NewReader([]byte("not gzip")))
	require.Error(t, err)
	var httpErr httputil.HTTPError
	require.True(t, errors.As(err, &httpErr))
	assert.Equal(t, http.StatusBadRequest, httpErr.Code)
}

func TestUploadSubmission_Success(t *testing.T) {
	dir := t.TempDir()
	svc := NewUploadService(&mockFileStorage{submissionsDir: dir}, &mockGPGVerifier{})

	content := gzipBytes(t, []byte("tarball content"))
	id, err := svc.UploadSubmission([]byte("token-data"), bytes.NewReader(content))
	require.NoError(t, err)
	assert.NotEmpty(t, id)

	// Both files should exist
	_, err = os.Stat(filepath.Join(dir, id+".token"))
	assert.NoError(t, err)
	_, err = os.Stat(filepath.Join(dir, id+".tar.gz"))
	assert.NoError(t, err)
}
