package usecase_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/blankon/irgsh-go/internal/cli/domain"
	"github.com/blankon/irgsh-go/internal/cli/usecase"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// publishedTarget is a chief that names the repository the packages go into,
// which is what the check resolves against.
func publishedTarget() domain.RepoInfo {
	return domain.RepoInfo{
		PublicURL:      "http://arsip-dev.blankonlinux.id/sinambung",
		DistCodename:   "sinambung",
		DistComponents: "main restricted extras",
	}
}

// strongswanSource is the shape that produced the false negative this check
// used to report: one source package, many binaries, two of which conflict.
func strongswanSource() map[string]scriptedSource {
	return map[string]scriptedSource{
		"strongswan": {
			version: "6.1.0-2",
			binaries: []string{
				"charon-systemd", "libstrongswan", "strongswan",
				"strongswan-charon", "strongswan-libcharon",
				"strongswan-starter", "strongswan-swanctl",
			},
			sizes: map[string]int64{"libstrongswan": 1_200_000, "strongswan": 400_000},
		},
	}
}

func newResolvingUsecase(t *testing.T, chief *mockChiefAPI, shell usecase.ShellRunner, prompter usecase.Prompter) *usecase.CLIUsecase {
	t.Helper()
	return usecase.NewCLIUsecase(
		&mockConfigStore{config: domain.Config{
			ChiefAddress:         "https://irgsh.example.id",
			MaintainerSigningKey: "54495BCCA444849BD55A84ED5115CB575CE255A8",
		}},
		&mockPipelineStore{}, chief, shell, nil, nil,
		&mockGPGSigner{identity: "Herpiko Dwi Aguno <herpiko@gmail.com>"},
		nil, nil, prompter, "1.0.0",
	)
}

func strongswanParams() domain.ImportParams {
	return domain.ImportParams{
		SourceURL:    "https://kartolo.sby.datautama.net.id/debian/",
		SourceDist:   "sid",
		Dist:         "sinambung",
		PackageNames: []string{"strongswan", "strongswan-charon", "charon-systemd"},
	}
}

// The binaries of a source package are all imported together, so they must be
// resolvable against each other. Pinning only the named packages made apt
// report libstrongswan as "not going to be installed" when the import was
// always going to bring it along.
func TestImportCheck_SiblingBinariesAreResolvable(t *testing.T) {
	shell := &scriptedShell{srcPackages: strongswanSource()}
	chief := &mockChiefAPI{importResp: domain.SubmitResponse{PipelineID: "id"}, repoInfo: publishedTarget()}

	_, err := newResolvingUsecase(t, chief, shell, &mockPrompter{}).
		SubmitImport(context.Background(), strongswanParams())
	require.NoError(t, err)

	preferences := shell.preferences(t)
	for _, binary := range []string{"libstrongswan", "strongswan-swanctl", "strongswan-starter", "strongswan-libcharon"} {
		assert.Contains(t, preferences, binary,
			"every binary of the source package must be resolvable, not just the named ones")
	}
}

// strongswan-charon and charon-systemd declare Conflicts on each other. Asking
// apt to install them together can never succeed, and a repository never asks
// that of them: each is tested on its own.
func TestImportCheck_TestsEachPackageSeparately(t *testing.T) {
	shell := &scriptedShell{srcPackages: strongswanSource()}
	chief := &mockChiefAPI{importResp: domain.SubmitResponse{PipelineID: "id"}, repoInfo: publishedTarget()}

	_, err := newResolvingUsecase(t, chief, shell, &mockPrompter{}).
		SubmitImport(context.Background(), strongswanParams())
	require.NoError(t, err)

	assert.ElementsMatch(t, []string{"strongswan", "strongswan-charon", "charon-systemd"},
		shell.simulatedPackages, "each named package is resolved on its own")
}

// A dependency the target repository does not have at all is another package
// to import. The maintainer is shown what it costs and asked before anything
// is queued.
func TestImportCheck_PullsInMissingDependenciesAfterConfirmation(t *testing.T) {
	unmet := `The following packages have unmet dependencies:
 strongswan : Depends: libnl-3-200 (>= 3.7.0) but it is not going to be installed`

	shell := &scriptedShell{
		srcPackages: map[string]scriptedSource{
			"strongswan": {version: "6.1.0-2", binaries: []string{"strongswan"}, sizes: map[string]int64{"strongswan": 400_000}},
			"libnl3": {
				version:  "3.7.0-1",
				binaries: []string{"libnl-3-200", "libnl-3-dev"},
				sizes:    map[string]int64{"libnl-3-200": 412_000, "libnl-3-dev": 100_000},
			},
		},
		// Fails until libnl3 joins the set, then resolves.
		simulateSeq: map[string][]shellResult{
			"strongswan": {{out: unmet, err: errors.New("exit status 100")}},
		},
		targetPackages: map[string]string{},
	}
	chief := &mockChiefAPI{importResp: domain.SubmitResponse{PipelineID: "id"}, repoInfo: publishedTarget()}
	prompter := &mockPrompter{confirmed: true}

	params := strongswanParams()
	params.PackageNames = []string{"strongswan"}

	_, err := newResolvingUsecase(t, chief, shell, prompter).SubmitImport(context.Background(), params)
	require.NoError(t, err)

	// The extra source package has to be named in the submission, or the repo
	// worker would import only strongswan and inject a set that does not
	// resolve.
	assert.Contains(t, chief.importSubmitted.PackageNames, "strongswan")
	assert.Contains(t, chief.importSubmitted.PackageNames, "libnl-3-200")
}

