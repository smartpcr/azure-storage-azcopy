# Implementation Plan: Resumable Chunk-Level Download

## 1. Problem Statement

Currently, when a large file download (e.g., 500GB) fails at 80% completion:
1. The partial temp file (`.azDownload-<jobID>-<filename>`) is **deleted**
2. On `jobs resume`, the entire file is re-downloaded from byte 0
3. No chunk-level progress is persisted

## 2. Design Goals

1. **Persist chunk completion status** to disk so resume can skip completed chunks
2. **Use random-access file writes** instead of sequential buffering
3. **Maintain backward compatibility** with existing plan file format
4. **Support all downloader types** (Blob, Azure Files, HTTP with Range support)
5. **Preserve MD5 validation** capability (compute incrementally or skip on resume)

---

## 3. Architecture Overview

### 3.1. New Components

```
┌─────────────────────────────────────────────────────────────────┐
│                     Job Plan File (existing)                    │
│  JobPartPlanHeader + JobPartPlanTransfer[]                      │
└─────────────────────────────────────────────────────────────────┘
                                │
                                ▼
┌─────────────────────────────────────────────────────────────────┐
│              NEW: Chunk Progress File (.chunks)                 │
│  - One per transfer (large files only)                          │
│  - Stores: chunk bitmap, per-chunk MD5, metadata                │
│  - Path: <planFolder>/<jobID>-<partNum>-<transferIdx>.chunks    │
└─────────────────────────────────────────────────────────────────┘
                                │
                                ▼
┌─────────────────────────────────────────────────────────────────┐
│              NEW: Sparse/Random-Access File Writer              │
│  - Writes chunks directly to final offset (pwrite/WriteAt)      │
│  - No sequential buffering required                             │
│  - Enables true resume from any chunk                           │
└─────────────────────────────────────────────────────────────────┘
```

### 3.2. Key Design Decisions

| Decision | Choice | Rationale |
|----------|--------|-----------|
| Chunk status storage | Separate `.chunks` file | Don't break existing plan file format; easier to manage |
| File write mode | Random access (WriteAt) | Required for non-sequential chunk completion |
| MD5 handling | Per-chunk MD5 + final verification | Enables incremental validation |
| Threshold for feature | Files > 256MB | Overhead not worth it for small files |
| Temp file behavior | Keep on failure | Required for resume; rename on success |

---

## 4. Detailed Design

> **Note on Implementation Choice:** This design uses **memory-mapped files (mmap)** for chunk progress tracking instead of regular file I/O. This decision is based on the specific architectural requirements:
> - Per-transfer progress files (small, isolated)
> - Multiple concurrent workers writing to the same file
> - Need for lock-free concurrent access
> - Real-world experience showing file lock contention issues with regular I/O
>
> See [`why_use_mmap.md`](./why_use_mmap.md) for detailed analysis and justification.

### 4.1. Chunk Progress File Format

**File:** `ste/chunkProgressFile.go` (new)

```go
// ChunkProgressFileHeader - fixed size header (64 bytes)
type ChunkProgressFileHeader struct {
    Magic           [8]byte   // "AZCCHUNK" - file identifier
    Version         uint16    // Format version (start with 1)
    Flags           uint16    // Feature flags (e.g., has per-chunk MD5)
    ChunkSize       int64     // Bytes per chunk
    TotalSize       int64     // Total file size
    NumChunks       uint32    // Total number of chunks
    CompletedChunks uint32    // Count of completed chunks (for quick check)
    SourceMD5       [16]byte  // Expected MD5 from source (if available)
    Reserved        [8]byte   // Future use
}

// ChunkStatus - per-chunk status (24 bytes each)
type ChunkStatus struct {
    Status    uint32    // 0=pending, 1=in-progress, 2=completed, 3=failed (atomic access)
    Reserved  [4]byte   // Padding for alignment
    MD5       [16]byte  // MD5 of this chunk's data (optional)
}

// File layout (memory-mapped):
// [Header: 64 bytes]
// [ChunkStatus array: 24 * NumChunks bytes]
```

**Operations:**
```go
type ChunkProgressFile struct {
    path       string
    file       *os.File
    mmapData   []byte                    // Memory-mapped region
    header     *ChunkProgressFileHeader  // Points into mmapData[0:64]
    chunks     []ChunkStatus             // Slice over mmapData[64:]
    syncTicker *time.Ticker              // Background sync
    done       chan struct{}             // Shutdown signal
}

// Create new progress file with mmap
func CreateChunkProgressFile(path string, totalSize, chunkSize int64, sourceMD5 []byte) (*ChunkProgressFile, error)

// Open existing progress file with mmap
func OpenChunkProgressFile(path string) (*ChunkProgressFile, error)

// Lock-free concurrent operations using atomic operations
func (cpf *ChunkProgressFile) MarkChunkComplete(chunkIndex uint32, md5 []byte) error
func (cpf *ChunkProgressFile) MarkChunkFailed(chunkIndex uint32) error
func (cpf *ChunkProgressFile) IsChunkComplete(chunkIndex uint32) bool
func (cpf *ChunkProgressFile) GetCompletedChunks() []uint32
func (cpf *ChunkProgressFile) GetPendingChunks() []uint32

// Sync and cleanup
func (cpf *ChunkProgressFile) Close() error  // Calls MS_SYNC, then munmap
func (cpf *ChunkProgressFile) Delete() error
```

### 4.2. Random-Access File Writer

**File:** `common/randomAccessFileWriter.go` (new)

```go
// RandomAccessFileWriter writes chunks directly to their file offsets
type RandomAccessFileWriter struct {
    file            *os.File
    totalSize       int64
    chunkProgress   *ChunkProgressFile
    md5Hasher       hash.Hash          // For whole-file MD5 (computed on finalize)
    chunkMD5Enabled bool
    mu              sync.Mutex         // Protects concurrent WriteAt
}

func NewRandomAccessFileWriter(
    filePath string,
    totalSize int64,
    chunkProgressPath string,
    chunkSize int64,
    sourceMD5 []byte,
) (*RandomAccessFileWriter, error)

// WriteChunk writes data at the specified offset and marks chunk complete
func (w *RandomAccessFileWriter) WriteChunk(chunkIndex uint32, offset int64, data []byte) error

// Finalize verifies all chunks complete, computes final MD5, renames file
func (w *RandomAccessFileWriter) Finalize(destPath string) ([]byte, error)

// Close closes without finalizing (for failure cases)
func (w *RandomAccessFileWriter) Close() error

// CanResume checks if a partial download exists and can be resumed
func CanResume(destPath string, chunkProgressPath string) (bool, *ChunkProgressFile, error)
```

**Implementation details:**
```go
func (w *RandomAccessFileWriter) WriteChunk(chunkIndex uint32, offset int64, data []byte) error {
    // 1. Compute chunk MD5 if enabled
    var chunkMD5 []byte
    if w.chunkMD5Enabled {
        h := md5.New()
        h.Write(data)
        chunkMD5 = h.Sum(nil)
    }

    // 2. Write data at offset (pwrite - atomic, no seek required)
    w.mu.Lock()
    _, err := w.file.WriteAt(data, offset)
    w.mu.Unlock()
    if err != nil {
        return err
    }

    // 3. Mark chunk complete in progress file
    return w.chunkProgress.MarkChunkComplete(chunkIndex, chunkMD5)
}
```

### 4.3. Modified Download Flow

**File:** `ste/xfer-remoteToLocal-file.go` (modify)

#### 4.3.1. New Threshold Constant

```go
const (
    azcopyTempDownloadPrefix      = ".azDownload-%s-"
    resumableDownloadThreshold    = 256 * 1024 * 1024  // 256MB
    defaultResumableChunkSize     = 64 * 1024 * 1024   // 64MB chunks for progress tracking
)
```

#### 4.3.2. Modified remoteToLocal_file() Function

**Changes at line ~140 (after getting file size):**

