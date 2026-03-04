package repository

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type GPG struct {
	GnupgDir string
	IsDev    bool
}

func NewGPG(gnupgDir string, isDev bool) *GPG {
	return &GPG{GnupgDir: gnupgDir, IsDev: isDev}
}

func (g *GPG) gpgCmd(args ...string) *exec.Cmd {
	cmd := exec.Command("gpg", args...)
	if !g.IsDev {
		cmd.Env = append(os.Environ(), "GNUPGHOME="+g.GnupgDir)
	}
	return cmd
}

func (g *GPG) ListKeysWithColons() (string, error) {
	output, err := g.gpgCmd("--list-keys", "--with-colons").Output()
	if err != nil {
		return "", err
	}
	return string(output), nil
}

func (g *GPG) ListKeys() (string, error) {
	output, err := g.gpgCmd("--list-keys").Output()
	if err != nil {
		return "", err
	}
	// Skip the first line (trust database info)
	lines := strings.SplitN(string(output), "\n", 2)
	if len(lines) > 1 {
		return lines[1], nil
	}
	return string(output), nil
}

func (g *GPG) VerifySignedSubmission(submissionPath string) error {
	matches, err := filepath.Glob(filepath.Join(submissionPath, "signed", "*.dsc"))
	if err != nil {
		return err
	}
	if len(matches) == 0 {
		return exec.ErrNotFound
	}
	return g.gpgCmd("--verify", matches[0]).Run()
}

func (g *GPG) VerifyFile(filePath string) error {
	return g.gpgCmd("--verify", filePath).Run()
}
