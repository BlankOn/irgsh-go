package repository

import (
	"os"
	"os/exec"

	"github.com/blankon/irgsh-go/pkg/systemutil"
)

// ShellRunner executes shell commands for CLI usecases.
type ShellRunner struct{}

func (runner ShellRunner) Output(cmd string) (string, error) {
	return systemutil.CmdExec(cmd, "", "")
}

func (runner ShellRunner) Run(cmd string) error {
	_, err := systemutil.CmdExec(cmd, "", "")
	return err
}

func (runner ShellRunner) RunInteractive(cmd string) error {
	command := exec.Command("bash", "-c", cmd)
	command.Stdout = os.Stdout
	command.Stderr = os.Stderr
	command.Stdin = os.Stdin
	return command.Run()
}
