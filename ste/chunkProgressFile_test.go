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
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestCreateChunkProgressFile(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "test.chunks")

	totalSize := int64(1024 * 1024)      // 1MB
	chunkSize := int64(64 * 1024)        // 64KB
	sourceMD5 := md5.Sum([]byte("test")) // Example MD5

	cpf, err := CreateChunkProgressFile(path, totalSize, chunkSize, sourceMD5[:], time.Now())
	if err != nil {
		t.Fatalf("Failed to create chunk progress file: %v", err)
	}
	defer cpf.Delete()

	// Verify file created
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Fatalf("Progress file was not created")
	}

	// Verify magic bytes
	if string(cpf.header.Magic[:]) != ChunkProgressFileMagic {
		t.Errorf("Magic bytes mismatch: got %q, want %q", cpf.header.Magic, ChunkProgressFileMagic)
	}

	// Verify header fields
	if cpf.header.Version != ChunkProgressFileVersion {
		t.Errorf("Version mismatch: got %d, want %d", cpf.header.Version, ChunkProgressFileVersion)
	}

	if cpf.header.ChunkSize != chunkSize {
		t.Errorf("ChunkSize mismatch: got %d, want %d", cpf.header.ChunkSize, chunkSize)
	}

	if cpf.header.TotalSize != totalSize {
		t.Errorf("TotalSize mismatch: got %d, want %d", cpf.header.TotalSize, totalSize)
	}

	expectedChunks := uint32((totalSize + chunkSize - 1) / chunkSize)
	if cpf.header.NumChunks != expectedChunks {
		t.Errorf("NumChunks mismatch: got %d, want %d", cpf.header.NumChunks, expectedChunks)
	}

	if cpf.header.CompletedChunks != 0 {
		t.Errorf("CompletedChunks should be 0, got %d", cpf.header.CompletedChunks)
	}

	// Verify source MD5
	for i := 0; i < 16; i++ {
		if cpf.header.SourceMD5[i] != sourceMD5[i] {
			t.Errorf("SourceMD5 mismatch at byte %d", i)
			break
		}
	}

	// Verify mmap region allocated
	if len(cpf.mmapData) == 0 {
		t.Error("mmap region not allocated")
	}

	expectedSize := ChunkProgressFileHeaderSize + ChunkStatusSize*int(expectedChunks)
	if len(cpf.mmapData) != expectedSize {
		t.Errorf("mmap size mismatch: got %d, want %d", len(cpf.mmapData), expectedSize)
	}
}

func TestOpenChunkProgressFile(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "test.chunks")

	totalSize := int64(512 * 1024) // 512KB
	chunkSize := int64(64 * 1024)  // 64KB

	// Create a file first
	cpf1, err := CreateChunkProgressFile(path, totalSize, chunkSize, nil, time.Now())
	if err != nil {
		t.Fatalf("Failed to create chunk progress file: %v", err)
	}

	// Mark some chunks complete
	cpf1.MarkChunkComplete(0, nil)
	cpf1.MarkChunkComplete(2, nil)
	cpf1.Close()

	// Now open the existing file
	cpf2, err := OpenChunkProgressFile(path)
	if err != nil {
		t.Fatalf("Failed to open chunk progress file: %v", err)
	}
	defer cpf2.Delete()

	// Verify header
	if string(cpf2.header.Magic[:]) != ChunkProgressFileMagic {
		t.Errorf("Magic bytes mismatch after reopen")
	}

	// Verify chunk states persisted
	if !cpf2.IsChunkComplete(0) {
		t.Error("Chunk 0 should be complete after reopen")
	}

	if !cpf2.IsChunkComplete(2) {
		t.Error("Chunk 2 should be complete after reopen")
	}

	if cpf2.IsChunkComplete(1) {
		t.Error("Chunk 1 should not be complete")
	}

	// Verify completed count
	completed, total := cpf2.GetProgress()
	if completed != 2 {
		t.Errorf("Completed chunks mismatch: got %d, want 2", completed)
	}
	if total != 8 { // 512KB / 64KB = 8 chunks
		t.Errorf("Total chunks mismatch: got %d, want 8", total)
	}
}

