package main

import (
	"archive/tar"
	"bufio"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/blankon/irgsh-go/pkg/systemutil"
)

var safePathID = regexp.MustCompile(`^[a-zA-Z0-9._+-]+$`)

type buildSubmission struct {
	TaskUUID               string `json:"taskUUID"`
	Dist                   string `json:"dist"`
	PackageName            string `json:"packageName"`
	PackageVersion         string `json:"packageVersion"`
	PackageExtendedVersion string `json:"packageExtendedVersion"`
	PackageURL             string `json:"packageUrl"`
	SourceURL              string `json:"sourceUrl"`
	Maintainer             string `json:"maintainer"`
	Component              string `json:"component"`
	IsExperimental         bool   `json:"isExperimental"`
	ForceVersion           bool   `json:"forceVersion"`
	PackageBranch          string `json:"packageBranch"`
	SourceBranch           string `json:"sourceBranch"`
}

type buildJob struct {
	Submission buildSubmission
	Root       string
	Artifacts  string
	Archive    string
	Log        string
}

type attemptPaths struct {
	Number int
	Root   string
	Input  string
	Result string
	Temp   string
	Base   string
}

type sourceSet struct {
	DSC   string
	Files []string
}

func validPathID(id string) bool {
	return safePathID.MatchString(id) && id != "." && id != ".."
}

func decodeBuildSubmission(payload string, servedDist string) (buildSubmission, error) {
	var submission buildSubmission
	if err := json.Unmarshal([]byte(payload), &submission); err != nil {
		return submission, fmt.Errorf("decode build submission: %w", err)
	}
	for name, value := range map[string]string{
		"taskUUID": submission.TaskUUID, "dist": submission.Dist,
		"packageName": submission.PackageName, "packageVersion": submission.PackageVersion,
		"maintainer": submission.Maintainer, "component": submission.Component,
	} {
		if value == "" {
			return submission, fmt.Errorf("build submission missing %s", name)
		}
	}
	if !validPathID(submission.TaskUUID) || !validPathID(submission.Dist) || !validPathID(servedDist) {
		return submission, fmt.Errorf("unsafe build submission identifier or distribution")
	}
	if submission.Dist != servedDist {
		return submission, fmt.Errorf("build task targeted dist %q but this builder instance serves %q", submission.Dist, servedDist)
	}
	return submission, nil
}

func newBuildJob(workdir string, submission buildSubmission) (buildJob, error) {
	if !validPathID(submission.TaskUUID) {
		return buildJob{}, fmt.Errorf("unsafe taskUUID %q", submission.TaskUUID)
	}
	root, err := filepath.Abs(workdir)
	if err != nil {
		return buildJob{}, fmt.Errorf("resolve builder workdir: %w", err)
	}
	artifacts := filepath.Join(root, "artifacts", submission.TaskUUID)
	return buildJob{
		Submission: submission,
		Root:       filepath.Join(root, "jobs", submission.TaskUUID),
		Artifacts:  artifacts,
		Archive:    filepath.Join(root, "artifacts", submission.TaskUUID+".tar.gz"),
		Log:        filepath.Join(artifacts, "build.log"),
	}, nil
}

func (j buildJob) newAttempt(number int) (attemptPaths, error) {
	if number <= 0 {
		return attemptPaths{}, fmt.Errorf("attempt number must be positive")
	}
	root := filepath.Join(j.Root, strconv.Itoa(number))
	if err := os.RemoveAll(root); err != nil {
		return attemptPaths{}, fmt.Errorf("remove previous attempt: %w", err)
	}
	paths := attemptPaths{
		Number: number,
		Root:   root,
		Input:  filepath.Join(root, "input"),
		Result: filepath.Join(root, "result"),
		Temp:   filepath.Join(root, "tmp"),
		Base:   filepath.Join(root, "base.tar"),
	}
	for _, dir := range []string{paths.Input, paths.Result, paths.Temp} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return attemptPaths{}, fmt.Errorf("create attempt directory %s: %w", dir, err)
		}
	}
	return paths, nil
}

func downloadSubmission(ctx context.Context, client *http.Client, chiefAddress string, taskUUID string, destination string) (err error) {
	if !validPathID(taskUUID) {
		return fmt.Errorf("unsafe taskUUID %q", taskUUID)
	}
	endpoint, err := url.Parse(chiefAddress)
	if err != nil {
		return fmt.Errorf("parse chief address: %w", err)
	}
	endpoint.Path = "/submissions/" + url.PathEscape(taskUUID) + ".tar.gz"
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return fmt.Errorf("create submission request: %w", err)
	}
	response, err := client.Do(request)
	if err != nil {
		return fmt.Errorf("download submission: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("download submission: HTTP %d", response.StatusCode)
	}
	file, err := os.OpenFile(destination, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return fmt.Errorf("create submission archive: %w", err)
	}
	defer func() {
		if err != nil {
			_ = os.Remove(destination)
		}
	}()
	if _, err = io.Copy(file, response.Body); err != nil {
		_ = file.Close()
		return fmt.Errorf("save submission archive: %w", err)
	}
	if err = file.Sync(); err != nil {
		_ = file.Close()
		return fmt.Errorf("sync submission archive: %w", err)
	}
	if err = file.Close(); err != nil {
		return fmt.Errorf("close submission archive: %w", err)
	}
	return nil
}

