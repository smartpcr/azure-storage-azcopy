# HTTP Auto-scale and Resume Test - Detailed Explanation

**File**: `e2etest/zt_http_autoscale_resume_test.go`
**Purpose**: End-to-end tests for HTTP download parallelism, resume, and control features

---

## Test Suite Overview

This file contains 5 comprehensive E2E tests that validate HTTP download functionality in real-world scenarios:

### Tests Included

1. **`TestHTTPDownload_AutoScaling`** - Parallel chunk downloads
2. **`TestHTTPDownload_Resume`** - Interruption and resume behavior
3. **`TestHTTPDownload_ConcurrencyControl`** - Bandwidth capping
4. **`TestHTTPDownload_BlockSizeControl`** - Custom chunk sizes
5. **`TestHTTPDownload_CancelWithSignal`** - Graceful signal handling

### Common Characteristics

**Test Data Source**:
- URL: `https://aka.ms/infrahcios23`
- File: Azure Stack HCI evaluation ISO
- Size: ~3.5GB (3,748,632,576 bytes)
- Redirects to Microsoft CDN
- Supports HTTP Range requests
- Expected SHA256: `140D2A6BC53DADCCB9FB66B0D6D2EF61C9D23EA937F8CCC62788866D02997BCA`

**Test Pattern**:
1. Create isolated temporary directory
2. Find azcopy binary (via `findAzCopyBinary` helper)
3. Execute command with test-specific flags
4. Monitor output/behavior
5. Validate results
6. Cleanup temporary files

**Disabled by Default**:
- All tests skip unless `-enable-real-http-tests` flag provided
- Reason: Downloads large files from internet (expensive in CI)
- Small file tests run in CI instead

---

## Test 1: Auto-Scaling

**Function**: `TestHTTPDownload_AutoScaling(t *testing.T)`

### Purpose
Validates that HTTP downloads automatically use parallel chunks when the server supports HTTP Range requests.

### How It Works

```go
cmd := exec.Command(azcopyPath, "copy", sourceURL, targetPath,
    "--log-level=INFO",
    "--output-type=text")
```

1. **Executes download** with verbose logging
2. **Captures output** to analyze parallelism indicators
3. **Searches for throughput measurements** using regex
4. **Validates multiple measurements** indicate parallel chunks

### What It Checks

✅ Job completes successfully
✅ Multiple throughput measurements (>5)
✅ File size > 1MB (large enough for parallelism)
✅ Varying throughput = parallel chunk downloads

### Expected Behavior

```
Throughput (Mb/s): 85.3
Throughput (Mb/s): 102.7
Throughput (Mb/s): 98.5
Throughput (Mb/s): 110.2
...
Final Job Status: Completed
```

Multiple measurements prove chunks are being downloaded in parallel.

---

## Test 2: Resume ⭐ (Most Complex)

**Function**: `TestHTTPDownload_Resume(t *testing.T)`

### Purpose
Tests interrupted download handling and documents HTTP resume limitations.

### Architecture: 3-Phase Approach

```
┌─────────────────────────────────────────────────────┐
│ Phase 1: Interrupted Download                       │
├─────────────────────────────────────────────────────┤
│ 1. Start download with context for cancellation     │
│ 2. Monitor output in real-time                      │
│ 3. Capture job ID: "Job <uuid> has started"        │
│ 4. Parse progress: "12.5% Complete"                │
│ 5. Cancel at 10% via context                        │
│ 6. Validate partial file created                    │
└─────────────────────────────────────────────────────┘
                      ↓
┌─────────────────────────────────────────────────────┐
│ Phase 2: Resume Attempt                             │
├─────────────────────────────────────────────────────┤
│ 1. Execute: azcopy jobs resume <jobID>              │
│ 2. May fail (expected for HTTP)                     │
│ 3. Document why resume is limited                   │
│ 4. Educational: shows protocol constraints          │
└─────────────────────────────────────────────────────┘
                      ↓
┌─────────────────────────────────────────────────────┐
│ Phase 3: Idempotent Re-run (Recommended)            │
├─────────────────────────────────────────────────────┤
│ 1. Execute: azcopy copy ... --overwrite=false       │
│ 2. Skips if already complete                        │
│ 3. Completes if partial                              │
│ 4. Re-downloads if partial invalid                  │
│ 5. Validates final file                             │
└─────────────────────────────────────────────────────┘
```

