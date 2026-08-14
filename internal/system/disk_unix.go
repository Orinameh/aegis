//go:build !windows

package system

import (
	"fmt"
	"syscall"
)

// GetDiskUsage returns disk usage information for the filesystem containing
// path, using the POSIX statfs syscall (available on Linux, macOS, BSD...).
func (d *DiskChecker) GetDiskUsage(path string) (*DiskUsage, error) {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil {
		return nil, fmt.Errorf("failed to get disk usage for %s: %w", path, err)
	}

	blockSize := uint64(stat.Bsize)
	total := stat.Blocks * blockSize
	free := stat.Bfree * blockSize
	available := stat.Bavail * blockSize

	// `used` is total minus ALL free blocks (matching what `df`'s Used
	// column shows), not total minus available blocks — those are two
	// different numbers whenever the filesystem reserves blocks for root.
	var used uint64
	if total >= free {
		used = total - free
	}

	// Compute the percentage the way `df` does: used / (used + available)
	// * 100, which reflects what's actually consumable by this process.
	// If the denominator is 0 (e.g. some virtual/network filesystems
	// report zero blocks) we return 0 instead of NaN, so a broken
	// filesystem can never trip the threshold.
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
