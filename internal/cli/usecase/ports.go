package usecase

import (
	"context"
	"io"

	"github.com/blankon/irgsh-go/internal/cli/domain"
)

type ConfigStore interface {
	Load() (domain.Config, error)
	Save(config domain.Config) error
}

type PipelineStore interface {
	LoadPackageID() (string, error)
	SavePackageID(id string) error
	LoadISOID() (string, error)
	SaveISOID(id string) error
	LoadRetryID() (string, error)
	SaveRetryID(id string) error
}

type RepoSync interface {
	Sync(repoURL, branch, targetDir string) error
}

type ShellRunner interface {
	Output(cmd string) (string, error)
	Run(cmd string) error
	RunInteractive(cmd string) error
}

type ChiefAPI interface {
	GetVersion(ctx context.Context) (domain.VersionResponse, error)
	UploadSubmission(ctx context.Context, blobPath, tokenPath string, onProgress func(uploaded, total int64)) (domain.UploadResponse, error)
	SubmitPackage(ctx context.Context, submission domain.Submission) (domain.SubmitResponse, error)
	SubmitISO(ctx context.Context, submission domain.ISOSubmission) (domain.SubmitResponse, error)
	GetPackageStatus(ctx context.Context, pipelineID string) (domain.PackageStatus, error)
	GetISOStatus(ctx context.Context, pipelineID string) (domain.ISOStatus, error)
	Retry(ctx context.Context, pipelineID string) (domain.RetryResponse, error)
	FetchLog(ctx context.Context, logPath string) (string, error)
}

type ReleaseFetcher interface {
	FetchLatest(ctx context.Context) (domain.GitHubRelease, error)
	Download(ctx context.Context, url string) (io.ReadCloser, error)
}

type UpdateApplier interface {
	Apply(reader io.Reader) error
}

// Prompter abstracts user-interactive confirmations.
type Prompter interface {
	Confirm(label string) (bool, error)
}

// DebianPackager abstracts Debian packaging shell operations.
type DebianPackager interface {
	ExtractPackageName(controlPath string) (string, error)
	ExtractVersion(changelogPath string) (string, error)
	ExtractExtendedVersion(changelogPath string) (string, error)
	ExtractChangelogMaintainer(changelogPath string) (string, error)
	ExtractUploaders(controlPath string) (string, error)
	BuildSource(dir string) error
	Sign(dir, keyFingerprint string) error
	GenBuildInfo(dir string) error
}

// GPGSigner abstracts GPG operations for the CLI.
type GPGSigner interface {
	GetIdentity(fingerprint string) (string, error)
	ClearSign(inputPath, outputPath, fingerprint string) error
}
