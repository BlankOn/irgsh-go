package main

import (
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/blankon/irgsh-go/internal/config"
)

type hostFileInfo struct {
	name string
	mode os.FileMode
	uid  uint32
}

func (info hostFileInfo) Name() string       { return info.name }
func (info hostFileInfo) Size() int64        { return 0 }
func (info hostFileInfo) Mode() os.FileMode  { return info.mode }
func (info hostFileInfo) ModTime() time.Time { return time.Time{} }
func (info hostFileInfo) IsDir() bool        { return false }
func (info hostFileInfo) Sys() any           { return &syscall.Stat_t{Uid: info.uid} }

func validHostProbe() hostProbe {
	return hostProbe{
		EUID:     1000,
		Username: "irgsh-builder",
		SubUID:   []byte("other:100000:65536\nirgsh-builder:165536:65536\n"),
		SubGID:   []byte("irgsh-builder:165536:65536\n"),
		LookupPath: func(name string) (string, error) {
			return filepath.Join("/usr/bin", name), nil
		},
		Stat: func(path string) (os.FileInfo, error) {
			return hostFileInfo{name: filepath.Base(path), mode: 0755 | os.ModeSetuid}, nil
		},
		Output: func(path string, args ...string) ([]byte, error) {
			if filepath.Base(path) == "sbuild" {
				return []byte("sbuild (Debian sbuild) 0.88.3ubuntu2~bpo24.04.1 (08 July 2025)\n"), nil
			}
			return nil, nil
		},
	}
}

func TestValidateBuilderHostSbuildVersion(t *testing.T) {
	cases := []struct {
		name    string
		output  string
		runErr  error
		version string
		compare error
		want    string
	}{
		{name: "minimum", output: "sbuild (Debian sbuild) 0.87.0 (01 September 2024)\n", version: "0.87.0"},
		{name: "backports", output: "sbuild (Debian sbuild) 0.88.3ubuntu2~bpo24.04.1 (08 July 2025)\n", version: "0.88.3ubuntu2~bpo24.04.1"},
		{name: "old", output: "sbuild (Debian sbuild) 0.85.10ubuntu0.3 (01 May 2024)\n", version: "0.85.10ubuntu0.3", compare: errors.New("older"), want: "sbuild >= 0.87.0"},
		{name: "unreadable version", runErr: errors.New("bad configuration"), want: "read sbuild version"},
		{name: "empty output", want: "read sbuild version"},
		{name: "unexpected output", output: "different command 999\n", want: "read sbuild version"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			probe := validHostProbe()
			compared := false
			probe.Output = func(path string, args ...string) ([]byte, error) {
				switch filepath.Base(path) {
				case "sbuild":
					if strings.Join(args, " ") != "--version" {
						t.Fatalf("unexpected sbuild arguments: %q", args)
					}
					return []byte(tc.output), tc.runErr
				case "dpkg":
					compared = true
					if strings.Join(args, " ") != "--compare-versions "+tc.version+" ge 0.87.0" {
						t.Fatalf("unexpected version comparison: %q", args)
					}
					return nil, tc.compare
				default:
					t.Fatalf("unexpected executable: %s", path)
					return nil, nil
				}
			}
			err := validateBuilderHost(probe)
			if tc.want == "" && err != nil || tc.want != "" && (err == nil || !strings.Contains(err.Error(), tc.want)) {
				t.Fatalf("error = %v; want %q", err, tc.want)
			}
			if compared != (tc.version != "") {
				t.Fatalf("version compared = %v; version %q", compared, tc.version)
			}
		})
	}
}

func TestValidateBuilderHost(t *testing.T) {
	missingTool := func(name string) hostProbe {
		probe := validHostProbe()
		probe.LookupPath = func(candidate string) (string, error) {
			if candidate == name {
				return "", errors.New("not found")
			}
			return filepath.Join("/usr/bin", candidate), nil
		}
		return probe
	}
	helperProblem := func(name string, info hostFileInfo) hostProbe {
		probe := validHostProbe()
		probe.Stat = func(path string) (os.FileInfo, error) {
			if filepath.Base(path) == name {
				return info, nil
			}
			return hostFileInfo{name: filepath.Base(path), mode: 0755 | os.ModeSetuid}, nil
		}
		return probe
	}
	cases := []struct {
		name  string
		probe hostProbe
		want  string
	}{
		{name: "valid host", probe: validHostProbe()},
		{name: "root EUID", probe: func() hostProbe { probe := validHostProbe(); probe.EUID = 0; return probe }(), want: "non-root"},
		{name: "missing username", probe: func() hostProbe { probe := validHostProbe(); probe.Username = ""; return probe }(), want: "username"},
		{name: "missing sbuild", probe: missingTool("sbuild"), want: "sbuild"},
		{name: "missing mmdebstrap", probe: missingTool("mmdebstrap"), want: "mmdebstrap"},
		{name: "missing dpkg", probe: missingTool("dpkg"), want: "dpkg"},
		{name: "missing newuidmap", probe: missingTool("newuidmap"), want: "newuidmap"},
		{name: "missing newgidmap", probe: missingTool("newgidmap"), want: "newgidmap"},
		{name: "missing subordinate UID row", probe: func() hostProbe {
			probe := validHostProbe()
			probe.SubUID = []byte("other:100000:65536\n")
			return probe
		}(), want: "/etc/subuid"},
		{name: "missing subordinate GID row", probe: func() hostProbe { probe := validHostProbe(); probe.SubGID = nil; return probe }(), want: "/etc/subgid"},
		{name: "subordinate UID range below minimum", probe: func() hostProbe {
			probe := validHostProbe()
			probe.SubUID = []byte("irgsh-builder:165536:65535\n")
			return probe
		}(), want: "65536"},
		{name: "subordinate GID range below minimum", probe: func() hostProbe {
			probe := validHostProbe()
			probe.SubGID = []byte("irgsh-builder:165536:65535\n")
			return probe
		}(), want: "65536"},
		{name: "newuidmap not owned by root", probe: helperProblem("newuidmap", hostFileInfo{name: "newuidmap", mode: 0755 | os.ModeSetuid, uid: 1000}), want: "owned by root"},
		{name: "newgidmap not owned by root", probe: helperProblem("newgidmap", hostFileInfo{name: "newgidmap", mode: 0755 | os.ModeSetuid, uid: 1000}), want: "owned by root"},
		{name: "newuidmap missing setuid", probe: helperProblem("newuidmap", hostFileInfo{name: "newuidmap", mode: 0755}), want: "setuid"},
		{name: "newgidmap missing setuid", probe: helperProblem("newgidmap", hostFileInfo{name: "newgidmap", mode: 0755}), want: "setuid"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateBuilderHost(tc.probe)
			if tc.want == "" && err != nil {
				t.Fatalf("valid host rejected: %v", err)
			}
			if tc.want != "" && (err == nil || !strings.Contains(err.Error(), tc.want)) {
				t.Fatalf("error = %v; want %q", err, tc.want)
			}
		})
	}
}

