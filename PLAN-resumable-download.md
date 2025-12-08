# Implementation Plan: Resumable Chunk-Level Download

## Problem Statement

Currently, when a large file download (e.g., 500GB) fails at 80% completion:
1. The partial temp file (`.azDownload-<jobID>-<filename>`) is **deleted**
2. On `jobs resume`, the entire file is re-downloaded from byte 0
3. No chunk-level progress is persisted

## Design Goals

1. **Persist chunk completion status** to disk so resume can skip completed chunks
2. **Use random-access file writes** instead of sequential buffering
3. **Maintain backward compatibility** with existing plan file format
4. **Support all downloader types** (Blob, Azure Files, HTTP with Range support)
5. **Preserve MD5 validation** capability (compute incrementally or skip on resume)

---

## Architecture Overview

### New Components

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

### Key Design Decisions

| Decision | Choice | Rationale |
|----------|--------|-----------|
| Chunk status storage | Separate `.chunks` file | Don't break existing plan file format; easier to manage |
| File write mode | Random access (WriteAt) | Required for non-sequential chunk completion |
| MD5 handling | Per-chunk MD5 + final verification | Enables incremental validation |
| Threshold for feature | Files > 256MB | Overhead not worth it for small files |
| Temp file behavior | Keep on failure | Required for resume; rename on success |

---

## Detailed Design

### 1. Chunk Progress File Format

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
    Status    uint8     // 0=pending, 1=in-progress, 2=completed, 3=failed
    Reserved  [7]byte   // Alignment padding
    MD5       [16]byte  // MD5 of this chunk's data (optional)
}

// File layout:
// [Header: 64 bytes]
// [ChunkStatus array: 24 * NumChunks bytes]
// [Bitmap: ceil(NumChunks/8) bytes] - redundant but fast lookup
```

**Operations:**
```go
type ChunkProgressFile struct {
    path      string
    mmf       *common.MMF  // Memory-mapped for atomic updates
    header    *ChunkProgressFileHeader
    chunks    []ChunkStatus
}

func CreateChunkProgressFile(path string, totalSize, chunkSize int64, sourceMD5 []byte) (*ChunkProgressFile, error)
func OpenChunkProgressFile(path string) (*ChunkProgressFile, error)
func (cpf *ChunkProgressFile) MarkChunkComplete(chunkIndex uint32, md5 []byte) error
func (cpf *ChunkProgressFile) MarkChunkFailed(chunkIndex uint32) error
func (cpf *ChunkProgressFile) IsChunkComplete(chunkIndex uint32) bool
func (cpf *ChunkProgressFile) GetCompletedChunks() []uint32
func (cpf *ChunkProgressFile) GetPendingChunks() []uint32
func (cpf *ChunkProgressFile) Close() error
func (cpf *ChunkProgressFile) Delete() error
```

### 2. Random-Access File Writer

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

### 3. Modified Download Flow

**File:** `ste/xfer-remoteToLocal-file.go` (modify)

#### 3.1 New Threshold Constant

```go
const (
    azcopyTempDownloadPrefix      = ".azDownload-%s-"
    resumableDownloadThreshold    = 256 * 1024 * 1024  // 256MB
    defaultResumableChunkSize     = 64 * 1024 * 1024   // 64MB chunks for progress tracking
)
```

#### 3.2 Modified remoteToLocal_file() Function

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

#### 3.3 Modified Chunk Scheduling Loop

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

#### 3.4 Modified Failure Handling

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

### 4. Extended Downloader Interface

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

### 5. Modified Blob Downloader

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

### 6. Resume Job Modifications

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

### 7. Cleanup on Success

**File:** `ste/xfer-remoteToLocal-file.go` (modify in epilogue)

```go
// After successful rename, delete chunk progress file
if jptm.IsResumableDownload() {
    chunkProgressPath := getChunkProgressPath(jptm)
    os.Remove(chunkProgressPath) // Best effort, ignore errors
}
```

### 8. MD5 Validation Strategy

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

## Implementation Phases

### Phase 1: Core Infrastructure (Essential)
1. `ste/chunkProgressFile.go` - New file for chunk progress tracking
2. `common/randomAccessFileWriter.go` - New random-access writer
3. `common/chunkID.go` - Add ChunkIndex field to ChunkID

### Phase 2: Download Flow Integration
4. `ste/xfer-remoteToLocal-file.go` - Integrate resumable download logic
5. `ste/downloader.go` - Add resumableDownloader interface
6. `ste/downloader-blob.go` - Implement resumable download for blobs

### Phase 3: Other Downloaders
7. `ste/downloader-azureFiles.go` - Implement resumable download
8. `ste/downloader-http.go` - Implement resumable download (check Range support)
9. `ste/downloader-blobFS.go` - Implement resumable download

### Phase 4: Resume Command Enhancements
10. `cmd/jobsResume.go` - Show chunk-level progress
11. `cmd/jobs.go` - List command shows partial progress

### Phase 5: Testing & Polish
12. Unit tests for ChunkProgressFile
13. Unit tests for RandomAccessFileWriter
14. Integration tests for resume scenarios
15. E2E tests with real storage

---

## Files to Create

| File | Description |
|------|-------------|
| `ste/chunkProgressFile.go` | Chunk progress persistence |
| `common/randomAccessFileWriter.go` | Random-access file writer |
| `ste/chunkProgressFile_test.go` | Unit tests |
| `common/randomAccessFileWriter_test.go` | Unit tests |

## Files to Modify

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

## Configuration Options

### Environment Variables

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

### Command-Line Flags (Optional)

```bash
# Force resumable mode even for smaller files
azcopy copy "source" "dest" --resumable

