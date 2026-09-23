package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"
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
			err := includeBinaries(repository, artifacts, "verbeek", "main", environment, "repo.log", func(ctx context.Context, name string, args, env []string, desc, log string) (string, error) {
				calls++
				want := []string{"--basedir", repository, "-v", "-v", "-v", "--nothingiserror", "--component", "main"}
				want = append(want, tc.ignore...)
				want = append(want, tc.command, "verbeek", filepath.Join(artifacts, "hello_1.0_amd64"+tc.suffix))
				if name != "reprepro" || !reflect.DeepEqual(args, want) || !reflect.DeepEqual(env, environment) || log != "repo.log" {
					t.Fatalf("command = %s %q, env = %q, log = %q; want %q", name, args, env, log, want)
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
	err := includeBinaries(repository, artifacts, "verbeek-experimental", "main", nil, "", func(ctx context.Context, name string, args, env []string, desc, log string) (string, error) {
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
	err := includeBinaries(repository, artifacts, "verbeek", "main", nil, "", func(context.Context, string, []string, []string, string, string) (string, error) {
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
			err := includeBinaries(repository, artifacts, "verbeek", "main", nil, "", func(context.Context, string, []string, []string, string, string) (string, error) {
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
	err := includeBinaries(repository, artifacts, "verbeek", "main", nil, "", func(context.Context, string, []string, []string, string, string) (string, error) {
		t.Fatal("injected before validating binary files")
		return "", nil
	})
	if err == nil {
		t.Fatal("accepted non-regular binary")
	}
}
