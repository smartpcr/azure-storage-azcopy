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
	"fmt"
	"os"
	"sync/atomic"
	"time"
	"unsafe"
)

const (
	// ChunkProgressFileHeaderSize is the fixed size of the header (64 bytes)
	ChunkProgressFileHeaderSize = 64
	// ChunkStatusSize is the size of each chunk status entry (24 bytes)
	ChunkStatusSize = 24
	// ChunkProgressFileMagic identifies chunk progress files
	ChunkProgressFileMagic = "AZCCHUNK"
	// ChunkProgressFileVersion is the current file format version
	ChunkProgressFileVersion = uint16(1)
	// DefaultBackgroundSyncInterval is how often to async sync mmap to disk
	DefaultBackgroundSyncInterval = 5 * time.Second
)

// Chunk status values
const (
	ChunkStatusPending    uint32 = 0
	ChunkStatusInProgress uint32 = 1
	ChunkStatusCompleted  uint32 = 2
	ChunkStatusFailed     uint32 = 3
)

// ChunkProgressFileHeader represents the fixed-size header of a chunk progress file
// Layout: 64 bytes total
type ChunkProgressFileHeader struct {
	Magic           [8]byte  // "AZCCHUNK" - file identifier
	Version         uint16   // Format version (currently 1)
	Flags           uint16   // Feature flags (reserved for future use)
	ChunkSize       int64    // Bytes per chunk
	TotalSize       int64    // Total file size
	NumChunks       uint32   // Total number of chunks
	CompletedChunks uint32   // Count of completed chunks (atomic access)
	SourceMD5       [16]byte // Expected MD5 from source (if available)
	Reserved        [8]byte  // Reserved for future use
}

// ChunkStatus represents the status of a single chunk
// Layout: 24 bytes total
type ChunkStatus struct {
	Status   uint32   // 0=pending, 1=in-progress, 2=completed, 3=failed (atomic access)
	Reserved [4]byte  // Padding for alignment
	MD5      [16]byte // MD5 of this chunk's data (optional)
}

// ChunkProgressFile manages chunk-level progress tracking using memory-mapped files
// for lock-free concurrent access by multiple workers
type ChunkProgressFile struct {
	path       string
	file       *os.File
	mmapData   []byte                    // Memory-mapped region
	header     *ChunkProgressFileHeader  // Points into mmapData[0:64]
	chunks     []ChunkStatus             // Slice over mmapData[64:]
	syncTicker *time.Ticker              // Background sync ticker
	done       chan struct{}             // Signal to stop background sync
	fsInfo     *FilesystemInfo           // Filesystem information
}

// UnsupportedFilesystemError indicates the filesystem doesn't support mmap
type UnsupportedFilesystemError struct {
	Path   string
	FSInfo *FilesystemInfo
	Err    error
}

func (e *UnsupportedFilesystemError) Error() string {
	return fmt.Sprintf("unsupported filesystem for memory mapping: %s (error: %v)", e.Path, e.Err)
}

func (e *UnsupportedFilesystemError) Unwrap() error {
	return e.Err
}

// CreateChunkProgressFile creates a new chunk progress file with memory mapping
func CreateChunkProgressFile(path string, totalSize, chunkSize int64, sourceMD5 []byte) (*ChunkProgressFile, error) {
	if totalSize <= 0 {
		return nil, fmt.Errorf("total size must be positive, got %d", totalSize)
	}
	if chunkSize <= 0 {
		return nil, fmt.Errorf("chunk size must be positive, got %d", chunkSize)
	}

	// Calculate number of chunks
	numChunks := uint32((totalSize + chunkSize - 1) / chunkSize)
	if numChunks == 0 {
		return nil, fmt.Errorf("calculated zero chunks for totalSize=%d, chunkSize=%d", totalSize, chunkSize)
	}

	// Calculate file size: header + chunk status array
	fileSize := int64(ChunkProgressFileHeaderSize + ChunkStatusSize*int(numChunks))

	// Create the file with appropriate flags for the filesystem
	file, fsInfo, err := openFileForMmap(path, fileSize)
	if err != nil {
		// Check if it's a filesystem not supported error
		if _, ok := err.(*FilesystemNotSupportedError); ok {
			return nil, &UnsupportedFilesystemError{
				Path:   path,
				FSInfo: fsInfo,
				Err:    err,
			}
		}
		return nil, fmt.Errorf("failed to create progress file: %w", err)
	}

	// Memory-map the file
	mmapData, err := mmapFile(file, int(fileSize))
	if err != nil {
		file.Close()
		return nil, fmt.Errorf("mmap failed: %w", err)
	}

	cpf := &ChunkProgressFile{
		path:     path,
		file:     file,
		mmapData: mmapData,
		done:     make(chan struct{}),
		fsInfo:   fsInfo,
	}

	// Create header pointer at offset 0
	cpf.header = (*ChunkProgressFileHeader)(unsafe.Pointer(&mmapData[0]))

	// Create chunks slice at offset 64
	cpf.chunks = unsafe.Slice(
		(*ChunkStatus)(unsafe.Pointer(&mmapData[ChunkProgressFileHeaderSize])),
		numChunks,
	)

	// Initialize header
	copy(cpf.header.Magic[:], ChunkProgressFileMagic)
	cpf.header.Version = ChunkProgressFileVersion
	cpf.header.Flags = 0
	cpf.header.ChunkSize = chunkSize
	cpf.header.TotalSize = totalSize
	cpf.header.NumChunks = numChunks
	cpf.header.CompletedChunks = 0

	// Copy source MD5 if provided
	if len(sourceMD5) == 16 {
		copy(cpf.header.SourceMD5[:], sourceMD5)
	}

	// Initialize all chunks to pending status
	for i := range cpf.chunks {
		cpf.chunks[i].Status = ChunkStatusPending
	}

	// Start background sync
	cpf.startBackgroundSync()

	return cpf, nil
}

