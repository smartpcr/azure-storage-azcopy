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
	"crypto/md5"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestValidateSourceMetadata_SizeChange tests detection of file size changes
func TestValidateSourceMetadata_SizeChange(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "test.chunks")

	originalSize := int64(1024 * 1024) // 1MB
	chunkSize := int64(64 * 1024)      // 64KB
	lastModified := time.Now()

	cpf, err := CreateChunkProgressFile(path, originalSize, chunkSize, nil, lastModified)
	if err != nil {
		t.Fatalf("Failed to create chunk progress file: %v", err)
	}
	defer cpf.Delete()

	// Validate with same size - should succeed
	if err := cpf.ValidateSourceMetadata(originalSize, lastModified, nil); err != nil {
		t.Errorf("Validation should succeed with same size: %v", err)
	}

	// Validate with different size - should fail
	differentSize := originalSize + 1024
	if err := cpf.ValidateSourceMetadata(differentSize, lastModified, nil); err == nil {
		t.Error("Validation should fail when file size changes")
	}
}

// TestValidateSourceMetadata_LastModifiedChange tests detection of last modified time changes
func TestValidateSourceMetadata_LastModifiedChange(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "test.chunks")

	fileSize := int64(1024 * 1024)
	chunkSize := int64(64 * 1024)
	originalTime := time.Now().Add(-1 * time.Hour) // 1 hour ago

	cpf, err := CreateChunkProgressFile(path, fileSize, chunkSize, nil, originalTime)
	if err != nil {
		t.Fatalf("Failed to create chunk progress file: %v", err)
	}
	defer cpf.Delete()

	// Get the stored time (which loses fractional seconds due to Unix() conversion)
	storedTime := time.Unix(cpf.header.LastModified, 0)

	// Validate with same time (within 1 second tolerance) - should succeed
	slightlyDifferentTime := storedTime.Add(500 * time.Millisecond)
	if err := cpf.ValidateSourceMetadata(fileSize, slightlyDifferentTime, nil); err != nil {
		t.Errorf("Validation should succeed with time within tolerance: %v", err)
	}

	// Validate with significantly different time - should fail
	differentTime := storedTime.Add(10 * time.Minute)
	if err := cpf.ValidateSourceMetadata(fileSize, differentTime, nil); err == nil {
		t.Error("Validation should fail when last modified time changes significantly")
	}
}

// TestValidateSourceMetadata_MD5Change tests detection of MD5 changes
func TestValidateSourceMetadata_MD5Change(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "test.chunks")

	fileSize := int64(1024 * 1024)
	chunkSize := int64(64 * 1024)
	lastModified := time.Now()
	originalMD5 := md5.Sum([]byte("original content"))

	cpf, err := CreateChunkProgressFile(path, fileSize, chunkSize, originalMD5[:], lastModified)
	if err != nil {
		t.Fatalf("Failed to create chunk progress file: %v", err)
	}
	defer cpf.Delete()

	// Validate with same MD5 - should succeed
	if err := cpf.ValidateSourceMetadata(fileSize, lastModified, originalMD5[:]); err != nil {
		t.Errorf("Validation should succeed with same MD5: %v", err)
	}

	// Validate with different MD5 - should fail
	differentMD5 := md5.Sum([]byte("different content"))
	if err := cpf.ValidateSourceMetadata(fileSize, lastModified, differentMD5[:]); err == nil {
		t.Error("Validation should fail when MD5 changes")
	}

	// Validate with nil MD5 when original has MD5 - should succeed (MD5 became unavailable)
	if err := cpf.ValidateSourceMetadata(fileSize, lastModified, nil); err != nil {
		t.Errorf("Validation should succeed when new MD5 is nil: %v", err)
	}
}

// TestValidateSourceMetadata_NoOriginalMD5 tests validation when original file had no MD5
func TestValidateSourceMetadata_NoOriginalMD5(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "test.chunks")

	fileSize := int64(1024 * 1024)
	chunkSize := int64(64 * 1024)
	lastModified := time.Now()

	// Create without MD5
	cpf, err := CreateChunkProgressFile(path, fileSize, chunkSize, nil, lastModified)
	if err != nil {
		t.Fatalf("Failed to create chunk progress file: %v", err)
	}
	defer cpf.Delete()

	// Validate with nil MD5 - should succeed
	if err := cpf.ValidateSourceMetadata(fileSize, lastModified, nil); err != nil {
		t.Errorf("Validation should succeed when both MD5s are nil: %v", err)
	}

	// Validate with non-nil MD5 when original was nil - should succeed (MD5 became available)
	newMD5 := md5.Sum([]byte("new content"))
	if err := cpf.ValidateSourceMetadata(fileSize, lastModified, newMD5[:]); err != nil {
		t.Errorf("Validation should succeed when original MD5 was nil: %v", err)
	}
}

