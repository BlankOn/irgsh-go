package usecase_test

import (
	"context"
	"errors"
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
	return usecase.NewCLIUsecase(
		&mockConfigStore{config: domain.Config{
			ChiefAddress:         "https://irgsh.example.id",
			MaintainerSigningKey: "54495BCCA444849BD55A84ED5115CB575CE255A8",
		}},
		pipelines, chief, nil, nil, nil,
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
