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

package common

import (
	"crypto/md5"
	"crypto/rand"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func TestNewRandomAccessFileWriter(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "test.dat")

	totalSize := int64(1024 * 1024) // 1MB
	chunkSize := int64(64 * 1024)   // 64KB

	writer, err := NewRandomAccessFileWriter(filePath, totalSize, chunkSize, true)
	if err != nil {
		t.Fatalf("Failed to create writer: %v", err)
	}
	defer writer.Close()

	// Verify file was created and pre-allocated
	stat, err := os.Stat(filePath)
	if err != nil {
		t.Fatalf("Failed to stat file: %v", err)
	}

	if stat.Size() != totalSize {
		t.Errorf("File size mismatch: got %d, want %d", stat.Size(), totalSize)
	}

	// Verify writer properties
	if writer.totalSize != totalSize {
		t.Errorf("totalSize mismatch: got %d, want %d", writer.totalSize, totalSize)
	}
	if writer.chunkSize != chunkSize {
		t.Errorf("chunkSize mismatch: got %d, want %d", writer.chunkSize, chunkSize)
	}
	if !writer.chunkMD5Enabled {
		t.Error("chunkMD5Enabled should be true")
	}
}

func TestNewRandomAccessFileWriter_InvalidSize(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "test.dat")

	tests := []struct {
		name      string
		totalSize int64
		chunkSize int64
		wantErr   bool
	}{
		{"zero total size", 0, 1024, true},
		{"negative total size", -1, 1024, true},
		{"zero chunk size", 1024, 0, true},
		{"negative chunk size", 1024, -1, true},
		{"valid sizes", 1024, 512, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			writer, err := NewRandomAccessFileWriter(filePath, tt.totalSize, tt.chunkSize, false)
			if tt.wantErr {
				if err == nil {
					writer.Close()
					t.Error("Expected error, got nil")
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error: %v", err)
				} else {
					writer.Close()
				}
			}
		})
	}
}

func TestOpenExistingRandomAccessFileWriter(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "test.dat")

	totalSize := int64(1024 * 1024)
	chunkSize := int64(64 * 1024)

	// Create initial writer
	writer1, err := NewRandomAccessFileWriter(filePath, totalSize, chunkSize, false)
	if err != nil {
		t.Fatalf("Failed to create writer: %v", err)
	}

	// Write some data
	testData := []byte("test data at offset 0")
	if err := writer1.WriteChunk(0, 0, testData); err != nil {
		t.Fatalf("Failed to write chunk: %v", err)
	}
	writer1.Close()

	// Open existing file
	writer2, err := OpenExistingRandomAccessFileWriter(filePath, totalSize, chunkSize, false)
	if err != nil {
		t.Fatalf("Failed to open existing writer: %v", err)
	}
	defer writer2.Close()

	// Verify data was persisted
	readData := make([]byte, len(testData))
	n, err := writer2.file.ReadAt(readData, 0)
	if err != nil {
		t.Fatalf("Failed to read data: %v", err)
	}
	if n != len(testData) {
		t.Errorf("Read size mismatch: got %d, want %d", n, len(testData))
	}
	if string(readData) != string(testData) {
		t.Errorf("Data mismatch: got %q, want %q", readData, testData)
	}
}

func TestOpenExistingRandomAccessFileWriter_SizeMismatch(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "test.dat")

	// Create file with size 1MB
	totalSize := int64(1024 * 1024)
	writer1, err := NewRandomAccessFileWriter(filePath, totalSize, 64*1024, false)
	if err != nil {
		t.Fatalf("Failed to create writer: %v", err)
	}
	writer1.Close()

	// Try to open with different size
	_, err = OpenExistingRandomAccessFileWriter(filePath, totalSize+1, 64*1024, false)
	if err == nil {
		t.Error("Expected error for size mismatch, got nil")
	}
}

