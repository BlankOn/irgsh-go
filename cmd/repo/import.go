package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/blankon/irgsh-go/internal/logstream"
	"github.com/blankon/irgsh-go/internal/notification"
	"github.com/blankon/irgsh-go/pkg/systemutil"
)

// importSubmission mirrors internal/chief/domain.ImportSubmission. Unlike the
// build and repo tasks, which read their payload out of a map, an import job
// has enough structure to be worth unmarshalling properly.
type importSubmission struct {
	TaskUUID        string   `json:"taskUUID"`
	SourceURL       string   `json:"sourceUrl"`
	Dist            string   `json:"dist"`
	SourceComponent string   `json:"sourceComponent"`
	PackageNames    []string `json:"packageNames"`
	Component       string   `json:"component"`
	IsExperimental  bool     `json:"isExperimental"`
	ForceVersion    bool     `json:"forceVersion"`
	Insecure        bool     `json:"insecure"`
	// KeyringPath is an optional keyring on the worker to verify the source
	// repository against, for a repo whose key is not installed system wide.
	KeyringPath string `json:"keyringPath"`
	// DryRun fetches and checks the packages without injecting them.
	DryRun bool `json:"dryRun"`
	// IgnoreDependencies imports even when the dependency check fails.
	IgnoreDependencies bool `json:"ignoreDependencies"`
}

func uploadImportLog(logPath string, id string) {
	if info, err := os.Stat(logPath); err != nil {
		fmt.Printf("error: log file %s is not uploadable: %v\n", logPath, err)
		return
	} else {
		fmt.Printf("Uploading log file %s (%d bytes) to %s\n", logPath, info.Size(), irgshConfig.Chief.Address)
	}
	cmdStr := "curl -v -F 'uploadFile=@" + logPath + "' '" + irgshConfig.Chief.Address +
		"/api/v1/log-upload?id=" + id + "&type=import'"
	if _, err := systemutil.CmdExec(cmdStr, "Uploading log file to chief", ""); err != nil {
		fmt.Printf("error: failed to upload log file %s: %v\n", logPath, err)
	}
}

