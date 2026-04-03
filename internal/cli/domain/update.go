package domain

// GitHubReleaseAsset mirrors the GitHub API response format.
// JSON tags use snake_case to match the GitHub API contract.
type GitHubReleaseAsset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
}

// GitHubRelease mirrors the GitHub API response format.
type GitHubRelease struct {
	URL    string               `json:"url"`
	Assets []GitHubReleaseAsset `json:"assets"`
}
