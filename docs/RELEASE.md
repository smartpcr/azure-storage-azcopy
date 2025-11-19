# Release Process

AzCopy uses automated releases triggered by git tags with [GoReleaser](https://goreleaser.com/).

## Creating a New Release

### Automated Release (Recommended)

1. **Update the version** in `common/version.go`:
   ```go
   const AzcopyVersion = "10.32.0"  // Update this - must match the tag version
   ```

   **Important**: The version in `common/version.go` should match your git tag (without the `v` prefix).

2. **Commit and push the version change**:
   ```bash
   git add common/version.go
   git commit -m "chore: bump version to 10.32.0"
   git push origin main
   ```

3. **Create and push a git tag**:
   ```bash
   # Create the tag (version must match common/version.go)
   git tag -a v10.32.0 -m "Release v10.32.0"

   # Push the tag
   git push origin v10.32.0
   ```

4. **Automatic release**: The `release-automated.yml` workflow will automatically:
   - Run all tests
   - Build binaries for all platforms (Linux, Windows, macOS for AMD64 and ARM64)
   - Generate checksums
   - Create a GitHub release with all artifacts
   - Generate changelog from commits

### Version Formats

The automated release supports various version formats:

- **Stable releases**: `v10.32.0`
- **Preview releases**: `v10.32.0-preview.1` (automatically marked as pre-release)
- **Beta releases**: `v10.32.0-beta.1` (automatically marked as pre-release)
- **Alpha releases**: `v10.32.0-alpha.1` (automatically marked as pre-release)

### Manual Release (Fallback)

For special cases where the automated release doesn't fit your needs, you can use the manual workflow:

1. Go to **Actions** → **Manual Release**
2. Click **Run workflow**
3. Optionally specify:
   - Release version (defaults to version from `common/version.go`)
   - Custom release notes

## Testing Locally with GoReleaser

You can test the release configuration locally:

```bash
# Install goreleaser
go install github.com/goreleaser/goreleaser/v2@latest

# Test the build without publishing
goreleaser release --snapshot --clean --skip=publish

# Check the generated artifacts
ls -lh dist/
```

## Release Checklist

Before creating a release:

- [ ] All tests pass in CI
- [ ] Version updated in `common/version.go`
- [ ] Breaking changes documented
- [ ] CHANGELOG updated (optional - GoReleaser auto-generates from commits)
- [ ] No open critical bugs

## GoReleaser Configuration

The release configuration is in `.goreleaser.yaml`:

- **Builds**: Compiles binaries for Linux, Windows, macOS (AMD64 and ARM64)
- **Archives**: Creates `.tar.gz` for Linux/macOS and `.zip` for Windows
- **Checksums**: Generates SHA256 checksums
- **Changelog**: Auto-generated from git commits using conventional commits

## Commit Message Format for Better Changelogs

Use conventional commits for better automatic changelog generation:

- `feat: add new feature` → "Features" section
- `fix: resolve bug` → "Bug Fixes" section
- `perf: improve performance` → "Performance Improvements" section
- `docs: update documentation` → Excluded from changelog
- `test: add tests` → Excluded from changelog
- `ci: update CI config` → Excluded from changelog

## Troubleshooting

### Release failed during tests

Check the test logs in the GitHub Actions workflow. The automated release runs the same tests as the CI pipeline.

### Tag already exists

If you need to re-release:
```bash
# Delete local tag
git tag -d v10.32.0

# Delete remote tag
git push origin :refs/tags/v10.32.0

# Create new tag
git tag -a v10.32.0 -m "Release v10.32.0"
git push origin v10.32.0
```

### GoReleaser fails to build

Test locally first:
```bash
goreleaser release --snapshot --clean --skip=publish
```

## Support

For issues with the release process, check:
- [GoReleaser Documentation](https://goreleaser.com/)
- [GitHub Actions Documentation](https://docs.github.com/en/actions)
- [Project Issues](https://github.com/Azure/azure-storage-azcopy/issues)