// Import fetches already built packages from an external Debian repository and
// injects them into our repository.
//
// For every requested binary package it resolves the source package it was
// built from, then pulls that source package (.dsc plus its tarballs) and
// every binary built from it, so the repository ends up with the same
// self-consistent source and binary set a local build would have produced.
func Import(payload string) (err error) {
	var submission importSubmission
	if err = json.Unmarshal([]byte(payload), &submission); err != nil {
		return fmt.Errorf("invalid import payload: %w", err)
	}
	taskUUID := submission.TaskUUID

	jobInfo := notification.JobNotificationInfo{
		PackageName:    strings.Join(submission.PackageNames, " "),
		IsExperimental: submission.IsExperimental,
		SourceURL:      submission.SourceURL,
	}

	workdir := filepath.Join(irgshConfig.Repo.Workdir, "imports", taskUUID)
	logPath := filepath.Join(workdir, "import.log")
	if prepErr := systemutil.PrepareLogFile(logPath); prepErr != nil {
		return fmt.Errorf("unable to prepare log file %s: %w", logPath, prepErr)
	}
	stopLogStream := logstream.Mirror(logPublisher, taskUUID, "import", logPath)
	defer stopLogStream()

	defer func() {
		if err != nil {
			sendRepoNotification(taskUUID, "FAILED", jobInfo)
		} else {
			sendRepoNotification(taskUUID, "SUCCESS", jobInfo)
		}
	}()

	fail := func(stage string, cause error) error {
		systemutil.WriteLog(logPath, "[ IMPORT FAILED ] "+stage+": "+systemutil.FailureSummary(cause))
		uploadImportLog(logPath, taskUUID)
		return cause
	}

	systemutil.WriteLog(logPath, fmt.Sprintf(
		"##### Importing %s\n##### from %s (%s/%s) into %s/%s",
		strings.Join(submission.PackageNames, ", "),
		submission.SourceURL, submission.Dist, submission.SourceComponent,
		irgshConfig.Repo.DistCodename+experimentalSuffix(submission.IsExperimental), submission.Component))

	apt := newAptSandbox(workdir, submission)
	if err = apt.prepare(logPath); err != nil {
		return fail("Failed to prepare the source repository", err)
	}

	sources, err := apt.resolveSourcePackages(logPath, submission.PackageNames)
	if err != nil {
		return fail("Failed to resolve source packages", err)
	}
	systemutil.WriteLog(logPath, "##### Source packages to import: "+strings.Join(sources, ", "))

	metadata := map[string]sourceMeta{}
	for _, source := range sources {
		if err = apt.fetchSource(logPath, source); err != nil {
			return fail("Failed to fetch source package "+source, err)
		}
		metadata[source] = apt.sourceMetadata(logPath, source)
		binaries, binErr := apt.binariesOf(logPath, source)
		if binErr != nil {
			return fail("Failed to list the binaries of "+source, binErr)
		}
		if err = apt.fetchBinaries(logPath, binaries); err != nil {
			return fail("Failed to fetch binaries of "+source, err)
		}
	}

	debFiles, err := filepath.Glob(filepath.Join(apt.downloads, "*.deb"))
	if err != nil {
		return fail("Failed to list the downloaded packages", err)
	}
	if len(debFiles) == 0 {
		return fail("Nothing was downloaded", fmt.Errorf("no binary packages were downloaded from %s", submission.SourceURL))
	}

	// Check the packages against the repository they are going into before
	// they go into it.
	if depErr := checkDependencies(logPath, workdir, submission, debFiles); depErr != nil {
		systemutil.WriteLog(logPath, "##### The imported packages cannot be installed on top of "+
			irgshConfig.Repo.DistCodename+experimentalSuffix(submission.IsExperimental)+
			" and its upstream distribution. See the unmet dependencies above.")
		if !submission.IgnoreDependencies {
			systemutil.WriteLog(logPath, "##### Import it anyway with: irgsh-cli import --ignore-dependencies ...")
			err = depErr
			return fail("The imported packages have unmet dependencies", depErr)
		}
		systemutil.WriteLog(logPath, "##### Importing anyway (--ignore-dependencies)")
	} else {
		systemutil.WriteLog(logPath, "##### Dependency check passed: the imported packages are installable")
	}

	if submission.DryRun {
		systemutil.WriteLog(logPath, "[ IMPORT DONE ] Dry run: nothing was injected into the repository")
		uploadImportLog(logPath, taskUUID)
		return nil
	}

	if err = injectImportedFiles(logPath, workdir, submission, metadata); err != nil {
		return fail("Failed to inject the imported packages", err)
	}

	systemutil.WriteLog(logPath, "[ IMPORT DONE ]")
	uploadImportLog(logPath, taskUUID)
	return nil
}

func experimentalSuffix(isExperimental bool) string {
	if isExperimental {
		return "-experimental"
	}
	return ""
}

// aptSandbox is a self-contained apt root pointing at one external
// repository, so importing never touches the worker's own apt configuration.
type aptSandbox struct {
	root       string
	downloads  string
	submission importSubmission
}

func newAptSandbox(workdir string, submission importSubmission) *aptSandbox {
	return &aptSandbox{
		root:       filepath.Join(workdir, "apt"),
		downloads:  filepath.Join(workdir, "files"),
		submission: submission,
	}
}

// aptOpts points every apt directory at the sandbox.
func (a *aptSandbox) aptOpts() string {
	return aptOptsFor(a.root, a.submission.Insecure)
}

