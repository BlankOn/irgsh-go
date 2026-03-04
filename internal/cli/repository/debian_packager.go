package repository

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/blankon/irgsh-go/internal/cli/usecase"
)

// ShellDebianPackager implements usecase.DebianPackager using shell commands.
type ShellDebianPackager struct {
	shell usecase.ShellRunner
}

func NewShellDebianPackager(shell usecase.ShellRunner) *ShellDebianPackager {
	return &ShellDebianPackager{shell: shell}
}

func (d *ShellDebianPackager) ExtractPackageName(controlPath string) (string, error) {
	f, err := os.Open(controlPath)
	if err != nil {
		return "", fmt.Errorf("failed to open control file: %w", err)
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "Source:") {
			name := strings.TrimSpace(strings.TrimPrefix(line, "Source:"))
			if name == "" {
				return "", fmt.Errorf("no Source: value found in %s", controlPath)
			}
			return name, nil
		}
	}
	if err := scanner.Err(); err != nil {
		return "", fmt.Errorf("failed to read control file: %w", err)
	}
	return "", fmt.Errorf("no Source: field found in %s", controlPath)
}

func (d *ShellDebianPackager) ExtractVersion(changelogPath string) (string, error) {
	firstLine, err := readFirstLine(changelogPath)
	if err != nil {
		return "", fmt.Errorf("failed to get package version: %w", err)
	}
	version := parseChangelogVersion(firstLine)
	// Strip epoch (e.g. "1:2.0" -> "2.0")
	if strings.Contains(version, ":") {
		version = strings.SplitN(version, ":", 2)[1]
	}
	// Strip revision (e.g. "2.0-1" -> "2.0")
	if strings.Contains(version, "-") {
		version = strings.SplitN(version, "-", 2)[0]
	}
	return version, nil
}

func (d *ShellDebianPackager) ExtractExtendedVersion(changelogPath string) (string, error) {
	firstLine, err := readFirstLine(changelogPath)
	if err != nil {
		return "", fmt.Errorf("failed to get package extended version: %w", err)
	}
	version := parseChangelogVersion(firstLine)
	// Strip epoch
	if strings.Contains(version, ":") {
		version = strings.SplitN(version, ":", 2)[1]
	}
	// Extract revision part (after first '-')
	if strings.Contains(version, "-") {
		return strings.SplitN(version, "-", 2)[1], nil
	}
	// No revision part - return the version itself
	return version, nil
}

func (d *ShellDebianPackager) ExtractChangelogMaintainer(changelogPath string) (string, error) {
	f, err := os.Open(changelogPath)
	if err != nil {
		return "", fmt.Errorf("failed to open changelog: %w", err)
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, " -- ") {
			// Format: " -- Name <email>  date"
			maintainer := strings.TrimPrefix(line, " -- ")
			// Trim everything after and including the double space before the date
			if idx := strings.Index(maintainer, "  "); idx > 0 {
				maintainer = maintainer[:idx]
			}
			return strings.TrimSpace(maintainer), nil
		}
	}
	if err := scanner.Err(); err != nil {
		return "", fmt.Errorf("failed to read changelog: %w", err)
	}
	return "", fmt.Errorf("no maintainer found in %s", changelogPath)
}

func (d *ShellDebianPackager) ExtractUploaders(controlPath string) (string, error) {
	f, err := os.Open(controlPath)
	if err != nil {
		return "", fmt.Errorf("failed to open control file: %w", err)
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "Uploaders:") {
			value := strings.TrimSpace(strings.TrimPrefix(line, "Uploaders:"))
			return value, nil
		}
	}
	if err := scanner.Err(); err != nil {
		return "", fmt.Errorf("failed to read control file: %w", err)
	}
	return "", nil
}

func (d *ShellDebianPackager) BuildSource(dir string) error {
	cmd := fmt.Sprintf("cd %s && dpkg-source --build .", dir)
	return d.shell.RunInteractive(cmd)
}

func (d *ShellDebianPackager) Sign(dir, keyFingerprint string) error {
	cmd := fmt.Sprintf("cd %s && debsign -k%s *.dsc", dir, keyFingerprint)
	return d.shell.RunInteractive(cmd)
}

func (d *ShellDebianPackager) GenBuildInfo(dir string) error {
	cmd := fmt.Sprintf("cd %s && dpkg-genbuildinfo", dir)
	return d.shell.RunInteractive(cmd)
}

func (d *ShellDebianPackager) GenChanges(dir string) (string, error) {
	cmd := fmt.Sprintf("cd %s && dpkg-genchanges", dir)
	return d.shell.Output(cmd)
}

func (d *ShellDebianPackager) Lintian(changesPath string) (string, error) {
	cmd := fmt.Sprintf("lintian --profile blankon %s 2>&1", changesPath)
	return d.shell.Output(cmd)
}

// readFirstLine reads the first line from a file.
func readFirstLine(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	if scanner.Scan() {
		return scanner.Text(), nil
	}
	if err := scanner.Err(); err != nil {
		return "", err
	}
	return "", fmt.Errorf("file is empty: %s", path)
}

// parseChangelogVersion extracts the version string from a debian/changelog first line.
// Format: "package (version) distribution; urgency=level"
func parseChangelogVersion(line string) string {
	start := strings.Index(line, "(")
	end := strings.Index(line, ")")
	if start >= 0 && end > start {
		return line[start+1 : end]
	}
	return ""
}
