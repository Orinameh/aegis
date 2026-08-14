package guard

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"go.uber.org/zap"
)

func newTestGuard(t *testing.T, cfg *Config) *Guard {
	t.Helper()
	if cfg.Logger == nil {
		cfg.Logger = zap.NewNop()
	}
	if cfg.Rules == nil {
		cfg.Rules = NewDefaultRuleSet()
	}
	g, err := NewGuard(cfg)
	if err != nil {
		t.Fatalf("NewGuard returned error: %v", err)
	}
	return g
}

// orchestrationContainer is a resource matching the default
// "protect-orchestration-containers" strict rule.
func orchestrationContainer() *Resource {
	return &Resource{
		Type:      ResourceContainer,
		Name:      "k8s_apiserver-abc123",
		Namespace: "docker",
		Labels:    map[string]string{},
	}
}

func TestUnprotectedResourceAllowed(t *testing.T) {
	g := newTestGuard(t, &Config{})
	resource := &Resource{
		Type:      ResourceContainer,
		Name:      "my-app-container",
		Namespace: "docker",
		Labels:    map[string]string{},
	}

	executed := false
	err := g.CheckAndExecute(context.Background(), resource, "delete", func() error {
		executed = true
		return nil
	})
	if err != nil {
		t.Fatalf("expected approval, got error: %v", err)
	}
	if !executed {
		t.Error("expected execute callback to run")
	}
}

func TestStrictRuleRequiresApprovalInteractive(t *testing.T) {
	// With interactive enabled but stdin not available, we can't easily
	// simulate a user. Instead verify that without auto-approve and without
	// a terminal, the strict rule is denied.
	g := newTestGuard(t, &Config{Interactive: false})
	err := g.CheckAndExecute(context.Background(), orchestrationContainer(), "delete", func() error {
		t.Error("execute should not run for denied strict resource")
		return nil
	})
	if err == nil {
		t.Fatal("expected strict resource to be denied in non-interactive mode")
	}
}

func TestStrictRuleApprovedWithAutoApprove(t *testing.T) {
	g := newTestGuard(t, &Config{Interactive: false, AutoApprove: true})
	executed := false
	err := g.CheckAndExecute(context.Background(), orchestrationContainer(), "delete", func() error {
		executed = true
		return nil
	})
	if err != nil {
		t.Fatalf("expected auto-approval, got error: %v", err)
	}
	if !executed {
		t.Error("expected execute callback to run")
	}
}

func TestCriticalRuleAlwaysDenied(t *testing.T) {
	// pvc-* volumes are LevelCritical and never deletable, even with auto-approve.
	g := newTestGuard(t, &Config{Interactive: false, AutoApprove: true, OverrideToken: "*/*/*"})
	resource := &Resource{
		Type:      ResourceVolume,
		Name:      "pvc-critical-data",
		Namespace: "docker",
		Labels:    map[string]string{},
	}
	err := g.CheckAndExecute(context.Background(), resource, "delete", func() error {
		t.Error("execute should never run for critical resource")
		return nil
	})
	if err == nil {
		t.Fatal("expected critical resource to be denied even with auto-approve and override")
	}
}

func TestStrictModeUnmatchedBecomesWarning(t *testing.T) {
	// In strict mode, unmatched resources get a warning-level default rule.
	// In non-interactive mode warnings default to approval (they are not a
	// hard denial), so the action proceeds.
	g := newTestGuard(t, &Config{Interactive: false, StrictMode: true})
	resource := &Resource{
		Type:      ResourceImage,
		Name:      "registry.example.com/custom/app",
		Namespace: "docker",
		Labels:    map[string]string{},
	}

	permission, err := g.CheckPermission(context.Background(), resource, "delete")
	if err != nil {
		t.Fatalf("CheckPermission returned error: %v", err)
	}
	if !permission.Approved {
		t.Fatal("expected unmatched resource under strict mode to be warning-approved in non-interactive mode")
	}
}

func TestDryRunDoesNotExecute(t *testing.T) {
	g := newTestGuard(t, &Config{Interactive: false, DryRun: true})
	// Use an unprotected resource so it is approved, then dry-run must skip execution.
	resource := &Resource{
		Type:      ResourceContainer,
		Name:      "my-app-container",
		Namespace: "docker",
		Labels:    map[string]string{},
	}

	executed := false
	err := g.CheckAndExecute(context.Background(), resource, "delete", func() error {
		executed = true
		return nil
	})
	if err != nil {
		t.Fatalf("dry run should approve and skip execution, got error: %v", err)
	}
	if executed {
		t.Error("dry run should not invoke the execute callback")
	}
}

