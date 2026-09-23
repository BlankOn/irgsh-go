package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/blankon/irgsh-go/pkg/systemutil"
)

func binaryFixture(t *testing.T, names ...string) (string, string) {
	t.Helper()
	root := t.TempDir()
	artifacts := filepath.Join(root, "artifact files")
	if err := os.Mkdir(artifacts, 0755); err != nil {
		t.Fatal(err)
	}
	for _, name := range names {
		if err := os.WriteFile(filepath.Join(artifacts, name), []byte(name), 0600); err != nil {
			t.Fatal(err)
		}
	}
	return filepath.Join(root, "archive dir"), artifacts
}

func TestIncludeBinariesSelectsExactCommandsForPresentTypes(t *testing.T) {
	for _, tc := range []struct {
		suffix, command string
		ignore          []string
	}{{".deb", "includedeb", nil}, {".udeb", "includeudeb", nil}, {".ddeb", "includedeb", []string{"--ignore=extension"}}} {
		t.Run(tc.suffix, func(t *testing.T) {
			repository, artifacts := binaryFixture(t, "hello_1.0_amd64"+tc.suffix, "hello_1.0_amd64.buildinfo", "hello_1.0.dsc")
			calls := 0
			environment := []string{"GNUPGHOME=" + filepath.Join(repository, "keys")}
			err := includeBinaries(repository, artifacts, "verbeek", "main", environment, "repo.log", func(ctx context.Context, name string, args, env []string, directory, desc, log string) (string, error) {
				calls++
				want := []string{"--basedir", repository, "-v", "-v", "-v", "--nothingiserror", "--component", "main"}
				want = append(want, tc.ignore...)
				want = append(want, tc.command, "verbeek", filepath.Join(artifacts, "hello_1.0_amd64"+tc.suffix))
				if name != "reprepro" || !reflect.DeepEqual(args, want) || !reflect.DeepEqual(env, environment) || log != "repo.log" || directory != repository {
					t.Fatalf("command = %s %q, env = %q, log = %q, directory = %q; want %q in %q", name, args, env, log, directory, want, repository)
				}
				if ctx.Done() != nil {
					t.Fatal("archive transaction is interruptible")
				}
				if info, err := os.Stat(repository); err != nil || !info.IsDir() {
					t.Fatalf("repository directory missing: %v", err)
				}
				return "", nil
			})
			if err != nil || calls != 1 {
				t.Fatalf("calls = %d, error = %v", calls, err)
			}
		})
	}
}

func TestIncludeBinariesIncludesAllPresentTypesInOrder(t *testing.T) {
	repository, artifacts := binaryFixture(t, "hello.deb", "other.deb", "hello.udeb", "hello.ddeb")
	var got [][]string
	err := includeBinaries(repository, artifacts, "verbeek-experimental", "main", nil, "", func(ctx context.Context, name string, args, env []string, directory, desc, log string) (string, error) {
		got = append(got, args[8:])
		return "", nil
	})
	want := [][]string{
		{"includedeb", "verbeek-experimental", filepath.Join(artifacts, "hello.deb"), filepath.Join(artifacts, "other.deb")},
		{"includeudeb", "verbeek-experimental", filepath.Join(artifacts, "hello.udeb")},
		{"--ignore=extension", "includedeb", "verbeek-experimental", filepath.Join(artifacts, "hello.ddeb")},
	}
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("inclusions = %q, error = %v; want %q", got, err, want)
	}
}

func TestIncludeBinariesRejectsNoBinary(t *testing.T) {
	repository, artifacts := binaryFixture(t, "hello.dsc", "hello.buildinfo")
	err := includeBinaries(repository, artifacts, "verbeek", "main", nil, "", func(context.Context, string, []string, []string, string, string, string) (string, error) {
		t.Fatal("invoked reprepro without a binary")
		return "", nil
	})
	if err == nil {
		t.Fatal("accepted no binaries")
	}
}

