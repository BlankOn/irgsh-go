package main

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/blankon/irgsh-go/internal/cli/domain"
	"github.com/blankon/irgsh-go/internal/cli/repository"
	"github.com/blankon/irgsh-go/internal/cli/usecase"
)

func TestTargetConfigIsolationAndValidation(t *testing.T) {
	home := t.TempDir()
	legacy := filepath.Join(home, ".irgsh", "IRGSH_CHIEF_ADDRESS")
	if err := os.MkdirAll(filepath.Dir(legacy), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(legacy, []byte("https://legacy.example"), 0644); err != nil {
		t.Fatal(err)
	}

	if err := runCLI(context.Background(), []string{"irgsh-cli", "--target", "staging", "config", "--chief", "https://wrong.example", "--key", "test"}, home, "test"); err == nil || !strings.Contains(err.Error(), "invalid target") {
		t.Fatalf("invalid target error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(home, ".irgsh", "targets")); !os.IsNotExist(err) {
		t.Fatalf("invalid target touched profiles: %v", err)
	}

	for _, tc := range []struct {
		args []string
		path string
		want string
	}{
		{[]string{"irgsh-cli", "config", "--chief", "https://dev.example", "--key", "dev-key"}, "dev", "https://dev.example"},
		{[]string{"irgsh-cli", "--target", "prod", "config", "--chief", "https://prod.example", "--key", "prod-key"}, "prod", "https://prod.example"},
		{[]string{"irgsh-cli", "--target=dev", "config", "--chief", "https://explicit.example", "--key", "dev-key"}, "dev", "https://explicit.example"},
	} {
		if err := runCLI(context.Background(), tc.args, home, "test"); err != nil {
			t.Fatal(err)
		}
		got, err := os.ReadFile(filepath.Join(home, ".irgsh", "targets", tc.path, "IRGSH_CHIEF_ADDRESS"))
		if err != nil || string(got) != tc.want {
			t.Fatalf("%s chief = %q, %v", tc.path, got, err)
		}
	}
	got, err := os.ReadFile(legacy)
	if err != nil || string(got) != "https://legacy.example" {
		t.Fatalf("legacy chief changed: %q, %v", got, err)
	}
}

func TestTargetPipelineIDsStaySeparate(t *testing.T) {
	home := t.TempDir()
	legacy := repository.NewFilePipelineStore(filepath.Join(home, ".irgsh"))
	if err := legacy.SavePackageID("legacy-package"); err != nil {
		t.Fatal(err)
	}
	dev := repository.NewFilePipelineStore(profilePath(home, "dev"))
	prod := repository.NewFilePipelineStore(profilePath(home, "prod"))
	if _, err := dev.LoadPackageID(); !os.IsNotExist(err) {
		t.Fatalf("dev read legacy package ID: %v", err)
	}
	for _, tc := range []struct {
		name string
		save func(*repository.FilePipelineStore, string) error
		load func(*repository.FilePipelineStore) (string, error)
	}{
		{"package", (*repository.FilePipelineStore).SavePackageID, (*repository.FilePipelineStore).LoadPackageID},
		{"iso", (*repository.FilePipelineStore).SaveISOID, (*repository.FilePipelineStore).LoadISOID},
		{"import", (*repository.FilePipelineStore).SaveImportID, (*repository.FilePipelineStore).LoadImportID},
		{"retry", (*repository.FilePipelineStore).SaveRetryID, (*repository.FilePipelineStore).LoadRetryID},
	} {
		if err := tc.save(dev, "dev-"+tc.name); err != nil {
			t.Fatal(err)
		}
		if err := tc.save(prod, "prod-"+tc.name); err != nil {
			t.Fatal(err)
		}
		for _, target := range []struct {
			store *repository.FilePipelineStore
			want  string
		}{{dev, "dev-" + tc.name}, {prod, "prod-" + tc.name}} {
			got, err := tc.load(target.store)
			if err != nil || got != target.want {
				t.Fatalf("%s ID = %q, %v; want %q", tc.name, got, err, target.want)
			}
		}
	}
}

func TestProdBlocksEveryNetworkCommand(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()
	home := t.TempDir()
	if err := runCLI(context.Background(), []string{"irgsh-cli", "--target", "prod", "config", "--chief", server.URL, "--key", "test-key"}, home, "test"); err != nil {
		t.Fatal(err)
	}
	for _, command := range [][]string{
		{"package", "--dist", "test", "--package", "https://example.com/pkg"},
		{"package", "status", "pkg-id"}, {"package", "log", "pkg-id"},
		{"import", "--source", "https://example.com", "--dist", "test", "--source-dist", "sid", "--package-name", "pkg"},
		{"import", "status", "imp-id"}, {"import", "log", "imp-id"},
		{"build-iso", "--dist", "test", "--branch", "test"},
		{"build-iso", "status", "iso-id"}, {"build-iso", "log", "iso-id"},
		{"retry", "pkg-id"}, {"cancel", "pkg-id"}, {"update"},
	} {
		args := append([]string{"irgsh-cli", "--target", "prod"}, command...)
		err := runCLI(context.Background(), args, home, "test")
		if err == nil || !strings.Contains(err.Error(), "prod") {
			t.Fatalf("%v error = %v", command, err)
		}
		if requests != 0 {
			t.Fatalf("%v made %d requests", command, requests)
		}
	}
}

type displayingService struct {
	*usecase.CLIUsecase
	output      *os.File
	chief       string
	wantDisplay string
	reject      []string
}

func (s *displayingService) LoadConfig() (domain.Config, error) {
	return domain.Config{ChiefAddress: s.chief}, nil
}

func (s *displayingService) PackageStatus(ctx context.Context, id string) (domain.PackageStatus, error) {
	if err := s.output.SetReadDeadline(time.Now().Add(time.Second)); err != nil {
		return domain.PackageStatus{}, err
	}
	buf := make([]byte, 256)
	n, err := s.output.Read(buf)
	if err != nil || !strings.Contains(string(buf[:n]), "Target: dev") || !strings.Contains(string(buf[:n]), "Chief: "+s.wantDisplay) {
		return domain.PackageStatus{}, fmt.Errorf("target and chief missing before service call: %q, %v", buf[:n], err)
	}
	for _, secret := range s.reject {
		if strings.Contains(string(buf[:n]), secret) {
			return domain.PackageStatus{}, fmt.Errorf("chief display exposed credential: %q", secret)
		}
	}
	return domain.PackageStatus{}, nil
}

func TestDevShowsTargetAndChiefBeforeServiceCall(t *testing.T) {
	service := &displayingService{chief: "https://dev.example", wantDisplay: "https://dev.example"}
	read, write, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	original := os.Stdout
	os.Stdout = write
	defer func() { os.Stdout = original; read.Close(); write.Close() }()
	service.output = read
	app := buildApp(context.Background(), service, "test", "dev")
	if err := app.Run([]string{"irgsh-cli", "package", "status", "pkg-id"}); err != nil {
		t.Fatalf("display ordering error = %v", err)
	}
}

func TestChiefDisplayOmitsURLCredentials(t *testing.T) {
	service := &displayingService{chief: "https://name:secret@dev.example/path?token=test", wantDisplay: "https://dev.example/path", reject: []string{"secret", "token=test"}}
	read, write, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	original := os.Stdout
	os.Stdout = write
	defer func() { os.Stdout = original; read.Close(); write.Close() }()
	service.output = read
	app := buildApp(context.Background(), service, "test", "dev")
	if err := app.Run([]string{"irgsh-cli", "package", "status", "pkg-id"}); err != nil {
		t.Fatal(err)
	}
}
