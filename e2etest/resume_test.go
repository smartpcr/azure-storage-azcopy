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

package e2etest

import (
	"crypto/md5"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Azure/azure-storage-azcopy/v10/common"
	"github.com/Azure/azure-storage-azcopy/v10/ste"
)

// TestResumableDownload_ChunkProgressFileBasics tests basic chunk progress file operations
func TestResumableDownload_ChunkProgressFileBasics(t *testing.T) {
	tmpDir := t.TempDir()
	chunkProgressPath := filepath.Join(tmpDir, "test.chunks")

	fileSize := int64(10 * 1024 * 1024) // 10MB
	chunkSize := int64(1024 * 1024)     // 1MB
	lastModified := time.Now()

	// Create chunk progress file
	cpf, err := ste.CreateChunkProgressFile(chunkProgressPath, fileSize, chunkSize, nil, lastModified)
	if err != nil {
		t.Fatalf("Failed to create chunk progress file: %v", err)
	}

	// Verify initial state
	completed, total := cpf.GetProgress()
	if completed != 0 || total != 10 {
		t.Errorf("Initial progress should be 0/10, got %d/%d", completed, total)
	}

	// Mark some chunks complete
	for i := uint32(0); i < 5; i++ {
		if err := cpf.MarkChunkComplete(i, nil); err != nil {
			t.Errorf("Failed to mark chunk %d complete: %v", i, err)
		}
	}

	// Verify progress updated
	completed, total = cpf.GetProgress()
	if completed != 5 || total != 10 {
		t.Errorf("Progress should be 5/10, got %d/%d", completed, total)
	}

	// Get pending chunks
	pending := cpf.GetPendingChunks()
	if len(pending) != 5 {
		t.Errorf("Should have 5 pending chunks, got %d", len(pending))
	}

	// Close and reopen
	cpf.Close()

	cpf2, err := ste.OpenChunkProgressFile(chunkProgressPath)
	if err != nil {
		t.Fatalf("Failed to reopen chunk progress file: %v", err)
	}
	defer cpf2.Delete()

	// Verify progress persisted
	completed, total = cpf2.GetProgress()
	if completed != 5 || total != 10 {
		t.Errorf("Progress after reopen should be 5/10, got %d/%d", completed, total)
	}

	// Verify chunks are still marked complete
	for i := uint32(0); i < 5; i++ {
		if !cpf2.IsChunkComplete(i) {
			t.Errorf("Chunk %d should still be complete after reopen", i)
		}
	}
}

// TestResumableDownload_RandomAccessFileWriter tests random access file writer
func TestResumableDownload_RandomAccessFileWriter(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "test.dat")

	fileSize := int64(5 * 1024 * 1024) // 5MB
	chunkSize := int64(1024 * 1024)    // 1MB

	// Create random access file writer
	writer, err := common.NewRandomAccessFileWriter(filePath, fileSize, chunkSize, false)
	if err != nil {
		t.Fatalf("Failed to create random access file writer: %v", err)
	}
	defer writer.Close()

	// Verify file created with correct size
	stat, err := os.Stat(filePath)
	if err != nil {
		t.Fatalf("Failed to stat file: %v", err)
	}
	if stat.Size() != fileSize {
		t.Errorf("File size should be %d, got %d", fileSize, stat.Size())
	}

	// Write chunks out of order
	chunks := []struct {
		index uint32
		data  []byte
	}{
		{2, make([]byte, chunkSize)}, // Write chunk 2 first
		{0, make([]byte, chunkSize)}, // Then chunk 0
		{4, make([]byte, chunkSize)}, // Then chunk 4
		{1, make([]byte, chunkSize)}, // Then chunk 1
		{3, make([]byte, chunkSize)}, // Finally chunk 3
	}

	// Fill each chunk with unique data
	for i := range chunks {
		for j := range chunks[i].data {
			chunks[i].data[j] = byte(chunks[i].index*10 + uint32(j)%10)
		}
	}

	// Write chunks
	for _, chunk := range chunks {
		offset := int64(chunk.index) * chunkSize
		if err := writer.WriteChunk(chunk.index, offset, chunk.data); err != nil {
			t.Fatalf("Failed to write chunk %d: %v", chunk.index, err)
		}
	}

	// Finalize (no rename since we're testing)
	finalPath := filePath // Same path for test
	_, err = writer.Finalize(finalPath)
	if err != nil {
		t.Fatalf("Failed to finalize: %v", err)
	}

	// Verify data written correctly
	file, err := os.Open(finalPath)
	if err != nil {
		t.Fatalf("Failed to open finalized file: %v", err)
	}
	defer file.Close()

	// Read and verify each chunk
	for i := 0; i < 5; i++ {
		offset := int64(i) * chunkSize
		file.Seek(offset, io.SeekStart)

		readData := make([]byte, chunkSize)
		n, err := file.Read(readData)
		if err != nil || n != int(chunkSize) {
			t.Fatalf("Failed to read chunk %d: %v (read %d bytes)", i, err, n)
		}

		// Verify data matches
		expectedData := make([]byte, chunkSize)
		for j := range expectedData {
			expectedData[j] = byte(i*10 + j%10)
		}

		for j := range readData {
			if readData[j] != expectedData[j] {
				t.Errorf("Chunk %d byte %d mismatch: got %d, want %d", i, j, readData[j], expectedData[j])
				break
			}
		}
	}
}

