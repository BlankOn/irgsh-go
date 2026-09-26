package main

import (
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"strings"

	"github.com/blankon/irgsh-go/internal/rootless"
)

type hostProbe struct {
	EUID       int
	Username   string
	SubUID     []byte
	SubGID     []byte
	LookupPath func(string) (string, error)
	Stat       func(string) (os.FileInfo, error)
	Output     func(string, ...string) ([]byte, error)
}

func validateBuilderHost(probe hostProbe) error {
	if probe.EUID == 0 {
		return fmt.Errorf("builder must run as a non-root account; use the dedicated irgsh-builder user")
	}
	if probe.Username == "" {
		return fmt.Errorf("builder username is unavailable; use a local account with /etc/subuid and /etc/subgid entries")
	}
	paths := make(map[string]string, 2)
	for _, name := range []string{"sbuild", "mmdebstrap", "dpkg", "newuidmap", "newgidmap"} {
		path, err := probe.LookupPath(name)
		if err != nil {
			return fmt.Errorf("required executable %s is not on PATH; install it before starting irgsh-builder", name)
		}
		paths[name] = path
	}
	output, err := probe.Output(paths["sbuild"], "--version")
	if err != nil {
		return fmt.Errorf("read sbuild version; install sbuild >= 0.87.0 with a valid configuration: %w", err)
	}
	version := ""
	for _, line := range strings.Split(string(output), "\n") {
		if value, ok := strings.CutPrefix(line, "sbuild (Debian sbuild) "); ok {
			if fields := strings.Fields(value); len(fields) > 0 {
				version = fields[0]
			}
			break
		}
	}
	if version == "" {
		return fmt.Errorf("read sbuild version; expected the sbuild version banner, requiring sbuild >= 0.87.0")
	}
	if _, err := probe.Output(paths["dpkg"], "--compare-versions", version, "ge", "0.87.0"); err != nil {
		return fmt.Errorf("sbuild >= 0.87.0 is required for unshare_mmdebstrap_auto_create; install a supported version: %w", err)
	}
	if !rootless.HasSubordinateRange(probe.SubUID, probe.Username) {
		return fmt.Errorf("/etc/subuid needs its first %s row in exact <account>:<start>:<count> form with at least 65536 IDs ending at or below 4294967294; provision the builder subordinate UID range", probe.Username)
	}
	if !rootless.HasSubordinateRange(probe.SubGID, probe.Username) {
		return fmt.Errorf("/etc/subgid needs its first %s row in exact <account>:<start>:<count> form with at least 65536 IDs ending at or below 4294967294; provision the builder subordinate GID range", probe.Username)
	}
	for _, name := range []string{"newuidmap", "newgidmap"} {
		info, err := probe.Stat(paths[name])
		if err != nil {
			return fmt.Errorf("inspect %s mapping helper; reinstall the uidmap package: %w", name, err)
		}
		if err := rootless.CheckMappingHelper(name, info); err != nil {
			return err
		}
	}
	return nil
}

func validateCurrentBuilderHost() error {
	euid := os.Geteuid()
	if euid == 0 {
		return validateBuilderHost(hostProbe{EUID: euid})
	}
	current, err := user.Current()
	if err != nil {
		return fmt.Errorf("determine builder username; run under a local account with subordinate ID mappings: %w", err)
	}
	subUID, err := os.ReadFile("/etc/subuid")
	if err != nil {
		return fmt.Errorf("read /etc/subuid; provision the builder subordinate UID range: %w", err)
	}
	subGID, err := os.ReadFile("/etc/subgid")
	if err != nil {
		return fmt.Errorf("read /etc/subgid; provision the builder subordinate GID range: %w", err)
	}
	return validateBuilderHost(hostProbe{
		EUID:       euid,
		Username:   current.Username,
		SubUID:     subUID,
		SubGID:     subGID,
		LookupPath: exec.LookPath,
		Stat:       os.Stat,
		Output: func(path string, args ...string) ([]byte, error) {
			return exec.Command(path, args...).Output()
		},
	})
}
