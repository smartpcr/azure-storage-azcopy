# AzCopy Implementation Guide

## Table of Contents
1. [Overview](#overview)
2. [Architecture](#architecture)
3. [Core Components](#core-components)
4. [Transfer Flow](#transfer-flow)
5. [Authentication System](#authentication-system)
6. [Storage Service Integrations](#storage-service-integrations)
7. [Job Management](#job-management)
8. [Development Patterns](#development-patterns)
9. [Usage Examples](#usage-examples)

---

## Overview

AzCopy is a high-performance command-line utility for transferring data to and from cloud storage services. The codebase (~490 Go files) implements a sophisticated, job-based transfer engine with support for multiple cloud storage providers.

**Key Characteristics:**
- **Language:** Go
- **Architecture:** Job-based transfer engine with memory-mapped file persistence
- **Concurrency:** Highly parallelized with configurable worker pools
- **Resilience:** Automatic retry logic, resume capability, and progress persistence
- **Scale:** Designed to handle millions of files efficiently

---

## Architecture

### High-Level Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                      CLI Layer (cmd/)                       │
│  ┌──────────────┐  ┌─────────────┐  ┌──────────────────┐    │
│  │ Command      │  │ Traversers  │  │  Enumerators     │    │
│  │ Handlers     │  │             │  │  & Processors    │    │
│  └──────┬───────┘  └──────┬──────┘  └────────┬─────────┘    │
└─────────┼─────────────────┼──────────────────┼──────────────┘
          │                 │                  │
          ▼                 ▼                  ▼
┌─────────────────────────────────────────────────────────────┐
│              JobsAdmin Layer (jobsAdmin/)                   │
│  - Job lifecycle management                                 │
│  - Job part coordination                                    │
│  - Concurrency configuration                                │
└─────────────────────────┬───────────────────────────────────┘
                          │
                          ▼
┌─────────────────────────────────────────────────────────────┐
│        Storage Transfer Engine (STE) (ste/)                 │
│  ┌──────────────┐  ┌─────────────┐  ┌──────────────────┐    │
│  │ Job Part     │  │  Uploaders  │  │   Downloaders    │    │
│  │ Manager      │  │  & Senders  │  │                  │    │
│  └──────────────┘  └─────────────┘  └──────────────────┘    │
└─────────────────────────────────────────────────────────────┘
                          │
                          ▼
┌─────────────────────────────────────────────────────────────┐
│              Common Utilities (common/)                     │
│  - Authentication (OAuth, SAS, Keys)                        │
│  - Lifecycle management                                     │
│  - Logging & monitoring                                     │
│  - Retry policies                                           │
└─────────────────────────────────────────────────────────────┘
```

### Design Principles

1. **Separation of Concerns:** Clear boundaries between command parsing (cmd/), job management (jobsAdmin/), transfer execution (ste/), and utilities (common/)

2. **Job-Based Architecture:** All operations are modeled as jobs divided into parts for parallel processing and resumability

3. **Memory-Mapped Files:** Job plans persist in memory-mapped files for crash recovery and status tracking

4. **Provider Abstraction:** Storage-agnostic interfaces (traversers, senders, downloaders) with provider-specific implementations

---

## Core Components

### 1. Command Layer (`cmd/`)

#### Root Command Handler (`cmd/root.go`)
- **Entry Point:** Uses Cobra framework for CLI command routing
- **Initialization:** Sets up logging, output format, concurrency settings
- **Job ID Management:** Creates or resumes jobs with unique JobIDs
- **Version Check:** Optional background version verification via GitHub API

Key initialization flow:
```
rootCmd.Execute() → Initialize() → Create/Resume JobID → Setup logging → Start STE
```

#### Commands
Located in `cmd/`:
- `copy.go` - Core copy operations between storage services
- `sync.go` - Synchronization with change detection
- `list.go`, `make.go`, `remove.go` - Storage operations
- `login.go`/`logout.go` - Azure AD authentication
- `resume.go` - Resume interrupted jobs

#### Traversers (`cmd/zc_traverser_*.go`)

**Purpose:** Enumerate source objects for transfer

**Implementations:**
- `blobTraverser` - Azure Blob Storage (flat & hierarchical)
- `fileTraverser` - Azure Files
- `localTraverser` - Local filesystem (with platform-specific variants)
- `s3Traverser` - AWS S3
- `gcpTraverser` - Google Cloud Storage

**Pattern:**
```go
type Traverser interface {
    IsDirectory(isSource bool) (bool, error)
    Traverse(preprocessor objectMorpher, processor objectProcessor, filters []ObjectFilter) error
}
```

Each traverser:
1. Validates source (single file vs directory)
2. Lists objects matching filters
3. Applies preprocessor transformations
4. Passes objects to processor for job scheduling

#### Enumerators (`cmd/zc_enumerator.go`)

**StoredObject** - Universal representation of source/destination entities:
```go
type StoredObject struct {
    name             string
    entityType       common.EntityType  // File, Folder, Symlink
    lastModifiedTime time.Time
    size             int64
    md5              []byte
    blobType         blob.BlobType
    relativePath     string             // Relative to root
    Metadata         common.Metadata
    // ... storage-specific properties
}
```

**Responsibilities:**
- Normalize objects from different storage providers
- Apply filters (include/exclude patterns, date ranges)
- Handle entity type compatibility (files, folders, symlinks)

#### Processors (`cmd/zc_processor.go`)

**copyTransferProcessor** - Converts StoredObjects into transfer jobs:
- Batches transfers into job parts (configurable size)
- Applies preservation rules (access tier, permissions, metadata)
- Handles dry-run mode for preview
- Dispatches job parts to JobsAdmin

---

### 2. JobsAdmin Layer (`jobsAdmin/`)

**Responsibilities:**
- Job lifecycle management (create, pause, resume, cancel)
- Job part coordination
- Concurrency tuning
- Job resurrection from persisted plans

**Key Functions (`jobsAdmin/init.go`):**

```go
// Creates job part plan and schedules transfers
ExecuteNewCopyJobPartOrder(order common.CopyJobPartOrderRequest)

// Pause/cancel active jobs
CancelPauseJobOrder(jobID common.JobID, desiredJobStatus common.JobStatus)

// Resume from persisted job plans
ResumeJobOrder(req common.ResumeJobRequest)
```

**Job Part Plan File:** Memory-mapped file structure containing:
- Job metadata (JobID, part number, source/dest roots)
- Transfer list with source/dest paths
- Transfer properties (HTTP headers, metadata, tags)
- Status tracking (atomic job/part status)

---

### 3. Storage Transfer Engine (STE) (`ste/`)

The execution layer that performs actual data transfers.

#### Job Part Plan (`ste/JobPartPlan.go`)

Memory-mapped structure persisting job state:

```go
type JobPartPlanHeader struct {
    Version              common.Version
    JobID                common.JobID
    PartNum              common.PartNumber
    SourceRoot           [1000]byte
    DestinationRoot      [1000]byte
    NumTransfers         uint32
    FromTo               common.FromTo
    // Atomic status fields
    atomicJobStatus      common.JobStatus
    atomicPartStatus     common.JobStatus
    // ... configuration fields
}
```

**Benefits:**
- Crash recovery: Resumes from exact position
- Status queries: Real-time job status without network calls
- Persistence: Job history available after completion

#### Senders (`ste/sender*.go`)

**Abstraction for uploads and service-to-service copies:**

```go
type sender interface {
    ChunkSize() int64
    NumChunks() uint32
    RemoteFileExists() (bool, time.Time, error)
    Prologue(state common.PrologueState) bool
    Epilogue()
    Cleanup()
}

type uploader interface {
    sender
    GenerateUploadFunc(chunkID, blockIndex, reader, chunkIsWholeFile) chunkFunc
    Md5Channel() chan<- []byte
}

type s2sCopier interface {
    sender
    GenerateCopyFunc(chunkID, blockIndex, adjustedChunkSize, chunkIsWholeFile) chunkFunc
}
```

**Implementations:**
- `sender-blockBlobFromLocal.go` - Upload block blobs
- `sender-pageBlobFromLocal.go` - Upload page blobs
- `sender-azureFileFromLocal.go` - Upload to Azure Files
- `sender-blockBlobFromURL.go` - S2S block blob copy
- `sender-blobFolders.go` - Folder property handling

**Transfer Flow:**
1. **Prologue:** Create destination, validate overwrite policy
2. **Chunk Generation:** Break file into chunks, schedule parallel uploads
3. **Chunk Execution:** Upload/copy chunks with retry logic
4. **Epilogue:** Finalize (e.g., PutBlockList for block blobs)
5. **Cleanup:** Set metadata, tags, tier; report completion

#### Downloaders (`ste/downloader*.go`)

Implementations for downloading from cloud storage to local filesystem, with similar abstraction patterns.

#### Job Part Manager (`ste/mgr-JobPartMgr.go`)

Orchestrates transfer execution:
- **ScheduleTransfers()** - Iterates job plan, creates transfer managers
- **Worker Pool Management** - Configurable parallelism
- **Status Aggregation** - Reports progress to JobMgr
- **Chunk Scheduling** - Breaks transfers into chunks for parallel execution

---

### 4. Common Utilities (`common/`)

#### Authentication (`common/oauthTokenManager.go`)

**UserOAuthTokenManager** - Handles Azure AD authentication:

**Supported Methods:**
1. **Interactive Login** - Device code flow
2. **Service Principal** - Client credentials
3. **Managed Identity** - Azure VM/Container identity
4. **Azure CLI** - Reuse `az login` tokens
5. **Workload Identity** - Kubernetes workload identity
6. **Token Cache** - Platform-specific secure storage

**Token Management:**
```go
type OAuthTokenInfo struct {
    LoginType              EAutoLoginType
    Tenant                 string
    ActiveDirectoryEndpoint string
    Persist                bool
}

func (uotm *UserOAuthTokenManager) GetTokenInfo(ctx context.Context) (*OAuthTokenInfo, error)
```

**Credential Types:**
- OAuth tokens (Azure AD)
- SAS tokens (query string parameters)
- Storage account keys
- AWS access keys (for S3)
- GCP service account keys

#### Lifecycle Manager (`common/lifecycleMgr.go`)

Coordinates application lifecycle:
- Output formatting (text, JSON)
- Progress reporting
- Error handling and exit codes
- Graceful shutdown

#### Data Models (`common/fe-ste-models.go`)

**CopyJobPartOrderRequest** - Job specification sent to STE:
```go
type CopyJobPartOrderRequest struct {
    JobID        JobID
    PartNum      PartNumber
    FromTo       FromTo
    Transfers    Transfers
    ForceWrite   OverwriteOption
    // Preservation options
    PreservePermissions      PreservePermissionsOption
    PreserveSMBInfo          bool
    PreservePOSIXProperties  bool
    // S2S options
    S2SPreserveProperties    bool
    S2SPreserveAccessTier    bool
    // Credentials
    CredentialInfo           CredentialInfo
}
```

---

## Transfer Flow

### End-to-End Copy Flow

```
┌──────────────────────────────────────────────────────────────┐
│ 1. Command Parsing (cmd/copy.go)                             │
│    - Parse source/destination                                 │
│    - Validate FromTo (Local→Blob, Blob→Blob, etc.)           │
│    - Build CookedCopyCmdArgs                                  │
└────────────────────────┬─────────────────────────────────────┘
                         │
                         ▼
┌──────────────────────────────────────────────────────────────┐
│ 2. Source Enumeration (cmd/zc_traverser_*.go)                │
│    - Traverser.IsDirectory() checks if source is dir/file    │
│    - Traverser.Traverse() enumerates all objects             │
│    - Apply filters (include/exclude, date range)             │
│    - Create StoredObject for each item                       │
└────────────────────────┬─────────────────────────────────────┘
                         │
                         ▼
┌──────────────────────────────────────────────────────────────┐
│ 3. Object Processing (cmd/zc_processor.go)                   │
│    - Batch objects into job parts (~10k transfers/part)      │
│    - StoredObject → CopyTransfer conversion                  │
│    - Build CopyJobPartOrderRequest                           │
└────────────────────────┬─────────────────────────────────────┘
                         │
                         ▼
┌──────────────────────────────────────────────────────────────┐
│ 4. Job Submission (jobsAdmin/init.go)                        │
│    - Create job part plan file (memory-mapped)               │
│    - Get/create JobMgr for this JobID                        │
│    - JobMgr.AddJobPart() with plan                           │
└────────────────────────┬─────────────────────────────────────┘
                         │
                         ▼
┌──────────────────────────────────────────────────────────────┐
│ 5. Transfer Scheduling (ste/mgr-JobPartMgr.go)               │
│    - ScheduleTransfers() reads job plan                      │
│    - For each transfer, create JobPartTransferMgr            │
│    - Dispatch to worker pool                                 │
└────────────────────────┬─────────────────────────────────────┘
                         │
                         ▼
┌──────────────────────────────────────────────────────────────┐
│ 6. Transfer Execution (ste/xfer-*.go)                        │
│    - Create sender (uploader/s2sCopier/downloader)           │
│    - Prologue: validate overwrite, create destination        │
│    - Generate chunk funcs (parallel upload/copy)             │
│    - Execute chunks via worker pool                          │
│    - Epilogue: finalize (PutBlockList, etc.)                 │
│    - Cleanup: set properties, report completion              │
└────────────────────────┬─────────────────────────────────────┘
                         │
                         ▼
┌──────────────────────────────────────────────────────────────┐
│ 7. Status Reporting                                           │
│    - Update JobPartPlan atomically                            │
│    - Report progress via lifecycleMgr                         │
│    - Log to job log file                                      │
└──────────────────────────────────────────────────────────────┘
```

### Chunk-Level Parallelism

Large files are split into chunks for parallel transfer:

```
File: 1 GB, ChunkSize: 8 MB → 128 chunks

Worker Pool (configurable, e.g., 100 workers):
  Worker 1: Upload chunk 0
  Worker 2: Upload chunk 1
  ...
  Worker 100: Upload chunk 99
  [Workers loop until all chunks complete]

Coordination:
  - Each chunk reports completion to JobPartTransferMgr
  - Progress tracked per-chunk
  - Retry logic per chunk
  - After all chunks complete → Epilogue (e.g., PutBlockList)
```

---

## Authentication System

### Credential Resolution Order

**Azure Storage (Blob, Files):**
1. SAS token in URL (highest priority)
2. Environment variable `AZCOPY_OAUTH_TOKEN_INFO`
3. Cached OAuth token (from `azcopy login`)
4. Azure CLI token (`az login`)
5. Managed Identity (VM/Container)
6. Workload Identity (Kubernetes)
7. Service Principal (client credentials)
8. Interactive device code flow (last resort)

**AWS S3:**
- AWS access key/secret from environment or `~/.aws/credentials`
- S3 SAS equivalent (pre-signed URLs)

**Google Cloud Storage:**
- GCP service account key JSON
- Application default credentials

### Token Caching

**Platform-Specific Storage:**
- **Windows:** Windows Credential Manager
- **macOS:** Keychain
- **Linux:** Encrypted file in `~/.azcopy/`

**Cache File:** `AzCopyTokenCache` with expiration tracking

### Example: OAuth Flow

```go
// Login
uotm := NewUserOAuthTokenManagerInstance(credCacheOptions)
uotm.AzCliLogin(tenantID, persist=true)  // Reuse az login token

// Get token for transfer
tokenInfo, err := uotm.GetTokenInfo(ctx)
credential, err := tokenInfo.GetTokenCredential()

// Use with Azure SDK
serviceClient, err := service.NewClient(accountURL, credential, nil)
```

---

## Storage Service Integrations

### Azure Blob Storage

**Traverser:** `blobTraverser` (`cmd/zc_traverser_blob.go`)
- Uses Azure Blob SDK (`github.com/Azure/azure-sdk-for-go/sdk/storage/azblob`)
- Supports flat and hierarchical listing
- Handles blob types (Block, Page, Append)
- Directory stub detection (`hdi_isfolder` metadata)
- Parallel listing for large containers

**Senders:**
- `sender-blockBlobFromLocal.go` - Chunked upload with PutBlock/PutBlockList
- `sender-blockBlobFromURL.go` - S2S with Put Block From URL
- `sender-pageBlobFromLocal.go` - Page-aligned uploads
- `sender-appendBlobFromLocal.go` - Append operations

**Features:**
- Access tier support (Hot, Cool, Archive, Cold)
- Blob tags and metadata preservation
- Snapshot and version handling
- CPK (customer-provided key) support
- Lease management

### Azure Files

**Traverser:** `fileTraverser` (`cmd/zc_traverser_file.go`)
- SMB property preservation
- ACL handling (preserveSMBPermissions flag)
- Folder hierarchy support

**Senders:**
- `sender-azureFileFromLocal.go` - SMB-aware uploads
- `sender-azureFileFromURL.go` - S2S copies

**Features:**
- SMB timestamps (creation, last write, change time)
- Windows file attributes (readonly, hidden, system, archive)
- NTFS ACLs (when --preserve-smb-permissions=true)

### Azure Data Lake Storage Gen2 (ADLS)

**Traverser:** `blobTraverser` with `isDFS=true`
- Uses ADLS SDK (`github.com/Azure/azure-sdk-for-go/sdk/storage/azdatalake`)
- Hierarchical namespace operations
- POSIX permissions

**Senders:**
- `sender-blobFS.go` - Path-based operations
- `sender-blobFSFromLocal.go` - POSIX-aware uploads

**Features:**
- ACL preservation (preservePOSIXProperties flag)
- Recursive delete optimization

### AWS S3

**Traverser:** `s3Traverser` (`cmd/zc_traverser_s3.go`)
- Uses AWS SDK for Go
- Bucket and prefix enumeration
- Credential handling via AWS standard mechanisms

**Limitations:**
- S2S only (S3 → Azure)
- Metadata translation (S3 headers → Azure metadata)

### Google Cloud Storage

**Traverser:** `gcpTraverser` (`cmd/zc_traverser_gcp.go`)
- Uses GCP Storage SDK
- Bucket enumeration

**Limitations:**
- S2S only (GCS → Azure)

### Local Filesystem

**Traversers:**
- `localTraverser` (base) (`cmd/zc_traverser_local.go`)
- `localTraverser` (Windows) (`cmd/zc_traverser_local_windows.go`)
- `localTraverser` (Other) (`cmd/zc_traverser_local_other.go`)

**Features:**
- Symlink handling (follow, preserve)
- Hardlink handling
- Extended attributes (Windows, Linux)
- File permissions preservation

---

## Job Management

### Job States

```
Created → InProgress → [Paused/Cancelled] → Completed/Failed
```

**Atomic State Transitions:**
```go
jpph.SetJobStatus(newJobStatus common.JobStatus)  // Thread-safe atomic store
status := jpph.JobStatus()                        // Thread-safe atomic load
```

### Job Persistence

**Plan File Location:** `~/.azcopy/<jobID>/<partNum>.steV<version>`

**Structure:**
1. Header: JobPartPlanHeader (fixed size)
2. Command string: Original CLI command
3. Transfers: Array of JobPartPlanTransfer structs
4. Strings: Source/dest paths (variable offsets)

**Resume Capability:**
```bash
# Job interrupted
azcopy copy https://source.blob.core.windows.net/container /dest --recursive

# Resume
azcopy resume <jobID>
```

**Job Resurrection:**
- On resume, JobsAdmin scans plan files
- Recreates JobMgr and JobPartMgr structures
- Skips completed transfers
- Resumes in-progress chunks

### Status Monitoring

```bash
# List jobs
azcopy jobs list

# Show job details
azcopy jobs show <jobID>

# Clean completed jobs
azcopy jobs clean
```

---

## Development Patterns

### 1. Provider Abstraction Pattern

New storage provider integration checklist:

**Traverser Implementation:**
```go
type newProviderTraverser struct {
    // Provider-specific client
    // Context, filters, etc.
}

func (t *newProviderTraverser) IsDirectory(isSource bool) (bool, error)
func (t *newProviderTraverser) Traverse(preprocessor, processor, filters) error
```

**Sender Implementation:**
```go
type newProviderUploader struct {
    jptm IJobPartTransferMgr
    // Provider-specific client
}

func (u *newProviderUploader) ChunkSize() int64
func (u *newProviderUploader) NumChunks() uint32
func (u *newProviderUploader) Prologue(state PrologueState) bool
func (u *newProviderUploader) GenerateUploadFunc(...) chunkFunc
func (u *newProviderUploader) Epilogue()
func (u *newProviderUploader) Cleanup()
```

**FromTo Registration:**
```go
// common/fe-ste-models.go
func (FromTo) NewProviderLocal() FromTo { return FromTo(X) }
```

### 2. Error Handling Pattern

**Retry Logic:**
```go
// Automatic retry for transient errors
ste.RetryStatusCodes = []int{408, 429, 500, 502, 503, 504}

// Exponential backoff (handled by Azure SDK retry policies)
```

**Error Propagation:**
```go
// Chunk-level error
jptm.FailActiveUpload("operation description", err)

// Transfer-level error
jptm.SetStatus(common.ETransferStatus.Failed())
jptm.ReportTransferDone()
```

### 3. Concurrency Pattern

**Worker Pool:**
```go
// jobsAdmin sets concurrency
JobsAdmin.SetConcurrencySettings(concurrency ste.ConcurrencySettings)

// Worker pool processes chunk funcs
for chunkFunc := range chunkChannel {
    go worker(chunkFunc)
}
```

**Configuration:**
- Environment variable: `AZCOPY_CONCURRENCY_VALUE` or `auto`
- Auto-tuning based on network bandwidth and CPU
- Benchmark mode uses auto-tuning by default

### 4. Testing Patterns

**E2E Tests (`e2etest/`):**
- Scenario-based tests (upload, download, sync)
- Real storage account integration
- Declarative test definitions

**Unit Tests:**
- Traverser tests: `cmd/zt_traverser_*_test.go`
- Sender tests: `ste/sender_*_test.go`
- Mock server: `mock_server/` for offline testing

---

## Usage Examples

### Basic Copy Operations

```bash
# Upload directory to blob container
azcopy copy "/local/path" "https://account.blob.core.windows.net/container?<SAS>" --recursive

# Download blob container
azcopy copy "https://account.blob.core.windows.net/container?<SAS>" "/local/path" --recursive

# S2S copy (Blob to Blob)
azcopy copy "https://source.blob.core.windows.net/container?<SAS>" \
            "https://dest.blob.core.windows.net/container?<SAS>" --recursive

# S2S from S3
azcopy copy "https://s3.amazonaws.com/bucket" \
            "https://dest.blob.core.windows.net/container?<SAS>" --recursive
```

### Advanced Features

```bash
# Preserve access tier
azcopy copy "https://source/..." "https://dest/..." \
       --s2s-preserve-access-tier=true

# Preserve SMB permissions (Azure Files)
azcopy copy "/local/path" "https://account.file.core.windows.net/share?<SAS>" \
       --preserve-smb-permissions=true --preserve-smb-info=true

# Include/exclude patterns
azcopy copy "/local/path" "https://dest/..." \
       --include-pattern="*.jpg;*.png" --exclude-pattern="*/temp/*"

# Sync (only newer files)
azcopy sync "/local/path" "https://dest/..." --recursive --delete-destination=true

# Dry run (preview)
azcopy copy "/local/path" "https://dest/..." --dry-run
```

### Authentication

```bash
# Login with Azure AD
azcopy login --tenant-id <tenant-id>

# Login with service principal
azcopy login --service-principal \
       --application-id <app-id> --tenant-id <tenant-id> \
       --certificate-path <cert-path>

# Use managed identity
azcopy login --identity

# Logout
azcopy logout
```

### Job Management

```bash
# Resume interrupted job
azcopy resume <jobID>

# List all jobs
azcopy jobs list

# Show job status
azcopy jobs show <jobID>

# Cancel job
azcopy jobs remove <jobID>

# Clean completed jobs
azcopy jobs clean
```

### Performance Tuning

```bash
# Set concurrency (number of parallel operations)
export AZCOPY_CONCURRENCY_VALUE=100

# Cap bandwidth (Mbps)
azcopy copy "..." "..." --cap-mbps=500

# Adjust block size (MB)
azcopy copy "..." "..." --block-size-mb=16

# Auto-tune for benchmarking
azcopy bench "https://dest/..." --size-per-file=100M --num-of-files=100
```

---

## Key File Reference

| Path | Purpose |
|------|---------|
| `cmd/root.go` | CLI entry point, command routing |
| `cmd/copy.go` | Copy command implementation |
| `cmd/zc_traverser_*.go` | Storage provider traversers |
| `cmd/zc_enumerator.go` | StoredObject and filtering |
| `cmd/zc_processor.go` | Job part creation |
| `ste/JobPartPlan.go` | Memory-mapped job plan structure |
| `ste/sender.go` | Sender interface definitions |
| `ste/sender-blockBlobFromLocal.go` | Block blob upload implementation |
| `ste/mgr-JobPartMgr.go` | Transfer scheduling and execution |
| `jobsAdmin/init.go` | Job lifecycle management |
| `common/oauthTokenManager.go` | OAuth authentication |
| `common/lifecycleMgr.go` | Application lifecycle coordination |
| `common/fe-ste-models.go` | Job and transfer data models |

---

## Summary

AzCopy implements a robust, scalable architecture for cloud data transfers:

**Strengths:**
- **Modularity:** Clear separation between CLI, job management, and transfer engine
- **Resumability:** Memory-mapped job plans enable crash recovery
- **Performance:** Chunk-level parallelism and auto-tuning
- **Extensibility:** Provider abstraction allows easy integration of new storage services
- **Authentication:** Comprehensive support for multiple authentication methods

**Design Highlights:**
- Job-based architecture with persistent state
- Traverser pattern for storage enumeration
- Sender abstraction for upload/copy operations
- Atomic status tracking via memory-mapped files
- Configurable concurrency and auto-tuning

This architecture enables AzCopy to efficiently transfer petabytes of data while maintaining reliability and providing a seamless user experience.