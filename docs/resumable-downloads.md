# Resumable Downloads in AzCopy

## Overview

AzCopy now supports **resumable chunk-level downloads** for large files. If a download is interrupted (due to network issues, process termination, or system restart), AzCopy can resume from where it left off instead of starting over, saving time and bandwidth.

## Key Features

- **Automatic Resume**: Downloads resume automatically when restarting an interrupted job
- **Chunk-Level Progress**: Tracks progress at the chunk level (typically 64MB chunks)
- **Source Change Detection**: Validates that the source file hasn't changed before resuming
- **Corruption Detection**: Validates chunk progress file integrity
- **Concurrent Protection**: Prevents multiple processes from resuming the same download simultaneously
- **Disk Space Validation**: Checks available disk space before starting downloads
- **Platform Support**: Works on Windows, Linux, and macOS

## How It Works

### High-Level Flow

1. **Download Starts**: For files ≥256MB, AzCopy creates a chunk progress file (`.chunks`)
2. **Progress Tracking**: As each chunk downloads, it's marked complete in the progress file
3. **Interruption**: If the download is interrupted, the progress file remains on disk
4. **Resume**: When the job resumes, AzCopy:
   - Validates the source file hasn't changed (size, last modified time, MD5)
   - Reads the chunk progress file to identify completed chunks
   - Downloads only the remaining (pending) chunks
5. **Completion**: Once all chunks are downloaded, the file is finalized and the progress file is deleted

### Technical Details

- **Chunk Progress Files**: Stored in the job plan directory (default: `~/.azcopy/`)
- **File Format**: Memory-mapped binary format for efficient concurrent access
- **Chunk Size**: Configurable (default: 64MB, min: 4MB, max: 100MB)
- **File Threshold**: Configurable (default: 256MB minimum file size for resumable mode)
- **Progress File Size**: ~384KB for a 1TB file (6 bytes per chunk + 64-byte header)

## When Resumable Downloads Are Enabled

Resumable downloads are **automatically enabled** when ALL of the following conditions are met:

