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

type Maintainer struct {
	KeyID string
	Name  string
	Email string
}

type SubmitPayloadResponse struct {
	PipelineId string   `json:"pipelineId"`
	Jobs       []string `json:"jobs,omitempty"`
}

type UsecaseError struct {
	Code    int
	Message string
}

func (e UsecaseError) Error() string {
	return e.Message
}
