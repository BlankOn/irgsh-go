package systemutil

import (
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
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

// WriteLog appends a message to both stdout and the log file using echo and tee
func WriteLog(logPath string, message string) error {
	if len(logPath) == 0 {
		fmt.Println(message)
		return nil
	}

	// Use echo with tee to write to both stdout and log file
	// This ensures the message appears in the streaming log
	cmdStr := fmt.Sprintf("echo '%s' | tee -a %s", message, logPath)
	_, err := exec.Command("bash", "-c", cmdStr).Output()
	return err
}

func ResetDir(dir string, mode os.FileMode) error {
	_, err := os.Stat(dir)
	if err == nil {
		if err := os.RemoveAll(dir); err != nil {
			return err
		}
	} else if !os.IsNotExist(err) {
		return err
	}

	return os.MkdirAll(dir, mode)
}

func CopyDir(src string, dst string) error {
	log.Printf("[copyDir] copying dir from %s to %s", src, dst)

	srcInfo, err := os.Stat(src)
	if err != nil {
		return fmt.Errorf("[copyDir] failed to stat source dir: %w", err)
	}
	if !srcInfo.IsDir() {
		return fmt.Errorf("[copyDir] source is not a directory: %s", src)
	}

	err = ResetDir(dst, srcInfo.Mode())
	if err != nil {
		return fmt.Errorf("[copyDir] failed to prepare destination dir: %w", err)
	}

	return filepath.Walk(src, func(currentPath string, fileInfo os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relPath, err := filepath.Rel(src, currentPath)
		if err != nil {
			return err
		}
		if relPath == "." {
			return nil
		}

		targetPath := filepath.Join(dst, relPath)
		if fileInfo.IsDir() {
			return os.MkdirAll(targetPath, fileInfo.Mode())
		}
		if fileInfo.Mode()&os.ModeSymlink != 0 {
			linkTarget, err := os.Readlink(currentPath)
			if err != nil {
				return err
			}
			return os.Symlink(linkTarget, targetPath)
		}

		return CopyFile(currentPath, targetPath, fileInfo.Mode())
	})
}

func CopyFile(src string, dst string, mode os.FileMode) (err error) {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	srcInfo, err := in.Stat()
	if err != nil {
		return fmt.Errorf("failed to stat source file: %w", err)
	}

	out, err := os.OpenFile(dst, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, mode)
	if err != nil {
		return err
	}
	defer func() {
		if closeErr := out.Close(); closeErr != nil && err == nil {
			err = closeErr
		}
	}()

	written, err := io.Copy(out, in)
	if err != nil {
		return err
	}

	if written != srcInfo.Size() {
		return fmt.Errorf("incomplete copy: wrote %d bytes, expected %d bytes", written, srcInfo.Size())
	}

	if err := out.Sync(); err != nil {
		return err
	}

	return nil
}

func MoveFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}

	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	if err := out.Close(); err != nil {
		return err
	}

	return os.Remove(src)
}

func ReadFileTrimmed(path string) (string, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(content)), nil
}

func WriteFile(path, value string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(value), 0644)
}