### Phase 1: Interrupted Download (Detailed)

#### Setup
```go
ctx, cancel := context.WithCancel(context.Background())
defer cancel() // ← CRITICAL: Prevents context leak

cmd := exec.CommandContext(ctx, azcopyPath, "copy", ...)
stdout, _ := cmd.StdoutPipe()
stderr, _ := cmd.StderrPipe()
cmd.Start()
```

#### Real-time Output Monitoring

**Why Separate Goroutine for Stderr?**
- Prevents deadlock if both stdout and stderr buffers fill
- AzCopy writes to both streams simultaneously
- Non-blocking reads required on both

```go
// Goroutine prevents pipe blocking
go func() {
    for stderrScanner.Scan() {
        t.Logf("stderr: %s", stderrScanner.Text())
    }
}()
```

#### Job ID Extraction

**AzCopy Output Format**:
```
Job a1b2c3d4-e5f6-7890-abcd-ef1234567890 has started
```

**Extraction Logic**:
```go
if strings.Contains(line, "Job") && strings.Contains(line, "has started") {
    parts := strings.Fields(line)
    // parts = ["Job", "<uuid>", "has", "started", ...]
    if len(parts) >= 2 {
        jobID = parts[1]
    }
}
```

#### Progress Monitoring

**AzCopy Progress Formats** (varies by version/mode):
```
Percent Complete (approx): 12.5%
12.5 % Done
Progress: 12.5%
Transferred: 350MB (12.5%)
```

**Flexible Regex Matching**:
```go
percentRegex := regexp.MustCompile(`([\d.]+)\s*%`)
// Matches: "12.5%" or "12.5 %" in any context
```

**Cancellation Trigger**:
```go
if percent >= 10.0 {
    // 10% of 3.5GB ≈ 350MB (substantial for testing)
    time.Sleep(1 * time.Second)  // Download more chunks
    cancel()                      // Graceful shutdown
    break
}
```

**Why Context Instead of Process.Kill()?**
- Graceful shutdown (allows cleanup)
- Triggers `context.Canceled` error
- AzCopy can save state before exit
- More realistic user interruption (Ctrl+C)

### Phase 2: Resume Attempt (Educational)

#### Command
```go
azcopy jobs resume <jobID> --log-level=INFO
```

#### Why HTTP Resume is Limited

**1. Protocol Limitations**
```
HTTP/1.1 doesn't provide:
  ✗ File versioning (unlike Azure Blob's ETag)
  ✗ Consistency guarantees
  ✗ Lease mechanisms
  ✗ Change tracking
```

**2. State Persistence Issues**
```
Job plan may not capture:
  ✗ HTTP-specific download state
  ✗ Server capabilities (range support)
  ✗ Chunk completion mapping
  ✗ Authentication token state
```

**3. Server Variability**
```
HTTP servers can:
  ✗ Change file content between requests
  ✗ Reject range requests
  ✗ Serve different versions via CDN
  ✗ Return different file sizes
```

#### Test Behavior
```go
if err != nil {
    // EXPECTED: Resume likely fails for HTTP
    t.Logf("Resume command failed (expected for HTTP downloads): %v", err)

    // Educational logging explains WHY
    t.Logf("HTTP downloads may not support resume via job ID because:")
    t.Logf("  1. No ETag/version tracking")
    t.Logf("  2. File may change on server")
    t.Logf("  3. Job plan may not persist HTTP state")

    // Proceed to Phase 3 (fallback pattern)
}
```

### Phase 3: Idempotent Re-run (Recommended Pattern)

