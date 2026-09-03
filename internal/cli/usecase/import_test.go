package usecase_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/blankon/irgsh-go/internal/cli/domain"
	"github.com/blankon/irgsh-go/internal/cli/usecase"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSplitPackageNames(t *testing.T) {
	cases := map[string][]string{
		"grub-pc":                      {"grub-pc"},
		"grub-pc,calamares":            {"grub-pc", "calamares"},
		"grub-pc calamares":            {"grub-pc", "calamares"},
		"grub-pc, calamares ,plymouth": {"grub-pc", "calamares", "plymouth"},
		"":                             nil,
		"   ":                          nil,
	}

	for input, want := range cases {
		assert.Equal(t, want, usecase.SplitPackageNames(input), "input %q", input)
	}
}

func newImportUsecase(t *testing.T, chief *mockChiefAPI, pipelines *mockPipelineStore) *usecase.CLIUsecase {
	t.Helper()
	// A shell that reports apt as unavailable, so the local dependency check
	// is skipped: the tests here are about submission, not about apt.
	return newImportUsecaseWithShell(t, chief, pipelines, &mockShellRunner{err: errors.New("not found")})
}

func newImportUsecaseWithShell(t *testing.T, chief *mockChiefAPI, pipelines *mockPipelineStore, shell usecase.ShellRunner) *usecase.CLIUsecase {
	t.Helper()
	return usecase.NewCLIUsecase(
		&mockConfigStore{config: domain.Config{
			ChiefAddress:         "https://irgsh.example.id",
			MaintainerSigningKey: "54495BCCA444849BD55A84ED5115CB575CE255A8",
		}},
		pipelines, chief, shell, nil, nil,
		&mockGPGSigner{identity: "Herpiko Dwi Aguno <herpiko@gmail.com>"},
		nil, nil, nil, "1.0.0",
	)
}

func TestSubmitImport_ValidationErrors(t *testing.T) {
	base := domain.ImportParams{
		SourceURL:    "https://kartolo.sby.datautama.net.id/debian/",
		Dist:         "sid",
		PackageNames: []string{"grub-pc"},
	}

	cases := map[string]struct {
		mutate func(*domain.ImportParams)
		want   string
	}{
		"missing source":  {func(p *domain.ImportParams) { p.SourceURL = "" }, "--source is required"},
		"missing dist":    {func(p *domain.ImportParams) { p.Dist = "" }, "--dist is required"},
		"missing package": {func(p *domain.ImportParams) { p.PackageNames = nil }, "--package-name is required"},
		"unsafe package":  {func(p *domain.ImportParams) { p.PackageNames = []string{"grub;reboot"} }, "invalid package name"},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			params := base
			tc.mutate(&params)
			_, err := newImportUsecase(t, &mockChiefAPI{}, &mockPipelineStore{}).SubmitImport(context.Background(), params)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.want)
		})
	}
}

func TestSubmitImport_AppliesDefaultsAndSavesPipelineID(t *testing.T) {
	chief := &mockChiefAPI{importResp: domain.SubmitResponse{PipelineID: "2026-09-03-101010_abc_import"}}
	pipelines := &mockPipelineStore{}

	resp, err := newImportUsecase(t, chief, pipelines).SubmitImport(context.Background(), domain.ImportParams{
		SourceURL:    "https://kartolo.sby.datautama.net.id/debian/",
		Dist:         "sid",
		PackageNames: []string{"grub-efi-amd64-bin"},
	})
	require.NoError(t, err)

	assert.Equal(t, "2026-09-03-101010_abc_import", resp.PipelineID)
	assert.Equal(t, "main", chief.importSubmitted.Component)
	assert.Equal(t, "main", chief.importSubmitted.SourceComponent)
	assert.Equal(t, "sid", chief.importSubmitted.Dist)
	// The dashboard shows who triggered the import.
	assert.Equal(t, "Herpiko Dwi Aguno <herpiko@gmail.com>", chief.importSubmitted.Maintainer)
	// The ID is remembered so `irgsh-cli import status` works with no argument.
	assert.Equal(t, "2026-09-03-101010_abc_import", pipelines.importID)
}

