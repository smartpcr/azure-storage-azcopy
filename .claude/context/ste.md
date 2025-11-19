# Storage Transfer Engine (STE) - Comprehensive Documentation

**Last Updated**: 2025-11-18
**Component**: ste/
**Total Files**: 108 Go files

## Overview

The Storage Transfer Engine (STE) is the core execution engine of AzCopy responsible for managing and executing all data transfer operations. It handles job orchestration, parallel chunk transfers, resource management, and ensures reliability through retry logic and resume capabilities.

## Architecture

### High-Level Design Principles

1. **Job-Based Architecture**: All transfers are organized into jobs, which are subdivided into parts and transfers
2. **Chunk-Level Parallelism**: Files are split into chunks for parallel transfer
3. **Memory-Mapped Job Plans**: Job state is persisted using memory-mapped files for efficient resume
4. **Abstraction Layers**: Separate interfaces for downloaders, uploaders, and service-to-service copiers
5. **Platform-Specific Implementations**: Different file handlers for Linux/Windows/macOS

### Core Components Hierarchy

```
JobMgr (Job Manager)
  └── JobPartMgr[] (Job Part Managers)
       └── JobPartTransferMgr[] (Transfer Managers)
            ├── Downloader (for downloads)
            ├── Uploader (for uploads)
            └── S2SCopier (for service-to-service copies)
```

## Job Management System

### 1. JobPartPlanHeader (`JobPartPlan.go`)

**Purpose**: Defines the structure of job plan stored in memory-mapped files

**Key Fields**:
- `Version`: Data schema version (currently v19)
- `JobID`, `PartNum`: Unique identifiers
- `SourceRoot`, `DestinationRoot`: Transfer endpoints
- `NumTransfers`: Number of transfers in this part
- `FromTo`: Source and destination types (e.g., LocalBlob, BlobLocal)
- `atomicJobStatus`: Thread-safe status tracking

**Features**:
- Memory-mapped for efficient access and persistence
- Enables resume capability after interruption
- Stores per-transfer metadata (headers, properties, tags)
- Platform-independent binary format

**File Location**: `ste/JobPartPlan.go:1`

---

### 2. JobMgr - Job Manager (`mgr-JobMgr.go`)

**Purpose**: Top-level job orchestrator managing all job parts

**Key Responsibilities**:
- Create and manage job parts
- Schedule transfers across goroutine pools
- Track overall job progress
- Manage HTTP client connections
- Handle job pause/resume/cancel operations
- Report job status and statistics

**Key Methods**:
- `AddJobPart()`: Add a new part to the job
- `ScheduleTransfer()`: Queue a transfer for execution
- `ScheduleChunk()`: Queue a chunk operation
- `CancelPauseJobOrder()`: Cancel or pause job
- `ResumeTransfers()`: Resume interrupted job

**Concurrency Management**:
- Main goroutine pool for chunk execution
- Configurable pool sizes via environment variables
- Dynamic pool resizing based on workload
- Connection limiting to prevent socket exhaustion

**File Location**: `ste/mgr-JobMgr.go:1`

---

### 3. JobPartMgr - Job Part Manager (`mgr-JobPartMgr.go`)

**Purpose**: Manages a single part of a job (subset of transfers)

**Key Responsibilities**:
- Read job part plan from memory-mapped file
- Create transfer managers for each transfer
- Schedule chunks for parallel execution
- Manage service clients (Azure SDK clients)
- Handle overwrite prompts
- Track folder creation
- Manage security info persistence

**Key Methods**:
- `ScheduleTransfers()`: Start all transfers in this part
- `StartJobXfer()`: Begin a single transfer
- `ReportTransferDone()`: Mark transfer as complete
- `SrcServiceClient()`, `DstServiceClient()`: Get Azure SDK clients

**Resource Management**:
- ByteSlicePooler: Reusable byte buffers
- CacheLimiter: Control concurrent operations
- ExclusiveDestinationMap: Prevent concurrent writes to same destination

**HTTP Client Configuration**:
```go
MaxConnsPerHost: concurrentDialsPerCpu * runtime.NumCPU()
MaxIdleConns: 0 (unlimited)
MaxIdleConnsPerHost: maxIdleConns
IdleConnTimeout: 180s
DisableCompression: true (download gzipped files as-is)
```

**File Location**: `ste/mgr-JobPartMgr.go:1`

---

### 4. JobPartTransferMgr - Transfer Manager (`mgr-JobPartTransferMgr.go`)

**Purpose**: Manages a single file/blob transfer within a job part

**Key Responsibilities**:
- Coordinate chunk-level transfers
- Track transfer status and progress
- Handle errors and retries
- Manage destination locking (prevent concurrent writes)
- Report chunk completion
- Execute epilogue actions after transfer completes

**Transfer States**:
- Pending → In Progress → Completed/Failed
- Chunk-level tracking for resume capability
- Atomic status updates for thread safety

**Key Methods**:
- `ReportChunkDone()`: Mark chunk as complete
- `SetStatus()`: Update transfer status
- `ScheduleChunks()`: Queue chunks for execution
- `FailActiveUpload/Download/S2SCopy()`: Handle transfer failures
- `WaitUntilLockDestination()`: Acquire exclusive write lock

**TransferInfo Structure**:
```go
type TransferInfo struct {
    JobID          common.JobID
    BlockSize      int64
    Source         string
    SourceSize     int64
    Destination    string
    EntityType     common.EntityType  // File, Folder, Symlink
    SrcContainer   string
    DstContainer   string
    SrcFilePath    string
    DstFilePath    string
    SrcBlobType    blob.BlobType
    // ... properties, metadata, permissions
}
```

