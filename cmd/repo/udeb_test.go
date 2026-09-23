package main

import (
	"bufio"
	"fmt"
	"net/textproto"
	"os"
	"path/filepath"
	"strings"
	"syscall"
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
	for _, tc := range []struct{ dist, components string }{{"../escape", "main"}, {"verbeek/updates", "main"}, {"verbeek", ""}, {"verbeek", "main invalid:field"}, {"verbeek", "main main"}} {
		repo := udebRepoFixture(t)
		repo.DistCodename, repo.DistComponents = tc.dist, tc.components
		if err := migrateUDebComponents(repo); err == nil {
			t.Errorf("accepted config %+v", tc)
		}
	}
}

func TestMigrateUDebComponentsPreservesSlashSeparatedCodenames(t *testing.T) {
	for _, duplicate := range []bool{false, true} {
		t.Run(fmt.Sprint(duplicate), func(t *testing.T) {
			repo := udebRepoFixture(t)
			owned := ownedUDebFixture(repo, repo.DistCodename)
			unrelated := "Codename: private/updates\nComponents: updates/main\nArchitectures: amd64 source\nFakeComponentPrefix: updates\nUDebComponents: updates/main\nContentsUComponents: updates/main\n"
			other := strings.Replace(unrelated, "private/updates", "verbeek/private/releases", 1)
			original := owned + "\n" + unrelated + "\n" + other
			if duplicate {
				original += "\n" + unrelated
			}
			path := writeDistributionFixture(t, repo, "", original)
			err := migrateUDebComponents(repo)
			if duplicate {
				if err == nil || !strings.Contains(err.Error(), `duplicate Codename "private/updates"`) {
					t.Fatalf("duplicate slash-separated Codename error = %v", err)
				}
			} else if err != nil {
				t.Fatal(err)
			}
			contents, err := os.ReadFile(path)
			want := original
			if !duplicate {
				want = strings.ReplaceAll(owned, "Components: main\n", "Components: "+repo.DistComponents+"\n") + "\n" + unrelated + "\n" + other
			}
			if err != nil || string(contents) != want {
				t.Fatalf("slash-separated config = %q, %v; want %q", contents, err, want)
			}
		})
	}
}

func TestMigrateUDebComponentsPreservesInlineComments(t *testing.T) {
	for _, ending := range []string{"\n", "\r\n"} {
		t.Run(fmt.Sprintf("%q", ending), func(t *testing.T) {
			repo := udebRepoFixture(t)
			old := "Codename: verbeek # production\nComponents: main restricted # binary components\n extras restricted-firmware # extra components\nArchitectures: amd64 source # supported\nUDebComponents:\tmain  # installer # comment\n restricted # keep folded comment\nContentsUComponents: main# contents\nDescription: keep # unrelated\n"
			old = strings.ReplaceAll(old, "\n", ending)
			path := writeDistributionFixture(t, repo, "", old)
			if err := migrateUDebComponents(repo); err != nil {
				t.Fatal(err)
			}
			contents, err := os.ReadFile(path)
			want := strings.ReplaceAll(old, "UDebComponents:\tmain  #", "UDebComponents:\t"+repo.DistComponents+"  #")
			want = strings.ReplaceAll(want, "ContentsUComponents: main#", "ContentsUComponents: "+repo.DistComponents+"#")
			want = strings.ReplaceAll(want, " restricted # keep folded comment", "# keep folded comment")
			if err != nil || string(contents) != want {
				t.Fatalf("config = %q, %v; want %q", contents, err, want)
			}
			before, err := os.Stat(path)
			if err != nil {
				t.Fatal(err)
			}
			if err := migrateUDebComponents(repo); err != nil {
				t.Fatal(err)
			}
			after, err := os.Stat(path)
			if err != nil || !os.SameFile(before, after) {
				t.Fatalf("comment-preserving migration is not idempotent: %v", err)
			}
		})
	}
}

func writeIncludedUDebFixture(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(contents), 0640); err != nil {
		t.Fatal(err)
	}
}

func ownedUDebFixture(repo config.RepoConfig, codename string) string {
	return "Codename: " + codename + "\nComponents: " + repo.DistComponents + "\nArchitectures: amd64 source\nUDebComponents: main\nContentsUComponents: main\n"
}

