# AzCopy Development Notes

## Quick Start

### Building
```bash
# Standard build
go build

# With static linking
go build -tags "netgo" -o azcopy_bin

# Platform-specific
CGO_ENABLED=1 go build -o azcopy_darwin_amd64
```

### Testing
```bash
# Unit tests (fast)
go test -v ./cmd ./common ./ste

# E2E tests (requires Azure credentials)
go test -timeout=2h -v ./e2etest

# Specific test
go test -v -run TestHTTPDownload ./e2etest
```

## Code Navigation Tips

### Finding Things

**Command implementation**: Look in `cmd/`
- `copy.go` - Copy command
- `sync.go` - Sync command
- `list.go`, `make.go`, `remove.go` - Other commands

**Transfer logic**: Look in `ste/`
- `downloader-*.go` - Download implementations
- `uploader-*.go` - Upload implementations
- `sender-*.go` - Service-to-service copy

**Source enumeration**: Look for `traverser` and `enumerator`
- `cmd/zc_traverser_*.go` - Traversers for each source type
- `cmd/*Enumerator*.go` - Enumeration logic

**Authentication**: Look in `common/`
- `oauthTokenManager.go` - OAuth token handling
- `credentialFactory*.go` - Credential creation

### Common Patterns

**Error Handling**:
```go
if err != nil {
    return fmt.Errorf("failed to <action>: %w", err)
}
```

**Resource Cleanup**:
```go
defer file.Close()
defer body.Close()
```

**Context Usage**:
```go
ctx, cancel := context.WithTimeout(parentCtx, timeout)
defer cancel()
```

## Debugging Tips

### Transfer Issues
1. Check traverser/enumerator for source enumeration
2. Verify job plan creation in STE
3. Examine downloader/uploader logs
4. Check credential handling and renewal

### Test Failures
1. Look for resource cleanup issues
2. Check for timing/race conditions in parallel tests
3. Verify mock setup in unit tests
4. Check environment variables for e2e tests

### Performance Problems
1. Profile with `pprof`
2. Check chunk size configuration
3. Monitor CPU and memory usage
4. Analyze network utilization

## Gotchas

### Platform Differences
- File paths: Use `filepath.Join()`, not string concatenation
- Line endings: Different on Windows vs Unix
- File permissions: Platform-specific handling

### Testing
- E2E tests require Azure credentials
- Some tests need specific environment variables
- Long timeouts needed for large transfer tests
- Clean up test resources to avoid quota issues

### Azure SDK
- Check API version compatibility
- Handle continuation tokens correctly
- Respect rate limits and retry policies
- Monitor for SDK updates and breaking changes

## Best Practices

### Code Style
- Follow standard Go conventions
- Use meaningful variable names
- Comment complex logic
- Keep functions focused and small

### Error Messages
- Be specific about what failed
- Include relevant context (file name, operation)
- Suggest fixes when possible
- Don't expose sensitive information (tokens, keys)

### Testing
- Write table-driven tests for multiple cases
- Test error paths, not just happy path
- Clean up resources in test teardown
- Use subtests for better organization

---

*Update this file with new learnings and discoveries*
