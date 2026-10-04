package usecase

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeFormat(t *testing.T, format string) string {
	t.Helper()
	dir := t.TempDir()
	sourceDir := filepath.Join(dir, "debian", "source")
	if err := os.MkdirAll(sourceDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sourceDir, "format"), []byte(format), 0644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestCheckNativeVersion_NativeWithRevision(t *testing.T) {
	err := checkNativeVersion(writeFormat(t, "3.0 (native)\n"), "1")
	if err == nil {
		t.Fatal("a native package with a Debian revision must be rejected")
	}
	// The message has to name the fix, not just the symptom.
	for _, want := range []string{"native package version may not have a revision", "debian/changelog"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("expected %q in %q", want, err.Error())
		}
	}
}

func TestCheckNativeVersion_NativeWithoutRevision(t *testing.T) {
	if err := checkNativeVersion(writeFormat(t, "3.0 (native)\n"), ""); err != nil {
		t.Fatalf("a native package without a revision is valid: %v", err)
	}
}

func TestCheckNativeVersion_QuiltWithRevision(t *testing.T) {
	if err := checkNativeVersion(writeFormat(t, "3.0 (quilt)\n"), "1"); err != nil {
		t.Fatalf("a quilt package may carry a revision: %v", err)
	}
}

func TestCheckNativeVersion_NoFormatFile(t *testing.T) {
	if err := checkNativeVersion(t.TempDir(), "1"); err != nil {
		t.Fatalf("a missing debian/source/format is left to dpkg-source: %v", err)
	}
}
