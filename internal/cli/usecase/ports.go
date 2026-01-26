package usecase

import (
	"context"
	"io"

	"github.com/blankon/irgsh-go/internal/cli/entity"
)

type ProgressFunc func(uploaded, total int64)

type ConfigStore interface {
	Load() (entity.Config, error)
	Save(config entity.Config) error
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
	GetVersion(ctx context.Context) (entity.VersionResponse, error)
	UploadSubmission(ctx context.Context, blobPath, tokenPath string, onProgress ProgressFunc) (entity.UploadResponse, error)
	SubmitPackage(ctx context.Context, submission entity.Submission) (entity.SubmitResponse, error)
	SubmitISO(ctx context.Context, submission entity.ISOSubmission) (entity.SubmitResponse, error)
	GetPackageStatus(ctx context.Context, pipelineID string) (entity.PackageStatus, error)
	GetISOStatus(ctx context.Context, pipelineID string) (entity.ISOStatus, error)
	Retry(ctx context.Context, pipelineID string) (entity.RetryResponse, error)
	FetchLog(ctx context.Context, logPath string) (string, error)
}

type ReleaseFetcher interface {
	FetchLatest(ctx context.Context) (entity.GitHubRelease, error)
	Download(ctx context.Context, url string) (io.ReadCloser, error)
}

type UpdateApplier interface {
	Apply(reader io.Reader) error
}