func TestMigrateUDebComponentsFollowsIncludedFiles(t *testing.T) {
	for _, form := range []string{"relative", "dot relative", "confdir", "basedir", "outdir", "other prefix", "absolute", "home", "symlink", "nested", "unrelated"} {
		t.Run(form, func(t *testing.T) {
			repo := udebRepoFixture(t)
			root := filepath.Join(repo.Workdir, repo.DistCodename)
			included := filepath.Join(root, "conf", "owned.conf")
			directive := "owned.conf"
			switch form {
			case "dot relative":
				workingDirectory := t.TempDir()
				t.Chdir(workingDirectory)
				included = filepath.Join(workingDirectory, "owned.conf")
				directive = "./owned.conf"
			case "confdir":
				directive = "+c/owned.conf"
			case "basedir":
				included, directive = filepath.Join(root, "owned.conf"), "+b/owned.conf"
			case "outdir":
				included, directive = filepath.Join(root, "www", "owned.conf"), "+o/owned.conf"
			case "other prefix":
				workingDirectory := t.TempDir()
				t.Chdir(workingDirectory)
				included, directive = filepath.Join(workingDirectory, "+x", "owned.conf"), "+x/owned.conf"
			case "absolute":
				included = filepath.Join(t.TempDir(), "owned.conf")
				directive = included
			case "home":
				homeDirectory, err := os.UserHomeDir()
				if err != nil {
					t.Fatal(err)
				}
				relative, err := filepath.Rel(homeDirectory, included)
				if err != nil {
					t.Fatal(err)
				}
				directive = "~/" + relative
			case "nested":
				directive = "nested/first.conf"
				writeIncludedUDebFixture(t, filepath.Join(root, "conf", directive), "!include: owned.conf\n")
			case "symlink":
				directive = "alias.conf"
			case "unrelated":
				directive = "private.conf"
			}
			original := ownedUDebFixture(repo, repo.DistCodename)
			mainContents := "# keep directive\n!include: " + directive + " # local file\n"
			if form == "unrelated" {
				mainContents = original + "\n" + mainContents
				included = filepath.Join(root, "conf", directive)
				original = "Codename: private\nComponents: special\nArchitectures: all\nUDebComponents: special\n"
			}
			main := writeDistributionFixture(t, repo, "", mainContents)
			writeIncludedUDebFixture(t, included, original)
			if form == "symlink" {
				if err := os.Symlink("owned.conf", filepath.Join(root, "conf", directive)); err != nil {
					t.Fatal(err)
				}
			}
			if err := migrateUDebComponents(repo); err != nil {
				t.Fatal(err)
			}
			for path, old := range map[string]string{main: mainContents, included: original} {
				want := strings.ReplaceAll(old, "Components: main\n", "Components: "+repo.DistComponents+"\n")
				contents, err := os.ReadFile(path)
				if err != nil || string(contents) != want {
					t.Fatalf("%s = %q, %v; want %q", path, contents, err, want)
				}
				before, err := os.Stat(path)
				if err != nil || before.Mode().Perm() != 0640 {
					t.Fatalf("included config mode: %v, %v", before, err)
				}
				if err := migrateUDebComponents(repo); err != nil {
					t.Fatal(err)
				}
				after, err := os.Stat(path)
				if err != nil || !os.SameFile(before, after) {
					t.Fatalf("included config is not idempotent: %v", err)
				}
			}
			if form == "symlink" {
				if target, err := os.Readlink(filepath.Join(root, "conf", directive)); err != nil || target != "owned.conf" {
					t.Fatalf("include symlink changed: %q, %v", target, err)
				}
			}
		})
	}
}