**File Location**: `ste/mgr-JobPartTransferMgr.go:1`

## Transfer Abstractions

### Downloader Interface (`downloader.go`)

**Purpose**: Abstract interface for downloading from remote sources to local

**Key Methods**:
- `Prologue()`: Initialize before first chunk
- `GenerateDownloadFunc()`: Create function to download a chunk
- `Epilogue()`: Cleanup after all chunks complete

**Specialized Interfaces**:
- `creationTimeDownloader`: Custom file creation (Linux special files)
- `unixPropertyAwareDownloader`: Apply UNIX properties
- `folderDownloader`: Process folder properties
- `symlinkDownloader`: Handle symbolic links
- `smbPropertyAwareDownloader`: Windows SMB properties
- `smbACLAwareDownloader`: Windows security descriptors
- `nfsPropertyAwareDownloader`: NFS properties
- `nfsPermissionsAwareDownloader`: NFS permissions

**Implementations**:
- `blobDownloader`: Azure Blob Storage downloads
- `blobFSDownloader`: Azure Data Lake Gen2 downloads
- `azureFilesDownloader`: Azure Files downloads
- `httpDownloader`: Generic HTTP downloads

**Chunk Processing**:
```go
GenerateDownloadFunc(
    jptm IJobPartTransferMgr,
    writer common.ChunkedFileWriter,  // Writes to local file
    id common.ChunkID,                // Chunk identifier
    length int64,                     // Chunk size
    pacer pacer                       // Rate limiter
) chunkFunc
```

**File Location**: `ste/downloader.go:1`

---

### Sender Interface (`sender.go`)

**Purpose**: Abstract interface for sending data (uploads and S2S copies)

**Key Methods**:
- `ChunkSize()`: Get chunk size for this transfer
- `NumChunks()`: Get total number of chunks
- `RemoteFileExists()`: Check if destination exists
- `Prologue()`: Initialize remote file/blob
- `Epilogue()`: Finalize transfer (commit blocks, set properties)
- `Cleanup()`: Post-transfer cleanup
- `GetDestinationLength()`: Get current remote file size

**Specialized Interfaces**:
- `propertiesSender`: Copy metadata/tags/tier without data
- `folderSender`: Create and set folder properties
- `symlinkSender`: Send symbolic link information

**Uploader Interface** (extends sender):
```go
type uploader interface {
    sender
    GenerateUploadFunc(
        chunkID common.ChunkID,
        blockIndex int32,
        reader common.SingleChunkReader,  // Reads from local file
        chunkIsWholeFile bool
    ) chunkFunc
    Md5Channel() chan<- []byte  // For MD5 hash computation
}
```

**S2S Copier Interface** (extends sender):
```go
type s2sCopier interface {
    sender
    GenerateCopyFunc(
        chunkID common.ChunkID,
        blockIndex int32,
        adjustedChunkSize int64,
        chunkIsWholeFile bool
    ) chunkFunc
}
```

**File Location**: `ste/sender.go:1`

## Source Info Providers

### Purpose
Source Info Providers abstract access to source file/blob properties and data

### Interface Hierarchy (`sourceInfoProvider.go`)

**ISourceInfoProvider** (base interface):
- `Properties()`: Get HTTP headers, metadata, tags
- `GetFreshFileLastModifiedTime()`: Get current LMT
- `IsLocal()`: Check if source is local filesystem
- `EntityType()`: File, Folder, Symlink
- `GetMD5()`: Compute MD5 hash for range

**ILocalSourceInfoProvider**:
- `OpenSourceFile()`: Open local file for reading

**IRemoteSourceInfoProvider**:
- `PreSignedSourceURL()`: Get source URL with SAS
- `SourceSize()`: Get file size
- `RawSource()`: Get original source string

**IBlobSourceInfoProvider**:
- `BlobTier()`: Get blob access tier
- `BlobType()`: Block, Page, or Append blob

**Platform-Specific Providers**:
- `ISMBPropertyBearingSourceInfoProvider`: Windows SMB properties and ACLs
- `INFSPropertyBearingSourceInfoProvider`: NFS properties and permissions
- `IUNIXPropertyBearingSourceInfoProvider`: UNIX stat properties

### Implementations

| Provider | Source Type | File |
|----------|-------------|------|
| `localSourceInfoProvider` | Local filesystem | `sourceInfoProvider-Local.go` |
| `blobSourceInfoProvider` | Azure Blob | `sourceInfoProvider-Blob.go` |
| `fileSourceInfoProvider` | Azure Files | `sourceInfoProvider-File.go` |
| `s3SourceInfoProvider` | AWS S3 | `sourceInfoProvider-S3.go` |
| `gcpSourceInfoProvider` | Google Cloud | `sourceInfoProvider-GCP.go` |
| `benchmarkSourceInfoProvider` | Benchmark mode | `sourceInfoProvider-Benchmark.go` |

**File Location**: `ste/sourceInfoProvider.go:1`

## Downloader Implementations

### 1. Blob Downloader (`downloader-blob.go`)

**Source**: Azure Blob Storage (all blob types)

**Special Features**:
- **Page Blob Optimization**: Uses `pageRangeOptimizer` to skip empty ranges
- **Per-Blob Pacer**: Auto-tunes download rate for premium page blobs
- **Version/Snapshot Support**: Can download specific versions or snapshots

