package systemutil

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestCmdExecArgsContextTreatsArgumentsLiterally(t *testing.T) {
	dir := t.TempDir()
	marker := filepath.Join(dir, "expanded")
	logPath := filepath.Join(dir, "command.log")
	arg := "$(touch " + marker + ")"
	out, err := CmdExecArgsContext(context.Background(), "printf", []string{"%s", arg}, nil, "literal argv", logPath)
	if err != nil {
		t.Fatal(err)
	}
	if out != arg {
		t.Fatalf("expected %q, got %q", arg, out)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("argument was interpreted by a shell: %v", err)
	}
	contents, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(contents), "literal argv") || !strings.Contains(string(contents), arg) {
		t.Fatalf("missing command record:\n%s", contents)
	}
}

func TestCmdExecArgsContextInDirUsesChildWorkingDirectory(t *testing.T) {
	worker := t.TempDir()
	t.Chdir(worker)
	directory := t.TempDir()
	name := "input file; literal"
	if err := os.WriteFile(filepath.Join(directory, name), []byte("child directory input"), 0600); err != nil {
		t.Fatal(err)
	}
	out, err := CmdExecArgsContextInDir(context.Background(), "cat", []string{name}, nil, directory, "read relative file", "")
	if err != nil || out != "child directory input" {
		t.Fatalf("child relative file = %q, %v", out, err)
	}
	out, err = CmdExecArgsContextInDir(context.Background(), "printenv", []string{"PWD"}, nil, directory, "child PWD", "")
	if err != nil || strings.TrimSpace(out) != directory {
		t.Fatalf("child PWD = %q, %v; want %q", out, err, directory)
	}
	if current, err := os.Getwd(); err != nil || current != worker {
		t.Fatalf("worker directory changed: %q, %v", current, err)
	}
}

func TestCmdExecArgsContextInDirRejectsMissingDirectory(t *testing.T) {
	directory := filepath.Join(t.TempDir(), "missing")
	if _, err := CmdExecArgsContextInDir(context.Background(), "pwd", nil, nil, directory, "missing directory", ""); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("missing child directory error = %v", err)
	}
}

func TestCmdExecArgsContextCancellationDoesNotUseSudo(t *testing.T) {
	dir := t.TempDir()
	marker := filepath.Join(dir, "sudo-called")
	sudo := filepath.Join(dir, "sudo")
	if err := os.WriteFile(sudo, []byte("#!/bin/sh\ntouch \"$SUDO_MARKER\"\n"), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("SUDO_MARKER", marker)
	pidPath := filepath.Join(dir, "command.pid")
	t.Setenv("COMMAND_PID", pidPath)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		_, err := CmdExecArgsContext(ctx, "sh", []string{"-c", "echo $$ > \"$COMMAND_PID\"; exec sleep 60"}, nil, "cancel", "")
		done <- err
	}()
	waitForTestPID(t, pidPath)
	cancel()
	if err := <-done; err == nil {
		t.Fatal("expected cancellation error")
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("sudo was called: %v", err)
	}
}