func TestImportStatus_UsesLastPipelineID(t *testing.T) {
	chief := &mockChiefAPI{importStatus: domain.ImportStatus{JobStatus: "DONE", ImportStatus: "SUCCESS"}}
	pipelines := &mockPipelineStore{importID: "2026-09-03-101010_abc_import"}

	status, err := newImportUsecase(t, chief, pipelines).ImportStatus(context.Background(), "")
	require.NoError(t, err)
	assert.Equal(t, "DONE", status.JobStatus)
}

func TestImportStatus_WithoutAnyPipelineID(t *testing.T) {
	_, err := newImportUsecase(t, &mockChiefAPI{}, &mockPipelineStore{}).ImportStatus(context.Background(), "")
	assert.ErrorIs(t, err, usecase.ErrPipelineIDMissing)
}

func TestSubmitImport_SigningKeyIdentityUnavailable(t *testing.T) {
	usecaseWithBrokenGPG := usecase.NewCLIUsecase(
		&mockConfigStore{config: domain.Config{ChiefAddress: "https://irgsh.example.id"}},
		&mockPipelineStore{}, &mockChiefAPI{}, nil, nil, nil,
		&mockGPGSigner{err: errors.New("no secret key")},
		nil, nil, nil, "1.0.0",
	)

	_, err := usecaseWithBrokenGPG.SubmitImport(context.Background(), domain.ImportParams{
		SourceURL:    "https://kartolo.sby.datautama.net.id/debian/",
		Dist:         "sid",
		PackageNames: []string{"firefox"},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "signing key")
}

func TestSubmitImport_PassesCheckFlagsThrough(t *testing.T) {
	chief := &mockChiefAPI{importResp: domain.SubmitResponse{PipelineID: "id"}}

	_, err := newImportUsecase(t, chief, &mockPipelineStore{}).SubmitImport(context.Background(), domain.ImportParams{
		SourceURL:          "https://kartolo.sby.datautama.net.id/debian/",
		Dist:               "sid",
		PackageNames:       []string{"firefox"},
		DryRun:             true,
		IgnoreDependencies: true,
		Insecure:           true,
		ForceVersion:       true,
	})
	require.NoError(t, err)

	assert.True(t, chief.importSubmitted.DryRun)
	assert.True(t, chief.importSubmitted.IgnoreDependencies)
	assert.True(t, chief.importSubmitted.Insecure)
	assert.True(t, chief.importSubmitted.ForceVersion)
}

func TestSubmitImport_DefaultsAreConservative(t *testing.T) {
	chief := &mockChiefAPI{importResp: domain.SubmitResponse{PipelineID: "id"}}

	_, err := newImportUsecase(t, chief, &mockPipelineStore{}).SubmitImport(context.Background(), domain.ImportParams{
		SourceURL:    "https://kartolo.sby.datautama.net.id/debian/",
		Dist:         "sid",
		PackageNames: []string{"firefox"},
	})
	require.NoError(t, err)

	// An import checks dependencies and injects unless told otherwise.
	assert.False(t, chief.importSubmitted.DryRun)
	assert.False(t, chief.importSubmitted.IgnoreDependencies)
	assert.False(t, chief.importSubmitted.Insecure)
}

// A machine without apt cannot check, and that must not block a submission.
func TestSubmitImport_LocalCheckUnavailable(t *testing.T) {
	chief := &mockChiefAPI{importResp: domain.SubmitResponse{PipelineID: "id"}}

	_, err := newImportUsecaseWithShell(t, chief, &mockPipelineStore{},
		&mockShellRunner{err: errors.New("command not found")},
	).SubmitImport(context.Background(), domain.ImportParams{
		SourceURL:    "https://kartolo.sby.datautama.net.id/debian/",
		Dist:         "sid",
		PackageNames: []string{"firefox"},
	})

	require.NoError(t, err, "an unavailable check must not block the import")
	assert.Equal(t, "firefox", chief.importSubmitted.PackageNames[0])
}

// When the check runs and the packages are not installable, the submission is
// refused with apt's own report.
func TestSubmitImport_LocalCheckFails(t *testing.T) {
	unmet := `The following packages have unmet dependencies:
 firefox : Depends: libc6 (>= 2.43) but 2.41-12+deb13u3 is to be installed
           Depends: libvpx12 (>= 1.16.0) but it is not installable`

	// Every command succeeds except the simulation, which reports the unmet
	// dependencies and exits non-zero.
	shell := &scriptedShell{
		outputs: map[string]shellResult{
			"--simulate": {out: unmet, err: errors.New("exit status 100")},
		},
	}
	chief := &mockChiefAPI{importResp: domain.SubmitResponse{PipelineID: "id"}}

	_, err := newImportUsecaseWithShell(t, chief, &mockPipelineStore{}, shell).
		SubmitImport(context.Background(), domain.ImportParams{
			SourceURL:    "https://kartolo.sby.datautama.net.id/debian/",
			Dist:         "sid",
			PackageNames: []string{"firefox"},
		})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "libc6 (>= 2.43)")
	assert.Contains(t, err.Error(), "--ignore-dependencies")
	assert.Empty(t, chief.importSubmitted.PackageNames, "a failing check must not submit anything")
}