func TestWriteChunk(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "test.dat")

	totalSize := int64(1024)
	chunkSize := int64(256)

	writer, err := NewRandomAccessFileWriter(filePath, totalSize, chunkSize, false)
	if err != nil {
		t.Fatalf("Failed to create writer: %v", err)
	}
	defer writer.Close()

	// Write chunk at offset 0
	chunk0 := []byte("chunk 0 data")
	if err := writer.WriteChunk(0, 0, chunk0); err != nil {
		t.Errorf("Failed to write chunk 0: %v", err)
	}

	// Write chunk at offset 256
	chunk1 := []byte("chunk 1 data")
	if err := writer.WriteChunk(1, 256, chunk1); err != nil {
		t.Errorf("Failed to write chunk 1: %v", err)
	}

	// Write chunk at offset 512
	chunk2 := []byte("chunk 2 data")
	if err := writer.WriteChunk(2, 512, chunk2); err != nil {
		t.Errorf("Failed to write chunk 2: %v", err)
	}

	// Verify data
	readData := make([]byte, len(chunk0))
	if _, err := writer.file.ReadAt(readData, 0); err != nil {
		t.Errorf("Failed to read chunk 0: %v", err)
	}
	if string(readData) != string(chunk0) {
		t.Errorf("Chunk 0 mismatch: got %q, want %q", readData, chunk0)
	}

	readData = make([]byte, len(chunk1))
	if _, err := writer.file.ReadAt(readData, 256); err != nil {
		t.Errorf("Failed to read chunk 1: %v", err)
	}
	if string(readData) != string(chunk1) {
		t.Errorf("Chunk 1 mismatch: got %q, want %q", readData, chunk1)
	}
}

func TestWriteChunk_InvalidOffset(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "test.dat")

	totalSize := int64(1024)
	chunkSize := int64(256)

	writer, err := NewRandomAccessFileWriter(filePath, totalSize, chunkSize, false)
	if err != nil {
		t.Fatalf("Failed to create writer: %v", err)
	}
	defer writer.Close()

	tests := []struct {
		name   string
		offset int64
		data   []byte
	}{
		{"negative offset", -1, []byte("data")},
		{"offset beyond file", totalSize, []byte("data")},
		{"write beyond file", totalSize - 2, []byte("too long")},
		{"empty data", 0, []byte{}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := writer.WriteChunk(0, tt.offset, tt.data)
			if err == nil {
				t.Error("Expected error, got nil")
			}
		})
	}
}

func TestWriteChunk_Concurrent(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "test.dat")

	numChunks := 100
	chunkSize := int64(1024)
	totalSize := int64(numChunks) * chunkSize

	writer, err := NewRandomAccessFileWriter(filePath, totalSize, chunkSize, false)
	if err != nil {
		t.Fatalf("Failed to create writer: %v", err)
	}
	defer writer.Close()

	// Write chunks concurrently
	var wg sync.WaitGroup
	errors := make(chan error, numChunks)

	for i := 0; i < numChunks; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()

			offset := int64(idx) * chunkSize
			data := make([]byte, chunkSize)
			// Fill with chunk index pattern
			for j := range data {
				data[j] = byte(idx)
			}

			if err := writer.WriteChunk(uint32(idx), offset, data); err != nil {
				errors <- err
			}
		}(i)
	}

	wg.Wait()
	close(errors)

	// Check for errors
	for err := range errors {
		t.Errorf("Concurrent write error: %v", err)
	}

	// Verify all chunks were written correctly
	for i := 0; i < numChunks; i++ {
		offset := int64(i) * chunkSize
		data := make([]byte, chunkSize)

		if _, err := writer.file.ReadAt(data, offset); err != nil {
			t.Errorf("Failed to read chunk %d: %v", i, err)
			continue
		}

		// Verify pattern
		for j := range data {
			if data[j] != byte(i) {
				t.Errorf("Chunk %d byte %d mismatch: got %d, want %d", i, j, data[j], i)
				break
			}
		}
	}
}

