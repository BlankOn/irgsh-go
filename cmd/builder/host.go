package main

import (
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"strconv"
	"strings"
	"syscall"
)

type hostProbe struct {
	EUID       int
	Username   string
	SubUID     []byte
	SubGID     []byte
	LookupPath func(string) (string, error)
	Stat       func(string) (os.FileInfo, error)
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
	if !hasSubordinateRange(probe.SubUID, probe.Username) {
		return fmt.Errorf("/etc/subuid needs an exact %s:<start>:<count> row with at least 65536 IDs ending at or below 4294967295; provision the builder subordinate UID range", probe.Username)
	}
	if !hasSubordinateRange(probe.SubGID, probe.Username) {
		return fmt.Errorf("/etc/subgid needs an exact %s:<start>:<count> row with at least 65536 IDs ending at or below 4294967295; provision the builder subordinate GID range", probe.Username)
	}
	for _, name := range []string{"newuidmap", "newgidmap"} {
		info, err := probe.Stat(paths[name])
		if err != nil {
			return fmt.Errorf("inspect %s mapping helper; reinstall the uidmap package: %w", name, err)
		}
		stat, ok := info.Sys().(*syscall.Stat_t)
		if !ok || stat.Uid != 0 {
			return fmt.Errorf("%s mapping helper must be owned by root; restore permissions with the uidmap package", name)
		}
		if info.Mode()&os.ModeSetuid == 0 {
			return fmt.Errorf("%s mapping helper needs the setuid bit; restore permissions with the uidmap package", name)
		}
	}
	return nil
}

func hasSubordinateRange(contents []byte, username string) bool {
	for _, line := range strings.Split(string(contents), "\n") {
		fields := strings.Split(line, ":")
		if len(fields) != 3 || fields[0] != username {
			continue
		}
		if fields[1] == "" || fields[2] == "" {
			continue
		}
		start, err := strconv.ParseUint(fields[1], 10, 64)
		if err != nil {
			continue
		}
		count, err := strconv.ParseUint(fields[2], 10, 64)
		const maxID = uint64(1<<32 - 1)
		if err == nil && count >= 65536 && start <= maxID && count-1 <= maxID-start {
			return true
		}
	}
	return false
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
	})
}
