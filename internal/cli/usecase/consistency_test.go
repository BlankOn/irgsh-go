package usecase

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeChangelog(t *testing.T, content string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "changelog")
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestCheckChangelogSourceName(t *testing.T) {
	tests := []struct {
		name      string
		changelog string
		control   string
		wantErr   string
	}{
		{"match", "hello (1.0-1) unstable; urgency=low\n", "hello", ""},
		{"mismatch", "hallo (1.0-1) unstable; urgency=low\n", "hello", `"hallo"`},
		{"no version parentheses", "hello unstable\n", "hello", ""},
		{"empty", "", "hello", "empty"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := checkChangelogSourceName(writeChangelog(t, tt.changelog), tt.control)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("error = %v, want it to contain %q", err, tt.wantErr)
			}
		})
	}
}

func TestCheckChangelogSourceName_MissingFile(t *testing.T) {
	if err := checkChangelogSourceName(filepath.Join(t.TempDir(), "nope"), "hello"); err == nil {
		t.Fatal("expected an error for a missing changelog")
	}
}

func TestCheckTarballVersion(t *testing.T) {
	tests := []struct {
		name    string
		url     string
		version string
		wantErr bool
	}{
		{"plain", "https://example.org/hello-1.2.3.tar.gz", "1.2.3", false},
		{"orig name", "https://example.org/dl/hello_1.2.3.orig.tar.xz", "1.2.3", false},
		{"v prefix", "https://github.com/o/r/archive/v1.2.3.tar.gz", "1.2.3", false},
		{"repack suffix", "https://example.org/hello-1.2.3.tar.xz", "1.2.3+dfsg", false},
		{"stale tarball", "https://example.org/hello-1.2.2.tar.gz", "1.2.3", true},
		{"query string ignored", "https://example.org/hello-1.2.2.tar.gz?v=1.2.3", "1.2.3", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := checkTarballVersion(tt.url, tt.version)
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