func TestOpenChunkProgressFile_InvalidMagic(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "test.chunks")

	// Create a file with invalid magic bytes
	file, err := os.Create(path)
	if err != nil {
		t.Fatalf("Failed to create file: %v", err)
	}
	// Write wrong magic bytes
	file.Write([]byte("WRONGMAG"))
	file.Write(make([]byte, 56)) // Pad to header size
	file.Close()

	// Try to open - should fail
	_, err = OpenChunkProgressFile(path)
	if err == nil {
		t.Error("Expected error when opening file with invalid magic bytes")
	}
}

func TestMarkChunkComplete(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "test.chunks")

	totalSize := int64(256 * 1024) // 256KB
	chunkSize := int64(64 * 1024)  // 64KB

	cpf, err := CreateChunkProgressFile(path, totalSize, chunkSize, nil, time.Now())
	if err != nil {
		t.Fatalf("Failed to create chunk progress file: %v", err)
	}
	defer cpf.Delete()

	// Test MD5
	testData := []byte("test chunk data")
	testMD5 := md5.Sum(testData)

	// Mark chunk complete
	err = cpf.MarkChunkComplete(1, testMD5[:])
	if err != nil {
		t.Fatalf("Failed to mark chunk complete: %v", err)
	}

	// Verify status
	if !cpf.IsChunkComplete(1) {
		t.Error("Chunk 1 should be complete")
	}

	// Verify counter incremented
	if cpf.header.CompletedChunks != 1 {
		t.Errorf("Completed chunks should be 1, got %d", cpf.header.CompletedChunks)
	}

	// Verify MD5 stored
	storedMD5 := cpf.GetChunkMD5(1)
	if storedMD5 == nil {
		t.Error("MD5 should be stored")
	} else {
		for i := 0; i < 16; i++ {
			if storedMD5[i] != testMD5[i] {
				t.Errorf("MD5 mismatch at byte %d", i)
				break
			}
		}
	}

	// Mark another chunk
	cpf.MarkChunkComplete(3, nil)
	if cpf.header.CompletedChunks != 2 {
		t.Errorf("Completed chunks should be 2, got %d", cpf.header.CompletedChunks)
	}
}

func TestMarkChunkFailed(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "test.chunks")

	cpf, err := CreateChunkProgressFile(path, 256*1024, 64*1024, nil, time.Now())
	if err != nil {
		t.Fatalf("Failed to create chunk progress file: %v", err)
	}
	defer cpf.Delete()

	// Mark chunk as failed
	err = cpf.MarkChunkFailed(1)
	if err != nil {
		t.Fatalf("Failed to mark chunk as failed: %v", err)
	}

	// Verify status
	status := cpf.GetChunkStatus(1)
	if status != ChunkStatusFailed {
		t.Errorf("Chunk status should be Failed (%d), got %d", ChunkStatusFailed, status)
	}

	// Verify it appears in pending list
	pending := cpf.GetPendingChunks()
	found := false
	for _, idx := range pending {
		if idx == 1 {
			found = true
			break
		}
	}
	if !found {
		t.Error("Failed chunk should appear in pending list")
	}
}

func TestGetCompletedChunks(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "test.chunks")

	cpf, err := CreateChunkProgressFile(path, 512*1024, 64*1024, nil, time.Now())
	if err != nil {
		t.Fatalf("Failed to create chunk progress file: %v", err)
	}
	defer cpf.Delete()

	// Initially no chunks completed
	completed := cpf.GetCompletedChunks()
	if len(completed) != 0 {
		t.Errorf("Should have 0 completed chunks, got %d", len(completed))
	}

	// Mark some chunks complete
	cpf.MarkChunkComplete(0, nil)
	cpf.MarkChunkComplete(2, nil)
	cpf.MarkChunkComplete(5, nil)

	completed = cpf.GetCompletedChunks()
	if len(completed) != 3 {
		t.Fatalf("Should have 3 completed chunks, got %d", len(completed))
	}

	expected := map[uint32]bool{0: true, 2: true, 5: true}
	for _, idx := range completed {
		if !expected[idx] {
			t.Errorf("Unexpected completed chunk: %d", idx)
		}
		delete(expected, idx)
	}

	if len(expected) > 0 {
		t.Errorf("Missing completed chunks: %v", expected)
	}
}

