package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/blankon/irgsh-go/internal/config"
	"github.com/blankon/irgsh-go/pkg/systemutil"
)

func TestMain(m *testing.M) {
	if os.Getenv("IRGSH_TEST_NATIVE_COMMAND") == "1" {
		switch filepath.Base(os.Args[0]) {
		case "dpkg":
			if !reflect.DeepEqual(os.Args[1:], []string{"--print-architecture"}) {
				os.Exit(2)
			}
			fmt.Println("amd64")
		case "sbuild":
			result := strings.TrimPrefix(os.Args[len(os.Args)-2], "--build-dir=")
			for _, suffix := range []string{".deb", ".udeb", ".ddeb", ".buildinfo"} {
				if err := os.WriteFile(filepath.Join(result, "hello_1.0_amd64"+suffix), []byte("built artifact"), 0600); err != nil {
					os.Exit(2)
				}
			}
			if !slices.Contains(os.Args, "--nolog") {
				if err := os.Symlink("sbuild.log", filepath.Join(result, "hello_1.0_amd64.build")); err != nil {
					os.Exit(2)
				}
			}
			fmt.Println("native sbuild output")
			data, err := json.Marshal([]any{os.Args[1:], os.Getenv("SBUILD_CONFIG"), os.Getenv("TMPDIR")})
			if err != nil {
				os.Exit(2)
			}
			if err := os.WriteFile(os.Getenv("IRGSH_TEST_NATIVE_RECORD"), data, 0600); err != nil {
				os.Exit(2)
			}
		default:
			os.Exit(2)
		}
		os.Exit(0)
	}
	if os.Getenv("IRGSH_TEST_BUILDER_MAIN") == "1" {
		main()
		return
	}
	os.Exit(m.Run())
}

func TestBuildPackageUsesNativeBackend(t *testing.T) {
	job, first, source, base := sbuildFixture(t)
	previous := irgshConfig
	t.Cleanup(func() { irgshConfig = previous })
	irgshConfig.Builder = config.BuilderConfig{Workdir: filepath.Dir(filepath.Dir(job.Root)), DistCodename: "verbeek", UpstreamDistCodename: "sid", UpstreamDistUrl: "http://deb.debian.org/debian", BuildAttempts: 1}
	record := nativeCommandFixture(t)
	last, err := buildPackage(context.Background(), job, first, source)
	if err != nil || last != first {
		t.Fatalf("build result = %+v, %v", last, err)
	}
	if _, err := collectArtifacts(context.Background(), job, last, source); err != nil {
		t.Fatalf("collect native build result: %v", err)
	}
	log, err := os.ReadFile(job.Log)
	if err != nil || !strings.Contains(string(log), "native sbuild output") {
		t.Fatalf("native build output missing from job log: %q, %v", log, err)
	}
	data, err := os.ReadFile(record)
	if err != nil {
		t.Fatal(err)
	}
	want, err := json.Marshal([]any{[]string{"--chroot-mode=unshare", "--chroot=" + first.Base, "--dist=verbeek", "--arch=amd64", "--arch-all", "--arch-any", "--no-source", "--enable-network", "--nolog", "--build-dir=" + first.Result, source.DSC}, base.Config, first.Temp})
	if err != nil || string(data) != string(want) {
		t.Fatalf("native command = %s, want %s, error = %v", data, want, err)
	}
}

func nativeCommandFixture(t *testing.T) string {
	t.Helper()
	commands := t.TempDir()
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"dpkg", "sbuild"} {
		if err := os.Symlink(self, filepath.Join(commands, name)); err != nil {
			t.Fatal(err)
		}
	}
	record := filepath.Join(commands, "command.json")
	t.Setenv("PATH", commands)
	t.Setenv("IRGSH_TEST_NATIVE_COMMAND", "1")
	t.Setenv("IRGSH_TEST_NATIVE_RECORD", record)
	return record
}

func TestSbuildArgsUsePinnedBaseAndControlledConfig(t *testing.T) {
	attempt := attemptPaths{Base: "/work/jobs/task/1/base.tar", Result: "/work/jobs/task/1/result", Temp: "/work/jobs/task/1/tmp"}
	source := sourceSet{DSC: "/work/jobs/task/1/input/hello_1.0.dsc"}
	base := basePaths{Config: "/work/bases/verbeek-sid-amd64-hash/sbuild.conf"}
	args, env := sbuildArgs(attempt, source, base, "verbeek", "amd64")
	want := []string{"--chroot-mode=unshare", "--chroot=/work/jobs/task/1/base.tar", "--dist=verbeek", "--arch=amd64", "--arch-all", "--arch-any", "--no-source", "--enable-network", "--nolog", "--build-dir=/work/jobs/task/1/result", "/work/jobs/task/1/input/hello_1.0.dsc"}
	if !reflect.DeepEqual(args, want) {
		t.Fatalf("args = %q; want %q", args, want)
	}
	if !reflect.DeepEqual(env, []string{"SBUILD_CONFIG=/work/bases/verbeek-sid-amd64-hash/sbuild.conf", "TMPDIR=/work/jobs/task/1/tmp"}) {
		t.Fatalf("environment = %q", env)
	}
}

