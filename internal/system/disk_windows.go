//go:build windows

package system

import (
	"fmt"

	"golang.org/x/sys/windows"
)

// GetDiskUsage returns disk usage information for the filesystem containing
// path, using the GetDiskFreeSpaceEx Windows API.
func (d *DiskChecker) GetDiskUsage(path string) (*DiskUsage, error) {
	pathPtr, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return nil, fmt.Errorf("failed to convert path %s: %w", path, err)
	}

	var freeBytesAvailable uint64
	var totalBytes uint64
	var totalFreeBytes uint64
	if err := windows.GetDiskFreeSpaceEx(
		pathPtr,
		&freeBytesAvailable,
		&totalBytes,
		&totalFreeBytes,
	); err != nil {
		return nil, fmt.Errorf("failed to get disk usage for %s: %w", path, err)
	}

	// Mirror the Unix semantics used in disk_unix.go: Free/Used reflect ALL
	// free blocks (totalFreeBytes), while Available reflects what this
	// process can actually consume (freeBytesAvailable). Used percent is
	// computed as used / (used + available), like `df` does.
	available := freeBytesAvailable
	total := totalBytes
	free := totalFreeBytes
	var used uint64
	if total >= free {
		used = total - free
	}

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
