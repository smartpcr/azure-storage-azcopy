# AzCopy v10
AzCopy v10 is a command-line utility that you can use to copy data to and from containers and file shares in Azure Storage accounts.
AzCopy V10 presents easy-to-use commands that are optimized for high performance and throughput.

## Features and capabilities

:white_check_mark: Use with storage accounts that have a hierarchical namespace (Azure Data Lake Storage Gen2).

:white_check_mark: Create containers and file shares.

:white_check_mark: Upload files and directories.

:white_check_mark: Download files and directories.

:white_check_mark: Copy containers, directories and blobs between storage accounts (Service to Service).

:white_check_mark: Synchronize data between Local <=> Blob Storage, Blob Storage <=> File Storage, and Local <=> File Storage.

:white_check_mark: Delete blobs or files from an Azure storage account

:white_check_mark: Copy objects, directories, and buckets from Amazon Web Services (AWS) to Azure Blob Storage (Blobs only).

:white_check_mark: Copy objects, directories, and buckets from Google Cloud Platform (GCP) to Azure Blob Storage (Blobs only).

:white_check_mark: Download files from HTTP/HTTPS endpoints with automatic parallelization and authentication support.

:white_check_mark: List files in a container.

:white_check_mark: Recover from failures by restarting previous jobs.

:white_check_mark: **Resumable chunk-level downloads** for large files (>256MB) - automatically resume interrupted downloads from where they left off.

