package main

import "testing"

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