func TestMigrateUDebComponentsFollowsIncludedDirectories(t *testing.T) {
	for _, mainDirectory := range []bool{false, true} {
		t.Run(fmt.Sprint(mainDirectory), func(t *testing.T) {
			repo := udebRepoFixture(t)
			directory := filepath.Join(repo.Workdir, repo.DistCodename, "conf", "distributions")
			if !mainDirectory {
				writeDistributionFixture(t, repo, "", "!include: suites\n")
				directory = filepath.Join(filepath.Dir(directory), "suites")
			}
			for _, suite := range []string{"", "-security", "-updates"} {
				path := filepath.Join(directory, "verbeek"+suite+".conf")
				if suite == "-updates" {
					target := filepath.Join(t.TempDir(), "updates.conf")
					writeIncludedUDebFixture(t, target, ownedUDebFixture(repo, "verbeek"+suite))
					if err := os.Symlink(target, path); err != nil {
						t.Fatal(err)
					}
				} else {
					writeIncludedUDebFixture(t, path, ownedUDebFixture(repo, "verbeek"+suite))
				}
			}
			for _, name := range []string{".hidden.conf", "ignored.txt"} {
				writeIncludedUDebFixture(t, filepath.Join(directory, name), "invalid but ignored\n")
			}
			if err := migrateUDebComponents(repo); err != nil {
				t.Fatal(err)
			}
			for _, suite := range []string{"", "-security", "-updates"} {
				contents, err := os.ReadFile(filepath.Join(directory, "verbeek"+suite+".conf"))
				want := strings.ReplaceAll(ownedUDebFixture(repo, "verbeek"+suite), "Components: main\n", "Components: "+repo.DistComponents+"\n")
				if err != nil || string(contents) != want {
					t.Fatalf("included directory config = %q, %v; want %q", contents, err, want)
				}
			}
		})
	}
}

func TestMigrateUDebComponentsSkipsNonFileDirectoryEntries(t *testing.T) {
	for _, kind := range []string{"directory", "fifo"} {
		t.Run(kind, func(t *testing.T) {
			repo := udebRepoFixture(t)
			main := writeDistributionFixture(t, repo, "", "!include: suites\n")
			directory := filepath.Join(filepath.Dir(main), "suites")
			owned := filepath.Join(directory, "owned.conf")
			original := ownedUDebFixture(repo, repo.DistCodename)
			writeIncludedUDebFixture(t, owned, original)
			ignored := filepath.Join(directory, "ignored.conf")
			ignoredContents := ownedUDebFixture(repo, repo.DistCodename+"-security")
			if kind == "directory" {
				ignored = filepath.Join(ignored, "nested.conf")
				writeIncludedUDebFixture(t, ignored, ignoredContents)
			} else if err := syscall.Mkfifo(ignored, 0600); err != nil {
				t.Fatal(err)
			}
			if err := migrateUDebComponents(repo); err != nil {
				t.Fatal(err)
			}
			contents, err := os.ReadFile(owned)
			want := strings.ReplaceAll(original, "Components: main\n", "Components: "+repo.DistComponents+"\n")
			if err != nil || string(contents) != want {
				t.Fatalf("owned config = %q, %v; want %q", contents, err, want)
			}
			if kind == "directory" {
				contents, err := os.ReadFile(ignored)
				if err != nil || string(contents) != ignoredContents {
					t.Fatalf("ignored nested config changed: %q, %v", contents, err)
				}
			}
		})
	}
}

func TestMigrateUDebComponentsRejectsDirectorySymlinkEntryBeforeWriting(t *testing.T) {
	repo := udebRepoFixture(t)
	original := ownedUDebFixture(repo, repo.DistCodename) + "\n!include: suites\n"
	main := writeDistributionFixture(t, repo, "", original)
	directory := filepath.Join(filepath.Dir(main), "suites")
	if err := os.Mkdir(directory, 0755); err != nil {
		t.Fatal(err)
	}
	otherDirectory := t.TempDir()
	other := filepath.Join(otherDirectory, "owned.conf")
	otherContents := ownedUDebFixture(repo, repo.DistCodename+"-security")
	writeIncludedUDebFixture(t, other, otherContents)
	if err := os.Symlink(otherDirectory, filepath.Join(directory, "linked.conf")); err != nil {
		t.Fatal(err)
	}
	if err := migrateUDebComponents(repo); err == nil || !strings.Contains(err.Error(), otherDirectory) {
		t.Fatalf("directory symlink entry error = %v", err)
	}
	for path, want := range map[string]string{main: original, other: otherContents} {
		contents, err := os.ReadFile(path)
		if err != nil || string(contents) != want {
			t.Fatalf("directory symlink entry changed %s: %q, %v", path, contents, err)
		}
	}
}

