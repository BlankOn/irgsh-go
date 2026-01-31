package entity

type VersionResponse struct {
	Version string `json:"version"`
}

type UploadResponse struct {
	ID string `json:"id"`
}

type SubmitResponse struct {
	PipelineID string `json:"pipelineId"`
	Error      string `json:"error,omitempty"`
}

type RetryResponse struct {
	PipelineID string `json:"pipelineId"`
	Error      string `json:"error,omitempty"`
}

type PackageStatus struct {
	PipelineID  string `json:"pipelineId"`
	JobStatus   string `json:"jobStatus"`
	BuildStatus string `json:"buildStatus"`
	RepoStatus  string `json:"repoStatus"`
	State       string `json:"state"`
}

type ISOStatus struct {
	PipelineID string `json:"pipelineId"`
	JobStatus  string `json:"jobStatus"`
	ISOStatus  string `json:"isoStatus"`
	State      string `json:"state"`
}
