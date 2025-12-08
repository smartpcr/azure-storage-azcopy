# Is mmap Better for Concurrent Progress Tracking?

## Your Real-World Issues with Regular File I/O

Based on your experience with a previous implementation:

1. ❌ **File lock contention** - Multiple workers competing for locks
2. ❌ **Difficult correlation** - Hard to sync progress file with temp files and chunks
3. ❌ **Memory issues** - Loading/managing progress data
4. ❌ **Sync complexity** - Coordinating writes from concurrent workers

These are **exactly the problems mmap solves well!**

---

## Why Your Issues Happened with Regular File I/O

### Problem 1: File Lock Contention

```go
// Worker 1 writing chunk 5
func (cpf *ChunkProgressFile) MarkChunkComplete(idx uint32) error {
    cpf.mu.Lock()  // ⬅️ BLOCKS if Worker 2 is writing
    defer cpf.mu.Unlock()

    cpf.file.WriteAt(data, offset)
    cpf.file.Sync()  // ⬅️ Expensive I/O while holding lock
    return nil
}

// Worker 2 writing chunk 10 - BLOCKED waiting for Worker 1's lock
// Worker 3 writing chunk 15 - BLOCKED waiting for lock
// ... serialization kills parallelism
```

**The Problem:**
- With regular file I/O, you need a mutex to protect concurrent writes
- Each write holds the lock during I/O operations (slow)
- Workers serialize, defeating parallelism
- Can cause deadlocks if not careful

### Problem 2: Correlation Complexity

```go
// Single progress file for all transfers - how to find the right entry?
type ProgressFile struct {
    transfers []TransferProgress  // Which index is which transfer?
}

// Need complex mapping
transferID -> progressFileIndex -> chunkArray -> chunkStatus
           ↓
    How to keep this in sync with:
    - Temp file paths
    - RandomAccessFileWriter instances
    - Chunk completion states
```

### Problem 3: Memory Pressure

```go
// Loading entire progress file into memory
type ProgressFile struct {
    transfers []TransferProgress  // Could be 1000s of transfers
    chunks    [][]ChunkStatus     // Could be millions of chunks
}

// For 100 concurrent 1GB downloads = 100 * 16 chunks = 1600 chunks * 24 bytes
// Plus overhead = significant memory
```

---

## How mmap Solves These Issues

### ✅ Solution 1: Lock-Free Concurrent Writes

```go
type ChunkProgressFile struct {
    mmapRegion []byte  // Mapped memory
    header     *ChunkProgressFileHeader
    chunks     []ChunkStatus  // Slice over mmap region
}

// Worker 1 - No lock needed!
func (cpf *ChunkProgressFile) MarkChunkComplete(idx uint32) error {
    // Direct memory write - no lock needed if writing to different offsets
    atomic.StoreUint32(&cpf.chunks[idx].Status, ChunkStatusCompleted)
    atomic.AddUint32(&cpf.header.CompletedChunks, 1)

    // No blocking I/O here - just memory write
    return nil  // Fast!
}

// Worker 2, 3, 4... all write concurrently - no contention!
```

**Why This Works:**
- Each chunk has its own offset in memory
- Workers write to different memory locations (no interference)
- OS handles page-level locking automatically
- Can use atomic operations for safety
- No mutex needed for most operations

### ✅ Solution 2: Direct Memory Mapping = Simpler Correlation

```go
// One-to-one mapping: file offset = memory offset
type RandomAccessFileWriter struct {
    destFile      *os.File           // The actual download file
    progressMmap  []byte             // Memory-mapped progress file
    chunks        []ChunkStatus      // Direct slice into progressMmap
}

// Clear correlation:
chunk[5] in memory = byte offset 64 + (5 * 24) in file
↓
When accessing chunk[5], you're reading/writing the ACTUAL file bytes
↓
No need to track "which entry", it's always at chunks[idx]
```

**Simplification:**
```go
// Before (complex tracking)
progressFileIndex := cpf.transferMap[transferID]
chunkArray := cpf.transfers[progressFileIndex].chunks
chunkStatus := chunkArray[chunkIdx]

// After (direct access)
chunkStatus := cpf.chunks[chunkIdx]  // That's it!
```

### ✅ Solution 3: OS-Managed Memory (Less Pressure)

```go
// With regular I/O - You manage memory
type ProgressFile struct {
    data []byte  // You allocate and manage this
}

// With mmap - OS manages memory
type ProgressFile struct {
    mmap []byte  // OS pages this in/out as needed
}
```

**How OS Helps:**
- Progress file is 400KB for 1TB download
- OS only keeps "hot" pages in RAM (maybe 4-16KB)
- Cold pages automatically paged out
- You get virtual memory benefits without managing it
- Under memory pressure, OS evicts pages (not your problem)

