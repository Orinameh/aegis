package system

import (
	"fmt"

	"go.uber.org/zap"
)

// DiskChecker handles disk usage operations.
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