**Prologue Behavior**:
```go
- Initialize transfer info and context
- For page blobs:
  * Create page blob auto-pacer
  * Fetch page ranges to identify data regions
  * Skip downloading zero-filled ranges
```

**Chunk Download**:
```go
1. Check if page range contains data (page blobs only)
2. If empty range, enqueue empty chunk (no download)
3. Otherwise, download chunk from blob
4. Apply rate limiting via pacer
5. Enqueue chunk data to file writer
```

**Platform-Specific Files**:
- `downloader-blob_linux.go`: Linux-specific POSIX property handling
- `downloader-blob_other.go`: Stub for non-Linux platforms

**File Location**: `ste/downloader-blob.go:1`

---

### 2. BlobFS Downloader (`downloader-blobFS.go`)

**Source**: Azure Data Lake Storage Gen2 (hierarchical namespace)

**Differences from Blob**:
- Uses Data Lake SDK instead of Blob SDK
- Supports POSIX permissions and ACLs
- Handles hierarchical namespace properties

**Platform Files**:
- `downloader-blobFS_linux.go`: POSIX properties
- `downloader-blobFS_other.go`: Stub

**File Location**: `ste/downloader-blobFS.go:1`

---

### 3. Azure Files Downloader (`downloader-azureFiles.go`)

**Source**: Azure Files (SMB file shares)

**Features**:
- SMB property preservation (creation time, attributes)
- Windows ACL/SDDL support
- POSIX property support on Linux

**Platform-Specific Files**:
- `downloader-azureFiles_linux.go`: POSIX properties, NFSv3 support
- `downloader-azureFiles_windows.go`: SMB properties, Windows ACLs

**File Location**: `ste/downloader-azureFiles.go:1`

---

### 4. HTTP Downloader (`downloader-http.go`)

**Source**: Generic HTTP/HTTPS URLs

**Features**:
- Download from any HTTP endpoint
- Content-Type and Last-Modified preservation
- Resume support via Range headers
- Auto-scaling for improved throughput

**Use Cases**:
- Download from web servers
- Import from public URLs
- Testing and benchmarking

**Testing**: Includes comprehensive unit tests in `downloader-http_test.go`

**File Location**: `ste/downloader-http.go:1`

## Sender Implementations

### Block Blob Uploader (`sender-blockBlob.go`)

**Destination**: Azure Blob Storage (Block Blobs)

**Architecture**:
```
blockBlobSenderBase
  ├── blockBlobUploader (from local)
  └── blockBlobS2SCopier (from URL)
```

**Key Features**:

1. **Memory Management**:
   - Validates chunk size against available memory
   - Prevents OOM by checking `AZCOPY_BUFFER_GB`
   - Supports PutBlob for small files (<= 256MB)
   - Uses PutBlockList for large files

2. **Block ID Generation**:
   ```go
   blockNamePrefix = base64(UUID)
   blockID = prefix + base64(blockIndex)
   ```

3. **Chunk Upload Strategy**:
   ```go
   if srcSize <= putBlobSize:
       Upload entire file with PutBlob (single operation)
   else:
       Upload chunks with PutBlock
       Finalize with PutBlockList
   ```

4. **Metadata & Tags**:
   - Apply headers, metadata in final commit
   - Tags in header if <2KB, separate SetTags if larger
   - Blob tier set after commit

5. **Epilogue Operations**:
   ```go
   1. Commit block list (PutBlockList)
   2. Set blob tier (if specified)
   3. Set blob tags (if >2KB or couldn't set in header)
   4. Report transfer complete
   ```

**Related Files**:
- `sender-blockBlobFromLocal.go`: Upload from local file
- `sender-blockBlobFromURL.go`: Copy from URL (S2S)
- `sender_blockBlob_test.go`: Unit tests

**File Location**: `ste/sender-blockBlob.go:1`

---

### Page Blob Sender (`sender-pageBlob.go`)

**Destination**: Azure Page Blobs (VHD files, disks)

**Features**:
- 512-byte alignment requirement
- Sparse file optimization (skip zero ranges)
- Auto-pacer for premium storage
- Maximum blob size: 8TB

**Related Files**:
- `sender-pageBlobFromLocal.go`
- `sender-pageBlobFromURL.go`

---

### Append Blob Sender (`sender-appendBlob.go`)

**Destination**: Azure Append Blobs (logs, audit trails)

**Features**:
- Append-only semantics
- Maximum blob size: 195GB
- Maximum append size per operation: 4MB
- Conditional append support

**Related Files**:
- `sender-appendBlobFromLocal.go`
- `sender-appendBlobFromURL.go`
- `sender-appendBlob_test.go`

---

### Azure Files Sender (`sender-azureFile.go`)

**Destination**: Azure Files (SMB shares)

**Features**:
- SMB property preservation
- Range-based uploads
- File attributes and timestamps
- ACL/SDDL support (Windows)

**Helper**: `sender-azureFile-helper.go` contains shared utilities

**Related Files**:
- `sender-azureFileFromLocal.go`
- `sender-azureFileFromURL.go`

---

### BlobFS Sender (`sender-blobFS.go`)

**Destination**: Azure Data Lake Gen2

**Features**:
- Hierarchical namespace operations
- POSIX permissions and ACLs
- Efficient directory operations
- Flush-based commit model

**Related Files**:
- `sender-blobFSFromLocal.go`

---

### Folder & Symlink Senders

