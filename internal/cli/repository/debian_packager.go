package repository

import (
	"fmt"
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
	cmd := fmt.Sprintf("cat %s | grep 'Source:' | head -n 1 | cut -d ' ' -f 2", controlPath)
	out, err := d.shell.Output(cmd)
	if err != nil {
		return "", fmt.Errorf("failed to get package name: %w", err)
	}
	name := strings.TrimSpace(out)
	if name == "" {
		return "", fmt.Errorf("no Source: field found in %s", controlPath)
	}
	return name, nil
}

func (d *ShellDebianPackager) ExtractVersion(changelogPath string) (string, error) {
	cmd := fmt.Sprintf("cat %s | head -n 1 | cut -d '(' -f 2 | cut -d ')' -f 1 | cut -d '-' -f 1", changelogPath)
	out, err := d.shell.Output(cmd)
	if err != nil {
		return "", fmt.Errorf("failed to get package version: %w", err)
	}
	version := strings.TrimSpace(out)
	if strings.Contains(version, ":") {
		version = strings.Split(version, ":")[1]
	}
	return version, nil
}

func (d *ShellDebianPackager) ExtractExtendedVersion(changelogPath string) (string, error) {
	cmd := fmt.Sprintf("cat %s | head -n 1 | cut -d '(' -f 2 | cut -d ')' -f 1 | cut -d '-' -f 2", changelogPath)
	out, err := d.shell.Output(cmd)
	if err != nil {
		return "", fmt.Errorf("failed to get package extended version: %w", err)
	}
	return strings.TrimSpace(out), nil
}

func (d *ShellDebianPackager) ExtractChangelogMaintainer(changelogPath string) (string, error) {
	cmd := fmt.Sprintf("echo $(cat %s | grep ' --' | cut -d '-' -f 3 | cut -d '>' -f 1 | head -n 1)'>'", changelogPath)
	out, err := d.shell.Output(cmd)
	if err != nil {
		return "", fmt.Errorf("failed to get changelog maintainer: %w", err)
	}
	return strings.TrimSpace(out), nil
}

func (d *ShellDebianPackager) ExtractUploaders(controlPath string) (string, error) {
	cmd := fmt.Sprintf("cat %s | grep 'Uploaders:' | head -n 1 | cut -d ':' -f 2", controlPath)
	out, err := d.shell.Output(cmd)
	if err != nil {
		return "", fmt.Errorf("failed to get uploaders: %w", err)
	}
	return strings.TrimSpace(out), nil
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