func buildFailure(output string) error {
	return &systemutil.CommandError{Output: output, Err: errors.New("exit status 1")}
}

func TestTransientBuildFailureRecognizesDNS(t *testing.T) {
	testTransientBuildMessages(t, []string{"Could not resolve", "Temporary failure resolving", "Temporary failure in name resolution"})
}

func TestTransientBuildFailureRecognizesTLS(t *testing.T) {
	testTransientBuildMessages(t, []string{"TLS handshake timeout", "certificate verify failed", "SSL certificate problem", "Problem with the SSL CA cert"})
}

func TestTransientBuildFailureRecognizesTimeout(t *testing.T) {
	testTransientBuildMessages(t, []string{"connection timed out", "connection failed", "network is unreachable", "cannot initiate the connection", "unable to connect to"})
}

func TestTransientBuildFailureRecognizesFetchFailure(t *testing.T) {
	testTransientBuildMessages(t, []string{"failed to fetch"})
}

func testTransientBuildMessages(t *testing.T, messages []string) {
	t.Helper()
	for _, message := range messages {
		err := fmt.Errorf("build: %w", buildFailure(strings.ToUpper(message)))
		if !isTransientBuildFailure(err) {
			t.Errorf("did not recognize %q", message)
		}
	}
}

func TestTransientBuildFailureRejectsCompilerFailure(t *testing.T) {
	for _, err := range []error{nil, buildFailure("undefined reference to main"), buildFailure("unit test timed out"), errors.New("failed to fetch"), &systemutil.CommandError{Cmd: "failed to fetch", Desc: "could not resolve", Err: errors.New("compiler failed")}, &systemutil.CommandError{Output: "failed to fetch", Err: context.Canceled}} {
		if isTransientBuildFailure(err) {
			t.Errorf("classified permanent error as transient: %v", err)
		}
	}
}

func sbuildFixture(t *testing.T) (buildJob, attemptPaths, sourceSet, basePaths) {
	t.Helper()
	builder := baseFixture(t)
	base := oldBaseFixture(t, builder)
	job, err := newBuildJob(builder.Workdir, buildSubmission{TaskUUID: "job-123", Dist: "verbeek"})
	if err != nil {
		t.Fatal(err)
	}
	attempt, err := job.newAttempt(1)
	if err != nil {
		t.Fatal(err)
	}
	root, _, _ := sourceFixture(t, "hello_1.0.orig.tar.xz", []byte("source bytes"), "signed")
	source, err := prepareSource(context.Background(), root, attempt.Input)
	if err != nil {
		t.Fatal(err)
	}
	return job, attempt, source, base
}

func TestRunSbuildStopsAfterConfiguredAttempts(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		job, first, source, base := sbuildFixture(t)
		cause := buildFailure("FAILED TO FETCH archive")
		started := time.Now()
		var elapsed []time.Duration
		last, err := runSbuild(context.Background(), job, first, source, base, "amd64", 5, func(ctx context.Context, name string, args, env []string, desc, log string) (string, error) {
			if name != "sbuild" || log != job.Log {
				t.Fatalf("command = %s; log = %q", name, log)
			}
			elapsed = append(elapsed, time.Since(started))
			return "", cause
		})
		want := []time.Duration{0, 15 * time.Second, 45 * time.Second, 90 * time.Second, 135 * time.Second}
		if !errors.Is(err, cause) || last.Number != 5 || !reflect.DeepEqual(elapsed, want) {
			t.Fatalf("attempt = %d, times = %v, error = %v; want %v", last.Number, elapsed, err, want)
		}
	})
}

func TestRunSbuildDoesNotRetryPermanentFailure(t *testing.T) {
	job, first, source, base := sbuildFixture(t)
	cause := buildFailure("compiler failed")
	calls := 0
	_, err := runSbuild(context.Background(), job, first, source, base, "amd64", 3, func(context.Context, string, []string, []string, string, string) (string, error) {
		calls++
		return "", cause
	})
	if calls != 1 || !errors.Is(err, cause) {
		t.Fatalf("calls = %d, error = %v", calls, err)
	}
}