**Folder Senders**:
- `sender-blobFolders.go`: Create blob folders (0-byte blobs)
- Platform files: `sender-blobFolders_linux.go`, `sender-blobFolders_other.go`

**Symlink Senders**:
- `sender-blobSymlinks.go`: Store symlink as blob metadata
- Platform files: `sender-blobSymlinks_linux.go`, `sender-blobSymlinks_other.go`

## Transfer Coordination (xfer-*.go files)

### Purpose
The `xfer-*.go` files coordinate the overall transfer flow, connecting downloaders/uploaders to the job part transfer manager.

### Key Files

**1. anyToRemote-file.go** (`xfer-anyToRemote-file.go`)
- **Purpose**: Upload files from any source (local, blob, S3, etc.) to remote
- **Flow**:
  1. Validate blob tier compatibility
  2. Create source info provider
  3. Open local file or prepare remote source
  4. Compute MD5 if required
  5. Create sender (uploader or S2S copier)
  6. Schedule chunks for transfer
  7. Set properties and finalize

**2. remoteToLocal-file.go** (`xfer-remoteToLocal-file.go`)
- **Purpose**: Download files from remote to local
- **Flow**:
  1. Create downloader for source type
  2. Run downloader prologue
  3. Create local file with proper attributes
  4. Schedule chunk downloads
  5. Write chunks to file
  6. Apply properties (timestamps, permissions, ACLs)
  7. Run downloader epilogue

**3. anyToRemote-folder.go** (`xfer-anyToRemote-folder.go`)
- **Purpose**: Create folders at destination
- **Flow**:
  1. Check if folder exists
  2. Create folder if needed
  3. Set folder properties (metadata, permissions)

**4. remoteToLocal-folder.go** (`xfer-remoteToLocal-folder.go`)
- **Purpose**: Create local directories
- **Flow**:
  1. Create directory structure
  2. Apply folder properties
  3. Set timestamps and permissions

**5. anyToRemote-symlink.go** (`xfer-anyToRemote-symlink.go`)
- **Purpose**: Transfer symbolic links
- **Platform**: Linux only
- **Flow**:
  1. Read symlink target
  2. Send symlink metadata to destination

**6. remoteToLocal-symlink.go** (`xfer-remoteToLocal-symlink.go`)
- **Purpose**: Create local symbolic links
- **Platform**: Linux only

**7. deleteBlob.go, deleteBlobFS.go, deleteFile.go** (`xfer-delete*.go`)
- **Purpose**: Delete operations for sync/remove commands

**8. setProperties.go** (`xfer-setProperties.go`)
- **Purpose**: Set blob properties without copying data
- **Use Case**: Metadata-only updates, tier changes

**9. anyToRemote-fileProperties.go** (`xfer-anyToRemote-fileProperties.go`)
- **Purpose**: Copy only properties/metadata without data

## Performance & Concurrency

### Concurrency Settings (`concurrency.go`)

**Purpose**: Configure goroutine pool sizes and parallelism

**Key Settings**:
```go
type ConcurrencySettings struct {
    InitialMainPoolSize int           // Starting pool size
    MaxMainPoolSize     *ConfiguredInt // Maximum pool size
    TransferInitiationPoolSize ConfiguredInt
    FileAndChunkBased   struct {
        MaxFilesAndChunks int           // Total parallelism
        ChunkMultiplier   ConfiguredInt // Chunks per file
    }
}
```

**Environment Variables**:
- `AZCOPY_CONCURRENT_FILES`: Max concurrent files
- `AZCOPY_CONCURRENT_SCAN`: Max concurrent enumeration
- `AZCOPY_CONCURRENCY_VALUE`: Total parallelism
- `AZCOPY_CHUNK_MULTIPLIER`: Chunks per file

**Default Behavior**:
- Based on CPU count: `runtime.NumCPU()`
- Scaled for different transfer types
- Dynamic tuning via `concurrencyTuner.go`

**File Location**: `ste/concurrency.go:1`

---

### Concurrency Tuner (`concurrencyTuner.go`)

**Purpose**: Dynamically adjust concurrency based on throughput

**Strategy**:
1. Monitor throughput every 10 seconds
2. If throughput low, reduce concurrency
3. If throughput high, increase concurrency
4. Prevent thrashing with hysteresis

**Tuning Algorithm**:
```go
if throughput < targetThroughput * 0.5:
    Reduce concurrency by 20%
else if throughput > targetThroughput * 1.2:
    Increase concurrency by 10%
```

**File Location**: `ste/concurrencyTuner.go:1`
**Tests**: `zt_concurrencyTuner_test.go`

---

### Pacers (Rate Limiting)

**Purpose**: Control transfer rate to prevent throttling and optimize throughput

**Implementations**:

1. **autoPacer** (`pacer-autoPacer.go`)
   - Automatically adjusts rate based on service responses
   - Increases rate on success, decreases on throttling
   - Used for page blobs in premium storage

2. **tokenBucketPacer** (`pacer-tokenBucketPacer.go`)
   - Fixed-rate limiting using token bucket algorithm
   - Smooths burst traffic
   - Configurable via `--cap-mbps` flag

3. **nullAutoPacer** (`pacer-nullAutoPacer.go`)
   - No-op pacer for unlimited rate
   - Used when no rate limiting needed

**Usage Pattern**:
```go
filePacer := newPageBlobAutoPacer(initialBytesPerSecond, blockSize, ...)
filePacer.RequestRightToSend(ctx, bytesToSend)
// ... perform transfer ...
filePacer.ReportSuccess(bytesSent)
```

