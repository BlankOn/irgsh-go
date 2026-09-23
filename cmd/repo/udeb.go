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
		original  string
		contents  string
		info      os.FileInfo
		temporary string
	}
	var updates []*update
	files := map[string]*update{}
	for _, dist := range []string{repo.DistCodename, repo.DistCodename + "-experimental"} {
		root := filepath.Join(repo.Workdir, dist)
		if _, err := os.Stat(root); os.IsNotExist(err) {
			continue
		} else if err != nil {
			return fmt.Errorf("inspect repository %s: %w", root, err)
		}
		visited := map[string]bool{}
		codenames := map[string]bool{}
		var visit func(string, bool) error
		visit = func(path string, allowDirectory bool) error {
			resolved, err := filepath.EvalSymlinks(path)
			if err != nil {
				return fmt.Errorf("resolve repository config %s: %w", path, err)
			}
			path, err = filepath.Abs(resolved)
			if err != nil {
				return err
			}
			if visited[path] {
				return fmt.Errorf("cyclic or duplicate repository include %s", path)
			}
			visited[path] = true
			info, err := os.Stat(path)
			if err != nil {
				return fmt.Errorf("inspect repository config %s: %w", path, err)
			}
			if info.IsDir() && allowDirectory {
				entries, err := os.ReadDir(path)
				if err != nil {
					return fmt.Errorf("read repository config directory %s: %w", path, err)
				}
				for _, entry := range entries {
					if entry.Type() != 0 && entry.Type() != os.ModeSymlink {
						continue
					}
					if !strings.HasPrefix(entry.Name(), ".") && strings.HasSuffix(entry.Name(), ".conf") {
						if err := visit(filepath.Join(path, entry.Name()), false); err != nil {
							return err
						}
					}
				}
				return nil
			}
			if !info.Mode().IsRegular() {
				return fmt.Errorf("repository config %s is not a regular file or permitted directory", path)
			}
			change := files[path]
			if change == nil {
				for _, existing := range updates {
					if os.SameFile(info, existing.info) {
						return fmt.Errorf("hard-linked repository configs %s and %s cannot be atomically migrated", path, existing.path)
					}
				}
				contents, err := os.ReadFile(path)
				if err != nil {
					return fmt.Errorf("read repository config %s: %w", path, err)
				}
				change = &update{path: path, original: string(contents), contents: string(contents), info: info}
				files[path] = change
				updates = append(updates, change)
			}
			include := func(name string) error {
				switch {
				case filepath.IsAbs(name), strings.HasPrefix(name, "./"):
				case strings.HasPrefix(name, "+b/"):
					name = filepath.Join(root, name[3:])
				case strings.HasPrefix(name, "+c/"):
					name = filepath.Join(root, "conf", name[3:])
				case strings.HasPrefix(name, "+o/"):
					name = filepath.Join(root, "www", name[3:])
				case strings.HasPrefix(name, "~/"):
					homeDirectory, err := os.UserHomeDir()
					if err != nil {
						return fmt.Errorf("resolve repository include %q: %w", name, err)
					}
					name = filepath.Join(homeDirectory, name[2:])
				case len(name) >= 3 && name[0] == '+' && name[2] == '/':
				default:
					name = filepath.Join(root, "conf", name)
				}
				return visit(name, true)
			}
			rewritten, err := rewriteUDebComponents(change.contents, dist, components, codenames, include)
			if err != nil {
				return fmt.Errorf("migrate repository config %s: %w", path, err)
			}
			change.contents = rewritten
			return nil
		}
		path := filepath.Join(root, "conf", "distributions")
		if err := visit(path, true); err != nil {
			return err
		}
		if !codenames[dist] {
			return fmt.Errorf("repository config %s is missing Codename %q", path, dist)
		}
	}
	for _, change := range updates {
		if change.contents == change.original {
			continue
		}
		file, err := os.CreateTemp(filepath.Dir(change.path), ".distributions-*")
		if err != nil {
			return fmt.Errorf("stage repository config %s: %w", change.path, err)
		}
		change.temporary = file.Name()
		defer os.Remove(file.Name())
		if err = file.Chmod(change.info.Mode().Perm()); err == nil {
			_, err = file.WriteString(change.contents)
		}
		err = errors.Join(err, file.Sync(), file.Close())
		if err != nil {
			return fmt.Errorf("write repository config %s: %w", change.path, err)
		}
	}
	for _, change := range updates {
		if change.temporary == "" {
			continue
		}
		if err := os.Rename(change.temporary, change.path); err != nil {
			return fmt.Errorf("replace repository config %s: %w", change.path, err)
		}
	}
	return nil
}

func rewriteUDebComponents(contents, dist string, components []string, seen map[string]bool, include func(string) error) (string, error) {
	var result strings.Builder
	var stanza []string
	flush := func() error {
		updated, codename, err := rewriteUDebStanza(stanza, dist, components, include)
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
	return result.String(), nil
}

func rewriteUDebStanza(lines []string, dist string, components []string, include func(string) error) (string, string, error) {
	fields := map[string]string{}
	starts := map[string]int{}
	keys := make([]string, len(lines))
	last := ""
	for i, line := range lines {
		content, _, _ := strings.Cut(strings.TrimRight(line, "\r\n"), "#")
		if strings.TrimSpace(content) == "" {
			continue
		}
		if content[0] == ' ' || content[0] == '\t' {
			if last == "" {
				return "", "", fmt.Errorf("orphan field continuation")
			}
			fields[last] += " " + strings.TrimSpace(content)
			keys[i] = last
			continue
		}
		name, value, ok := strings.Cut(content, ":")
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
	if path, directive := fields["!include"]; directive {
		if len(fields) != 1 || path == "" {
			return "", "", fmt.Errorf("!include paragraph must contain only a nonempty include path")
		}
		return strings.Join(lines, ""), "", include(path)
	}
	codename := fields["codename"]
	if fields["components"] == "" || fields["architectures"] == "" {
		return "", "", fmt.Errorf("distribution requires a valid Codename, Components and Architectures")
	}
	for _, part := range strings.Split(codename, "/") {
		if !repoConfigToken.MatchString(part) {
			return "", "", fmt.Errorf("distribution has invalid Codename %q", codename)
		}
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
		_, target := names[keys[i]]
		if !target {
			result.WriteString(line)
			continue
		}
		if starts[keys[i]] != i {
			if hash := strings.IndexByte(line, '#'); hash >= 0 {
				result.WriteString(line[hash:])
			}
			continue
		}
		ending := line[len(strings.TrimRight(line, "\r\n")):]
		content, comment, hasComment := strings.Cut(strings.TrimRight(line, "\r\n"), "#")
		if hasComment {
			comment = content[len(strings.TrimRight(content, " \t")):] + "#" + comment
		}
		name, value, _ := strings.Cut(content, ":")
		padding := value[:len(value)-len(strings.TrimLeft(value, " \t"))]
		fmt.Fprintf(&result, "%s:%s%s%s%s", name, padding, strings.Join(components, " "), comment, ending)
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