// OpenChunkProgressFile opens an existing chunk progress file with memory mapping
func OpenChunkProgressFile(path string) (*ChunkProgressFile, error) {
	// Detect filesystem type (for informational purposes)
	fsInfo, _ := detectFilesystem(path)
	// Ignore detection errors when opening existing files

	// Open the file
	file, err := os.OpenFile(path, os.O_RDWR, 0644)
	if err != nil {
		return nil, fmt.Errorf("failed to open progress file: %w", err)
	}

	// Get file size
	stat, err := file.Stat()
	if err != nil {
		file.Close()
		return nil, fmt.Errorf("failed to stat progress file: %w", err)
	}
	fileSize := stat.Size()

	// Validate minimum size
	if fileSize < ChunkProgressFileHeaderSize {
		file.Close()
		return nil, fmt.Errorf("progress file too small: %d bytes", fileSize)
	}

	// Memory-map the file
	mmapData, err := mmapFile(file, int(fileSize))
	if err != nil {
		file.Close()
		return nil, fmt.Errorf("mmap failed: %w", err)
	}

	cpf := &ChunkProgressFile{
		path:     path,
		file:     file,
		mmapData: mmapData,
		done:     make(chan struct{}),
		fsInfo:   fsInfo,
	}

	// Create header pointer at offset 0
	cpf.header = (*ChunkProgressFileHeader)(unsafe.Pointer(&mmapData[0]))

	// Validate magic bytes
	magic := string(cpf.header.Magic[:])
	if magic != ChunkProgressFileMagic {
		cpf.Close()
		return nil, fmt.Errorf("invalid magic bytes: %q (expected %q)", magic, ChunkProgressFileMagic)
	}

	// Validate version
	if cpf.header.Version != ChunkProgressFileVersion {
		cpf.Close()
		return nil, fmt.Errorf("unsupported version: %d (expected %d)", cpf.header.Version, ChunkProgressFileVersion)
	}

	// Calculate expected file size
	expectedSize := int64(ChunkProgressFileHeaderSize + ChunkStatusSize*int(cpf.header.NumChunks))
	if fileSize != expectedSize {
		cpf.Close()
		return nil, fmt.Errorf("file size mismatch: got %d, expected %d", fileSize, expectedSize)
	}

	// Create chunks slice at offset 64
	cpf.chunks = unsafe.Slice(
		(*ChunkStatus)(unsafe.Pointer(&mmapData[ChunkProgressFileHeaderSize])),
		cpf.header.NumChunks,
	)

	// Start background sync
	cpf.startBackgroundSync()

	return cpf, nil
}

// startBackgroundSync starts a goroutine that periodically syncs mmap to disk
func (cpf *ChunkProgressFile) startBackgroundSync() {
	cpf.syncTicker = time.NewTicker(DefaultBackgroundSyncInterval)

	go func() {
		for {
			select {
			case <-cpf.syncTicker.C:
				// Async sync - doesn't block workers
				// MS_ASYNC queues dirty pages for writing but doesn't wait
				msyncFile(cpf.mmapData, msyncAsync)

			case <-cpf.done:
				return
			}
		}
	}()
}

// MarkChunkComplete marks a chunk as completed with lock-free atomic operations
func (cpf *ChunkProgressFile) MarkChunkComplete(idx uint32, md5 []byte) error {
	if idx >= cpf.header.NumChunks {
		return fmt.Errorf("chunk index %d out of range (max %d)", idx, cpf.header.NumChunks-1)
	}

	chunk := &cpf.chunks[idx]

	// Atomic status update - no lock needed
	atomic.StoreUint32(&chunk.Status, ChunkStatusCompleted)

	// Copy MD5 if provided (safe - only one worker per chunk)
	if len(md5) == 16 {
		copy(chunk.MD5[:], md5)
	}

	// Atomic counter increment - shared across workers
	atomic.AddUint32(&cpf.header.CompletedChunks, 1)

	return nil
}