**Page Range Optimizer** (`pageRangeOptimizer.go`):
- Fetches page ranges before download
- Skips empty (zero-filled) ranges
- Reduces data transfer for sparse page blobs
- **Tests**: `pageRangeOptimizer_test.go`

---

### Performance Advisor (`performanceAdvisor.go`)

**Purpose**: Detect performance bottlenecks and provide recommendations

**Monitored Metrics**:
- Disk I/O speed
- Network throughput
- CPU utilization
- Memory pressure

**Advice Examples**:
- "Disk I/O is slow, consider using faster storage"
- "Network bandwidth is not saturated, increase concurrency"
- "High CPU usage detected, reduce chunk count"

**File Location**: `ste/performanceAdvisor.go:1`
**Tests**: `zt_performanceAdvisor_test.go`

## Retry & Error Handling

### Retry Helper (`xferRetryHelper.go`)

**Purpose**: Implement exponential backoff retry logic

**Retry Strategy**:
```go
maxRetries = 5
initialDelay = 1 second
maxDelay = 60 seconds
backoffMultiplier = 2
jitter = random(0, 0.5 * delay)
```

**Retryable Errors**:
- Network timeouts
- HTTP 500, 502, 503, 504
- Connection errors
- Throttling (429)

**Non-Retryable Errors**:
- HTTP 403 (Forbidden)
- HTTP 404 (Not Found)
- Invalid credentials
- File not found

**Platform-Specific Files**:
- `xferRetryHelper_unix.go`: Unix-specific error handling
- `xferRetryHelper_windows.go`: Windows-specific error handling
- **Tests**: `xferRetryHelper_test.go`

**File Location**: `ste/xferRetryHelper.go:1`

---

### Retry Notification Policy (`xferRetryNotificationPolicy.go`)

**Purpose**: Azure SDK pipeline policy to log retry attempts

**Features**:
- Logs each retry with reason
- Tracks retry count per request
- Integrates with AzCopy logging

**File Location**: `ste/xferRetryNotificationPolicy.go:1`
**Tests**: `zt_xferRetryNotificationPolicy_test.go`

---

### Error Extensions (`ErrorExt.go`)

**Purpose**: Extend standard errors with transfer context

**Features**:
- Attach source/destination URLs
- Include transfer index
- Preserve stack trace
- Categorize error types

## Azure SDK Integration

### Pipeline Policies

AzCopy extends Azure SDK pipeline with custom policies:

1. **xferLogPolicy.go**
   - Logs all HTTP requests/responses
   - Redacts sensitive information (SAS tokens)
   - Configurable log levels

2. **xferVersionPolicy.go**
   - Adds AzCopy version to User-Agent header
   - Tracks client version for telemetry

3. **xferStatsPolicy.go**
   - Collects transfer statistics
   - Tracks bytes transferred
   - Measures latency

4. **sourceAuthPolicy.go**
   - Handles source authentication for S2S
   - Refreshes tokens
   - **Tests**: `srcAuthPolicy_test.go`

5. **destReauthPolicy.go**
   - Re-authenticates on destination token expiry
   - **Tests**: `zt_destReauthPolicy_test.go`

6. **request_priority_policy.go**
   - Sets request priority headers
   - **Tests**: `request_priority_policy_test.go`

### Network Stats (`PipelineNetworkStats`)

Located in job manager, tracks:
- Total bytes sent/received
- Request count
- Average latency
- Error rate

## Helper Components

### Folder Creation Tracker (`folderCreationTracker.go`)

**Purpose**: Ensure parent folders are created before files

**Strategy**:
- Track created folders in-memory
- Thread-safe via mutex
- Create parent chain recursively
- Deduplicate creation attempts

**File Location**: `ste/folderCreationTracker.go:1`
**Tests**: `folderCreationTracker_test.go`

---

### Security Info Persistence Manager (`securityInfoPersistenceManager.go`)

**Purpose**: Store and retrieve Windows ACLs for later application

**Use Case**:
- Download blob with ACL metadata
- Store ACL temporarily
- Apply ACL to local file after download completes

**Platform**: Windows only

---

### MD5 Comparer (`md5Comparer.go`)

**Purpose**: Validate data integrity using MD5 hashes

**Strategy**:
- Compute MD5 during upload/download
- Compare with source/destination MD5
- Fail transfer on mismatch

---

### Overwrite Prompter (`overwritePrompter.go`)

**Purpose**: Prompt user when overwrite decision needed

**Modes**:
- Auto-overwrite (force)
- Auto-skip (if newer)
- Prompt user (interactive)
- Fail (never overwrite)

---

### Folder Deletion Manager

**Purpose**: Clean up empty folders after sync operations

**Strategy**:
- Track folders during sync
- Delete empty folders after file deletions
- Respect folder retention policies

## File Reference Guide

### Job Management
| File | Purpose | Key Types |
|------|---------|-----------|
| `JobPartPlan.go` | Memory-mapped job plan structure | `JobPartPlanHeader` |
| `JobPartPlanFileName.go` | Job plan file naming | File path utilities |
| `mgr-JobMgr.go` | Top-level job orchestrator | `IJobMgr`, `jobMgr` |
| `mgr-JobPartMgr.go` | Part-level manager | `IJobPartMgr`, `jobPartMgr` |
| `mgr-JobPartTransferMgr.go` | Transfer-level manager | `IJobPartTransferMgr`, `TransferInfo` |
| `jobStatusManager.go` | Track job status | Status tracking |

