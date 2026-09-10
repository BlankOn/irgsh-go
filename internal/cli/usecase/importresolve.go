package usecase

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// maxResolveRounds bounds the dependency walk. Each round can only add source
// packages that are missing from the target repository entirely, so the set
// converges quickly; the cap is there so a pathological archive cannot make
// irgsh-cli spin.
const maxResolveRounds = 8

// importSet is the set of source packages an import will bring in, and the
// binaries each of them produces.
type importSet struct {
	order   []string
	sources map[string]*sourcePackage
}

// sourcePackage is one source package and what importing it costs.
type sourcePackage struct {
	Name     string
	Version  string
	Binaries []string
	// Requested marks a source package the maintainer named directly, as
	// opposed to one pulled in to satisfy a dependency.
	Requested bool
	// Bytes is the total download size of its binaries, 0 when unknown.
	Bytes int64
}

func newImportSet() *importSet {
	return &importSet{sources: map[string]*sourcePackage{}}
}

func (s *importSet) has(source string) bool {
	_, ok := s.sources[source]
	return ok
}

func (s *importSet) add(pkg *sourcePackage) {
	if s.has(pkg.Name) {
		return
	}
	s.sources[pkg.Name] = pkg
	s.order = append(s.order, pkg.Name)
}

// binaries is every binary package the import will inject, which is what the
// sandbox has to allow through the pin.
func (s *importSet) binaries() []string {
	var all []string
	for _, name := range s.order {
		all = append(all, s.sources[name].Binaries...)
	}
	sort.Strings(all)
	return all
}

// representatives is the package list to submit: what the maintainer named,
// plus one binary from every source package the resolver added.
//
// Naming one binary of a source is enough - the repo worker resolves it back
// to its source and imports every binary of it - and it keeps the submission
// readable instead of listing dozens of siblings.
func (s *importSet) representatives(requested []string) []string {
	names := append([]string{}, requested...)
	for _, pkg := range s.pulled() {
		if len(pkg.Binaries) > 0 {
			names = append(names, pkg.Binaries[0])
		}
	}
	return names
}

// pulled lists the source packages that were added to satisfy a dependency
// rather than being asked for.
func (s *importSet) pulled() []*sourcePackage {
	var extra []*sourcePackage
	for _, name := range s.order {
		if !s.sources[name].Requested {
			extra = append(extra, s.sources[name])
		}
	}
	return extra
}

// blocker is a dependency that cannot be fixed by importing more packages.
type blocker struct {
	// Package is the dependency that could not be satisfied, e.g. "libc6".
	Package string
	// Needed is the constraint that failed, e.g. ">= 2.38".
	Needed string
	// TargetVersion is what the target repository already carries.
	TargetVersion string
	// Wanted is the package whose dependency this is.
	Wanted string
}

// importPlan is the outcome of resolving an import.
type importPlan struct {
	Set      *importSet
	Blockers []blocker
	// Output is apt's own report for whatever is still uninstallable.
	Output string
}

// Resolvable reports whether the plan can go ahead.
func (p *importPlan) Resolvable() bool { return len(p.Blockers) == 0 && p.Output == "" }

// resolveImport works out everything that has to be imported for the named
// packages to be installable on top of the target repository.
//
// It starts from the source packages the named binaries were built from -
// every binary of those is imported anyway, so all of them may be resolved
// against - and then widens the set one round at a time: whatever apt still
// reports as missing is looked up, and a dependency the target repository does
// not have at all becomes another source package to import. A dependency the
// target does have, only at the wrong version, is a blocker instead: importing
// a newer libc6 out of the source suite replaces the C library of the
// distribution, which is never what a maintainer means by "import strongswan".
func (u *CLIUsecase) resolveImport(sandbox *importSandbox, targets *targetView, names []string) (*importPlan, error) {
	set := newImportSet()
	for _, name := range names {
		pkg, err := sandbox.sourceOf(name)
		if err != nil {
			return nil, err
		}
		pkg.Requested = true
		set.add(pkg)
	}

	plan := &importPlan{Set: set}
	for round := 0; round < maxResolveRounds; round++ {
		if err := sandbox.allow(set.binaries()); err != nil {
			return nil, err
		}

		var failures []string
		var missing []unmetDependency
		for _, name := range names {
			out, ok := sandbox.installable(name)
			if ok {
				continue
			}
			failures = append(failures, out)
			missing = append(missing, parseUnmetDependencies(out)...)
		}
		if len(failures) == 0 {
			plan.Blockers = nil
			plan.Output = ""
			return plan, nil
		}

		plan.Output = strings.Join(failures, "\n")
		plan.Blockers = nil

		var added bool
		for _, dep := range missing {
			if set.hasBinary(dep.Package) {
				continue
			}
			if version, present := targets.version(dep.Package); present {
				plan.Blockers = append(plan.Blockers, blocker{
					Package:       dep.Package,
					Needed:        dep.Constraint,
					TargetVersion: version,
					Wanted:        dep.Wanted,
				})
				continue
			}
			pkg, err := sandbox.sourceOf(dep.Package)
			if err != nil {
				// Not in the source repository either: nothing to import.
				plan.Blockers = append(plan.Blockers, blocker{
					Package: dep.Package,
					Needed:  dep.Constraint,
					Wanted:  dep.Wanted,
				})
				continue
			}
			if set.has(pkg.Name) {
				continue
			}
			set.add(pkg)
			added = true
		}

		if !added {
			return plan, nil
		}
	}

	return plan, nil
}

