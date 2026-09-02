package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCollectKeyrings_FindsKeyringsInEveryDirectory(t *testing.T) {
	base := t.TempDir()
	// A derivative worker: its own key in /etc/apt/trusted.gpg.d, the Debian
	// archive keys only in /usr/share/keyrings.
	aptDir := filepath.Join(base, "etc")
	shareDir := filepath.Join(base, "share")
	for _, dir := range []string{aptDir, shareDir} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
	}
	write := func(dir, name string) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("key"), 0644); err != nil {
			t.Fatal(err)
		}
	}
	write(aptDir, "blankon.key.chroot.gpg")
	write(shareDir, "debian-archive-keyring.gpg")
	write(shareDir, "debian-archive-trixie-automatic.asc")
	write(shareDir, "debian-archive-bookworm-automatic.pgp") // apt cannot read .pgp
	write(shareDir, "README")

	dest := filepath.Join(base, "sandbox")
	linked, err := collectKeyrings([]string{aptDir, shareDir, "/nonexistent"}, "", dest)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if linked != 3 {
		t.Fatalf("expected 3 usable keyrings, got %d", linked)
	}

	entries, _ := os.ReadDir(dest)
	var names []string
	for _, entry := range entries {
		names = append(names, entry.Name())
	}
	joined := strings.Join(names, " ")
	for _, want := range []string{"blankon.key.chroot.gpg", "debian-archive-keyring.gpg", "debian-archive-trixie-automatic.asc"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("expected %s to be linked, got %v", want, names)
		}
	}
	if strings.Contains(joined, ".pgp") || strings.Contains(joined, "README") {
		t.Fatalf("only .gpg and .asc keyrings may be linked, got %v", names)
	}
}

func TestCollectKeyrings_SameNameInTwoDirectories(t *testing.T) {
	base := t.TempDir()
	first := filepath.Join(base, "a")
	second := filepath.Join(base, "b")
	for _, dir := range []string{first, second} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "debian-archive-keyring.gpg"), []byte("key"), 0644); err != nil {
			t.Fatal(err)
		}
	}

	dest := filepath.Join(base, "sandbox")
	linked, err := collectKeyrings([]string{first, second}, "", dest)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if linked != 2 {
		t.Fatalf("a name collision must not drop a keyring: linked %d", linked)
	}
}

func TestCollectKeyrings_ExplicitKeyring(t *testing.T) {
	base := t.TempDir()
	keyring := filepath.Join(base, "custom-archive.gpg")
	if err := os.WriteFile(keyring, []byte("key"), 0644); err != nil {
		t.Fatal(err)
	}

	linked, err := collectKeyrings(nil, keyring, filepath.Join(base, "sandbox"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if linked != 1 {
		t.Fatalf("expected the explicit keyring to be linked, got %d", linked)
	}

	// A keyring the worker does not have must fail loudly rather than
	// silently falling back to no verification.
	if _, err := collectKeyrings(nil, filepath.Join(base, "missing.gpg"), filepath.Join(base, "sandbox2")); err == nil {
		t.Fatal("expected an error for a keyring that is not on the worker")
	}
	if _, err := collectKeyrings(nil, filepath.Join(base, "custom-archive.txt"), filepath.Join(base, "sandbox3")); err == nil {
		t.Fatal("expected an error for a keyring apt cannot read")
	}
}

func TestSignatureHint(t *testing.T) {
	aptOutput := `Get:1 https://kartolo.sby.datautama.net.id/debian sid InRelease [193 kB]
Err:1 https://kartolo.sby.datautama.net.id/debian sid InRelease
  The following signatures couldn't be verified because the public key is not available: NO_PUBKEY 6ED0E7B82643E131 NO_PUBKEY 78DBA3BC47EF2265
E: The repository 'https://kartolo.sby.datautama.net.id/debian sid InRelease' is not signed.`

	hint := signatureHint(aptOutput)
	if hint == "" {
		t.Fatal("a signature failure must produce a hint")
	}
	for _, want := range []string{"6ED0E7B82643E131", "78DBA3BC47EF2265", "debian-archive-keyring", "--keyring", "--insecure"} {
		if !strings.Contains(hint, want) {
			t.Fatalf("expected %q in the hint:\n%s", want, hint)
		}
	}
}

func TestSignatureHint_UnrelatedFailure(t *testing.T) {
	if hint := signatureHint("E: Could not resolve host: kartolo.sby.datautama.net.id"); hint != "" {
		t.Fatalf("a non-signature failure must not produce a signature hint: %s", hint)
	}
}

func TestMissingKeyIDs_Deduplicates(t *testing.T) {
	keys := missingKeyIDs("NO_PUBKEY ABC NO_PUBKEY ABC NO_PUBKEY DEF")
	if len(keys) != 2 || keys[0] != "ABC" || keys[1] != "DEF" {
		t.Fatalf("expected [ABC DEF], got %v", keys)
	}
}
