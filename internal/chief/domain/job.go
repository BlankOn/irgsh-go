package domain

// SubmitPayloadResponse is the API response after a successful submission.
type SubmitPayloadResponse struct {
	PipelineID string   `json:"pipelineId"`
	Jobs       []string `json:"jobs,omitempty"`
}

// BuildStatusResponse is the API response for package build status queries.
type BuildStatusResponse struct {
	PipelineID  string `json:"pipelineId"`
	JobStatus   string `json:"jobStatus"`
	BuildStatus string `json:"buildStatus"`
	RepoStatus  string `json:"repoStatus"`
	State       string `json:"state"`
}
