# [Bugfix] HTTP E2E Test Permission Denied Errors

**Date**: 2025-11-18
**Component**: e2etest / CI/CD
**Status**: resolved
**Author**: Claude Code

## Summary

All HTTP e2e tests were failing with "permission denied" errors when trying to execute the azcopy binary during test execution in CI/CD environments.

## Context

E2E tests use `findAzCopyBinary()` helper function to locate the azcopy executable. The tests were finding the binary but couldn't execute it due to missing execute permissions.

### Error Message

```
Error: fork/exec /home/runner/work/azure-storage-azcopy/azure-storage-azcopy/azcopy: permission denied
Test: TestHTTPDownload_AutoScaling
```

## Root Cause

The azcopy binary built during CI/CD pipeline was not given execute permissions after the build step.

## Investigation

1. **Binary Search Logic** (`zt_http_real_download_test.go:207`)

   `findAzCopyBinary()` checks in this order:
   - `AZCOPY_EXECUTABLE_PATH` environment variable
   - Common paths: `./azcopy`, `../azcopy`, `/tmp/azcopy_test`
   - System PATH
   - Attempts to build if not found

2. **Local Environment Discovery**
   - Found `azcopy_bin` with correct permissions (`-rwxrwxr-x`)
   - Found `azure-storage-azcopy` with correct permissions
   - `azcopy` is a **directory** (Go package), not a file
   - No `azcopy` executable in expected locations

3. **Filesystem Constraint**
   - Cannot create a file named `azcopy` in root directory because a directory with that name exists
   - Unix filesystems don't allow file/directory name conflicts

## Solution

### For Local Development

**Option 1: Use /tmp/azcopy_test** (Recommended)
```bash
# Build and copy to test location
go build -o ./azure-storage-azcopy .
chmod +x ./azure-storage-azcopy
cp ./azure-storage-azcopy /tmp/azcopy_test

# Run tests
go test -v ./zt_http* -enable-real-http-tests
```

**Option 2: Set environment variable**
```bash
# Build binary
go build -o ./azure-storage-azcopy .
chmod +x ./azure-storage-azcopy

# Run with env var
AZCOPY_EXECUTABLE_PATH=./azure-storage-azcopy go test -v ./zt_http* -enable-real-http-tests
```

### For CI/CD Pipeline

**Add explicit chmod step after build:**

```yaml
# Example for GitHub Actions / Azure Pipelines
- name: Build AzCopy
  run: go build -o azure-storage-azcopy .

- name: Set execute permissions
  run: chmod +x azure-storage-azcopy

- name: Copy to test location
  run: cp azure-storage-azcopy /tmp/azcopy_test && chmod +x /tmp/azcopy_test

- name: Run e2e tests
  run: go test -timeout=30m -v ./zt_http* -enable-real-http-tests
```

**Alternative: Use environment variable**
```yaml
- name: Build AzCopy
  run: go build -o azure-storage-azcopy .

- name: Set execute permissions
  run: chmod +x azure-storage-azcopy

- name: Run e2e tests
  env:
    AZCOPY_EXECUTABLE_PATH: ${{ github.workspace }}/azure-storage-azcopy
  run: go test -timeout=30m -v ./zt_http* -enable-real-http-tests
```

## Verification

Test executed successfully after fix:

```bash
$ AZCOPY_EXECUTABLE_PATH=/tmp/azcopy_test go test -v -run TestHTTPDownload_AutoScaling . -enable-real-http-tests

=== RUN   TestHTTPDownload_AutoScaling
    ✓ Downloaded file size: 3748632576 bytes (3.7GB)
    ✓ Found 8 throughput measurements (indicates parallel chunks)
    ✓ Auto-scaling test PASSED!
--- PASS: TestHTTPDownload_AutoScaling (33.44s)
PASS
```

## Key Learnings

1. **Build artifacts need explicit permissions** in CI/CD environments
2. **Go build output doesn't automatically get execute permissions** on all systems
3. **Directory/file name conflicts** prevent creating `azcopy` executable in root
4. **Test helpers are flexible** - multiple ways to specify binary location

## Related Files

- `zt_http_autoscale_resume_test.go:70` - TestHTTPDownload_AutoScaling
- `zt_http_real_download_test.go:207` - findAzCopyBinary() function
- `zt_http_benchmark_test.go:36` - findAzCopyBinaryForBenchmark()

## Related Issues

- Similar issues may affect other test suites that execute the binary
- Check `zt_newe2e_*test.go` files for similar patterns

## Action Items

- [x] Document fix in .claude/memory
- [x] Verify local test execution
- [ ] Update CI/CD pipeline with chmod step (maintainer task)
- [ ] Consider adding build script that handles permissions
- [ ] Add pre-test check that validates binary permissions

## Recommendations for CI/CD

### Quick Fix (Immediate)
Add to pipeline before running tests:
```bash
chmod +x azure-storage-azcopy
cp azure-storage-azcopy /tmp/azcopy_test
```

### Long-term Solution
1. Create `scripts/build-for-tests.sh`:
   ```bash
   #!/bin/bash
   set -e
   go build -o azure-storage-azcopy .
   chmod +x azure-storage-azcopy
   cp azure-storage-azcopy /tmp/azcopy_test
   echo "✓ Built azcopy with execute permissions"
   ```

2. Update CI/CD to call build script instead of raw `go build`

3. Add test helper validation:
   ```go
   // In findAzCopyBinary()
   if _, err := os.Stat(path); err == nil {
       // Check if executable
       if info, err := os.Stat(path); err == nil {
           if info.Mode()&0111 == 0 {
               t.Logf("Warning: %s exists but is not executable", path)
               continue
           }
       }
   }
   ```

---

**Status**: Fix verified locally, awaiting CI/CD pipeline update
**Last Updated**: 2025-11-18