func extractSubmission(archivePath string, destination string) error {
	root, err := filepath.Abs(destination)
	if err != nil {
		return fmt.Errorf("resolve extraction root: %w", err)
	}
	archive, err := os.Open(archivePath)
	if err != nil {
		return fmt.Errorf("open submission archive: %w", err)
	}
	defer archive.Close()
	compressed, err := gzip.NewReader(archive)
	if err != nil {
		return fmt.Errorf("read submission gzip: %w", err)
	}
	defer compressed.Close()
	reader := tar.NewReader(compressed)
	seen := make(map[string]bool)
	for {
		header, err := reader.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return fmt.Errorf("read submission tar: %w", err)
		}
		if header.Typeflag != tar.TypeDir && header.Typeflag != tar.TypeReg && header.Typeflag != tar.TypeRegA {
			return fmt.Errorf("unsupported archive member %q", header.Name)
		}
		if path.IsAbs(header.Name) || strings.Contains(header.Name, `\`) {
			return fmt.Errorf("unsafe archive member %q", header.Name)
		}
		for _, segment := range strings.Split(header.Name, "/") {
			if segment == ".." {
				return fmt.Errorf("unsafe archive member %q", header.Name)
			}
		}
		name := path.Clean(header.Name)
		if name == "." && header.Typeflag == tar.TypeDir {
			continue
		}
		if name == "." {
			return fmt.Errorf("unsafe archive member %q", header.Name)
		}
		target := filepath.Join(root, filepath.FromSlash(name))
		rel, err := filepath.Rel(root, target)
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
			return fmt.Errorf("archive member outside extraction root %q", header.Name)
		}
		if seen[target] {
			return fmt.Errorf("duplicate archive member %q", header.Name)
		}
		seen[target] = true
		if header.Typeflag == tar.TypeDir {
			if err := os.MkdirAll(target, 0755); err != nil {
				return fmt.Errorf("create archive directory %q: %w", header.Name, err)
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
			return fmt.Errorf("create archive parent %q: %w", header.Name, err)
		}
		file, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if err != nil {
			return fmt.Errorf("create archive member %q: %w", header.Name, err)
		}
		_, copyErr := io.Copy(file, reader)
		closeErr := file.Close()
		if copyErr != nil {
			return fmt.Errorf("extract archive member %q: %w", header.Name, copyErr)
		}
		if closeErr != nil {
			return fmt.Errorf("close archive member %q: %w", header.Name, closeErr)
		}
	}
}

type dscMember struct {
	Name   string
	Size   int64
	SHA256 string
}

func parseDSCMembers(dscPath string) ([]dscMember, error) {
	file, err := os.Open(dscPath)
	if err != nil {
		return nil, fmt.Errorf("open DSC: %w", err)
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 4096), 1024*1024)
	if !scanner.Scan() || scanner.Text() != "-----BEGIN PGP SIGNED MESSAGE-----" {
		return nil, fmt.Errorf("DSC is not clear-signed")
	}
	var members []dscMember
	inChecksums := false
	foundChecksums := false
	foundSignature := false
	foundEnd := false
	names := make(map[string]bool)
	for scanner.Scan() {
		line := strings.TrimSuffix(scanner.Text(), "\r")
		if line == "-----BEGIN PGP SIGNATURE-----" {
			foundSignature = true
			inChecksums = false
			continue
		}
		if line == "-----END PGP SIGNATURE-----" && foundSignature {
			foundEnd = true
			break
		}
		if foundSignature {
			continue
		}
		if line == "Checksums-Sha256:" {
			if foundChecksums {
				return nil, fmt.Errorf("duplicate DSC Checksums-Sha256 field")
			}
			foundChecksums = true
			inChecksums = true
			continue
		}
		if !inChecksums {
			continue
		}
		if !strings.HasPrefix(line, " ") && !strings.HasPrefix(line, "\t") {
			inChecksums = false
			continue
		}
		fields := strings.Fields(line)
		if len(fields) != 3 || len(fields[0]) != 64 {
			return nil, fmt.Errorf("invalid DSC SHA-256 row")
		}
		if _, err := hex.DecodeString(fields[0]); err != nil {
			return nil, fmt.Errorf("invalid DSC SHA-256 digest: %w", err)
		}
		if fields[1] == "" || strings.Trim(fields[1], "0123456789") != "" {
			return nil, fmt.Errorf("invalid DSC member size %q", fields[1])
		}
		size, err := strconv.ParseInt(fields[1], 10, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid DSC member size %q: %w", fields[1], err)
		}
		name := fields[2]
		if name == "." || name == ".." || filepath.Base(name) != name || strings.Contains(name, `\`) || names[name] {
			return nil, fmt.Errorf("unsafe or duplicate DSC member %q", name)
		}
		names[name] = true
		members = append(members, dscMember{Name: name, Size: size, SHA256: fields[0]})
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read DSC: %w", err)
	}
	if !foundChecksums || len(members) == 0 || !foundSignature || !foundEnd {
		return nil, fmt.Errorf("incomplete clear-signed DSC")
	}
	return members, nil
}

func prepareSource(extractedRoot string, inputDir string) (source sourceSet, err error) {
	dscPaths, err := filepath.Glob(filepath.Join(extractedRoot, "signed", "*.dsc"))
	if err != nil {
		return sourceSet{}, fmt.Errorf("find signed DSC: %w", err)
	}
	if len(dscPaths) != 1 {
		return sourceSet{}, fmt.Errorf("expected one signed DSC, found %d", len(dscPaths))
	}
	dsc := dscPaths[0]
	if info, err := os.Lstat(dsc); err != nil || !info.Mode().IsRegular() {
		return sourceSet{}, fmt.Errorf("signed DSC is not a regular file: %v", err)
	}
	members, err := parseDSCMembers(dsc)
	if err != nil {
		return sourceSet{}, err
	}
	paths := map[string]string{filepath.Base(dsc): dsc}
	for _, member := range members {
		if _, exists := paths[member.Name]; exists {
			return sourceSet{}, fmt.Errorf("DSC member duplicates DSC %q", member.Name)
		}
		var found string
		for _, candidate := range []string{filepath.Join(extractedRoot, "signed", member.Name), filepath.Join(extractedRoot, member.Name)} {
			info, statErr := os.Lstat(candidate)
			if os.IsNotExist(statErr) {
				continue
			}
			if statErr != nil || !info.Mode().IsRegular() {
				return sourceSet{}, fmt.Errorf("source member %q is not a regular file: %v", candidate, statErr)
			}
			if found != "" {
				return sourceSet{}, fmt.Errorf("source member %q exists in two locations", member.Name)
			}
			found = candidate
		}
		if found == "" {
			return sourceSet{}, fmt.Errorf("missing source member %q", member.Name)
		}
		if err := checkSourceMember(found, member); err != nil {
			return sourceSet{}, err
		}
		paths[member.Name] = found
	}
	if err := os.MkdirAll(inputDir, 0755); err != nil {
		return sourceSet{}, fmt.Errorf("create source input: %w", err)
	}
	entries, err := os.ReadDir(inputDir)
	if err != nil || len(entries) != 0 {
		return sourceSet{}, fmt.Errorf("source input is not empty: %v", err)
	}
	defer func() {
		if err != nil {
			for name := range paths {
				_ = os.Remove(filepath.Join(inputDir, name))
			}
		}
	}()
	source.DSC = filepath.Join(inputDir, filepath.Base(dsc))
	for name, original := range paths {
		target := filepath.Join(inputDir, name)
		if err = systemutil.CopyFile(original, target, 0444); err != nil {
			return sourceSet{}, fmt.Errorf("stage source member %q: %w", name, err)
		}
		if err = os.Chmod(target, 0444); err != nil {
			return sourceSet{}, fmt.Errorf("make staged source read-only %q: %w", name, err)
		}
		source.Files = append(source.Files, name)
	}
	sort.Strings(source.Files)
	if err = validateSource(inputDir, source); err != nil {
		return sourceSet{}, err
	}
	return source, nil
}

func validateSource(inputDir string, source sourceSet) error {
	if filepath.Dir(source.DSC) != filepath.Clean(inputDir) {
		return fmt.Errorf("DSC outside source input")
	}
	info, err := os.Lstat(source.DSC)
	if err != nil || !info.Mode().IsRegular() {
		return fmt.Errorf("staged DSC is not a regular file: %v", err)
	}
	members, err := parseDSCMembers(source.DSC)
	if err != nil {
		return err
	}
	want := make([]string, 0, len(members)+1)
	want = append(want, filepath.Base(source.DSC))
	for _, member := range members {
		want = append(want, member.Name)
		if err := checkSourceMember(filepath.Join(inputDir, member.Name), member); err != nil {
			return err
		}
	}
	sort.Strings(want)
	if len(want) != len(source.Files) {
		return fmt.Errorf("staged source set changed")
	}
	for i, name := range want {
		if source.Files[i] != name {
			return fmt.Errorf("staged source set changed")
		}
	}
	entries, err := os.ReadDir(inputDir)
	if err != nil || len(entries) != len(want) {
		return fmt.Errorf("staged source files changed: %v", err)
	}
	return nil
}

func checkSourceMember(filename string, member dscMember) error {
	info, err := os.Lstat(filename)
	if err != nil || !info.Mode().IsRegular() {
		return fmt.Errorf("source member %q is not a regular file: %v", filename, err)
	}
	if info.Size() != member.Size {
		return fmt.Errorf("source member %q has size %d, want %d", filename, info.Size(), member.Size)
	}
	file, err := os.Open(filename)
	if err != nil {
		return fmt.Errorf("open source member %q: %w", filename, err)
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return fmt.Errorf("hash source member %q: %w", filename, err)
	}
	if fmt.Sprintf("%x", hash.Sum(nil)) != strings.ToLower(member.SHA256) {
		return fmt.Errorf("source member %q has wrong SHA-256", filename)
	}
	return nil
}
