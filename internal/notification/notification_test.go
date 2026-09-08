package notification

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// captureNotification runs SendJobNotification against a stub webhook and
// returns the message it posted.
func captureNotification(t *testing.T, jobType, status string, jobInfo JobNotificationInfo) string {
	t.Helper()

	var got WebhookPayload
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Errorf("bad payload: %v", err)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	SendJobNotification(srv.URL, jobType, "task-uuid", status, jobInfo)
	return got.Message
}

func TestJobNotificationCarriesDistAndComponent(t *testing.T) {
	msg := captureNotification(t, "Build", "SUCCESS", JobNotificationInfo{
		PackageName:    "blankres",
		PackageVersion: "0.2.4",
		Maintainer:     "Herpiko Dwi Aguno <herpiko@gmail.com>",
		Dist:           "verbeek",
		Component:      "main",
	})

	if !strings.Contains(msg, "[verbeek/main]") {
		t.Errorf("message does not name the suite and component: %s", msg)
	}
	if !strings.HasPrefix(msg, "📦 irgsh-builder: blankres_0.2.4 ") {
		t.Errorf("unexpected message: %s", msg)
	}
}

func TestJobNotificationExperimentalIsASuiteNotAComponent(t *testing.T) {
	msg := captureNotification(t, "Repo", "SUCCESS", JobNotificationInfo{
		PackageName:    "blankres",
		PackageVersion: "0.2.4",
		Dist:           "sinambung",
		Component:      "extras",
		IsExperimental: true,
	})

	// The experimental split is the suite <dist>-experimental; the component
	// is still whatever the maintainer submitted.
	if !strings.Contains(msg, "[sinambung-experimental/extras]") {
		t.Errorf("unexpected message: %s", msg)
	}
}

func TestJobNotificationWithoutDistOrComponent(t *testing.T) {
	// An ISO job carries neither; the tag is dropped rather than rendered
	// empty.
	msg := captureNotification(t, "ISO Build", "SUCCESS", JobNotificationInfo{
		PackageName: "ISO Image",
	})

	if strings.Contains(msg, "[") {
		t.Errorf("empty target should not be rendered: %s", msg)
	}
}

func TestJobNotificationDistOnly(t *testing.T) {
	msg := captureNotification(t, "Build", "SUCCESS", JobNotificationInfo{
		PackageName:    "blankres",
		PackageVersion: "0.2.4",
		Dist:           "verbeek",
	})

	if !strings.Contains(msg, "[verbeek]") {
		t.Errorf("unexpected message: %s", msg)
	}
}