// TestValidateIntegrity_ValidFile tests integrity validation on a valid file
func TestValidateIntegrity_ValidFile(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "test.chunks")

	fileSize := int64(256 * 1024)
	chunkSize := int64(64 * 1024)

	cpf, err := CreateChunkProgressFile(path, fileSize, chunkSize, nil, time.Now())
	if err != nil {
		t.Fatalf("Failed to create chunk progress file: %v", err)
	}
	defer cpf.Delete()

	// Mark some chunks complete
	cpf.MarkChunkComplete(0, nil)
	cpf.MarkChunkComplete(2, nil)

	// Validate integrity - should succeed
	if err := cpf.ValidateIntegrity(); err != nil {
		t.Errorf("Validation should succeed on valid file: %v", err)
	}
}

// TestValidateIntegrity_CorruptedCompletedCount tests auto-correction of corrupted completed count
func TestValidateIntegrity_CorruptedCompletedCount(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "test.chunks")

	fileSize := int64(256 * 1024)
	chunkSize := int64(64 * 1024)

	cpf, err := CreateChunkProgressFile(path, fileSize, chunkSize, nil, time.Now())
	if err != nil {
		t.Fatalf("Failed to create chunk progress file: %v", err)
	}
	defer cpf.Delete()

	// Mark one chunk complete
	cpf.MarkChunkComplete(0, nil)

	// Manually corrupt the completed count
	cpf.header.CompletedChunks = 999

	// Validate integrity - should auto-correct the count
	if err := cpf.ValidateIntegrity(); err != nil {
		t.Errorf("Validation should succeed and auto-correct: %v", err)
	}

	// Verify count was corrected to actual value (1)
	if cpf.header.CompletedChunks != 1 {
		t.Errorf("Completed count should be corrected to 1, got %d", cpf.header.CompletedChunks)
	}
}

// TestValidateIntegrity_InvalidChunkStatus tests detection of invalid chunk status
func TestValidateIntegrity_InvalidChunkStatus(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "test.chunks")

	fileSize := int64(256 * 1024)
	chunkSize := int64(64 * 1024)

	cpf, err := CreateChunkProgressFile(path, fileSize, chunkSize, nil, time.Now())
	if err != nil {
		t.Fatalf("Failed to create chunk progress file: %v", err)
	}
	defer cpf.Delete()

	// Manually set an invalid status value
	cpf.chunks[0].Status = 999 // Invalid status

	// Validate integrity - should fail
	if err := cpf.ValidateIntegrity(); err == nil {
		t.Error("Validation should fail with invalid chunk status")
	}
}

// TestFileLocking_PreventsConcurrentAccess tests that file locking prevents concurrent access
func TestFileLocking_PreventsConcurrentAccess(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "test.chunks")

	fileSize := int64(256 * 1024)
	chunkSize := int64(64 * 1024)

	// Create first instance with lock
	cpf1, err := CreateChunkProgressFile(path, fileSize, chunkSize, nil, time.Now())
	if err != nil {
		t.Fatalf("Failed to create chunk progress file: %v", err)
	}
	defer cpf1.Delete()

	// Try to open second instance - should fail due to lock
	cpf2, err := OpenChunkProgressFile(path)
	if err == nil {
		cpf2.Close()
		t.Error("Opening locked file should fail")
	}

	// Error should be a timeout error
	if _, ok := err.(*FileLockTimeoutError); !ok {
		// On some systems, might get immediate error instead of timeout
		t.Logf("Expected FileLockTimeoutError, got: %T: %v", err, err)
	}
}

// TestFileLocking_AllowsAccessAfterClose tests that file can be opened after closing
func TestFileLocking_AllowsAccessAfterClose(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "test.chunks")

	fileSize := int64(256 * 1024)
	chunkSize := int64(64 * 1024)

	// Create and close first instance
	cpf1, err := CreateChunkProgressFile(path, fileSize, chunkSize, nil, time.Now())
	if err != nil {
		t.Fatalf("Failed to create chunk progress file: %v", err)
	}

	cpf1.MarkChunkComplete(0, nil)
	cpf1.Close()

	// Open second instance - should succeed now
	cpf2, err := OpenChunkProgressFile(path)
	if err != nil {
		t.Fatalf("Opening file after close should succeed: %v", err)
	}
	defer cpf2.Delete()

	// Verify data persisted
	if !cpf2.IsChunkComplete(0) {
		t.Error("Chunk 0 should still be complete after reopen")
	}
}

// TestFileLocking_DeleteRemovesLock tests that deleting a file removes the lock
func TestFileLocking_DeleteRemovesLock(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "test.chunks")

	fileSize := int64(256 * 1024)
	chunkSize := int64(64 * 1024)

	// Create first instance
	cpf1, err := CreateChunkProgressFile(path, fileSize, chunkSize, nil, time.Now())
	if err != nil {
		t.Fatalf("Failed to create chunk progress file: %v", err)
	}

	// Delete the file (which also closes it)
	cpf1.Delete()

	// Verify file is gone
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Error("File should be deleted")
	}
}
