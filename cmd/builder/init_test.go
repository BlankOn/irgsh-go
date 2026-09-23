package main

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/blankon/irgsh-go/internal/config"
)

func baseFixture(t *testing.T) config.BuilderConfig {
	t.Helper()
	return config.BuilderConfig{Workdir: t.TempDir(), DistCodename: "verbeek", UpstreamDistCodename: "sid", UpstreamDistUrl: "http://deb.debian.org/debian"}
}

func TestBaseIdentityUsesExactArchiveURLHash(t *testing.T) {
	builder := baseFixture(t)
	for _, archive := range []string{"http://deb.debian.org/debian", "http://deb.debian.org/debian/"} {
		builder.UpstreamDistUrl = archive
		digest := fmt.Sprintf("%x", sha256.Sum256([]byte(archive)))
		got, err := baseIdentity(builder, "amd64")
		if want := "verbeek-sid-amd64-" + digest[:12]; err != nil || got != want {
			t.Fatalf("identity = %q, %v; want %q", got, err, want)
		}
	}
}

func TestBaseIdentityRejectsUnsafeDistribution(t *testing.T)  { testUnsafeBaseIdentity(t, "dist") }
func TestBaseIdentityRejectsUnsafeUpstreamSuite(t *testing.T) { testUnsafeBaseIdentity(t, "suite") }
func TestBaseIdentityRejectsUnsafeArchitecture(t *testing.T) {
	testUnsafeBaseIdentity(t, "architecture")
}

func testUnsafeBaseIdentity(t *testing.T, field string) {
	t.Helper()
	for _, value := range []string{"", ".", "..", "../sid", "/sid", "sid/name", "sid name", "sid\n", "--help"} {
		builder := baseFixture(t)
		architecture := "amd64"
		switch field {
		case "dist":
			builder.DistCodename = value
		case "suite":
			builder.UpstreamDistCodename = value
		default:
			architecture = value
		}
		if _, err := baseIdentity(builder, architecture); err == nil {
			t.Errorf("accepted unsafe %s %q", field, value)
		}
	}
}

func TestNativeArchitectureTrimsDpkgOutput(t *testing.T) {
	got, err := nativeArchitecture(context.Background(), func(ctx context.Context, name string, args, env []string, desc, log string) (string, error) {
		if name != "dpkg" || !reflect.DeepEqual(args, []string{"--print-architecture"}) {
			t.Fatalf("command = %s %v", name, args)
		}
		return " amd64\n", nil
	})
	if err != nil || got != "amd64" {
		t.Fatalf("architecture = %q, %v", got, err)
	}
}

func TestNativeArchitectureRejectsInvalidOutputAndCommandFailure(t *testing.T) {
	cause := errors.New("dpkg failed")
	for _, tc := range []struct {
		output string
		err    error
	}{{"../amd64", nil}, {"", nil}, {"amd64", cause}} {
		_, err := nativeArchitecture(context.Background(), func(context.Context, string, []string, []string, string, string) (string, error) {
			return tc.output, tc.err
		})
		if err == nil || tc.err != nil && !errors.Is(err, cause) {
			t.Fatalf("output %q: %v", tc.output, err)
		}
	}
}

func TestBuilderBasePathsAreAbsoluteAndConfined(t *testing.T) {
	paths, err := builderBasePaths("relative", "verbeek-sid-amd64-hash")
	root, absErr := filepath.Abs("relative/bases/verbeek-sid-amd64-hash")
	want := basePaths{Identity: "verbeek-sid-amd64-hash", Root: root, Tar: filepath.Join(root, "base.tar"), Config: filepath.Join(root, "sbuild.conf"), Temp: filepath.Join(root, "tmp")}
	if err != nil || absErr != nil || paths != want {
		t.Fatalf("paths = %+v, %v; want %+v", paths, err, want)
	}
	if _, err := builderBasePaths(t.TempDir(), "../escape"); err == nil {
		t.Fatal("accepted unsafe identity")
	}
}

