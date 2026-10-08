package httputil

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestVersionHandlerReturnsVersion(t *testing.T) {
	recorder := httptest.NewRecorder()
	VersionHandler("2.3.2").ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/v1/version", nil))

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}
	if got := recorder.Header().Get("Content-Type"); got != "application/json" {
		t.Fatalf("Content-Type = %q, want application/json", got)
	}
	if got := recorder.Body.String(); got != `{"version":"2.3.2"}` {
		t.Fatalf("body = %q", got)
	}
}

func TestVersionHandlerRejectsPost(t *testing.T) {
	recorder := httptest.NewRecorder()
	VersionHandler("2.3.2").ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/api/v1/version", nil))

	if recorder.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusMethodNotAllowed)
	}
}