### Transfer Abstractions
| File | Purpose | Key Interfaces |
|------|---------|----------------|
| `downloader.go` | Downloader interface | `downloader`, specialized downloaders |
| `sender.go` | Sender interface | `sender`, `uploader`, `s2sCopier` |
| `sourceInfoProvider.go` | Source metadata provider | `ISourceInfoProvider` hierarchy |

### Downloaders
| File | Source Type | Platform Notes |
|------|-------------|----------------|
| `downloader-blob.go` | Azure Blob | All platforms |
| `downloader-blob_linux.go` | Azure Blob | Linux POSIX properties |
| `downloader-blobFS.go` | ADLS Gen2 | All platforms |
| `downloader-blobFS_linux.go` | ADLS Gen2 | Linux POSIX |
| `downloader-azureFiles.go` | Azure Files | All platforms |
| `downloader-azureFiles_linux.go` | Azure Files | Linux NFS |
| `downloader-azureFiles_windows.go` | Azure Files | Windows SMB |
| `downloader-http.go` | HTTP/HTTPS | All platforms |

### Senders (Uploaders)
| File | Destination Type | Notes |
|------|------------------|-------|
| `sender-blockBlob.go` | Block Blob | Base implementation |
| `sender-blockBlobFromLocal.go` | Block Blob | Upload from local |
| `sender-blockBlobFromURL.go` | Block Blob | S2S copy |
| `sender-pageBlob.go` | Page Blob | VHD files |
| `sender-pageBlobFromLocal.go` | Page Blob | Upload |
| `sender-pageBlobFromURL.go` | Page Blob | S2S copy |
| `sender-appendBlob.go` | Append Blob | Logs, audit trails |
| `sender-appendBlobFromLocal.go` | Append Blob | Upload |
| `sender-appendBlobFromURL.go` | Append Blob | S2S copy |
| `sender-azureFile.go` | Azure Files | Base |
| `sender-azureFileFromLocal.go` | Azure Files | Upload |
| `sender-azureFileFromURL.go` | Azure Files | S2S copy |
| `sender-blobFS.go` | ADLS Gen2 | Base |
| `sender-blobFSFromLocal.go` | ADLS Gen2 | Upload |
| `sender-blobFolders.go` | Blob folders | Folder markers |
| `sender-blobSymlinks.go` | Blob symlinks | Linux symlinks |

### Source Info Providers
| File | Source Type |
|------|-------------|
| `sourceInfoProvider-Local.go` | Local filesystem (all platforms) |
| `sourceInfoProvider-Local_linux.go` | Local (Linux POSIX) |
| `sourceInfoProvider-Local_windows.go` | Local (Windows) |
| `sourceInfoProvider-Blob.go` | Azure Blob |
| `sourceInfoProvider-File.go` | Azure Files |
| `sourceInfoProvider-S3.go` | AWS S3 |
| `sourceInfoProvider-GCP.go` | Google Cloud Storage |
| `sourceInfoProvider-Benchmark.go` | Benchmarking /dev/null |

### Transfer Coordination
| File | Purpose |
|------|---------|
| `xfer-anyToRemote-file.go` | Upload files |
| `xfer-anyToRemote-folder.go` | Create remote folders |
| `xfer-anyToRemote-symlink.go` | Upload symlinks |
| `xfer-anyToRemote-fileProperties.go` | Copy properties only |
| `xfer-remoteToLocal-file.go` | Download files |
| `xfer-remoteToLocal-folder.go` | Create local folders |
| `xfer-remoteToLocal-symlink.go` | Download symlinks |
| `xfer-deleteBlob.go` | Delete blobs |
| `xfer-deleteBlobFS.go` | Delete ADLS Gen2 files |
| `xfer-deleteFile.go` | Delete Azure Files |
| `xfer-setProperties.go` | Set properties |

### Performance & Concurrency
| File | Purpose |
|------|---------|
| `concurrency.go` | Concurrency settings |
| `concurrencyTuner.go` | Dynamic tuning |
| `pacer-autoPacer.go` | Auto rate limiting |
| `pacer-tokenBucketPacer.go` | Fixed rate limiting |
| `pacer-nullAutoPacer.go` | No rate limiting |
| `pageRangeOptimizer.go` | Skip empty page ranges |
| `performanceAdvisor.go` | Bottleneck detection |

### Retry & Error Handling
| File | Purpose |
|------|---------|
| `xferRetryHelper.go` | Retry logic (all platforms) |
| `xferRetryHelper_unix.go` | Unix error handling |
| `xferRetryHelper_windows.go` | Windows error handling |
| `ErrorExt.go` | Extended error types |

### Azure SDK Integration
| File | Purpose |
|------|---------|
| `xferLogPolicy.go` | Request/response logging |
| `xferVersionPolicy.go` | User-Agent tracking |
| `xferStatsPolicy.go` | Statistics collection |
| `sourceAuthPolicy.go` | Source authentication |
| `destReauthPolicy.go` | Destination re-auth |
| `request_priority_policy.go` | Request priority |