func TestFinalize(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "test.dat")
	destPath := filepath.Join(dir, "final.dat")

	totalSize := int64(1024)
	chunkSize := int64(256)

	writer, err := NewRandomAccessFileWriter(filePath, totalSize, chunkSize, true)
	if err != nil {
		t.Fatalf("Failed to create writer: %v", err)
	}

	// Write some data
	data := make([]byte, totalSize)
	if _, err := rand.Read(data); err != nil {
		t.Fatalf("Failed to generate random data: %v", err)
	}

	// Write in chunks
	for i := int64(0); i < totalSize; i += chunkSize {
		end := i + chunkSize
		if end > totalSize {
			end = totalSize
		}
		if err := writer.WriteChunk(uint32(i/chunkSize), i, data[i:end]); err != nil {
			t.Fatalf("Failed to write chunk: %v", err)
		}
	}

	// Compute expected MD5
	expectedMD5 := md5.Sum(data)

	// Finalize
	md5Hash, err := writer.Finalize(destPath)
	if err != nil {
		t.Fatalf("Finalize failed: %v", err)
	}

	// Verify MD5
	if len(md5Hash) != 16 {
		t.Errorf("MD5 hash length mismatch: got %d, want 16", len(md5Hash))
	}

	for i := 0; i < 16; i++ {
		if md5Hash[i] != expectedMD5[i] {
			t.Errorf("MD5 mismatch at byte %d: got %02x, want %02x", i, md5Hash[i], expectedMD5[i])
		}
	}

	// Verify file was renamed
	if _, err := os.Stat(destPath); err != nil {
		t.Errorf("Final file does not exist: %v", err)
	}

	if _, err := os.Stat(filePath); err == nil {
		t.Error("Temp file still exists after finalize")
	}

	// Verify content
	readData, err := os.ReadFile(destPath)
	if err != nil {
		t.Fatalf("Failed to read final file: %v", err)
	}

	if len(readData) != len(data) {
		t.Errorf("Size mismatch: got %d, want %d", len(readData), len(data))
	}

	for i := range data {
		if readData[i] != data[i] {
			t.Errorf("Data mismatch at byte %d", i)
			break
		}
	}
}

func TestFinalize_NoMD5(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "test.dat")
	destPath := filepath.Join(dir, "final.dat")

	totalSize := int64(1024)
	chunkSize := int64(256)

	writer, err := NewRandomAccessFileWriter(filePath, totalSize, chunkSize, false)
	if err != nil {
		t.Fatalf("Failed to create writer: %v", err)
	}

	// Write some data
	data := []byte("test data")
	if err := writer.WriteChunk(0, 0, data); err != nil {
		t.Fatalf("Failed to write chunk: %v", err)
	}

	// Finalize without MD5
	md5Hash, err := writer.Finalize(destPath)
	if err != nil {
		t.Fatalf("Finalize failed: %v", err)
	}

	// MD5 should be nil when not enabled
	if md5Hash != nil {
		t.Errorf("Expected nil MD5, got %v", md5Hash)
	}

	// Verify file was renamed
	if _, err := os.Stat(destPath); err != nil {
		t.Errorf("Final file does not exist: %v", err)
	}
}

func TestVerifyChunkIntegrity(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "test.dat")

	totalSize := int64(1024)
	chunkSize := int64(256)

	writer, err := NewRandomAccessFileWriter(filePath, totalSize, chunkSize, false)
	if err != nil {
		t.Fatalf("Failed to create writer: %v", err)
	}
	defer writer.Close()

	// Write a chunk
	data := make([]byte, chunkSize)
	for i := range data {
		data[i] = byte(i % 256)
	}

	if err := writer.WriteChunk(0, 0, data); err != nil {
		t.Fatalf("Failed to write chunk: %v", err)
	}

	// Compute expected MD5
	hasher := md5.New()
	hasher.Write(data)
	expectedMD5 := hasher.Sum(nil)

	// Verify integrity
	valid, err := writer.VerifyChunkIntegrity(0, 0, expectedMD5)
	if err != nil {
		t.Fatalf("VerifyChunkIntegrity failed: %v", err)
	}
	if !valid {
		t.Error("Chunk integrity verification failed for valid chunk")
	}

	// Verify with wrong MD5
	wrongMD5 := make([]byte, 16)
	valid, err = writer.VerifyChunkIntegrity(0, 0, wrongMD5)
	if err != nil {
		t.Fatalf("VerifyChunkIntegrity failed: %v", err)
	}
	if valid {
		t.Error("Chunk integrity verification passed for invalid MD5")
	}
}