// hasBinary reports whether a binary package is already part of the import.
func (s *importSet) hasBinary(name string) bool {
	for _, source := range s.sources {
		for _, binary := range source.Binaries {
			if binary == name {
				return true
			}
		}
	}
	return false
}

// sourceOf resolves a binary package to the source package it was built from,
// along with every binary that source produces.
func (s *importSandbox) sourceOf(binary string) (*sourcePackage, error) {
	out, err := s.u.shell.Output(fmt.Sprintf(
		"apt-cache %s showsrc %s | grep -m1 '^Package:' | cut -d' ' -f2", s.opts(), sq(binary)))
	name := strings.TrimSpace(lastLine(out))
	if err != nil || name == "" {
		return nil, fmt.Errorf("no source package found for %q in %s %s",
			binary, s.params.SourceURL, s.params.SourceDist)
	}

	pkg := &sourcePackage{Name: name}

	if version, verErr := s.u.shell.Output(fmt.Sprintf(
		"apt-cache %s showsrc %s | grep -m1 '^Version:' | cut -d' ' -f2", s.opts(), sq(name))); verErr == nil {
		pkg.Version = strings.TrimSpace(lastLine(version))
	}

	binaries, err := s.u.shell.Output(fmt.Sprintf(
		"apt-cache %s showsrc %s | grep -m1 '^Binary:' | cut -d' ' -f2- | tr -d ' ' | tr ',' '\\n'",
		s.opts(), sq(name)))
	if err != nil {
		return nil, fmt.Errorf("failed to list the binary packages of %s: %w", name, err)
	}
	for _, line := range strings.Split(binaries, "\n") {
		if trimmed := strings.TrimSpace(line); trimmed != "" {
			pkg.Binaries = append(pkg.Binaries, trimmed)
		}
	}
	if len(pkg.Binaries) == 0 {
		pkg.Binaries = []string{binary}
	}
	sort.Strings(pkg.Binaries)

	pkg.Bytes = s.sizeOf(pkg.Binaries)
	return pkg, nil
}

// sizeOf totals the download size of a set of binary packages. A size apt
// cannot report is simply left out: the number is for the maintainer's
// judgement, not for accounting.
func (s *importSandbox) sizeOf(binaries []string) int64 {
	if len(binaries) == 0 {
		return 0
	}
	out, err := s.u.shell.Output(fmt.Sprintf("apt-cache %s --no-all-versions show %s | grep '^Size:' | cut -d' ' -f2",
		s.opts(), strings.Join(quoteAll(binaries), " ")))
	if err != nil {
		return 0
	}

	var total int64
	for _, line := range strings.Split(out, "\n") {
		if size, convErr := strconv.ParseInt(strings.TrimSpace(line), 10, 64); convErr == nil {
			total += size
		}
	}
	return total
}

// targetView answers what the target repository already carries, with no
// sight of the source repository at all.
type targetView struct {
	u    *CLIUsecase
	root string
}

func (u *CLIUsecase) newTargetView(params ImportCheckParams) (*targetView, error) {
	root, err := os.MkdirTemp("", "irgsh-import-target")
	if err != nil {
		return nil, fmt.Errorf("failed to create the target directory: %w", err)
	}

	view := &targetView{u: u, root: root}
	for _, dir := range []string{
		filepath.Join(root, "state", "lists", "partial"),
		filepath.Join(root, "cache", "archives", "partial"),
		filepath.Join(root, "preferences.d"),
		filepath.Join(root, "sources.list.d"),
	} {
		if mkErr := os.MkdirAll(dir, 0755); mkErr != nil {
			view.close()
			return nil, fmt.Errorf("failed to create %s: %w", dir, mkErr)
		}
	}
	if writeErr := os.WriteFile(filepath.Join(root, "state", "status"), nil, 0644); writeErr != nil {
		view.close()
		return nil, fmt.Errorf("failed to create the apt status file: %w", writeErr)
	}
	if writeErr := os.WriteFile(filepath.Join(root, "sources.list"),
		[]byte(strings.Join(params.TargetSources, "\n")+"\n"), 0644); writeErr != nil {
		view.close()
		return nil, fmt.Errorf("failed to write the target sources list: %w", writeErr)
	}
	if writeErr := os.WriteFile(filepath.Join(root, "preferences"), nil, 0644); writeErr != nil {
		view.close()
		return nil, fmt.Errorf("failed to write the target preferences: %w", writeErr)
	}
	if runErr := u.shell.Run(fmt.Sprintf("apt-get %s update", sandboxAptOpts(root))); runErr != nil {
		view.close()
		return nil, fmt.Errorf("failed to read the target package indices: %w", runErr)
	}
	return view, nil
}

