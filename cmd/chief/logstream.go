package main

import (
	"bufio"
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/blankon/irgsh-go/internal/chief/domain"
	"github.com/blankon/irgsh-go/internal/logstream"
)

// logTypes are the job logs that can be streamed.
var logTypes = map[string]bool{"build": true, "repo": true, "iso": true, "import": true}

// logStreamSource reads job logs published by the workers. It is satisfied by
// *logstream.Subscriber, and stubbed in tests.
type logStreamSource interface {
	Follow(ctx context.Context, taskUUID, logType string, onMessage func(logstream.Message) error) error
}

// parseLogStreamParams validates the id and type query parameters shared by
// the log stream endpoint and the viewer page.
func parseLogStreamParams(r *http.Request) (id string, logType string, err error) {
	id = r.URL.Query().Get("id")
	if err := domain.ValidateID(id, "log id"); err != nil {
		return "", "", err
	}

	logType = r.URL.Query().Get("type")
	if !logTypes[logType] {
		return "", "", fmt.Errorf("invalid log type: %q", logType)
	}

	return id, logType, nil
}

// logStreamHandler streams a job log to the browser as Server-Sent Events.
//
// A finished job is served from the log file uploaded by the worker; a
// running job is served from Redis, where the worker mirrors every line as it
// is written.
func logStreamHandler(logsDir string, source logStreamSource) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, logType, err := parseLogStreamParams(r)
		if err != nil {
			writeJSONError(w, http.StatusBadRequest, err.Error())
			return
		}

		flusher, ok := w.(http.Flusher)
		if !ok {
			writeJSONError(w, http.StatusInternalServerError, "streaming unsupported")
			return
		}

		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		// nginx buffers proxied responses by default, which would hold every
		// line back until the job finishes.
		w.Header().Set("X-Accel-Buffering", "no")
		w.WriteHeader(http.StatusOK)
		flusher.Flush()

		// A reconnecting EventSource replays from the last line it saw, so
		// the viewer does not accumulate duplicates.
		var lastSeen int64
		if v, convErr := strconv.ParseInt(r.Header.Get("Last-Event-ID"), 10, 64); convErr == nil {
			lastSeen = v
		}

		send := func(event string, seq int64, data string) error {
			if event != "" {
				if _, err := fmt.Fprintf(w, "event: %s\n", event); err != nil {
					return err
				}
			}
			if seq > 0 {
				if _, err := fmt.Fprintf(w, "id: %d\n", seq); err != nil {
					return err
				}
			}
			// A line ending in the payload would terminate the event early,
			// so each line of data gets its own data: field.
			for _, part := range strings.Split(data, "\n") {
				if _, err := fmt.Fprintf(w, "data: %s\n", part); err != nil {
					return err
				}
			}
			if _, err := fmt.Fprint(w, "\n"); err != nil {
				return err
			}
			flusher.Flush()
			return nil
		}

		// A finished job has its complete log on disk; prefer it over the
		// capped backlog in Redis.
		logFile := filepath.Join(logsDir, id+"."+logType+".log")
		if f, err := os.Open(logFile); err == nil {
			defer f.Close()
			scanner := bufio.NewScanner(f)
			scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
			var seq int64
			for scanner.Scan() {
				seq++
				if seq <= lastSeen {
					continue
				}
				if err := send("", seq, scanner.Text()); err != nil {
					return
				}
			}
			if err := scanner.Err(); err != nil {
				_ = send("", 0, "[ chief ] failed to read log file: "+err.Error())
			}
			_ = send("end", 0, "")
			return
		}

		if source == nil {
			_ = send("", 0, "[ chief ] live log streaming is unavailable: chief could not connect to Redis")
			_ = send("end", 0, "")
			return
		}

		err = source.Follow(r.Context(), id, logType, func(msg logstream.Message) error {
			if msg.End {
				return send("end", 0, "")
			}
			if msg.Seq <= lastSeen {
				return nil
			}
			return send("", msg.Seq, msg.Line)
		})
		if err != nil && r.Context().Err() == nil {
			log.Printf("log stream for %s.%s ended: %v", id, logType, err)
			_ = send("", 0, "[ chief ] log stream ended: "+err.Error())
			_ = send("end", 0, "")
		}
	}
}

// logViewerHandler serves the page that displays a streamed job log.
func logViewerHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, logType, err := parseLogStreamParams(r)
		if err != nil {
			writeJSONError(w, http.StatusBadRequest, err.Error())
			return
		}

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if err := chiefService.RenderLogViewerHTML(w, id, logType); err != nil {
			log.Printf("log viewer render error: %v", err)
		}
	}
}
