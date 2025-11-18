# AzCopy Known Issues

Last updated: 2025-11-18

## Current Issues

*Add known issues as they are discovered*

## Limitations

### Platform-Specific

**Windows**:
- Long path handling requires Windows 10 version 1607 or later
- Some file attributes may not transfer to non-Windows destinations

**Linux**:
- Extended attributes (xattr) preservation has limitations
- Symbolic link handling varies by storage type

**macOS**:
- Resource forks not preserved
- Finder metadata not transferred

### Storage Service Limitations

**Azure Blob Storage**:
- Blob names have character restrictions
- Maximum blob size limits vary by type (block, page, append)
- Page blob size must be 512-byte aligned

**Azure Files**:
- File share snapshot limitations
- Maximum file/directory name lengths
- Case sensitivity handling

**S3/GCP**:
- Service-to-service copy requires proper IAM permissions
- Object metadata mapping may lose fidelity
- Region restrictions for certain operations

## Workarounds

*Document workarounds for known issues*

## Fixed Issues

*Move resolved issues here for reference*

---

*Update this file when issues are discovered or resolved*
