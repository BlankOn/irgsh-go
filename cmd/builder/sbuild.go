package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/blankon/irgsh-go/pkg/systemutil"
)

func sbuildArgs(attempt attemptPaths, source sourceSet, base basePaths, dist string, architecture string) ([]string, []string) {
	return []string{
		"--chroot-mode=unshare",
		"--chroot=" + attempt.Base,
		"--dist=" + dist,
		"--arch=" + architecture,
		"--arch-all",
		"--arch-any",
		"--no-source",
		"--enable-network",
		"--nolog",
		"--build-dir=" + attempt.Result,
		source.DSC,
	}, []string{"SBUILD_CONFIG=" + base.Config, "TMPDIR=" + attempt.Temp}
}

var transientBuildMessages = []string{
	"could not resolve",
	"temporary failure resolving",
	"temporary failure in name resolution",
	"failed to fetch",
	"connection timed out",
	"connection failed",
	"network is unreachable",
	"cannot initiate the connection",
	"unable to connect to",
	"tls handshake timeout",
	"certificate verify failed",
	"ssl certificate problem",
	"problem with the ssl ca cert",
}

func isTransientBuildFailure(err error) bool {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	var commandErr *systemutil.CommandError
	if !errors.As(err, &commandErr) {
		return false
	}
	output := strings.ToLower(commandErr.Output)
	for _, message := range transientBuildMessages {
		if strings.Contains(output, message) {
			return true
		}
	}
	return false
}

func waitForRetry(ctx context.Context, delay time.Duration) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func runSbuild(ctx context.Context, job buildJob, first attemptPaths, source sourceSet, base basePaths, architecture string, attempts int, run commandRunner) (attemptPaths, error) {
	attempt := first
	if attempts < 1 {
		return attempt, fmt.Errorf("build attempts must be positive")
	}
	for number := 1; number <= attempts; number++ {
		if err := ctx.Err(); err != nil {
			return attempt, err
		}
		if number > 1 {
			var err error
			attempt, err = job.newAttempt(number)
			if err != nil {
				return attempt, err
			}
			for _, name := range source.Files {
				if err := os.Link(filepath.Join(first.Input, name), filepath.Join(attempt.Input, name)); err != nil {
					return attempt, fmt.Errorf("link retry source %q: %w", name, err)
				}
			}
		}
		source.DSC = filepath.Join(attempt.Input, filepath.Base(source.DSC))
		if err := validateSource(ctx, attempt.Input, source); err != nil {
			return attempt, fmt.Errorf("validate build source: %w", err)
		}
		baseSource := base.Tar
		if number > 1 {
			baseSource = first.Base
		}
		if err := pinBase(baseSource, attempt.Base); err != nil {
			return attempt, fmt.Errorf("pin build base: %w", err)
		}
		for _, dir := range []string{attempt.Result, attempt.Temp} {
			if err := os.RemoveAll(dir); err != nil {
				return attempt, fmt.Errorf("clear build directory: %w", err)
			}
			if err := os.Mkdir(dir, 0755); err != nil {
				return attempt, fmt.Errorf("create build directory: %w", err)
			}
		}
		args, env := sbuildArgs(attempt, source, base, job.Submission.Dist, architecture)
		_, err := run(ctx, "sbuild", args, env, fmt.Sprintf("Building the package (attempt %d/%d)", number, attempts), job.Log)
		if ctx.Err() != nil {
			return attempt, ctx.Err()
		}
		if err == nil || number == attempts || !isTransientBuildFailure(err) {
			return attempt, err
		}
		delay := time.Duration(min(number, 3)) * 15 * time.Second
		_ = systemutil.WriteLog(job.Log, fmt.Sprintf("Transient build failure; retrying in %s", delay))
		if err := waitForRetry(ctx, delay); err != nil {
			return attempt, err
		}
	}
	return attempt, nil
}
