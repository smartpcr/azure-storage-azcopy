# AzCopy Memory Index

Last updated: 2025-11-18

## Active Investigations

None

## Recent Decisions

### 2025-11-18: HTTP Download Redirect Handling Fix
**Status**: Resolved
**Impact**: All HTTP downloads from redirect URLs (aka.ms, bit.ly, etc.)
**Details**: [2025-11-18/bugfix-http-head-request-redirect-handling.md](2025-11-18/bugfix-http-head-request-redirect-handling.md)

Fixed HTTP downloads failing from redirect URLs by improving HEAD request handling and adding GET fallback for servers that don't support HEAD properly. Tested successfully with 3.5GB download from aka.ms.

### 2025-11-18: HTTP E2E Test Permission Fix
**Status**: Resolved
**Impact**: All HTTP e2e tests
**Details**: [2025-11-18/bugfix-http-e2e-test-permissions.md](2025-11-18/bugfix-http-e2e-test-permissions.md)

Fixed "permission denied" errors in HTTP e2e tests by ensuring azcopy binary has execute permissions after build. Added documentation for both local development and CI/CD pipeline fixes.

## Known Issues

### HTTP E2E Tests Require Execute Permissions
- **Issue**: Binary built without execute permissions causes test failures
- **Workaround**: `chmod +x azure-storage-azcopy && cp azure-storage-azcopy /tmp/azcopy_test`
- **Permanent Fix**: Update CI/CD pipeline to set permissions after build
- **Reference**: [2025-11-18/bugfix-http-e2e-test-permissions.md](2025-11-18/bugfix-http-e2e-test-permissions.md)

## Quick Links

- [Architecture Decisions](context/architecture-decisions.md)
- [Known Issues](context/known-issues.md)
- [Optimization Opportunities](context/optimization-opportunities.md)

---

## How to Use This Index

1. Add new memory entries to dated folders
2. Update this index with key entries
3. Link related entries together
4. Mark items as resolved when complete
5. Search this file to find relevant historical context
