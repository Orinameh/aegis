package logger

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go.uber.org/zap"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.Level != "info" {
		t.Errorf("expected level info, got %q", cfg.Level)
	}
	if cfg.Encoding != "json" {
		t.Errorf("expected encoding json, got %q", cfg.Encoding)
	}
	if cfg.OutputPath != "stdout" {
		t.Errorf("expected output stdout, got %q", cfg.OutputPath)
	}
	if cfg.Development {
		t.Error("expected Development=false by default")
	}
}

func TestNewReturnsUsableLogger(t *testing.T) {
	cfg := &Config{
		Level:      "debug",
		Encoding:   "console",
		OutputPath: "stdout",
	}

	log, err := New(cfg)
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}

	log.Debug("debug message", zap.String("key", "value"))
	log.Info("info message")
	log.Warn("warn message")
	log.Error("error message")
	_ = log.Sync()
}

func TestNewWritesJSONToFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.log")

	log, err := New(&Config{
		Level:      "info",
		Encoding:   "json",
		OutputPath: path,
	})
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}

	log.Info("hello world", zap.String("k", "v"))
	if err := log.Sync(); err != nil {
		t.Fatalf("Sync returned error: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read log file: %v", err)
	}
	content := string(data)

	for _, want := range []string{`"message":"hello world"`, `"k":"v"`, `"level":"info"`} {
		if !strings.Contains(content, want) {
			t.Errorf("log file missing %s, got:\n%s", want, content)
		}
	}
}

func TestNewUnknownLevelFallsBackToInfo(t *testing.T) {
	log, err := New(&Config{
		Level:      "bogus",
		Encoding:   "json",
		OutputPath: "stdout",
	})
	if err != nil {
		t.Fatalf("New returned error for unknown level: %v", err)
	}
	if log == nil {
		t.Fatal("expected non-nil logger")
	}
}

func TestNewInvalidEncodingFails(t *testing.T) {
	_, err := New(&Config{
		Level:      "info",
		Encoding:   "bogus-encoding",
		OutputPath: "stdout",
	})
	if err == nil {
		t.Fatal("expected error for unknown encoding, got nil")
	}
}

func TestMustNewPanicsOnInvalidConfig(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected MustNew to panic on invalid encoding")
		}
	}()

	MustNew(&Config{
		Level:      "info",
		Encoding:   "not-an-encoding",
		OutputPath: "stdout",
	})
}

func TestSetupLogFileCreatesDirectory(t *testing.T) {
	root := t.TempDir()
	logDir := filepath.Join(root, "nested", "logs")

	path, err := SetupLogFile(logDir)
	if err != nil {
		t.Fatalf("SetupLogFile returned error: %v", err)
	}

	if path != filepath.Join(logDir, "aegis-audit.log") {
		t.Errorf("unexpected path: %q", path)
	}

	info, err := os.Stat(logDir)
	if err != nil {
		t.Fatalf("expected log dir to be created: %v", err)
	}
	if !info.IsDir() {
		t.Error("expected log dir to be a directory")
	}
}