func TestIncludeBinariesStopsOnFirstFailure(t *testing.T) {
	for _, failAt := range []int{1, 2, 3} {
		t.Run(fmt.Sprint(failAt), func(t *testing.T) {
			repository, artifacts := binaryFixture(t, "hello.deb", "hello.udeb", "hello.ddeb")
			cause := errors.New("inclusion failed")
			calls := 0
			err := includeBinaries(repository, artifacts, "verbeek", "main", nil, "", func(context.Context, string, []string, []string, string, string, string) (string, error) {
				calls++
				if calls == failAt {
					return "", cause
				}
				return "", nil
			})
			if calls != failAt || !errors.Is(err, cause) {
				t.Fatalf("calls = %d, error = %v", calls, err)
			}
		})
	}
}

func TestIncludeBinariesRejectsNonRegularBinaryBeforeInjection(t *testing.T) {
	repository, artifacts := binaryFixture(t, "hello.deb")
	if err := os.Symlink(filepath.Join(artifacts, "hello.deb"), filepath.Join(artifacts, "linked.ddeb")); err != nil {
		t.Fatal(err)
	}
	err := includeBinaries(repository, artifacts, "verbeek", "main", nil, "", func(context.Context, string, []string, []string, string, string, string) (string, error) {
		t.Fatal("injected before validating binary files")
		return "", nil
	})
	if err == nil {
		t.Fatal("accepted non-regular binary")
	}
}

func TestRepreproWorkingDirectoryChild(t *testing.T) {
	if os.Getenv("IRGSH_TEST_REPREPRO_CWD") != "1" {
		return
	}
	directory, err := os.Getwd()
	if err != nil {
		os.Exit(2)
	}
	contents, err := os.ReadFile("./local.conf")
	if err != nil {
		os.Exit(2)
	}
	if err := os.WriteFile(os.Getenv("IRGSH_TEST_REPREPRO_RECORD"), []byte(directory+"\n"+string(contents)), 0600); err != nil {
		os.Exit(2)
	}
	os.Exit(0)
}

func TestIncludeBinariesUsesRepositoryWorkingDirectory(t *testing.T) {
	worker := t.TempDir()
	t.Chdir(worker)
	writeIncludedUDebFixture(t, filepath.Join(worker, "local.conf"), "worker configuration")
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	for _, suffix := range []string{"", "-experimental"} {
		dist := "verbeek" + suffix
		t.Run(dist, func(t *testing.T) {
			_, artifacts := binaryFixture(t, "hello.deb")
			repo := udebRepoFixture(t)
			repository := filepath.Join(repo.Workdir, dist)
			writeDistributionFixture(t, repo, suffix, "!include: ./local.conf\n")
			contents := ownedUDebFixture(repo, dist)
			writeIncludedUDebFixture(t, filepath.Join(repository, "local.conf"), contents)
			if err := migrateUDebComponents(repo); err != nil {
				t.Fatal(err)
			}
			contents = strings.ReplaceAll(contents, "Components: main\n", "Components: "+repo.DistComponents+"\n")
			record := filepath.Join(t.TempDir(), "child.txt")
			environment := []string{"IRGSH_TEST_REPREPRO_CWD=1", "IRGSH_TEST_REPREPRO_RECORD=" + record}
			err := includeBinaries(repository, artifacts, dist, "main", environment, "", func(ctx context.Context, name string, args, env []string, directory, desc, log string) (string, error) {
				if name != "reprepro" {
					t.Fatalf("binary command = %q", name)
				}
				return systemutil.CmdExecArgsContextInDir(ctx, self, []string{"-test.run=^TestRepreproWorkingDirectoryChild$"}, env, directory, desc, log)
			})
			if err != nil {
				t.Fatal(err)
			}
			got, err := os.ReadFile(record)
			if want := repository + "\n" + contents; err != nil || string(got) != want {
				t.Fatalf("binary child directory/config = %q, %v; want %q", got, err, want)
			}
			if current, err := os.Getwd(); err != nil || current != worker {
				t.Fatalf("worker directory changed: %q, %v", current, err)
			}
		})
	}
}
