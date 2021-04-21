package endpoint

// SubmissionRequest request parameter
type SubmissionRequest struct {
	Tarball string `json:"tarball"`
}

// SubmissionResponse response
type SubmissionResponse struct {
	PipelineID string   `json:"pipelineId"`
	Jobs       []string `json:"jobs"`
}