### Helpers
| File | Purpose |
|------|---------|
| `folderCreationTracker.go` | Track folder creation |
| `securityInfoPersistenceManager.go` | Store ACLs temporarily |
| `md5Comparer.go` | MD5 validation |
| `overwritePrompter.go` | User prompts |
| `fileAttributesHelper.go` | File attribute utilities |
| `emptyCloseableReaderAt.go` | Empty reader implementation |
| `pacedReadSeeker.go` | Rate-limited reader |
| `putListNeed.go` | Block list utilities |
| `remoteObjectExists.go` | Check existence |
| `ste-pathUtils.go` | Path manipulation |

### S2S Specific
| File | Purpose |
|------|---------|
| `s2sCopier-URLToBlob.go` | URL-to-Blob copy |

### Tests
All test files follow naming pattern: `zt_*.go` or `*_test.go`

## Key Patterns & Conventions

### 1. Chunk Function Pattern

All chunk operations follow this pattern:

```go
type chunkFunc func(workerId int)

func createChunkFunc(setDoneStatusOnExit bool, jptm IJobPartTransferMgr,
                     id common.ChunkID, body func()) chunkFunc {
    return func(workerId int) {
        defer jptm.ReportChunkDone(id)
        defer jptm.OccupyAConnection()
        defer jptm.ReleaseAConnection()

        if jptm.WasCanceled() {
            jptm.LogChunkStatus(id, common.EWaitReason.Cancelled())
            return
        }

        jptm.SetDestinationIsModified()
        body()  // Actual work happens here
    }
}
```

### 2. Prologue-Transfer-Epilogue Pattern

All transfers follow this sequence:

```go
// Prologue: Initialize transfer
downloader.Prologue(jptm)

// Transfer: Execute chunks in parallel
for each chunk:
    chunkFunc := downloader.GenerateDownloadFunc(...)
    jptm.ScheduleChunks(chunkFunc)

// Epilogue: Finalize transfer
downloader.Epilogue()
```

### 3. Interface-Based Abstraction

STE uses interfaces extensively for abstraction:
- `IJobMgr`, `IJobPartMgr`, `IJobPartTransferMgr`
- `downloader`, `uploader`, `s2sCopier`
- `ISourceInfoProvider` hierarchy
- `pacer` interface

This enables:
- Easy testing with mocks
- Platform-specific implementations
- Extensibility for new storage types

### 4. Platform-Specific Build Tags

Files use build tags for platform-specific code:
```go
//go:build linux
// or
//go:build windows
```

Common patterns:
- `*_linux.go`: Linux-specific (POSIX, symlinks)
- `*_windows.go`: Windows-specific (SMB, ACLs)
- `*_other.go`: Stub for unsupported platforms

### 5. Thread-Safe State Management

Job state uses atomic operations:
```go
atomic.LoadInt32(&jptm.atomicJobStatus)
atomic.StoreInt32(&jptm.atomicJobStatus, newStatus)
atomic.AddInt32(&jptm.atomicChunksWritten, 1)
```

### 6. Resource Pooling

Reusable resources to reduce allocations:
```go
- ByteSlicePooler: Byte buffer pools
- HTTP connection pools
- Goroutine pools
```

### 7. Error Categorization

Errors are categorized for appropriate handling:
```go
- Retryable: Network errors, throttling
- Non-retryable: Auth failures, not found
- Terminal: Invalid configuration
```

## Transfer Flow Example

### Scenario: Upload local file to Azure Blob

1. **Job Creation** (`cmd/copy.go`)
   ```
   User runs: azcopy copy /local/file.txt https://account.blob.core.windows.net/container/
   → Create JobPartPlan with transfer details
   → Persist to memory-mapped file
   ```

2. **Job Manager Initialization** (`mgr-JobMgr.go`)
   ```
   → Create JobMgr with concurrency settings
   → Initialize HTTP client
   → Create goroutine pools
   ```

3. **Job Part Manager** (`mgr-JobPartMgr.go`)
   ```
   → Load JobPartPlan from MMF
   → Create service clients
   → Initialize resource pools
   ```

4. **Transfer Manager** (`mgr-JobPartTransferMgr.go`)
   ```
   → Create TransferInfo from job plan
   → Determine chunk count and size
   → Lock destination to prevent concurrent writes
   ```

5. **Source Info Provider** (`sourceInfoProvider-Local.go`)
   ```
   → Open local file
   → Read file metadata (size, timestamps, permissions)
   → Compute MD5 if required
   ```

6. **Sender Creation** (`sender.go → sender-blockBlob.go`)
   ```
   → Determine blob type (block/page/append)
   → Create blockBlobUploader
   → Validate chunk size against memory limits
   → Generate block IDs
   ```

7. **Prologue** (`sender-blockBlob.go`)
   ```
   → Check if destination blob exists
   → Prepare headers, metadata, tags
   → No remote file creation (blocks uploaded first)
   ```

8. **Chunk Scheduling** (`xfer-anyToRemote-file.go`)
   ```
   → For each chunk:
     - Create SingleChunkReader for file range
     - Generate upload function
     - Queue chunkFunc to goroutine pool
   ```

9. **Chunk Upload** (`sender-blockBlob.go`)
   ```
   → Worker goroutine picks up chunk:
     - Read chunk from local file
     - Compute MD5 for chunk
     - Upload via PutBlock API
     - Report chunk done
   ```

10. **Epilogue** (`sender-blockBlob.go`)
    ```
    → Wait for all chunks to complete
    → Commit blocks with PutBlockList
    → Set blob tier (if specified)
    → Set blob tags (if >2KB)
    → Set metadata and headers
    ```

11. **Completion** (`mgr-JobPartTransferMgr.go`)
    ```
    → Report transfer done
    → Update job part plan status
    → Log completion
    → Release resources
    ```

