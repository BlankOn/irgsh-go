package main

import (
	"bufio"
	"fmt"
	"net/textproto"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/blankon/irgsh-go/internal/config"
)

func distributionHeaders(t *testing.T, contents string) []textproto.MIMEHeader {
	t.Helper()
	var lines []string
	for _, line := range strings.Split(contents, "\n") {
		if !strings.HasPrefix(strings.TrimSpace(line), "#") {
			lines = append(lines, line)
		}
	}
	var headers []textproto.MIMEHeader
	for _, stanza := range strings.Split(strings.Join(lines, "\n"), "\n\n") {
		if strings.TrimSpace(stanza) == "" {
			continue
		}
		header, err := textproto.NewReader(bufio.NewReader(strings.NewReader(strings.TrimLeft(stanza, "\r\n") + "\n\n"))).ReadMIMEHeader()
		if err != nil {
			t.Fatal(err)
		}
		headers = append(headers, header)
	}
	return headers
}

func TestRepoTemplateEnablesUdebsForAllComponents(t *testing.T) {
	template, err := os.ReadFile("../../utils/reprepro-template/conf/distributions.orig")
	if err != nil {
		t.Fatal(err)
	}
	components := "main restricted extras restricted-firmware"
	contents := strings.NewReplacer("DIST_COMPONENTS", components, "DIST_CODENAME", "verbeek").Replace(string(template))
	headers := distributionHeaders(t, contents)
	if len(headers) != 3 {
		t.Fatalf("template has %d distributions", len(headers))
	}
	for _, header := range headers {
		for _, key := range []string{"UDebComponents", "ContentsUComponents"} {
			if got := strings.TrimSpace(header.Get(key)); got != components {
				t.Errorf("%s %s = %q; want %q", header.Get("Codename"), key, got, components)
			}
		}
	}
}

func udebRepoFixture(t *testing.T) config.RepoConfig {
	t.Helper()
	return config.RepoConfig{Workdir: t.TempDir(), DistCodename: "verbeek", DistComponents: "main restricted extras restricted-firmware"}
}

func writeDistributionFixture(t *testing.T, repo config.RepoConfig, suffix, contents string) string {
	t.Helper()
	path := filepath.Join(repo.Workdir, repo.DistCodename+suffix, "conf", "distributions")
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(contents), 0640); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestMigrateUDebComponentsUpdatesBothRootsAtomicallyAndIdempotently(t *testing.T) {
	repo := udebRepoFixture(t)
	for _, suffix := range []string{"", "-experimental"} {
		var old strings.Builder
		old.WriteString("# operator settings\n\n")
		for _, suite := range []string{"", "-security", "-updates"} {
			fmt.Fprintf(&old, "Codename: verbeek%s%s\nComponents: %s\nArchitectures: amd64 source\nUDebComponents: main\nContentsUComponents: main\nDescription: keep: this value\n continued description\nLog: custom.log\n --type=dsc custom-hook.sh\n# keep this note\n\n", suffix, suite, repo.DistComponents)
		}
		old.WriteString("Codename: private\nComponents: special\nArchitectures: all\nUDebComponents: special\nContentsUComponents: special\n")
		path := writeDistributionFixture(t, repo, suffix, old.String())
		opened, err := os.Open(path)
		if err != nil {
			t.Fatal(err)
		}
		defer opened.Close()
		t.Cleanup(func() {
			contents, err := os.ReadFile(path)
			want := strings.ReplaceAll(strings.ReplaceAll(old.String(), "UDebComponents: main\n", "UDebComponents: "+repo.DistComponents+"\n"), "ContentsUComponents: main\n", "ContentsUComponents: "+repo.DistComponents+"\n")
			if err != nil || string(contents) != want {
				t.Errorf("migrated config differs: %q, %v; want %q", contents, err, want)
			}
		})
		if err := migrateUDebComponents(repo); err != nil {
			t.Fatal(err)
		}
		info, err := os.Stat(path)
		if err != nil || info.Mode().Perm() != 0640 {
			t.Fatalf("migrated mode: %v, %v", info, err)
		}
		original, err := opened.Stat()
		if err != nil || os.SameFile(original, info) {
			t.Fatalf("config was overwritten in place: %v", err)
		}
		buffer := make([]byte, original.Size())
		if _, err := opened.ReadAt(buffer, 0); err != nil || string(buffer) != old.String() {
			t.Fatalf("open config saw partial migration: %q, %v", buffer, err)
		}
		if err := migrateUDebComponents(repo); err != nil {
			t.Fatal(err)
		}
		again, err := os.Stat(path)
		if err != nil || !os.SameFile(info, again) {
			t.Fatalf("idempotent migration rewrote config: %v", err)
		}
		entries, err := os.ReadDir(filepath.Dir(path))
		if err != nil || len(entries) != 1 {
			t.Fatalf("migration left temporary files: %v, %v", entries, err)
		}
	}
}

