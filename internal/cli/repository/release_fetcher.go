package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/blankon/irgsh-go/internal/cli/entity"
)

const githubReleasesURL = "https://api.github.com/repos/BlankOn/irgsh-go/releases/latest"

// GitHubReleaseFetcher implements usecase.ReleaseFetcher for GitHub releases.
type GitHubReleaseFetcher struct {
	httpClient *http.Client
}

func NewGitHubReleaseFetcher() *GitHubReleaseFetcher {
	return &GitHubReleaseFetcher{httpClient: &http.Client{}}
}

func (f *GitHubReleaseFetcher) FetchLatest(ctx context.Context) (entity.GitHubRelease, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, githubReleasesURL, nil)
	if err != nil {
		return entity.GitHubRelease{}, err
	}

	resp, err := f.httpClient.Do(req)
	if err != nil {
		return entity.GitHubRelease{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return entity.GitHubRelease{}, fmt.Errorf("GitHub API returned status %d", resp.StatusCode)
	}

	var release entity.GitHubRelease
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return entity.GitHubRelease{}, err
	}
	return release, nil
}

func (f *GitHubReleaseFetcher) Download(ctx context.Context, url string) (io.ReadCloser, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := f.httpClient.Do(req)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("download returned status %d", resp.StatusCode)
	}

	return resp.Body, nil
}