// --ignore-dependencies submits anyway.
func TestSubmitImport_LocalCheckOverridden(t *testing.T) {
	shell := &scriptedShell{
		outputs: map[string]shellResult{
			"--simulate": {out: "unmet dependencies", err: errors.New("exit status 100")},
		},
	}
	chief := &mockChiefAPI{importResp: domain.SubmitResponse{PipelineID: "id"}}

	_, err := newImportUsecaseWithShell(t, chief, &mockPipelineStore{}, shell).
		SubmitImport(context.Background(), domain.ImportParams{
			SourceURL:          "https://kartolo.sby.datautama.net.id/debian/",
			Dist:               "sid",
			PackageNames:       []string{"firefox"},
			IgnoreDependencies: true,
		})

	require.NoError(t, err)
	assert.Equal(t, "firefox", chief.importSubmitted.PackageNames[0])
}

// --skip-check does not run the check at all.
func TestSubmitImport_SkipCheck(t *testing.T) {
	shell := &scriptedShell{
		outputs: map[string]shellResult{
			"--simulate": {out: "unmet dependencies", err: errors.New("exit status 100")},
		},
	}
	chief := &mockChiefAPI{importResp: domain.SubmitResponse{PipelineID: "id"}}

	_, err := newImportUsecaseWithShell(t, chief, &mockPipelineStore{}, shell).
		SubmitImport(context.Background(), domain.ImportParams{
			SourceURL:    "https://kartolo.sby.datautama.net.id/debian/",
			Dist:         "sid",
			PackageNames: []string{"firefox"},
			SkipCheck:    true,
		})

	require.NoError(t, err)
	assert.False(t, shell.simulated, "--skip-check must not run the simulation")
}

// shellResult is one canned command result.
type shellResult struct {
	out string
	err error
}

// scriptedShell answers by substring: any command containing a key returns
// that result, everything else succeeds.
type scriptedShell struct {
	outputs   map[string]shellResult
	simulated bool
	// sourcesListContent and preferencesContent snapshot the sandbox files
	// while the check still has them on disk: checkImportLocally removes its
	// directory (defer os.RemoveAll) before returning, so a test cannot read
	// them back afterward.
	sourcesListContent string
	preferencesContent string
}

// sourcesList returns the sources.list the check wrote.
func (s *scriptedShell) sourcesList(t *testing.T) string {
	t.Helper()
	if s.sourcesListContent == "" {
		t.Fatal("the check never built a sandbox")
	}
	return s.sourcesListContent
}

// preferences returns the apt pinning the check wrote.
func (s *scriptedShell) preferences(t *testing.T) string {
	t.Helper()
	if s.preferencesContent == "" {
		t.Fatal("the check never built a sandbox")
	}
	return s.preferencesContent
}