func (t *targetView) close() {
	if t != nil && t.root != "" {
		os.RemoveAll(t.root)
	}
}

// version reports the version of a package in the target repository, and
// whether it is there at all.
func (t *targetView) version(pkg string) (string, bool) {
	out, err := t.u.shell.Output(fmt.Sprintf(
		"apt-cache %s --no-all-versions show %s | grep -m1 '^Version:' | cut -d' ' -f2",
		sandboxAptOpts(t.root), sq(pkg)))
	if err != nil {
		return "", false
	}
	version := strings.TrimSpace(lastLine(out))
	if version == "" {
		return "", false
	}
	return version, true
}

// unmetDependency is one line of apt's unmet dependency report.
type unmetDependency struct {
	// Wanted is the package that needs it.
	Wanted string
	// Package is the dependency itself.
	Package string
	// Constraint is the version requirement, e.g. ">= 2.38", if any.
	Constraint string
}

var (
	// " firefox : Depends: libc6 (>= 2.43) but ..." - the package whose
	// dependencies could not be met.
	wantedLine = regexp.MustCompile(`^\s*(\S+)\s*:\s*(?:Pre)?Depends:`)
	// The dependency itself, on that line or on a continuation of it.
	dependency = regexp.MustCompile(`(?:Pre)?Depends:\s+(\S+)(?:\s+\(([^)]*)\))?`)
)

// parseUnmetDependencies reads the package names out of apt's unmet
// dependency report.
//
// Conflicts are deliberately ignored: two packages that conflict are not
// missing anything, they simply cannot be installed at the same time, which a
// repository never asks of them.
func parseUnmetDependencies(output string) []unmetDependency {
	var deps []unmetDependency
	var wanted string

	for _, line := range strings.Split(output, "\n") {
		if match := wantedLine.FindStringSubmatch(line); match != nil {
			wanted = match[1]
		}
		match := dependency.FindStringSubmatch(line)
		if match == nil {
			continue
		}
		// "but it is not installable" and "but X is to be installed" both
		// mean the dependency is unmet; apt only prints the line at all when
		// it is.
		deps = append(deps, unmetDependency{
			Wanted:     wanted,
			Package:    match[1],
			Constraint: strings.TrimSpace(match[2]),
		})
	}
	return deps
}

// lastLine returns the final non-empty line of a command's output, which is
// where a piped grep/cut leaves its answer.
func lastLine(out string) string {
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		if trimmed := strings.TrimSpace(lines[i]); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

// formatBytes renders a download size the way a maintainer reads one.
func formatBytes(b int64) string {
	switch {
	case b <= 0:
		return "?"
	case b < 1000:
		return fmt.Sprintf("%d B", b)
	case b < 1000*1000:
		return fmt.Sprintf("%.0f kB", float64(b)/1000)
	case b < 1000*1000*1000:
		return fmt.Sprintf("%.1f MB", float64(b)/(1000*1000))
	default:
		return fmt.Sprintf("%.1f GB", float64(b)/(1000*1000*1000))
	}
}

// renderPulledReport describes the extra source packages an import would drag
// in, so the maintainer can judge the size of what they are about to do.
func renderPulledReport(requested []string, pulled []*sourcePackage) string {
	var b strings.Builder
	fmt.Fprintf(&b, "\n%s needs %s to be imported as well:\n\n",
		strings.Join(requested, ", "), pluralize(len(pulled), "more source package"))

	fmt.Fprintf(&b, "  %-28s %-18s %8s  %10s\n", "SOURCE", "VERSION", "BINARIES", "SIZE")
	var binaries int
	var bytes int64
	for _, pkg := range pulled {
		fmt.Fprintf(&b, "  %-28s %-18s %8d  %10s\n",
			pkg.Name, pkg.Version, len(pkg.Binaries), formatBytes(pkg.Bytes))
		binaries += len(pkg.Binaries)
		bytes += pkg.Bytes
	}
	fmt.Fprintf(&b, "\n  Total: %s, %s, %s\n",
		pluralize(len(pulled), "source package"), pluralize(binaries, "binary package"), formatBytes(bytes))
	return b.String()
}

// renderBlockers describes dependencies that importing more packages cannot
// fix, which is a reason to stop rather than a list to confirm.
func renderBlockers(blockers []blocker) string {
	var b strings.Builder
	b.WriteString("\nThese dependencies cannot be satisfied by importing:\n\n")
	for _, blk := range blockers {
		name := blk.Package
		if blk.Needed != "" {
			name += " (" + blk.Needed + ")"
		}
		switch {
		case blk.TargetVersion != "":
			fmt.Fprintf(&b, "  %s\n      needed by %s; the target repository has %s.\n"+
				"      Importing it would replace a package the distribution is built on.\n",
				name, blk.Wanted, blk.TargetVersion)
		default:
			fmt.Fprintf(&b, "  %s\n      needed by %s, and it is in neither repository.\n", name, blk.Wanted)
		}
	}
	return b.String()
}

func pluralize(n int, noun string) string {
	if n == 1 {
		return fmt.Sprintf("%d %s", n, noun)
	}
	return fmt.Sprintf("%d %ss", n, noun)
}