func TestStrictDenialEnqueuesReviewNonInteractive(t *testing.T) {
	path := filepath.Join(t.TempDir(), "review.json")
	g := newTestGuard(t, &Config{Interactive: false, ReviewQueuePath: path})

	err := g.CheckAndExecute(context.Background(), orchestrationContainer(), "delete", func() error {
		t.Error("execute should not run for denied strict resource")
		return nil
	})
	if err == nil {
		t.Fatal("expected denial")
	}

	entries, err := g.reviewQueue.List()
	if err != nil {
		t.Fatalf("failed to list review queue: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 review entry, got %d", len(entries))
	}
	if entries[0].Resource != "docker/k8s_apiserver-abc123" {
		t.Errorf("unexpected resource in review entry: %s", entries[0].Resource)
	}
	if entries[0].Action != "delete" {
		t.Errorf("unexpected action: %s", entries[0].Action)
	}
}

func TestInteractiveModeDoesNotEnqueueReview(t *testing.T) {
	path := filepath.Join(t.TempDir(), "review.json")
	g := newTestGuard(t, &Config{Interactive: true, ReviewQueuePath: path})

	// In interactive mode a strict resource prompts the user; simulate a
	// rejection so the resource is denied but NOT queued for review.
	oldStdin := os.Stdin
	os.Stdin = newReadCloser("n\n")
	defer func() { os.Stdin = oldStdin }()

	err := g.CheckAndExecute(context.Background(), orchestrationContainer(), "delete", func() error {
		return nil
	})
	if err == nil {
		t.Fatal("expected denial when user rejects the prompt")
	}

	entries, err := g.reviewQueue.List()
	if err != nil {
		t.Fatalf("failed to list review queue: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("expected no review entries in interactive mode, got %d", len(entries))
	}
}

func TestInteractiveApprovalExecutes(t *testing.T) {
	path := filepath.Join(t.TempDir(), "review.json")
	g := newTestGuard(t, &Config{Interactive: true, ReviewQueuePath: path})

	oldStdin := os.Stdin
	os.Stdin = newReadCloser("y\n")
	defer func() { os.Stdin = oldStdin }()

	executed := false
	err := g.CheckAndExecute(context.Background(), orchestrationContainer(), "delete", func() error {
		executed = true
		return nil
	})
	if err != nil {
		t.Fatalf("expected approval when user confirms, got error: %v", err)
	}
	if !executed {
		t.Error("expected execute callback to run after user approval")
	}
}

// newReadCloser wraps a string so it can replace os.Stdin in tests.
func newReadCloser(s string) *os.File {
	// Reuse a temp pipe to keep os.Stdin's concrete type.
	r, w, _ := os.Pipe()
	w.WriteString(s)
	w.Close()
	return r
}

func TestInteractiveUnprotectedConfirmationRejected(t *testing.T) {
	g := newTestGuard(t, &Config{Interactive: true})

	resource := &Resource{
		Type:      ResourceContainer,
		Name:      "my-app-container",
		Namespace: "docker",
		Labels:    map[string]string{"app": "demo"},
	}

	oldStdin := os.Stdin
	os.Stdin = newReadCloser("n\n")
	defer func() { os.Stdin = oldStdin }()

	executed := false
	err := g.CheckAndExecute(context.Background(), resource, "delete", func() error {
		executed = true
		return nil
	})
	if err == nil {
		t.Fatal("expected error when user rejects destruction confirmation")
	}
	if executed {
		t.Error("execute callback must not run when user rejects confirmation")
	}
}

func TestInteractiveUnprotectedConfirmationApproved(t *testing.T) {
	g := newTestGuard(t, &Config{Interactive: true})

	resource := &Resource{
		Type:      ResourceContainer,
		Name:      "my-app-container",
		Namespace: "docker",
		Labels:    map[string]string{"app": "demo"},
		Metadata:  map[string]any{"image": "demo:v1", "size": "128MB"},
	}

	oldStdin := os.Stdin
	os.Stdin = newReadCloser("y\n")
	defer func() { os.Stdin = oldStdin }()

	executed := false
	err := g.CheckAndExecute(context.Background(), resource, "delete", func() error {
		executed = true
		return nil
	})
	if err != nil {
		t.Fatalf("expected execution after confirmation, got error: %v", err)
	}
	if !executed {
		t.Error("expected execute callback to run after confirmation")
	}
}

func TestInteractiveAutoApproveSkipsConfirmation(t *testing.T) {
	g := newTestGuard(t, &Config{Interactive: true, AutoApprove: true})

	resource := &Resource{
		Type:      ResourceContainer,
		Name:      "my-app-container",
		Namespace: "docker",
		Labels:    map[string]string{},
	}

	// No stdin provided: if the guard tried to prompt, this would fail.
	executed := false
	err := g.CheckAndExecute(context.Background(), resource, "delete", func() error {
		executed = true
		return nil
	})
	if err != nil {
		t.Fatalf("auto-approve should skip confirmation, got error: %v", err)
	}
	if !executed {
		t.Error("expected execute callback to run under auto-approve")
	}
}

func TestCriticalDenialEnqueuesReview(t *testing.T) {
	path := filepath.Join(t.TempDir(), "review.json")
	g := newTestGuard(t, &Config{Interactive: false, ReviewQueuePath: path})

	resource := &Resource{
		Type:      ResourceVolume,
		Name:      "pvc-critical-data",
		Namespace: "docker",
		Labels:    map[string]string{},
	}
	err := g.CheckAndExecute(context.Background(), resource, "delete", func() error {
		return nil
	})
	if err == nil {
		t.Fatal("expected critical resource to be denied")
	}

	// Critical rules are denied with "critically protected" but currently only
	// strict-level denials are queued, so there should be nothing to review.
	entries, err := g.reviewQueue.List()
	if err != nil {
		t.Fatalf("failed to list review queue: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("expected no review entries for critical denial, got %d", len(entries))
	}
}

func TestOverrideTokenAllowsStrictResource(t *testing.T) {
	g := newTestGuard(t, &Config{Interactive: false, OverrideToken: "container/*/*"})
	executed := false
	err := g.CheckAndExecute(context.Background(), orchestrationContainer(), "delete", func() error {
		executed = true
		return nil
	})
	if err != nil {
		t.Fatalf("expected override to allow deletion, got error: %v", err)
	}
	if !executed {
		t.Error("expected execute callback to run")
	}
}

func TestMatchesByPattern(t *testing.T) {
	r := &Rule{
		ID:              "test",
		ResourceType:    ResourcePod,
		NamePatterns:    []string{".*-[0-9]+$"},
		ProtectionLevel: LevelWarning,
	}

	match := &Resource{Type: ResourcePod, Name: "app-0", Labels: map[string]string{}}
	if !r.Matches(match) {
		t.Error("expected 'app-0' to match stateful pod pattern")
	}

	noMatch := &Resource{Type: ResourcePod, Name: "app", Labels: map[string]string{}}
	if r.Matches(noMatch) {
		t.Error("expected 'app' to not match pattern")
	}
}

func TestRuleSetMerge(t *testing.T) {
	base := NewDefaultRuleSet()
	extra := &RuleSet{Rules: []Rule{{ID: "extra-rule"}}}
	base.Merge(extra)

	found := false
	for _, r := range base.Rules {
		if r.ID == "extra-rule" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected merged rule to be present")
	}
}

func TestReviewQueueRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "review.json")
	q := NewReviewQueue(path)

	entries, err := q.List()
	if err != nil {
		t.Fatalf("List on empty queue: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("expected empty queue, got %d entries", len(entries))
	}

	pending, _ := q.CountPending()
	if pending != 0 {
		t.Fatalf("expected 0 pending, got %d", pending)
	}

	for i := 0; i < 3; i++ {
		if err := q.Enqueue(&ReviewEntry{
			Resource:   "docker/container",
			Type:       ResourceContainer,
			Rule:       "test",
			Action:     "delete",
			Reason:     "strict",
			ApprovedBy: "system",
		}); err != nil {
			t.Fatalf("Enqueue returned error: %v", err)
		}
	}

	entries, err = q.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(entries) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(entries))
	}

	pending, _ = q.CountPending()
	if pending != 3 {
		t.Fatalf("expected 3 pending, got %d", pending)
	}

	if err := q.Clear(); err != nil {
		t.Fatalf("Clear returned error: %v", err)
	}
	entries, _ = q.List()
	if len(entries) != 0 {
		t.Fatalf("expected empty queue after clear, got %d", len(entries))
	}
}