```go
func remoteToLocal_file(jptm IJobPartTransferMgr, info *TransferInfo, ...) {
    // ... existing code ...

    // NEW: Determine if this transfer should use resumable download
    useResumableDownload := info.SourceSize >= resumableDownloadThreshold &&
                            supportsRandomAccess(jptm) &&
                            !jptm.ShouldDecompress()  // Can't resume compressed downloads

    var raWriter *common.RandomAccessFileWriter
    var pendingChunks []uint32

    if useResumableDownload {
        chunkProgressPath := getChunkProgressPath(jptm)

        // Check for existing progress
        canResume, existingProgress, err := common.CanResume(
            info.getDownloadPath(),
            chunkProgressPath,
        )

        if canResume && err == nil {
            // RESUME PATH: Load existing progress
            pendingChunks = existingProgress.GetPendingChunks()
            jptm.Log(common.LogInfo, fmt.Sprintf(
                "Resuming download: %d/%d chunks already complete",
                existingProgress.header.CompletedChunks,
                existingProgress.header.NumChunks,
            ))
            raWriter, err = common.OpenRandomAccessFileWriter(
                info.getDownloadPath(),
                existingProgress,
            )
        } else {
            // FRESH START: Create new progress file
            raWriter, err = common.NewRandomAccessFileWriter(
                info.getDownloadPath(),
                info.SourceSize,
                chunkProgressPath,
                defaultResumableChunkSize,
                info.SrcHTTPHeaders.ContentMD5,
            )
            pendingChunks = nil // All chunks pending
        }

        if err != nil {
            // Fall back to non-resumable download
            useResumableDownload = false
            jptm.Log(common.LogWarning, "Failed to initialize resumable download, using standard mode")
        }
    }

    // ... continue with chunk scheduling, using raWriter if available ...
}
```

#### 4.3.3. Modified Chunk Scheduling Loop

**Changes at line ~309:**

```go
    // Schedule chunks
    for chunkIndex := uint32(0); chunkIndex < numChunks; chunkIndex++ {
        // NEW: Skip already-completed chunks on resume
        if useResumableDownload && pendingChunks != nil {
            if !containsChunk(pendingChunks, chunkIndex) {
                // Chunk already complete, skip it
                jptm.ReportChunkDone(id) // Count it as done
                atomic.AddInt64(&jptm.atomicSuccessfulBytes, adjustedChunkSize)
                continue
            }
        }

        startIndex := int64(chunkIndex) * downloadChunkSize
        adjustedChunkSize := downloadChunkSize
        if chunkIndex == numChunks-1 {
            adjustedChunkSize = fileSize - startIndex
        }

        id := common.NewChunkID(info.Destination, startIndex, adjustedChunkSize)
        id.SetChunkIndex(chunkIndex)  // NEW: Store index for progress tracking

        // Generate download function with appropriate writer
        var downloadFunc chunkFunc
        if useResumableDownload {
            downloadFunc = dl.GenerateResumableDownloadFunc(jptm, raWriter, id, adjustedChunkSize, pacer)
        } else {
            downloadFunc = dl.GenerateDownloadFunc(jptm, cfw, id, adjustedChunkSize, pacer)
        }

        jptm.ScheduleChunks(downloadFunc)
    }
```

#### 4.3.4. Modified Failure Handling

**Changes at line ~477 (in commonDownloaderCompletion):**

```go
    if jptm.IsDeadInflight() || jptm.IsDeadBeforeStart() {
        if jptm.ShouldLog(common.LogDebug) {
            jptm.Log(common.LogDebug, " Finalizing Transfer Cancellation/Failure")
        }

        // NEW: Don't delete temp file if resumable download is enabled
        if entityType == entityType.File() && jptm.IsDeadInflight() && jptm.HoldsDestinationLock() {
            if jptm.IsResumableDownload() {
                // Keep the temp file and progress file for resume
                jptm.LogAtLevelForCurrentTransfer(common.LogInfo,
                    "Keeping partial download for resume. Use 'azcopy jobs resume' to continue.")
            } else {
                jptm.LogAtLevelForCurrentTransfer(common.LogInfo, "Deleting incomplete destination file")
                tryDeleteFile(info, jptm)
            }
        }
    }
```

### 4.4. Extended Downloader Interface

**File:** `ste/downloader.go` (modify)

```go
type downloader interface {
    Prologue(jptm IJobPartTransferMgr)
    GenerateDownloadFunc(jptm IJobPartTransferMgr, writer common.ChunkedFileWriter,
        id common.ChunkID, length int64, pacer pacer) chunkFunc
    Epilogue()
}

// NEW: Extended interface for resumable downloads
type resumableDownloader interface {
    downloader
    GenerateResumableDownloadFunc(jptm IJobPartTransferMgr, writer *common.RandomAccessFileWriter,
        id common.ChunkID, length int64, pacer pacer) chunkFunc
    SupportsResume() bool
}
```

### 4.5. Modified Blob Downloader

**File:** `ste/downloader-blob.go` (modify)

```go
// Add method to implement resumableDownloader interface
func (bd *blobDownloader) SupportsResume() bool {
    return true // Blob storage always supports Range requests
}

func (bd *blobDownloader) GenerateResumableDownloadFunc(
    jptm IJobPartTransferMgr,
    writer *common.RandomAccessFileWriter,
    id common.ChunkID,
    length int64,
    pacer pacer,
) chunkFunc {
    return func(workerId int) {
        // Similar to existing GenerateDownloadFunc but:
        // 1. Uses writer.WriteChunk() instead of chunkedFileWriter.EnqueueChunk()
        // 2. Passes chunk index for progress tracking

        info := jptm.Info()

        // Download the chunk
        get, err := bd.source.DownloadStream(jptm.Context(), &blob.DownloadStreamOptions{
            Range: blob.HTTPRange{Offset: id.OffsetInFile(), Count: length},
        })
        if err != nil {
            jptm.FailActiveDownload("Getting blob range", err)
            return
        }
        defer get.Body.Close()

        // Read into buffer
        data := make([]byte, length)
        _, err = io.ReadFull(get.Body, data)
        if err != nil {
            jptm.FailActiveDownload("Reading blob data", err)
            return
        }

        // Write to random-access file with progress tracking
        err = writer.WriteChunk(id.ChunkIndex(), id.OffsetInFile(), data)
        if err != nil {
            jptm.FailActiveDownload("Writing chunk", err)
            return
        }

        // Report chunk done
        lastChunk := jptm.ReportChunkDone(id)
        if lastChunk {
            // Finalize the file
            md5Hash, err := writer.Finalize(info.Destination)
            if err != nil {
                jptm.FailActiveDownload("Finalizing file", err)
                return
            }
            // MD5 validation using computed hash
            jptm.SetActualMD5(md5Hash)
        }
    }
}
```

### 4.6. Resume Job Modifications

**File:** `cmd/jobsResume.go` (modify)

The existing resume logic already handles restarting failed transfers. The key change is that with chunk-level progress:

1. Transfer status remains `Failed` or `InProgress` (no change needed)
2. When transfer restarts, `remoteToLocal_file()` detects existing progress file
3. Only pending chunks are downloaded

**Optional enhancement - show chunk progress:**

```go
// In resume command output
func showResumeProgress(jobID common.JobID) {
    // ... existing code ...

    // NEW: Show chunk-level progress for large files
    for _, transfer := range failedTransfers {
        chunkProgressPath := getChunkProgressPathForTransfer(jobID, transfer)
        if progress, err := common.OpenChunkProgressFile(chunkProgressPath); err == nil {
            fmt.Printf("  %s: %d/%d chunks complete (%.1f%%)\n",
                transfer.Destination,
                progress.header.CompletedChunks,
                progress.header.NumChunks,
                float64(progress.header.CompletedChunks)/float64(progress.header.NumChunks)*100,
            )
            progress.Close()
        }
    }
}
```

### 4.7. Cleanup on Success

**File:** `ste/xfer-remoteToLocal-file.go` (modify in epilogue)

```go
// After successful rename, delete chunk progress file
if jptm.IsResumableDownload() {
    chunkProgressPath := getChunkProgressPath(jptm)
    os.Remove(chunkProgressPath) // Best effort, ignore errors
}
```

### 4.8. MD5 Validation Strategy

**Option A: Recompute on finalize (simpler)**
- On finalize, read entire file and compute MD5
- Pro: Simple, uses existing validation code
- Con: Extra I/O pass for large files

