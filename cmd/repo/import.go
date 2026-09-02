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

	for _, source := range sources {
		if err = apt.fetchSource(logPath, source); err != nil {
			return fail("Failed to fetch source package "+source, err)
		}
		binaries, binErr := apt.binariesOf(logPath, source)
		if binErr != nil {
			return fail("Failed to list the binaries of "+source, binErr)
		}
		if err = apt.fetchBinaries(logPath, binaries); err != nil {
			return fail("Failed to fetch binaries of "+source, err)
		}
	}

	if err = injectImportedFiles(logPath, workdir, submission); err != nil {
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
	opts := []string{
		"-o Dir::Etc::sourcelist=" + sq(filepath.Join(a.root, "sources.list")),
		"-o Dir::Etc::sourceparts=/dev/null",
		"-o Dir::State=" + sq(filepath.Join(a.root, "state")),
		"-o Dir::State::status=" + sq(filepath.Join(a.root, "state", "status")),
		"-o Dir::Cache=" + sq(filepath.Join(a.root, "cache")),
		// Verify against the keyrings the worker already trusts, which is
		// what makes importing from a Debian mirror work out of the box.
		"-o Dir::Etc::trustedparts=/etc/apt/trusted.gpg.d",
		"-o APT::Get::List-Cleanup=false",
		"-o Acquire::Languages=none",
		// apt drops to the _apt user for downloads, which cannot read the
		// job's work directory; without this every fetch warns about running
		// unsandboxed.
		"-o APT::Sandbox::User=root",
	}
	if a.submission.Insecure {
		// Explicitly requested by the maintainer with --insecure.
		opts = append(opts,
			"-o Acquire::AllowInsecureRepositories=true",
			"-o Acquire::AllowDowngradeToInsecureRepositories=true",
			"-o APT::Get::AllowUnauthenticated=true",
		)
	}
	return strings.Join(opts, " ")
}

func (a *aptSandbox) prepare(logPath string) error {
	for _, dir := range []string{
		filepath.Join(a.root, "state", "lists", "partial"),
		filepath.Join(a.root, "cache", "archives", "partial"),
		a.downloads,
	} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("failed to create %s: %w", dir, err)
		}
	}
	if err := os.WriteFile(filepath.Join(a.root, "state", "status"), nil, 0644); err != nil {
		return fmt.Errorf("failed to create the apt status file: %w", err)
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

	_, err := systemutil.CmdExec(
		fmt.Sprintf("apt-get %s update", a.aptOpts()),
		"Fetching the package indices of the source repository",
		logPath,
	)
	return err
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
func injectImportedFiles(logPath, workdir string, submission importSubmission) error {
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

	for _, dsc := range dscFiles {
		cmdStr := fmt.Sprintf("mkdir -p %s && cd %s/ && %s reprepro -v -v -v %s --component %s includedsc %s %s",
			sq(distDir), sq(distDir), gnupgDir, ignoreDistribution, submission.Component, dist, sq(dsc))
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

// removeExistingVersions drops the source packages being imported, and their
// binaries, before injecting the new ones.
func removeExistingVersions(logPath, dist, distDir, gnupgDir string, dscFiles []string) error {
	for _, dsc := range dscFiles {
		source := strings.SplitN(filepath.Base(dsc), "_", 2)[0]
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