func TestGetPendingChunks(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "test.chunks")

	totalSize := int64(256 * 1024) // 256KB = 4 chunks
	chunkSize := int64(64 * 1024)

	cpf, err := CreateChunkProgressFile(path, totalSize, chunkSize, nil, time.Now())
	if err != nil {
		t.Fatalf("Failed to create chunk progress file: %v", err)
	}
	defer cpf.Delete()

	// Initially all chunks pending
	pending := cpf.GetPendingChunks()
	if len(pending) != 4 {
		t.Errorf("Should have 4 pending chunks, got %d", len(pending))
	}

	// Mark some complete
	cpf.MarkChunkComplete(0, nil)
	cpf.MarkChunkComplete(2, nil)

	pending = cpf.GetPendingChunks()
	if len(pending) != 2 {
		t.Fatalf("Should have 2 pending chunks, got %d", len(pending))
	}

	expected := map[uint32]bool{1: true, 3: true}
	for _, idx := range pending {
		if !expected[idx] {
			t.Errorf("Unexpected pending chunk: %d", idx)
		}
		delete(expected, idx)
	}

	if len(expected) > 0 {
		t.Errorf("Missing pending chunks: %v", expected)
	}

	// Failed chunks should also appear in pending
	cpf.MarkChunkFailed(1)
	pending = cpf.GetPendingChunks()
	if len(pending) != 2 {
		t.Errorf("Failed chunks should still be in pending list")
	}
}

func TestConcurrentAccess(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "test.chunks")

	totalSize := int64(10 * 1024 * 1024) // 10MB
	chunkSize := int64(64 * 1024)        // 64KB chunks = 160 chunks

	cpf, err := CreateChunkProgressFile(path, totalSize, chunkSize, nil, time.Now())
	if err != nil {
		t.Fatalf("Failed to create chunk progress file: %v", err)
	}
	defer cpf.Delete()

	numWorkers := 10
	chunksPerWorker := cpf.header.NumChunks / uint32(numWorkers)

	var wg sync.WaitGroup
	var errorCount int32

	// Launch concurrent workers
	for w := 0; w < numWorkers; w++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()

			start := uint32(workerID) * chunksPerWorker
			end := start + chunksPerWorker
			if workerID == numWorkers-1 {
				end = cpf.header.NumChunks
			}

			for i := start; i < end; i++ {
				testData := []byte(fmt.Sprintf("worker %d chunk %d", workerID, i))
				testMD5 := md5.Sum(testData)

				if err := cpf.MarkChunkComplete(i, testMD5[:]); err != nil {
					atomic.AddInt32(&errorCount, 1)
					t.Logf("Worker %d failed to mark chunk %d: %v", workerID, i, err)
				}
			}
		}(w)
	}

	wg.Wait()

	if errorCount > 0 {
		t.Fatalf("Concurrent access had %d errors", errorCount)
	}

	// Verify all chunks marked complete
	completed, total := cpf.GetProgress()
	if completed != total {
		t.Errorf("Not all chunks completed: %d/%d", completed, total)
	}

	// Verify atomic counter matches
	if cpf.header.CompletedChunks != cpf.header.NumChunks {
		t.Errorf("Completed counter mismatch: %d != %d", cpf.header.CompletedChunks, cpf.header.NumChunks)
	}

	// Verify all chunks individually
	for i := uint32(0); i < cpf.header.NumChunks; i++ {
		if !cpf.IsChunkComplete(i) {
			t.Errorf("Chunk %d should be complete", i)
		}
	}
}