func TestVerifyChunkIntegrity_LastChunk(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "test.dat")

	// Total size not evenly divisible by chunk size
	totalSize := int64(1000)
	chunkSize := int64(256)

	writer, err := NewRandomAccessFileWriter(filePath, totalSize, chunkSize, false)
	if err != nil {
		t.Fatalf("Failed to create writer: %v", err)
	}
	defer writer.Close()

	// Last chunk size should be 1000 - 768 = 232 bytes
	lastChunkOffset := int64(768)
	lastChunkSize := totalSize - lastChunkOffset

	data := make([]byte, lastChunkSize)
	for i := range data {
		data[i] = byte(i % 256)
	}

	if err := writer.WriteChunk(3, lastChunkOffset, data); err != nil {
		t.Fatalf("Failed to write last chunk: %v", err)
	}

	// Compute expected MD5
	hasher := md5.New()
	hasher.Write(data)
	expectedMD5 := hasher.Sum(nil)

	// Verify integrity of last chunk
	valid, err := writer.VerifyChunkIntegrity(3, lastChunkOffset, expectedMD5)
	if err != nil {
		t.Fatalf("VerifyChunkIntegrity failed: %v", err)
	}
	if !valid {
		t.Error("Last chunk integrity verification failed")
	}
}

func TestClose(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "test.dat")

	totalSize := int64(1024)
	chunkSize := int64(256)

	writer, err := NewRandomAccessFileWriter(filePath, totalSize, chunkSize, false)
	if err != nil {
		t.Fatalf("Failed to create writer: %v", err)
	}

	// Write some data
	data := []byte("test data")
	if err := writer.WriteChunk(0, 0, data); err != nil {
		t.Fatalf("Failed to write chunk: %v", err)
	}

	// Close
	if err := writer.Close(); err != nil {
		t.Errorf("Close failed: %v", err)
	}

	// Verify file still exists (not deleted on close)
	if _, err := os.Stat(filePath); err != nil {
		t.Errorf("File was deleted on close: %v", err)
	}

	// Verify data was persisted
	readData, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("Failed to read file after close: %v", err)
	}

	if len(readData) < len(data) {
		t.Errorf("File too small: got %d, want at least %d", len(readData), len(data))
	}

	for i := range data {
		if readData[i] != data[i] {
			t.Errorf("Data mismatch at byte %d after close", i)
			break
		}
	}
}

func TestGetPath(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "test.dat")

	writer, err := NewRandomAccessFileWriter(filePath, 1024, 256, false)
	if err != nil {
		t.Fatalf("Failed to create writer: %v", err)
	}
	defer writer.Close()

	if writer.GetPath() != filePath {
		t.Errorf("GetPath mismatch: got %s, want %s", writer.GetPath(), filePath)
	}
}

