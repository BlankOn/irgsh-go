package repository

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/blankon/irgsh-go/internal/cli/domain"
)

type stubConfigStore struct{ addr string }

func (s stubConfigStore) Load() (domain.Config, error) {
	return domain.Config{ChiefAddress: s.addr}, nil
}

func importSubmission() domain.ImportSubmission {
	return domain.ImportSubmission{
		SourceURL:    "https://kartolo.sby.datautama.net.id/debian/",
		Dist:         "sid",
		PackageNames: []string{"firefox"},
	}
}

// An irgsh-chief older than the CLI answers an endpoint it does not know from
// its dashboard catch-all: HTML with HTTP 200.
func TestSubmitImport_AgainstAChiefWithoutTheEndpoint(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprint(w, "<!DOCTYPE html>\n<html><body>dashboard</body></html>")
	}))
	defer server.Close()

	_, err := NewHTTPChiefClient(stubConfigStore{addr: server.URL}).SubmitImport(context.Background(), importSubmission())
	if err == nil {
		t.Fatal("expected an error for an HTML response")
	}
	for _, want := range []string{"web page instead of JSON", "/api/v1/import", "older than your irgsh-cli"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("expected %q in %q", want, err.Error())
		}
	}
	if strings.Contains(err.Error(), "invalid character") {
		t.Fatalf("the raw JSON parse error must not be what the user sees: %v", err)
	}
}

func TestSubmitImport_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/import" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"pipelineId":"2026-09-03-005041_abc_import"}`)
	}))
	defer server.Close()

	resp, err := NewHTTPChiefClient(stubConfigStore{addr: server.URL}).SubmitImport(context.Background(), importSubmission())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.PipelineID != "2026-09-03-005041_abc_import" {
		t.Fatalf("unexpected pipeline ID: %s", resp.PipelineID)
	}
}

func TestDecodeJSON_NonJSONNonHTML(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		fmt.Fprint(w, "502 Bad Gateway")
	}))
	defer server.Close()

	_, err := NewHTTPChiefClient(stubConfigStore{addr: server.URL}).GetImportStatus(context.Background(), "some-id")
	if err == nil {
		t.Fatal("expected an error for a non-JSON response")
	}
	for _, want := range []string{"unreadable response", "/api/v1/import-status", "text/plain"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("expected %q in %q", want, err.Error())
		}
	}
}
