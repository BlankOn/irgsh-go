package usecase_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/blankon/irgsh-go/internal/cli/usecase"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	return home
}

func TestPruneTmp_RemovesEverythingAndReportsSize(t *testing.T) {
	home := setHome(t)
	tmp := filepath.Join(home, ".irgsh", "tmp")
	require.NoError(t, os.MkdirAll(filepath.Join(tmp, "aaa", "package", "debian"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(tmp, "aaa", "package", "debian", "control"), []byte("12345"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(tmp, "aaa.tar.gz"), []byte("1234567890"), 0o644))
	require.NoError(t, os.MkdirAll(filepath.Join(tmp, "aaa.localbuild"), 0o755))

	svc := usecase.NewCLIUsecase(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, "")
	result, err := svc.PruneTmp()

	require.NoError(t, err)
	assert.Equal(t, 3, result.Removed)
	assert.Equal(t, int64(15), result.Bytes)

	entries, err := os.ReadDir(tmp)
	require.NoError(t, err)
	assert.Empty(t, entries)
}

func TestPruneTmp_KeepsConfigAndSiblingFiles(t *testing.T) {
	home := setHome(t)
	irgsh := filepath.Join(home, ".irgsh")
	require.NoError(t, os.MkdirAll(filepath.Join(irgsh, "tmp", "aaa"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(irgsh, "config.yml"), []byte("chief"), 0o644))

	svc := usecase.NewCLIUsecase(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, "")
	_, err := svc.PruneTmp()

	require.NoError(t, err)
	assert.FileExists(t, filepath.Join(irgsh, "config.yml"))
	assert.DirExists(t, filepath.Join(irgsh, "tmp"))
}

func TestPruneTmp_MissingTmpDirIsNotAnError(t *testing.T) {
	setHome(t)

	svc := usecase.NewCLIUsecase(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, "")
	result, err := svc.PruneTmp()

	require.NoError(t, err)
	assert.Zero(t, result.Removed)
	assert.Zero(t, result.Bytes)
}
