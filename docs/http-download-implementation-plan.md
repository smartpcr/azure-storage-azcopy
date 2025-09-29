# Implementation Plan: HTTP Downloads with OAuth Support

## Executive Summary

This document provides a comprehensive, production-ready implementation plan to add **generic HTTP download support** to AzCopy, including:
- Segmented/chunked downloads via HTTP Range requests
- OAuth 2.0 Bearer token authentication
- Fallback for servers without range support
- Integration with existing AzCopy architecture

**Target Milestone:** AzCopy v10.x
**Estimated Effort:** 8-12 weeks (1-2 engineers)
**Risk Level:** Medium (requires careful integration with existing systems)

---

## Table of Contents

1. [Background and Motivation](#background-and-motivation)
2. [Architecture Overview](#architecture-overview)
3. [Implementation Phases](#implementation-phases)
4. [Detailed Component Design](#detailed-component-design)
5. [Integration Points](#integration-points)
6. [Testing Strategy](#testing-strategy)
7. [Performance Considerations](#performance-considerations)
8. [Security Considerations](#security-considerations)
9. [Migration and Compatibility](#migration-and-compatibility)
10. [Open Questions and Risks](#open-questions-and-risks)
11. [Timeline and Milestones](#timeline-and-milestones)

---

## Background and Motivation

### Current State
AzCopy currently supports downloads from:
- Azure Blob Storage
- Azure Files
- Azure Data Lake Storage Gen2
- AWS S3 (S2S to Azure only)
- GCP Storage (S2S to Azure only)

**Gap:** No support for generic HTTP/HTTPS endpoints with custom authentication.

### Use Cases
1. **Data Migration:** Downloading from HTTP-based object stores (MinIO, Ceph, etc.)
2. **CI/CD Pipelines:** Fetching build artifacts from HTTP servers with OAuth
3. **Content Delivery:** Downloading from CDNs with token-based auth
4. **Hybrid Cloud:** Accessing on-premises HTTP storage with API keys
5. **Research Data:** Downloading datasets from scientific repositories

### Requirements
- ✅ Support HTTP Range requests for segmented downloads
- ✅ OAuth 2.0 Bearer token authentication
- ✅ Fallback to single-threaded download if no range support
- ✅ Reuse existing chunked writer and validation infrastructure
- ✅ Maintain backward compatibility with existing AzCopy features

---

## Architecture Overview

### High-Level Design

```
┌─────────────────────────────────────────────────────────────┐
│                    User Command (CLI)                       │
│  azcopy copy "https://api.example.com/files/data.bin"       │
│              "/local/path" --bearer-token="..."             │
└────────────────────────┬────────────────────────────────────┘
                         │
                         ▼
┌─────────────────────────────────────────────────────────────┐
│              Copy Command Handler (cmd/copy.go)             │
│  - Detect HTTP location (new: ELocation.Http())             │
│  - Parse authentication flags                                │
└────────────────────────┬────────────────────────────────────┘
                         │
                         ▼
┌─────────────────────────────────────────────────────────────┐
│         HTTP Traverser (cmd/zc_traverser_http.go)           │
│  - HEAD request to get Content-Length, Accept-Ranges        │
│  - Detect range support                                      │
│  - Enumerate single file (HTTP doesn't support listing)     │
└────────────────────────┬────────────────────────────────────┘
                         │
                         ▼
┌─────────────────────────────────────────────────────────────┐
│        Transfer Engine (ste/xfer-remoteToLocal-file.go)     │
│  - Existing download orchestration (reused)                 │
└────────────────────────┬────────────────────────────────────┘
                         │
                         ▼
┌─────────────────────────────────────────────────────────────┐
│         HTTP Downloader (ste/downloader-http.go)            │
│  - Range-based chunk downloads                               │
│  - OAuth Bearer token injection                              │
│  - Retry logic for transient errors                          │
│  - Fallback to single-threaded if no range support          │
└────────────────────────┬────────────────────────────────────┘
                         │
                         ▼
┌─────────────────────────────────────────────────────────────┐
│    Chunked File Writer (common/chunkedFileWriter.go)        │
│  - Existing sequential write logic (reused)                 │
│  - MD5 validation (reused)                                   │
└─────────────────────────────────────────────────────────────┘
```

### New Components Required

| Component | File | Purpose |
|-----------|------|---------|
| **HTTP Location** | `common/fe-ste-models.go` | Add `ELocation.Http()` |
| **HTTP Traverser** | `cmd/zc_traverser_http.go` | Enumerate HTTP sources |
| **HTTP Downloader** | `ste/downloader-http.go` | Perform range downloads |
| **HTTP Source Info** | `ste/sourceInfoProvider-http.go` | Provide HTTP metadata |
| **HTTP Credential** | `common/credentialFactory.go` | OAuth token management |
| **HTTP URL Parser** | `common/httpUrlParts.go` | Parse HTTP URLs |

### Integration with Existing Systems

**Reuse (No Changes Needed):**
- ✅ `common/chunkedFileWriter.go` - Sequential write logic
- ✅ `ste/xfer-remoteToLocal-file.go` - Download orchestration
- ✅ `ste/md5Comparer.go` - Hash validation
- ✅ Worker pool and concurrency management

**Extend (Minor Changes):**
- 🔄 `common/fe-ste-models.go` - Add `Location.Http()` and `FromTo.HttpLocal()`
- 🔄 `cmd/zc_enumerator.go` - Add HTTP case in `InitResourceTraverser()`
- 🔄 `ste/xfer.go` - Register HTTP→Local transfer function

**New Implementation:**
- ➕ All components listed in table above

---

## Implementation Phases

### Phase 1: Foundation (Weeks 1-2)

**Goal:** Establish core HTTP infrastructure

#### Tasks:
1. **Add HTTP Location Type**
   - File: `common/fe-ste-models.go`
   - Add `func (Location) Http() Location { return Location(11) }`
   - Add `func (FromTo) HttpLocal() FromTo { return FromToValue(ELocation.Http(), ELocation.Local()) }`

2. **Create HTTP URL Parser**
   - File: `common/httpUrlParts.go`
   - Parse HTTP/HTTPS URLs
   - Extract host, path, query parameters
   - Support URL normalization

3. **Implement HTTP Credential Provider**
   - File: `common/credentialFactory.go`
   - Add `ECredentialType.OAuthToken()` for generic OAuth
   - Support Bearer token from:
     - CLI flag: `--bearer-token`
     - Environment variable: `AZCOPY_HTTP_BEARER_TOKEN`
     - Token file: `--bearer-token-file`
   - Token refresh logic (if supported by server)

#### Deliverables:
- [ ] HTTP location enum added
- [ ] HTTP URL parsing tests passing
- [ ] Credential provider unit tests passing

#### Acceptance Criteria:
- Can parse HTTP URLs correctly
- Can store and retrieve OAuth tokens
- All unit tests pass

---

### Phase 2: HTTP Traverser (Weeks 3-4)

**Goal:** Implement HTTP source enumeration

#### Tasks:
1. **Create HTTP Traverser**
   - File: `cmd/zc_traverser_http.go`
   - Implement `ResourceTraverser` interface
   - `IsDirectory()` - Always false (HTTP endpoints are files)
   - `Traverse()` - Enumerate single file

2. **Implement Range Detection**
   - HEAD request to detect:
     - `Accept-Ranges: bytes`
     - `Content-Length`
     - `Content-Type`
     - `Content-MD5` (optional)
     - `ETag` (for consistency checks)
   - Store capabilities in traverser state

3. **Integrate with Enumerator**
   - File: `cmd/zc_enumerator.go`
   - Add HTTP case to `InitResourceTraverser()`:
     ```go
     case common.ELocation.Http():
         output, err = newHTTPTraverser(resource.ValueHTTP(), ctx, opts)
     ```

#### Deliverables:
- [ ] HTTP traverser implementation
- [ ] Range detection logic
- [ ] Integration tests with mock HTTP server

#### Acceptance Criteria:
- Can detect range support correctly
- Can enumerate single HTTP file
- Handles servers without range support gracefully

---

### Phase 3: HTTP Downloader (Weeks 5-7)

**Goal:** Implement segmented HTTP downloads

#### Tasks:

1. **Create HTTP Downloader Factory**
   - File: `ste/xfer.go`
   - Register downloader factory:
     ```go
     case common.EFromTo.HttpLocal():
         return remoteToLocal(jptm, p, newHTTPDownloader)
     ```

2. **Implement HTTP Downloader**
   - File: `ste/downloader-http.go`
   - Implement `downloader` interface:
     - `Prologue()` - Setup HTTP client, validate connectivity
     - `GenerateDownloadFunc()` - Create range download functions
     - `Epilogue()` - Cleanup

3. **Range Download Logic**
   ```go
   func (hd *httpDownloader) GenerateDownloadFunc(
       jptm IJobPartTransferMgr,
       destWriter common.ChunkedFileWriter,
       id common.ChunkID,
       length int64,
       pacer pacer,
   ) chunkFunc {
       return createDownloadChunkFunc(jptm, id, func() {
           // Create HTTP GET request with Range header
           req, err := http.NewRequestWithContext(
               jptm.Context(),
               "GET",
               hd.url,
               nil,
           )

           // Add Range header
           req.Header.Set("Range",
               fmt.Sprintf("bytes=%d-%d",
                   id.OffsetInFile(),
                   id.OffsetInFile()+length-1))

           // Add OAuth Bearer token
           if hd.bearerToken != "" {
               req.Header.Set("Authorization",
                   "Bearer " + hd.bearerToken)
           }

           // Execute request with retry
           resp, err := hd.executeWithRetry(req)
           if err != nil {
               jptm.FailActiveDownload("HTTP request", err)
               return
           }
           defer resp.Body.Close()

           // Validate response
           if resp.StatusCode != http.StatusPartialContent {
               jptm.FailActiveDownload("Unexpected status",
                   fmt.Errorf("expected 206, got %d", resp.StatusCode))
               return
           }

           // Enqueue chunk for writing
           err = destWriter.EnqueueChunk(
               jptm.Context(),
               id,
               length,
               resp.Body,
               true, // retryable via close/reopen
           )
       })
   }
   ```

4. **Implement Retry Logic**
   - Exponential backoff for:
     - `5xx` server errors
     - `429` rate limiting
     - Network timeouts
   - Respect `Retry-After` header
   - Max retries configurable (default: 5)

5. **Single-Threaded Fallback**
   ```go
   if !hd.supportsRange {
       // Download entire file in one chunk
       return hd.generateSingleChunkDownload(jptm, destWriter, pacer)
   }
   ```

6. **Implement Source Info Provider**
   - File: `ste/sourceInfoProvider-http.go`
   - Provide metadata from HEAD response:
     - Content-Length → SourceSize
     - Content-MD5 → SrcHTTPHeaders.ContentMD5
     - ETag → VersionID (for consistency)
     - Last-Modified → LastModifiedTime

#### Deliverables:
- [ ] HTTP downloader implementation
- [ ] Retry logic with exponential backoff
- [ ] Single-threaded fallback
- [ ] Source info provider

#### Acceptance Criteria:
- Can download files with range support
- Falls back to single-threaded correctly
- Retries transient failures
- Respects rate limiting

---

### Phase 4: CLI Integration (Week 8)

**Goal:** Expose HTTP download functionality via CLI

#### Tasks:

1. **Add CLI Flags**
   - File: `cmd/copy.go`
   ```go
   // HTTP authentication
   copyCmd.PersistentFlags().String("bearer-token", "",
       "OAuth Bearer token for HTTP authentication")
   copyCmd.PersistentFlags().String("bearer-token-file", "",
       "File containing OAuth Bearer token")
   copyCmd.PersistentFlags().StringToString("http-headers", nil,
       "Custom HTTP headers (key=value)")

   // HTTP options
   copyCmd.PersistentFlags().Bool("http-allow-insecure", false,
       "Allow insecure HTTPS connections (skip cert validation)")
   copyCmd.PersistentFlags().Duration("http-timeout", 30*time.Second,
       "HTTP request timeout")
   ```

2. **Update Location Detection**
   - File: `cmd/copy.go`
   - Detect HTTP URLs:
     ```go
     func inferArgumentLocation(arg string) Location {
         if strings.HasPrefix(strings.ToLower(arg), "http://") ||
            strings.HasPrefix(strings.ToLower(arg), "https://") {
             return ELocation.Http()
         }
         // ... existing logic
     }
     ```

3. **Credential Parsing**
   - Parse `--bearer-token` flag
   - Read from `--bearer-token-file` if specified
   - Store in `CredentialInfo.OAuthTokenInfo`

4. **Validation**
   - Ensure HTTP→Local only (no HTTP→HTTP, HTTP→Azure yet)
   - Validate URL format
   - Validate authentication provided (if required)

#### Deliverables:
- [ ] CLI flags added
- [ ] Location detection updated
- [ ] End-to-end CLI test

#### Acceptance Criteria:
- Can invoke: `azcopy copy "https://..." "/local" --bearer-token="..."`
- Proper error messages for invalid flags
- Help text updated

---

### Phase 5: Testing and Validation (Weeks 9-10)

**Goal:** Comprehensive testing across scenarios

#### Test Categories:

1. **Unit Tests**
   - HTTP URL parsing
   - Credential management
   - Range detection logic
   - Retry logic with mock server

2. **Integration Tests**
   - Download with range support
   - Download without range support (fallback)
   - OAuth authentication
   - Custom headers
   - Retry on failures
   - Cancellation during download

3. **E2E Tests**
   - Real HTTP server (nginx, Apache)
   - OAuth server (mock OAuth provider)
   - Large file downloads (>1GB)
   - Slow network simulation
   - Connection interruption

4. **Performance Tests**
   - Benchmark segmented vs single-threaded
   - Measure overhead of HTTP vs Azure Blob
   - Test with various chunk sizes

5. **Security Tests**
   - Token leakage in logs
   - HTTPS certificate validation
   - Insecure connection handling

#### Test Servers:

**Setup Mock HTTP Server:**
```go
// e2etest/http_server_test.go
func startMockHTTPServer() *httptest.Server {
    return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        // Validate Bearer token
        auth := r.Header.Get("Authorization")
        if !strings.HasPrefix(auth, "Bearer ") {
            w.WriteHeader(http.StatusUnauthorized)
            return
        }

        // Support range requests
        w.Header().Set("Accept-Ranges", "bytes")
        w.Header().Set("Content-Length", fmt.Sprintf("%d", len(testData)))

        rangeHeader := r.Header.Get("Range")
        if rangeHeader != "" {
            // Parse range and serve partial content
            // ...
            w.WriteHeader(http.StatusPartialContent)
        } else {
            w.WriteHeader(http.StatusOK)
        }

        w.Write(testData)
    }))
}
```

#### Deliverables:
- [ ] 50+ unit tests
- [ ] 20+ integration tests
- [ ] 10+ E2E tests
- [ ] Performance benchmarks

#### Acceptance Criteria:
- >90% code coverage
- All tests pass consistently
- Performance within 10% of Azure Blob downloads

---

### Phase 6: Documentation and Polish (Weeks 11-12)

**Goal:** Production-ready release

#### Tasks:

1. **User Documentation**
   - Update `README.md`
   - Add HTTP download examples
   - Document OAuth setup
   - Troubleshooting guide

2. **API Documentation**
   - GoDoc comments for all public APIs
   - Architecture diagrams
   - Sequence diagrams

3. **Error Messages**
   - User-friendly error messages
   - Actionable suggestions
   - Link to documentation

4. **Logging**
   - Structured logging for HTTP requests
   - Debug mode for troubleshooting
   - Redact sensitive tokens

5. **Performance Tuning**
   - Optimize chunk size for HTTP
   - Connection pooling
   - HTTP/2 support

6. **Security Audit**
   - Review token handling
   - HTTPS certificate validation
   - Input validation

#### Deliverables:
- [ ] User guide published
- [ ] API documentation complete
- [ ] Security audit passed
- [ ] Performance optimized

#### Acceptance Criteria:
- Documentation clear and comprehensive
- No security vulnerabilities
- Performance meets SLA

---

## Detailed Component Design

### 1. HTTP Location Enum

**File:** `common/fe-ste-models.go`

```go
// Add to Location enum
func (Location) Http() Location { return Location(11) }

// Add to FromTo combinations
func (FromTo) HttpLocal() FromTo {
    return FromToValue(ELocation.Http(), ELocation.Local())
}

// Add to Location.String()
func (l Location) String() string {
    switch l {
    // ... existing cases
    case l.Http():
        return "Http"
    }
}

// Add IsRemote check
func (l Location) IsRemote() bool {
    return l == l.Blob() || l == l.File() || l == l.BlobFS() ||
           l == l.S3() || l == l.GCP() || l == l.Http()
}
```

---

### 2. HTTP URL Parser

**File:** `common/httpUrlParts.go`

```go
package common

import (
    "fmt"
    "net/url"
    "strings"
)

// HTTPURLParts represents a parsed HTTP URL
type HTTPURLParts struct {
    Scheme   string // "http" or "https"
    Host     string // "api.example.com"
    Port     string // "8080" or ""
    Path     string // "/files/data.bin"
    Query    string // "version=2&format=json"
    Fragment string // "section1"
    URL      string // Full URL
}

// NewHTTPURLParts parses an HTTP URL
func NewHTTPURLParts(rawURL string) (HTTPURLParts, error) {
    parsed, err := url.Parse(rawURL)
    if err != nil {
        return HTTPURLParts{}, fmt.Errorf("invalid HTTP URL: %w", err)
    }

    if parsed.Scheme != "http" && parsed.Scheme != "https" {
        return HTTPURLParts{}, fmt.Errorf("expected http or https, got: %s", parsed.Scheme)
    }

    return HTTPURLParts{
        Scheme:   parsed.Scheme,
        Host:     parsed.Hostname(),
        Port:     parsed.Port(),
        Path:     parsed.Path,
        Query:    parsed.RawQuery,
        Fragment: parsed.Fragment,
        URL:      rawURL,
    }, nil
}

// String reconstructs the URL
func (h HTTPURLParts) String() string {
    return h.URL
}

// IsSecure returns true if HTTPS
func (h HTTPURLParts) IsSecure() bool {
    return h.Scheme == "https"
}
```

---

### 3. HTTP Traverser

**File:** `cmd/zc_traverser_http.go`

```go
package cmd

import (
    "context"
    "fmt"
    "net/http"
    "strconv"
    "time"

    "github.com/Azure/azure-storage-azcopy/v10/common"
)

// httpTraverser enumerates a single HTTP file
type httpTraverser struct {
    rawURL              string
    ctx                 context.Context
    httpClient          *http.Client
    bearerToken         string
    customHeaders       map[string]string

    // Capabilities detected via HEAD request
    supportsRange       bool
    contentLength       int64
    contentMD5          []byte
    etag                string
    lastModified        time.Time

    incrementEnumerationCounter func(common.EntityType)
}

func newHTTPTraverser(rawURL string, ctx context.Context, opts InitResourceTraverserOptions) (*httpTraverser, error) {
    // Parse URL
    urlParts, err := common.NewHTTPURLParts(rawURL)
    if err != nil {
        return nil, err
    }

    // Create HTTP client with timeout
    client := &http.Client{
        Timeout: 30 * time.Second,
        Transport: &http.Transport{
            MaxIdleConns:        100,
            MaxIdleConnsPerHost: 100,
            IdleConnTimeout:     90 * time.Second,
        },
    }

    // Extract credentials
    bearerToken := ""
    if opts.Credential != nil && opts.Credential.OAuthTokenInfo.Token != "" {
        bearerToken = opts.Credential.OAuthTokenInfo.Token
    }

    t := &httpTraverser{
        rawURL:                      rawURL,
        ctx:                         ctx,
        httpClient:                  client,
        bearerToken:                 bearerToken,
        customHeaders:               opts.HTTPHeaders,
        incrementEnumerationCounter: opts.IncrementEnumeration,
    }

    // Perform HEAD request to detect capabilities
    if err := t.detectCapabilities(); err != nil {
        return nil, fmt.Errorf("failed to detect HTTP capabilities: %w", err)
    }

    return t, nil
}

// IsDirectory always returns false (HTTP endpoints are files)
func (t *httpTraverser) IsDirectory(isSource bool) (bool, error) {
    return false, nil
}

// Traverse enumerates the single HTTP file
func (t *httpTraverser) Traverse(
    preprocessor objectMorpher,
    processor objectProcessor,
    filters []ObjectFilter,
) error {
    // Create StoredObject representing this HTTP file
    object := StoredObject{
        name:             t.getFileName(),
        entityType:       common.EEntityType.File(),
        lastModifiedTime: t.lastModified,
        size:             t.contentLength,
        md5:              t.contentMD5,
        contentType:      "", // Could extract from HEAD response
        metadata:         common.Metadata{},
    }

    // Apply preprocessor
    if preprocessor != nil {
        object = preprocessor(object)
    }

    // Apply filters
    for _, filter := range filters {
        if !filter.DoesSupportThisOS() || filter.DoesNotAlreadyExistInDestination() {
            continue
        }
        if !filter.DoesPass(object) {
            glcm.Info("Skipping " + object.name + " due to filter")
            return nil
        }
    }

    // Process the object
    err := processor(object)
    if err != nil {
        return fmt.Errorf("failed to process HTTP file: %w", err)
    }

    t.incrementEnumerationCounter(common.EEntityType.File())

    return nil
}

// detectCapabilities performs HEAD request to detect server capabilities
func (t *httpTraverser) detectCapabilities() error {
    req, err := http.NewRequestWithContext(t.ctx, "HEAD", t.rawURL, nil)
    if err != nil {
        return err
    }

    // Add authentication
    if t.bearerToken != "" {
        req.Header.Set("Authorization", "Bearer "+t.bearerToken)
    }

    // Add custom headers
    for k, v := range t.customHeaders {
        req.Header.Set(k, v)
    }

    resp, err := t.httpClient.Do(req)
    if err != nil {
        return fmt.Errorf("HEAD request failed: %w", err)
    }
    defer resp.Body.Close()

    if resp.StatusCode != http.StatusOK {
        return fmt.Errorf("HEAD request returned %d", resp.StatusCode)
    }

    // Detect range support
    acceptRanges := resp.Header.Get("Accept-Ranges")
    t.supportsRange = (acceptRanges == "bytes")

    // Parse Content-Length
    contentLength := resp.Header.Get("Content-Length")
    if contentLength != "" {
        t.contentLength, _ = strconv.ParseInt(contentLength, 10, 64)
    }

    // Parse Content-MD5 (optional)
    contentMD5 := resp.Header.Get("Content-MD5")
    if contentMD5 != "" {
        // Base64 decode
        t.contentMD5 = []byte(contentMD5) // TODO: proper base64 decode
    }

    // Parse ETag
    t.etag = resp.Header.Get("ETag")

    // Parse Last-Modified
    lastMod := resp.Header.Get("Last-Modified")
    if lastMod != "" {
        t.lastModified, _ = http.ParseTime(lastMod)
    }

    return nil
}

// getFileName extracts filename from URL
func (t *httpTraverser) getFileName() string {
    urlParts, _ := common.NewHTTPURLParts(t.rawURL)
    // Extract last segment of path
    parts := strings.Split(strings.TrimSuffix(urlParts.Path, "/"), "/")
    if len(parts) > 0 && parts[len(parts)-1] != "" {
        return parts[len(parts)-1]
    }
    return "downloaded_file"
}
```

---

### 4. HTTP Downloader

**File:** `ste/downloader-http.go`

```go
package ste

import (
    "context"
    "fmt"
    "io"
    "net/http"
    "time"

    "github.com/Azure/azure-storage-azcopy/v10/common"
)

// httpDownloader implements downloader interface for HTTP sources
type httpDownloader struct {
    url            string
    httpClient     *http.Client
    bearerToken    string
    customHeaders  map[string]string
    supportsRange  bool
    contentLength  int64
    etag           string

    jptm           IJobPartTransferMgr
    txInfo         *TransferInfo
}

func newHTTPDownloader(jptm IJobPartTransferMgr) (downloader, error) {
    info := jptm.Info()

    // Create HTTP client with configured timeout
    client := &http.Client{
        Timeout: 0, // No timeout on client, use context timeouts instead
        Transport: &http.Transport{
            MaxIdleConns:        100,
            MaxIdleConnsPerHost: 100,
            IdleConnTimeout:     90 * time.Second,
            // Enable HTTP/2
            ForceAttemptHTTP2: true,
        },
    }

    // Get bearer token from credential info
    bearerToken := ""
    cred := jptm.CredentialInfo()
    if cred.OAuthTokenInfo.Token != "" {
        bearerToken = cred.OAuthTokenInfo.Token
    }

    return &httpDownloader{
        url:           info.Source,
        httpClient:    client,
        bearerToken:   bearerToken,
        customHeaders: info.HTTPHeaders,
        contentLength: int64(info.SourceSize),
    }, nil
}

func (hd *httpDownloader) Prologue(jptm IJobPartTransferMgr) {
    hd.txInfo = jptm.Info()
    hd.jptm = jptm

    // Perform HEAD request to validate and detect capabilities
    req, err := http.NewRequestWithContext(jptm.Context(), "HEAD", hd.url, nil)
    if err != nil {
        jptm.FailActiveDownload("Creating HEAD request", err)
        return
    }

    hd.addAuthHeaders(req)

    resp, err := hd.httpClient.Do(req)
    if err != nil {
        jptm.FailActiveDownload("HEAD request failed", err)
        return
    }
    defer resp.Body.Close()

    if resp.StatusCode != http.StatusOK {
        jptm.FailActiveDownload("HEAD request",
            fmt.Errorf("status %d", resp.StatusCode))
        return
    }

    // Detect range support
    acceptRanges := resp.Header.Get("Accept-Ranges")
    hd.supportsRange = (acceptRanges == "bytes")

    // Get ETag for consistency checks
    hd.etag = resp.Header.Get("ETag")

    if !hd.supportsRange {
        jptm.LogAtLevelForCurrentTransfer(common.LogWarning,
            "Server does not support range requests, using single-threaded download")
    }
}

func (hd *httpDownloader) GenerateDownloadFunc(
    jptm IJobPartTransferMgr,
    destWriter common.ChunkedFileWriter,
    id common.ChunkID,
    length int64,
    pacer pacer,
) chunkFunc {
    // If server doesn't support ranges and this isn't the first chunk, skip
    if !hd.supportsRange && id.OffsetInFile() > 0 {
        // This should never happen as we should only schedule 1 chunk
        return createDownloadChunkFunc(jptm, id, func() {
            jptm.FailActiveDownload("Range request",
                fmt.Errorf("server doesn't support ranges"))
        })
    }

    return createDownloadChunkFunc(jptm, id, func() {
        // Create GET request
        req, err := http.NewRequestWithContext(jptm.Context(), "GET", hd.url, nil)
        if err != nil {
            jptm.FailActiveDownload("Creating request", err)
            return
        }

        // Add Range header for segmented download
        if hd.supportsRange {
            req.Header.Set("Range",
                fmt.Sprintf("bytes=%d-%d",
                    id.OffsetInFile(),
                    id.OffsetInFile()+length-1))
        }

        // Add authentication and custom headers
        hd.addAuthHeaders(req)

        // Add ETag for consistency (If-Match)
        if hd.etag != "" {
            req.Header.Set("If-Match", hd.etag)
        }

        // Execute request with retry
        jptm.LogChunkStatus(id, common.EWaitReason.HeaderResponse())
        resp, err := hd.executeWithRetry(req, jptm)
        if err != nil {
            jptm.FailActiveDownload("HTTP request", err)
            return
        }
        defer resp.Body.Close()

        // Validate response status
        expectedStatus := http.StatusOK
        if hd.supportsRange {
            expectedStatus = http.StatusPartialContent
        }
        if resp.StatusCode != expectedStatus {
            jptm.FailActiveDownload("Unexpected status",
                fmt.Errorf("expected %d, got %d", expectedStatus, resp.StatusCode))
            return
        }

        // Enqueue chunk for writing
        jptm.LogChunkStatus(id, common.EWaitReason.Body())

        // Wrap body with pacing and retry capabilities
        retryableBody := &httpRetryBody{
            downloader: hd,
            request:    req,
            response:   resp,
            jptm:       jptm,
            pacer:      pacer,
        }

        err = destWriter.EnqueueChunk(
            jptm.Context(),
            id,
            length,
            newPacedResponseBody(jptm.Context(), retryableBody, pacer),
            true, // retryable
        )
        if err != nil {
            jptm.FailActiveDownload("Enqueuing chunk", err)
            return
        }
    })
}

func (hd *httpDownloader) Epilogue() {
    // Cleanup if needed
}

// addAuthHeaders adds authentication and custom headers to request
func (hd *httpDownloader) addAuthHeaders(req *http.Request) {
    // Add Bearer token
    if hd.bearerToken != "" {
        req.Header.Set("Authorization", "Bearer "+hd.bearerToken)
    }

    // Add custom headers
    for k, v := range hd.customHeaders {
        req.Header.Set(k, v)
    }
}

// executeWithRetry executes HTTP request with exponential backoff
func (hd *httpDownloader) executeWithRetry(
    req *http.Request,
    jptm IJobPartTransferMgr,
) (*http.Response, error) {
    const maxRetries = 5
    const initialBackoff = 1 * time.Second

    var resp *http.Response
    var err error

    for attempt := 0; attempt <= maxRetries; attempt++ {
        if attempt > 0 {
            // Exponential backoff
            backoff := initialBackoff * (1 << uint(attempt-1))
            jptm.LogAtLevelForCurrentTransfer(common.LogInfo,
                fmt.Sprintf("Retry attempt %d after %v", attempt, backoff))

            select {
            case <-time.After(backoff):
            case <-jptm.Context().Done():
                return nil, jptm.Context().Err()
            }
        }

        resp, err = hd.httpClient.Do(req)
        if err != nil {
            // Network error, retry
            continue
        }

        // Check for retryable status codes
        if resp.StatusCode == http.StatusTooManyRequests ||
           resp.StatusCode >= 500 {
            // Respect Retry-After header
            if retryAfter := resp.Header.Get("Retry-After"); retryAfter != "" {
                // Parse and wait
                // TODO: implement Retry-After parsing
            }
            resp.Body.Close()
            continue
        }

        // Success or non-retryable error
        return resp, nil
    }

    return resp, fmt.Errorf("max retries exceeded: %w", err)
}

// httpRetryBody wraps HTTP response body with retry capability
type httpRetryBody struct {
    downloader *httpDownloader
    request    *http.Request
    response   *http.Response
    jptm       IJobPartTransferMgr
    pacer      pacer
}

func (hrb *httpRetryBody) Read(p []byte) (n int, err error) {
    return hrb.response.Body.Read(p)
}

func (hrb *httpRetryBody) Close() error {
    // Force retry by closing and reopening
    if hrb.response != nil {
        hrb.response.Body.Close()
    }

    // Re-execute request
    resp, err := hrb.downloader.executeWithRetry(hrb.request, hrb.jptm)
    if err != nil {
        return err
    }
    hrb.response = resp

    return nil
}
```

---

### 5. HTTP Source Info Provider

**File:** `ste/sourceInfoProvider-http.go`

```go
package ste

import (
    "context"
    "fmt"
    "net/http"

    "github.com/Azure/azure-storage-azcopy/v10/common"
)

// httpSourceInfoProvider provides metadata for HTTP sources
type httpSourceInfoProvider struct {
    defaultRemoteSourceInfoProvider
    url        string
    httpClient *http.Client
    bearerToken string
    ctx        context.Context
}

func newHTTPSourceInfoProvider(jptm IJobPartTransferMgr) (ISourceInfoProvider, error) {
    base, err := newDefaultRemoteSourceInfoProvider(jptm)
    if err != nil {
        return nil, err
    }

    client := &http.Client{
        Timeout: 30 * time.Second,
    }

    bearerToken := ""
    cred := jptm.CredentialInfo()
    if cred.OAuthTokenInfo.Token != "" {
        bearerToken = cred.OAuthTokenInfo.Token
    }

    return &httpSourceInfoProvider{
        defaultRemoteSourceInfoProvider: *base,
        url:        jptm.Info().Source,
        httpClient: client,
        bearerToken: bearerToken,
        ctx:        jptm.Context(),
    }, nil
}

func (p *httpSourceInfoProvider) PreSignedSourceURL() (string, error) {
    // HTTP URLs are already "pre-signed" (include auth in headers)
    return p.url, nil
}

func (p *httpSourceInfoProvider) RawSource() string {
    return p.url
}

// GetMD5 is not supported for HTTP (no range MD5)
func (p *httpSourceInfoProvider) GetMD5(offset, count int64) ([]byte, error) {
    return nil, fmt.Errorf("per-range MD5 not supported for HTTP sources")
}
```

---

## Integration Points

### 1. Copy Command Integration

**File:** `cmd/copy.go`

```go
// Add to inferArgumentLocation()
func inferArgumentLocation(arg string) Location {
    argLower := strings.ToLower(arg)

    // Check for HTTP/HTTPS
    if strings.HasPrefix(argLower, "http://") ||
       strings.HasPrefix(argLower, "https://") {
        return ELocation.Http()
    }

    // ... existing logic
}

// Add to initEnumerator() credential setup
if cca.FromTo.From() == common.ELocation.Http() {
    // Parse HTTP authentication
    bearerToken := cca.bearerToken
    if cca.bearerTokenFile != "" {
        // Read from file
        tokenBytes, err := os.ReadFile(cca.bearerTokenFile)
        if err != nil {
            return nil, fmt.Errorf("failed to read bearer token file: %w", err)
        }
        bearerToken = strings.TrimSpace(string(tokenBytes))
    }

    srcCredInfo = common.CredentialInfo{
        CredentialType: common.ECredentialType.OAuthToken(),
        OAuthTokenInfo: common.OAuthTokenInfo{
            Token: bearerToken,
        },
    }
}
```

---

### 2. Transfer Function Registration

**File:** `ste/xfer.go`

```go
func init() {
    // ... existing registrations

    // Register HTTP→Local transfer
    registerXferFunc(
        common.EFromTo.HttpLocal(),
        func(jptm IJobPartTransferMgr, p pacer, sender func(IJobPartTransferMgr) (sender, error)) {
            // Use remoteToLocal with HTTP downloader
            remoteToLocal(jptm, p, newHTTPDownloader)
        },
    )
}
```

---

### 3. Credential Factory Extension

**File:** `common/credentialFactory.go`

```go
// Add to CredentialType enum
const (
    // ... existing types
    ECredentialType_OAuthToken() = CredentialType(10)
)

// Add to CredentialInfo struct
type CredentialInfo struct {
    CredentialType CredentialType

    // ... existing fields

    // Generic OAuth token info
    OAuthTokenInfo OAuthTokenInfo
}

type OAuthTokenInfo struct {
    Token        string // Bearer token
    RefreshToken string // Refresh token (future)
    ExpiresAt    time.Time // Token expiration
}
```

---

## Testing Strategy

### Unit Tests

```go
// ste/downloader-http_test.go
func TestHTTPDownloader_RangeSupport(t *testing.T) {
    // Setup mock HTTP server with range support
    server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        w.Header().Set("Accept-Ranges", "bytes")
        w.Header().Set("Content-Length", "1000")

        rangeHeader := r.Header.Get("Range")
        if rangeHeader != "" {
            w.WriteHeader(http.StatusPartialContent)
        } else {
            w.WriteHeader(http.StatusOK)
        }
        w.Write([]byte("test data"))
    }))
    defer server.Close()

    // Create mock JPTM
    jptm := &mockJobPartTransferMgr{
        info: &TransferInfo{
            Source: server.URL,
            SourceSize: 1000,
        },
    }

    // Create downloader
    dl, err := newHTTPDownloader(jptm)
    assert.NoError(t, err)

    // Test prologue
    dl.Prologue(jptm)

    httpDL := dl.(*httpDownloader)
    assert.True(t, httpDL.supportsRange, "Should detect range support")
}

func TestHTTPDownloader_NoRangeSupport(t *testing.T) {
    // Setup mock HTTP server without range support
    server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        w.Header().Set("Accept-Ranges", "none")
        w.WriteHeader(http.StatusOK)
        w.Write([]byte("test data"))
    }))
    defer server.Close()

    jptm := &mockJobPartTransferMgr{
        info: &TransferInfo{
            Source: server.URL,
            SourceSize: 100,
        },
    }

    dl, err := newHTTPDownloader(jptm)
    assert.NoError(t, err)

    dl.Prologue(jptm)

    httpDL := dl.(*httpDownloader)
    assert.False(t, httpDL.supportsRange, "Should not detect range support")
}

func TestHTTPDownloader_OAuthAuthentication(t *testing.T) {
    const validToken = "valid-bearer-token"

    server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        auth := r.Header.Get("Authorization")
        if auth != "Bearer "+validToken {
            w.WriteHeader(http.StatusUnauthorized)
            return
        }
        w.WriteHeader(http.StatusOK)
        w.Write([]byte("authenticated"))
    }))
    defer server.Close()

    jptm := &mockJobPartTransferMgr{
        info: &TransferInfo{
            Source: server.URL,
        },
        credInfo: common.CredentialInfo{
            CredentialType: common.ECredentialType.OAuthToken(),
            OAuthTokenInfo: common.OAuthTokenInfo{
                Token: validToken,
            },
        },
    }

    dl, err := newHTTPDownloader(jptm)
    assert.NoError(t, err)

    dl.Prologue(jptm)
    // Should succeed without authentication error
}

func TestHTTPDownloader_RetryLogic(t *testing.T) {
    attempts := 0
    server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        attempts++
        if attempts < 3 {
            w.WriteHeader(http.StatusInternalServerError)
        } else {
            w.WriteHeader(http.StatusOK)
            w.Write([]byte("success"))
        }
    }))
    defer server.Close()

    // Test that downloader retries and eventually succeeds
    // ...
}
```

---

### Integration Tests

```go
// e2etest/zt_http_download_test.go
func TestHTTPDownload_LargeFile(t *testing.T) {
    // Create 100MB test file
    testData := make([]byte, 100*1024*1024)
    rand.Read(testData)

    // Setup HTTP server
    server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        w.Header().Set("Accept-Ranges", "bytes")
        w.Header().Set("Content-Length", fmt.Sprintf("%d", len(testData)))

        rangeHeader := r.Header.Get("Range")
        if rangeHeader != "" {
            // Parse and serve range
            // ...
            w.WriteHeader(http.StatusPartialContent)
        } else {
            w.WriteHeader(http.StatusOK)
        }
        w.Write(testData)
    }))
    defer server.Close()

    // Create temp destination
    tempDir := t.TempDir()
    destPath := filepath.Join(tempDir, "downloaded.bin")

    // Execute azcopy copy
    cmd := exec.Command("azcopy", "copy", server.URL, destPath)
    output, err := cmd.CombinedOutput()
    assert.NoError(t, err, string(output))

    // Verify file
    downloadedData, err := os.ReadFile(destPath)
    assert.NoError(t, err)
    assert.Equal(t, testData, downloadedData, "Downloaded data should match")
}
```

---

## Performance Considerations

### 1. Chunk Size Optimization

**Strategy:**
- Default chunk size: 8MB (same as Azure Blob)
- Auto-tune based on network conditions
- Configurable via `--block-size-mb` flag

**Benchmark:**
```go
func BenchmarkHTTPDownload_ChunkSize(b *testing.B) {
    chunkSizes := []int64{1*MB, 4*MB, 8*MB, 16*MB, 32*MB}

    for _, size := range chunkSizes {
        b.Run(fmt.Sprintf("Chunk%dMB", size/MB), func(b *testing.B) {
            // Benchmark download with specific chunk size
        })
    }
}
```

---

### 2. Connection Pooling

**Implementation:**
```go
// Use HTTP/2 with connection reuse
client := &http.Client{
    Transport: &http.Transport{
        MaxIdleConns:        100,
        MaxIdleConnsPerHost: 100,
        IdleConnTimeout:     90 * time.Second,
        ForceAttemptHTTP2:   true, // Enable HTTP/2
    },
}
```

---

### 3. Memory Management

**Reuse existing:**
- Byte slice pool (`ByteSlicePooler`)
- Cache limiter for RAM management
- Sequential write to avoid excessive buffering

---

## Security Considerations

### 1. Token Handling

**Requirements:**
- ✅ Never log bearer tokens
- ✅ Redact tokens in error messages
- ✅ Clear tokens from memory after use
- ✅ Validate token format before use

**Implementation:**
```go
func (hd *httpDownloader) addAuthHeaders(req *http.Request) {
    if hd.bearerToken != "" {
        // Never log the actual token
        if hd.jptm.ShouldLog(common.LogDebug) {
            hd.jptm.Log(common.LogDebug, "Adding Bearer token authentication")
        }
        req.Header.Set("Authorization", "Bearer "+hd.bearerToken)
    }
}

// Redact tokens in logs
func redactToken(msg string) string {
    // Replace any token-like strings with [REDACTED]
    return tokenRegex.ReplaceAllString(msg, "[REDACTED]")
}
```

---

### 2. HTTPS Certificate Validation

**Default:** Always validate certificates

**Option:** Allow insecure connections (for testing only)

```go
if allowInsecure {
    client.Transport.(*http.Transport).TLSClientConfig = &tls.Config{
        InsecureSkipVerify: true,
    }
    jptm.Log(common.LogWarning, "INSECURE: Skipping HTTPS certificate validation")
}
```

---

### 3. Input Validation

**Validate:**
- URL format and scheme
- Token format (if applicable)
- Custom headers (no injection)

```go
func validateHTTPURL(rawURL string) error {
    parsed, err := url.Parse(rawURL)
    if err != nil {
        return fmt.Errorf("invalid URL: %w", err)
    }

    if parsed.Scheme != "http" && parsed.Scheme != "https" {
        return fmt.Errorf("only HTTP/HTTPS supported, got: %s", parsed.Scheme)
    }

    return nil
}
```

---

## Migration and Compatibility

### Backward Compatibility

**No Breaking Changes:**
- Existing AzCopy commands unchanged
- New HTTP support is additive
- Existing tests continue to pass

### Feature Flags

**Environment Variables:**
```bash
# Enable HTTP download support (future: may start as opt-in)
export AZCOPY_HTTP_DOWNLOAD_ENABLED=true

# Set default HTTP timeout
export AZCOPY_HTTP_TIMEOUT=60s

# Max retries for HTTP requests
export AZCOPY_HTTP_MAX_RETRIES=5
```

---

## Open Questions and Risks

### Open Questions

1. **Should HTTP→Azure uploads be supported?**
   - Scope: Currently only HTTP→Local
   - Future: Could enable HTTP→Blob for data import

2. **How to handle dynamic content?**
   - Problem: Content-Length may change between HEAD and GET
   - Solution: Use ETag for consistency, fail if mismatch

3. **Support for authentication schemes beyond OAuth?**
   - Basic Auth
   - API Key in headers
   - Custom auth plugins

4. **How to handle redirects?**
   - Follow redirects automatically?
   - Preserve auth headers on redirects?
   - Security implications?

5. **Support for HTTP/3 (QUIC)?**
   - When available in Go standard library

### Risks

| Risk | Impact | Mitigation |
|------|--------|------------|
| **Server Incompatibility** | High | Detect capabilities via HEAD, graceful fallback |
| **Performance Regression** | Medium | Benchmark against Azure Blob, optimize chunk size |
| **Security Vulnerabilities** | High | Security audit, token handling best practices |
| **Integration Complexity** | Medium | Incremental implementation, extensive testing |
| **Maintenance Burden** | Low | Reuse existing infrastructure, minimize new code |

---

## Timeline and Milestones

### Phase 1: Foundation (Weeks 1-2)
- [ ] Add HTTP location enum
- [ ] Implement HTTP URL parser
- [ ] Create credential provider
- **Milestone:** Can parse HTTP URLs and manage OAuth tokens

### Phase 2: Traverser (Weeks 3-4)
- [ ] Implement HTTP traverser
- [ ] Range detection logic
- [ ] Integration with enumerator
- **Milestone:** Can enumerate HTTP files and detect capabilities

### Phase 3: Downloader (Weeks 5-7)
- [ ] Implement HTTP downloader
- [ ] Range download logic
- [ ] Retry and fallback mechanisms
- [ ] Source info provider
- **Milestone:** Can download files with OAuth authentication

### Phase 4: CLI (Week 8)
- [ ] Add CLI flags
- [ ] Update location detection
- [ ] Credential parsing
- **Milestone:** End-to-end CLI functionality

### Phase 5: Testing (Weeks 9-10)
- [ ] Unit tests (50+)
- [ ] Integration tests (20+)
- [ ] E2E tests (10+)
- [ ] Performance benchmarks
- **Milestone:** >90% code coverage, all tests passing

### Phase 6: Documentation (Weeks 11-12)
- [ ] User documentation
- [ ] API documentation
- [ ] Security audit
- [ ] Performance tuning
- **Milestone:** Production-ready release

---

## Success Criteria

### Functional Requirements
- ✅ Can download files from HTTP/HTTPS endpoints
- ✅ Supports OAuth Bearer token authentication
- ✅ Utilizes range requests for segmented downloads
- ✅ Falls back to single-threaded when no range support
- ✅ Validates downloads via MD5 (if provided by server)
- ✅ Retries transient failures automatically

### Non-Functional Requirements
- ✅ Performance within 10% of Azure Blob downloads
- ✅ >90% unit test coverage
- ✅ No security vulnerabilities
- ✅ Backward compatible with existing AzCopy features
- ✅ Clear documentation and examples

### User Experience
- ✅ Simple CLI: `azcopy copy "https://..." "/local" --bearer-token="..."`
- ✅ Informative error messages
- ✅ Progress reporting (reuse existing)
- ✅ Cancellation support

---

## Example Usage

### Basic Download
```bash
# Download single file with OAuth
azcopy copy \
  "https://api.example.com/files/dataset.tar.gz" \
  "/local/downloads/" \
  --bearer-token="eyJhbGciOiJSUzI1NiIsInR5cCI..."
```

### With Token File
```bash
# Store token in file (more secure)
echo "eyJhbGciOiJSUzI1NiIsInR5cCI..." > /secure/token.txt
chmod 600 /secure/token.txt

azcopy copy \
  "https://api.example.com/files/dataset.tar.gz" \
  "/local/downloads/" \
  --bearer-token-file="/secure/token.txt"
```

### Custom Headers
```bash
# Add custom headers
azcopy copy \
  "https://api.example.com/files/dataset.tar.gz" \
  "/local/downloads/" \
  --bearer-token-file="/secure/token.txt" \
  --http-headers="X-API-Version=2.0,X-Request-ID=12345"
```

### Insecure Connection (Testing Only)
```bash
# Skip HTTPS certificate validation (NOT RECOMMENDED)
azcopy copy \
  "https://localhost:8443/files/test.bin" \
  "/local/downloads/" \
  --http-allow-insecure
```

---

## References

- [AzCopy Architecture](./readme.md)
- [Segmented Download Design](./segmented-download-design.md)
- [HTTP/1.1 Range Requests (RFC 7233)](https://tools.ietf.org/html/rfc7233)
- [OAuth 2.0 Bearer Token (RFC 6750)](https://tools.ietf.org/html/rfc6750)
- [HTTP/2 Specification (RFC 7540)](https://tools.ietf.org/html/rfc7540)

---

## Appendix: Code Skeleton

### Complete Downloader Implementation

See inline code examples in [Detailed Component Design](#detailed-component-design) section.

### CLI Flag Definitions

```go
// cmd/copy.go
var (
    httpBearerToken     string
    httpBearerTokenFile string
    httpHeaders         map[string]string
    httpAllowInsecure   bool
    httpTimeout         time.Duration
)

func init() {
    copyCmd.PersistentFlags().StringVar(&httpBearerToken, "bearer-token", "",
        "OAuth Bearer token for HTTP authentication")
    copyCmd.PersistentFlags().StringVar(&httpBearerTokenFile, "bearer-token-file", "",
        "File containing OAuth Bearer token")
    copyCmd.PersistentFlags().StringToStringVar(&httpHeaders, "http-headers", nil,
        "Custom HTTP headers (key=value)")
    copyCmd.PersistentFlags().BoolVar(&httpAllowInsecure, "http-allow-insecure", false,
        "Allow insecure HTTPS connections (skip certificate validation)")
    copyCmd.PersistentFlags().DurationVar(&httpTimeout, "http-timeout", 30*time.Second,
        "HTTP request timeout")
}
```

---

**Document Version:** 1.0
**Last Updated:** 2025-09-29
**Author:** AzCopy Team
**Status:** Draft - Ready for Review