// aptOptsFor points every apt directory at an isolated root.
func aptOptsFor(root string, insecure bool) string {
	opts := []string{
		"-o Dir::Etc::sourcelist=" + sq(filepath.Join(root, "sources.list")),
		"-o Dir::Etc::sourceparts=/dev/null",
		"-o Dir::State=" + sq(filepath.Join(root, "state")),
		"-o Dir::State::status=" + sq(filepath.Join(root, "state", "status")),
		"-o Dir::Cache=" + sq(filepath.Join(root, "cache")),
		// Verify against the keyrings collected in prepare(), which is what
		// makes importing from a Debian mirror work out of the box.
		"-o Dir::Etc::trustedparts=" + sq(filepath.Join(root, "trusted.gpg.d")),
		"-o APT::Get::List-Cleanup=false",
		"-o Acquire::Languages=none",
		// apt drops to the _apt user for downloads, which cannot read the
		// job's work directory; without this every fetch warns about running
		// unsandboxed.
		"-o APT::Sandbox::User=root",
	}
	if insecure {
		// Explicitly requested by the maintainer with --insecure.
		opts = append(opts,
			"-o Acquire::AllowInsecureRepositories=true",
			"-o Acquire::AllowDowngradeToInsecureRepositories=true",
			"-o APT::Get::AllowUnauthenticated=true",
		)
	}
	return strings.Join(opts, " ")
}

// trustedParts is the sandbox directory holding every keyring apt may verify
// the source repository against.
func (a *aptSandbox) trustedParts() string {
	return filepath.Join(a.root, "trusted.gpg.d")
}

// systemKeyringDirs are the directories searched for installed keyrings.
//
// /etc/apt/trusted.gpg.d alone is not enough: on a derivative like BlankOn it
// holds the distribution's own key, while the Debian archive keys that
// debian-archive-keyring installs live in /usr/share/keyrings and are what
// verifying a Debian mirror needs.
var systemKeyringDirs = []string{"/etc/apt/trusted.gpg.d", "/usr/share/keyrings"}

// collectKeyrings links every keyring found in dirs, plus the optional extra
// keyring file, into dest and reports how many were linked.
func collectKeyrings(dirs []string, extra string, dest string) (int, error) {
	if err := os.MkdirAll(dest, 0755); err != nil {
		return 0, fmt.Errorf("failed to create %s: %w", dest, err)
	}

	link := func(tag, path string) bool {
		// apt only reads keyrings ending in .gpg or .asc.
		if ext := filepath.Ext(path); ext != ".gpg" && ext != ".asc" {
			return false
		}
		// Tag the link with its source directory so two directories holding a
		// keyring of the same name do not collide.
		target := filepath.Join(dest, tag+"-"+filepath.Base(path))
		if _, err := os.Lstat(target); err == nil {
			return false
		}
		return os.Symlink(path, target) == nil
	}

	var linked int
	for _, dir := range dirs {
		entries, err := os.ReadDir(dir)
		if err != nil {
			// A missing keyring directory is normal, not an error.
			continue
		}
		tag := strings.NewReplacer("/", "_", ".", "_").Replace(strings.Trim(dir, "/"))
		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}
			if link(tag, filepath.Join(dir, entry.Name())) {
				linked++
			}
		}
	}

	if extra != "" {
		if _, err := os.Stat(extra); err != nil {
			return linked, fmt.Errorf("keyring %s is not readable on this worker: %w", extra, err)
		}
		if !link("keyring", extra) {
			return linked, fmt.Errorf("keyring %s could not be used; it must end in .gpg or .asc", extra)
		}
		linked++
	}

	return linked, nil
}

// signatureHint turns apt's signature failure into something a maintainer can
// act on, rather than a bare exit status.
func signatureHint(output string) string {
	if !strings.Contains(output, "NO_PUBKEY") && !strings.Contains(output, "is not signed") {
		return ""
	}

	hint := "##### The source repository could not be verified with the keyrings installed on this worker.\n"
	if keys := missingKeyIDs(output); len(keys) > 0 {
		hint += "##### Missing public key(s): " + strings.Join(keys, ", ") + "\n"
	}
	hint += "##### Fix it in one of these ways:\n" +
		"#####   1. Install the source repository's keyring on the repo worker,\n" +
		"#####      for a Debian mirror: sudo apt-get install debian-archive-keyring\n" +
		"#####      (already installed? it may be too old for this suite: apt-get upgrade it)\n" +
		"#####   2. Point at a keyring explicitly: irgsh-cli import --keyring /usr/share/keyrings/debian-archive-keyring.gpg ...\n" +
		"#####   3. Import without verifying the source at all: irgsh-cli import --insecure ..."
	return hint
}

