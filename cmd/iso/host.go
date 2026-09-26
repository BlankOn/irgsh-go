package main

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"os/user"

	"github.com/blankon/irgsh-go/internal/rootless"
)

const liveBuildSysfs = "/usr/lib/live/build/chroot_sysfs"

type isoHostProbe struct {
	EUID       int
	Username   string
	SubUID     []byte
	SubGID     []byte
	LookupPath func(string) (string, error)
	Stat       func(string) (os.FileInfo, error)
	ReadFile   func(string) ([]byte, error)
	Run        func(string, ...string) error
}

func validateISOHost(probe isoHostProbe) error {
	if probe.EUID == 0 {
		return fmt.Errorf("irgsh-iso must run as a non-root account; use the dedicated irgsh-iso user")
	}
	if probe.Username == "" {
		return fmt.Errorf("irgsh-iso username is unavailable; use a local account with /etc/subuid and /etc/subgid entries")
	}
	paths := make(map[string]string)
	for _, name := range []string{"unshare", "lb", "git", "zsyncmake", "flock", "newuidmap", "newgidmap"} {
		path, err := probe.LookupPath(name)
		if err != nil {
			return fmt.Errorf("required executable %s is not on PATH; install it before starting irgsh-iso", name)
		}
		paths[name] = path
	}
	if !rootless.HasSubordinateRange(probe.SubUID, probe.Username) {
		return fmt.Errorf("/etc/subuid needs its first %s row in exact <account>:<start>:<count> form with at least 65536 IDs ending at or below 4294967294; provision the ISO subordinate UID range", probe.Username)
	}
	if !rootless.HasSubordinateRange(probe.SubGID, probe.Username) {
		return fmt.Errorf("/etc/subgid needs its first %s row in exact <account>:<start>:<count> form with at least 65536 IDs ending at or below 4294967294; provision the ISO subordinate GID range", probe.Username)
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
	sysfs, err := probe.ReadFile(liveBuildSysfs)
	if err != nil {
		return fmt.Errorf("read %s; install a live-build that supports unprivileged builds: %w", liveBuildSysfs, err)
	}
	if !bytes.Contains(sysfs, []byte("mount -o rbind /sys chroot/sys")) || !bytes.Contains(sysfs, []byte("umount -l chroot/sys")) {
		return fmt.Errorf("%s must fall back to mount -o rbind /sys chroot/sys and detach it with umount -l chroot/sys; install the BlankOn live-build package that supports unprivileged builds", liveBuildSysfs)
	}
	if err := probe.Run(paths["unshare"], "--map-auto", "--map-root-user", "--mount", "--pid", "--fork", "--kill-child", "--mount-proc", "true"); err != nil {
		return fmt.Errorf("create an unprivileged user, mount, and PID namespace; allow unprivileged user namespaces (on Ubuntu 24.04 set kernel.apparmor_restrict_unprivileged_userns=0): %w", err)
	}
	return nil
}

func validateCurrentISOHost() error {
	euid := os.Geteuid()
	if euid == 0 {
		return validateISOHost(isoHostProbe{EUID: euid})
	}
	current, err := user.Current()
	if err != nil {
		return fmt.Errorf("determine irgsh-iso username; run under a local account with subordinate ID mappings: %w", err)
	}
	subUID, err := os.ReadFile("/etc/subuid")
	if err != nil {
		return fmt.Errorf("read /etc/subuid; provision the ISO subordinate UID range: %w", err)
	}
	subGID, err := os.ReadFile("/etc/subgid")
	if err != nil {
		return fmt.Errorf("read /etc/subgid; provision the ISO subordinate GID range: %w", err)
	}
	return validateISOHost(isoHostProbe{
		EUID:       euid,
		Username:   current.Username,
		SubUID:     subUID,
		SubGID:     subGID,
		LookupPath: exec.LookPath,
		Stat:       os.Stat,
		ReadFile:   os.ReadFile,
		Run: func(path string, args ...string) error {
			return exec.Command(path, args...).Run()
		},
	})
}
