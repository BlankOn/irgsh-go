package gitref

import (
	"strings"
	"testing"
)

func TestValidBranch(t *testing.T) {
	for _, name := range []string{"main", "sinambung", "variant-gnome", "feature/iso_2", "release-2026.09", "a", strings.Repeat("a", 255)} {
		if !ValidBranch(name) {
			t.Errorf("ValidBranch(%q) = false", name)
		}
	}
	for _, name := range []string{"", "-main", ".hidden", "a..b", "a/", "/a", "a//b", "a.lock", "a.", "a b", "a;id", "a$b", "main\n", "a/.b", "a/-b", "ü", strings.Repeat("a", 256)} {
		if ValidBranch(name) {
			t.Errorf("ValidBranch(%q) = true", name)
		}
	}
}

func TestValidCommit(t *testing.T) {
	sha := "0123456789abcdef0123456789abcdef01234567"
	if !ValidCommit(sha) {
		t.Errorf("ValidCommit(%q) = false", sha)
	}
	for _, candidate := range []string{"", sha[:39], sha + "0", strings.ToUpper(sha), strings.Repeat("g", 40), sha + "\n", "0123456"} {
		if ValidCommit(candidate) {
			t.Errorf("ValidCommit(%q) = true", candidate)
		}
	}
}