// missingKeyIDs extracts the key IDs apt reported as unavailable.
func missingKeyIDs(output string) []string {
	var keys []string
	seen := map[string]bool{}
	for _, field := range strings.Fields(output) {
		if !strings.HasPrefix(field, "NO_PUBKEY") {
			continue
		}
		key := strings.TrimPrefix(field, "NO_PUBKEY")
		if key == "" {
			continue
		}
		if !seen[key] {
			seen[key] = true
			keys = append(keys, key)
		}
	}

	// "NO_PUBKEY <id>" is two fields; pick up the ids that follow.
	fields := strings.Fields(output)
	for i, field := range fields {
		if field == "NO_PUBKEY" && i+1 < len(fields) {
			key := fields[i+1]
			if !seen[key] {
				seen[key] = true
				keys = append(keys, key)
			}
		}
	}
	return keys
}

// prepareAptRoot lays out an isolated apt root and collects the keyrings it
// verifies against.
func prepareAptRoot(root, keyringPath string, extraDirs ...string) (int, error) {
	dirs := append([]string{
		filepath.Join(root, "state", "lists", "partial"),
		filepath.Join(root, "cache", "archives", "partial"),
	}, extraDirs...)
	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return 0, fmt.Errorf("failed to create %s: %w", dir, err)
		}
	}
	if err := os.WriteFile(filepath.Join(root, "state", "status"), nil, 0644); err != nil {
		return 0, fmt.Errorf("failed to create the apt status file: %w", err)
	}
	return collectKeyrings(systemKeyringDirs, keyringPath, filepath.Join(root, "trusted.gpg.d"))
}

func (a *aptSandbox) prepare(logPath string) error {
	keyrings, err := prepareAptRoot(a.root, a.submission.KeyringPath, a.downloads)
	if err != nil {
		return err
	}
	systemutil.WriteLog(logPath, fmt.Sprintf("##### Verifying against %d keyring(s) installed on this worker", keyrings))
	if keyrings == 0 && !a.submission.Insecure {
		return fmt.Errorf("no keyrings are installed on this worker, so %s cannot be verified; "+
			"install the source repository's keyring, pass --keyring, or import with --insecure",
			a.submission.SourceURL)
	}

	trusted := ""
	if a.submission.Insecure {
		trusted = "[trusted=yes] "
	}
	sourcesList := fmt.Sprintf("deb %s%s %s %s\ndeb-src %s%s %s %s\n",
		trusted, a.submission.SourceURL, a.submission.Dist, a.submission.SourceComponent,
		trusted, a.submission.SourceURL, a.submission.Dist, a.submission.SourceComponent)
	if err := os.WriteFile(filepath.Join(a.root, "sources.list"), []byte(sourcesList), 0644); err != nil {
		return fmt.Errorf("failed to write the sources list: %w", err)
	}
	systemutil.WriteLog(logPath, "##### sources.list\n"+sourcesList)

	out, err := systemutil.CmdExec(
		fmt.Sprintf("apt-get %s update", a.aptOpts()),
		"Fetching the package indices of the source repository",
		logPath,
	)
	if err != nil {
		if hint := signatureHint(out); hint != "" {
			systemutil.WriteLog(logPath, hint)
		}
		return err
	}
	return nil
}

// resolveSourcePackages maps each requested binary package to the source
// package it was built from, keeping the result de-duplicated and ordered.
func (a *aptSandbox) resolveSourcePackages(logPath string, packages []string) ([]string, error) {
	seen := map[string]bool{}
	var sources []string

	for _, pkg := range packages {
		out, err := systemutil.CmdExec(
			fmt.Sprintf("apt-cache %s showsrc %s | grep -m1 '^Package:' | cut -d' ' -f2", a.aptOpts(), sq(pkg)),
			"Resolving the source package of "+pkg,
			logPath,
		)
		source := strings.TrimSpace(lastLine(out))
		if err != nil || source == "" {
			return nil, fmt.Errorf("no source package found for %q in %s %s: %w",
				pkg, a.submission.SourceURL, a.submission.Dist, err)
		}
		if !seen[source] {
			seen[source] = true
			sources = append(sources, source)
		}
	}

	sort.Strings(sources)
	return sources, nil
}