### ✅ Solution 4: Simpler Sync Model

```go
// Regular I/O - complex synchronization
type ProgressFile struct {
    mu          sync.Mutex
    pendingWrites map[uint32]bool
    syncTicker  *time.Ticker
}

func background() {
    for range syncTicker.C {
        mu.Lock()
        // Flush pending writes
        file.Sync()
        mu.Unlock()
    }
}

// mmap - simple periodic sync
type ProgressFile struct {
    mmap []byte
}

func background() {
    for range time.Tick(5 * time.Second) {
        syscall.Msync(mmap, syscall.MS_ASYNC)  // That's it!
    }
}
```

---

## **Revised Recommendation: Use mmap for Your Use Case**

Given your specific issues, **mmap is the better choice** because:

### 1. Concurrent Access Pattern
```go
// Multiple workers writing to same file - mmap shines here
// Worker 1, 2, 3, 4 all writing different chunks simultaneously

type ChunkProgressFile struct {
    mmapData  []byte
    header    *ChunkProgressFileHeader  // At offset 0
    chunks    []ChunkStatus             // Array starting at offset 64
}

// No locks needed for writes to different chunks!
func (cpf *ChunkProgressFile) MarkChunkComplete(idx uint32, md5 []byte) error {
    // Atomic write to chunk status
    chunk := &cpf.chunks[idx]
    atomic.StoreUint32((*uint32)(unsafe.Pointer(&chunk.Status)), uint32(ChunkStatusCompleted))
    copy(chunk.MD5[:], md5)

    // Atomic increment of completed count
    atomic.AddUint32(&cpf.header.CompletedChunks, 1)

    return nil  // No blocking I/O!
}
```

### 2. Per-Transfer Progress Files
```go
// Your plan uses one progress file per transfer - perfect for mmap!
// Path: <planFolder>/<jobID>-<partNum>-<transferIdx>.chunks

// Each transfer has its own:
type Transfer struct {
    writer       *RandomAccessFileWriter  // Writing data to disk
    progressFile *ChunkProgressFile       // mmap'd progress tracking
}

// Clear 1:1 mapping:
transfer.progressFile.chunks[idx] ↔ transfer.writer.chunks[idx]
```

### 3. Read-Heavy Access Pattern
```go
// On resume, you read progress file to find pending chunks
func (cpf *ChunkProgressFile) GetPendingChunks() []uint32 {
    // Fast bitmap scan - all in memory
    var pending []uint32
    for i := 0; i < len(cpf.chunks); i++ {
        if cpf.chunks[i].Status != ChunkStatusCompleted {
            pending = append(pending, uint32(i))
        }
    }
    return pending
}

// With regular I/O, this would be:
// - Read entire file (slow)
// - Or seek + read each chunk entry (many syscalls)
```

---

## Recommended mmap Implementation

### Structure

```go
type ChunkProgressFile struct {
    file       *os.File
    mmapData   []byte              // Mapped region
    header     *ChunkProgressFileHeader
    chunks     []ChunkStatus
    syncTicker *time.Ticker
    done       chan struct{}
}

// File layout:
// [Header: 64 bytes]
// [ChunkStatus array: 24 * NumChunks bytes]
// [Bitmap: ceil(NumChunks/8) bytes]
```

### Create with mmap

```go
func CreateChunkProgressFile(path string, totalSize, chunkSize int64) (*ChunkProgressFile, error) {
    numChunks := uint32((totalSize + chunkSize - 1) / chunkSize)

    // Calculate file size
    headerSize := int64(64)
    chunksSize := int64(24 * numChunks)
    bitmapSize := int64((numChunks + 7) / 8)
    fileSize := headerSize + chunksSize + bitmapSize

    // Create file
    file, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0644)
    if err != nil {
        return nil, err
    }

    // Pre-allocate space
    if err := file.Truncate(fileSize); err != nil {
        file.Close()
        return nil, err
    }

    // Memory-map the file
    mmapData, err := syscall.Mmap(
        int(file.Fd()),
        0,
        int(fileSize),
        syscall.PROT_READ|syscall.PROT_WRITE,
        syscall.MAP_SHARED,
    )
    if err != nil {
        file.Close()
        return nil, fmt.Errorf("mmap failed: %w", err)
    }

    cpf := &ChunkProgressFile{
        file:     file,
        mmapData: mmapData,
        done:     make(chan struct{}),
    }

    // Create header struct at offset 0
    cpf.header = (*ChunkProgressFileHeader)(unsafe.Pointer(&mmapData[0]))

    // Create chunks slice at offset 64
    cpf.chunks = unsafe.Slice(
        (*ChunkStatus)(unsafe.Pointer(&mmapData[headerSize])),
        numChunks,
    )

    // Initialize header
    copy(cpf.header.Magic[:], "AZCCHUNK")
    cpf.header.Version = 1
    cpf.header.ChunkSize = chunkSize
    cpf.header.TotalSize = totalSize
    cpf.header.NumChunks = numChunks
    cpf.header.CompletedChunks = 0

    // Start background sync
    cpf.startBackgroundSync()

    return cpf, nil
}
```

