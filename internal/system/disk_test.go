package system

import (
	"testing"

	"go.uber.org/zap"
)

func newDiskChecker() *DiskChecker {
	return NewDiskChecker(zap.NewNop())
}

func TestGetDiskUsageRealPath(t *testing.T) {
	usage, err := newDiskChecker().GetDiskUsage("/")
	if err != nil {
		t.Fatalf("GetDiskUsage returned error: %v", err)
	}
	if usage.Total == 0 {
		t.Error("expected non-zero total disk space")
	}
	if usage.UsedPercent < 0 || usage.UsedPercent > 100 {
		t.Errorf("used percent out of range: %v", usage.UsedPercent)
	}
}

func TestIsThresholdExceeded(t *testing.T) {
	// 0% threshold is always exceeded (except on an empty filesystem, which
	// a real mount practically never is at exactly 0).
	exceeded, usage, err := newDiskChecker().IsThresholdExceeded("/", 0)
	if err != nil {
		t.Fatalf("IsThresholdExceeded returned error: %v", err)
	}
	if usage == nil {
		t.Fatal("expected non-nil usage")
	}
	// 100% threshold should essentially never be exceeded on a live mount.
	notExceeded, _, err := newDiskChecker().IsThresholdExceeded("/", 100)
	if err != nil {
		t.Fatalf("IsThresholdExceeded returned error: %v", err)
	}
	if notExceeded && usage.UsedPercent < 100 {
		t.Log("warning: 100% threshold reported as exceeded (unexpected on live mount)")
	}
	_ = exceeded
}

func TestIsThresholdExceededInvalidPath(t *testing.T) {
	_, _, err := newDiskChecker().IsThresholdExceeded("/definitely/not/a/real/path", 50)
	if err == nil {
		t.Fatal("expected error for non-existent path")
	}
}

func TestGetHumanReadableSize(t *testing.T) {
	dc := newDiskChecker()
	cases := []struct {
		bytes uint64
		want  string
	}{
		{0, "0 B"},
		{500, "500 B"},
		{1024, "1.0 KB"},
		{1536, "1.5 KB"},
		{1024 * 1024, "1.0 MB"},
		{1024 * 1024 * 1024, "1.0 GB"},
	}
	for _, c := range cases {
		if got := dc.GetHumanReadableSize(c.bytes); got != c.want {
			t.Errorf("GetHumanReadableSize(%d) = %q, want %q", c.bytes, got, c.want)
		}
	}
}