// sourceMeta is the section and priority reprepro needs to file a source
// package, which a .dsc does not have to carry.
type sourceMeta struct {
	section  string
	priority string
}

// sourceMetadata reads the section and priority of a source package from the
// repository index.
//
// Most .dsc files describe their binaries in Package-List but have no Section
// or Priority of their own, and reprepro then refuses the package with
// "No priority for '<source>', skipping." The index always has both.
func (a *aptSandbox) sourceMetadata(logPath, source string) sourceMeta {
	read := func(field string) string {
		out, err := systemutil.CmdExec(
			fmt.Sprintf("apt-cache %s showsrc %s | grep -m1 '^%s:' | cut -d' ' -f2", a.aptOpts(), sq(source), field),
			"Reading the "+strings.ToLower(field)+" of "+source,
			logPath,
		)
		if err != nil {
			return ""
		}
		return strings.TrimSpace(lastLine(out))
	}

	return normalizeSourceMeta(read("Section"), read("Priority"))
}

// normalizeSourceMeta fills in what the index does not give us.
func normalizeSourceMeta(section, priority string) sourceMeta {
	section = strings.TrimSpace(section)
	priority = strings.TrimSpace(priority)

	// Sources indices still carry the legacy "Priority: source", which is not
	// a package priority reprepro can file the package under.
	if priority == "" || priority == "source" {
		priority = "optional"
	}
	if section == "" {
		section = "misc"
	}
	return sourceMeta{section: section, priority: priority}
}

func (a *aptSandbox) fetchSource(logPath, source string) error {
	_, err := systemutil.CmdExec(
		fmt.Sprintf("cd %s && apt-get %s source --download-only %s", sq(a.downloads), a.aptOpts(), sq(source)),
		"Fetching the source package "+source,
		logPath,
	)
	return err
}

// binariesOf lists the binary packages built from a source package.
func (a *aptSandbox) binariesOf(logPath, source string) ([]string, error) {
	out, err := systemutil.CmdExec(
		fmt.Sprintf("apt-cache %s showsrc %s | grep -m1 '^Binary:' | cut -d' ' -f2- | tr -d ' ' | tr ',' '\\n'",
			a.aptOpts(), sq(source)),
		"Listing the binary packages built from "+source,
		logPath,
	)
	if err != nil {
		return nil, err
	}

	var binaries []string
	for _, line := range strings.Split(out, "\n") {
		name := strings.TrimSpace(line)
		if name != "" {
			binaries = append(binaries, name)
		}
	}
	if len(binaries) == 0 {
		return nil, fmt.Errorf("source package %s lists no binary packages", source)
	}
	return binaries, nil
}

// fetchBinaries downloads the binaries that exist for this architecture.
// A binary listed by the source package but not built for our architecture is
// reported and skipped rather than failing the import.
func (a *aptSandbox) fetchBinaries(logPath string, binaries []string) error {
	var downloaded int
	for _, binary := range binaries {
		_, err := systemutil.CmdExec(
			fmt.Sprintf("cd %s && apt-get %s download %s", sq(a.downloads), a.aptOpts(), sq(binary)),
			"Fetching the binary package "+binary,
			logPath,
		)
		if err != nil {
			systemutil.WriteLog(logPath, "##### SKIPPED: "+binary+
				" is not available for this architecture in the source repository")
			continue
		}
		downloaded++
	}
	if downloaded == 0 {
		return fmt.Errorf("none of the binary packages (%s) could be downloaded", strings.Join(binaries, ", "))
	}
	return nil
}

