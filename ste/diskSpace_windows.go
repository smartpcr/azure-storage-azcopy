// Copyright © Microsoft <wastore@microsoft.com>
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in
// all copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
// THE SOFTWARE.

//go:build windows

package ste

import (
	"fmt"
	"path/filepath"
	"syscall"
	"unsafe"
)

// This file implements disk space checking for Windows systems.
// It uses the GetDiskFreeSpaceExW Windows API to query filesystem statistics
// and ensure adequate disk space is available before starting large downloads.
//
// The implementation includes a 10% safety margin (or 1GB, whichever is smaller) to
// avoid completely filling the filesystem, which can cause system instability.
//
// Platform-specific details:
//   - Uses kernel32.dll's GetDiskFreeSpaceExW function
//   - Handles UTF-16 path encoding required by Windows API
//   - FreeBytesAvailable: bytes available to the calling user (respects quotas)
//   - TotalNumberOfBytes: total bytes on the volume
//   - TotalNumberOfFreeBytes: total free bytes (ignoring quotas)

// DiskSpaceInfo contains information about available disk space
type DiskSpaceInfo struct {
	TotalBytes     uint64
	AvailableBytes uint64
	UsedBytes      uint64
	PercentUsed    float64
}

var (
	kernel32           = syscall.NewLazyDLL("kernel32.dll")
	getDiskFreeSpaceEx = kernel32.NewProc("GetDiskFreeSpaceExW")
)

// GetAvailableDiskSpace returns the available disk space for the filesystem containing the given path
func GetAvailableDiskSpace(path string) (*DiskSpaceInfo, error) {
	// Get the directory containing the path
	dir := filepath.Dir(path)

	// Convert to UTF16 for Windows API
	pathPtr, err := syscall.UTF16PtrFromString(dir)
	if err != nil {
		return nil, fmt.Errorf("failed to convert path to UTF16: %w", err)
	}

	var freeBytesAvailable, totalBytes, totalFreeBytes uint64

	ret, _, err := getDiskFreeSpaceEx.Call(
		uintptr(unsafe.Pointer(pathPtr)),
		uintptr(unsafe.Pointer(&freeBytesAvailable)),
		uintptr(unsafe.Pointer(&totalBytes)),
		uintptr(unsafe.Pointer(&totalFreeBytes)),
	)

	if ret == 0 {
		return nil, fmt.Errorf("GetDiskFreeSpaceEx failed for %s: %w", path, err)
	}

	usedBytes := totalBytes - totalFreeBytes
	var percentUsed float64
	if totalBytes > 0 {
		percentUsed = float64(usedBytes) * 100.0 / float64(totalBytes)
	}

	return &DiskSpaceInfo{
		TotalBytes:     totalBytes,
		AvailableBytes: freeBytesAvailable,
		UsedBytes:      usedBytes,
		PercentUsed:    percentUsed,
	}, nil
}

// CheckDiskSpaceAvailable checks if there is enough disk space available for the given size
// It includes a safety margin (10% or 1GB, whichever is smaller) to avoid filling the disk completely
func CheckDiskSpaceAvailable(path string, requiredBytes int64) error {
	if requiredBytes <= 0 {
		return nil
	}

	info, err := GetAvailableDiskSpace(path)
	if err != nil {
		// If we can't determine disk space, don't block the download
		// The OS will return an error if there's truly no space
		return nil
	}

	// Calculate safety margin: 10% of required size or 1GB, whichever is smaller
	safetyMargin := requiredBytes / 10
	if safetyMargin > 1024*1024*1024 {
		safetyMargin = 1024 * 1024 * 1024 // 1GB
	}

	totalRequired := uint64(requiredBytes) + uint64(safetyMargin)

	if info.AvailableBytes < totalRequired {
		return &InsufficientDiskSpaceError{
			Path:           path,
			RequiredBytes:  requiredBytes,
			AvailableBytes: int64(info.AvailableBytes),
			TotalBytes:     int64(info.TotalBytes),
		}
	}

	return nil
}

// InsufficientDiskSpaceError indicates there is not enough disk space available
type InsufficientDiskSpaceError struct {
	Path           string
	RequiredBytes  int64
	AvailableBytes int64
	TotalBytes     int64
}

func (e *InsufficientDiskSpaceError) Error() string {
	return fmt.Sprintf(`insufficient disk space for download:
  Path: %s
  Required: %s (includes 10%% safety margin)
  Available: %s
  Total: %s

Suggested actions:
  1. Free up disk space by deleting unused files
  2. Use Windows Disk Cleanup utility
  3. Check disk usage in File Explorer
  4. Use a different destination drive with more space
  5. Empty the Recycle Bin`,
		e.Path,
		formatBytes(e.RequiredBytes),
		formatBytes(e.AvailableBytes),
		formatBytes(e.TotalBytes))
}

// formatBytes formats bytes into human-readable format
func formatBytes(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	switch exp {
	case 0:
		return fmt.Sprintf("%.1f KB", float64(bytes)/float64(div))
	case 1:
		return fmt.Sprintf("%.1f MB", float64(bytes)/float64(div))
	case 2:
		return fmt.Sprintf("%.1f GB", float64(bytes)/float64(div))
	case 3:
		return fmt.Sprintf("%.1f TB", float64(bytes)/float64(div))
	default:
		return fmt.Sprintf("%.1f PB", float64(bytes)/float64(div))
	}
}
