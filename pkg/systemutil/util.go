package systemutil

import (
	"context"
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

// cmdOutputExcerptLines is how many trailing output lines are attached to a
// failing command error.
const cmdOutputExcerptLines = 20

// CommandError describes a command that exited non-zero.
//
// Its Error() carries the full context, for the worker's own stderr where
// there is nothing else to go on. Summary() is the short form for a job log,
// where the command and its output are already written out just above.
type CommandError struct {
	// Desc is the human readable step description passed to CmdExec.
	Desc string
	// Cmd is the command as given to CmdExec, before log redirection.
	Cmd string
	// Output is the combined output of the command.
	Output string
	// Err is the underlying error, typically *exec.ExitError.
	Err error
	// InLog reports whether the command and its output were also written to
	// a job log file.
	InLog bool
}

func (e *CommandError) Error() string {
	return fmt.Sprintf("%s failed (command: %s): %v%s",
		e.step(), truncate(strings.TrimSpace(e.Cmd), 500), e.Err, outputExcerpt(e.Output))
}

// Summary is the one line form: which step failed and how it exited.
func (e *CommandError) Summary() string {
	return fmt.Sprintf("%s failed: %v", e.step(), e.Err)
}

func (e *CommandError) Unwrap() error { return e.Err }

func (e *CommandError) step() string {
	desc := strings.TrimSpace(strings.ReplaceAll(e.Desc, "\n", " "))
	if desc == "" {
		desc = "command"
	}
	return desc
}

// FailureSummary renders err for a job log. A command whose output already
// went to that log is summarised instead of repeated; anything else is
// reported verbatim.
func FailureSummary(err error) string {
	if err == nil {
		return ""
	}
	var cmdErr *CommandError
	if errors.As(err, &cmdErr) && cmdErr.InLog {
		return cmdErr.Summary()
	}
	return err.Error()
}

// CmdExec run os command
func CmdExec(cmdStr string, cmdDesc string, logPath string) (out string, err error) {
	if len(cmdStr) == 0 {
		return "", errors.New("No command string provided.")
	}

	origCmd := cmdStr

	if len(logPath) > 0 {
		logDir := filepath.Dir(logPath)
		if mkErr := os.MkdirAll(logDir, os.ModePerm); mkErr != nil {
			return "", fmt.Errorf("failed to create log directory %s: %w", logDir, mkErr)
		}
		f, openErr := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
		if openErr != nil {
			return "", fmt.Errorf("failed to open log file %s: %w", logPath, openErr)
		}
		_, _ = f.WriteString("\n")
		if len(cmdDesc) > 0 {
			cmdDescSplitted := strings.Split(cmdDesc, "\n")
			for _, desc := range cmdDescSplitted {
				_, _ = f.WriteString("##### " + desc + "\n")
			}
		}
		_, _ = f.WriteString("##### RUN " + cmdStr + "\n")
		f.Close()
		// The command is wrapped in a brace group before being piped: `|`
		// binds tighter than `&&`, so `a && b | tee log` would only log the
		// output of `b`, silently dropping every earlier step of the
		// `&&`-chains the workers are built from.
		cmdStr = "{ " + cmdStr + "\n} 2>&1 | tee -a " + logPath
	}
	// `set -o pipefail` will forces to return the original exit code
	output, err := exec.Command("bash", "-c", "set -o pipefail && "+cmdStr).CombinedOutput()
	out = string(output)
	if err != nil {
		cmdErr := &CommandError{
			Desc:   cmdDesc,
			Cmd:    origCmd,
			Output: out,
			Err:    err,
			InLog:  len(logPath) > 0,
		}
		// Mark the failure in the job log itself, right after the output that
		// caused it, so a reader (or a live log stream) sees which step failed
		// even when the caller goes on to ignore the error.
		if cmdErr.InLog {
			_ = WriteLogFile(logPath, "##### FAILED: "+cmdErr.Summary())
		}
		err = cmdErr
	}

	return
}

// outputExcerpt returns the last few lines of a command output so the error
// itself carries the reason the command failed.
func outputExcerpt(out string) string {
	trimmed := strings.TrimRight(out, "\n")
	if trimmed == "" {
		return " (no output)"
	}
	lines := strings.Split(trimmed, "\n")
	if len(lines) > cmdOutputExcerptLines {
		lines = lines[len(lines)-cmdOutputExcerptLines:]
	}
	return "\noutput (last " + fmt.Sprint(len(lines)) + " line(s)):\n" + truncate(strings.Join(lines, "\n"), 4000)
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "... (truncated)"
}

// PrepareLogFile makes sure the directory and the log file itself exist, so a
// tailer can attach to it and early failures can still be written and uploaded.
func PrepareLogFile(logPath string) error {
	if logPath == "" {
		return nil
	}
	logDir := filepath.Dir(logPath)
	if err := os.MkdirAll(logDir, os.ModePerm); err != nil {
		return fmt.Errorf("failed to create log directory %s: %w", logDir, err)
	}
	f, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("failed to create log file %s: %w", logPath, err)
	}
	return f.Close()
}