**Option B: Incremental MD5 with chunk ordering (complex)**
- Store per-chunk MD5 in progress file
- On finalize, combine chunk MD5s... but MD5 doesn't work this way!
- Need to re-read chunks in order to compute final MD5

**Option C: Skip MD5 on resume, validate via chunk MD5s (recommended)**
- Store per-chunk MD5 during initial download
- On resume, verify each chunk's MD5 before skipping
- Final MD5 validation only on fresh downloads
- Add flag: `--resume-skip-md5-validation`

**Recommendation:** Option C for performance, with Option A as fallback when MD5 validation is required.

---

## 5. Implementation Phases (Detailed)

### 5.1. Phase 1: Core Infrastructure (Essential)

#### 5.1.1. Chunk Progress File Implementation
**File:** `ste/chunkProgressFile.go` (NEW)
- [ ] Define `ChunkProgressFileHeader` struct (64 bytes)
  - [ ] Add magic bytes "AZCCHUNK" for file identification
  - [ ] Add version field (uint16, start with 1)
  - [ ] Add flags field for feature toggles
  - [ ] Add chunk size, total size, chunk count fields
  - [ ] Add completed chunks counter for quick stats (atomic access)
  - [ ] Add source MD5 field for validation
  - [ ] Add reserved bytes for future expansion
- [ ] Define `ChunkStatus` struct (24 bytes per chunk)
  - [ ] Status uint32: 0=pending, 1=in-progress, 2=completed, 3=failed (atomic access)
  - [ ] Reserved padding (7 bytes)
  - [ ] Per-chunk MD5 hash field (16 bytes)
- [ ] Implement `ChunkProgressFile` struct
  - [ ] File handle reference
  - [ ] Memory-mapped byte slice (mmapData []byte)
  - [ ] Header pointer into mmap region (*ChunkProgressFileHeader)
  - [ ] Chunks slice over mmap region ([]ChunkStatus)
  - [ ] Background sync goroutine with ticker
  - [ ] Done channel for shutdown
- [ ] Implement `CreateChunkProgressFile()` function
  - [ ] Calculate file size (header + chunks array)
  - [ ] Create file with O_RDWR|O_CREATE|O_TRUNC
  - [ ] Pre-allocate space using Truncate()
  - [ ] Memory-map using syscall.Mmap() with MAP_SHARED
  - [ ] Create header pointer at offset 0 using unsafe.Pointer
  - [ ] Create chunks slice using unsafe.Slice over offset 64
  - [ ] Initialize header fields (magic, version, sizes)
  - [ ] Start background sync goroutine
- [ ] Implement `OpenChunkProgressFile()` function
  - [ ] Open existing file with O_RDWR
  - [ ] Validate magic bytes
  - [ ] Version compatibility check
  - [ ] Memory-map the file using syscall.Mmap()
  - [ ] Create pointers/slices into mmap region
  - [ ] Start background sync goroutine
- [ ] Implement `MarkChunkComplete()` method
  - [ ] Use atomic.StoreUint32() for status update (lock-free)
  - [ ] Copy chunk MD5 directly (safe - only one worker per chunk)
  - [ ] Use atomic.AddUint32() for completed counter
  - [ ] No explicit sync - handled by background goroutine
- [ ] Implement `MarkChunkFailed()` method
  - [ ] Use atomic.StoreUint32() to set status to failed
  - [ ] Allow retry on next resume
- [ ] Implement `IsChunkComplete()` method
  - [ ] Use atomic.LoadUint32() for lock-free read
  - [ ] Compare status == 2 (completed)
- [ ] Implement `GetCompletedChunks()` method
  - [ ] Iterate chunks array with atomic reads
  - [ ] Return list of completed chunk indices
- [ ] Implement `GetPendingChunks()` method
  - [ ] Iterate chunks array with atomic reads
  - [ ] Return list of incomplete/failed chunk indices
- [ ] Implement background sync goroutine
  - [ ] Create ticker (every 5 seconds)
  - [ ] Call syscall.Msync(mmapData, MS_ASYNC) on tick
  - [ ] Listen for done channel to stop
- [ ] Implement `Close()` method
  - [ ] Stop background sync ticker
  - [ ] Close done channel
  - [ ] Call syscall.Msync(mmapData, MS_SYNC) for final sync
  - [ ] Call syscall.Munmap(mmapData) to unmap
  - [ ] Close file handle
- [ ] Implement `Delete()` method
  - [ ] Call Close() first
  - [ ] Remove file from disk with os.Remove()
- [ ] Add helper function `getChunkProgressPath()`
  - [ ] Generate path: `<planFolder>/<jobID>-<partNum>-<transferIdx>.chunks`
  - [ ] Use same directory as job plan files
- [ ] Add platform abstraction layer
  - [ ] Create mmap_unix.go (+build linux darwin)
  - [ ] Create mmap_windows.go (+build windows)
  - [ ] Abstract syscall.Mmap/Munmap/Msync differences
- [ ] Add error handling for edge cases
  - [ ] Disk full during Truncate
  - [ ] Mmap failure (fallback to regular I/O)
  - [ ] File corruption detection (magic bytes)
  - [ ] Network filesystem detection (fallback to regular I/O)

**Unit Tests:** `ste/chunkProgressFile_test.go` (NEW)
- [ ] `TestCreateChunkProgressFile`
  - [ ] Verify file created with correct magic bytes
  - [ ] Verify header fields set correctly
  - [ ] Verify file size calculation
  - [ ] Verify mmap region allocated
  - [ ] Verify background sync goroutine started
- [ ] `TestOpenChunkProgressFile`
  - [ ] Open existing valid file
  - [ ] Reject file with invalid magic bytes
  - [ ] Reject file with unsupported version
  - [ ] Verify mmap established correctly
- [ ] `TestMarkChunkComplete`
  - [ ] Mark single chunk complete with atomic ops
  - [ ] Verify counter increments atomically
  - [ ] Verify status persisted to mmap
  - [ ] Verify persistence after close/reopen
- [ ] `TestMarkChunkFailed`
  - [ ] Mark chunk as failed atomically
  - [ ] Verify it appears in pending list
- [ ] `TestGetCompletedChunks`
  - [ ] No chunks completed
  - [ ] Some chunks completed
  - [ ] All chunks completed
  - [ ] Verify lock-free reads with atomics
- [ ] `TestGetPendingChunks`
  - [ ] All chunks pending initially
  - [ ] Mixed complete/pending state
  - [ ] Failed chunks included in pending
  - [ ] Verify lock-free reads
- [ ] `TestConcurrentAccess`
  - [ ] 10 goroutines marking different chunks simultaneously
  - [ ] Verify no data races (run with -race flag)
  - [ ] Verify all updates persisted correctly
  - [ ] Verify atomic counter correctness
  - [ ] Verify true parallelism (no lock contention)
- [ ] `TestBackgroundSync`
  - [ ] Verify sync goroutine runs periodically
  - [ ] Verify MS_ASYNC called on ticker
  - [ ] Verify goroutine stops on close
- [ ] `TestCloseSync`
  - [ ] Verify MS_SYNC called on close
  - [ ] Verify munmap called
  - [ ] Verify file handle closed
- [ ] `TestCorruptedFile`
  - [ ] Invalid magic bytes
  - [ ] Truncated file
  - [ ] Verify fallback to regular I/O
- [ ] `TestLargeFileChunks`
  - [ ] 1TB file (16K chunks)
  - [ ] Verify progress file size reasonable (~400KB)
  - [ ] Verify performance acceptable with mmap
- [ ] `TestPerChunkMD5`
  - [ ] Store MD5 with each chunk
  - [ ] Retrieve MD5 for validation
  - [ ] Handle missing MD5 gracefully
- [ ] `TestPlatformSpecific`
  - [ ] Test on Linux (syscall.Mmap)
  - [ ] Test on Windows (CreateFileMapping)
  - [ ] Test on macOS (syscall.Mmap)
  - [ ] Verify abstraction works correctly
- [ ] `TestNetworkFilesystem`
  - [ ] Detect NFS/SMB filesystem
  - [ ] Fallback to regular I/O
  - [ ] Verify functionality maintained

