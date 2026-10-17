//go:build unix

package guard

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

// TestReviewQueueNewFileHonorsUmask guards the property that a freshly created
// review queue file is subject to the process umask, exactly like the plain
// os.WriteFile this package replaced — it must not be forced to a fixed mode.
func TestReviewQueueNewFileHonorsUmask(t *testing.T) {
	old := syscall.Umask(0o077)
	defer syscall.Umask(old)

	path := filepath.Join(t.TempDir(), "review.json")
	if err := NewReviewQueue(path).Enqueue(&ReviewEntry{Resource: "docker/container"}); err != nil {
		t.Fatalf("Enqueue returned error: %v", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("failed to stat queue file: %v", err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Errorf("expected umask 077 to yield mode 0600, got %04o", got)
	}
}
