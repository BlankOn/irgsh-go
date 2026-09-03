package usecase

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/blankon/irgsh-go/internal/cli/domain"
)

var (
	// errCheckUnavailable reports that this machine cannot run the check.
	errCheckUnavailable = errors.New("apt is not available on this machine")
	// errNoSystemSources reports that this machine has no apt sources to
	// check against.
	errNoSystemSources = errors.New("this machine has no apt sources configured")
	// errNoTarget reports that no target repository could be determined.
	errNoTarget = errors.New("the target repository could not be determined")
)

// checkImportLocally resolves the packages about to be imported against the
// repositories this machine is configured with, before anything is submitted.
//
// The maintainer's machine already runs the distribution the packages are
// going into, so its own apt configuration is the target: if the packages
// cannot be installed here, they cannot be installed by anyone else either.
//
// The source repository is added as an extra apt source, pinned so that only
// the named packages may come from it. Without that pin apt would happily
// satisfy their dependencies from the same suite, which is exactly the
// mistake this check exists to catch.
func (u *CLIUsecase) checkImportLocally(params ImportCheckParams) error {
	if len(params.TargetSources) == 0 {
		return errNoTarget
	}

	if u.shell == nil || !u.shellHas("apt-get") || !u.shellHas("apt-cache") {
		return errCheckUnavailable
	}

	root, err := os.MkdirTemp("", "irgsh-import-check")
	if err != nil {
		return fmt.Errorf("failed to create the check directory: %w", err)
	}
	defer os.RemoveAll(root)

	if err := u.writeCheckSandbox(root, params); err != nil {
		return err
	}

	opts := localAptOpts(root)
	if err := u.shell.Run(fmt.Sprintf("apt-get %s update", opts)); err != nil {
		return fmt.Errorf("failed to read the package indices: %w", err)
	}

	// --simulate resolves without touching this machine.
	out, err := u.shell.Output(fmt.Sprintf("apt-get %s --simulate --no-install-recommends install %s",
		opts, strings.Join(quoteAll(params.PackageNames), " ")))
	if err != nil {
		return &ImportDependencyError{Output: out}
	}
	return nil
}

// ImportCheckParams is what the local check needs to know.
type ImportCheckParams struct {
	SourceURL       string
	Dist            string
	SourceComponent string
	PackageNames    []string
	// TargetSources are the sources.list entries of the repository the
	// packages are going into.
	TargetSources []string
}

// ImportDependencyError reports packages that cannot be installed on top of
// the repositories this machine uses.
type ImportDependencyError struct {
	Output string
}

func (e *ImportDependencyError) Error() string {
	return "the packages are not installable on top of the target repository:\n" + strings.TrimSpace(e.Output)
}

// writeCheckSandbox lays out an apt root that sees this machine's own sources
// plus the source repository, with the latter pinned out of dependency
// resolution.
func (u *CLIUsecase) writeCheckSandbox(root string, params ImportCheckParams) error {
	for _, dir := range []string{
		filepath.Join(root, "state", "lists", "partial"),
		filepath.Join(root, "cache", "archives", "partial"),
		filepath.Join(root, "preferences.d"),
	} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("failed to create %s: %w", dir, err)
		}
	}
	if err := os.WriteFile(filepath.Join(root, "state", "status"), nil, 0644); err != nil {
		return fmt.Errorf("failed to create the apt status file: %w", err)
	}

	sources := append([]string{}, params.TargetSources...)
	sources = append(sources, fmt.Sprintf("deb %s %s %s", params.SourceURL, params.Dist, params.SourceComponent))
	if err := os.WriteFile(filepath.Join(root, "sources.list"), []byte(strings.Join(sources, "\n")+"\n"), 0644); err != nil {
		return fmt.Errorf("failed to write the sources list: %w", err)
	}

	// Pin by suite name rather than by host: a mirror commonly serves both
	// the source suite and this machine's own, so the host does not identify
	// which is which.
	preferences := fmt.Sprintf(`Package: *
Pin: release n=%s
Pin-Priority: -1

Package: %s
Pin: release n=%s
Pin-Priority: 990
`, params.Dist, strings.Join(params.PackageNames, " "), params.Dist)

	return os.WriteFile(filepath.Join(root, "preferences"), []byte(preferences), 0644)
}