#### 5.1.2. Random Access File Writer Implementation
**File:** `common/randomAccessFileWriter.go` (NEW)
- [ ] Define `RandomAccessFileWriter` struct
  - [ ] File handle for WriteAt operations
  - [ ] Total file size
  - [ ] Chunk progress file reference
  - [ ] Optional MD5 hasher for final validation
  - [ ] Mutex for thread-safe writes
  - [ ] Chunk size for index calculations
- [ ] Implement `NewRandomAccessFileWriter()` function
  - [ ] Create or truncate destination file
  - [ ] Pre-allocate file space using Truncate()
  - [ ] Create new chunk progress file
  - [ ] Initialize MD5 hasher if enabled
  - [ ] Platform-specific optimizations (O_DIRECT on Linux)
- [ ] Implement `OpenExistingRandomAccessFileWriter()` function
  - [ ] Open existing partial download file
  - [ ] Open existing chunk progress file
  - [ ] Validate file sizes match expected
  - [ ] Verify source hasn't changed (ETag/MD5)
- [ ] Implement `WriteChunk()` method
  - [ ] Compute chunk MD5 if enabled
  - [ ] Use WriteAt() for atomic random-access write
  - [ ] No seek required - direct offset write
  - [ ] Mark chunk complete in progress file
  - [ ] Handle partial write errors
  - [ ] Retry logic for transient errors
- [ ] Implement `Finalize()` method
  - [ ] Verify all chunks marked complete
  - [ ] Compute final file MD5 (read pass if needed)
  - [ ] Compare with expected source MD5
  - [ ] Rename temp file to final destination
  - [ ] Delete chunk progress file
  - [ ] Fsync parent directory
- [ ] Implement `Close()` method
  - [ ] Close file handle without finalization
  - [ ] Keep chunk progress file for resume
  - [ ] Flush any pending writes
- [ ] Implement `CanResume()` function
  - [ ] Check if partial download exists
  - [ ] Check if chunk progress file exists
  - [ ] Validate progress file not corrupted
  - [ ] Check source hasn't changed (ETag/LastModified)
  - [ ] Return progress file handle if valid
- [ ] Add `verifyChunkIntegrity()` method
  - [ ] Optionally verify chunk MD5 on resume
  - [ ] Read chunk from file and compare hash
  - [ ] Mark as pending if mismatch
- [ ] Platform-specific optimizations
  - [ ] Linux: use fallocate() for space reservation
  - [ ] Windows: use SetFileValidData() if available
  - [ ] macOS: use fcntl(F_PREALLOCATE)

**Unit Tests:** `common/randomAccessFileWriter_test.go` (NEW)
- [ ] `TestNewRandomAccessFileWriter`
  - [ ] Creates file with correct size
  - [ ] Creates chunk progress file
  - [ ] File pre-allocated on disk
- [ ] `TestWriteChunk`
  - [ ] Write chunk at offset 0
  - [ ] Write chunk at middle offset
  - [ ] Write chunk at end
  - [ ] Verify data written correctly
  - [ ] Verify chunk marked complete
- [ ] `TestWriteChunksOutOfOrder`
  - [ ] Write chunks in random order
  - [ ] Verify all data correct on finalize
  - [ ] Verify no gaps in file
- [ ] `TestConcurrentWrites`
  - [ ] Multiple goroutines writing different chunks
  - [ ] Verify no data corruption
  - [ ] Verify all chunks marked complete
- [ ] `TestFinalize`
  - [ ] All chunks complete - success
  - [ ] Missing chunks - failure
  - [ ] MD5 validation - match
  - [ ] MD5 validation - mismatch
  - [ ] File renamed correctly
  - [ ] Progress file deleted
- [ ] `TestCanResume`
  - [ ] Valid partial download - return true
  - [ ] No partial download - return false
  - [ ] Corrupted progress file - return false
  - [ ] Source file changed - return false
- [ ] `TestCloseWithoutFinalize`
  - [ ] Close preserves progress file
  - [ ] Close preserves partial data
  - [ ] Can resume after close
- [ ] `TestDiskFullError`
  - [ ] WriteChunk fails with disk full
  - [ ] Error propagated correctly
  - [ ] Progress file consistent
- [ ] `TestChunkMD5Validation`
  - [ ] Store MD5 with each chunk
  - [ ] Verify MD5 on resume
  - [ ] Invalidate chunk if MD5 mismatch
- [ ] `TestLargeFile`
  - [ ] 1GB+ file with many chunks
  - [ ] Verify performance acceptable
  - [ ] Verify memory usage reasonable

#### 5.1.3. ChunkID Enhancement
**File:** `common/chunkStatusLogger.go` (MODIFY)
- [ ] Add `chunkIndex uint32` field to `ChunkID` struct
- [ ] Implement `SetChunkIndex(index uint32)` method
- [ ] Implement `ChunkIndex() uint32` getter method
- [ ] Update constructor to optionally include chunk index
- [ ] Ensure backward compatibility with existing code

**Unit Tests:** Extend existing `common/chunkStatusLogger_test.go`
- [ ] `TestChunkIDWithIndex`
  - [ ] Create ChunkID with index
  - [ ] Verify index stored correctly
  - [ ] Verify index retrieved correctly
- [ ] `TestChunkIDWithoutIndex`
  - [ ] Backward compatibility - no index set
  - [ ] Default value handling

---

### 5.2. Phase 2: Download Flow Integration

#### 5.2.1. Main Download Flow Modifications
**File:** `ste/xfer-remoteToLocal-file.go` (MODIFY)

**Add Constants (after line ~50):**
- [ ] Add `resumableDownloadThreshold = 256 * 1024 * 1024`
- [ ] Add `defaultResumableChunkSize = 64 * 1024 * 1024`
- [ ] Add `resumableDownloadEnabled = true` (config check)

