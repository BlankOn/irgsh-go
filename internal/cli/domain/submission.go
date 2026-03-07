package domain

// Submission is the wire format sent to the chief API.
// The JSON tags must stay in sync with internal/chief/domain/submission.go.
type Submission struct {
	PackageName            string `json:"packageName"`
	PackageVersion         string `json:"packageVersion"`
	PackageExtendedVersion string `json:"packageExtendedVersion"`
	PackageURL             string `json:"packageUrl"`
	SourceURL              string `json:"sourceUrl"`
	Maintainer             string `json:"maintainer"`
	MaintainerFingerprint  string `json:"maintainerFingerprint"`
	Component              string `json:"component"`
	IsExperimental         bool   `json:"isExperimental"`
	ForceVersion           bool   `json:"forceVersion"`
	Tarball                string `json:"tarball"`
	PackageBranch          string `json:"packageBranch"`
	SourceBranch           string `json:"sourceBranch"`
}

// SubmitParams holds the CLI input parameters for a package submission.
type SubmitParams struct {
	PackageURL     string
	SourceURL      string
	Component      string
	PackageBranch  string
	SourceBranch   string
	IsExperimental bool
	IgnoreChecks   bool
	ForceVersion   bool
}
