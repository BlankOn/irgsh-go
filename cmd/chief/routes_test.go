package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/blankon/irgsh-go/internal/config"
)

type versionStub struct{ ChiefService }

func (versionStub) GetVersion() string { return "9.9.9" }

func TestSetupRoutesServesAPIAtRootAndUnderBaseURL(t *testing.T) {
	prev := chiefService
	chiefService = versionStub{}
	t.Cleanup(func() { chiefService = prev })

	tests := []struct {
		name    string
		baseURL string
		path    string
	}{
		{"worker path with base_url", "/irgsh", "/api/v1/version"},
		{"browser path with base_url", "/irgsh", "/irgsh/api/v1/version"},
		{"worker path without base_url", "", "/api/v1/version"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := config.IrgshConfig{Chief: config.ChiefConfig{BaseURL: tt.baseURL, Workdir: t.TempDir()}}
			srv := setupRoutes(cfg, nil)

			rec := httptest.NewRecorder()
			srv.Handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, tt.path, nil))

			if rec.Code != http.StatusOK {
				t.Fatalf("GET %s = %d, want %d", tt.path, rec.Code, http.StatusOK)
			}
			if !strings.Contains(rec.Body.String(), "9.9.9") {
				t.Errorf("GET %s body = %q, want version", tt.path, rec.Body.String())
			}
		})
	}
}