// snapshotSandbox reads the sandbox files an apt command line points at,
// while the check still has them on disk (the check removes its directory
// before checkImportLocally returns).
func (s *scriptedShell) snapshotSandbox(cmd string) {
	root := sandboxRoot(cmd)
	if root == "" {
		return
	}
	if content, err := os.ReadFile(filepath.Join(root, "sources.list")); err == nil {
		s.sourcesListContent = string(content)
	}
	if content, err := os.ReadFile(filepath.Join(root, "preferences")); err == nil {
		s.preferencesContent = string(content)
	}
}

// sandboxRoot extracts the sandbox directory from an apt command line.
func sandboxRoot(cmd string) string {
	const marker = "-o Dir::State='"
	i := strings.Index(cmd, marker)
	if i < 0 {
		return ""
	}
	rest := cmd[i+len(marker):]
	end := strings.Index(rest, "'")
	if end < 0 {
		return ""
	}
	return filepath.Dir(rest[:end])
}

func (s *scriptedShell) result(cmd string) shellResult {
	s.snapshotSandbox(cmd)
	if strings.Contains(cmd, "--simulate") {
		s.simulated = true
	}
	for key, result := range s.outputs {
		if strings.Contains(cmd, key) {
			return result
		}
	}
	return shellResult{}
}

func (s *scriptedShell) Output(cmd string) (string, error) {
	r := s.result(cmd)
	return r.out, r.err
}

func (s *scriptedShell) Run(cmd string) error            { return s.result(cmd).err }
func (s *scriptedShell) RunInteractive(cmd string) error { return s.result(cmd).err }

// The target is whatever chief publishes to, not whatever this machine
// happens to have in its sources.list.
func TestSubmitImport_ChecksAgainstTheRepositoryChiefPublishesTo(t *testing.T) {
	shell := &scriptedShell{}
	chief := &mockChiefAPI{
		importResp: domain.SubmitResponse{PipelineID: "id"},
		repoInfo: domain.RepoInfo{
			PublicURL:      "http://arsip-dev.blankonlinux.id/dev",
			DistCodename:   "verbeek",
			DistComponents: "main restricted extras",
		},
	}

	_, err := newImportUsecaseWithShell(t, chief, &mockPipelineStore{}, shell).
		SubmitImport(context.Background(), domain.ImportParams{
			SourceURL:    "https://kartolo.sby.datautama.net.id/debian/",
			Dist:         "sid",
			PackageNames: []string{"firefox"},
		})
	require.NoError(t, err)

	if !shell.simulated {
		t.Fatal("the check must have run")
	}
	sources := shell.sourcesList(t)
	assert.Contains(t, sources, "http://arsip-dev.blankonlinux.id/dev verbeek main restricted extras",
		"the target must be the repository chief publishes to")
	assert.Contains(t, sources, "https://kartolo.sby.datautama.net.id/debian/ sid main",
		"the source repository must be added so the candidate can be found")

	// The source suite is pinned out of dependency resolution, otherwise it
	// would satisfy the dependencies it is supposed to be checked against.
	preferences := shell.preferences(t)
	assert.Contains(t, preferences, "Pin: release n=sid")
	assert.Contains(t, preferences, "Pin-Priority: -1")
	assert.Contains(t, preferences, "Package: firefox")
}

// An older chief has no repo-info endpoint; the check falls back rather than
// blocking the import.
func TestSubmitImport_RepoInfoUnavailable(t *testing.T) {
	shell := &scriptedShell{}
	chief := &mockChiefAPI{
		importResp:  domain.SubmitResponse{PipelineID: "id"},
		repoInfoErr: errors.New("HTTP 404"),
	}

	_, err := newImportUsecaseWithShell(t, chief, &mockPipelineStore{}, shell).
		SubmitImport(context.Background(), domain.ImportParams{
			SourceURL:    "https://kartolo.sby.datautama.net.id/debian/",
			Dist:         "sid",
			PackageNames: []string{"firefox"},
		})

	require.NoError(t, err, "an unreachable repo-info must not block the import")
	assert.Equal(t, "firefox", chief.importSubmitted.PackageNames[0])
}