### Data Flow Diagram

```
Local File
    ↓ (read chunks)
SingleChunkReader
    ↓ (read data)
Upload ChunkFunc
    ↓ (PutBlock)
Azure Blob Storage (uncommitted blocks)
    ↓ (after all chunks)
PutBlockList
    ↓
Committed Block Blob
    ↓
Set Tier/Tags/Metadata
    ↓
Final Blob
```

## Memory Management

### Buffer Pooling

**ByteSlicePooler**: Reusable byte slices for chunk data
```go
pool.GetBuffer(chunkSize) → []byte
// ... use buffer ...
pool.ReleaseBuffer(buffer)
```

**Benefits**:
- Reduce GC pressure
- Prevent memory fragmentation
- Control memory usage

### Memory Limits

Controlled via `AZCOPY_BUFFER_GB` environment variable:
```go
totalMemory = AZCOPY_BUFFER_GB * 1GiB
chunkMemory = totalMemory * 0.8  // 80% for chunks
poolSize = chunkMemory / chunkSize
```

**Safeguards**:
- Reject chunk size if > available memory
- Warn if chunk count < minimum parallelism
- Auto-tune chunk size based on file size

### Cache Limiters

**CacheLimiter**: Limit concurrent operations
```go
limiter := common.NewCacheLimiter(maxConcurrent)
limiter.WaitUntilAdd() // Block if at limit
// ... perform operation ...
limiter.Remove()
```

**Use Cases**:
- Limit open file handles
- Control concurrent API calls
- Prevent resource exhaustion

## Testing Strategy

### Unit Tests

Pattern: `*_test.go` files
- Test individual components in isolation
- Mock dependencies
- Fast execution

Examples:
- `folderCreationTracker_test.go`
- `pageRangeOptimizer_test.go`
- `sender_blockBlob_test.go`

### Integration Tests

Pattern: `zt_*_test.go` files (zt = "ztest")
- Test component interactions
- May require Azure credentials
- Longer execution time

Examples:
- `zt_concurrencyTuner_test.go`
- `zt_performanceAdvisor_test.go`
- `zt_xfer-anyToRemote-file_test.go`

### Test Helpers

`testJobPartTransferManager_test.go`: Mock transfer manager for testing

## Performance Tuning

### Concurrency

**Optimize for**:
- Large files: High chunk multiplier
- Many small files: High file concurrency
- Mixed workload: Balanced settings

**Tuning**:
```bash
export AZCOPY_CONCURRENCY_VALUE=32      # Total parallelism
export AZCOPY_CHUNK_MULTIPLIER=4        # Chunks per file
export AZCOPY_CONCURRENT_FILES=8        # Concurrent files
```

### Chunk Size

**Trade-offs**:
- Larger chunks: Fewer API calls, more memory
- Smaller chunks: Better parallelism, more overhead

**Guidelines**:
- Small files (<100MB): Single chunk (PutBlob)
- Medium files (100MB-1GB): 8MB chunks
- Large files (>1GB): 32-100MB chunks

### Rate Limiting

**When to use**:
- Prevent service throttling
- Share bandwidth with other apps
- Testing and validation

**Configuration**:
```bash
azcopy copy ... --cap-mbps=100
```

### Memory Tuning

**Settings**:
```bash
export AZCOPY_BUFFER_GB=4  # Total memory for transfers
```

**Recommendations**:
- Minimum: 1GB for basic transfers
- Recommended: 4-8GB for optimal performance
- High-throughput: 16GB+ for large concurrent operations

## Common Debugging Techniques

### 1. Enable Verbose Logging

```bash
export AZCOPY_LOG_LEVEL=DEBUG
azcopy copy ... --log-level=DEBUG
```

Check logs in:
- `~/.azcopy/` (default log location)

### 2. Check Job Plan Files

Location: `~/.azcopy/plans/`
- Contains serialized job state
- Can be inspected for debugging resume issues

### 3. Monitor Network Stats

Job manager tracks:
- Bytes sent/received
- Request latency
- Error rates

Access via job status APIs

### 4. Analyze Chunk Status

ChunkStatusLogger tracks:
- Chunk completion
- Wait reasons (network, disk, cancellation)
- Bottleneck identification

### 5. Performance Advisor Output

Automatically detects:
- Slow disk I/O
- Network bottlenecks
- CPU/memory pressure

Provides actionable recommendations

## Future Enhancements

### Potential Improvements

1. **Memory Management**
   - Implement buffer pooling optimizations
   - Reduce allocations in hot paths
   - Better memory pressure handling

2. **Concurrency**
   - More sophisticated auto-tuning
   - Adaptive chunk sizing
   - Load balancing across workers

3. **Retry Logic**
   - Circuit breaker pattern
   - Smarter backoff strategies
   - Better error categorization

4. **Monitoring**
   - Real-time performance metrics
   - Telemetry integration
   - Detailed profiling hooks

5. **Testing**
   - Increase unit test coverage in `ste/`
   - More edge case tests for resume
   - Performance regression tests

## Related Documentation

- [Architecture Decisions](.claude/context/architecture-decisions.md)
- [Known Issues](.claude/context/known-issues.md)
- [Development Notes](.claude/context/development-notes.md)
- [Project README](../README.md)
- [CLAUDE.md](../CLAUDE.md)

---

**Document Version**: 1.0
**Contributors**: Claude Code AI Analysis
**Next Review**: When significant STE changes are made