func rebuildFixture(t *testing.T, builder config.BuilderConfig, create func(basePaths) error) (basePaths, error) {
	t.Helper()
	identity, err := baseIdentity(builder, "amd64")
	if err != nil {
		t.Fatal(err)
	}
	paths, err := builderBasePaths(builder.Workdir, identity)
	if err != nil {
		t.Fatal(err)
	}
	calls := 0
	result, err := rebuildBase(context.Background(), builder, func(ctx context.Context, name string, args, env []string, desc, log string) (string, error) {
		calls++
		if calls == 1 {
			if name != "dpkg" || !reflect.DeepEqual(args, []string{"--print-architecture"}) {
				t.Fatalf("architecture command = %s %v", name, args)
			}
			return "amd64\n", nil
		}
		want := []string{"--mode=unshare", "--variant=buildd", "--format=tar", "--architectures=amd64", "--include=ca-certificates", "sid", paths.Tar + ".new", "http://deb.debian.org/debian"}
		if calls != 2 || name != "mmdebstrap" || !reflect.DeepEqual(args, want) {
			t.Fatalf("base command = %s %v; want %v", name, args, want)
		}
		if !reflect.DeepEqual(env, []string{"TMPDIR=" + paths.Temp}) {
			t.Fatalf("environment = %v", env)
		}
		for _, dir := range []string{paths.Root, paths.Temp} {
			info, err := os.Stat(dir)
			if err != nil || !info.IsDir() || info.Mode().Perm() != 0755 {
				t.Fatalf("base directory %s: %v, %v", dir, info, err)
			}
		}
		return "", create(paths)
	})
	return result, err
}

func TestRebuildBaseUsesExactMMDebstrapArguments(t *testing.T) {
	_, err := rebuildFixture(t, baseFixture(t), func(paths basePaths) error { return os.WriteFile(paths.Tar+".new", []byte("new base"), 0600) })
	if err != nil {
		t.Fatal(err)
	}
}

func TestRebuildBaseWritesExactSbuildConfig(t *testing.T) {
	paths, err := rebuildFixture(t, baseFixture(t), func(paths basePaths) error { return os.WriteFile(paths.Tar+".new", []byte("new base"), 0600) })
	if err != nil {
		t.Fatal(err)
	}
	contents, err := os.ReadFile(paths.Config)
	want := "$unshare_mmdebstrap_auto_create = 0;\n$unshare_tmpdir_template = '" + paths.Temp + "/tmp.sbuild.XXXXXXXXXX';\n1;\n"
	if err != nil || string(contents) != want {
		t.Fatalf("config = %q, %v; want %q", contents, err, want)
	}
	info, err := os.Stat(paths.Config)
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatalf("config mode: %v, %v", info, err)
	}
}

func TestRebuildBaseEscapesPerlConfigPath(t *testing.T) {
	builder := baseFixture(t)
	builder.Workdir = filepath.Join(builder.Workdir, "a'b\\c")
	paths, err := rebuildFixture(t, builder, func(paths basePaths) error { return os.WriteFile(paths.Tar+".new", []byte("new base"), 0600) })
	if err != nil {
		t.Fatal(err)
	}
	contents, err := os.ReadFile(paths.Config)
	if err != nil || !strings.Contains(string(contents), "a\\'b\\\\c/bases/") {
		t.Fatalf("unescaped config: %q, %v", contents, err)
	}
}

func oldBaseFixture(t *testing.T, builder config.BuilderConfig) basePaths {
	t.Helper()
	identity, err := baseIdentity(builder, "amd64")
	if err != nil {
		t.Fatal(err)
	}
	paths, err := builderBasePaths(builder.Workdir, identity)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(paths.Root, 0755); err != nil {
		t.Fatal(err)
	}
	writeArtifactFixture(t, paths.Tar, "old base")
	writeArtifactFixture(t, paths.Config, "old config")
	writeArtifactFixture(t, paths.Tar+".new", "stale base")
	return paths
}

