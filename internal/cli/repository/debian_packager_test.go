package repository

import (
	"errors"
	"testing"
)

// stubShell returns canned output/errors for the command it is given.
type stubShell struct {
	out        string
	err        error
	lastCmd    string
	interError error
}

func (s *stubShell) Output(cmd string) (string, error) {
	s.lastCmd = cmd
	return s.out, s.err
}

func (s *stubShell) RunInteractive(cmd string) error {
	s.lastCmd = cmd
	return s.interError
}

func TestCheckBuildDeps_Satisfied(t *testing.T) {
	shell := &stubShell{}
	missing, err := NewShellDebianPackager(shell).CheckBuildDeps("/tmp/pkg")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if missing != "" {
		t.Fatalf("expected no missing deps, got %q", missing)
	}
}

func TestCheckBuildDeps_Unmet(t *testing.T) {
	shell := &stubShell{
		out: "dpkg-checkbuilddeps: error: unmet build dependencies: inkscape optipng\n",
		err: errors.New("exit status 1"),
	}
	missing, err := NewShellDebianPackager(shell).CheckBuildDeps("/tmp/pkg")
	if err != nil {
		t.Fatalf("unmet deps must not be reported as an error: %v", err)
	}
	if missing != "inkscape optipng" {
		t.Fatalf("expected %q, got %q", "inkscape optipng", missing)
	}
}

func TestCheckBuildDeps_OtherFailure(t *testing.T) {
	shell := &stubShell{
		out: "dpkg-checkbuilddeps: error: cannot read debian/control\n",
		err: errors.New("exit status 2"),
	}
	missing, err := NewShellDebianPackager(shell).CheckBuildDeps("/tmp/pkg")
	if err == nil {
		t.Fatal("expected an error for a non-dependency failure")
	}
	if missing != "" {
		t.Fatalf("expected no missing deps, got %q", missing)
	}
}
