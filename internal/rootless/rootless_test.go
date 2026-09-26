package rootless

import (
	"os"
	"strings"
	"syscall"
	"testing"
	"time"
)

type fileInfo struct {
	mode os.FileMode
	sys  any
}

func (info fileInfo) Name() string       { return "helper" }
func (info fileInfo) Size() int64        { return 0 }
func (info fileInfo) Mode() os.FileMode  { return info.mode }
func (info fileInfo) ModTime() time.Time { return time.Time{} }
func (info fileInfo) IsDir() bool        { return false }
func (info fileInfo) Sys() any           { return info.sys }

func TestHasSubordinateRange(t *testing.T) {
	for _, tc := range []struct {
		rows  string
		valid bool
	}{
		{"irgsh-iso:589824:65536\n", true},
		{"other:100000:65536\nirgsh-iso:589824:65536\n", true},
		{"irgsh-iso:4294901759:65536\n", true},
		{"irgsh-iso:4294901760:65536\n", false},
		{"irgsh-iso:589824:65535\n", false},
		{"irgsh-iso:589824:65536:extra\n", false},
		{"irgsh-iso:start:65536\n", false},
		{"irgsh-iso:589824:1\nirgsh-iso:655360:65536\n", false},
		{"irgsh-iso-extra:589824:65536\n", false},
		{"", false},
	} {
		if got := HasSubordinateRange([]byte(tc.rows), "irgsh-iso"); got != tc.valid {
			t.Errorf("HasSubordinateRange(%q) = %v, want %v", tc.rows, got, tc.valid)
		}
	}
}

func TestCheckMappingHelper(t *testing.T) {
	if err := CheckMappingHelper("newuidmap", fileInfo{mode: 0755 | os.ModeSetuid, sys: &syscall.Stat_t{Uid: 0}}); err != nil {
		t.Fatalf("valid helper rejected: %v", err)
	}
	for _, tc := range []struct {
		info fileInfo
		want string
	}{
		{fileInfo{mode: 0755 | os.ModeSetuid, sys: &syscall.Stat_t{Uid: 1000}}, "newuidmap mapping helper must be owned by root"},
		{fileInfo{mode: 0755 | os.ModeSetuid}, "newuidmap mapping helper must be owned by root"},
		{fileInfo{mode: 0755, sys: &syscall.Stat_t{Uid: 0}}, "newuidmap mapping helper needs the setuid bit"},
	} {
		if err := CheckMappingHelper("newuidmap", tc.info); err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("CheckMappingHelper(%+v) = %v, want %q", tc.info, err, tc.want)
		}
	}
}