func TestRebuildBaseAtomicallyReplacesOldBase(t *testing.T) {
	builder := baseFixture(t)
	paths := oldBaseFixture(t, builder)
	old, err := os.Stat(paths.Tar)
	if err != nil {
		t.Fatal(err)
	}
	_, err = rebuildFixture(t, builder, func(paths basePaths) error {
		contents, err := os.ReadFile(paths.Tar)
		if err != nil || string(contents) != "old base" {
			t.Fatalf("old base removed before replacement: %q, %v", contents, err)
		}
		if _, err := os.Lstat(paths.Tar + ".new"); !os.IsNotExist(err) {
			t.Fatalf("stale output remains: %v", err)
		}
		return os.WriteFile(paths.Tar+".new", []byte("new base"), 0600)
	})
	if err != nil {
		t.Fatal(err)
	}
	contents, err := os.ReadFile(paths.Tar)
	if err != nil || string(contents) != "new base" {
		t.Fatalf("base = %q, %v", contents, err)
	}
	current, err := os.Stat(paths.Tar)
	if err != nil || os.SameFile(old, current) {
		t.Fatalf("base inode was reused: %v", err)
	}
	if _, err := os.Stat(paths.Tar + ".new"); !os.IsNotExist(err) {
		t.Fatalf("temporary base remains: %v", err)
	}
}

func TestRebuildBaseFailurePreservesOldBase(t *testing.T) {
	builder := baseFixture(t)
	paths := oldBaseFixture(t, builder)
	cause := errors.New("archive unavailable")
	_, err := rebuildFixture(t, builder, func(paths basePaths) error {
		writeArtifactFixture(t, paths.Tar+".new", "partial base")
		return cause
	})
	if !errors.Is(err, cause) {
		t.Fatalf("error = %v", err)
	}
	for filename, want := range map[string]string{paths.Tar: "old base", paths.Config: "old config"} {
		contents, err := os.ReadFile(filename)
		if err != nil || string(contents) != want {
			t.Fatalf("previous state changed: %q, %v", contents, err)
		}
	}
	if _, err := os.Lstat(paths.Tar + ".new"); !os.IsNotExist(err) {
		t.Fatalf("failed base remains: %v", err)
	}
}

func TestRebuildBaseRejectsEmptyOrNonRegularOutput(t *testing.T) {
	for _, kind := range []string{"missing", "empty", "directory", "symlink"} {
		t.Run(kind, func(t *testing.T) {
			builder := baseFixture(t)
			paths := oldBaseFixture(t, builder)
			_, err := rebuildFixture(t, builder, func(paths basePaths) error {
				switch kind {
				case "empty":
					return os.WriteFile(paths.Tar+".new", nil, 0600)
				case "directory":
					return os.Mkdir(paths.Tar+".new", 0755)
				case "symlink":
					return os.Symlink(paths.Tar, paths.Tar+".new")
				}
				return nil
			})
			if err == nil {
				t.Fatal("accepted invalid base")
			}
			contents, err := os.ReadFile(paths.Tar)
			if err != nil || string(contents) != "old base" {
				t.Fatalf("previous base changed: %q, %v", contents, err)
			}
			if _, err := os.Lstat(paths.Tar + ".new"); !os.IsNotExist(err) {
				t.Fatalf("invalid output remains: %v", err)
			}
		})
	}
}

func TestPinBaseKeepsOldInodeAfterReplacement(t *testing.T) {
	builder := baseFixture(t)
	paths := oldBaseFixture(t, builder)
	pinned := filepath.Join(builder.Workdir, "pinned.tar")
	if err := pinBase(paths.Tar, pinned); err != nil {
		t.Fatal(err)
	}
	original, err := os.Stat(paths.Tar)
	if err != nil {
		t.Fatal(err)
	}
	linked, err := os.Stat(pinned)
	if err != nil || !os.SameFile(original, linked) {
		t.Fatalf("base not hard-linked: %v", err)
	}
	if err := pinBase(paths.Tar, pinned); !errors.Is(err, os.ErrExist) {
		t.Fatalf("existing pin overwritten: %v", err)
	}
	_, err = rebuildFixture(t, builder, func(paths basePaths) error { return os.WriteFile(paths.Tar+".new", []byte("new base"), 0600) })
	if err != nil {
		t.Fatal(err)
	}
	contents, err := os.ReadFile(pinned)
	if err != nil || string(contents) != "old base" {
		t.Fatalf("pinned base changed: %q, %v", contents, err)
	}
}
