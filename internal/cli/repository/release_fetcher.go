package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/blankon/irgsh-go/internal/cli/domain"
)

const githubReleasesURL = "https://api.github.com/repos/BlankOn/irgsh-go/releases/latest"

// GitHubReleaseFetcher implements usecase.ReleaseFetcher for GitHub releases.
type GitHubReleaseFetcher struct {
	httpClient *http.Client
}

func NewGitHubReleaseFetcher() *GitHubReleaseFetcher {
	return &GitHubReleaseFetcher{httpClient: &http.Client{Timeout: 30 * time.Second}}
}

func (f *GitHubReleaseFetcher) FetchLatest(ctx context.Context) (domain.GitHubRelease, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, githubReleasesURL, nil)
	if err != nil {
		return domain.GitHubRelease{}, err
	}

	resp, err := f.httpClient.Do(req)
	if err != nil {
		return domain.GitHubRelease{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		io.Copy(io.Discard, resp.Body) // drain for connection reuse
		return domain.GitHubRelease{}, fmt.Errorf("GitHub API returned status %d", resp.StatusCode)
	}

	var release domain.GitHubRelease
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return domain.GitHubRelease{}, err
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
