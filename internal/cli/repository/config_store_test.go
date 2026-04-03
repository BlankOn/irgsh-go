package repository_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/blankon/irgsh-go/internal/cli/domain"
	"github.com/blankon/irgsh-go/internal/cli/repository"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFileConfigStore_SaveAndLoad(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "config-store-test")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	store := repository.NewFileConfigStore(tmpDir)

	cfg := domain.Config{
		ChiefAddress:         "http://chief.example.com",
		MaintainerSigningKey: "ABCDEF1234567890",
	}
	err = store.Save(cfg)
	require.NoError(t, err)

	// Verify files exist
	chiefContent, err := os.ReadFile(filepath.Join(tmpDir, "IRGSH_CHIEF_ADDRESS"))
	require.NoError(t, err)
	assert.Equal(t, "http://chief.example.com", string(chiefContent))

	keyContent, err := os.ReadFile(filepath.Join(tmpDir, "IRGSH_MAINTAINER_SIGNING_KEY"))
	require.NoError(t, err)
	assert.Equal(t, "ABCDEF1234567890", string(keyContent))

	// Test Load
	loaded, err := store.Load()
	require.NoError(t, err)
	assert.Equal(t, cfg, loaded)
}

func TestFileConfigStore_LoadMissing(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "config-store-test")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	store := repository.NewFileConfigStore(tmpDir)
	_, err = store.Load()
	assert.Error(t, err)
}
