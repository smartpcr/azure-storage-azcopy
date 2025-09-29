# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Commands

### Build
```bash
# Build standard binary
go build

# Build with netgo tags (for static linking)
go build -tags "netgo" -o azcopy_bin

# Build with coverage enabled
go build -cover -o azcopy

# Platform-specific builds
CGO_ENABLED=1 go build -o azcopy_darwin_amd64  # macOS AMD64
GOARCH=arm64 CGO_ENABLED=1 go build -o azcopy_darwin_arm64  # macOS ARM64
go build -o azcopy_windows_amd64.exe  # Windows
```

### Test
```bash
# Run unit tests
go test -timeout=1h -v ./cmd
go test -timeout=1h -v ./common
go test -timeout=1h -v ./ste

# Run unit tests with coverage
go test -timeout=1h -v -coverprofile=coverage.txt ./cmd

# Run a single test
go test -v -run TestNamePattern ./cmd

# Run e2e tests (requires environment setup)
go test -timeout=2h -v ./e2etest
```

### Lint
```bash
go vet
```

## Architecture

### Core Components

1. **cmd/** - Command-line interface and business logic
   - `root.go` - Main command router using Cobra
   - `copy.go` - Core copy operations between storage services
   - `sync.go` - Synchronization operations
   - `list.go`, `make.go`, `remove.go` - Storage operations
   - `login.go`/`logout.go` - Azure AD authentication
   - Various traversers (`zc_traverser_*.go`) - Handle different storage types (blob, file, S3, GCP, local)
   - Enumerators (`*Enumerator*.go`) - Enumerate sources and apply filters
   - Processors (`*Processor.go`) - Process enumerated items

2. **ste/** - Storage Transfer Engine
   - `JobPartPlan.go` - Job planning and management
   - Various downloaders (`downloader-*.go`) - Handle downloads from different sources
   - Various uploaders (`uploader-*.go`) - Handle uploads to different destinations
   - `sender-*.go` - Coordinate transfers between sources and destinations
   - Platform-specific implementations for Linux/Windows file handling

3. **common/** - Shared utilities and types
   - Authentication utilities (`oauthTokenManager.go`, credential classes)
   - Lifecycle management (`lifecycleMgr.go`)
   - Performance monitoring (`cpuMonitor.go`, `chunkStatusLogger.go`)
   - Error handling and retry logic

4. **e2etest/** - End-to-end test suite
   - Scenario-based testing for all major operations
   - Integration tests with real storage services

### Transfer Architecture

AzCopy uses a job-based architecture where:
- Commands create jobs that are executed by the Storage Transfer Engine (STE)
- Jobs are divided into parts that can be processed in parallel
- Each transfer goes through: Enumeration → Filtering → Processing → Transfer
- Supports resume capability through job plan persistence

### Storage Service Support

- **Azure Blob Storage** - Full support including block/page/append blobs
- **Azure Files** - File share operations with SMB metadata preservation  
- **Azure Data Lake Storage Gen2** - Hierarchical namespace support
- **AWS S3** - Service-to-service copy to Azure
- **Google Cloud Storage** - Service-to-service copy to Azure
- **Local File System** - Upload/download with attribute preservation

### Authentication Methods

- Azure AD OAuth (login/logout commands)
- SAS tokens (URL parameters)
- Storage account keys
- Service principals
- Managed identities
- AWS access keys (for S3)
- GCP service account keys