func TestMigrateUDebComponentsAddsMissingFieldsAndReplacesContinuations(t *testing.T) {
	repo := udebRepoFixture(t)
	old := "Codename: verbeek\nComponents: main restricted\n extras restricted-firmware\nArchitectures: amd64 source\nUDebComponents: main\n restricted\n# keep comment\nDescription: keep\n"
	path := writeDistributionFixture(t, repo, "", old)
	if err := migrateUDebComponents(repo); err != nil {
		t.Fatal(err)
	}
	contents, err := os.ReadFile(path)
	want := "Codename: verbeek\nComponents: main restricted\n extras restricted-firmware\nArchitectures: amd64 source\nUDebComponents: " + repo.DistComponents + "\n# keep comment\nDescription: keep\nContentsUComponents: " + repo.DistComponents + "\n"
	if err != nil || string(contents) != want {
		t.Fatalf("config = %q, %v; want %q", contents, err, want)
	}
}

func TestMigrateUDebComponentsSkipsUninitializedRepositories(t *testing.T) {
	repo := udebRepoFixture(t)
	if err := migrateUDebComponents(repo); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(repo.Workdir)
	if err != nil || len(entries) != 0 {
		t.Fatalf("created repository state: %v, %v", entries, err)
	}
}

func TestMigrateUDebComponentsRejectsMalformedConfigBeforeChangingEitherRoot(t *testing.T) {
	for name, malformed := range map[string]string{
		"empty": "", "missing colon": "Codename verbeek-experimental\n",
		"missing codename":             "Components: main\nArchitectures: amd64\n",
		"missing components":           "Codename: verbeek-experimental\nArchitectures: amd64\n",
		"missing architecture":         "Codename: verbeek-experimental\nComponents: main\n",
		"orphan continuation":          " orphan\nCodename: verbeek-experimental\n",
		"duplicate field":              "Codename: verbeek-experimental\nCodename: verbeek-experimental\nComponents: main\nArchitectures: amd64\n",
		"wrong codename":               "Codename: elsewhere\nComponents: main\nArchitectures: amd64\n",
		"missing configured component": "Codename: verbeek-experimental\nComponents: main\nArchitectures: amd64\n",
	} {
		t.Run(name, func(t *testing.T) {
			repo := udebRepoFixture(t)
			original := "Codename: verbeek\nComponents: " + repo.DistComponents + "\nArchitectures: amd64 source\nUDebComponents: main\nContentsUComponents: main\n"
			normal := writeDistributionFixture(t, repo, "", original)
			experimental := writeDistributionFixture(t, repo, "-experimental", malformed)
			if err := migrateUDebComponents(repo); err == nil || !strings.Contains(err.Error(), experimental) {
				t.Fatalf("malformed migration error = %v", err)
			}
			for path, want := range map[string]string{normal: original, experimental: malformed} {
				contents, err := os.ReadFile(path)
				if err != nil || string(contents) != want {
					t.Fatalf("failed migration changed %s: %q, %v", path, contents, err)
				}
			}
		})
	}
}

func TestMigrateUDebComponentsRejectsMissingOrNonRegularInitializedConfig(t *testing.T) {
	for _, kind := range []string{"missing", "directory", "symlink"} {
		t.Run(kind, func(t *testing.T) {
			repo := udebRepoFixture(t)
			path := writeDistributionFixture(t, repo, "", "original")
			if err := os.Remove(path); err != nil {
				t.Fatal(err)
			}
			switch kind {
			case "directory":
				if err := os.Mkdir(path, 0755); err != nil {
					t.Fatal(err)
				}
			case "symlink":
				if err := os.Symlink("missing", path); err != nil {
					t.Fatal(err)
				}
			}
			if err := migrateUDebComponents(repo); err == nil || !strings.Contains(err.Error(), path) {
				t.Fatalf("accepted %s config: %v", kind, err)
			}
		})
	}
}

func TestMigrateUDebComponentsRejectsUnsafeConfiguration(t *testing.T) {
	for _, tc := range []struct{ dist, components string }{{"../escape", "main"}, {"verbeek", ""}, {"verbeek", "main invalid:field"}, {"verbeek", "main main"}} {
		repo := udebRepoFixture(t)
		repo.DistCodename, repo.DistComponents = tc.dist, tc.components
		if err := migrateUDebComponents(repo); err == nil {
			t.Errorf("accepted config %+v", tc)
		}
	}
}
