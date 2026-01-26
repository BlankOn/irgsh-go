package entity

type GitHubReleaseAsset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
}

type GitHubRelease struct {
	URL    string               `json:"url"`
	Assets []GitHubReleaseAsset `json:"assets"`
}