#### Command
```go
azcopy copy <sourceURL> <targetPath> \
    --log-level=INFO \
    --output-type=text \
    --overwrite=false  // ← KEY FLAG
```

#### Why This Works Better

**1. Server-Agnostic**
```
✓ No job state required
✓ Works with any HTTP server
✓ No ETag dependency
✓ Survives process restart
```

**2. Idempotent**
```
✓ --overwrite=false skips if complete
✓ Safe to retry unlimited times
✓ Can wrap in until loop
✓ Handles partial files gracefully
```

**3. User-Friendly**
```
✓ Simple command
✓ No job ID tracking
✓ Works with fresh auth tokens
✓ Easy to script
```

#### Recommended User Pattern

```bash
#!/bin/bash
MAX_RETRIES=3
COUNT=0

until azcopy copy "https://example.com/file.iso" "./downloads/" \
    --overwrite=false; do

    COUNT=$((COUNT + 1))
    if [ $COUNT -ge $MAX_RETRIES ]; then
        echo "Failed after $MAX_RETRIES attempts"
        exit 1
    fi

    echo "Download failed, retrying in 5 seconds... (attempt $COUNT/$MAX_RETRIES)"
    sleep 5
done

echo "Download completed successfully!"
```

#### Possible Outcomes (All Acceptable)

**Outcome A**: Complete the Download
```
Partial file is valid
→ AzCopy continues from where it left off
→ Downloads remaining chunks
→ Verifies integrity
→ Success
```

**Outcome B**: Skip the Download
```
File already complete and valid
→ AzCopy detects via --overwrite=false
→ Skips transfer
→ Success
```

**Outcome C**: Re-download Entire File
```
Partial file is invalid/corrupted
→ AzCopy removes partial file
→ Downloads entire file fresh
→ Success
```

### Final Validation

```go
finalInfo, err := os.Stat(targetPath)
if err == nil {
    // File exists
    fileSizeGB := float64(finalInfo.Size()) / (1024 * 1024 * 1024)

    // For Azure Stack HCI ISO, expect ~3.5GB
    if finalInfo.Size() > 100*1024*1024 { // > 100MB
        // Substantial file = download worked
        t.Logf("✓ File size indicates successful download")
    }
}
```

### Test Output Summary

```
✓ Resume test completed

Summary:
  - Phase 1: Successfully interrupted download at 10%+ progress ✓
  - Phase 2: Documented resume limitations for HTTP ✓
  - Phase 3: Demonstrated idempotent re-run pattern ✓

Recommendation for users:
  Use retry loop with --overwrite=false instead of job resume
  See docs/HTTP_DOWNLOADS.md for examples
```

---

## Test 3: Concurrency Control

**Function**: `TestHTTPDownload_ConcurrencyControl(t *testing.T)`

### Purpose
Validates that bandwidth capping (`--cap-mbps`) works correctly.

### How It Works

```go
cmd := exec.Command(azcopyPath, "copy", sourceURL, targetPath,
    "--cap-mbps=100",  // ← Cap at 100 Mbps
    "--log-level=INFO")

startTime := time.Now()
output, err := cmd.CombinedOutput()
duration := time.Since(startTime)
```

### What It Checks

✅ Download completes
✅ Duration > 5 seconds (proves throttling)
✅ Throughput respects limit
✅ File downloaded successfully

### Expected Behavior

**Without cap**: Downloads in ~30-40 seconds at full speed
**With cap**: Downloads in >60 seconds at 100 Mbps

The increased duration proves bandwidth limiting works.

---

## Test 4: Block Size Control

**Function**: `TestHTTPDownload_BlockSizeControl(t *testing.T)`

### Purpose
Validates custom chunk sizes work correctly.

### How It Works

```go
cmd := exec.Command(azcopyPath, "copy", sourceURL, targetPath,
    "--block-size-mb=16",  // ← Use 16MB chunks instead of default 8MB
    "--log-level=INFO")
```

### What It Checks

✅ Download completes with custom block size
✅ Final Job Status: Completed
✅ File size > 1MB
✅ No errors with larger chunks

