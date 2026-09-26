package main

import (
	"encoding/json"
	"slices"
	"testing"
)

func TestISOScriptPathUsesDefault(t *testing.T) {
	t.Setenv("IRGSH_ISO_SCRIPT", "")
	if got := isoScriptPath(); got != "/usr/share/irgsh/iso-build.sh" {
		t.Fatalf("isoScriptPath() = %q", got)
	}
}

func TestISOScriptPathUsesEnvironment(t *testing.T) {
	t.Setenv("IRGSH_ISO_SCRIPT", "/opt/irgsh/current/iso/share/iso-build.sh")
	if got := isoScriptPath(); got != "/opt/irgsh/current/iso/share/iso-build.sh" {
		t.Fatalf("isoScriptPath() = %q", got)
	}
}

const (
	testScript = "/usr/share/irgsh/iso-build.sh"
	testRepo   = "https://github.com/BlankOn/blankon-live-build.git"
	testCommit = "0123456789abcdef0123456789abcdef01234567"
)

func TestISOScriptArgs(t *testing.T) {
	got, err := isoScriptArgs(testScript, testRepo, ISOSubmission{Branch: "variant-gnome"})
	if want := []string{"-n", testScript, testRepo, "variant-gnome"}; err != nil || !slices.Equal(got, want) {
		t.Fatalf("args = %q, %v; want %q", got, err, want)
	}
	got, err = isoScriptArgs(testScript, testRepo, ISOSubmission{Branch: "feature/iso", Commit: testCommit})
	if want := []string{"-n", testScript, testRepo, "feature/iso", testCommit}; err != nil || !slices.Equal(got, want) {
		t.Fatalf("args = %q, %v; want %q", got, err, want)
	}
}

func TestISOScriptArgsRejectsUnsafeRevision(t *testing.T) {
	for _, submission := range []ISOSubmission{
		{Branch: ""},
		{Branch: "main;id"},
		{Branch: "-main"},
		{Branch: "$(id)"},
		{Branch: "main", Commit: "0123456"},
		{Branch: "main", Commit: "0123456789ABCDEF0123456789ABCDEF01234567"},
		{Branch: "main", Commit: testCommit + ";id"},
	} {
		if args, err := isoScriptArgs(testScript, testRepo, submission); err == nil {
			t.Errorf("isoScriptArgs(%+v) = %q", submission, args)
		}
	}
}

func TestISOSubmissionWithoutCommitBuildsBranchTip(t *testing.T) {
	var submission ISOSubmission
	if err := json.Unmarshal([]byte(`{"taskUUID":"t","dist":"verbeek","branch":"main","noCache":false}`), &submission); err != nil {
		t.Fatal(err)
	}
	got, err := isoScriptArgs(testScript, testRepo, submission)
	if want := []string{"-n", testScript, testRepo, "main"}; err != nil || !slices.Equal(got, want) {
		t.Fatalf("args = %q, %v; want %q", got, err, want)
	}
}