# Disable resumable mode
azcopy copy "source" "dest" --resumable=false
```

---

## Edge Cases & Error Handling

### 1. Source file changed during resume
- Compare source MD5/ETag with stored value in progress file
- If different, discard progress and restart fresh

### 2. Chunk progress file corrupted
- Validate magic bytes and checksum on open
- If invalid, delete and start fresh

### 3. Disk full during download
- Existing error handling applies
- Progress file preserved for resume after freeing space

### 4. Concurrent resume attempts
- Use file locking on progress file
- Second attempt waits or fails gracefully

### 5. Very large files (>1TB)
- Bitmap size = 1TB / 64MB = 16K chunks = 2KB bitmap
- Progress file size = 64 + (24 * 16K) = ~400KB - manageable

### 6. Network file systems
- Random WriteAt should work on NFS/SMB
- Progress file uses mmap - may have issues (fall back to regular I/O)

---

## Performance Considerations

### Memory Usage
- No sequential buffering needed (current: can buffer multiple chunks)
- Per-chunk buffer: 64MB (adjustable)
- Progress file mmap: ~400KB for 1TB file

### Disk I/O
- Random writes instead of sequential (potential impact on HDDs)
- Consider: sequential pre-allocation with random writes
- SSD performance should be similar or better

### CPU
- Per-chunk MD5: additional ~3% CPU overhead
- Can be disabled via environment variable

---

## Backward Compatibility

1. **Plan file format**: Unchanged - chunk progress in separate files
2. **Existing jobs**: Can resume with new code (will start fresh, no chunk progress)
3. **Downgrade**: New progress files ignored by old versions (downloads restart)
4. **Mixed versions**: Safe - progress files are advisory only

---

## Testing Strategy

### Unit Tests
- ChunkProgressFile CRUD operations
- RandomAccessFileWriter write/finalize
- Edge cases: corrupted progress file, concurrent access

### Integration Tests
- Fresh download with progress tracking
- Resume after simulated failure at various points
- Resume with source file changed
- Resume with corrupted progress file

### E2E Tests
- Real blob download with kill -9 mid-transfer
- Resume and verify final MD5
- Large file (>1GB) resume test

---

## Success Metrics

1. **Resume efficiency**: % of bytes not re-downloaded on resume
2. **Overhead**: Additional time for fresh downloads (<5% acceptable)
3. **Reliability**: No data corruption in resumed downloads
4. **Compatibility**: All existing tests pass