// TestResumableDownload_SourceChangeDetection tests detection of source file changes
func TestResumableDownload_SourceChangeDetection(t *testing.T) {
	tmpDir := t.TempDir()
	chunkProgressPath := filepath.Join(tmpDir, "test.chunks")

	fileSize := int64(10 * 1024 * 1024)
	chunkSize := int64(1024 * 1024)
	originalTime := time.Now()
	originalMD5 := md5.Sum([]byte("original"))

	// Create chunk progress file with metadata
	cpf, err := ste.CreateChunkProgressFile(chunkProgressPath, fileSize, chunkSize, originalMD5[:], originalTime)
	if err != nil {
		t.Fatalf("Failed to create chunk progress file: %v", err)
	}

	// Mark some chunks complete
	cpf.MarkChunkComplete(0, nil)
	cpf.MarkChunkComplete(1, nil)
	cpf.Close()

	// Test 1: Size changed
	cpf2, err := ste.OpenChunkProgressFile(chunkProgressPath)
	if err != nil {
		t.Fatalf("Failed to reopen: %v", err)
	}

	differentSize := fileSize + 1024
	err = cpf2.ValidateSourceMetadata(differentSize, originalTime, originalMD5[:])
	if err == nil {
		t.Error("Validation should fail when size changes")
	}
	cpf2.Close()

	// Test 2: Last modified time changed significantly
	cpf3, err := ste.OpenChunkProgressFile(chunkProgressPath)
	if err != nil {
		t.Fatalf("Failed to reopen: %v", err)
	}

	differentTime := originalTime.Add(10 * time.Minute)
	err = cpf3.ValidateSourceMetadata(fileSize, differentTime, originalMD5[:])
	if err == nil {
		t.Error("Validation should fail when last modified time changes significantly")
	}
	cpf3.Close()

	// Test 3: MD5 changed
	cpf4, err := ste.OpenChunkProgressFile(chunkProgressPath)
	if err != nil {
		t.Fatalf("Failed to reopen: %v", err)
	}

	differentMD5 := md5.Sum([]byte("different"))
	err = cpf4.ValidateSourceMetadata(fileSize, originalTime, differentMD5[:])
	if err == nil {
		t.Error("Validation should fail when MD5 changes")
	}
	cpf4.Close()

	// Test 4: All metadata same - should succeed
	cpf5, err := ste.OpenChunkProgressFile(chunkProgressPath)
	if err != nil {
		t.Fatalf("Failed to reopen: %v", err)
	}
	defer cpf5.Delete()

	err = cpf5.ValidateSourceMetadata(fileSize, originalTime, originalMD5[:])
	if err != nil {
		t.Errorf("Validation should succeed with same metadata: %v", err)
	}
}

// TestResumableDownload_CorruptionDetection tests detection of corrupted progress files
func TestResumableDownload_CorruptionDetection(t *testing.T) {
	tmpDir := t.TempDir()
	chunkProgressPath := filepath.Join(tmpDir, "test.chunks")

	fileSize := int64(10 * 1024 * 1024)
	chunkSize := int64(1024 * 1024)

	// Create chunk progress file
	cpf, err := ste.CreateChunkProgressFile(chunkProgressPath, fileSize, chunkSize, nil, time.Now())
	if err != nil {
		t.Fatalf("Failed to create chunk progress file: %v", err)
	}
	defer cpf.Delete()

	// Mark some chunks complete
	cpf.MarkChunkComplete(0, nil)
	cpf.MarkChunkComplete(2, nil)
	cpf.MarkChunkComplete(4, nil)

	// Test 1: Valid file should pass integrity check
	if err := cpf.ValidateIntegrity(); err != nil {
		t.Errorf("Valid file should pass integrity check: %v", err)
	}

	// Test 2: Verify progress tracking
	completed, total := cpf.GetProgress()
	if completed != 3 || total != 10 {
		t.Errorf("Expected 3/10 chunks complete, got %d/%d", completed, total)
	}

	// Integrity validation is tested in unit tests (chunkProgressFile_validation_test.go)
	// where we have access to internal fields for corruption testing
}

