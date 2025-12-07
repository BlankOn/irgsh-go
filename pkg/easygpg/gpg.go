package easypgp

import (
	"bytes"
	"errors"
	"os/exec"
)

// package EasyPGP provide ready to use gpg interface

// IEasyPGP ...
type IEasyPGP interface {
	Verify(dirPath, fileName string) (bool, error)
}

const gpgCommand string = "gpg --verify "

// EasyPGP provide easy to use gpg
type EasyPGP struct {
}

// Verify provided signature
func (E EasyPGP) Verify(dirPath, fileName string) (ok bool, err error) {
	var stdErr bytes.Buffer
	cmd := exec.Command("sh", "-c", gpgCommand+fileName)
	cmd.Dir = dirPath
	cmd.Stderr = &stdErr
	err = cmd.Run()

	if err != nil {
		err = errors.New(stdErr.String())
	} else {
		ok = true
	}

	return
}
