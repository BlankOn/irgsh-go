package domain

type ISOSubmission struct {
	RepoURL string `json:"repoUrl"`
	Branch  string `json:"branch"`
}
