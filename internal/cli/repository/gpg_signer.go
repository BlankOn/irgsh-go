package repository

import (
	"fmt"
	"os/exec"
	"strings"
)

// ShellGPGSigner implements usecase.GPGSigner using gpg commands.
type ShellGPGSigner struct{}

func NewShellGPGSigner() *ShellGPGSigner {
	return &ShellGPGSigner{}
}

func (g *ShellGPGSigner) GetIdentity(fingerprint string) (string, error) {
	out, err := exec.Command("gpg", "-K", "--with-colons", fingerprint).Output()
	if err != nil {
		return "", fmt.Errorf("failed to get maintainer identity: %w", err)
	}
	// Parse uid line from colons output
	for _, line := range strings.Split(string(out), "\n") {
		fields := strings.Split(line, ":")
		if len(fields) > 9 && fields[0] == "uid" {
			return strings.TrimSpace(fields[9]), nil
		}
	}
	return "", fmt.Errorf("no uid found for fingerprint %s", fingerprint)
}

func (g *ShellGPGSigner) ClearSign(inputPath, outputPath, fingerprint string) error {
	cmd := exec.Command("gpg", "-u", fingerprint, "--clearsign", "--output", outputPath, "--sign", inputPath)
	cmd.Stdin = nil
	return cmd.Run()
}
