package notify

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Config holds notification configuration.
type Config struct {
	Enabled    bool
	WebhookURL string
	Provider   string // generic, slack, discord, ntfy
	Timeout    time.Duration
}

// Notifier sends alert messages to a webhook endpoint.
type Notifier struct {
	cfg    Config
	client *http.Client
}

// New creates a Notifier from the given config.
func New(cfg Config) *Notifier {
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = 10 * time.Second
	}

	return &Notifier{
		cfg:    cfg,
		client: &http.Client{Timeout: timeout},
	}
}

// Notify sends a message to the configured webhook. It returns an error if
// notifications are disabled, no webhook URL is set, or the request fails.
func (n *Notifier) Notify(message string) error {
	if !n.cfg.Enabled {
		return nil
	}
	if n.cfg.WebhookURL == "" {
		return fmt.Errorf("notification webhook URL is not configured")
	}

	payload, err := buildPayload(n.cfg.Provider, message)
	if err != nil {
		return fmt.Errorf("failed to build notification payload: %w", err)
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to encode notification payload: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), n.client.Timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, n.cfg.WebhookURL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("failed to create notification request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := n.client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send notification: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("notification webhook returned %s: %s", resp.Status, string(respBody))
	}

	return nil
}

// buildPayload shapes the JSON body for the given provider. All providers
// accept a simple JSON POST to a webhook URL.
func buildPayload(provider, message string) (map[string]any, error) {
	switch provider {
	case "slack", "generic", "":
		return map[string]any{"text": message}, nil
	case "discord":
		return map[string]any{"content": message}, nil
	case "ntfy":
		return map[string]any{"message": message}, nil
	default:
		return nil, fmt.Errorf("unknown notification provider %q", provider)
	}
}
