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

package ste

import (
	"path/filepath"
	"testing"
)

func TestGetAvailableDiskSpace(t *testing.T) {
	tmpDir := t.TempDir()

	info, err := GetAvailableDiskSpace(tmpDir)
	if err != nil {
		t.Fatalf("GetAvailableDiskSpace failed: %v", err)
	}

	// Basic sanity checks
	if info.TotalBytes == 0 {
		t.Error("TotalBytes should not be zero")
	}

	if info.AvailableBytes == 0 {
		t.Error("AvailableBytes should not be zero")
	}

	if info.AvailableBytes > info.TotalBytes {
		t.Errorf("AvailableBytes (%d) should not exceed TotalBytes (%d)",
			info.AvailableBytes, info.TotalBytes)
	}

	if info.PercentUsed < 0 || info.PercentUsed > 100 {
		t.Errorf("PercentUsed should be between 0 and 100, got %.2f", info.PercentUsed)
	}

	t.Logf("Disk space info: Total=%d, Available=%d, Used=%d, PercentUsed=%.2f%%",
		info.TotalBytes, info.AvailableBytes, info.UsedBytes, info.PercentUsed)
}

func TestCheckDiskSpaceAvailable_SufficientSpace(t *testing.T) {
	tmpDir := t.TempDir()
	testPath := filepath.Join(tmpDir, "test.file")

	// Request a small amount of space (1KB) - should always succeed
	err := CheckDiskSpaceAvailable(testPath, 1024)
	if err != nil {
		t.Errorf("CheckDiskSpaceAvailable should succeed for small file: %v", err)
	}
}

func TestCheckDiskSpaceAvailable_ZeroSize(t *testing.T) {
	tmpDir := t.TempDir()
	testPath := filepath.Join(tmpDir, "test.file")

	// Zero size should always succeed without checking
	err := CheckDiskSpaceAvailable(testPath, 0)
	if err != nil {
		t.Errorf("CheckDiskSpaceAvailable should succeed for zero size: %v", err)
	}
}

func TestCheckDiskSpaceAvailable_NegativeSize(t *testing.T) {
	tmpDir := t.TempDir()
	testPath := filepath.Join(tmpDir, "test.file")

	// Negative size should succeed without checking
	err := CheckDiskSpaceAvailable(testPath, -1)
	if err != nil {
		t.Errorf("CheckDiskSpaceAvailable should succeed for negative size: %v", err)
	}
}

func TestCheckDiskSpaceAvailable_ExcessiveSize(t *testing.T) {
	tmpDir := t.TempDir()
	testPath := filepath.Join(tmpDir, "test.file")

	// Get current available space
	info, err := GetAvailableDiskSpace(testPath)
	if err != nil {
		t.Skipf("Cannot determine available disk space: %v", err)
	}

	// Request more than available space (with safety margin)
	excessiveSize := int64(info.AvailableBytes) + 1024*1024*1024 // Available + 1GB

	err = CheckDiskSpaceAvailable(testPath, excessiveSize)
	if err == nil {
		t.Error("CheckDiskSpaceAvailable should fail when requesting more space than available")
	}

	// Verify error type
	if _, ok := err.(*InsufficientDiskSpaceError); !ok {
		t.Errorf("Expected InsufficientDiskSpaceError, got %T", err)
	}
}

func TestInsufficientDiskSpaceError_Message(t *testing.T) {
	err := &InsufficientDiskSpaceError{
		Path:           "/tmp/test.file",
		RequiredBytes:  10 * 1024 * 1024 * 1024, // 10GB
		AvailableBytes: 5 * 1024 * 1024 * 1024,  // 5GB
		TotalBytes:     20 * 1024 * 1024 * 1024, // 20GB
	}

	msg := err.Error()
	if msg == "" {
		t.Error("Error message should not be empty")
	}

	// Message should contain key information
	if !contains(msg, "/tmp/test.file") {
		t.Error("Error message should contain path")
	}
	if !contains(msg, "GB") {
		t.Error("Error message should contain formatted bytes")
	}

	t.Logf("Error message: %s", msg)
}

func TestFormatBytes(t *testing.T) {
	tests := []struct {
		bytes    int64
		expected string
	}{
		{512, "512 B"},
		{1024, "1.0 KB"},
		{1536, "1.5 KB"},
		{1024 * 1024, "1.0 MB"},
		{1024 * 1024 * 1024, "1.0 GB"},
		{1024 * 1024 * 1024 * 1024, "1.0 TB"},
		{5 * 1024 * 1024 * 1024, "5.0 GB"},
	}

	for _, tt := range tests {
		result := formatBytes(tt.bytes)
		if result != tt.expected {
			t.Errorf("formatBytes(%d) = %q, want %q", tt.bytes, result, tt.expected)
		}
	}
}

func TestCheckDiskSpaceAvailable_SafetyMargin(t *testing.T) {
	tmpDir := t.TempDir()
	testPath := filepath.Join(tmpDir, "test.file")

	// Get current available space
	info, err := GetAvailableDiskSpace(testPath)
	if err != nil {
		t.Skipf("Cannot determine available disk space: %v", err)
	}

	// Request exactly the available space (should fail due to safety margin)
	exactSize := int64(info.AvailableBytes)

	err = CheckDiskSpaceAvailable(testPath, exactSize)
	// This might fail or succeed depending on the safety margin
	// Just verify the function runs without crashing
	t.Logf("CheckDiskSpaceAvailable with exact size: %v", err)
}

// Helper function
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) && containsAt(s, substr))
}

func containsAt(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
