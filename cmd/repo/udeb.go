package main

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/blankon/irgsh-go/internal/config"
)

var repoConfigToken = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._+-]*$`)

func migrateUDebComponents(repo config.RepoConfig) error {
	components := strings.Fields(repo.DistComponents)
	if !repoConfigToken.MatchString(repo.DistCodename) || len(components) == 0 {
		return fmt.Errorf("invalid repository distribution or components")
	}
	seen := map[string]bool{}
	for _, component := range components {
		if !repoConfigToken.MatchString(component) || seen[component] {
			return fmt.Errorf("invalid or duplicate repository component %q", component)
		}
		seen[component] = true
	}
	type update struct {
		path      string
		contents  string
		mode      os.FileMode
		temporary string
	}
	var updates []update
	for _, dist := range []string{repo.DistCodename, repo.DistCodename + "-experimental"} {
		root := filepath.Join(repo.Workdir, dist)
		if _, err := os.Stat(root); os.IsNotExist(err) {
			continue
		} else if err != nil {
			return fmt.Errorf("inspect repository %s: %w", root, err)
		}
		path := filepath.Join(root, "conf", "distributions")
		info, err := os.Lstat(path)
		if err != nil {
			return fmt.Errorf("inspect initialized repository config %s: %w", path, err)
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("repository config %s is not a regular file", path)
		}
		contents, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read repository config %s: %w", path, err)
		}
		rewritten, err := rewriteUDebComponents(string(contents), dist, components)
		if err != nil {
			return fmt.Errorf("migrate repository config %s: %w", path, err)
		}
		if rewritten != string(contents) {
			updates = append(updates, update{path: path, contents: rewritten, mode: info.Mode().Perm()})
		}
	}
	for i := range updates {
		change := &updates[i]
		file, err := os.CreateTemp(filepath.Dir(change.path), ".distributions-*")
		if err != nil {
			return fmt.Errorf("stage repository config %s: %w", change.path, err)
		}
		change.temporary = file.Name()
		defer os.Remove(file.Name())
		if err = file.Chmod(change.mode); err == nil {
			_, err = file.WriteString(change.contents)
		}
		err = errors.Join(err, file.Sync(), file.Close())
		if err != nil {
			return fmt.Errorf("write repository config %s: %w", change.path, err)
		}
	}
	for _, change := range updates {
		if err := os.Rename(change.temporary, change.path); err != nil {
			return fmt.Errorf("replace repository config %s: %w", change.path, err)
		}
	}
	return nil
}

func rewriteUDebComponents(contents, dist string, components []string) (string, error) {
	var result strings.Builder
	var stanza []string
	seen := map[string]bool{}
	flush := func() error {
		updated, codename, err := rewriteUDebStanza(stanza, dist, components)
		if err != nil {
			return err
		}
		if codename != "" {
			if seen[codename] {
				return fmt.Errorf("duplicate Codename %q", codename)
			}
			seen[codename] = true
		}
		result.WriteString(updated)
		stanza = nil
		return nil
	}
	for _, line := range strings.SplitAfter(contents, "\n") {
		if strings.TrimSpace(line) == "" {
			if err := flush(); err != nil {
				return "", err
			}
			result.WriteString(line)
		} else {
			stanza = append(stanza, line)
		}
	}
	if err := flush(); err != nil {
		return "", err
	}
	if !seen[dist] {
		return "", fmt.Errorf("missing Codename %q", dist)
	}
	return result.String(), nil
}

func rewriteUDebStanza(lines []string, dist string, components []string) (string, string, error) {
	fields := map[string]string{}
	starts := map[string]int{}
	keys := make([]string, len(lines))
	last := ""
	for i, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), "#") {
			continue
		}
		if line[0] == ' ' || line[0] == '\t' {
			if last == "" {
				return "", "", fmt.Errorf("orphan field continuation")
			}
			fields[last] += " " + strings.TrimSpace(line)
			keys[i] = last
			continue
		}
		name, value, ok := strings.Cut(strings.TrimRight(line, "\r\n"), ":")
		name = strings.ToLower(name)
		if !ok || name == "" || strings.ContainsAny(name, " \t\r\n") {
			return "", "", fmt.Errorf("malformed distribution field %q", strings.TrimSpace(line))
		}
		if _, exists := fields[name]; exists {
			return "", "", fmt.Errorf("duplicate distribution field %q", name)
		}
		fields[name], starts[name], keys[i], last = strings.TrimSpace(value), i, name, name
	}
	if len(fields) == 0 {
		return strings.Join(lines, ""), "", nil
	}
	codename := fields["codename"]
	if !repoConfigToken.MatchString(codename) || fields["components"] == "" || fields["architectures"] == "" {
		return "", "", fmt.Errorf("distribution requires a valid Codename, Components and Architectures")
	}
	if codename != dist && codename != dist+"-security" && codename != dist+"-updates" {
		return strings.Join(lines, ""), codename, nil
	}
	available := map[string]bool{}
	for _, component := range strings.Fields(fields["components"]) {
		available[component] = true
	}
	for _, component := range components {
		if !available[component] {
			return "", "", fmt.Errorf("distribution %s Components is missing configured component %q", codename, component)
		}
	}
	names := map[string]string{"udebcomponents": "UDebComponents", "contentsucomponents": "ContentsUComponents"}
	var result bytes.Buffer
	for i, line := range lines {
		name, target := names[keys[i]]
		if !target {
			result.WriteString(line)
			continue
		}
		if starts[keys[i]] != i {
			continue
		}
		ending := line[len(strings.TrimRight(line, "\r\n")):]
		fmt.Fprintf(&result, "%s: %s%s", name, strings.Join(components, " "), ending)
	}
	ending := "\n"
	if strings.Contains(strings.Join(lines, ""), "\r\n") {
		ending = "\r\n"
	}
	for _, key := range []string{"udebcomponents", "contentsucomponents"} {
		if _, exists := fields[key]; exists {
			continue
		}
		if !bytes.HasSuffix(result.Bytes(), []byte("\n")) {
			result.WriteString(ending)
		}
		fmt.Fprintf(&result, "%s: %s%s", names[key], strings.Join(components, " "), ending)
	}
	return result.String(), codename, nil
}