### Lock-Free Writes

```go
func (cpf *ChunkProgressFile) MarkChunkComplete(idx uint32, md5 []byte) error {
    if idx >= cpf.header.NumChunks {
        return fmt.Errorf("chunk index out of range")
    }

    chunk := &cpf.chunks[idx]

    // Atomic writes (no lock needed!)
    atomic.StoreUint32(
        (*uint32)(unsafe.Pointer(&chunk.Status)),
        uint32(ChunkStatusCompleted),
    )

    // Copy MD5 (safe - only this worker writes to this chunk)
    copy(chunk.MD5[:], md5)

    // Atomic increment
    atomic.AddUint32(&cpf.header.CompletedChunks, 1)

    return nil
}
```

### Background Sync

```go
func (cpf *ChunkProgressFile) startBackgroundSync() {
    cpf.syncTicker = time.NewTicker(5 * time.Second)

    go func() {
        for {
            select {
            case <-cpf.syncTicker.C:
                // Async sync - doesn't block workers
                syscall.Msync(cpf.mmapData, syscall.MS_ASYNC)
            case <-cpf.done:
                return
            }
        }
    }()
}
```

### Read Operations (Lock-Free)

```go
func (cpf *ChunkProgressFile) IsChunkComplete(idx uint32) bool {
    status := atomic.LoadUint32(
        (*uint32)(unsafe.Pointer(&cpf.chunks[idx].Status)),
    )
    return status == ChunkStatusCompleted
}

func (cpf *ChunkProgressFile) GetPendingChunks() []uint32 {
    var pending []uint32
    for i := uint32(0); i < cpf.header.NumChunks; i++ {
        if !cpf.IsChunkComplete(i) {
            pending = append(pending, i)
        }
    }
    return pending
}

func (cpf *ChunkProgressFile) GetProgress() (completed, total uint32) {
    completed = atomic.LoadUint32(&cpf.header.CompletedChunks)
    total = cpf.header.NumChunks
    return
}
```

### Clean Shutdown

```go
func (cpf *ChunkProgressFile) Close() error {
    // Stop background sync
    if cpf.syncTicker != nil {
        cpf.syncTicker.Stop()
        close(cpf.done)
    }

    // Final sync (synchronous)
    if err := syscall.Msync(cpf.mmapData, syscall.MS_SYNC); err != nil {
        return fmt.Errorf("final msync failed: %w", err)
    }

    // Unmap
    if err := syscall.Munmap(cpf.mmapData); err != nil {
        return fmt.Errorf("munmap failed: %w", err)
    }

    // Close file
    return cpf.file.Close()
}
```

---

## Platform-Specific Implementation

### Linux/macOS

```go
// +build linux darwin

func mmapFile(file *os.File, size int) ([]byte, error) {
    return syscall.Mmap(
        int(file.Fd()),
        0,
        size,
        syscall.PROT_READ|syscall.PROT_WRITE,
        syscall.MAP_SHARED,
    )
}

func msyncFile(data []byte, flags int) error {
    return syscall.Msync(data, flags)
}

func munmapFile(data []byte) error {
    return syscall.Munmap(data)
}
```

### Windows

```go
// +build windows

import (
    "golang.org/x/sys/windows"
    "unsafe"
)

type mmapHandle struct {
    data   []byte
    handle windows.Handle
}

func mmapFile(file *os.File, size int) (*mmapHandle, error) {
    // CreateFileMapping
    handle, err := windows.CreateFileMapping(
        windows.Handle(file.Fd()),
        nil,
        windows.PAGE_READWRITE,
        uint32(size>>32),
        uint32(size),
        nil,
    )
    if err != nil {
        return nil, err
    }

    // MapViewOfFile
    addr, err := windows.MapViewOfFile(
        handle,
        windows.FILE_MAP_WRITE,
        0,
        0,
        uintptr(size),
    )
    if err != nil {
        windows.CloseHandle(handle)
        return nil, err
    }

    data := unsafe.Slice((*byte)(unsafe.Pointer(addr)), size)

    return &mmapHandle{
        data:   data,
        handle: handle,
    }, nil
}

func msyncFile(mh *mmapHandle) error {
    return windows.FlushViewOfFile(
        uintptr(unsafe.Pointer(&mh.data[0])),
        uintptr(len(mh.data)),
    )
}

func munmapFile(mh *mmapHandle) error {
    if err := windows.UnmapViewOfFile(uintptr(unsafe.Pointer(&mh.data[0]))); err != nil {
        return err
    }
    return windows.CloseHandle(mh.handle)
}
```

