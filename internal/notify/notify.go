// Package notify delivers odoonoir events to the user: a desktop
// notification via notify-send when available, or a JSON POST to a
// configured webhook. Without either, Notify is a no-op.
package notify

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os/exec"
	"time"

	"github.com/ahmed/odoonoir/internal/config"
)

type payload struct {
	Event     string `json:"event"`
	Title     string `json:"title"`
	Message   string `json:"message"`
	Timestamp string `json:"timestamp"`
}

// Notify sends a notification for the given event. It never blocks longer
// than 5 seconds (webhook path) and returns an error only when a configured
// channel fails.
func Notify(cfg *config.Config, event, title, message string) error {
	if cfg.WebhookURL != "" {
		return postWebhook(cfg.WebhookURL, payload{
			Event:     event,
			Title:     title,
			Message:   message,
			Timestamp: time.Now().Format(time.RFC3339),
		})
	}
	if _, err := exec.LookPath("notify-send"); err != nil {
		return nil // no desktop channel available
	}
	if err := exec.Command("notify-send", "-a", "odoonoir", title, message).Run(); err != nil {
		return fmt.Errorf("notify-send: %w", err)
	}
	return nil
}

func postWebhook(url string, p payload) error {
	body, err := json.Marshal(p)
	if err != nil {
		return err
	}
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Post(url, "application/json", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("webhook POST: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return fmt.Errorf("webhook returned %s", resp.Status)
	}
	return nil
}
