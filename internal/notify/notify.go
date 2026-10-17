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

	body, contentType, err := buildRequestBody(n.cfg.Provider, message)
	if err != nil {
		return fmt.Errorf("failed to build notification payload: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), n.client.Timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, n.cfg.WebhookURL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("failed to create notification request: %w", err)
	}
	req.Header.Set("Content-Type", contentType)

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

// buildRequestBody returns the HTTP body and Content-Type for the given
// provider.
//
// Slack, Discord, and generic webhooks receive a small structured JSON
// payload. ntfy is special-cased: its JSON publishing API requires POSTing to
// the service root URL with a "topic" field, so posting JSON to the
// topic-scoped webhook_url (the documented setup) is rejected. Sending the
// message as a plain-text body to the topic URL is ntfy's primary publishing
// API and matches how webhook_url is configured.
func buildRequestBody(provider, message string) ([]byte, string, error) {
	if provider == "ntfy" {
		return []byte(message), "text/plain; charset=utf-8", nil
	}

	payload, err := buildPayload(provider, message)
	if err != nil {
		return nil, "", err
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, "", fmt.Errorf("failed to encode notification payload: %w", err)
	}
	return body, "application/json", nil
}

// buildPayload shapes the JSON body for the given provider. All JSON
// providers accept a simple JSON POST to a webhook URL.
func buildPayload(provider, message string) (map[string]any, error) {
	switch provider {
	case "slack", "generic", "":
		return map[string]any{"text": message}, nil
	case "discord":
		return map[string]any{"content": message}, nil
	default:
		return nil, fmt.Errorf("unknown notification provider %q", provider)
	}
}