func TestRandomAccessFileWriter_RealWorldScenario(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "download.dat")
	destPath := filepath.Join(dir, "download_final.dat")

	// Simulate downloading a 10MB file in 1MB chunks
	totalSize := int64(10 * 1024 * 1024)
	chunkSize := int64(1024 * 1024)
	numChunks := int((totalSize + chunkSize - 1) / chunkSize)

	writer, err := NewRandomAccessFileWriter(filePath, totalSize, chunkSize, true)
	if err != nil {
		t.Fatalf("Failed to create writer: %v", err)
	}

	// Generate expected data
	expectedData := make([]byte, totalSize)
	if _, err := rand.Read(expectedData); err != nil {
		t.Fatalf("Failed to generate test data: %v", err)
	}

	// Simulate concurrent download of chunks in random order
	var wg sync.WaitGroup
	errors := make(chan error, numChunks)

	for i := 0; i < numChunks; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()

			offset := int64(idx) * chunkSize
			end := offset + chunkSize
			if end > totalSize {
				end = totalSize
			}

			chunkData := expectedData[offset:end]
			if err := writer.WriteChunk(uint32(idx), offset, chunkData); err != nil {
				errors <- err
			}
		}(i)
	}

	wg.Wait()
	close(errors)

	// Check for errors
	for err := range errors {
		t.Fatalf("Download error: %v", err)
	}

	// Finalize
	md5Hash, err := writer.Finalize(destPath)
	if err != nil {
		t.Fatalf("Finalize failed: %v", err)
	}

	// Verify MD5
	expectedMD5 := md5.Sum(expectedData)
	for i := 0; i < 16; i++ {
		if md5Hash[i] != expectedMD5[i] {
			t.Errorf("Final MD5 mismatch at byte %d", i)
			break
		}
	}

	// Verify final file
	finalData, err := os.ReadFile(destPath)
	if err != nil {
		t.Fatalf("Failed to read final file: %v", err)
	}

	if len(finalData) != len(expectedData) {
		t.Fatalf("Size mismatch: got %d, want %d", len(finalData), len(expectedData))
	}

	// Spot check some bytes
	checkPoints := []int{0, len(finalData) / 4, len(finalData) / 2, 3 * len(finalData) / 4, len(finalData) - 1}
	for _, pos := range checkPoints {
		if finalData[pos] != expectedData[pos] {
			t.Errorf("Data mismatch at position %d", pos)
		}
	}

	t.Logf("Successfully downloaded and verified %d bytes in %d chunks", totalSize, numChunks)
}

func BenchmarkWriteChunk(b *testing.B) {
	dir := b.TempDir()
	filePath := filepath.Join(dir, "bench.dat")

	totalSize := int64(100 * 1024 * 1024) // 100MB
	chunkSize := int64(1024 * 1024)       // 1MB

	writer, err := NewRandomAccessFileWriter(filePath, totalSize, chunkSize, false)
	if err != nil {
		b.Fatalf("Failed to create writer: %v", err)
	}
	defer writer.Close()

	data := make([]byte, chunkSize)
	if _, err := rand.Read(data); err != nil {
		b.Fatalf("Failed to generate test data: %v", err)
	}

	b.ResetTimer()
	b.SetBytes(chunkSize)

	for i := 0; i < b.N; i++ {
		offset := int64(i%100) * chunkSize
		if err := writer.WriteChunk(uint32(i%100), offset, data); err != nil {
			b.Fatalf("Write failed: %v", err)
		}
	}
}

func BenchmarkConcurrentWrites(b *testing.B) {
	dir := b.TempDir()
	filePath := filepath.Join(dir, "bench.dat")

	totalSize := int64(100 * 1024 * 1024) // 100MB
	chunkSize := int64(1024 * 1024)       // 1MB
	numChunks := int(totalSize / chunkSize)

	writer, err := NewRandomAccessFileWriter(filePath, totalSize, chunkSize, false)
	if err != nil {
		b.Fatalf("Failed to create writer: %v", err)
	}
	defer writer.Close()

	data := make([]byte, chunkSize)
	if _, err := rand.Read(data); err != nil {
		b.Fatalf("Failed to generate test data: %v", err)
	}

	b.ResetTimer()
	b.SetBytes(chunkSize)

	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			idx := i % numChunks
			offset := int64(idx) * chunkSize
			if err := writer.WriteChunk(uint32(idx), offset, data); err != nil {
				b.Fatalf("Write failed: %v", err)
			}
			i++
		}
	})
}
