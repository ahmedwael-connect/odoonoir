package notify

import (
	"testing"

	"github.com/ahmed/odoonoir/internal/config"
)

func TestNotifyNoWebhook(t *testing.T) {
	cfg := &config.Config{InstancesRoot: t.TempDir()}
	if err := Notify(cfg, "test", "title", "msg"); err != nil {
		t.Fatalf("Notify without webhook should not error: %v", err)
	}
}

func TestNotifyWithWebhookInvalidURL(t *testing.T) {
	cfg := &config.Config{InstancesRoot: t.TempDir(), WebhookURL: "http://127.0.0.1:1/invalid"}
	// should not panic, may error due to connection refused but not fatal for test
	_ = Notify(cfg, "test", "title", "msg")
}
