package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestStorePackageReportsChiefResponse(t *testing.T) {
	const taskUUID = "2026-10-04-120000_test"

	tests := []struct {
		name    string
		status  int
		body    string
		wantErr bool
	}{
		{"accepted", http.StatusOK, "", false},
		{"rejected as bad request", http.StatusBadRequest, "uploadFile is required", true},
		{"failed on the server", http.StatusInternalServerError, "storage is unavailable", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			type upload struct {
				id      string
				hasFile bool
			}
			received := make(chan upload, 1)
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_, _, err := r.FormFile("uploadFile")
				received <- upload{id: r.URL.Query().Get("id"), hasFile: err == nil}
				w.WriteHeader(tt.status)
				_, _ = w.Write([]byte(tt.body))
			}))
			defer srv.Close()

			workdir := t.TempDir()
			artifactDir := filepath.Join(workdir, "artifacts", taskUUID)
			if err := os.MkdirAll(artifactDir, 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(artifactDir, "pkg_1.0_amd64.deb"), []byte("deb"), 0o644); err != nil {
				t.Fatal(err)
			}

			prev := irgshConfig
			t.Cleanup(func() { irgshConfig = prev })
			irgshConfig.Builder.Workdir = workdir
			irgshConfig.Chief.Address = srv.URL

			payload, err := json.Marshal(map[string]string{"taskUUID": taskUUID})
			if err != nil {
				t.Fatal(err)
			}

			next, err := StorePackage(context.Background(), string(payload))

			got := <-received
			if got.id != taskUUID || !got.hasFile {
				t.Errorf("chief received id=%q file=%v, want id=%q with the artifact archive", got.id, got.hasFile, taskUUID)
			}

			if tt.wantErr {
				if err == nil {
					t.Fatalf("StorePackage() = nil error after chief answered %d, want an error", tt.status)
				}
				if next != "" {
					t.Errorf("StorePackage() next = %q after a rejected upload, want empty", next)
				}
				buildLog, readErr := os.ReadFile(filepath.Join(artifactDir, "build.log"))
				if readErr != nil {
					t.Fatal(readErr)
				}
				if !strings.Contains(string(buildLog), tt.body) {
					t.Errorf("build.log does not carry chief's response %q", tt.body)
				}
				return
			}

			if err != nil {
				t.Fatalf("StorePackage() error = %v, want nil", err)
			}
			if next != string(payload) {
				t.Errorf("StorePackage() next = %q, want the payload", next)
			}
		})
	}
}