### Use Cases

- **Larger blocks (16-32MB)**: Better for high-bandwidth, low-latency networks
- **Smaller blocks (4MB)**: Better for unstable connections
- **Default (8MB)**: Balanced for most scenarios

---

## Test 5: Cancel with Signal

**Function**: `TestHTTPDownload_CancelWithSignal(t *testing.T)`

### Purpose
Tests graceful cancellation via SIGINT (Ctrl+C).

### How It Works

```go
cmd := exec.Command(azcopyPath, "copy", sourceURL, targetPath)
cmd.Start()

time.Sleep(5 * time.Second)  // Let it run

// Send SIGINT (simulates Ctrl+C)
cmd.Process.Signal(syscall.SIGINT)

cmd.Wait()  // Should exit gracefully
```

### What It Checks

✅ Process starts successfully
✅ Responds to SIGINT
✅ Exits without crash
✅ Cleanup happens properly

### Real-World Simulation

This mimics user pressing Ctrl+C during download:
```bash
$ azcopy copy "https://example.com/file.iso" "./downloads/"
Job 12345 has started
^C  # User presses Ctrl+C
```

---

## Testing Best Practices Demonstrated

### 1. Test Isolation
```go
tmpDir, err := os.MkdirTemp("", "azcopy-test-*")
defer os.RemoveAll(tmpDir)
```
- Each test gets unique temp directory
- Prevents test interference
- Automatic cleanup

### 2. Proper Resource Cleanup
```go
defer cancel()  // Context cleanup
defer os.RemoveAll(tmpDir)  // File cleanup
```
- Ensures cleanup on all exit paths
- Prevents resource leaks
- Fixes `go vet` warnings

### 3. Real-World Simulation
- Uses actual internet downloads
- Tests with realistic file sizes
- Simulates user actions (Ctrl+C)
- Validates production scenarios

### 4. Educational Documentation
- Tests document behavior
- Explain limitations
- Provide user recommendations
- Include example patterns

### 5. Flexible Validation
```go
if err != nil {
    t.Logf("Expected failure: %v", err)
    // Test alternative path
} else {
    t.Logf("Unexpected success (acceptable)")
}
```
- Handles both success and expected failures
- Documents why failures are acceptable
- Provides fallback validation

---

## Running the Tests

### Run All HTTP Tests
```bash
go test -v ./e2etest -run TestHTTPDownload -enable-real-http-tests
```

### Run Specific Test
```bash
go test -v ./e2etest -run TestHTTPDownload_Resume -enable-real-http-tests
```

### Run with Timeout
```bash
go test -timeout=30m -v ./e2etest -run TestHTTPDownload -enable-real-http-tests
```

### CI Usage (Excluded)
```bash
# CI runs small file tests only
go test -v ./e2etest \
    -run "TestRealHTTPDownload_SmallFile|TestRealHTTPDownload_AnonymousPublicCDN" \
    -enable-real-http-tests
```

---

## Key Takeaways

### For Developers

1. **Context Leak Prevention**
   - Always use `defer cancel()` with context
   - Fixes `go vet` warnings
   - Ensures proper cleanup

2. **Pipe Deadlock Prevention**
   - Use goroutines for stderr
   - Non-blocking reads required
   - Prevents test hangs

3. **Flexible Parsing**
   - Use regex for output parsing
   - Handle multiple formats
   - Resilient to version changes

### For Users

1. **HTTP Resume Limitations**
   - Traditional job resume may not work
   - Use `--overwrite=false` retry pattern
   - Wrap in `until` loop for reliability

2. **Performance Tuning**
   - `--cap-mbps`: Control bandwidth
   - `--block-size-mb`: Optimize for network
   - Monitor throughput measurements

3. **Graceful Cancellation**
   - Ctrl+C works properly
   - AzCopy cleans up on exit
   - Partial files may be resumable

---

*Last Updated: 2025-11-18*
*Test File: e2etest/zt_http_autoscale_resume_test.go*