// MarkChunkInProgress marks a chunk as in progress
func (cpf *ChunkProgressFile) MarkChunkInProgress(idx uint32) error {
	if idx >= cpf.header.NumChunks {
		return fmt.Errorf("chunk index %d out of range (max %d)", idx, cpf.header.NumChunks-1)
	}

	chunk := &cpf.chunks[idx]
	atomic.StoreUint32(&chunk.Status, ChunkStatusInProgress)

	return nil
}

// MarkChunkFailed marks a chunk as failed for retry on next resume
func (cpf *ChunkProgressFile) MarkChunkFailed(idx uint32) error {
	if idx >= cpf.header.NumChunks {
		return fmt.Errorf("chunk index %d out of range (max %d)", idx, cpf.header.NumChunks-1)
	}

	chunk := &cpf.chunks[idx]
	atomic.StoreUint32(&chunk.Status, ChunkStatusFailed)

	return nil
}

// IsChunkComplete returns true if the chunk is marked as completed (lock-free read)
func (cpf *ChunkProgressFile) IsChunkComplete(idx uint32) bool {
	if idx >= cpf.header.NumChunks {
		return false
	}

	status := atomic.LoadUint32(&cpf.chunks[idx].Status)
	return status == ChunkStatusCompleted
}

// GetChunkStatus returns the status of a chunk (lock-free read)
func (cpf *ChunkProgressFile) GetChunkStatus(idx uint32) uint32 {
	if idx >= cpf.header.NumChunks {
		return ChunkStatusPending
	}

	return atomic.LoadUint32(&cpf.chunks[idx].Status)
}

// GetCompletedChunks returns a list of all completed chunk indices
func (cpf *ChunkProgressFile) GetCompletedChunks() []uint32 {
	var completed []uint32
	for i := uint32(0); i < cpf.header.NumChunks; i++ {
		if cpf.IsChunkComplete(i) {
			completed = append(completed, i)
		}
	}
	return completed
}

// GetPendingChunks returns a list of all pending or failed chunk indices
func (cpf *ChunkProgressFile) GetPendingChunks() []uint32 {
	var pending []uint32
	for i := uint32(0); i < cpf.header.NumChunks; i++ {
		status := cpf.GetChunkStatus(i)
		if status != ChunkStatusCompleted {
			pending = append(pending, i)
		}
	}
	return pending
}

// GetProgress returns the current progress (completed, total)
func (cpf *ChunkProgressFile) GetProgress() (completed, total uint32) {
	completed = atomic.LoadUint32(&cpf.header.CompletedChunks)
	total = cpf.header.NumChunks
	return
}

// GetChunkMD5 returns the MD5 hash for a specific chunk
func (cpf *ChunkProgressFile) GetChunkMD5(idx uint32) []byte {
	if idx >= cpf.header.NumChunks {
		return nil
	}

	md5 := cpf.chunks[idx].MD5
	if md5 == [16]byte{} {
		return nil
	}

	result := make([]byte, 16)
	copy(result, md5[:])
	return result
}

// Sync forces a synchronous sync of mmap to disk
func (cpf *ChunkProgressFile) Sync() error {
	return msyncFile(cpf.mmapData, msyncSync)
}

// Close stops background sync and cleanly closes the file
func (cpf *ChunkProgressFile) Close() error {
	// Stop background sync
	if cpf.syncTicker != nil {
		cpf.syncTicker.Stop()
		close(cpf.done)
	}

	var firstErr error

	// Final synchronous sync to ensure durability
	if err := msyncFile(cpf.mmapData, msyncSync); err != nil && firstErr == nil {
		firstErr = fmt.Errorf("final msync failed: %w", err)
	}

	// Unmap memory
	if err := munmapFile(cpf.mmapData); err != nil && firstErr == nil {
		firstErr = fmt.Errorf("munmap failed: %w", err)
	}

	// Close file handle
	if err := cpf.file.Close(); err != nil && firstErr == nil {
		firstErr = fmt.Errorf("close failed: %w", err)
	}

	return firstErr
}

// Delete closes the file and removes it from disk
func (cpf *ChunkProgressFile) Delete() error {
	if err := cpf.Close(); err != nil {
		// Try to delete anyway
		os.Remove(cpf.path)
		return err
	}

	return os.Remove(cpf.path)
}

// GetChunkProgressPath generates the path for a chunk progress file
func GetChunkProgressPath(planFolder string, jobID string, partNum, transferIdx uint32) string {
	return fmt.Sprintf("%s/%s-%d-%d.chunks", planFolder, jobID, partNum, transferIdx)
}
