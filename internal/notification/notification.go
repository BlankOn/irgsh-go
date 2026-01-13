package notification

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"regexp"
	"strings"
	"time"
)

// WebhookPayload represents the notification payload sent to webhook
type WebhookPayload struct {
	Title   string `json:"title"`
	Message string `json:"message"`
}

// JobNotificationInfo contains job details for notification
type JobNotificationInfo struct {
	PackageName     string
	PackageVersion  string
	Maintainer      string
	IsExperimental  bool
	SourceURL       string
	SourceBranch    string
	PackageURL      string
	PackageBranch   string
}

// SendWebhook sends a notification to the configured webhook URL
func SendWebhook(webhookURL, title, message string) error {
	if webhookURL == "" {
		log.Println("Notification webhook URL not configured, skipping notification")
		return nil
	}

	payload := WebhookPayload{
		Title:   title,
		Message: message,
	}

	jsonData, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal notification payload: %v", err)
	}

	client := &http.Client{
		Timeout: 10 * time.Second,
	}

	req, err := http.NewRequest("POST", webhookURL, bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("failed to create notification request: %v", err)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send notification: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("notification webhook returned non-success status: %d", resp.StatusCode)
	}

	log.Printf("Notification sent successfully: %s", title)
	return nil
}

// extractRepoName extracts username/repo from a git URL
// e.g., https://github.com/herpiko/foobar.git -> herpiko/foobar
func extractRepoName(url string) string {
	// Remove .git suffix if present
	url = strings.TrimSuffix(url, ".git")

	// Match github.com/username/repo or similar patterns
	re := regexp.MustCompile(`(?:github\.com|gitlab\.com|bitbucket\.org)[/:]([^/]+/[^/]+)$`)
	matches := re.FindStringSubmatch(url)
	if len(matches) > 1 {
		return matches[1]
	}

	// Fallback: try to get last two path segments
	parts := strings.Split(strings.TrimSuffix(url, "/"), "/")
	if len(parts) >= 2 {
		return parts[len(parts)-2] + "/" + parts[len(parts)-1]
	}

	return url
}

// SendJobNotification sends a job completion notification
func SendJobNotification(webhookURL, jobType, taskUUID, status string, jobInfo JobNotificationInfo) {
	title := fmt.Sprintf("IRGSH %s Job %s", jobType, status)

	// Add emoji based on status
	var emoji string
	switch status {
	case "SUCCESS", "DONE":
		emoji = "✅"
	case "FAILED":
		emoji = "❌"
	}

	// Determine target repo
	targetRepo := "dev"
	if jobInfo.IsExperimental {
		targetRepo = "experimental"
	}

	// Build source info (optional)
	sourceInfo := ""
	if jobInfo.SourceURL != "" {
		repoName := extractRepoName(jobInfo.SourceURL)
		if jobInfo.SourceBranch != "" {
			sourceInfo = fmt.Sprintf("%s (%s)", repoName, jobInfo.SourceBranch)
		} else {
			sourceInfo = repoName
		}
	}

	// Build package info (required, so always present)
	packageInfo := ""
	if jobInfo.PackageURL != "" {
		repoName := extractRepoName(jobInfo.PackageURL)
		if jobInfo.PackageBranch != "" {
			packageInfo = fmt.Sprintf("%s (%s)", repoName, jobInfo.PackageBranch)
		} else {
			packageInfo = repoName
		}
	}

	// Build repo links part
	repoLinks := ""
	if sourceInfo != "" && packageInfo != "" {
		repoLinks = fmt.Sprintf(", %s, %s", sourceInfo, packageInfo)
	} else if sourceInfo != "" {
		repoLinks = fmt.Sprintf(", %s", sourceInfo)
	} else if packageInfo != "" {
		repoLinks = fmt.Sprintf(", %s", packageInfo)
	}

	// Format: 📦 bromo-theme_1.0.0 [experimental] by Herpiko, herpiko/source (branch), herpiko/package (branch) ✅
	message := fmt.Sprintf("📦 %s_%s [%s] by %s%s %s",
		jobInfo.PackageName,
		jobInfo.PackageVersion,
		targetRepo,
		jobInfo.Maintainer,
		repoLinks,
		emoji,
	)

	// Always log the notification message for inspection
	log.Printf("Notification: %s - %s", title, message)

	if err := SendWebhook(webhookURL, title, message); err != nil {
		log.Printf("Failed to send job notification: %v", err)
	}
}