func TestMigrateUDebComponentsUpdatesSharedIncludeAcrossRoots(t *testing.T) {
	repo := udebRepoFixture(t)
	shared := filepath.Join(repo.Workdir, "shared.conf")
	original := ownedUDebFixture(repo, repo.DistCodename) + "\n" + ownedUDebFixture(repo, repo.DistCodename+"-experimental")
	writeIncludedUDebFixture(t, shared, original)
	for _, suffix := range []string{"", "-experimental"} {
		writeDistributionFixture(t, repo, suffix, "!include: "+shared+"\n")
	}
	if err := migrateUDebComponents(repo); err != nil {
		t.Fatal(err)
	}
	contents, err := os.ReadFile(shared)
	want := strings.ReplaceAll(original, "Components: main\n", "Components: "+repo.DistComponents+"\n")
	if err != nil || string(contents) != want {
		t.Fatalf("shared config = %q, %v; want %q", contents, err, want)
	}
}

func TestMigrateUDebComponentsValidatesAllIncludesBeforeWriting(t *testing.T) {
	for _, kind := range []string{"missing", "malformed", "cycle", "duplicate file", "duplicate codename", "mixed directive", "empty directive"} {
		t.Run(kind, func(t *testing.T) {
			repo := udebRepoFixture(t)
			normal := writeDistributionFixture(t, repo, "", ownedUDebFixture(repo, repo.DistCodename))
			experimental := writeDistributionFixture(t, repo, "-experimental", "!include: first.conf\n")
			first := filepath.Join(filepath.Dir(experimental), "first.conf")
			firstContents := ownedUDebFixture(repo, repo.DistCodename+"-experimental") + "\n!include: bad.conf\n"
			bad := filepath.Join(filepath.Dir(experimental), "bad.conf")
			badContents := "malformed\n"
			switch kind {
			case "cycle":
				badContents = "!include: first.conf\n"
			case "duplicate file":
				firstContents += "\n!include: +c/bad.conf\n"
				badContents = "# included twice\n"
			case "duplicate codename":
				badContents = ownedUDebFixture(repo, repo.DistCodename+"-experimental")
			case "mixed directive":
				badContents = "!include: private.conf\nDescription: not a directive paragraph\n"
			case "empty directive":
				badContents = "!include: # missing path\n"
			}
			writeIncludedUDebFixture(t, first, firstContents)
			if kind != "missing" {
				writeIncludedUDebFixture(t, bad, badContents)
			}
			originals := map[string]string{normal: ownedUDebFixture(repo, repo.DistCodename), experimental: "!include: first.conf\n", first: firstContents}
			if kind != "missing" {
				originals[bad] = badContents
			}
			err := migrateUDebComponents(repo)
			if err == nil || !strings.Contains(err.Error(), bad) {
				t.Fatalf("invalid include error = %v", err)
			}
			for path, want := range originals {
				contents, err := os.ReadFile(path)
				if err != nil || string(contents) != want {
					t.Fatalf("invalid include changed %s: %q, %v", path, contents, err)
				}
			}
		})
	}
}

func TestMigrateUDebComponentsRejectsHardLinkedConfigsBeforeWriting(t *testing.T) {
	repo := udebRepoFixture(t)
	original := ownedUDebFixture(repo, repo.DistCodename) + "\n" + ownedUDebFixture(repo, repo.DistCodename+"-experimental")
	normal := writeDistributionFixture(t, repo, "", original)
	experimental := filepath.Join(repo.Workdir, repo.DistCodename+"-experimental", "conf", "distributions")
	if err := os.MkdirAll(filepath.Dir(experimental), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Link(normal, experimental); err != nil {
		t.Fatal(err)
	}
	if err := migrateUDebComponents(repo); err == nil || !strings.Contains(err.Error(), "hard-linked") {
		t.Fatalf("hard-linked configs error = %v", err)
	}
	for _, path := range []string{normal, experimental} {
		contents, err := os.ReadFile(path)
		if err != nil || string(contents) != original {
			t.Fatalf("hard-linked config changed: %q, %v", contents, err)
		}
	}
}