// Saying no leaves the repository untouched.
func TestImportCheck_DeclinedExtraPackagesCancelTheImport(t *testing.T) {
	unmet := `The following packages have unmet dependencies:
 strongswan : Depends: libnl-3-200 (>= 3.7.0) but it is not going to be installed`

	shell := &scriptedShell{
		srcPackages: map[string]scriptedSource{
			"strongswan": {version: "6.1.0-2", binaries: []string{"strongswan"}},
			"libnl3":     {version: "3.7.0-1", binaries: []string{"libnl-3-200"}},
		},
		simulateSeq: map[string][]shellResult{
			"strongswan": {{out: unmet, err: errors.New("exit status 100")}},
		},
	}
	chief := &mockChiefAPI{importResp: domain.SubmitResponse{PipelineID: "id"}, repoInfo: publishedTarget()}

	params := strongswanParams()
	params.PackageNames = []string{"strongswan"}

	_, err := newResolvingUsecase(t, chief, shell, &mockPrompter{confirmed: false}).
		SubmitImport(context.Background(), params)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "cancelled")
	assert.Empty(t, chief.importSubmitted.PackageNames, "a declined import must not be queued")
}

// --yes is for a non-interactive run: the extra packages are accepted without
// a prompt.
func TestImportCheck_AssumeYesSkipsThePrompt(t *testing.T) {
	unmet := `The following packages have unmet dependencies:
 strongswan : Depends: libnl-3-200 (>= 3.7.0) but it is not going to be installed`

	shell := &scriptedShell{
		srcPackages: map[string]scriptedSource{
			"strongswan": {version: "6.1.0-2", binaries: []string{"strongswan"}},
			"libnl3":     {version: "3.7.0-1", binaries: []string{"libnl-3-200"}},
		},
		simulateSeq: map[string][]shellResult{
			"strongswan": {{out: unmet, err: errors.New("exit status 100")}},
		},
	}
	chief := &mockChiefAPI{importResp: domain.SubmitResponse{PipelineID: "id"}, repoInfo: publishedTarget()}

	params := strongswanParams()
	params.PackageNames = []string{"strongswan"}
	params.AssumeYes = true

	// A prompter that would refuse, to prove it is never consulted.
	_, err := newResolvingUsecase(t, chief, shell, &mockPrompter{confirmed: false}).
		SubmitImport(context.Background(), params)

	require.NoError(t, err)
	assert.Contains(t, chief.importSubmitted.PackageNames, "libnl-3-200")
}

// A dependency the target already has, only at an older version, cannot be
// fixed by importing: pulling libc6 out of sid replaces the C library the
// distribution is built on. That is a reason to stop, not a list to confirm.
func TestImportCheck_VersionConflictIsABlockerNotAnImport(t *testing.T) {
	unmet := `The following packages have unmet dependencies:
 strongswan : Depends: libc6 (>= 2.38) but 2.36-9 is to be installed`

	shell := &scriptedShell{
		srcPackages: map[string]scriptedSource{
			"strongswan": {version: "6.1.0-2", binaries: []string{"strongswan"}},
			"glibc":      {version: "2.41-1", binaries: []string{"libc6"}},
		},
		simulateSeq: map[string][]shellResult{
			"strongswan": {{out: unmet, err: errors.New("exit status 100")}},
		},
		targetPackages: map[string]string{"libc6": "2.36-9"},
	}
	chief := &mockChiefAPI{importResp: domain.SubmitResponse{PipelineID: "id"}, repoInfo: publishedTarget()}

	params := strongswanParams()
	params.PackageNames = []string{"strongswan"}

	// The prompter would accept, to prove a blocker is never offered as a choice.
	_, err := newResolvingUsecase(t, chief, shell, &mockPrompter{confirmed: true}).
		SubmitImport(context.Background(), params)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "libc6")
	assert.Contains(t, err.Error(), "2.36-9", "the maintainer needs to see what the target already has")
	assert.Empty(t, chief.importSubmitted.PackageNames)
}

// Without a published target there is nothing meaningful to resolve against.
// The maintainer's own machine is not it, and a wrong answer there would block
// a perfectly good import.
func TestImportCheck_SkippedWhenChiefNamesNoRepository(t *testing.T) {
	shell := &scriptedShell{}
	chief := &mockChiefAPI{importResp: domain.SubmitResponse{PipelineID: "id"}}

	params := strongswanParams()
	params.PackageNames = []string{"strongswan"}

	_, err := newResolvingUsecase(t, chief, shell, &mockPrompter{}).
		SubmitImport(context.Background(), params)

	require.NoError(t, err, "a missing repo_url must not block the import")
	assert.False(t, shell.simulated, "there is nothing to check against")
	assert.Equal(t, []string{"strongswan"}, chief.importSubmitted.PackageNames)
}

// The sandbox must not read the maintainer's own repositories: the machine
// running irgsh-cli does not have to be running the distribution being
// imported into.
func TestImportCheck_DoesNotReadTheMachinesOwnSources(t *testing.T) {
	shell := &scriptedShell{srcPackages: strongswanSource()}
	chief := &mockChiefAPI{importResp: domain.SubmitResponse{PipelineID: "id"}, repoInfo: publishedTarget()}

	_, err := newResolvingUsecase(t, chief, shell, &mockPrompter{}).
		SubmitImport(context.Background(), strongswanParams())
	require.NoError(t, err)

	sources := shell.sourcesList(t)
	assert.Contains(t, sources, "http://arsip-dev.blankonlinux.id/sinambung sinambung main restricted extras")
	assert.Contains(t, sources, "deb-src https://kartolo.sby.datautama.net.id/debian/ sid main",
		"deb-src is what lets a binary be traced back to its source package")
	for _, line := range strings.Split(sources, "\n") {
		assert.NotContains(t, line, "/etc/apt", "the sandbox must be self-contained")
	}
}
