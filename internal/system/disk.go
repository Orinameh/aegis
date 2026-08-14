package system

import (
	"fmt"
	"syscall"

	"go.uber.org/zap"
)

// DiskChecker handles disk usage operations.
//
// Note: this relies on syscall.Statfs, which only exists on Unix-like
// platforms (Linux, macOS, BSD). It will fail to compile on Windows.
// That's presumably fine for a server-side daemon like this, but flagging
// it explicitly rather than leaving it as a silent constraint.
type DiskChecker struct {
	logger *zap.Logger
}

// NewDiskChecker creates a new disk checker instance
func NewDiskChecker(logger *zap.Logger) *DiskChecker {
	return &DiskChecker{
		logger: logger,
	}
}

// DiskUsage represents disk usage information.
//
// Free is ALL free space on the filesystem, including root-reserved
// blocks (ext4 and friends typically reserve ~5% for root by default).
// Available is what's actually writable by this process — use Available,
// not Free, for any "are we about to run out of space" decision, since
// that's what your process can actually consume.
type DiskUsage struct {
	Total       uint64
	Used        uint64
	Free        uint64
	Available   uint64
	UsedPercent float64
}

// GetDiskUsage returns disk usage information for the filesystem containing path.
func (d *DiskChecker) GetDiskUsage(path string) (*DiskUsage, error) {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil {
		return nil, fmt.Errorf("failed to get disk usage for %s: %w", path, err)
	}

	blockSize := uint64(stat.Bsize)
	total := stat.Blocks * blockSize
	free := stat.Bfree * blockSize
	available := stat.Bavail * blockSize

	// BUG FIX: `used` here is total minus ALL free blocks (matching what
	// `df`'s Used column shows), not total minus available blocks — those
	// are two different numbers whenever the filesystem reserves blocks
	// for root. Used itself is fine computed this way; the percentage
	// below is where the original code went wrong.
	var used uint64
	if total >= free {
		used = total - free
	}

	// BUG FIX: the original computed usedPercent as used/total*100. That
	// silently disagrees with `df` (and with reality, from this process's
	// point of view) whenever there are root-reserved blocks, because
	// total != used + available in that case. `df` computes the
	// percentage as used / (used + available) * 100 specifically to
	// reflect what's actually consumable — that's what we do here.
	//
	// This also fixes a NaN bug: if the denominator is 0 (e.g. some
	// virtual/network filesystems report zero blocks), we now return 0
	// instead of silently producing NaN, which would have made
	// IsThresholdExceeded's `>` comparison always evaluate false —
	// i.e. a broken filesystem would never trip the threshold.
	denominator := used + available
	var usedPercent float64
	if denominator > 0 {
		usedPercent = (float64(used) / float64(denominator)) * 100
	}

	return &DiskUsage{
		Total:       total,
		Used:        used,
		Free:        free,
		Available:   available,
		UsedPercent: usedPercent,
	}, nil
}

// IsThresholdExceeded checks if disk usage exceeds the threshold
func (d *DiskChecker) IsThresholdExceeded(path string, threshold int) (bool, *DiskUsage, error) {
	usage, err := d.GetDiskUsage(path)
	if err != nil {
		return false, nil, err
	}

	exceeded := usage.UsedPercent > float64(threshold)

	if exceeded {
		d.logger.Warn("disk usage exceeds threshold",
			zap.Float64("used_percent", usage.UsedPercent),
			zap.Int("threshold_percent", threshold),
			zap.String("path", path),
			zap.String("used", d.humanizeBytes(usage.Used)),
			zap.String("available", d.humanizeBytes(usage.Available)),
			zap.String("total", d.humanizeBytes(usage.Total)),
		)
	}

	return exceeded, usage, nil
}

// humanizeBytes converts bytes to human-readable format
func (d *DiskChecker) humanizeBytes(bytes uint64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := uint64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}

// GetHumanReadableSize returns human-readable size
func (d *DiskChecker) GetHumanReadableSize(bytes uint64) string {
	return d.humanizeBytes(bytes)
}