func TestValidateBuilderHostRequiresExactSubordinateRows(t *testing.T) {
	for _, row := range []string{
		"irgsh-builder:165536:65536:extra",
		"irgsh-builder: 165536:65536",
		"irgsh-builder:165536:65536 ",
		"irgsh-builder:start:65536",
		"irgsh-builder:165536:count",
	} {
		t.Run(row, func(t *testing.T) {
			probe := validHostProbe()
			probe.SubUID = []byte(row + "\n")
			if err := validateBuilderHost(probe); err == nil || !strings.Contains(err.Error(), "/etc/subuid") {
				t.Fatalf("row %q accepted: %v", row, err)
			}
		})
	}
}

func TestValidateBuilderHostChecksSubordinateNamespaceBoundary(t *testing.T) {
	cases := []struct {
		name  string
		row   string
		valid bool
	}{
		{name: "highest valid range", row: "irgsh-builder:4294901760:65536", valid: true},
		{name: "start exceeds uint32", row: "irgsh-builder:4294967296:65536"},
		{name: "inclusive end exceeds uint32", row: "irgsh-builder:4294901761:65536"},
		{name: "count overflows end", row: "irgsh-builder:1:18446744073709551615"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			probe := validHostProbe()
			probe.SubUID = []byte(tc.row + "\n")
			err := validateBuilderHost(probe)
			if tc.valid && err != nil {
				t.Fatalf("valid boundary rejected: %v", err)
			}
			if !tc.valid && (err == nil || !strings.Contains(err.Error(), "/etc/subuid")) {
				t.Fatalf("invalid boundary accepted: %v", err)
			}
		})
	}
}

func TestValidateBuilderHostReportsSubordinateNamespaceBoundary(t *testing.T) {
	probe := validHostProbe()
	probe.SubUID = []byte("irgsh-builder:4294901761:65536\n")
	err := validateBuilderHost(probe)
	if err == nil || !strings.Contains(err.Error(), "4294967295") {
		t.Fatalf("boundary remediation missing: %v", err)
	}
}

func builderMainCommand(t *testing.T, args ...string) *exec.Cmd {
	t.Helper()
	cfg := config.IrgshConfig{
		Redis: "redis://127.0.0.1:1",
		Chief: config.ChiefConfig{Address: "http://127.0.0.1:1"},
		Builder: config.BuilderConfig{
			Workdir:              filepath.Join(t.TempDir(), "builder"),
			DistCodename:         "verbeek",
			UpstreamDistCodename: "sid",
			UpstreamDistUrl:      "http://deb.debian.org/debian",
		},
	}
	contents, err := json.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(configPath, contents, 0600); err != nil {
		t.Fatal(err)
	}
	commandArgs := append([]string{"-c", configPath}, args...)
	cmd := exec.Command(os.Args[0], commandArgs...)
	cmd.Env = append(os.Environ(), "IRGSH_TEST_BUILDER_MAIN=1", "PATH="+t.TempDir())
	return cmd
}

func containsBuilderHostPreflightError(output string) bool {
	for _, message := range []string{
		"builder must run as a non-root account",
		"builder username is unavailable",
		"determine builder username",
		"read /etc/subuid",
		"read /etc/subgid",
		"required executable sbuild",
	} {
		if strings.Contains(output, message) {
			return true
		}
	}
	return false
}

func TestBuilderMainExitsOnHostPreflightFailure(t *testing.T) {
	for _, args := range [][]string{nil, {"init-base"}, {"update-base"}} {
		name := "worker"
		if len(args) != 0 {
			name = args[0]
		}
		t.Run(name, func(t *testing.T) {
			output, err := builderMainCommand(t, args...).CombinedOutput()
			var exitErr *exec.ExitError
			if !errors.As(err, &exitErr) || exitErr.ExitCode() != 1 {
				t.Fatalf("exit error = %v; output:\n%s", err, output)
			}
			if !containsBuilderHostPreflightError(string(output)) {
				t.Fatalf("preflight error missing:\n%s", output)
			}
		})
	}
}

func TestBuilderMainHelpSkipsHostPreflight(t *testing.T) {
	output, err := builderMainCommand(t, "--help").CombinedOutput()
	if err != nil {
		t.Fatalf("help failed: %v\n%s", err, output)
	}
	if containsBuilderHostPreflightError(string(output)) || !strings.Contains(string(output), "USAGE:") {
		t.Fatalf("unexpected help output:\n%s", output)
	}
}