func TestRunSbuildCreatesFreshResultAndTempForRetry(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		job, first, source, base := sbuildFixture(t)
		for _, dir := range []string{first.Result, first.Temp} {
			writeArtifactFixture(t, filepath.Join(dir, "leftover"), "stale")
		}
		stale, err := job.newAttempt(2)
		if err != nil {
			t.Fatal(err)
		}
		for _, dir := range []string{stale.Result, stale.Temp} {
			writeArtifactFixture(t, filepath.Join(dir, "leftover"), "stale")
		}
		calls := 0
		last, err := runSbuild(context.Background(), job, first, source, base, "amd64", 3, func(ctx context.Context, name string, args, env []string, desc, log string) (string, error) {
			calls++
			result := strings.TrimPrefix(args[len(args)-2], "--build-dir=")
			temporary := strings.TrimPrefix(env[1], "TMPDIR=")
			for _, dir := range []string{result, temporary} {
				entries, err := os.ReadDir(dir)
				if err != nil || len(entries) != 0 {
					t.Fatalf("attempt %d did not start empty: %s, %v, %v", calls, dir, entries, err)
				}
				writeArtifactFixture(t, filepath.Join(dir, "attempt-output"), fmt.Sprint(calls))
			}
			if calls == 1 {
				return "", buildFailure("connection failed")
			}
			return "", nil
		})
		if err != nil || calls != 2 || last.Number != 2 || last.Result == first.Result || last.Temp == first.Temp {
			t.Fatalf("attempt = %+v, calls = %d, error = %v", last, calls, err)
		}
	})
}

func TestRunSbuildReusesImmutableInputAndPinnedBase(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		job, first, source, base := sbuildFixture(t)
		calls := 0
		last, err := runSbuild(context.Background(), job, first, source, base, "amd64", 3, func(ctx context.Context, name string, args, env []string, desc, log string) (string, error) {
			calls++
			input := filepath.Dir(args[len(args)-1])
			pinned := strings.TrimPrefix(args[1], "--chroot=")
			contents, err := os.ReadFile(pinned)
			if err != nil || string(contents) != "old base" {
				t.Fatalf("pinned base changed: %q, %v", contents, err)
			}
			for _, name := range source.Files {
				original, err := os.Stat(filepath.Join(first.Input, name))
				if err != nil {
					t.Fatal(err)
				}
				current, err := os.Stat(filepath.Join(input, name))
				if err != nil || !os.SameFile(original, current) || current.Mode().Perm() != 0444 {
					t.Fatalf("source %s not immutable hard-link: %v, %v", name, current, err)
				}
			}
			if calls == 1 {
				writeArtifactFixture(t, base.Tar+".new", "replacement base")
				if err := os.Rename(base.Tar+".new", base.Tar); err != nil {
					t.Fatal(err)
				}
				return "", buildFailure("TLS handshake timeout")
			}
			original, err := os.Stat(first.Base)
			if err != nil {
				t.Fatal(err)
			}
			current, err := os.Stat(pinned)
			if err != nil || !os.SameFile(original, current) {
				t.Fatalf("retry repinned replacement: %v", err)
			}
			return "", nil
		})
		if err != nil || last.Number != 2 {
			t.Fatalf("attempt = %+v, error = %v", last, err)
		}
	})
}

func TestRunSbuildValidatesOriginalSourceBeforeEveryAttempt(t *testing.T) {
	for _, corruptAt := range []int{0, 1} {
		t.Run(fmt.Sprint(corruptAt), func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				job, first, source, base := sbuildFixture(t)
				corrupt := func() {
					if err := os.Chmod(source.DSC, 0600); err != nil {
						t.Fatal(err)
					}
					contents, err := os.ReadFile(source.DSC)
					if err != nil {
						t.Fatal(err)
					}
					writeArtifactFixture(t, source.DSC, string(contents)+"\n")
				}
				if corruptAt == 0 {
					corrupt()
				}
				calls := 0
				_, err := runSbuild(context.Background(), job, first, source, base, "amd64", 3, func(context.Context, string, []string, []string, string, string) (string, error) {
					calls++
					corrupt()
					return "", buildFailure("failed to fetch")
				})
				if err == nil || !strings.Contains(err.Error(), "differs from original") || calls != corruptAt {
					t.Fatalf("calls = %d, error = %v", calls, err)
				}
			})
		})
	}
}

func TestWaitForRetryHonorsCancellation(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		started := time.Now()
		go func() { time.Sleep(time.Second); cancel() }()
		if err := waitForRetry(ctx, 45*time.Second); !errors.Is(err, context.Canceled) || time.Since(started) != time.Second {
			t.Fatalf("elapsed = %v, error = %v", time.Since(started), err)
		}
	})
}

func TestRunSbuildCancellationStopsAttempts(t *testing.T) {
	for _, duringWait := range []bool{false, true} {
		t.Run(fmt.Sprint(duringWait), func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				job, first, source, base := sbuildFixture(t)
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				calls := 0
				_, err := runSbuild(ctx, job, first, source, base, "amd64", 3, func(got context.Context, name string, args, env []string, desc, log string) (string, error) {
					calls++
					if got != ctx {
						t.Fatal("runner lost job context")
					}
					if duringWait {
						go func() { time.Sleep(time.Second); cancel() }()
					} else {
						cancel()
					}
					return "", buildFailure("failed to fetch")
				})
				if calls != 1 || !errors.Is(err, context.Canceled) {
					t.Fatalf("calls = %d, error = %v", calls, err)
				}
			})
		})
	}
}
