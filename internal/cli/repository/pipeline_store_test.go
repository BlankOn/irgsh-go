package repository_test

import (
	"os"
	"testing"

	"github.com/blankon/irgsh-go/internal/cli/repository"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFilePipelineStore_Package(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "pipeline-store-test")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	store := repository.NewFilePipelineStore(tmpDir)

	err = store.SavePackageID("pkg-uuid-123")
	require.NoError(t, err)

	id, err := store.LoadPackageID()
	require.NoError(t, err)
	assert.Equal(t, "pkg-uuid-123", id)
}

func TestFilePipelineStore_ISO(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "pipeline-store-test")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	store := repository.NewFilePipelineStore(tmpDir)

	err = store.SaveISOID("iso-uuid-456")
	require.NoError(t, err)

	id, err := store.LoadISOID()
	require.NoError(t, err)
	assert.Equal(t, "iso-uuid-456", id)
}

func TestFilePipelineStore_Retry(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "pipeline-store-test")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	store := repository.NewFilePipelineStore(tmpDir)

	err = store.SaveRetryID("retry-uuid-789")
	require.NoError(t, err)

	id, err := store.LoadRetryID()
	require.NoError(t, err)
	assert.Equal(t, "retry-uuid-789", id)
}

func TestFilePipelineStore_LoadMissing(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "pipeline-store-test")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	store := repository.NewFilePipelineStore(tmpDir)
	_, err = store.LoadPackageID()
	assert.Error(t, err)
}