// injectImportedFiles hands the downloaded files to reprepro.
//
// reprepro is deliberately run without --nothingiserror: a package version
// that our repository already carries is reported and skipped, which is a
// successful no-op for an import rather than a failure.
func injectImportedFiles(logPath, workdir string, submission importSubmission, metadata map[string]sourceMeta) error {
	downloads := filepath.Join(workdir, "files")
	dist := irgshConfig.Repo.DistCodename + experimentalSuffix(submission.IsExperimental)
	distDir := filepath.Join(irgshConfig.Repo.Workdir, dist)

	gnupgDir := "GNUPGHOME=" + irgshConfig.Repo.GnupgDir
	if irgshConfig.IsDev {
		gnupgDir = ""
	}
	ignoreDistribution := ""
	if submission.IsExperimental {
		ignoreDistribution = "--ignore=wrongdistribution"
	}

	dscFiles, err := filepath.Glob(filepath.Join(downloads, "*.dsc"))
	if err != nil {
		return err
	}
	debFiles, err := filepath.Glob(filepath.Join(downloads, "*.deb"))
	if err != nil {
		return err
	}
	if len(dscFiles) == 0 && len(debFiles) == 0 {
		return fmt.Errorf("nothing was downloaded from the source repository")
	}
	systemutil.WriteLog(logPath, fmt.Sprintf("##### Downloaded %d source package(s) and %d binary package(s)",
		len(dscFiles), len(debFiles)))

	if submission.ForceVersion {
		if err := removeExistingVersions(logPath, dist, distDir, gnupgDir, dscFiles); err != nil {
			return err
		}
	}

	if len(dscFiles) > 0 {
		systemutil.WriteLog(logPath, "##### Note: reprepro cannot check the .dsc signatures of an imported package, "+
			"because the uploaders' keys are not in the repository keyring. The download itself was already "+
			"verified against the source repository's signed Release file.")
	}

	for _, dsc := range dscFiles {
		meta := metadata[sourceNameOf(dsc)]
		if meta.section == "" {
			meta.section = "misc"
		}
		if meta.priority == "" {
			meta.priority = "optional"
		}

		cmdStr := fmt.Sprintf("mkdir -p %s && cd %s/ && %s reprepro -v -v -v %s --component %s --section %s --priority %s includedsc %s %s",
			sq(distDir), sq(distDir), gnupgDir, ignoreDistribution, submission.Component,
			sq(meta.section), sq(meta.priority), dist, sq(dsc))
		if _, err := systemutil.CmdExec(cmdStr, "Injecting the source package "+filepath.Base(dsc), logPath); err != nil {
			return err
		}
	}

	for _, deb := range debFiles {
		cmdStr := fmt.Sprintf("mkdir -p %s && cd %s/ && %s reprepro -v -v -v %s --component %s includedeb %s %s",
			sq(distDir), sq(distDir), gnupgDir, ignoreDistribution, submission.Component, dist, sq(deb))
		if _, err := systemutil.CmdExec(cmdStr, "Injecting the binary package "+filepath.Base(deb), logPath); err != nil {
			return err
		}
	}

	cmdStr := fmt.Sprintf("mkdir -p %s && cd %s/ && %s reprepro -v -v -v export", sq(distDir), sq(distDir), gnupgDir)
	_, err = systemutil.CmdExec(cmdStr, "Re-export and publish the reprepro repository", logPath)
	return err
}

// sourceNameOf derives the source package name from a .dsc filename, which
// is always "<source>_<version>.dsc".
func sourceNameOf(dscPath string) string {
	return strings.SplitN(filepath.Base(dscPath), "_", 2)[0]
}

// removeExistingVersions drops the source packages being imported, and their
// binaries, before injecting the new ones.
func removeExistingVersions(logPath, dist, distDir, gnupgDir string, dscFiles []string) error {
	for _, dsc := range dscFiles {
		source := sourceNameOf(dsc)
		cmdStr := fmt.Sprintf("mkdir -p %s && cd %s/ && %s reprepro -v -v -v removesrc %s %s",
			sq(distDir), sq(distDir), gnupgDir, dist, sq(source))
		if _, err := systemutil.CmdExec(cmdStr,
			"Force version: removing the existing "+source+" before importing", logPath); err != nil {
			// The package may simply not be there yet.
			systemutil.WriteLog(logPath, "##### Nothing to remove for "+source)
		}
	}
	return nil
}

// lastLine returns the last non-empty line of a command output.
func lastLine(out string) string {
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		if strings.TrimSpace(lines[i]) != "" {
			return lines[i]
		}
	}
	return ""
}

// sq shell-quotes a string.
func sq(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\"'\"'") + "'"
}

