package main

import (
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/blankon/irgsh-go/internal/config"
)

func TestMain(m *testing.M) {
	if os.Getenv("IRGSH_TEST_ISO_MAIN") == "1" {
		main()
		return
	}
	os.Exit(m.Run())
}

type isoFileInfo struct {
	mode os.FileMode
	uid  uint32
}

func (info isoFileInfo) Name() string       { return "helper" }
func (info isoFileInfo) Size() int64        { return 0 }
func (info isoFileInfo) Mode() os.FileMode  { return info.mode }
func (info isoFileInfo) ModTime() time.Time { return time.Time{} }
func (info isoFileInfo) IsDir() bool        { return false }
func (info isoFileInfo) Sys() any           { return &syscall.Stat_t{Uid: info.uid} }

const supportedChrootSysfs = "\t\tmount -t sysfs -o x-gvfs-hide sysfs-live chroot/sys || mount -o rbind /sys chroot/sys\n\t\tumount -l chroot/sys\n"

const namespaceProbe = "/usr/bin/unshare --map-auto --map-root-user --mount --pid --fork --kill-child --mount-proc true"

func validISOHostProbe() (isoHostProbe, *[]string) {
	ran := []string{}
	return isoHostProbe{
		EUID:     1000,
		Username: "irgsh-iso",
		SubUID:   []byte("other:100000:65536\nirgsh-iso:589824:65536\n"),
		SubGID:   []byte("irgsh-iso:589824:65536\n"),
		LookupPath: func(name string) (string, error) {
			return filepath.Join("/usr/bin", name), nil
		},
		Stat: func(string) (os.FileInfo, error) {
			return isoFileInfo{mode: 0755 | os.ModeSetuid}, nil
		},
		ReadFile: func(path string) ([]byte, error) {
			if path != liveBuildSysfs {
				return nil, os.ErrNotExist
			}
			return []byte(supportedChrootSysfs), nil
		},
		Run: func(path string, args ...string) error {
			ran = append(ran, strings.Join(append([]string{path}, args...), " "))
			return nil
		},
	}, &ran
}

type isoHostCase struct {
	name   string
	change func(*isoHostProbe)
	want   string
}

func TestValidateISOHost(t *testing.T) {
	sysfs := func(contents string) func(*isoHostProbe) {
		return func(probe *isoHostProbe) {
			probe.ReadFile = func(string) ([]byte, error) { return []byte(contents), nil }
		}
	}
	cases := []isoHostCase{
		{"valid host", func(*isoHostProbe) {}, ""},
		{"root EUID", func(probe *isoHostProbe) { probe.EUID = 0 }, "non-root"},
		{"missing username", func(probe *isoHostProbe) { probe.Username = "" }, "username"},
		{"missing subordinate UID row", func(probe *isoHostProbe) { probe.SubUID = []byte("other:100000:65536\n") }, "/etc/subuid"},
		{"short subordinate GID range", func(probe *isoHostProbe) { probe.SubGID = []byte("irgsh-iso:589824:65535\n") }, "/etc/subgid"},
		{"helper not owned by root", func(probe *isoHostProbe) {
			probe.Stat = func(string) (os.FileInfo, error) { return isoFileInfo{mode: 0755 | os.ModeSetuid, uid: 1000}, nil }
		}, "owned by root"},
		{"helper without setuid", func(probe *isoHostProbe) {
			probe.Stat = func(string) (os.FileInfo, error) { return isoFileInfo{mode: 0755}, nil }
		}, "setuid"},
		{"live-build without sysfs fallback", sysfs("\t\tmount -t sysfs -o x-gvfs-hide sysfs-live chroot/sys\n\t\tumount -l chroot/sys\n"), liveBuildSysfs},
		{"live-build without lazy teardown", sysfs("\t\tmount -t sysfs -o x-gvfs-hide sysfs-live chroot/sys || mount -o rbind /sys chroot/sys\n\t\tumount chroot/sys\n"), liveBuildSysfs},
		{"live-build missing", func(probe *isoHostProbe) {
			probe.ReadFile = func(string) ([]byte, error) { return nil, os.ErrNotExist }
		}, liveBuildSysfs},
		{"namespaces denied", func(probe *isoHostProbe) {
			probe.Run = func(string, ...string) error { return errors.New("exit status 1") }
		}, "kernel.apparmor_restrict_unprivileged_userns"},
	}
	for _, tool := range []string{"unshare", "lb", "git", "zsyncmake", "flock", "newuidmap", "newgidmap"} {
		cases = append(cases, isoHostCase{"missing " + tool, func(probe *isoHostProbe) {
			probe.LookupPath = func(name string) (string, error) {
				if name == tool {
					return "", errors.New("not found")
				}
				return filepath.Join("/usr/bin", name), nil
			}
		}, "required executable " + tool})
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			probe, ran := validISOHostProbe()
			tc.change(&probe)
			err := validateISOHost(probe)
			if tc.want == "" {
				if err != nil {
					t.Fatalf("valid host rejected: %v", err)
				}
				if want := []string{namespaceProbe}; !slices.Equal(*ran, want) {
					t.Fatalf("namespace probe = %q, want %q", *ran, want)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v; want %q", err, tc.want)
			}
		})
	}
}

func TestISOMainExitsOnHostPreflightFailure(t *testing.T) {
	cfg := config.IrgshConfig{
		Redis: "redis://127.0.0.1:1",
		Chief: config.ChiefConfig{Address: "http://127.0.0.1:1"},
		ISO: config.ISOConfig{
			Workdir:      filepath.Join(t.TempDir(), "iso"),
			Outputdir:    filepath.Join(t.TempDir(), "out"),
			DistCodename: "verbeek",
			RepoURL:      "https://github.com/BlankOn/blankon-live-build.git",
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
	cmd := exec.Command(os.Args[0], "-c", configPath)
	cmd.Env = append(os.Environ(), "IRGSH_TEST_ISO_MAIN=1", "PATH="+t.TempDir())
	output, err := cmd.CombinedOutput()
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) || exitErr.ExitCode() != 1 {
		t.Fatalf("exit error = %v; output:\n%s", err, output)
	}
	if info, err := os.Stat(cfg.ISO.Workdir); err != nil || info.Mode().Perm() != 0700 {
		t.Fatalf("workdir mode = %v, %v; want 0700", info, err)
	}
	for _, message := range []string{
		"irgsh-iso must run as a non-root account",
		"determine irgsh-iso username",
		"read /etc/subuid",
		"read /etc/subgid",
		"required executable unshare",
	} {
		if strings.Contains(string(output), message) {
			return
		}
	}
	t.Fatalf("preflight error missing:\n%s", output)
}
