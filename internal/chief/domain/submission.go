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

// ImportSubmission represents a request to import already built packages from
// an external Debian repository into ours.
// The JSON tags must stay in sync with internal/cli/domain/import.go.
type ImportSubmission struct {
	TaskUUID  string    `json:"taskUUID"`
	Timestamp time.Time `json:"timestamp"`
	// SourceURL is the base URL of the Debian repository to import from.
	SourceURL string `json:"sourceUrl"`
	// Dist is the suite in the source repository, e.g. "sid".
	Dist string `json:"dist"`
	// SourceComponent is the component to look in on the source side,
	// defaulting to "main".
	SourceComponent string `json:"sourceComponent"`
	// PackageNames are the binary package names to import. Every binary built
	// from the same source package is imported along with them.
	PackageNames []string `json:"packageNames"`
	// Component is the component to inject into on our side.
	Component      string `json:"component"`
	IsExperimental bool   `json:"isExperimental"`
	ForceVersion   bool   `json:"forceVersion"`
	// Insecure imports from a repository whose Release file cannot be
	// verified against the worker's keyrings.
	Insecure bool `json:"insecure"`
	// KeyringPath is an optional keyring on the repo worker to verify the
	// source repository against.
	KeyringPath string `json:"keyringPath"`
	// Maintainer is the identity of whoever triggered the import, taken from
	// the CLI's configured signing key.
	Maintainer string `json:"maintainer"`
	// DryRun fetches and checks the packages without injecting them.
	DryRun bool `json:"dryRun"`
	// IgnoreDependencies injects the packages even when they are not
	// installable on top of our repository.
	IgnoreDependencies bool `json:"ignoreDependencies"`
}

// RepoInfo describes the repository packages are published to, so that a
// client can check an import against the real target.
type RepoInfo struct {
	PublicURL      string `json:"publicUrl"`
	DistCodename   string `json:"distCodename"`
	DistComponents string `json:"distComponents"`
}

// ISOSubmission represents an ISO build request.
// The JSON tags must stay in sync with internal/cli/domain/iso.go.
type ISOSubmission struct {
	TaskUUID  string    `json:"taskUUID"`
	Timestamp time.Time `json:"timestamp"`
	RepoURL   string    `json:"repoUrl"`
	Branch    string    `json:"branch"`
}