func TestBackgroundSync(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "test.chunks")

	cpf, err := CreateChunkProgressFile(path, 256*1024, 64*1024, nil, time.Now())
	if err != nil {
		t.Fatalf("Failed to create chunk progress file: %v", err)
	}

	// Verify sync ticker started
	if cpf.syncTicker == nil {
		t.Error("Background sync ticker should be started")
	}

	// Mark a chunk and close
	cpf.MarkChunkComplete(0, nil)
	cpf.Close()

	// Sync ticker should be stopped
	// Note: Can't directly test if goroutine stopped, but Close() should handle it
}

func TestCloseSync(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "test.chunks")

	cpf, err := CreateChunkProgressFile(path, 256*1024, 64*1024, nil, time.Now())
	if err != nil {
		t.Fatalf("Failed to create chunk progress file: %v", err)
	}

	// Mark chunks
	cpf.MarkChunkComplete(0, nil)
	cpf.MarkChunkComplete(1, nil)

	// Close (should call MS_SYNC)
	err = cpf.Close()
	if err != nil {
		t.Errorf("Close failed: %v", err)
	}

	// Reopen and verify data persisted
	cpf2, err := OpenChunkProgressFile(path)
	if err != nil {
		t.Fatalf("Failed to reopen: %v", err)
	}
	defer cpf2.Delete()

	if !cpf2.IsChunkComplete(0) || !cpf2.IsChunkComplete(1) {
		t.Error("Data not persisted after close")
	}
}

func TestLargeFileChunks(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "test.chunks")

	// 1TB file with 64MB chunks = 16,384 chunks
	totalSize := int64(1024 * 1024 * 1024 * 1024) // 1TB
	chunkSize := int64(64 * 1024 * 1024)          // 64MB

	cpf, err := CreateChunkProgressFile(path, totalSize, chunkSize, nil, time.Now())
	if err != nil {
		t.Fatalf("Failed to create chunk progress file: %v", err)
	}
	defer cpf.Delete()

	expectedChunks := uint32((totalSize + chunkSize - 1) / chunkSize)
	if cpf.header.NumChunks != expectedChunks {
		t.Errorf("NumChunks mismatch: got %d, want %d", cpf.header.NumChunks, expectedChunks)
	}

	// Verify progress file size is reasonable
	stat, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Failed to stat file: %v", err)
	}

	expectedSize := int64(ChunkProgressFileHeaderSize + ChunkStatusSize*int(expectedChunks))
	if stat.Size() != expectedSize {
		t.Errorf("File size mismatch: got %d, want %d", stat.Size(), expectedSize)
	}

	// Should be around 400KB for 1TB file
	if stat.Size() > 500*1024 {
		t.Errorf("Progress file too large: %d bytes", stat.Size())
	}

	t.Logf("1TB file progress file size: %d bytes (%d KB)", stat.Size(), stat.Size()/1024)
}

func TestInvalidParameters(t *testing.T) {
	tmpDir := t.TempDir()

	// Test zero total size
	_, err := CreateChunkProgressFile(filepath.Join(tmpDir, "test1.chunks"), 0, 64*1024, nil, time.Now())
	if err == nil {
		t.Error("Should fail with zero total size")
	}

	// Test zero chunk size
	_, err = CreateChunkProgressFile(filepath.Join(tmpDir, "test2.chunks"), 1024*1024, 0, nil, time.Now())
	if err == nil {
		t.Error("Should fail with zero chunk size")
	}

	// Test negative total size
	_, err = CreateChunkProgressFile(filepath.Join(tmpDir, "test3.chunks"), -1024, 64*1024, nil, time.Now())
	if err == nil {
		t.Error("Should fail with negative total size")
	}
}

func TestGetChunkProgressPath(t *testing.T) {
	path := GetChunkProgressPath("/tmp/azcopy", "job123", 5, 10)
	expected := "/tmp/azcopy/job123-5-10.chunks"
	if path != expected {
		t.Errorf("Path mismatch: got %q, want %q", path, expected)
	}
}
