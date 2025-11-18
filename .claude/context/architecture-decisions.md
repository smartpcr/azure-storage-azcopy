# AzCopy Architecture Decisions

## Job-Based Transfer Architecture

**Decision**: Use job-based architecture with persisted job plans
**Rationale**:
- Enables resume capability after interruption
- Allows parallel processing of job parts
- Provides progress tracking and status reporting
- Separates enumeration from transfer execution

**Components**:
- `ste/JobPartPlan.go` - Job planning and management
- Job parts can be processed independently
- Job plans persisted to disk for resume

## Traverser Pattern for Source Enumeration

**Decision**: Use separate traversers for each storage type
**Rationale**:
- Clean separation of concerns
- Storage-specific optimization
- Easy to add new storage types
- Consistent interface across sources

**Implementations**:
- `cmd/zc_traverser_blob.go` - Azure Blob
- `cmd/zc_traverser_file.go` - Azure Files
- `cmd/zc_traverser_s3.go` - AWS S3
- `cmd/zc_traverser_gcp.go` - GCP Storage
- `cmd/zc_traverser_local.go` - Local filesystem

## Chunk-Based Parallel Transfers

**Decision**: Break large files into chunks for parallel transfer
**Rationale**:
- Better throughput for large files
- Efficient use of network bandwidth
- Enables retry at chunk level
- Progress reporting granularity

**Implementation**:
- Configurable chunk size
- Parallel goroutines per file
- Chunk status tracking for resume

## Platform-Specific File Handling

**Decision**: Use platform-specific implementations for file operations
**Rationale**:
- Preserve platform-specific attributes (ACLs, permissions, timestamps)
- Optimize for OS-specific file APIs
- Handle platform differences in symbolic links

**Implementation**:
- Separate Linux/Windows implementations in `ste/`
- Platform build tags
- Abstraction layer in `common/`

## Credential Management

**Decision**: Support multiple authentication methods with priority order
**Rationale**:
- Flexibility for different scenarios
- Security best practices (prefer OAuth over keys)
- Support for different environments (dev, prod, CI/CD)

**Methods** (in priority order):
1. SAS tokens (from URL)
2. OAuth tokens (from `login` command)
3. Managed Identity
4. Service Principal
5. Storage account keys

## Service-to-Service Copy

**Decision**: Implement direct service-to-service copy without local staging
**Rationale**:
- Eliminates local disk I/O bottleneck
- Reduces bandwidth requirements
- Faster for large datasets
- Lower cost (no egress for local download)

**Implementation**:
- Senders in `ste/sender-*.go`
- Use storage service copy APIs
- Handle cross-region copies

---

*Add new architectural decisions as they are made*