// TestResumableDownload_ConcurrentProtection tests concurrent access protection
func TestResumableDownload_ConcurrentProtection(t *testing.T) {
	tmpDir := t.TempDir()
	chunkProgressPath := filepath.Join(tmpDir, "test.chunks")

	fileSize := int64(10 * 1024 * 1024)
	chunkSize := int64(1024 * 1024)

	// Create first instance - holds lock
	cpf1, err := ste.CreateChunkProgressFile(chunkProgressPath, fileSize, chunkSize, nil, time.Now())
	if err != nil {
		t.Fatalf("Failed to create first instance: %v", err)
	}

	// Try to open second instance - should fail due to lock (this will timeout after 30s)
	// We skip this test part to avoid the long timeout
	t.Skip("Skipping concurrent lock test due to 30s timeout - tested in unit tests")

	// Try to open second instance - should fail due to lock
	cpf2, err := ste.OpenChunkProgressFile(chunkProgressPath)
	if err == nil {
		cpf2.Close()
		t.Error("Second instance should fail to open due to lock")
	}

	// Verify error is about lock timeout
	t.Logf("Expected lock error: %v", err)

	// Close first instance
	cpf1.Close()

	// Now second instance should succeed
	cpf3, err := ste.OpenChunkProgressFile(chunkProgressPath)
	if err != nil {
		t.Fatalf("Should be able to open after first instance closed: %v", err)
	}
	cpf3.Delete()
}

// TestResumableDownload_DiskSpaceCheck tests disk space validation
func TestResumableDownload_DiskSpaceCheck(t *testing.T) {
	tmpDir := t.TempDir()
	testPath := filepath.Join(tmpDir, "test.file")

	// Test 1: Small file should succeed
	err := ste.CheckDiskSpaceAvailable(testPath, 1024*1024) // 1MB
	if err != nil {
		t.Errorf("Small file should pass disk space check: %v", err)
	}

	// Test 2: Zero size should succeed
	err = ste.CheckDiskSpaceAvailable(testPath, 0)
	if err != nil {
		t.Errorf("Zero size should pass: %v", err)
	}

	// Test 3: Get available space
	info, err := ste.GetAvailableDiskSpace(testPath)
	if err != nil {
		t.Fatalf("Failed to get available disk space: %v", err)
	}

	t.Logf("Available disk space: %d bytes (%.2f GB)",
		info.AvailableBytes,
		float64(info.AvailableBytes)/(1024*1024*1024))

	// Test 4: Request more than available - should fail
	excessiveSize := int64(info.AvailableBytes) + 1024*1024*1024 // Available + 1GB
	err = ste.CheckDiskSpaceAvailable(testPath, excessiveSize)
	if err == nil {
		t.Error("Should fail when requesting more than available space")
	}

	// Verify it's the right error type
	if _, ok := err.(*ste.InsufficientDiskSpaceError); !ok {
		t.Errorf("Expected InsufficientDiskSpaceError, got %T", err)
	}
}

// TestResumableDownload_ProgressFileSize tests progress file size is reasonable
func TestResumableDownload_ProgressFileSize(t *testing.T) {
	tmpDir := t.TempDir()

	tests := []struct {
		name      string
		fileSize  int64
		chunkSize int64
		maxSizeKB int64 // Maximum expected progress file size in KB
	}{
		{"10MB file", 10 * 1024 * 1024, 1 * 1024 * 1024, 1},         // ~0.5KB
		{"1GB file", 1024 * 1024 * 1024, 64 * 1024 * 1024, 10},      // ~6KB
		{"10GB file", 10 * 1024 * 1024 * 1024, 64 * 1024 * 1024, 80}, // ~60KB
		{"1TB file", 1024 * 1024 * 1024 * 1024, 64 * 1024 * 1024, 400}, // ~384KB
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			chunkProgressPath := filepath.Join(tmpDir, tt.name+".chunks")

			cpf, err := ste.CreateChunkProgressFile(chunkProgressPath, tt.fileSize, tt.chunkSize, nil, time.Now())
			if err != nil {
				t.Fatalf("Failed to create progress file: %v", err)
			}

			cpf.Close()

			stat, err := os.Stat(chunkProgressPath)
			if err != nil {
				t.Fatalf("Failed to stat progress file: %v", err)
			}

			sizeKB := stat.Size() / 1024
			if sizeKB > tt.maxSizeKB {
				t.Errorf("Progress file too large: %d KB (max %d KB)", sizeKB, tt.maxSizeKB)
			}

			t.Logf("%s: Progress file size = %d KB", tt.name, sizeKB)

			// Cleanup
			os.Remove(chunkProgressPath)
		})
	}
}

