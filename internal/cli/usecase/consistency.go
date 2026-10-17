package usecase

import (
	"bufio"
	"fmt"
	"net/url"
	"os"
	"path"
	"strings"
)

// checkChangelogSourceName rejects a package whose debian/changelog names a
// different source package than debian/control. dpkg-source stops with
// "source package has two conflicting values" in that case, but only after the
// repository was cloned and the workdir was prepared.
func checkChangelogSourceName(changelogPath, controlName string) error {
	f, err := os.Open(changelogPath)
	if err != nil {
		return fmt.Errorf("failed to open changelog: %w", err)
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	if !scanner.Scan() {
		if err := scanner.Err(); err != nil {
			return fmt.Errorf("failed to read changelog: %w", err)
		}
		return fmt.Errorf("changelog is empty: %s", changelogPath)
	}

	line := scanner.Text()
	end := strings.Index(line, "(")
	if end < 0 {
		return nil
	}
	changelogName := strings.TrimSpace(line[:end])
	if changelogName == controlName {
		return nil
	}
	return fmt.Errorf("package name mismatch: debian/control has Source: %q but the first entry of debian/changelog is for %q",
		controlName, changelogName)
}

// checkTarballVersion rejects a downloaded source tarball whose file name does
// not carry the upstream version from debian/changelog, which usually means the
// changelog was bumped without pointing --source at the new release.
//
// Repacked tarballs often append a suffix to the upstream version in the
// changelog (1.2.3+dfsg) that the file name omits, so the version up to the
// first "+" is accepted too.
func checkTarballVersion(sourceURL, upstreamVersion string) error {
	name := path.Base(sourceURL)
	if u, err := url.Parse(sourceURL); err == nil {
		name = path.Base(u.Path)
	}
	if strings.Contains(name, upstreamVersion) {
		return nil
	}
	if i := strings.Index(upstreamVersion, "+"); i > 0 && strings.Contains(name, upstreamVersion[:i]) {
		return nil
	}
	return fmt.Errorf("version mismatch: debian/changelog has upstream version %q but the source tarball is %q",
		upstreamVersion, name)
}
