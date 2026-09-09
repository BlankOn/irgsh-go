package repository

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/blankon/irgsh-go/internal/cli/domain"
)

type staticConfig struct{ addr string }

func (s staticConfig) Load() (domain.Config, error) {
	return domain.Config{ChiefAddress: s.addr}, nil
}

func writeTemp(t *testing.T, name string, size int) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), name)
	buf := make([]byte, size)
	for i := range buf {
		buf[i] = byte('a' + i%26)
	}
	if err := os.WriteFile(p, buf, 0600); err != nil {
		t.Fatal(err)
	}
	return p
}

// The streamed body must match the Content-Length the client declares, or the
// server sees a truncated (or over-long) multipart form.
func TestUploadSubmission_StreamsExactContentLength(t *testing.T) {
	var gotLen int64
	var blobSize, tokenSize int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotLen = r.ContentLength
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			t.Errorf("ParseMultipartForm: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		for field, dst := range map[string]*int{"blob": &blobSize, "token": &tokenSize} {
			f, _, err := r.FormFile(field)
			if err != nil {
				t.Errorf("FormFile(%q): %v", field, err)
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			b, _ := io.ReadAll(f)
			f.Close()
			*dst = len(b)
		}
		fmt.Fprint(w, `{"id":"abc"}`)
	}))
	defer srv.Close()

	blob := writeTemp(t, "blob.tar.gz", 3*1024*1024)
	token := writeTemp(t, "token.sig", 700)

	c := NewHTTPChiefClient(staticConfig{addr: srv.URL})
	var lastUploaded, lastTotal int64
	resp, err := c.UploadSubmission(context.Background(), blob, token, func(uploaded, total int64) {
		lastUploaded, lastTotal = uploaded, total
	})
	if err != nil {
		t.Fatalf("UploadSubmission: %v", err)
	}
	if resp.ID != "abc" {
		t.Fatalf("ID = %q, want abc", resp.ID)
	}
	if blobSize != 3*1024*1024 || tokenSize != 700 {
		t.Fatalf("part sizes = blob %d, token %d", blobSize, tokenSize)
	}
	if lastUploaded != lastTotal || lastTotal != gotLen {
		t.Fatalf("progress %d/%d, server saw Content-Length %d", lastUploaded, lastTotal, gotLen)
	}
}

// A dropped connection is retried; a 4xx is not.
func TestUploadSubmission_Retries(t *testing.T) {
	var calls int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt64(&calls, 1) == 1 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		io.Copy(io.Discard, r.Body)
		fmt.Fprint(w, `{"id":"ok"}`)
	}))
	defer srv.Close()

	c := NewHTTPChiefClient(staticConfig{addr: srv.URL})
	resp, err := c.UploadSubmission(context.Background(), writeTemp(t, "b", 1024), writeTemp(t, "t", 10), nil)
	if err != nil {
		t.Fatalf("UploadSubmission: %v", err)
	}
	if resp.ID != "ok" || atomic.LoadInt64(&calls) != 2 {
		t.Fatalf("id=%q calls=%d", resp.ID, calls)
	}
}

func TestUploadSubmission_NoRetryOn4xx(t *testing.T) {
	var calls int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&calls, 1)
		io.Copy(io.Discard, r.Body)
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()

	c := NewHTTPChiefClient(staticConfig{addr: srv.URL})
	if _, err := c.UploadSubmission(context.Background(), writeTemp(t, "b", 1024), writeTemp(t, "t", 10), nil); err == nil {
		t.Fatal("expected error")
	}
	if atomic.LoadInt64(&calls) != 1 {
		t.Fatalf("calls = %d, want 1 (4xx must not retry)", calls)
	}
}
