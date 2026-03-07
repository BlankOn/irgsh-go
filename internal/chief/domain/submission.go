package domain

import "time"

// Submission represents a package build submission from a maintainer.
// The JSON tags must stay in sync with internal/cli/domain/submission.go.
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

// ISOSubmission represents an ISO build request.
// The JSON tags must stay in sync with internal/cli/domain/iso.go.
type ISOSubmission struct {
	TaskUUID  string    `json:"taskUUID"`
	Timestamp time.Time `json:"timestamp"`
	RepoURL   string    `json:"repoUrl"`
	Branch    string    `json:"branch"`
}
