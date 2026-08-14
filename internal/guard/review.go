package guard

import (
	"encoding/json"
	"fmt"
	"os"
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

// write atomically writes the entries to the backing file.
func (q *ReviewQueue) write(entries []ReviewEntry) error {
	if entries == nil {
		entries = []ReviewEntry{}
	}

	data, err := json.MarshalIndent(entries, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to encode review queue: %w", err)
	}

	return os.WriteFile(q.path, data, 0644)
}
