package domain

// ISOSubmission is the wire format sent to the chief API.
// The JSON tags must stay in sync with internal/chief/domain/submission.go.
type ISOSubmission struct {
	// Dist is the target distribution this ISO is built for, e.g. "verbeek".
	Dist    string `json:"dist"`
	RepoURL string `json:"repoUrl"`
	Branch  string `json:"branch"`
}
