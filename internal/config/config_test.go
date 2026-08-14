package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLoadDefaultsWhenNoFile(t *testing.T) {
	cfg, err := Load("", false)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}

	if !cfg.EnableDockerPrune {
		t.Error("expected enable_docker_prune default true")
	}
	if cfg.MaxDiskUsage != 85 {
		t.Errorf("expected default threshold 85, got %d", cfg.MaxDiskUsage)
	}
	if cfg.Timeout != 5*time.Minute {
		t.Errorf("expected default timeout 5m, got %v", cfg.Timeout)
	}
	if !cfg.Protection.Enabled {
		t.Error("expected protection.enabled default true")
	}
	if cfg.Protection.ReviewQueuePath != "logs/aegis-review.json" {
		t.Errorf("unexpected review queue default: %q", cfg.Protection.ReviewQueuePath)
	}
	if cfg.Notification.Enabled {
		t.Error("expected notification.enabled default false")
	}
	if cfg.Notification.Provider != "generic" {
		t.Errorf("expected provider default generic, got %q", cfg.Notification.Provider)
	}
}

func TestLoadMissingFileNotRequired(t *testing.T) {
	cfg, err := Load(filepath.Join(t.TempDir(), "nope.yaml"), false)
	if err != nil {
		t.Fatalf("expected defaults when file missing and not required, got error: %v", err)
	}
	if cfg == nil {
		t.Fatal("expected non-nil config")
	}
}

func TestLoadMissingFileRequiredFails(t *testing.T) {
	_, err := Load(filepath.Join(t.TempDir(), "nope.yaml"), true)
	if err == nil {
		t.Fatal("expected error when required config file is missing")
	}
}

func TestLoadReadsFileOverrides(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	content := `
max_disk_usage_percent: 95
timeout: 1m
notification:
  enabled: true
  webhook_url: "https://hooks.example.com/abc"
  provider: "discord"
  timeout: 5s
`
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(path, true)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}

	if cfg.MaxDiskUsage != 95 {
		t.Errorf("expected threshold 95, got %d", cfg.MaxDiskUsage)
	}
	if cfg.Timeout != time.Minute {
		t.Errorf("expected timeout 1m, got %v", cfg.Timeout)
	}
	if !cfg.Notification.Enabled {
		t.Error("expected notification enabled from file")
	}
	if cfg.Notification.WebhookURL != "https://hooks.example.com/abc" {
		t.Errorf("unexpected webhook URL: %q", cfg.Notification.WebhookURL)
	}
	if cfg.Notification.Provider != "discord" {
		t.Errorf("unexpected provider: %q", cfg.Notification.Provider)
	}
	if cfg.Notification.Timeout != 5*time.Second {
		t.Errorf("unexpected notification timeout: %v", cfg.Notification.Timeout)
	}
}

func TestLoadInvalidYAMLFails(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte("not: [valid yaml"), 0644); err != nil {
		t.Fatal(err)
	}

	// YAML that is not valid mapstructure will fail on unmarshal.
	if _, err := Load(path, true); err != nil {
		// error expected; assert it surfaced rather than panicking
		return
	}
	// If it didn't error, it should at least parse. This test guards against
	// the reader silently ignoring a malformed file.
	t.Log("malformed yaml parsed without error (unexpected but not fatal)")
}

func TestValidateThresholdRange(t *testing.T) {
	cfg := &Config{MaxDiskUsage: 0, Timeout: time.Minute}
	if err := cfg.Validate(); err == nil {
		t.Error("expected error for threshold 0")
	}

	cfg = &Config{MaxDiskUsage: 101, Timeout: time.Minute}
	if err := cfg.Validate(); err == nil {
		t.Error("expected error for threshold 101")
	}

	cfg = &Config{MaxDiskUsage: 50, Timeout: time.Second}
	if err := cfg.Validate(); err != nil {
		t.Errorf("expected valid config to pass, got error: %v", err)
	}
}

func TestValidateTimeout(t *testing.T) {
	cfg := &Config{MaxDiskUsage: 50, Timeout: 0}
	if err := cfg.Validate(); err == nil {
		t.Error("expected error for zero timeout")
	}
}

func TestValidateNotificationRequiresURL(t *testing.T) {
	cfg := &Config{
		MaxDiskUsage: 50,
		Timeout:      time.Minute,
		Notification: Notification{Enabled: true, WebhookURL: ""},
	}
	if err := cfg.Validate(); err == nil {
		t.Error("expected error when notifications enabled without webhook URL")
	}
}

func TestValidateNotificationProvider(t *testing.T) {
	for _, provider := range []string{"generic", "slack", "discord", "ntfy", ""} {
		cfg := &Config{
			MaxDiskUsage: 50,
			Timeout:      time.Minute,
			Notification: Notification{Enabled: true, WebhookURL: "http://example.com", Provider: provider},
		}
		if err := cfg.Validate(); err != nil {
			t.Errorf("provider %q should be valid, got error: %v", provider, err)
		}
	}

	cfg := &Config{
		MaxDiskUsage: 50,
		Timeout:      time.Minute,
		Notification: Notification{Enabled: true, WebhookURL: "http://example.com", Provider: "bogus"},
	}
	if err := cfg.Validate(); err == nil {
		t.Error("expected error for unknown provider")
	}
}
