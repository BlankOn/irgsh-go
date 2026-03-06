package usecase_test

import (
	"context"
	"io"

	"github.com/blankon/irgsh-go/internal/cli/domain"
)

// mockConfigStore implements usecase.ConfigStore for testing.
type mockConfigStore struct {
	config domain.Config
	err    error
	saved  *domain.Config
}

func (m *mockConfigStore) Load() (domain.Config, error) {
	return m.config, m.err
}

func (m *mockConfigStore) Save(cfg domain.Config) error {
	m.saved = &cfg
	return m.err
}

// mockPipelineStore implements usecase.PipelineStore for testing.
type mockPipelineStore struct {
	packageID string
	isoID     string
	retryID   string
	saveErr   error
	loadErr   error
}

func (m *mockPipelineStore) SavePackageID(id string) error {
	m.packageID = id
	return m.saveErr
}

func (m *mockPipelineStore) LoadPackageID() (string, error) {
	return m.packageID, m.loadErr
}

func (m *mockPipelineStore) SaveISOID(id string) error {
	m.isoID = id
	return m.saveErr
}

func (m *mockPipelineStore) LoadISOID() (string, error) {
	return m.isoID, m.loadErr
}

func (m *mockPipelineStore) SaveRetryID(id string) error {
	m.retryID = id
	return m.saveErr
}

func (m *mockPipelineStore) LoadRetryID() (string, error) {
	return m.retryID, m.loadErr
}

// mockChiefAPI implements usecase.ChiefAPI for testing.
type mockChiefAPI struct {
	version      domain.VersionResponse
	versionErr   error
	uploadResp   domain.UploadResponse
	uploadErr    error
	submitResp   domain.SubmitResponse
	submitErr    error
	isoResp      domain.SubmitResponse
	isoErr       error
	pkgStatus    domain.PackageStatus
	pkgStatusErr error
	isoStatus    domain.ISOStatus
	isoStatusErr error
	retryResp    domain.RetryResponse
	retryErr     error
	fetchLogResp string
	fetchLogErr  error
}

func (m *mockChiefAPI) GetVersion(_ context.Context) (domain.VersionResponse, error) {
	return m.version, m.versionErr
}

func (m *mockChiefAPI) UploadSubmission(_ context.Context, _, _ string, _ func(int64, int64)) (domain.UploadResponse, error) {
	return m.uploadResp, m.uploadErr
}

func (m *mockChiefAPI) SubmitPackage(_ context.Context, _ domain.Submission) (domain.SubmitResponse, error) {
	return m.submitResp, m.submitErr
}

func (m *mockChiefAPI) SubmitISO(_ context.Context, _ domain.ISOSubmission) (domain.SubmitResponse, error) {
	return m.isoResp, m.isoErr
}

func (m *mockChiefAPI) GetPackageStatus(_ context.Context, _ string) (domain.PackageStatus, error) {
	return m.pkgStatus, m.pkgStatusErr
}

func (m *mockChiefAPI) GetISOStatus(_ context.Context, _ string) (domain.ISOStatus, error) {
	return m.isoStatus, m.isoStatusErr
}

func (m *mockChiefAPI) Retry(_ context.Context, _ string) (domain.RetryResponse, error) {
	return m.retryResp, m.retryErr
}

func (m *mockChiefAPI) FetchLog(_ context.Context, _ string) (string, error) {
	return m.fetchLogResp, m.fetchLogErr
}

// mockShellRunner implements usecase.ShellRunner for testing.
type mockShellRunner struct {
	output string
	err    error
}

func (m *mockShellRunner) Output(_ string) (string, error) {
	return m.output, m.err
}

func (m *mockShellRunner) Run(_ string) error {
	return m.err
}

func (m *mockShellRunner) RunInteractive(_ string) error {
	return m.err
}

// mockRepoSync implements usecase.RepoSync for testing.
type mockRepoSync struct {
	err error
}

func (m *mockRepoSync) Sync(_, _, _ string) error {
	return m.err
}

// mockDebianPackager implements usecase.DebianPackager for testing.
type mockDebianPackager struct {
	packageName     string
	version         string
	extendedVersion string
	maintainer      string
	uploaders       string
	err             error
}

func (m *mockDebianPackager) ExtractPackageName(_ string) (string, error) {
	return m.packageName, m.err
}

func (m *mockDebianPackager) ExtractVersion(_ string) (string, error) {
	return m.version, m.err
}

func (m *mockDebianPackager) ExtractExtendedVersion(_ string) (string, error) {
	return m.extendedVersion, m.err
}

func (m *mockDebianPackager) ExtractChangelogMaintainer(_ string) (string, error) {
	return m.maintainer, m.err
}

func (m *mockDebianPackager) ExtractUploaders(_ string) (string, error) {
	return m.uploaders, m.err
}

func (m *mockDebianPackager) BuildSource(_ string) error {
	return m.err
}

func (m *mockDebianPackager) Sign(_, _ string) error {
	return m.err
}

func (m *mockDebianPackager) GenBuildInfo(_ string) error {
	return m.err
}

// mockGPGSigner implements usecase.GPGSigner for testing.
type mockGPGSigner struct {
	identity string
	err      error
}

func (m *mockGPGSigner) GetIdentity(_ string) (string, error) {
	return m.identity, m.err
}

func (m *mockGPGSigner) ClearSign(_, _, _ string) error {
	return m.err
}

// mockReleaseFetcher implements usecase.ReleaseFetcher for testing.
type mockReleaseFetcher struct {
	release domain.GitHubRelease
	err     error
	body    io.ReadCloser
	dlErr   error
}

func (m *mockReleaseFetcher) FetchLatest(_ context.Context) (domain.GitHubRelease, error) {
	return m.release, m.err
}

func (m *mockReleaseFetcher) Download(_ context.Context, _ string) (io.ReadCloser, error) {
	return m.body, m.dlErr
}

// mockUpdateApplier implements usecase.UpdateApplier for testing.
type mockUpdateApplier struct {
	err error
}

func (m *mockUpdateApplier) Apply(_ io.Reader) error {
	return m.err
}

// mockPrompter implements usecase.Prompter for testing.
type mockPrompter struct {
	confirmed bool
	err       error
}

func (m *mockPrompter) Confirm(_ string) (bool, error) {
	return m.confirmed, m.err
}