// targetSources returns the sources.list entries to check against, and a
// description of what they are, so the maintainer can see which repository
// the answer applies to.
//
// The repository chief publishes to is the real target. Falling back to this
// machine's own sources only guesses that the maintainer runs the same
// distribution, which is why chief is asked first.
func targetSources(info domain.RepoInfo) ([]string, string, error) {
	if info.PublicURL != "" && info.DistCodename != "" {
		components := info.DistComponents
		if components == "" {
			components = "main"
		}
		// The indices are only read to resolve dependencies, never installed
		// from, so an unavailable signing key must not block the check.
		entry := fmt.Sprintf("deb [trusted=yes] %s %s %s", info.PublicURL, info.DistCodename, components)
		return []string{entry}, fmt.Sprintf("%s %s (%s)", info.PublicURL, info.DistCodename, components), nil
	}

	sources, err := systemSources()
	if err != nil {
		return nil, "", err
	}
	return sources, "this machine's own apt sources (chief does not publish a repo_url)", nil
}

// systemSources collects this machine's apt sources, which describe the
// distribution the imported packages have to fit into.
func systemSources() ([]string, error) {
	var sources []string

	read := func(path string) {
		content, err := os.ReadFile(path)
		if err != nil {
			return
		}
		for _, line := range strings.Split(string(content), "\n") {
			trimmed := strings.TrimSpace(line)
			// deb822 (.sources) files are not sources.list syntax; they are
			// picked up separately below.
			if strings.HasPrefix(trimmed, "deb ") || strings.HasPrefix(trimmed, "deb-src ") {
				sources = append(sources, trimmed)
			}
		}
	}

	read("/etc/apt/sources.list")
	entries, err := os.ReadDir("/etc/apt/sources.list.d")
	if err == nil {
		for _, entry := range entries {
			if strings.HasSuffix(entry.Name(), ".list") {
				read(filepath.Join("/etc/apt/sources.list.d", entry.Name()))
			}
		}
	}

	if len(sources) == 0 {
		return nil, errNoSystemSources
	}
	return sources, nil
}

// localAptOpts isolates apt in the check directory, while still reading the
// system's trusted keyrings and deb822 sources.
func localAptOpts(root string) string {
	opts := []string{
		"-o Dir::Etc::sourcelist=" + sq(filepath.Join(root, "sources.list")),
		"-o Dir::Etc::sourceparts=/etc/apt/sources.list.d",
		"-o Dir::Etc::preferences=" + sq(filepath.Join(root, "preferences")),
		"-o Dir::Etc::preferencesparts=" + sq(filepath.Join(root, "preferences.d")),
		"-o Dir::State=" + sq(filepath.Join(root, "state")),
		"-o Dir::State::status=" + sq(filepath.Join(root, "state", "status")),
		"-o Dir::Cache=" + sq(filepath.Join(root, "cache")),
		"-o Dir::Etc::trustedparts=/etc/apt/trusted.gpg.d",
		"-o APT::Get::List-Cleanup=false",
		"-o Acquire::Languages=none",
	}
	return strings.Join(opts, " ")
}

func quoteAll(values []string) []string {
	quoted := make([]string, 0, len(values))
	for _, value := range values {
		quoted = append(quoted, sq(value))
	}
	return quoted
}

// shellHas reports whether a command is available on this machine.
func (u *CLIUsecase) shellHas(command string) bool {
	return u.shell.Run("command -v "+sq(command)+" >/dev/null 2>&1") == nil
}