---

## Addressing Previous Concerns

### 1. "Platform Complexity"
**Solution:** Use build tags and abstract the platform-specific parts:

```go
// mmap.go - common interface
type MmapFile interface {
    Data() []byte
    Sync() error
    Close() error
}

// mmap_unix.go
// +build linux darwin
func NewMmapFile(file *os.File, size int) (MmapFile, error)

// mmap_windows.go
// +build windows
func NewMmapFile(file *os.File, size int) (MmapFile, error)
```

### 2. "Durability Concerns"
**Solution:** Background async sync + explicit sync on close:

```go
// During operation: async sync every 5 seconds (low overhead)
syscall.Msync(data, syscall.MS_ASYNC)

// On close/error: synchronous sync (guarantee durability)
syscall.Msync(data, syscall.MS_SYNC)
```

### 3. "Network File Systems"
**Solution:** Detect and fallback:

```go
func CreateChunkProgressFile(path string, ...) (*ChunkProgressFile, error) {
    // Try to detect NFS
    if isNetworkFileSystem(path) {
        return createRegularIOProgressFile(path, ...)
    }

    // Try mmap
    cpf, err := createMmapProgressFile(path, ...)
    if err != nil {
        // Fallback to regular I/O
        return createRegularIOProgressFile(path, ...)
    }
    return cpf, nil
}
```

### 4. "Crash Safety"
**Solution:** Accept eventual consistency (it's just progress tracking):

```go
// Worst case on crash:
// - Lose up to 5 seconds of progress updates
// - On resume, re-download a few chunks that were actually complete
// - Still better than re-downloading entire file!

// For critical points, force sync:
defer cpf.Sync()  // Before process exit
```

---

## Performance Benefits for Your Use Case

### Scenario: 10GB file, 64MB chunks = 160 chunks, 10 concurrent workers

#### Regular File I/O
```
Each worker writing 16 chunks:
- Acquire lock
- Seek to offset
- Write 24 bytes
- Sync
- Release lock

Total time: 160 chunks * (lock wait + I/O) = ~500-2000ms
Serialization overhead: High
```

#### mmap
```
Each worker writing 16 chunks:
- Atomic write to memory (no lock)
- Background async sync

Total time: 160 chunks * (memory write) = ~5-20ms
Serialization overhead: None
```

**Result:** 100x faster progress updates!

---

## Final Recommendation

### ✅ Use mmap because:

1. **Your architecture is per-transfer progress files** ✓
   - Small files (hundreds of KB)
   - One mmap per transfer
   - Perfect fit for mmap

2. **You have concurrent workers** ✓
   - Lock-free writes
   - No contention
   - True parallelism

3. **You had lock issues before** ✓
   - mmap eliminates lock contention
   - Atomic operations for coordination

4. **You had correlation issues** ✓
   - Direct memory access simplifies mapping
   - No index tracking needed

5. **Memory pressure concerns** ✓
   - OS manages paging
   - Only hot pages in RAM
   - Automatic eviction under pressure

### Implementation Checklist

- [ ] Use one mmap'd progress file per transfer
- [ ] Abstract platform-specific code with build tags
- [ ] Use atomic operations for header fields
- [ ] Background async sync every 5 seconds
- [ ] Sync synchronously on close
- [ ] Add fallback to regular I/O for NFS detection
- [ ] Keep progress file size small (<1MB)

---

## Code Example: Integration

```go
type Transfer struct {
    info         TransferInfo
    destFile     *RandomAccessFileWriter
    progressFile *ChunkProgressFile  // mmap'd
}

// Worker downloading chunk
func (t *Transfer) downloadChunk(idx uint32) error {
    // Download chunk data
    data, err := downloadChunkData(idx)
    if err != nil {
        return err
    }

    // Write to destination file
    if err := t.destFile.WriteChunk(idx, data); err != nil {
        return err
    }

    // Update progress (no lock, no blocking I/O!)
    md5 := computeMD5(data)
    return t.progressFile.MarkChunkComplete(idx, md5)
}

// On resume
func (t *Transfer) resume() error {
    // Fast scan of mmap'd progress file
    pending := t.progressFile.GetPendingChunks()

    // Schedule only pending chunks
    for _, idx := range pending {
        scheduleChunk(idx)
    }

    return nil
}
```

**This is much cleaner than your previous implementation!**

---

## Conclusion

Given your specific requirements and past experience:
- **Per-transfer progress files** → mmap is perfect
- **Concurrent workers** → mmap shines
- **Lock contention issues** → mmap eliminates them
- **Correlation complexity** → mmap simplifies
- **Memory pressure** → mmap helps (OS-managed)

**Use mmap. It solves all your problems.**