1. ✅ File size ≥ 256MB (configurable via `AZCOPY_RESUMABLE_THRESHOLD`)
2. ✅ Destination is NOT `/dev/null` (or `NUL` on Windows)
3. ✅ Source supports random access (Azure Blob, Azure Files, HTTP with Range support)
4. ✅ Decompression is NOT enabled (can't resume compressed streams)
5. ✅ Resumable downloads are enabled (configurable via `AZCOPY_RESUMABLE_DOWNLOAD`)

### Supported Sources

- ✅ **Azure Blob Storage** - Fully supported
- ✅ **Azure Files** - Fully supported
- ✅ **Azure Data Lake Storage Gen2** - Fully supported
- ✅ **HTTP/HTTPS** - Supported if server supports Range requests
- ❌ **HTTP/HTTPS without Range support** - Falls back to non-resumable mode
- ❌ **Piped input** - Cannot resume

## Checking Resume Progress

### Using `azcopy jobs show`

```bash
# Show job summary with chunk progress
azcopy jobs show <jobID>
```

**Example output:**
```
Job 12345678-1234-1234-1234-123456789012 summary
Number of File Transfers: 5
Total Number of Transfers: 5
Number of File Transfers Completed: 3
Number of File Transfers Failed: 0
Total Number of Bytes Transferred: 2147483648
Percent Complete (approx): 60.0
Final Job Status: InProgress

Resumable Download Progress:
  Chunks Completed: 24/40 (60.0%)
```

### Using `azcopy jobs resume`

```bash
# Resume an interrupted job
azcopy jobs resume <jobID>
```

AzCopy will automatically detect which chunks have already been downloaded and resume only the remaining chunks.

## Configuration

### Environment Variables

Configure resumable downloads using these environment variables:

#### `AZCOPY_RESUMABLE_DOWNLOAD`
- **Description**: Enable or disable resumable downloads
- **Default**: `true`
- **Values**: `true`, `false`, `1`, `0`, `yes`, `no`, `on`, `off`
- **Example**:
  ```bash
  export AZCOPY_RESUMABLE_DOWNLOAD=true
  ```

#### `AZCOPY_RESUMABLE_THRESHOLD`
- **Description**: Minimum file size (in bytes) to enable resumable downloads
- **Default**: `268435456` (256MB)
- **Minimum**: `4194304` (4MB)
- **Example**:
  ```bash
  export AZCOPY_RESUMABLE_THRESHOLD=536870912  # 512MB
  ```

#### `AZCOPY_RESUMABLE_CHUNK_SIZE`
- **Description**: Size of each chunk (in bytes) for resumable downloads
- **Default**: `67108864` (64MB)
- **Minimum**: `4194304` (4MB)
- **Maximum**: `104857600` (100MB)
- **Example**:
  ```bash
  export AZCOPY_RESUMABLE_CHUNK_SIZE=33554432  # 32MB
  ```

#### `AZCOPY_RESUME_SKIP_MD5`
- **Description**: Skip MD5 validation when resuming (faster but less safe)
- **Default**: `false`
- **Values**: `true`, `false`
- **Example**:
  ```bash
  export AZCOPY_RESUME_SKIP_MD5=false
  ```

#### `AZCOPY_CHUNK_PROGRESS_DIR`
- **Description**: Directory to store chunk progress files
- **Default**: Same as job plan directory (`~/.azcopy/`)
- **Example**:
  ```bash
  export AZCOPY_CHUNK_PROGRESS_DIR=/tmp/azcopy-progress
  ```

### Configuration Examples

**Aggressive resumability** (more files use resumable mode):
```bash
export AZCOPY_RESUMABLE_THRESHOLD=104857600    # 100MB threshold
export AZCOPY_RESUMABLE_CHUNK_SIZE=33554432    # 32MB chunks
```

**Conservative resumability** (only very large files):
```bash
export AZCOPY_RESUMABLE_THRESHOLD=1073741824   # 1GB threshold
export AZCOPY_RESUMABLE_CHUNK_SIZE=104857600   # 100MB chunks
```

**Disable resumable downloads**:
```bash
export AZCOPY_RESUMABLE_DOWNLOAD=false
```

## Troubleshooting

### Issue: Resume keeps restarting from scratch

**Symptoms**: Job resumes but downloads entire file again

**Possible Causes:**
1. **Source file changed** - AzCopy detects the file was modified and restarts
   - Check logs for: "Source file changed: starting fresh download"
   - Solution: Ensure source file is not being modified during download

2. **Chunk progress file corrupted** - Progress file is invalid
   - Check logs for: "Progress file corrupted: starting fresh download"
   - Solution: Delete `.chunks` file and restart download

3. **Different job ID** - Using a different job than the original
   - Solution: Use the same job ID with `azcopy jobs resume <jobID>`

### Issue: "Insufficient disk space" error

**Symptoms**: Download fails with disk space error before starting

**Cause**: Not enough disk space for file + 10% safety margin

**Solution:**
```bash
# Check available space
df -h /destination/path

# Free up space or use a different destination
azcopy copy "source" "/path/with/more/space" --overwrite=true
```

### Issue: "Failed to acquire file lock" error

**Symptoms**: Cannot resume download, timeout after 30 seconds

**Possible Causes:**
1. **Another process is resuming the same job**
   - Solution: Wait for other process to complete, or kill it
   - Check with: `ps aux | grep azcopy`

2. **Stale lock from crashed process**
   - Solution: Manually delete the chunk progress file:
     ```bash
     rm ~/.azcopy/<jobID>-*.chunks
     ```

### Issue: Resume uses more bandwidth than expected

**Symptoms**: Resume downloads more data than expected

**Possible Causes:**
1. **MD5 validation failed on chunks** - Chunks are re-downloaded if corrupted
   - Check logs for: "MD5 mismatch: re-downloading chunk"
   - This is normal and ensures data integrity

2. **Source file changed** - New version is larger
   - Resume is rejected, fresh download starts

### Issue: Progress file taking too much disk space

**Symptoms**: `.chunks` files are large

**Explanation**: This is normal. Progress file size is proportional to file size:
- 1GB file: ~1KB progress file
- 100GB file: ~60KB progress file
- 1TB file: ~384KB progress file

**To reduce**: Increase chunk size (but decreases resume granularity):
```bash
export AZCOPY_RESUMABLE_CHUNK_SIZE=104857600  # 100MB chunks
```

## FAQ

### Q: Does resumable download work with `--from-to` parameter?
**A**: Yes, resumable downloads work with all supported source/destination combinations (Blob→Local, File→Local, HTTP→Local).

### Q: Can I resume a download started with an older version of AzCopy?
**A**: Resumable chunk-level downloads were introduced in v10.33.0. Older versions will ignore chunk progress files and start fresh downloads.

### Q: What happens if the source file changes during download?
**A**: On resume, AzCopy validates the source file (size, last modified time, MD5). If changed, the resume is rejected and a fresh download starts. Check logs for: "Source file changed: starting fresh download."

### Q: Can I manually delete chunk progress files?
**A**: Yes, it's safe to delete `.chunks` files. AzCopy will start a fresh download on next attempt. Progress files are automatically deleted on successful completion.

### Q: Does resumable download work with symbolic links?
**A**: No, resumable downloads are not used for symbolic links (they're typically very small).

### Q: How does resumable download affect performance?
**A**: Overhead is minimal (<5%) for fresh downloads. Resume operations save significant time and bandwidth by avoiding re-downloading completed chunks.

### Q: Can I use resumable downloads with compression?
**A**: No, resumable downloads are disabled when decompression is enabled (`Content-Encoding: gzip`). Compressed streams cannot be resumed at arbitrary points.

### Q: Where are chunk progress files stored?
**A**: By default, in the same directory as job plan files (`~/.azcopy/` or `%USERPROFILE%\.azcopy\` on Windows). Customize with `AZCOPY_CHUNK_PROGRESS_DIR`.

### Q: What happens if I run out of disk space mid-download?
**A**: The download fails gracefully, preserving progress. Free up disk space and resume:
```bash
# Free up space
rm -rf /tmp/old-files

# Resume the job
azcopy jobs resume <jobID>
```

### Q: Can multiple AzCopy processes resume the same download?
**A**: No, file locking prevents concurrent resumes of the same file. The second process will fail with "Failed to acquire file lock" after 30 seconds.

### Q: Does resumable download validate data integrity?
**A**: Yes, chunk-level MD5 validation ensures downloaded chunks are not corrupted. On resume, AzCopy validates both the chunk progress file integrity and the source file metadata (size, modified time, MD5).

## Best Practices

1. **Use default settings** for most scenarios - they're optimized for typical use cases

2. **Monitor disk space** when downloading large files:
   ```bash
   df -h /destination
   ```

3. **Don't modify source files** during download - changes invalidate resume progress

4. **Keep chunk progress files** until downloads complete - they enable resuming

5. **Use `azcopy jobs list`** to find interrupted jobs:
   ```bash
   azcopy jobs list
   ```

6. **Clean up old jobs** periodically:
   ```bash
   azcopy jobs clean --with-status=Completed
   azcopy jobs clean --with-status=Failed
   ```

7. **For very large files (>100GB)**, consider increasing chunk size for fewer chunks:
   ```bash
   export AZCOPY_RESUMABLE_CHUNK_SIZE=104857600  # 100MB chunks
   ```

8. **For unreliable networks**, decrease chunk size for finer resume granularity:
   ```bash
   export AZCOPY_RESUMABLE_CHUNK_SIZE=33554432  # 32MB chunks
   ```

## Logging

Resumable download operations are logged at various levels:

- **INFO**: Download starts, resume detected, completion
  - "Starting resumable download for <file>"
  - "Resuming download: X/Y chunks complete"
  - "Download completed: <file>"

- **WARNING**: Fallback scenarios, validation failures
  - "Source file changed: starting fresh download"
  - "Progress file corrupted: starting fresh download"
  - "Falling back to non-resumable mode: <reason>"

- **DEBUG**: Detailed chunk operations (enable with `--log-level=DEBUG`)
  - "Chunk <N> complete: <offset>-<end>"
  - "Marked chunk <N> as complete"

View logs:
```bash
# Set log level
export AZCOPY_LOG_LEVEL=INFO

# View recent logs
cat ~/.azcopy/<jobID>.log
```

## Performance Considerations

### Fresh Download Overhead
- **Overhead**: <5% for files ≥256MB
- **Components**: Chunk progress file creation, periodic sync, cleanup
- **Mitigation**: Overhead decreases as file size increases (percentage-wise)

### Resume Performance
- **Resume initialization**: <1 second for all file sizes
- **Bandwidth saved**: Downloads only pending chunks (not completed ones)
- **Example**: 90% complete → only downloads remaining 10%

### Memory Usage
- **Additional memory**: <50MB for resumable mode
- **Memory-mapped files**: Don't count toward heap allocation
- **Scalability**: Tested with 100GB+ files

### Disk I/O
- **Random writes**: <10% slower than sequential on SSD
- **Random writes**: <20% slower than sequential on HDD
- **Background sync**: <1% overhead (5-second interval)

## See Also

- [AzCopy Configuration](https://docs.microsoft.com/azure/storage/common/storage-use-azcopy-configure)
- [AzCopy Jobs Management](https://docs.microsoft.com/azure/storage/common/storage-ref-azcopy-jobs)
- [AzCopy Environment Variables](https://docs.microsoft.com/azure/storage/common/storage-use-azcopy-configure)