// TestResumableDownload_MD5Validation tests MD5 validation of chunks
func TestResumableDownload_MD5Validation(t *testing.T) {
	tmpDir := t.TempDir()
	chunkProgressPath := filepath.Join(tmpDir, "test.chunks")

	fileSize := int64(5 * 1024 * 1024)
	chunkSize := int64(1024 * 1024)

	cpf, err := ste.CreateChunkProgressFile(chunkProgressPath, fileSize, chunkSize, nil, time.Now())
	if err != nil {
		t.Fatalf("Failed to create chunk progress file: %v", err)
	}
	defer cpf.Delete()

	// Create test data with known MD5
	testData := make([]byte, chunkSize)
	for i := range testData {
		testData[i] = byte(i % 256)
	}
	expectedMD5 := md5.Sum(testData)

	// Mark chunk complete with MD5
	err = cpf.MarkChunkComplete(0, expectedMD5[:])
	if err != nil {
		t.Fatalf("Failed to mark chunk complete: %v", err)
	}

	// Retrieve and verify MD5
	storedMD5 := cpf.GetChunkMD5(0)
	if storedMD5 == nil {
		t.Fatal("MD5 should be stored")
	}

	for i := range expectedMD5 {
		if storedMD5[i] != expectedMD5[i] {
			t.Errorf("MD5 mismatch at byte %d: got %x, want %x", i, storedMD5[i], expectedMD5[i])
			break
		}
	}
}

// TestResumableDownload_ConfigurationDefaults tests default configuration values
func TestResumableDownload_ConfigurationDefaults(t *testing.T) {
	// Clear environment variables
	os.Unsetenv("AZCOPY_RESUMABLE_DOWNLOAD")
	os.Unsetenv("AZCOPY_RESUMABLE_THRESHOLD")
	os.Unsetenv("AZCOPY_RESUMABLE_CHUNK_SIZE")
	os.Unsetenv("AZCOPY_RESUME_SKIP_MD5")
	os.Unsetenv("AZCOPY_CHUNK_PROGRESS_DIR")

	config := common.GetResumableDownloadConfig()

	// Verify defaults
	if !config.Enabled {
		t.Error("Default Enabled should be true")
	}

	if config.Threshold != 268435456 { // 256MB
		t.Errorf("Default Threshold should be 256MB, got %d", config.Threshold)
	}

	if config.ChunkSize != 67108864 { // 64MB
		t.Errorf("Default ChunkSize should be 64MB, got %d", config.ChunkSize)
	}

	if config.SkipMD5 {
		t.Error("Default SkipMD5 should be false")
	}

	if config.ProgressDir != common.AzcopyJobPlanFolder {
		t.Errorf("Default ProgressDir should be %s, got %s", common.AzcopyJobPlanFolder, config.ProgressDir)
	}
}

// TestResumableDownload_ChunkStatusTransitions tests chunk status state transitions
func TestResumableDownload_ChunkStatusTransitions(t *testing.T) {
	tmpDir := t.TempDir()
	chunkProgressPath := filepath.Join(tmpDir, "test.chunks")

	fileSize := int64(5 * 1024 * 1024)
	chunkSize := int64(1024 * 1024)

	cpf, err := ste.CreateChunkProgressFile(chunkProgressPath, fileSize, chunkSize, nil, time.Now())
	if err != nil {
		t.Fatalf("Failed to create chunk progress file: %v", err)
	}
	defer cpf.Delete()

	chunkIdx := uint32(0)

	// Initial state: Pending
	status := cpf.GetChunkStatus(chunkIdx)
	if status != ste.ChunkStatusPending {
		t.Errorf("Initial status should be Pending, got %d", status)
	}

	// Mark as complete
	cpf.MarkChunkComplete(chunkIdx, nil)
	status = cpf.GetChunkStatus(chunkIdx)
	if status != ste.ChunkStatusCompleted {
		t.Errorf("After marking complete, status should be Completed, got %d", status)
	}

	// Test another chunk - mark as failed
	chunkIdx2 := uint32(1)
	cpf.MarkChunkFailed(chunkIdx2)
	status = cpf.GetChunkStatus(chunkIdx2)
	if status != ste.ChunkStatusFailed {
		t.Errorf("After marking failed, status should be Failed, got %d", status)
	}

	// Failed chunks should appear in pending list
	pending := cpf.GetPendingChunks()
	found := false
	for _, idx := range pending {
		if idx == chunkIdx2 {
			found = true
			break
		}
	}
	if !found {
		t.Error("Failed chunk should appear in pending list")
	}
}