## Download AzCopy
The latest binary for AzCopy along with installation instructions may be found
[here](https://docs.microsoft.com/en-us/azure/storage/common/storage-use-azcopy-v10).

## Find help

For complete guidance, visit any of these articles on the docs.microsoft.com website.

:eight_spoked_asterisk: [Get started with AzCopy (download links here)](https://docs.microsoft.com/azure/storage/common/storage-use-azcopy-v10)

:eight_spoked_asterisk: [Upload files to Azure Blob storage by using AzCopy](https://docs.microsoft.com/en-us/azure/storage/common/storage-use-azcopy-blobs-upload)

:eight_spoked_asterisk: [Download blobs from Azure Blob storage by using AzCopy](https://docs.microsoft.com/en-us/azure/storage/common/storage-use-azcopy-blobs-download)

:eight_spoked_asterisk: [Copy blobs between Azure storage accounts by using AzCopy](https://docs.microsoft.com/en-us/azure/storage/common/storage-use-azcopy-blobs-copy)

:eight_spoked_asterisk: [Synchronize between Local File System/Azure Blob Storage (Gen1)/Azure File Storage by using AzCopy](https://docs.microsoft.com/en-us/azure/storage/common/storage-use-azcopy-blobs-synchronize)

:eight_spoked_asterisk: [Transfer data with AzCopy and file storage](https://docs.microsoft.com/en-us/azure/storage/common/storage-use-azcopy-files)

:eight_spoked_asterisk: [Transfer data with AzCopy and Amazon S3 buckets](https://docs.microsoft.com/en-us/azure/storage/common/storage-use-azcopy-s3)

:eight_spoked_asterisk: [Transfer data with AzCopy and Google GCP buckets](https://docs.microsoft.com/en-us/azure/storage/common/storage-use-azcopy-google-cloud)

:eight_spoked_asterisk: [Download files from HTTP/HTTPS endpoints](docs/HTTP_DOWNLOADS.md)

:eight_spoked_asterisk: [Use data transfer tools in Azure Stack Hub Storage](https://docs.microsoft.com/en-us/azure-stack/user/azure-stack-storage-transfer)

:eight_spoked_asterisk: [Configure, optimize, and troubleshoot AzCopy](https://docs.microsoft.com/azure/storage/common/storage-use-azcopy-configure)

:eight_spoked_asterisk: [AzCopy WiKi](https://github.com/Azure/azure-storage-azcopy/wiki)

## Supported Operations

The general format of the AzCopy commands is: `azcopy [command] [arguments] --[flag-name]=[flag-value]`

* `bench` - Runs a performance benchmark by uploading or downloading test data to or from a specified destination

* `copy` - Copies source data to a destination location. The supported directions are:
    - Local File System <-> Azure Blob (SAS or OAuth authentication)
    - Local File System <-> Azure Files (Share/directory SAS or OAuth authentication)
    - Local File System <-> Azure Data Lake Storage (ADLS Gen2) (SAS, OAuth, or SharedKey authentication)
    - Azure Blob (SAS, OAuth or public authentication) -> Azure Blob (SAS or OAuth authentication)
    - Azure Blob (SAS, OAuth or public authentication) -> Azure Files (SAS or OAuth authentication)
    - Azure Files (SAS or OAuth authentication) -> Azure Files (SAS or OAuth authentication)
    - Azure Files (SAS or OAuth authentication) -> Azure Blob (SAS or OAuth authentication)
    - AWS S3 (Access Key) -> Azure Block Blob (SAS or OAuth authentication)
    - Google Cloud Storage (Service Account Key) -> Azure Block Blob (SAS or OAuth authentication) [Preview]
    - HTTP/HTTPS (Anonymous or Bearer token) -> Local File System

* `sync` - Replicate source to the destination location. The supported directions are:
    - Local File System <-> Azure Blob (SAS or OAuth authentication)
    - Local File System <-> Azure Files (Share/directory SAS or OAuth authentication)
    - Azure Blob (SAS, OAuth or public authentication) -> Azure Files (SAS or OAuth authentication)

* `login` - Log in to Azure Active Directory (AD) to access Azure Storage resources.

* `logout` - Log out to terminate access to Azure Storage resources.

* `list` - List the entities in a given resource

* `doc` - Generates documentation for the tool in Markdown format

* `env` - Shows the environment variables that you can use to configure the behavior of AzCopy.

* `help` - Help about any command

* `jobs` - Sub-commands related to managing jobs

* `load` - Sub-commands related to transferring data in specific formats

* `make` - Create a container or file share.

* `remove` - Delete blobs or files from an Azure storage account

## HTTP Downloads

AzCopy supports downloading files from generic HTTP/HTTPS endpoints with automatic parallelization and enterprise-grade reliability.

### Quick Examples

**Download a public file:**
```bash
azcopy copy "https://example.com/files/data.bin" "./downloads/"
```

**Download Azure Stack HCI ISO (3.5GB, real-world example):**
```bash
# Download Azure Stack HCI evaluation ISO from Microsoft CDN
azcopy copy "https://aka.ms/infrahcios23" "./AzureStackHCI.iso"

# With bandwidth limit for large downloads
azcopy copy "https://aka.ms/infrahcios23" "./AzureStackHCI.iso" --cap-mbps=100
```

**Download with OAuth Bearer token:**
```bash
azcopy copy "https://api.example.com/files/data.bin" "./downloads/" \
  --bearer-token="eyJ0eXAiOiJKV1QiLCJhbGci..."
```

**Download with custom headers:**
```bash
azcopy copy "https://api.example.com/files/data.json" "./downloads/" \
  --http-headers="X-API-Key=abc123;X-Request-ID=req-12345"
```

### Key Features

- **Auto-scaling parallel downloads** - Automatically uses multiple connections for faster downloads
- **Anonymous and authenticated access** - Supports public URLs and OAuth 2.0 Bearer tokens
- **Range request detection** - Automatically detects and uses HTTP Range requests for parallel chunks
- **Progress tracking** - Real-time progress, throughput, and ETA
- **Bandwidth control** - Cap download speed to avoid network saturation

### Performance Benchmark

Download performance comparison using Azure Stack HCI ISO (3.5GB) on 1Gbps connection:

```powershell
$downloadUrl = "https://aka.ms/infrahcios23"
$downloadIsoFile = "./AzureStackHCI.iso"
$azCopyPath = ".\azcopy.exe"

#region azcopy: full bandwidth at 1Gbps, 33 sec
if (Test-Path $downloadIsoFile) {
    Remove-item $downloadIsoFile -Force
}
$time1 = Measure-Command {
    & $azCopyPath copy $downloadUrl $downloadIsoFile
}
Write-Host "Download completed in $($time1.TotalSeconds) seconds using azcopy"
#endregion


#region invoke-webrequest: varies, start at 250Mbps, after 1 min, sustain at 120-130Mbps, 230 sec
if (Test-Path $downloadIsoFile) {
    Remove-item $downloadIsoFile -Force
}

$time2 = Measure-Command {
    Invoke-WebRequest -Uri $downloadUrl -OutFile $downloadIsoFile
}
Write-Host "Download completed in $($time2.TotalSeconds) seconds using invoke-webrequest"
#endregion


#region bits: 500-800Mbps, 51 sec
if (Test-Path $downloadIsoFile) {
    Remove-item $downloadIsoFile -Force
}

$time3 = Measure-Command {
    Start-BitsTransfer -Source $downloadUrl -Destination $downloadIsoFile -DisplayName "Azure Stack HCI Download"
}
Write-Host "Download completed in $($time3.TotalSeconds) seconds using BITS"
#endregion
```

**Results:**
- **AzCopy**: ~33 seconds (full 1Gbps bandwidth utilization)
- **BITS**: ~51 seconds (500-800Mbps)
- **Invoke-WebRequest**: ~230 seconds (120-130Mbps sustained)

AzCopy's parallel chunking delivers **7x faster** downloads compared to PowerShell's Invoke-WebRequest.

For complete documentation, see [HTTP_DOWNLOADS.md](docs/HTTP_DOWNLOADS.md).

## Resumable Downloads

AzCopy supports **resumable chunk-level downloads** for large files. If a download is interrupted (network failure, process termination, system restart), AzCopy can automatically resume from where it left off instead of starting over.

### Key Features

- **Automatic for large files**: Enabled by default for files ≥256MB
- **Chunk-level progress tracking**: Uses memory-mapped progress files for efficient tracking
- **Chunk hash validation**: Verifies MD5 hash of each chunk on resume to detect corruption
- **Source change detection**: Validates source hasn't changed before resuming
- **Cross-platform support**: Works on Windows, Linux, and macOS

### Data Integrity

AzCopy ensures data integrity during resumable downloads through **chunk hash validation**:

1. **During download**: Each chunk's MD5 hash is computed and stored in the progress file
2. **On resume**: Before skipping "completed" chunks, their content is re-read and verified against stored hashes
3. **Corruption recovery**: Any chunks that fail validation are automatically re-downloaded

This protects against scenarios where a process is terminated before data is fully flushed to disk. Even if the progress file shows a chunk as "complete", AzCopy will detect the corruption and re-download that chunk.

### Supported Sources

- Azure Blob Storage
- Azure Files
- Azure Data Lake Storage Gen2
- HTTP/HTTPS (servers that support Range requests)

### Quick Example

```bash
# Download a large file (resumable mode enabled automatically for files ≥256MB)
azcopy copy "https://example.com/large-file.iso" "./downloads/"

# If interrupted, simply re-run - AzCopy resumes from where it left off
azcopy jobs resume <jobID>

# Or use idempotent re-run pattern
azcopy copy "https://example.com/large-file.iso" "./downloads/" --overwrite=false
```

### Downloading Large Payloads from HTTPS

AzCopy is ideal for downloading large files (ISOs, VM images, datasets) from HTTPS endpoints with automatic resume capability.

**Real-world example - Download Azure Stack HCI ISO (3.5GB):**

```bash
# Basic download - resumable mode activates automatically for files ≥256MB
azcopy copy "https://aka.ms/infrahcios23" "./AzureStackHCI.iso"

# With bandwidth limiting (useful for background downloads)
azcopy copy "https://aka.ms/infrahcios23" "./AzureStackHCI.iso" --cap-mbps=100

# With verbose logging to monitor progress
azcopy copy "https://aka.ms/infrahcios23" "./AzureStackHCI.iso" --log-level=INFO
```

**If download is interrupted (Ctrl+C, network failure, system restart):**

```bash
# Method 1: Resume using job ID (shown in output when download started)
azcopy jobs resume 0106a4a4-52e8-e84a-646f-04ae84aa9535

# Method 2: List recent jobs and find the one to resume
azcopy jobs list
azcopy jobs resume <jobID>

# Method 3: Simply re-run the same command (detects existing progress)
azcopy copy "https://aka.ms/infrahcios23" "./AzureStackHCI.iso"
```

**Download with authentication:**

```bash
# OAuth Bearer token (for authenticated APIs)
azcopy copy "https://api.example.com/datasets/large-model.tar.gz" "./model.tar.gz" \
  --bearer-token="eyJ0eXAiOiJKV1QiLCJhbGci..."

# Custom headers (API keys, request IDs)
azcopy copy "https://api.example.com/files/dataset.zip" "./dataset.zip" \
  --http-headers="X-API-Key=your-api-key;X-Request-ID=req-123"
```

**Performance tips for large downloads:**

```bash
# Increase concurrency for high-bandwidth connections
export AZCOPY_CONCURRENCY_VALUE=32
azcopy copy "https://example.com/large-file.iso" "./file.iso"

# Use larger chunks for very large files (reduces overhead)
export AZCOPY_RESUMABLE_CHUNK_SIZE=104857600  # 100MB chunks
azcopy copy "https://example.com/huge-file.tar" "./file.tar"
```

### Configuration

Configure via environment variables:

```bash
# Enable/disable resumable downloads (default: true)
export AZCOPY_RESUMABLE_DOWNLOAD=true

# Minimum file size for resumable mode (default: 256MB)
export AZCOPY_RESUMABLE_THRESHOLD=268435456

# Chunk size for progress tracking (default: 64MB, range: 4MB-100MB)
export AZCOPY_RESUMABLE_CHUNK_SIZE=67108864
```

For complete documentation, see [resumable-download.md](docs/resumable-download.md).

## Configuration & Settings

AzCopy can be configured via environment variables. No configuration file is required.

### Storage Locations

Control where AzCopy stores logs, job plans, and progress files:

```bash
# Log files location (default: ~/.azcopy/)
export AZCOPY_LOG_LOCATION="/custom/path/logs"

# Job plan files - for job state and resume capability (default: ~/.azcopy/)
export AZCOPY_JOB_PLAN_LOCATION="/custom/path/plans"

# Chunk progress files - for resumable downloads (default: same as job plan location)
export AZCOPY_CHUNK_PROGRESS_DIR="/custom/path/progress"
```

**Default file locations:**

| File Type | Default Location | Description |
|-----------|------------------|-------------|
| Log files | `~/.azcopy/*.log` | Transfer logs and debugging info |
| Job plans | `~/.azcopy/*.steV*` | Job state for resume capability |
| Chunk progress | `~/.azcopy/plans/*.chunks` | Chunk-level progress for large files |
| Temp downloads | `<target-dir>/.azDownload-*` | Temporary files during download |

### Performance Tuning

```bash
# Number of parallel HTTP connections (default: auto-detected based on CPU cores)
export AZCOPY_CONCURRENCY_VALUE=32

# Memory buffer size in GB (default: auto-detected based on available memory)
export AZCOPY_BUFFER_GB=4

# Number of files transferred concurrently
export AZCOPY_CONCURRENT_FILES=300

# Parallel file scanning degree
export AZCOPY_CONCURRENT_SCAN=64
```

### Download Behavior

```bash
# Download to temp file first, then rename (default: true)
export AZCOPY_DOWNLOAD_TO_TEMP_PATH=true

# Enable resumable chunk-level downloads (default: true)
export AZCOPY_RESUMABLE_DOWNLOAD=true

# Minimum file size to enable resumable mode (default: 256MB)
export AZCOPY_RESUMABLE_THRESHOLD=268435456

# Chunk size for resumable downloads (default: 64MB)
export AZCOPY_RESUMABLE_CHUNK_SIZE=67108864
```

### Example: Dedicated Storage for Large Transfers

```bash
# Use a fast SSD for AzCopy state (avoids filling up system disk)
export AZCOPY_LOG_LOCATION="/mnt/fast-ssd/azcopy/logs"
export AZCOPY_JOB_PLAN_LOCATION="/mnt/fast-ssd/azcopy/plans"

# High-performance settings for large transfers
export AZCOPY_CONCURRENCY_VALUE=64
export AZCOPY_BUFFER_GB=8

# Run large transfer
azcopy copy "https://example.com/dataset.tar" "/data/downloads/"
```

### Windows Configuration (PowerShell)

```powershell
# Set for current session
$env:AZCOPY_LOG_LOCATION = "D:\AzCopy\logs"
$env:AZCOPY_JOB_PLAN_LOCATION = "D:\AzCopy\plans"
$env:AZCOPY_CONCURRENCY_VALUE = "32"

# Or set permanently via:
# System Properties > Advanced > Environment Variables
```

### View All Environment Variables

```bash
azcopy env
```

## Find help from your command prompt

For convenience, consider adding the AzCopy directory location to your system path for ease of use. That way you can type `azcopy` from any directory on your system.

To see a list of commands, type `azcopy -h` and then press the ENTER key.

To learn about a specific command, just include the name of the command (For example: `azcopy list -h`).

![AzCopy command help example](readme-command-prompt.png)

If you choose not to add AzCopy to your path, you'll have to change directories to the location of your AzCopy executable and type `azcopy` or `.\azcopy` in Windows PowerShell command prompts.

## Frequently asked questions

### What is the difference between `sync` and `copy`?

* The `copy` command is a simple transferring operation. It scans/enumerates the source and attempts to transfer every single file/blob present on the source to the destination.
  The supported source/destination pairs are listed in the help message of the tool.

* On the other hand, `sync` scans/enumerates both the source, and the destination to find the incremental change.
  It makes sure that whatever is present in the source will be replicated to the destination. For `sync`,

* If your goal is to simply move some files, then `copy` is definitely the right command, since it offers much better performance.
  If the use case is to incrementally transfer data (files present only on source) then `sync` is the better choice, since only the modified/missing files will be transferred.
  Since `sync` enumerates both source and destination to find the incremental change, it is relatively slower as compared to `copy`

### Will `copy` overwrite my files?

By default, AzCopy will overwrite the files at the destination if they already exist. To avoid this behavior, please use the flag `--overwrite=false`.

### Will `sync` overwrite my files?

By default, AzCopy `sync` use last-modified-time to determine whether to transfer the same file present at both the source, and the destination.
i.e, If the source file is newer compared to the destination file, we overwrite the destination
You can change this default behaviour and overwrite files at the destination by using the flag `--mirror-mode=true`

### Will 'sync' delete files in the destination if they no longer exist in the source location?

By default, the 'sync' command doesn't delete files in the destination unless you use an optional flag with the command.
To learn more, see [Synchronize files](https://docs.microsoft.com/en-us/azure/storage/common/storage-use-azcopy-blobs-synchronize).

## How to contribute to AzCopy v10

This project welcomes contributions and suggestions.  Most contributions require you to agree to a
Contributor License Agreement (CLA) declaring that you have the right to, and actually do, grant us
the rights to use your contribution. For details, visit https://cla.microsoft.com.

When you submit a pull request, a CLA-bot will automatically determine whether you need to provide
a CLA and decorate the PR appropriately (e.g., label, comment). Simply follow the instructions
provided by the bot. You will only need to do this once across all repos using our CLA.

This project has adopted the [Microsoft Open Source Code of Conduct](https://opensource.microsoft.com/codeofconduct/).
For more information see the [Code of Conduct FAQ](https://opensource.microsoft.com/codeofconduct/faq/) or
contact [opencode@microsoft.com](mailto:opencode@microsoft.com) with any additional questions or comments.
