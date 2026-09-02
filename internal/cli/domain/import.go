package domain

// ImportSubmission is the wire format sent to the chief API to import already
// built packages from an external Debian repository.
// The JSON tags must stay in sync with internal/chief/domain/submission.go.
type ImportSubmission struct {
	SourceURL       string   `json:"sourceUrl"`
	Dist            string   `json:"dist"`
	SourceComponent string   `json:"sourceComponent"`
	PackageNames    []string `json:"packageNames"`
	Component       string   `json:"component"`
	IsExperimental  bool     `json:"isExperimental"`
	ForceVersion    bool     `json:"forceVersion"`
	Insecure        bool     `json:"insecure"`
	KeyringPath     string   `json:"keyringPath"`
}

// ImportStatus is the chief response for an import pipeline.
type ImportStatus struct {
	PipelineID   string `json:"pipelineId"`
	JobStatus    string `json:"jobStatus"`
	ImportStatus string `json:"importStatus"`
	Error        string `json:"error"`
}

// ImportParams holds the CLI input parameters for an import submission.
type ImportParams struct {
	SourceURL       string
	Dist            string
	SourceComponent string
	PackageNames    []string
	Component       string
	IsExperimental  bool
	ForceVersion    bool
	Insecure        bool
	KeyringPath     string
}
