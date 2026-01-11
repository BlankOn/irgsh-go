package notification

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"
)

// WebhookPayload represents the notification payload sent to webhook
type WebhookPayload struct {
	Title   string `json:"title"`
	Message string `json:"message"`
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

// SendJobNotification sends a job completion notification
func SendJobNotification(webhookURL, jobType, taskUUID, status, details string) {
	title := fmt.Sprintf("IRGSH %s Job %s", jobType, status)

	// Add emoji suffix based on status
	var emoji string
	switch status {
	case "SUCCESS", "DONE":
		emoji = " ✅"
	case "FAILED":
		emoji = " ❌"
	}

	// Format: Job ID: xxx - build status: SUCCESS ✅
	jobTypeLower := "build"
	if jobType == "Repo" {
		jobTypeLower = "repo"
	}
	message := fmt.Sprintf("Job ID: %s - %s status: %s%s", taskUUID, jobTypeLower, status, emoji)

	if err := SendWebhook(webhookURL, title, message); err != nil {
		log.Printf("Failed to send job notification: %v", err)
	}
}