// targetSandbox models the machine a user installs on: our own repository
// plus the upstream distribution it sits on top of.
//
// Importing from a newer suite is the whole point of an import job, and it is
// also how a repository ends up with a package nobody can install: sid's
// firefox wants a libc6 that our base does not have. Resolving the imported
// packages against that pair before injecting them catches it while the
// repository is still clean.
type targetSandbox struct {
	root       string
	submission importSubmission
}

func newTargetSandbox(workdir string, submission importSubmission) *targetSandbox {
	return &targetSandbox{
		root:       filepath.Join(workdir, "apt-target"),
		submission: submission,
	}
}

func (t *targetSandbox) aptOpts() string {
	return aptOptsFor(t.root, true)
}

// upstreamComponents turns the reprepro update syntax used in the repo config
// ("main non-free>restricted contrib>extras") into plain component names.
func upstreamComponents(configured string) string {
	var components []string
	for _, field := range strings.Fields(configured) {
		components = append(components, strings.SplitN(field, ">", 2)[0])
	}
	if len(components) == 0 {
		return "main"
	}
	return strings.Join(components, " ")
}

func (t *targetSandbox) prepare(logPath string) error {
	if _, err := prepareAptRoot(t.root, t.submission.KeyringPath); err != nil {
		return err
	}

	dist := irgshConfig.Repo.DistCodename + experimentalSuffix(t.submission.IsExperimental)
	ourRepo := filepath.Join(irgshConfig.Repo.Workdir, "www")

	var sources []string
	// Our own repository, if it has been exported at least once.
	if _, err := os.Stat(filepath.Join(ourRepo, "dists", dist, "Release")); err == nil {
		components := irgshConfig.Repo.DistComponents
		if components == "" {
			components = t.submission.Component
		}
		sources = append(sources, fmt.Sprintf("deb [trusted=yes] file://%s %s %s", ourRepo, dist, components))
	} else {
		systemutil.WriteLog(logPath, "##### Note: "+dist+" has not been exported yet, "+
			"so the dependency check only considers the upstream distribution")
	}

	// The distribution our repository is an overlay on, which is where a user
	// gets everything we do not carry ourselves.
	if irgshConfig.Repo.UpstreamDistUrl != "" && irgshConfig.Repo.UpstreamDistCodename != "" {
		sources = append(sources, fmt.Sprintf("deb %s %s %s",
			irgshConfig.Repo.UpstreamDistUrl,
			irgshConfig.Repo.UpstreamDistCodename,
			upstreamComponents(irgshConfig.Repo.UpstreamDistComponents)))
	}

	if len(sources) == 0 {
		return fmt.Errorf("neither an exported repository nor an upstream distribution is configured, " +
			"so the imported packages cannot be checked")
	}

	sourcesList := strings.Join(sources, "\n") + "\n"
	if err := os.WriteFile(filepath.Join(t.root, "sources.list"), []byte(sourcesList), 0644); err != nil {
		return fmt.Errorf("failed to write the target sources list: %w", err)
	}
	systemutil.WriteLog(logPath, "##### Checking against\n"+sourcesList)

	_, err := systemutil.CmdExec(
		fmt.Sprintf("apt-get %s update", t.aptOpts()),
		"Fetching the package indices of the target repository",
		logPath,
	)
	return err
}

// simulate asks apt to resolve the downloaded packages against the target,
// exactly as it would on a user's machine.
func (t *targetSandbox) simulate(logPath string, debFiles []string) error {
	var quoted []string
	for _, deb := range debFiles {
		quoted = append(quoted, sq(deb))
	}

	_, err := systemutil.CmdExec(
		fmt.Sprintf("apt-get %s --simulate --no-install-recommends install %s",
			t.aptOpts(), strings.Join(quoted, " ")),
		"Simulating the installation of the imported packages",
		logPath,
	)
	return err
}

// checkDependencies reports whether the imported packages are installable on
// a machine that has our repository and its upstream distribution.
func checkDependencies(logPath, workdir string, submission importSubmission, debFiles []string) error {
	target := newTargetSandbox(workdir, submission)
	if err := target.prepare(logPath); err != nil {
		return fmt.Errorf("could not prepare the dependency check: %w", err)
	}
	return target.simulate(logPath, debFiles)
}
