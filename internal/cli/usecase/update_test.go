package usecase_test

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/blankon/irgsh-go/internal/cli/domain"
	"github.com/blankon/irgsh-go/internal/cli/usecase"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUpdateCLI_Success(t *testing.T) {
	svc := usecase.NewCLIUsecase(
		nil, nil, nil,
		&mockShellRunner{output: "1.2.3"},
		nil, nil, nil,
		&mockReleaseFetcher{
			release: domain.GitHubRelease{
				Assets: []domain.GitHubReleaseAsset{
					{Name: "irgsh-cli", BrowserDownloadURL: "https://example.com/irgsh-cli"},
				},
			},
			body: io.NopCloser(strings.NewReader("binary")),
		},
		&mockUpdateApplier{},
		nil, "",
	)
	err := svc.UpdateCLI(context.Background())
	assert.NoError(t, err)
}

func TestUpdateCLI_FetchError(t *testing.T) {
	svc := usecase.NewCLIUsecase(
		nil, nil, nil, nil, nil, nil, nil,
		&mockReleaseFetcher{err: errors.New("network error")},
		nil, nil, "",
	)
	err := svc.UpdateCLI(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "fetch latest release")
}

func TestUpdateCLI_AssetNotFound(t *testing.T) {
	svc := usecase.NewCLIUsecase(
		nil, nil, nil, nil, nil, nil, nil,
		&mockReleaseFetcher{
			release: domain.GitHubRelease{
				Assets: []domain.GitHubReleaseAsset{
					{Name: "other-binary", BrowserDownloadURL: "https://example.com/other"},
				},
			},
		},
		nil, nil, "",
	)
	err := svc.UpdateCLI(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "asset not found")
}

func TestUpdateCLI_DownloadError(t *testing.T) {
	svc := usecase.NewCLIUsecase(
		nil, nil, nil, nil, nil, nil, nil,
		&mockReleaseFetcher{
			release: domain.GitHubRelease{
				Assets: []domain.GitHubReleaseAsset{
					{Name: "irgsh-cli", BrowserDownloadURL: "https://example.com/irgsh-cli"},
				},
			},
			dlErr: errors.New("download failed"),
		},
		nil, nil, "",
	)
	err := svc.UpdateCLI(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "download update")
}

func TestUpdateCLI_ApplyError(t *testing.T) {
	svc := usecase.NewCLIUsecase(
		nil, nil, nil, nil, nil, nil, nil,
		&mockReleaseFetcher{
			release: domain.GitHubRelease{
				Assets: []domain.GitHubReleaseAsset{
					{Name: "irgsh-cli", BrowserDownloadURL: "https://example.com/irgsh-cli"},
				},
			},
			body: io.NopCloser(strings.NewReader("binary")),
		},
		&mockUpdateApplier{err: errors.New("permission denied")},
		nil, "",
	)
	err := svc.UpdateCLI(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "apply update")
}
