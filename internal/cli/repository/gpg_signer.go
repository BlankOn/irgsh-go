package repository

import (
	"fmt"
	"strings"

	"github.com/blankon/irgsh-go/internal/cli/usecase"
)

// ShellGPGSigner implements usecase.GPGSigner using gpg shell commands.
type ShellGPGSigner struct {
	shell usecase.ShellRunner
}

func NewShellGPGSigner(shell usecase.ShellRunner) *ShellGPGSigner {
	return &ShellGPGSigner{shell: shell}
}

func (g *ShellGPGSigner) GetIdentity(fingerprint string) (string, error) {
	cmd := fmt.Sprintf("gpg -K | grep -A 1 %s | tail -n 1 | cut -d ']' -f 2", fingerprint)
	out, err := g.shell.Output(cmd)
	if err != nil {
		return "", fmt.Errorf("failed to get maintainer identity: %w", err)
	}
	return strings.TrimSpace(out), nil
}

func (g *ShellGPGSigner) ClearSign(inputPath, outputPath, fingerprint string) error {
	cmd := fmt.Sprintf("gpg -u %s --clearsign --output %s --sign %s", fingerprint, outputPath, inputPath)
	return g.shell.RunInteractive(cmd)
}
