package usecase

import "time"

type Submission struct {
	TaskUUID               string    `json:"taskUUID"`
	Timestamp              time.Time `json:"timestamp"`
	PackageName            string    `json:"packageName"`
	PackageVersion         string    `json:"packageVersion"`
	PackageExtendedVersion string    `json:"packageExtendedVersion"`
	PackageURL             string    `json:"packageUrl"`
	SourceURL              string    `json:"sourceUrl"`
	Maintainer             string    `json:"maintainer"`
	MaintainerFingerprint  string    `json:"maintainerFingerprint"`
	Component              string    `json:"component"`
	IsExperimental         bool      `json:"isExperimental"`
	ForceVersion           bool      `json:"forceVersion"`
	Tarball                string    `json:"tarball"`
	PackageBranch          string    `json:"packageBranch"`
	SourceBranch           string    `json:"sourceBranch"`
}

type ISOSubmission struct {
	TaskUUID  string    `json:"taskUUID"`
	Timestamp time.Time `json:"timestamp"`
	RepoURL   string    `json:"repoUrl"`
	Branch    string    `json:"branch"`
}

type Maintainer struct {
	KeyID string
	Name  string
	Email string
}

type SubmitPayloadResponse struct {
	PipelineID string   `json:"pipelineId"`
	Jobs       []string `json:"jobs,omitempty"`
}

type BuildStatusResponse struct {
	PipelineID  string `json:"pipelineId"`
	JobStatus   string `json:"jobStatus"`
	BuildStatus string `json:"buildStatus"`
	RepoStatus  string `json:"repoStatus"`
	State       string `json:"state"`
}
