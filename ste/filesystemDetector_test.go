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

// TestFilesystemDetection_LocalDisk tests detection on local disk
func TestFilesystemDetection_LocalDisk(t *testing.T) {
	dir := t.TempDir()
	testFile := filepath.Join(dir, "test.dat")

	fsInfo, err := detectFilesystem(testFile)
	if err != nil {
		t.Fatalf("Failed to detect filesystem: %v", err)
	}

	if fsInfo.Type == FilesystemTypeUnknown {
		// This is ok for some platforms
		t.Logf("Filesystem type unknown (acceptable on some platforms)")
	}

	// Local filesystem should support mmap
	if !fsInfo.SupportsMemoryMap {
		t.Errorf("Local filesystem should support memory mapping")
	}

	// Local filesystem should not be remote
	if fsInfo.IsRemote {
		t.Errorf("Local filesystem should not be marked as remote")
	}

	t.Logf("Detected filesystem: type=%d, name=%s, supports_mmap=%v",
		fsInfo.Type, fsInfo.FileSystemName, fsInfo.SupportsMemoryMap)
}

// TestOpenFileForMmap_Local tests opening file on local filesystem
func TestOpenFileForMmap_Local(t *testing.T) {
	dir := t.TempDir()
	testFile := filepath.Join(dir, "test.dat")

	file, fsInfo, err := openFileForMmap(testFile, 1024)
	if err != nil {
		t.Fatalf("Failed to open file for mmap: %v", err)
	}
	defer file.Close()

	// Should succeed on local filesystem
	if fsInfo.Type == FilesystemTypeUnknown {
		t.Logf("Filesystem type unknown (acceptable)")
	}

	if !fsInfo.SupportsMemoryMap {
		t.Errorf("Local filesystem should support mmap")
	}

	// Verify file was created and allocated
	stat, err := file.Stat()
	if err != nil {
		t.Fatalf("Failed to stat file: %v", err)
	}

	if stat.Size() != 1024 {
		t.Errorf("Expected file size 1024, got %d", stat.Size())
	}
}

// TestFilesystemInfo_Methods tests the FilesystemInfo helper methods
func TestFilesystemInfo_Methods(t *testing.T) {
	tests := []struct {
		name     string
		info     FilesystemInfo
		wantMmap bool
		wantNet  bool
	}{
		{
			name: "local filesystem",
			info: FilesystemInfo{
				Type:              FilesystemTypeLocal,
				IsRemote:          false,
				SupportsMemoryMap: true,
			},
			wantMmap: true,
			wantNet:  false,
		},
		{
			name: "network filesystem (NFS)",
			info: FilesystemInfo{
				Type:              FilesystemTypeNFS,
				IsRemote:          true,
				SupportsMemoryMap: false,
			},
			wantMmap: false,
			wantNet:  true,
		},
		{
			name: "network filesystem (CIFS)",
			info: FilesystemInfo{
				Type:              FilesystemTypeCIFS,
				IsRemote:          true,
				SupportsMemoryMap: false,
			},
			wantMmap: false,
			wantNet:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.info.ShouldUseMmap(); got != tt.wantMmap {
				t.Errorf("ShouldUseMmap() = %v, want %v", got, tt.wantMmap)
			}

			if got := tt.info.IsNetworkFilesystem(); got != tt.wantNet {
				t.Errorf("IsNetworkFilesystem() = %v, want %v", got, tt.wantNet)
			}
		})
	}
}

// TestUnsupportedFilesystemError tests the error type
func TestUnsupportedFilesystemError(t *testing.T) {
	fsInfo := &FilesystemInfo{
		Type:     FilesystemTypeNFS,
		IsRemote: true,
	}

	err := &UnsupportedFilesystemError{
		Path:   "/mnt/nfs/test.dat",
		FSInfo: fsInfo,
		Err:    &FilesystemNotSupportedError{},
	}

	errStr := err.Error()
	if errStr == "" {
		t.Error("Error string should not be empty")
	}

	t.Logf("Error message: %s", errStr)
}

// Note: Testing CSV detection requires a real Windows environment with CSV volumes
// Testing NFS/SMB detection requires actual network filesystems mounted
// These tests would be manual or integration tests rather than unit tests
