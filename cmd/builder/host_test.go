package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
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
