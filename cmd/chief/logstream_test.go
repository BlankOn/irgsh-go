package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/blankon/irgsh-go/internal/logstream"
)

// stubSource replays a fixed set of messages as if they came from Redis.
type stubSource struct {
	messages []logstream.Message
	gotUUID  string
	gotType  string
}

func (s *stubSource) Follow(_ context.Context, taskUUID, logType string, onMessage func(logstream.Message) error) error {
	s.gotUUID = taskUUID
	s.gotType = logType
	for _, msg := range s.messages {
		if err := onMessage(msg); err != nil {
			return err
		}
	}
	return nil
}

func TestLogStreamHandler_RejectsBadParams(t *testing.T) {
	handler := logStreamHandler(t.TempDir(), &stubSource{})

	cases := map[string]string{
		"missing id":     "/api/v1/log-stream?type=build",
		"unsafe id":      "/api/v1/log-stream?id=../../etc/passwd&type=build",
		"missing type":   "/api/v1/log-stream?id=abc",
		"unknown type":   "/api/v1/log-stream?id=abc&type=shadow",
		"traversal type": "/api/v1/log-stream?id=abc&type=../build",
	}

	for name, target := range cases {
		t.Run(name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			handler(rec, httptest.NewRequest(http.MethodGet, target, nil))
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("expected 400, got %d", rec.Code)
			}
		})
	}
}

func TestLogStreamHandler_ServesFinishedLogFromDisk(t *testing.T) {
	logsDir := t.TempDir()
	logFile := filepath.Join(logsDir, "task-1.build.log")
	if err := os.WriteFile(logFile, []byte("first line\nsecond line\n"), 0644); err != nil {
		t.Fatal(err)
	}

	source := &stubSource{messages: []logstream.Message{{Seq: 1, Line: "from redis"}}}
	rec := httptest.NewRecorder()
	logStreamHandler(logsDir, source)(rec, httptest.NewRequest(http.MethodGet, "/api/v1/log-stream?id=task-1&type=build", nil))

	body := rec.Body.String()
	if !strings.Contains(body, "data: first line") || !strings.Contains(body, "data: second line") {
		t.Fatalf("expected the log file contents, got:\n%s", body)
	}
	if strings.Contains(body, "from redis") {
		t.Fatal("an uploaded log file must be preferred over the Redis backlog")
	}
	if !strings.Contains(body, "event: end") {
		t.Fatalf("expected an end event, got:\n%s", body)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "text/event-stream" {
		t.Fatalf("unexpected content type: %q", ct)
	}
	if buffering := rec.Header().Get("X-Accel-Buffering"); buffering != "no" {
		t.Fatalf("expected proxy buffering to be disabled, got %q", buffering)
	}
}

func TestLogStreamHandler_StreamsRunningJob(t *testing.T) {
	source := &stubSource{messages: []logstream.Message{
		{Seq: 1, Line: "##### Downloading the artifact"},
		{Seq: 2, Line: "done"},
		{Seq: 3, End: true},
	}}

	rec := httptest.NewRecorder()
	logStreamHandler(t.TempDir(), source)(rec, httptest.NewRequest(http.MethodGet, "/api/v1/log-stream?id=task-2&type=repo", nil))

	body := rec.Body.String()
	for _, want := range []string{"id: 1", "data: ##### Downloading the artifact", "id: 2", "data: done", "event: end"} {
		if !strings.Contains(body, want) {
			t.Fatalf("expected %q in response, got:\n%s", want, body)
		}
	}
	if source.gotUUID != "task-2" || source.gotType != "repo" {
		t.Fatalf("subscribed to the wrong log: %s.%s", source.gotUUID, source.gotType)
	}
}

func TestLogStreamHandler_ResumesFromLastEventID(t *testing.T) {
	source := &stubSource{messages: []logstream.Message{
		{Seq: 1, Line: "already seen"},
		{Seq: 2, Line: "also seen"},
		{Seq: 3, Line: "new line"},
	}}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/log-stream?id=task-3&type=build", nil)
	req.Header.Set("Last-Event-ID", "2")
	rec := httptest.NewRecorder()
	logStreamHandler(t.TempDir(), source)(rec, req)

	body := rec.Body.String()
	if strings.Contains(body, "already seen") || strings.Contains(body, "also seen") {
		t.Fatalf("replayed lines the client already had:\n%s", body)
	}
	if !strings.Contains(body, "data: new line") {
		t.Fatalf("expected the unseen line, got:\n%s", body)
	}
}

func TestLogStreamHandler_MultilineMessageStaysOneEvent(t *testing.T) {
	source := &stubSource{messages: []logstream.Message{{Seq: 1, Line: "line one\nline two"}}}

	rec := httptest.NewRecorder()
	logStreamHandler(t.TempDir(), source)(rec, httptest.NewRequest(http.MethodGet, "/api/v1/log-stream?id=task-4&type=build", nil))

	if !strings.Contains(rec.Body.String(), "data: line one\ndata: line two\n\n") {
		t.Fatalf("embedded newlines must not split the event:\n%s", rec.Body.String())
	}
}

func TestLogStreamHandler_WithoutRedis(t *testing.T) {
	rec := httptest.NewRecorder()
	logStreamHandler(t.TempDir(), nil)(rec, httptest.NewRequest(http.MethodGet, "/api/v1/log-stream?id=task-5&type=build", nil))

	body := rec.Body.String()
	if !strings.Contains(body, "unavailable") {
		t.Fatalf("expected an explanation that streaming is unavailable, got:\n%s", body)
	}
	if !strings.Contains(body, "event: end") {
		t.Fatal("the browser must be told the stream is over")
	}
}

// The viewer page lives under the same prefix as the static log file server,
// so the more specific route must win.
func TestLogStreamRouteBeatsStaticLogFileServer(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/logs/stream", func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte("viewer"))
	})
	mux.Handle("/logs/", http.StripPrefix("/logs/", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte("static"))
	})))

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/logs/stream?id=abc&type=build", nil))
	if got := rec.Body.String(); got != "viewer" {
		t.Fatalf("expected the viewer page, got %q", got)
	}

	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/logs/abc.build.log", nil))
	if got := rec.Body.String(); got != "static" {
		t.Fatalf("expected the static log file, got %q", got)
	}
}

// The dashboard is registered as a catch-all, so an unknown path must not be
// answered with HTML: a client decoding JSON would report a parse error
// instead of a missing endpoint.
func TestIndexHandler_UnknownPaths(t *testing.T) {
	cases := []struct {
		path       string
		wantStatus int
		wantJSON   bool
	}{
		{"/api/v1/import", http.StatusNotFound, true},
		{"/api/v1/nope", http.StatusNotFound, true},
		{"/whatever", http.StatusNotFound, false},
	}

	for _, tc := range cases {
		t.Run(tc.path, func(t *testing.T) {
			rec := httptest.NewRecorder()
			indexHandler(rec, httptest.NewRequest(http.MethodPost, tc.path, nil))

			if rec.Code != tc.wantStatus {
				t.Fatalf("expected %d, got %d", tc.wantStatus, rec.Code)
			}
			if strings.Contains(rec.Body.String(), "<!DOCTYPE") || strings.Contains(rec.Body.String(), "<html") {
				t.Fatalf("an unknown path must not return the dashboard:\n%s", rec.Body.String())
			}
			isJSON := strings.Contains(rec.Header().Get("Content-Type"), "json")
			if isJSON != tc.wantJSON {
				t.Fatalf("expected JSON=%v for %s, got %q", tc.wantJSON, tc.path, rec.Header().Get("Content-Type"))
			}
		})
	}
}