func waitForTestPID(t *testing.T, filename string) int {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		data, err := os.ReadFile(filename)
		if err != nil && !os.IsNotExist(err) {
			t.Fatal(err)
		}
		if err == nil {
			pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
			if err == nil && pid > 0 {
				return pid
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("process readiness was not recorded")
	return 0
}

func TestCmdExecArgsContextCancellationKillsDetachedOutputDescendant(t *testing.T) {
	pidPath := filepath.Join(t.TempDir(), "child.pid")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		_, err := CmdExecArgsContext(ctx, "sh", []string{"-c", `sh -c 'trap "" TERM; echo $$ > "$CHILD_PID"; exec sleep 60' >/dev/null 2>&1 & wait`}, []string{"CHILD_PID=" + pidPath}, "cancel resistant descendant", "")
		done <- err
	}()
	childPID := waitForTestPID(t, pidPath)
	t.Cleanup(func() { _ = syscall.Kill(childPID, syscall.SIGKILL) })
	cancel()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("expected cancellation error")
		}
	case <-time.After(killGrace + 5*time.Second):
		t.Fatal("command did not finish group cancellation")
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		data, err := os.ReadFile("/proc/" + strconv.Itoa(childPID) + "/stat")
		if os.IsNotExist(err) || (err == nil && strings.HasPrefix(string(data[strings.LastIndex(string(data), ")")+1:]), " Z ")) {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("SIGTERM-resistant descendant %d survived command cancellation", childPID)
}

func TestCmdExecArgsContextCancellationTerminatesDescendant(t *testing.T) {
	pidPath := filepath.Join(t.TempDir(), "child.pid")
	t.Setenv("CHILD_PID", pidPath)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		_, err := CmdExecArgsContext(ctx, "sh", []string{"-c", "sleep 60 & echo $! > \"$CHILD_PID\"; wait"}, nil, "cancel descendants", "")
		done <- err
	}()
	deadline := time.Now().Add(2 * time.Second)
	var childPID int
	for time.Now().Before(deadline) {
		contents, err := os.ReadFile(pidPath)
		if err == nil {
			childPID, err = strconv.Atoi(strings.TrimSpace(string(contents)))
			if err != nil {
				t.Fatal(err)
			}
			break
		}
		if !os.IsNotExist(err) {
			t.Fatal(err)
		}
		time.Sleep(10 * time.Millisecond)
	}
	if childPID == 0 {
		t.Fatal("child PID was not recorded")
	}
	cancel()
	if err := <-done; err == nil {
		t.Fatal("expected cancellation error")
	}
	deadline = time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if err := syscall.Kill(childPID, 0); errors.Is(err, syscall.ESRCH) {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	_ = syscall.Kill(childPID, syscall.SIGKILL)
	t.Fatalf("descendant process %d still exists", childPID)
}

func TestCmdExec_FailureMarksTheLog(t *testing.T) {
	logPath := filepath.Join(t.TempDir(), "job", "repo.log")

	_, err := CmdExec("echo 'first output line' && exit 1", "Injecting the deb files", logPath)
	if err == nil {
		t.Fatal("expected an error for a command that exits non-zero")
	}

	contents, readErr := os.ReadFile(logPath)
	if readErr != nil {
		t.Fatal(readErr)
	}
	log := string(contents)

	// The command output must be in the log, followed by a failure marker.
	if !strings.Contains(log, "first output line") {
		t.Fatalf("command output missing from the log:\n%s", log)
	}
	want := "##### FAILED: Injecting the deb files failed: exit status 1"
	if !strings.Contains(log, want) {
		t.Fatalf("expected %q in the log:\n%s", want, log)
	}
	// The marker is the last line, so a reader sees where the job stopped.
	lines := strings.Split(strings.TrimRight(log, "\n"), "\n")
	if lines[len(lines)-1] != want {
		t.Fatalf("failure marker is not the last line:\n%s", log)
	}
}

func TestCmdExec_SuccessLeavesNoFailureMarker(t *testing.T) {
	logPath := filepath.Join(t.TempDir(), "job", "repo.log")

	if _, err := CmdExec("echo ok", "Exporting the repository", logPath); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	contents, _ := os.ReadFile(logPath)
	if strings.Contains(string(contents), "FAILED") {
		t.Fatalf("a successful command must not be marked as failed:\n%s", contents)
	}
}

func TestCommandError_ErrorCarriesFullContext(t *testing.T) {
	_, err := CmdExec("echo 'the real reason' && exit 3", "Injecting the deb files", "")

	var cmdErr *CommandError
	if !errors.As(err, &cmdErr) {
		t.Fatalf("expected a *CommandError, got %T", err)
	}

	// The worker's own stderr has no log to refer to, so the error itself has
	// to carry the command and the output.
	msg := cmdErr.Error()
	for _, want := range []string{"Injecting the deb files failed", "exit status 3", "echo 'the real reason'", "the real reason"} {
		if !strings.Contains(msg, want) {
			t.Fatalf("expected %q in %q", want, msg)
		}
	}
}

func TestFailureSummary(t *testing.T) {
	logged := &CommandError{Desc: "Injecting the deb files", Cmd: "reprepro includedeb ...", Output: "noise\n", Err: errors.New("exit status 1"), InLog: true}
	if got, want := FailureSummary(logged), "Injecting the deb files failed: exit status 1"; got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
	if strings.Contains(FailureSummary(logged), "reprepro") {
		t.Fatal("a command already written to the log must not be repeated in the summary")
	}

	// Nothing wrote this one to a log, so the detail has to survive.
	unlogged := &CommandError{Desc: "Uploading log file", Cmd: "curl ...", Output: "boom\n", Err: errors.New("exit status 26")}
	if !strings.Contains(FailureSummary(unlogged), "curl ...") {
		t.Fatalf("expected the command in %q", FailureSummary(unlogged))
	}

	if got, want := FailureSummary(errors.New("plain failure")), "plain failure"; got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
	if got := FailureSummary(nil); got != "" {
		t.Fatalf("expected an empty summary for a nil error, got %q", got)
	}
}

func TestCommandError_UnwrapsToTheExitError(t *testing.T) {
	_, err := CmdExec("exit 7", "Some step", "")
	var cmdErr *CommandError
	if !errors.As(err, &cmdErr) {
		t.Fatalf("expected a *CommandError, got %T", err)
	}
	if cmdErr.Unwrap() == nil {
		t.Fatal("the underlying exec error must stay reachable")
	}
}

func TestCmdExec_LogsEveryStepOfAnAndChain(t *testing.T) {
	logPath := filepath.Join(t.TempDir(), "job", "build.log")

	if _, err := CmdExec("echo step-one && echo step-two && echo step-three", "Chained step", logPath); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	contents, _ := os.ReadFile(logPath)
	for _, want := range []string{"step-one", "step-two", "step-three"} {
		if !strings.Contains(string(contents), want) {
			t.Fatalf("expected %q in the log (pipe binds tighter than &&):\n%s", want, contents)
		}
	}
}

func TestCmdExecArgsContextInDirReadsEOF(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	out, err := CmdExecArgsContextInDir(ctx, "cat", nil, nil, "", "stdin", "")
	if err != nil || out != "" {
		t.Fatalf("cat without input = %q, %v", out, err)
	}
}