**Modify `remoteToLocal_file()` function (~line 140):**
- [ ] Add logic to determine if resumable download should be used
  - [ ] Check file size >= threshold
  - [ ] Check downloader supports range requests
  - [ ] Check not decompressing (can't resume decompress)
  - [ ] Check environment variable `AZCOPY_RESUMABLE_DOWNLOAD`
- [ ] Add resume detection logic
  - [ ] Generate chunk progress file path
  - [ ] Call `CanResume()` to check for existing progress
  - [ ] If resumable, load existing `ChunkProgressFile`
  - [ ] Get list of pending chunks only
  - [ ] Log resume statistics (X/Y chunks complete)
- [ ] Add fresh download logic for resumable mode
  - [ ] Create new `RandomAccessFileWriter`
  - [ ] Create new `ChunkProgressFile`
  - [ ] Initialize with source metadata (MD5, size)
- [ ] Add fallback to non-resumable mode
  - [ ] If any initialization fails, fall back
  - [ ] Log warning about fallback
  - [ ] Continue with existing sequential download

**Modify chunk scheduling loop (~line 309):**
- [ ] Add check before scheduling each chunk
  - [ ] If resumable mode + chunk already complete, skip
  - [ ] Immediately call `ReportChunkDone()` for skipped chunk
  - [ ] Add skipped bytes to successful bytes counter
  - [ ] Log chunks being skipped (debug level)
- [ ] Add chunk index to ChunkID
  - [ ] Call `id.SetChunkIndex(chunkIndex)` for tracking
- [ ] Generate appropriate download function
  - [ ] Use `GenerateResumableDownloadFunc()` if resumable
  - [ ] Use `GenerateDownloadFunc()` if not resumable
  - [ ] Pass `RandomAccessFileWriter` for resumable mode

**Modify failure handling (~line 477 in `commonDownloaderCompletion`):**
- [ ] Check if transfer is resumable download
- [ ] Add new method `jptm.IsResumableDownload()` to check
- [ ] If resumable and failed, keep temp file
- [ ] If resumable and failed, keep chunk progress file
- [ ] Log message: "Keeping partial download for resume"
- [ ] If not resumable, delete temp file as before

**Add cleanup in epilogue (~line 500):**
- [ ] After successful finalize, delete chunk progress file
- [ ] Best effort deletion (ignore errors)
- [ ] Log cleanup at debug level

**Helper functions to add:**
- [ ] `getChunkProgressPath(jptm IJobPartTransferMgr) string`
  - [ ] Generate path based on job ID, part number, transfer index
  - [ ] Use same directory as job plan files
- [ ] `containsChunk(chunks []uint32, target uint32) bool`
  - [ ] Helper to check if chunk in pending list
- [ ] `supportsRandomAccess(jptm IJobPartTransferMgr) bool`
  - [ ] Check if downloader supports resume
  - [ ] Type assert to `resumableDownloader` interface

**Unit Tests:** `ste/xfer-remoteToLocal-file_test.go` (EXTEND or NEW)
- [ ] `TestDetermineResumableDownload`
  - [ ] Large file -> true
  - [ ] Small file -> false
  - [ ] Decompression enabled -> false
  - [ ] Environment variable disabled -> false
- [ ] `TestResumeDetection`
  - [ ] Existing progress file found -> resume
  - [ ] No progress file -> fresh download
  - [ ] Corrupted progress file -> fresh download
- [ ] `TestChunkSkipping`
  - [ ] 50% chunks complete -> skip 50%
  - [ ] Verify skipped chunks reported as done
  - [ ] Verify bytes counter updated correctly
- [ ] `TestFailureHandling`
  - [ ] Resumable download failure -> keep temp file
  - [ ] Non-resumable failure -> delete temp file
  - [ ] Verify progress file preserved on failure
- [ ] `TestSuccessCleanup`
  - [ ] Successful transfer -> progress file deleted
  - [ ] Temp file renamed to final
  - [ ] Verify no artifacts left

#### 5.2.2. Downloader Interface Extension
**File:** `ste/downloader.go` (MODIFY)
- [ ] Define `resumableDownloader` interface
  - [ ] Embed existing `downloader` interface
  - [ ] Add `GenerateResumableDownloadFunc()` method signature
  - [ ] Add `SupportsResume() bool` method
- [ ] Update comments explaining when to use each interface
- [ ] Ensure backward compatibility with existing downloaders

**Unit Tests:** `ste/downloader_test.go` (EXTEND)
- [ ] `TestResumableDownloaderInterface`
  - [ ] Verify interface implemented correctly
  - [ ] Verify method signatures match

#### 5.2.3. Blob Downloader Implementation
**File:** `ste/downloader-blob.go` (MODIFY)
- [ ] Implement `SupportsResume()` method
  - [ ] Return `true` (blobs always support range requests)
- [ ] Implement `GenerateResumableDownloadFunc()` method
  - [ ] Similar structure to existing `GenerateDownloadFunc()`
  - [ ] Use `DownloadStream()` with range parameter
  - [ ] Download chunk to memory buffer
  - [ ] Call `writer.WriteChunk()` with index, offset, data
  - [ ] Handle errors and propagate to `jptm.FailActiveDownload()`
  - [ ] On last chunk, call `writer.Finalize()`
  - [ ] Validate final MD5 if available
  - [ ] Set actual MD5 on jptm
- [ ] Respect pacer for rate limiting
- [ ] Add proper error context to all failures
- [ ] Ensure chunk-level retry logic works

**Unit Tests:** `ste/downloader-blob_test.go` (EXTEND)
- [ ] `TestBlobSupportsResume`
  - [ ] Verify returns true
- [ ] `TestBlobResumableDownloadFunc`
  - [ ] Mock blob client
  - [ ] Download single chunk
  - [ ] Verify WriteChunk called correctly
  - [ ] Verify chunk marked complete
- [ ] `TestBlobResumableDownloadError`
  - [ ] Network error during download
  - [ ] Verify error propagated
  - [ ] Verify chunk not marked complete
- [ ] `TestBlobResumableDownloadFinalize`
  - [ ] Last chunk triggers finalize
  - [ ] MD5 validation occurs
  - [ ] File renamed correctly

---

### 5.3. Phase 3: Other Downloaders

#### 5.3.1. Azure Files Downloader
**File:** `ste/downloader-azureFiles.go` (MODIFY)
- [ ] Implement `SupportsResume()` method
  - [ ] Return `true` (Azure Files supports range requests)
- [ ] Implement `GenerateResumableDownloadFunc()` method
  - [ ] Use `DownloadStream()` with range options
  - [ ] Handle Azure Files-specific headers
  - [ ] Preserve SMB metadata handling
  - [ ] Call `writer.WriteChunk()` for data
  - [ ] Handle finalization with metadata
- [ ] Handle SMB attributes in resumable mode
- [ ] Ensure permission preservation works

**Unit Tests:** `ste/downloader-azureFiles_test.go` (EXTEND)
- [ ] `TestAzureFilesSupportsResume`
- [ ] `TestAzureFilesResumableDownload`
- [ ] `TestAzureFilesMetadataPreservation`
  - [ ] Verify SMB attributes preserved on resume
- [ ] `TestAzureFilesResumableWithEncryption`
  - [ ] If encryption involved, verify compatibility

#### 5.3.2. HTTP Downloader
**File:** `ste/downloader-http.go` (MODIFY)
- [ ] Implement `SupportsResume()` method
  - [ ] Check if server supports `Accept-Ranges` header
  - [ ] Perform HEAD request to check on prologue
  - [ ] Return `true` only if ranges supported
  - [ ] Cache result to avoid repeated checks
- [ ] Implement `GenerateResumableDownloadFunc()` method
  - [ ] Add `Range: bytes=X-Y` header to request
  - [ ] Handle 206 Partial Content response
  - [ ] Handle servers that ignore range (fallback)
  - [ ] Call `writer.WriteChunk()` for data
  - [ ] Handle redirect following
- [ ] Add warning if range not supported
- [ ] Fall back to non-resumable gracefully

**Unit Tests:** `ste/downloader-http_test.go` (EXTEND)
- [ ] `TestHTTPSupportsResume_WithRanges`
  - [ ] Server sends Accept-Ranges header -> true
- [ ] `TestHTTPSupportsResume_NoRanges`
  - [ ] Server doesn't support ranges -> false
- [ ] `TestHTTPResumableDownload`
  - [ ] Mock HTTP server with range support
  - [ ] Verify range requests sent correctly
- [ ] `TestHTTPResumableDownloadNoRangeSupport`
  - [ ] Server ignores range header
  - [ ] Falls back to full download
- [ ] `TestHTTPResumableWithRedirects`
  - [ ] Handle 301/302 redirects
  - [ ] Verify range preserved in redirect

#### 5.3.3. BlobFS Downloader
**File:** `ste/downloader-blobFS.go` (MODIFY)
- [ ] Implement `SupportsResume()` method
  - [ ] Return `true` (BlobFS/ADLS Gen2 supports ranges)
- [ ] Implement `GenerateResumableDownloadFunc()` method
  - [ ] Use appropriate ADLS Gen2 API for range reads
  - [ ] Handle hierarchical namespace specifics
  - [ ] Call `writer.WriteChunk()` for data
  - [ ] Handle ACL preservation if needed
- [ ] Ensure POSIX attributes preserved

**Unit Tests:** `ste/downloader-blobFS_test.go` (EXTEND)
- [ ] `TestBlobFSSupportsResume`
- [ ] `TestBlobFSResumableDownload`
- [ ] `TestBlobFSACLPreservation`
  - [ ] Verify ACLs preserved on resume

---

### 5.4. Phase 4: Job Management Integration

#### 5.4.1. Job Part Transfer Manager
**File:** `ste/mgr-JobPartTransferMgr.go` (MODIFY)
- [ ] Add `isResumableDownload bool` field to transfer struct
- [ ] Add `IsResumableDownload() bool` method
  - [ ] Return the field value
- [ ] Add `SetResumableDownload(value bool)` method
  - [ ] Set the field value
- [ ] Initialize field in constructor

**Unit Tests:** Extend existing tests in same file
- [ ] `TestIsResumableDownload`
  - [ ] Verify getter/setter work correctly
  - [ ] Verify default value is false

#### 5.4.2. Resume Command Enhancement
**File:** `cmd/jobsResume.go` (MODIFY)
- [ ] Add chunk progress display function
  - [ ] Enumerate all failed transfers in job
  - [ ] For each transfer, check for chunk progress file
  - [ ] If found, open and read statistics
  - [ ] Display: "filename: X/Y chunks (Z%)"
- [ ] Add to resume command output
  - [ ] Show chunk progress before resuming
  - [ ] Show estimated data already downloaded
- [ ] Add `--show-progress` flag (optional)
  - [ ] Detailed per-file chunk status
  - [ ] Progress bar visualization

**Unit Tests:** `cmd/jobsResume_test.go` (EXTEND or NEW)
- [ ] `TestResumeChunkProgressDisplay`
  - [ ] Mock job with partial progress
  - [ ] Verify progress displayed correctly
- [ ] `TestResumeWithoutChunkProgress`
  - [ ] Legacy job without progress files
  - [ ] Should work normally without errors

#### 5.4.3. Jobs List Enhancement
**File:** `cmd/jobs.go` (MODIFY)
- [ ] Add chunk progress to job status output
  - [ ] When listing jobs, check for progress files
  - [ ] Show aggregate progress across all transfers
  - [ ] Format: "Progress: 45% (2.3GB/5GB)"
- [ ] Add detailed transfer-level progress (optional flag)
  - [ ] `azcopy jobs show <jobID> --detailed`
  - [ ] List each transfer with chunk status

**Unit Tests:** `cmd/jobs_test.go` (EXTEND)
- [ ] `TestJobsListWithChunkProgress`
  - [ ] Verify progress displayed in list
- [ ] `TestJobsShowDetailed`
  - [ ] Verify detailed view includes chunk info

---

### 5.5. Phase 5: Configuration & Environment

#### 5.5.1. Environment Variables
**File:** `common/gcpUtils.go` or new `common/config.go` (MODIFY/NEW)
- [ ] Add `AZCOPY_RESUMABLE_DOWNLOAD` check
  - [ ] Default: `true`
  - [ ] Parse boolean value
- [ ] Add `AZCOPY_RESUMABLE_THRESHOLD` check
  - [ ] Default: `268435456` (256MB)
  - [ ] Parse byte size
- [ ] Add `AZCOPY_RESUMABLE_CHUNK_SIZE` check
  - [ ] Default: `67108864` (64MB)
  - [ ] Parse byte size
  - [ ] Validate: must be multiple of 4MB for blobs
- [ ] Add `AZCOPY_RESUME_SKIP_MD5` check
  - [ ] Default: `false`
  - [ ] Skip MD5 validation on resumed transfers
- [ ] Add `AZCOPY_CHUNK_PROGRESS_DIR` check
  - [ ] Default: same as plan files directory
  - [ ] Allow custom location for progress files

**Unit Tests:** `common/config_test.go` (NEW or EXTEND)
- [ ] `TestResumableDownloadEnvVars`
  - [ ] Parse enabled/disabled
  - [ ] Parse threshold values
  - [ ] Parse chunk size
  - [ ] Invalid values -> defaults
- [ ] `TestResumableThresholdValidation`
  - [ ] Too small -> minimum value
  - [ ] Too large -> maximum value
  - [ ] Negative -> default

#### 5.5.2. Command-Line Flags (Optional)
**File:** `cmd/copy.go` (MODIFY)
- [ ] Add `--resumable` flag (bool)
  - [ ] Override environment variable
  - [ ] Default: `nil` (use env var)
- [ ] Add `--resumable-threshold` flag (string/size)
  - [ ] Override default threshold
  - [ ] Parse with size suffix (MB, GB)
- [ ] Add validation for flag combinations
  - [ ] Can't use with `--decompress`
  - [ ] Warning if file smaller than threshold

**Unit Tests:** `cmd/copy_test.go` (EXTEND)
- [ ] `TestCopyResumableFlag`
  - [ ] Parse flag correctly
  - [ ] Override env variable
- [ ] `TestCopyResumableFlagValidation`
  - [ ] Invalid combinations rejected
  - [ ] Helpful error messages

---

### 5.6. Phase 6: Edge Cases & Error Handling

#### 5.6.1. Source Change Detection
**File:** `common/randomAccessFileWriter.go` (enhance CanResume)
- [ ] Add source metadata validation
  - [ ] Store ETag/LastModified in progress file header
  - [ ] Compare on resume attempt
  - [ ] If different, reject resume
- [ ] Add source size validation
  - [ ] Compare current size vs. stored size
  - [ ] If different, reject resume
- [ ] Log when resume rejected due to source change

**Unit Tests:** Extend `common/randomAccessFileWriter_test.go`
- [ ] `TestCanResume_SourceChanged`
  - [ ] ETag different -> false
  - [ ] Size different -> false
  - [ ] LastModified different -> false (with tolerance)
- [ ] `TestCanResume_SourceUnchanged`
  - [ ] Same metadata -> true

#### 5.6.2. Corruption Detection
**File:** `ste/chunkProgressFile.go` (enhance validation)
- [ ] Add checksum to header
  - [ ] CRC32 of header fields
  - [ ] Validate on open
- [ ] Add periodic integrity check
  - [ ] Verify chunk count matches file size
  - [ ] Verify completed count matches bitmap
- [ ] Add recovery mode
  - [ ] If recoverable, fix corruption
  - [ ] If not, reject and restart fresh

**Unit Tests:** Extend `ste/chunkProgressFile_test.go`
- [ ] `TestCorruptedHeader`
  - [ ] Invalid checksum -> error
  - [ ] Recovery not possible
- [ ] `TestCorruptedChunkStatus`
  - [ ] Out-of-range status value
  - [ ] Recovery by resetting to pending

#### 5.6.3. Disk Space Handling
**File:** `common/randomAccessFileWriter.go` (enhance)
- [ ] Add disk space check before starting
  - [ ] Check available space >= file size
  - [ ] Warn if space marginal
- [ ] Handle ENOSPC error during WriteChunk
  - [ ] Propagate error with helpful message
  - [ ] Preserve progress for retry after cleanup
- [ ] Add space reservation (platform-specific)
  - [ ] Linux: fallocate()
  - [ ] Windows: SetEndOfFile()
  - [ ] Prevents ENOSPC mid-download

**Unit Tests:** Extend `common/randomAccessFileWriter_test.go`
- [ ] `TestDiskSpaceCheck`
  - [ ] Insufficient space -> error
  - [ ] Sufficient space -> proceed
- [ ] `TestENOSPCHandling`
  - [ ] Mock disk full error
  - [ ] Verify progress preserved
  - [ ] Verify helpful error message

#### 5.6.4. Concurrent Resume Protection
**File:** `ste/chunkProgressFile.go` (add locking)
- [ ] Add file-based locking
  - [ ] Create `.lock` file alongside progress file
  - [ ] Use flock() on Linux, LockFileEx() on Windows
  - [ ] Timeout and fail if lock held
- [ ] Add process detection
  - [ ] Check if locking process still alive
  - [ ] Break stale locks automatically
- [ ] Graceful failure message
  - [ ] "Another process is resuming this transfer"

**Unit Tests:** Extend `ste/chunkProgressFile_test.go`
- [ ] `TestConcurrentResumeBlocked`
  - [ ] First process acquires lock
  - [ ] Second process fails gracefully
- [ ] `TestStaleLockRecovery`
  - [ ] Lock file from dead process
  - [ ] Automatically broken and reused

---

### 5.7. Phase 7: Testing & Validation

#### 5.7.1. Unit Tests Summary
**All unit test files from previous phases:**
- [ ] `ste/chunkProgressFile_test.go` - 10 tests minimum
- [ ] `common/randomAccessFileWriter_test.go` - 10 tests minimum
- [ ] `common/chunkStatusLogger_test.go` - 2 tests extended
- [ ] `ste/xfer-remoteToLocal-file_test.go` - 5 tests minimum
- [ ] `ste/downloader_test.go` - 1 test extended
- [ ] `ste/downloader-blob_test.go` - 4 tests minimum
- [ ] `ste/downloader-azureFiles_test.go` - 4 tests minimum
- [ ] `ste/downloader-http_test.go` - 5 tests minimum
- [ ] `ste/downloader-blobFS_test.go` - 3 tests minimum
- [ ] `common/config_test.go` - 3 tests minimum
- [ ] `cmd/copy_test.go` - 2 tests extended
- [ ] `cmd/jobsResume_test.go` - 2 tests minimum
- [ ] `cmd/jobs_test.go` - 2 tests extended
- [ ] **Total: 53+ unit tests**
- [ ] **Target coverage: 90%+ of new code**

**Run all unit tests:**
- [ ] `go test -timeout=1h -v -coverprofile=coverage.txt ./ste`
- [ ] `go test -timeout=1h -v -coverprofile=coverage.txt ./common`
- [ ] `go test -timeout=1h -v -coverprofile=coverage.txt ./cmd`
- [ ] Verify coverage meets target (90%+)

#### 5.7.2. Integration Tests
**File:** `e2etest/resume_test.go` (NEW)
- [ ] `TestResumableDownload_Fresh`
  - [ ] Download 512MB blob in resumable mode
  - [ ] Verify chunk progress file created
  - [ ] Verify all chunks marked complete
  - [ ] Verify progress file deleted on success
  - [ ] Verify final MD5 correct
- [ ] `TestResumableDownload_Resume50Percent`
  - [ ] Start 1GB download
  - [ ] Simulate failure after 50% complete
  - [ ] Resume the job
  - [ ] Verify only remaining 50% downloaded
  - [ ] Verify final file correct
  - [ ] Measure network bytes (should be ~50% of file size)
- [ ] `TestResumableDownload_Resume90Percent`
  - [ ] Start 1GB download
  - [ ] Simulate failure after 90% complete
  - [ ] Resume the job
  - [ ] Verify only remaining 10% downloaded
  - [ ] Verify final file correct
- [ ] `TestResumableDownload_MultipleResumes`
  - [ ] Download 1GB file
  - [ ] Fail at 25%, resume
  - [ ] Fail at 50%, resume
  - [ ] Fail at 75%, resume
  - [ ] Complete successfully
  - [ ] Verify cumulative data downloaded correct
- [ ] `TestResumableDownload_SourceChanged`
  - [ ] Start download
  - [ ] Fail mid-download
  - [ ] Change source blob (upload new version)
  - [ ] Attempt resume
  - [ ] Verify resume rejected, fresh download started
- [ ] `TestResumableDownload_CorruptedProgress`
  - [ ] Start download
  - [ ] Fail mid-download
  - [ ] Corrupt chunk progress file
  - [ ] Attempt resume
  - [ ] Verify falls back to fresh download
- [ ] `TestResumableDownload_CorruptedData`
  - [ ] Start download with chunk MD5 enabled
  - [ ] Fail mid-download
  - [ ] Corrupt some completed chunks in temp file
  - [ ] Attempt resume
  - [ ] Verify corrupted chunks re-downloaded
- [ ] `TestResumableDownload_SmallFile`
  - [ ] Download 128MB file (below threshold)
  - [ ] Verify non-resumable mode used
  - [ ] Verify no chunk progress file created
- [ ] `TestResumableDownload_Concurrent`
  - [ ] Start download
  - [ ] Attempt second resume while first running
  - [ ] Verify second attempt fails gracefully
- [ ] `TestResumableDownload_DiskFull`
  - [ ] Start download to partition with limited space
  - [ ] Fill disk during download
  - [ ] Verify ENOSPC handled gracefully
  - [ ] Free space and resume
  - [ ] Verify completes successfully
- [ ] `TestResumableDownload_CancelResume`
  - [ ] Start download, cancel mid-way
  - [ ] Resume, cancel again mid-way
  - [ ] Resume final time and complete
  - [ ] Verify cumulative progress tracked correctly
- [ ] `TestResumableDownload_AzureFiles`
  - [ ] Test resumable download from Azure Files
  - [ ] Verify SMB metadata preserved
- [ ] `TestResumableDownload_HTTP_RangeSupported`
  - [ ] Test resumable download from HTTP server with ranges
  - [ ] Verify resume works correctly
- [ ] `TestResumableDownload_HTTP_NoRangeSupport`
  - [ ] Test download from HTTP server without ranges
  - [ ] Verify falls back to non-resumable
- [ ] **Total: 14 integration tests**
- [ ] **All tests must pass**

#### 5.7.3. E2E Tests with Real Storage
**File:** `e2etest/resume_e2e_test.go` (NEW)
- [ ] `TestE2E_ResumeBlobDownload_1GB`
  - [ ] Upload 1GB blob to test storage account
  - [ ] Start azcopy download
  - [ ] Kill process with SIGTERM at 60%
  - [ ] Run `azcopy jobs resume`
  - [ ] Verify completion
  - [ ] Verify file MD5 matches source
  - [ ] Verify network bytes < full file size
- [ ] `TestE2E_ResumeBlobDownload_10GB`
  - [ ] Test with very large file (10GB)
  - [ ] Kill at 80% completion
  - [ ] Resume and verify
  - [ ] Check progress file size reasonable
- [ ] `TestE2E_ResumeMultipleFiles`
  - [ ] Download 10 files (500MB each)
  - [ ] Kill process mid-transfer
  - [ ] Verify some files complete, some partial
  - [ ] Resume all partial downloads
  - [ ] Verify all complete correctly
- [ ] `TestE2E_ResumeWithMD5Validation`
  - [ ] Enable MD5 validation
  - [ ] Download with resume
  - [ ] Verify MD5 validated correctly
- [ ] `TestE2E_ResumeCrossVersion`
  - [ ] Start download with version N
  - [ ] Kill process
  - [ ] Resume with version N+1
  - [ ] Verify backward compatibility
- [ ] **Total: 5 E2E tests**
- [ ] **All tests must pass**
- [ ] **Run with: `go test -timeout=2h -v ./e2etest -run Resume`**

#### 5.7.4. Performance Tests
**File:** `e2etest/resume_perf_test.go` (NEW)
- [ ] `TestPerf_ResumableDownload_Overhead`
  - [ ] Download same file with/without resumable mode
  - [ ] Measure completion time
  - [ ] Verify overhead < 5%
- [ ] `TestPerf_ResumableDownload_MemoryUsage`
  - [ ] Download 10GB file in resumable mode
  - [ ] Monitor memory usage
  - [ ] Verify no memory leaks
  - [ ] Verify reasonable memory footprint
- [ ] `TestPerf_ChunkProgressFile_WriteSpeed`
  - [ ] Benchmark MarkChunkComplete() throughput
  - [ ] Target: >10,000 ops/sec
- [ ] `TestPerf_RandomAccessWrite_Throughput`
  - [ ] Measure WriteChunk() throughput
  - [ ] Compare with sequential write
  - [ ] Verify <10% degradation on SSD
- [ ] **Total: 4 performance tests**
- [ ] **Verify performance targets met**

#### 5.7.5. Compatibility Tests
**File:** `e2etest/resume_compat_test.go` (NEW)
- [ ] `TestCompat_ExistingJobsWithoutResume`
  - [ ] Load job from previous azcopy version
  - [ ] Resume with new version
  - [ ] Verify works (starts fresh, no chunks)
- [ ] `TestCompat_DowngradeGraceful`
  - [ ] Create resumable download with new version
  - [ ] Attempt to resume with old version
  - [ ] Verify old version ignores progress, restarts
- [ ] `TestCompat_PlanFileFormat`
  - [ ] Verify plan file format unchanged
  - [ ] Verify can read old plan files
- [ ] **Total: 3 compatibility tests**
- [ ] **All tests must pass**

---

### 5.8. Phase 8: Documentation & Polish

#### 5.8.1. User Documentation
**File:** `docs/resumable-downloads.md` (NEW)
- [ ] Overview of resumable downloads feature
- [ ] How it works (high-level)
- [ ] When it's enabled automatically
- [ ] How to check resume progress
- [ ] Environment variables reference
- [ ] Command-line flags reference
- [ ] Troubleshooting guide
- [ ] FAQ section

#### 5.8.2. Code Documentation
- [ ] Add package-level comments to new files
- [ ] Add godoc comments to all exported functions
- [ ] Add inline comments for complex logic
- [ ] Add example code snippets
- [ ] Update ARCHITECTURE.md if exists

#### 5.8.3. Logging & Telemetry
**Files:** Various downloader and transfer files
- [ ] Add info-level log when resumable download starts
  - [ ] "Starting resumable download for <file>"
- [ ] Add info-level log when resume detected
  - [ ] "Resuming download: X/Y chunks complete"
- [ ] Add debug-level log for chunk completion
  - [ ] "Chunk <N> complete: <offset>-<end>"
- [ ] Add warning-level log when falling back
  - [ ] "Falling back to non-resumable download: <reason>"
- [ ] Add telemetry for resume statistics
  - [ ] Bytes saved by resuming
  - [ ] Resume success rate
  - [ ] Average completion percentage at resume

#### 5.8.4. Error Messages
- [ ] Review all error messages for clarity
- [ ] Add actionable suggestions
  - [ ] "Resume failed: source file changed. Restarting fresh download."
  - [ ] "Chunk progress file corrupted. Restarting download."
  - [ ] "Disk full. Free space and run 'azcopy jobs resume <jobID>'."
- [ ] Add error codes for programmatic handling

---

### 5.9. Phase 9: Final Validation

#### 5.9.1. Pre-Release Checklist
- [ ] All unit tests passing (53+ tests)
- [ ] All integration tests passing (14 tests)
- [ ] All E2E tests passing (5 tests)
- [ ] All performance tests passing (4 tests)
- [ ] All compatibility tests passing (3 tests)
- [ ] Code coverage >= 90% for new code
- [ ] No regressions in existing tests
- [ ] Manual testing on Windows, Linux, macOS
- [ ] Manual testing with various file sizes
  - [ ] Small (< threshold)
  - [ ] Medium (256MB - 1GB)
  - [ ] Large (1GB - 10GB)
  - [ ] Very large (>10GB)
- [ ] Manual testing with all storage types
  - [ ] Azure Blob Storage
  - [ ] Azure Files
  - [ ] Azure Data Lake Gen2
  - [ ] HTTP servers (with and without range support)
- [ ] Stress testing
  - [ ] 1000 files resuming simultaneously
  - [ ] Very large number of chunks (100K+)
- [ ] Security review
  - [ ] No sensitive data in logs
  - [ ] File permissions correct
  - [ ] No injection vulnerabilities

#### 5.9.2. Performance Validation
- [ ] Fresh download overhead < 5%
- [ ] Memory usage reasonable (<100MB extra)
- [ ] Progress file size reasonable
  - [ ] 1GB file: <100KB progress file
  - [ ] 1TB file: <1MB progress file
- [ ] Random write performance acceptable
  - [ ] <10% slower than sequential on SSD
  - [ ] <20% slower than sequential on HDD

#### 5.9.3. Backward Compatibility Validation
- [ ] Old jobs can be resumed with new code
- [ ] Plan file format unchanged
- [ ] No breaking changes to existing functionality
- [ ] Graceful handling of old progress files
- [ ] Graceful handling by old versions (ignore)

---

## 6. Test Coverage Summary

### 6.1. Unit Tests: 53+ tests
- Core infrastructure: 23 tests
- Download flow: 5 tests
- Downloaders: 16 tests
- Configuration: 5 tests
- Commands: 4 tests

### 6.2. Integration Tests: 14 tests
- Resume scenarios
- Error handling
- Edge cases

### 6.3. E2E Tests: 5 tests
- Real storage testing
- Large file scenarios
- Cross-version compatibility

### 6.4. Performance Tests: 4 tests
- Overhead measurement
- Memory usage
- Throughput benchmarks

### 6.5. Compatibility Tests: 3 tests
- Version compatibility
- Format compatibility

### **Total: 79+ tests**
### **Target: 100% coverage of new implementation**

---

## 7. Files to Create

| File | Description |
|------|-------------|
| `ste/chunkProgressFile.go` | Chunk progress persistence |
| `common/randomAccessFileWriter.go` | Random-access file writer |
| `ste/chunkProgressFile_test.go` | Unit tests |
| `common/randomAccessFileWriter_test.go` | Unit tests |

## 8. Files to Modify

| File | Changes |
|------|---------|
| `ste/xfer-remoteToLocal-file.go` | Main download flow, failure handling |
| `ste/downloader.go` | Add resumableDownloader interface |
| `ste/downloader-blob.go` | Implement GenerateResumableDownloadFunc |
| `ste/downloader-azureFiles.go` | Implement GenerateResumableDownloadFunc |
| `ste/downloader-http.go` | Implement GenerateResumableDownloadFunc |
| `ste/downloader-blobFS.go` | Implement GenerateResumableDownloadFunc |
| `common/chunkStatusLogger.go` | Add ChunkIndex to ChunkID |
| `cmd/jobsResume.go` | Show chunk progress |
| `ste/mgr-JobPartTransferMgr.go` | Add IsResumableDownload() method |

---

## 9. Configuration Options

### 9.1. Environment Variables

```go
// Enable/disable resumable downloads (default: true for large files)
AZCOPY_RESUMABLE_DOWNLOAD=true

// Minimum file size to enable resumable download (default: 256MB)
AZCOPY_RESUMABLE_THRESHOLD=268435456

// Chunk size for progress tracking (default: 64MB)
AZCOPY_RESUMABLE_CHUNK_SIZE=67108864

// Skip MD5 validation on resume (default: false)
AZCOPY_RESUME_SKIP_MD5=false
```

### 9.2. Command-Line Flags (Optional)

```bash
# Force resumable mode even for smaller files
azcopy copy "source" "dest" --resumable

# Disable resumable mode
azcopy copy "source" "dest" --resumable=false
```

---

## 10. Edge Cases & Error Handling

### 10.1. Source file changed during resume
- Compare source MD5/ETag with stored value in progress file
- If different, discard progress and restart fresh

### 10.2. Chunk progress file corrupted
- Validate magic bytes and checksum on open
- If invalid, delete and start fresh

### 10.3. Disk full during download
- Existing error handling applies
- Progress file preserved for resume after freeing space

### 10.4. Concurrent resume attempts
- Use file locking on progress file
- Second attempt waits or fails gracefully

### 10.5. Very large files (>1TB)
- Bitmap size = 1TB / 64MB = 16K chunks = 2KB bitmap
- Progress file size = 64 + (24 * 16K) = ~400KB - manageable

### 10.6. Network file systems
- Random WriteAt should work on NFS/SMB
- Progress file uses mmap - may have issues (fall back to regular I/O)

---

## 11. Performance Considerations

### 11.1. Memory Usage
- No sequential buffering needed (current: can buffer multiple chunks)
- Per-chunk buffer: 64MB (adjustable)
- Progress file mmap: ~400KB for 1TB file

### 11.2. Disk I/O
- Random writes instead of sequential (potential impact on HDDs)
- Consider: sequential pre-allocation with random writes
- SSD performance should be similar or better

### 11.3. CPU
- Per-chunk MD5: additional ~3% CPU overhead
- Can be disabled via environment variable

---

## 12. Backward Compatibility

1. **Plan file format**: Unchanged - chunk progress in separate files
2. **Existing jobs**: Can resume with new code (will start fresh, no chunk progress)
3. **Downgrade**: New progress files ignored by old versions (downloads restart)
4. **Mixed versions**: Safe - progress files are advisory only

---

## 13. Testing Strategy

### 13.1. Unit Tests
- ChunkProgressFile CRUD operations
- RandomAccessFileWriter write/finalize
- Edge cases: corrupted progress file, concurrent access

### 13.2. Integration Tests
- Fresh download with progress tracking
- Resume after simulated failure at various points
- Resume with source file changed
- Resume with corrupted progress file

### 13.3. E2E Tests
- Real blob download with kill -9 mid-transfer
- Resume and verify final MD5
- Large file (>1GB) resume test

---

## 14. Success Metrics

1. **Resume efficiency**: % of bytes not re-downloaded on resume
2. **Overhead**: Additional time for fresh downloads (<5% acceptable)
3. **Reliability**: No data corruption in resumed downloads
4. **Compatibility**: All existing tests pass
