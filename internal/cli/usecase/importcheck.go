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
	// errNoTarget reports that the repository the packages are going into
	// could not be determined, so there is nothing meaningful to check
	// against.
	errNoTarget = errors.New("chief does not publish a repo_url for this distribution, " +
		"so the packages cannot be checked against the repository they are going into " +
		"(set repo.public_url on that repo worker)")
)

// ImportCheckParams is what the local check needs to know.
type ImportCheckParams struct {
	SourceURL       string
	SourceDist      string
	SourceComponent string
	// PackageNames are the packages the maintainer asked for. They are what
	// the check has to prove installable; the other binaries of their source
	// packages come along for the ride and only widen what may be resolved
	// from the source suite.
	PackageNames []string
	// TargetSources are the sources.list entries of the repository the
	// packages are going into.
	TargetSources []string
}

// ImportDependencyError reports packages that cannot be installed on top of
// the target repository.
type ImportDependencyError struct {
	Output string
}

func (e *ImportDependencyError) Error() string {
	return "the packages are not installable on top of the target repository:\n" + strings.TrimSpace(e.Output)
}

// importSandbox is an apt root that resolves packages the way a user's
// machine will once they have been imported: the target repository, plus the
// source repository pinned so that only the packages actually being imported
// may be resolved from it.
//
// Without that pin apt would satisfy every dependency from the source suite
// as well, which is exactly the mistake this check exists to catch.
type importSandbox struct {
	u      *CLIUsecase
	root   string
	params ImportCheckParams
}

// newImportSandbox builds the apt root and reads the indices into it.
func (u *CLIUsecase) newImportSandbox(params ImportCheckParams) (*importSandbox, error) {
	if len(params.TargetSources) == 0 {
		return nil, errNoTarget
	}
	if u.shell == nil || !u.shellHas("apt-get") || !u.shellHas("apt-cache") {
		return nil, errCheckUnavailable
	}

	root, err := os.MkdirTemp("", "irgsh-import-check")
	if err != nil {
		return nil, fmt.Errorf("failed to create the check directory: %w", err)
	}

	s := &importSandbox{u: u, root: root, params: params}
	if err := s.write(); err != nil {
		s.close()
		return nil, err
	}
	if err := u.shell.Run(fmt.Sprintf("apt-get %s update", s.opts())); err != nil {
		s.close()
		return nil, fmt.Errorf("failed to read the package indices: %w", err)
	}
	return s, nil
}

func (s *importSandbox) close() {
	if s != nil && s.root != "" {
		os.RemoveAll(s.root)
	}
}

func (s *importSandbox) write() error {
	for _, dir := range []string{
		filepath.Join(s.root, "state", "lists", "partial"),
		filepath.Join(s.root, "cache", "archives", "partial"),
		filepath.Join(s.root, "preferences.d"),
		// Empty on purpose: apt reads its sources from this directory too,
		// and the maintainer's own repositories are not the target.
		filepath.Join(s.root, "sources.list.d"),
	} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("failed to create %s: %w", dir, err)
		}
	}
	if err := os.WriteFile(filepath.Join(s.root, "state", "status"), nil, 0644); err != nil {
		return fmt.Errorf("failed to create the apt status file: %w", err)
	}

	sources := append([]string{}, s.params.TargetSources...)
	sources = append(sources,
		fmt.Sprintf("deb %s %s %s", s.params.SourceURL, s.params.SourceDist, s.params.SourceComponent),
		// deb-src is what lets a binary be traced back to its source package,
		// and so to its sibling binaries.
		fmt.Sprintf("deb-src %s %s %s", s.params.SourceURL, s.params.SourceDist, s.params.SourceComponent))
	if err := os.WriteFile(filepath.Join(s.root, "sources.list"),
		[]byte(strings.Join(sources, "\n")+"\n"), 0644); err != nil {
		return fmt.Errorf("failed to write the sources list: %w", err)
	}

	return s.allow(s.params.PackageNames)
}

// allow rewrites the pinning so that exactly these packages may be resolved
// from the source suite. Everything else there stays out of reach, so a
// dependency it cannot satisfy from the target repository is reported instead
// of being silently pulled in.
func (s *importSandbox) allow(packages []string) error {
	// Pin by suite name rather than by host: a mirror commonly serves both
	// the source suite and the target, so the host does not identify which.
	preferences := fmt.Sprintf(`Package: *
Pin: release n=%s
Pin-Priority: -1

Package: %s
Pin: release n=%s
Pin-Priority: 990
`, s.params.SourceDist, strings.Join(packages, " "), s.params.SourceDist)

	return os.WriteFile(filepath.Join(s.root, "preferences"), []byte(preferences), 0644)
}

func (s *importSandbox) opts() string {
	return sandboxAptOpts(s.root)
}

// installable reports whether one package resolves on top of the target
// repository, returning apt's own explanation when it does not.
//
// Packages are tested one at a time on purpose. Asking apt to install them
// together answers a different question - whether they are co-installable -
// which a repository never requires: strongswan-charon and charon-systemd
// declare Conflicts on each other and both still belong in the archive.
func (s *importSandbox) installable(pkg string) (string, bool) {
	out, err := s.u.shell.Output(fmt.Sprintf("apt-get %s --simulate --no-install-recommends install %s",
		s.opts(), sq(pkg)))
	return out, err == nil
}

// sandboxAptOpts isolates apt inside a check directory. Nothing outside it is
// read except the system's trusted keyrings, which only affect verification.
func sandboxAptOpts(root string) string {
	opts := []string{
		"-o Dir::Etc::sourcelist=" + sq(filepath.Join(root, "sources.list")),
		"-o Dir::Etc::sourceparts=" + sq(filepath.Join(root, "sources.list.d")),
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

// targetSources returns the sources.list entries to check against, and a
// description of what they are, so the maintainer can see which repository
// the answer applies to.
//
// Only the repository chief names is used. Falling back to this machine's own
// sources would answer a question nobody asked - the maintainer's machine does
// not have to be running the distribution being imported into - and a wrong
// answer here blocks a submission that is perfectly fine.
func targetSources(info domain.RepoInfo) ([]string, string, error) {
	if info.PublicURL == "" || info.DistCodename == "" {
		return nil, "", errNoTarget
	}

	components := info.DistComponents
	if components == "" {
		components = "main"
	}
	// The indices are only read to resolve dependencies, never installed
	// from, so an unavailable signing key must not block the check.
	entry := fmt.Sprintf("deb [trusted=yes] %s %s %s", info.PublicURL, info.DistCodename, components)
	return []string{entry}, fmt.Sprintf("%s %s (%s)", info.PublicURL, info.DistCodename, components), nil
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
