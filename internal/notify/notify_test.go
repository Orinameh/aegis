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

func TestBuildRequestBody(t *testing.T) {
	t.Run("json providers", func(t *testing.T) {
		body, contentType, err := buildRequestBody("discord", "hello")
		if err != nil {
			t.Fatalf("buildRequestBody returned error: %v", err)
		}
		if !strings.Contains(contentType, "application/json") {
			t.Errorf("expected JSON content type, got %q", contentType)
		}
		var payload map[string]any
		if err := json.Unmarshal(body, &payload); err != nil {
			t.Fatalf("body is not valid JSON: %v", err)
		}
		if payload["content"] != "hello" {
			t.Errorf("expected content=hello, got %v", payload)
		}
	})

	t.Run("ntfy uses plain text body", func(t *testing.T) {
		body, contentType, err := buildRequestBody("ntfy", "disk almost full")
		if err != nil {
			t.Fatalf("buildRequestBody returned error: %v", err)
		}
		if !strings.HasPrefix(contentType, "text/plain") {
			t.Errorf("expected text/plain content type, got %q", contentType)
		}
		if string(body) != "disk almost full" {
			t.Errorf("expected raw message body, got %q", string(body))
		}
	})
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

func TestNotifyPostsNtfyPlainText(t *testing.T) {
	var gotContentType string
	var gotBody []byte

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotContentType = r.Header.Get("Content-Type")
		gotBody, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	// ntfy's primary publish API is a plain-text body posted to the topic
	// URL; JSON publishing would require the root URL plus a "topic" field.
	n := New(Config{
		Enabled:    true,
		WebhookURL: srv.URL + "/mytopic",
		Provider:   "ntfy",
		Timeout:    5 * time.Second,
	})
	if err := n.Notify("disk at 90%"); err != nil {
		t.Fatalf("Notify returned error: %v", err)
	}

	if !strings.HasPrefix(gotContentType, "text/plain") {
		t.Errorf("expected text/plain content type, got %q", gotContentType)
	}
	if string(gotBody) != "disk at 90%" {
		t.Errorf("expected raw message body, got %q", string(gotBody))
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