func TestReviewQueueNilQueueSafe(t *testing.T) {
	var q *ReviewQueue
	if err := q.Enqueue(&ReviewEntry{}); err != nil {
		t.Fatalf("Enqueue on nil queue: %v", err)
	}
	entries, err := q.List()
	if err != nil {
		t.Fatalf("List on nil queue: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("expected empty from nil queue, got %d", len(entries))
	}
	if err := q.Clear(); err != nil {
		t.Fatalf("Clear on nil queue: %v", err)
	}
}

func TestReviewQueueConcurrentEnqueue(t *testing.T) {
	path := filepath.Join(t.TempDir(), "review.json")
	q := NewReviewQueue(path)

	const workers = 8
	const perWorker = 25

	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < perWorker; i++ {
				if err := q.Enqueue(&ReviewEntry{Resource: "docker/container"}); err != nil {
					t.Errorf("Enqueue returned error: %v", err)
					return
				}
			}
		}()
	}
	wg.Wait()

	entries, err := q.List()
	if err != nil {
		t.Fatalf("List returned error: %v", err)
	}
	if len(entries) != workers*perWorker {
		t.Fatalf("expected %d entries, got %d", workers*perWorker, len(entries))
	}
}

func TestReviewQueueCorruptFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "review.json")
	if err := os.WriteFile(path, []byte("not json{{{"), 0644); err != nil {
		t.Fatal(err)
	}

	q := NewReviewQueue(path)
	if _, err := q.List(); err == nil {
		t.Fatal("expected error for corrupt review queue file")
	}
}
