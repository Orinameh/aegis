package notify

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestBuildPayload(t *testing.T) {
	cases := []struct {
		provider string
		key      string
	}{
		{"slack", "text"},
		{"generic", "text"},
		{"", "text"},
		{"discord", "content"},
		{"ntfy", "message"},
	}
	for _, c := range cases {
		p, err := buildPayload(c.provider, "hello")
		if err != nil {
			t.Fatalf("%q: %v", c.provider, err)
		}
		data, _ := json.Marshal(p)
		if p[c.key] != "hello" {
			t.Errorf("%q: expected key %q, got %s", c.provider, c.key, string(data))
		}
	}

	if _, err := buildPayload("bogus", "x"); err == nil {
		t.Error("expected error for unknown provider")
	}
}

func TestNotifyDisabledDoesNothing(t *testing.T) {
	n := New(Config{Enabled: false, WebhookURL: "http://example.com"})
	if err := n.Notify("hello"); err != nil {
		t.Fatalf("expected no error when disabled, got %v", err)
	}
}

func TestNotifyMissingURLFails(t *testing.T) {
	n := New(Config{Enabled: true, WebhookURL: ""})
	if err := n.Notify("hello"); err == nil {
		t.Fatal("expected error when webhook URL is empty")
	}
}

func TestNotifyPostsPayload(t *testing.T) {
	var gotMethod, gotContentType string
	var gotBody []byte

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotContentType = r.Header.Get("Content-Type")
		gotBody, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	n := New(Config{
		Enabled:    true,
		WebhookURL: srv.URL,
		Provider:   "discord",
		Timeout:    5 * time.Second,
	})
	if err := n.Notify("alert message"); err != nil {
		t.Fatalf("Notify returned error: %v", err)
	}

	if gotMethod != http.MethodPost {
		t.Errorf("expected POST, got %s", gotMethod)
	}
	if !strings.Contains(gotContentType, "application/json") {
		t.Errorf("expected JSON content type, got %q", gotContentType)
	}

	var payload map[string]any
	if err := json.Unmarshal(gotBody, &payload); err != nil {
		t.Fatalf("response body is not valid JSON: %v", err)
	}
	if payload["content"] != "alert message" {
		t.Errorf("expected content=%q, got %q", "alert message", payload["content"])
	}
}

func TestNotifyNonSuccessStatusFails(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusBadGateway)
	}))
	defer srv.Close()

	n := New(Config{Enabled: true, WebhookURL: srv.URL, Provider: "slack"})
	if err := n.Notify("hello"); err == nil {
		t.Fatal("expected error for non-2xx response")
	}
}

func TestNotifyUnreachableServerFails(t *testing.T) {
	n := New(Config{
		Enabled:    true,
		WebhookURL: "http://127.0.0.1:1/does-not-exist",
		Timeout:    500 * time.Millisecond,
	})
	if err := n.Notify("hello"); err == nil {
		t.Fatal("expected error for unreachable webhook")
	}
}
