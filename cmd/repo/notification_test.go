package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/blankon/irgsh-go/internal/config"
	"github.com/blankon/irgsh-go/internal/notification"
)

// A repo task that fails before it gets anywhere near reprepro must still
// notify. It used to return early - on a dist mismatch, or on a log file it
// could not create - before the notification defer was even registered, so
// those jobs failed in silence while the builder stage notified normally.
func TestRepoNotifiesOnEarlyFailure(t *testing.T) {
	payload := func(dist string) string {
		b, err := json.Marshal(map[string]interface{}{
			"taskUUID":       "task-1",
			"dist":           dist,
			"packageName":    "blankres",
			"packageVersion": "0.1.0",
			"maintainer":     "Someone",
			"isExperimental": false,
		})
		if err != nil {
			t.Fatal(err)
		}
		return string(b)
	}

	// A regular file where a directory is expected, so the log file cannot be
	// prepared however the workdir is laid out underneath it.
	blocked := filepath.Join(t.TempDir(), "not-a-dir")
	if err := os.WriteFile(blocked, []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name    string
		dist    string
		workdir string
	}{
		{"dist mismatch", "sid", t.TempDir()},
		{"unpreparable log file", "verbeek", blocked},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var got []notification.WebhookPayload
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body, _ := io.ReadAll(r.Body)
				var p notification.WebhookPayload
				if err := json.Unmarshal(body, &p); err != nil {
					t.Errorf("webhook received malformed body %q: %v", body, err)
				}
				got = append(got, p)
			}))
			defer srv.Close()

			irgshConfig = config.IrgshConfig{
				Repo:         config.RepoConfig{Workdir: tc.workdir, DistCodename: "verbeek"},
				Notification: config.NotificationConfig{WebhookURL: srv.URL},
			}

			if err := Repo(payload(tc.dist)); err == nil {
				t.Fatal("expected the task to fail")
			}

			if len(got) != 1 {
				t.Fatalf("expected exactly one webhook, got %d: %+v", len(got), got)
			}
			if !strings.Contains(got[0].Title, "Repo Job FAILED") {
				t.Errorf("unexpected title: %q", got[0].Title)
			}
			if !strings.Contains(got[0].Message, "blankres_0.1.0") {
				t.Errorf("message lost the package identity: %q", got[0].Message)
			}
		})
	}
}
