package guard

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// ReviewEntry is a single resource that was denied by a strict protection
// rule during a non-interactive run and is awaiting human review.
type ReviewEntry struct {
	Timestamp  time.Time    `json:"timestamp"`
	Resource   string       `json:"resource"`
	Type       ResourceType `json:"type"`
	Rule       string       `json:"rule"`
	Action     string       `json:"action"`
	Reason     string       `json:"reason"`
	ApprovedBy string       `json:"approved_by"`
}

// ReviewQueue persists denied-strict-protection items to a JSON file so
// unattended runs never block on stdin and the denial still gets surfaced
// to a human later (e.g. `aegis review`).
type ReviewQueue struct {
	path string
	mu   sync.Mutex
}

// NewReviewQueue creates a review queue backed by the given JSON file.
// A path is optional: a nil-able queue with an empty path disables enqueueing.
func NewReviewQueue(path string) *ReviewQueue {
	return &ReviewQueue{path: path}
}

// Enqueue appends an entry to the review queue file.
func (q *ReviewQueue) Enqueue(entry *ReviewEntry) error {
	if q == nil || q.path == "" {
		return nil
	}

	q.mu.Lock()
	defer q.mu.Unlock()

	entries, err := q.read()
	if err != nil {
		return fmt.Errorf("failed to read review queue: %w", err)
	}

	entries = append(entries, *entry)
	return q.write(entries)
}

// List returns all entries currently in the review queue.
func (q *ReviewQueue) List() ([]ReviewEntry, error) {
	if q == nil || q.path == "" {
		return nil, nil
	}

	q.mu.Lock()
	defer q.mu.Unlock()

	return q.read()
}

// CountPending returns the number of entries awaiting review.
func (q *ReviewQueue) CountPending() (int, error) {
	entries, err := q.List()
	if err != nil {
		return 0, err
	}
	return len(entries), nil
}

// Clear empties the review queue.
func (q *ReviewQueue) Clear() error {
	if q == nil || q.path == "" {
		return nil
	}

	q.mu.Lock()
	defer q.mu.Unlock()

	return q.write(nil)
}

// read loads all entries from the backing file. Returns an empty slice if
// the file does not exist yet.
func (q *ReviewQueue) read() ([]ReviewEntry, error) {
	data, err := os.ReadFile(q.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var entries []ReviewEntry
	if err := json.Unmarshal(data, &entries); err != nil {
		return nil, fmt.Errorf("failed to parse review queue %s: %w", q.path, err)
	}
	return entries, nil
}

// write atomically replaces the backing file: the entries are written to a
// temporary file in the same directory, synced, and renamed into place, so a
// crash or a concurrent reader can never observe a partially written queue.
func (q *ReviewQueue) write(entries []ReviewEntry) error {
	if entries == nil {
		entries = []ReviewEntry{}
	}

	data, err := json.MarshalIndent(entries, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to encode review queue: %w", err)
	}

	// Capture the mode of an existing queue file so replacing it does not
	// loosen permissions the operator chose. New files are created with mode
	// 0644 through the OS, so the process umask applies exactly as it did when
	// this was a plain os.WriteFile call.
	var (
		existingMode os.FileMode
		preserveMode bool
	)
	if info, err := os.Stat(q.path); err == nil {
		existingMode = info.Mode().Perm()
		preserveMode = true
	}

	tmp, err := createReviewQueueTemp(filepath.Dir(q.path), filepath.Base(q.path)+".tmp-")
	if err != nil {
		return fmt.Errorf("failed to create temporary review queue file: %w", err)
	}
	tmpPath := tmp.Name()

	// Best-effort cleanup for every path that does not end in a successful
	// rename; after a successful rename this file no longer exists.
	defer func() { _ = os.Remove(tmpPath) }()

	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("failed to write review queue: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("failed to sync review queue: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("failed to close review queue: %w", err)
	}

	if preserveMode {
		if err := os.Chmod(tmpPath, existingMode); err != nil {
			return fmt.Errorf("failed to preserve review queue permissions: %w", err)
		}
	}

	if err := os.Rename(tmpPath, q.path); err != nil {
		return fmt.Errorf("failed to replace review queue: %w", err)
	}
	return nil
}

// createReviewQueueTemp creates the temporary file used to atomically replace
// the review queue. Unlike os.CreateTemp it opens the file with mode 0644, so
// the kernel applies the process umask at creation time — the same semantics
// the plain os.WriteFile this package replaced had. The name is unique per
// process, clock tick, and attempt; O_EXCL guards against collisions.
func createReviewQueueTemp(dir, prefix string) (*os.File, error) {
	var lastErr error
	for attempt := 0; attempt < 10; attempt++ {
		name := fmt.Sprintf("%s%d-%d-%d", prefix, os.Getpid(), time.Now().UnixNano(), attempt)
		f, err := os.OpenFile(filepath.Join(dir, name), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
		if err == nil {
			return f, nil
		}
		if !errors.Is(err, fs.ErrExist) {
			return nil, err
		}
		lastErr = err
	}
	return nil, lastErr
}
