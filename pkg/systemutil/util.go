package systemutil

import (
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"strings"

	"github.com/hpcloud/tail"
)

// CmdExec run os command
func CmdExec(cmdStr string, cmdDesc string, logPath string) (out string, err error) {
	if len(cmdStr) == 0 {
		return "", errors.New("No command string provided.")
	}

	if len(logPath) > 0 {

		logPathArr := strings.Split(logPath, "/")
		logPathArr = logPathArr[:len(logPathArr)-1]
		logDir := "/" + strings.Join(logPathArr, "/")
		os.MkdirAll(logDir, os.ModePerm)
		f, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
		if err != nil {
			return "", err
		}
		defer f.Close()
		_, _ = f.WriteString("\n")
		if len(cmdDesc) > 0 {
			cmdDescSplitted := strings.Split(cmdDesc, "\n")
			for _, desc := range cmdDescSplitted {
				_, _ = f.WriteString("##### " + desc + "\n")
			}
		}
		_, _ = f.WriteString("##### RUN " + cmdStr + "\n")
		f.Close()
		cmdStr += " 2>&1 | tee -a " + logPath
	}
	// `set -o pipefail` will forces to return the original exit code
	output, err := exec.Command("bash", "-c", "set -o pipefail && "+cmdStr).Output()
	out = string(output)

	return
}

// StreamLog tailing a file
func StreamLog(path string) {
	t, err := tail.TailFile(path, tail.Config{Follow: true})
	if err != nil {
		log.Printf("error: %v\n", err)
	}
	for line := range t.Lines {
		fmt.Println(line.Text)
	}
}
