package repo

import "os/exec"

type GPG struct {
	GnupgDir string
	IsDev    bool
}

func NewGPG(gnupgDir string, isDev bool) *GPG {
	return &GPG{GnupgDir: gnupgDir, IsDev: isDev}
}

func (g *GPG) envPrefix() string {
	if g.IsDev {
		return ""
	}
	return "GNUPGHOME=" + g.GnupgDir + " "
}

func (g *GPG) ListKeysWithColons() (string, error) {
	cmdStr := g.envPrefix() + "gpg --list-keys --with-colons"
	output, err := exec.Command("bash", "-c", cmdStr).Output()
	if err != nil {
		return "", err
	}
	return string(output), nil
}

func (g *GPG) ListKeys() (string, error) {
	cmdStr := g.envPrefix() + "gpg --list-keys | tail -n +2"
	output, err := exec.Command("bash", "-c", cmdStr).Output()
	if err != nil {
		return "", err
	}
	return string(output), nil
}

func (g *GPG) VerifySignedSubmission(submissionPath string) error {
	cmdStr := "cd " + submissionPath + " && " + g.envPrefix() + "gpg --verify signed/*.dsc"
	return exec.Command("bash", "-c", cmdStr).Run()
}

func (g *GPG) VerifyFile(filePath string) error {
	cmdStr := g.envPrefix() + "gpg --verify " + filePath
	return exec.Command("bash", "-c", cmdStr).Run()
}