// StreamLog tailing a file
func StreamLog(path string) {
	StreamLogTo(path, nil)
}

// StreamLogTo tails a file, printing every line to stdout and, when sink is
// non-nil, handing it the same line. It is used to mirror a job log to chief
// while the job is still running.
func StreamLogTo(path string, sink func(string)) {
	StreamLogContext(context.Background(), path, sink)
}

// StreamLogContext is StreamLogTo with a lifetime: it stops tailing, and
// releases the underlying file watcher, once ctx is cancelled.
func StreamLogContext(ctx context.Context, path string, sink func(string)) {
	t, err := tail.TailFile(path, tail.Config{Follow: true})
	if err != nil {
		log.Printf("error: %v\n", err)
		return
	}
	defer func() {
		_ = t.Stop()
		t.Cleanup()
	}()

	for {
		select {
		case <-ctx.Done():
			return
		case line, ok := <-t.Lines:
			if !ok {
				return
			}
			fmt.Println(line.Text)
			if sink != nil {
				sink(line.Text)
			}
		}
	}
}

// WriteLog appends a message to both stdout and the log file.
func WriteLog(logPath string, message string) error {
	fmt.Println(message)
	return WriteLogFile(logPath, message)
}

// WriteLogFile appends a message to the log file only. Use it for messages
// that a log tailer will echo to stdout anyway.
func WriteLogFile(logPath string, message string) error {
	if logPath == "" {
		return nil
	}

	f, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()

	_, err = f.WriteString(message + "\n")
	return err
}

func resetDir(dir string, mode os.FileMode) error {
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

	err = resetDir(dst, srcInfo.Mode())
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

		// filepath.Walk follows symlinks, so use Lstat to detect them
		lstatInfo, err := os.Lstat(currentPath)
		if err != nil {
			return err
		}
		if lstatInfo.Mode()&os.ModeSymlink != 0 {
			linkTarget, err := os.Readlink(currentPath)
			if err != nil {
				return err
			}
			return os.Symlink(linkTarget, targetPath)
		}

		if fileInfo.IsDir() {
			return os.MkdirAll(targetPath, fileInfo.Mode())
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
	if err := os.Rename(src, dst); err == nil {
		return nil
	} else if _, ok := err.(*os.LinkError); !ok {
		return err
	}

	// Cross-device fallback: copy + remove
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	srcInfo, err := in.Stat()
	if err != nil {
		return err
	}

	out, err := os.OpenFile(dst, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, srcInfo.Mode())
	if err != nil {
		return err
	}

	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		os.Remove(dst)
		return err
	}
	if err := out.Sync(); err != nil {
		out.Close()
		os.Remove(dst)
		return err
	}
	if err := out.Close(); err != nil {
		os.Remove(dst)
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
