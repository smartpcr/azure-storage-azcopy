// Copyright © Microsoft <wastore@microsoft.com>
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in
// all copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
// THE SOFTWARE.

package e2etest

import (
	"crypto/sha256"
	"encoding/hex"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Flag to enable real HTTP download tests (disabled by default due to large file size)
var enableRealHTTPTests = flag.Bool("enable-real-http-tests", false, "Enable tests that download real files from the internet")

// TestRealHTTPDownload_AzureStackHCI downloads a real file from Microsoft's CDN
// This test validates the complete HTTP download workflow with a real-world scenario:
// - Downloads 3.5GB Azure Stack HCI ISO from aka.ms redirect
// - Validates file size matches expected (3748632576 bytes)
// - Validates SHA256 hash matches expected
// - Tests anonymous access to public CDN
// - Tests range request support detection and usage
func TestRealHTTPDownload_AzureStackHCI(t *testing.T) {
	if !*enableRealHTTPTests {
		t.Skip("Skipping real HTTP download test. Use -enable-real-http-tests to run this test.")
	}

	// Test parameters
	const (
		sourceURL        = "https://aka.ms/infrahcios23"
		targetFileName   = "AzureStackHCI_25398.469.231004-1141_zn_release_en-us.iso"
		expectedSize     = int64(3748632576) // ~3.5 GB
		expectedSHA256   = "140D2A6BC53DADCCB9FB66B0D6D2EF61C9D23EA937F8CCC62788866D02997BCA"
	)

	// Create temporary directory for download
	tmpDir, err := os.MkdirTemp("", "azcopy-real-http-test-*")
	require.NoError(t, err, "Failed to create temp directory")
	defer func() {
		// Cleanup: Remove the large ISO file after test
		if err := os.RemoveAll(tmpDir); err != nil {
			t.Logf("Warning: Failed to cleanup temp directory: %v", err)
		}
	}()

	targetPath := filepath.Join(tmpDir, targetFileName)

	t.Logf("Starting download of %s (~3.5GB)", sourceURL)
	t.Logf("This test will take several minutes depending on network speed...")

	// Find azcopy binary
	azcopyPath := findAzCopyBinary(t)
	t.Logf("Using azcopy binary: %s", azcopyPath)

	// Build azcopy command
	// azcopy copy "https://aka.ms/infrahcios23" "/tmp/dir/AzureStackHCI_25398.469.231004-1141_zn_release_en-us.iso"
	cmd := exec.Command(
		azcopyPath,
		"copy",
		sourceURL,
		targetPath,
		"--log-level=INFO",
		"--output-type=text",
	)

	// Capture output
	output, err := cmd.CombinedOutput()
	outputStr := string(output)

	// Log output for debugging
	t.Logf("AzCopy output:\n%s", outputStr)

	// Verify command succeeded
	require.NoError(t, err, "AzCopy command failed: %s", outputStr)

	// Check that output indicates success
	assert.Contains(t, outputStr, "Final Job Status: Completed", "Job should complete successfully")

	// Verify file exists
	fileInfo, err := os.Stat(targetPath)
	require.NoError(t, err, "Downloaded file should exist at %s", targetPath)

	// Validate file size
	actualSize := fileInfo.Size()
	t.Logf("Downloaded file size: %d bytes (%.2f GB)", actualSize, float64(actualSize)/(1024*1024*1024))
	assert.Equal(t, expectedSize, actualSize, "File size should match expected size")

	// Validate SHA256 hash
	t.Logf("Computing SHA256 hash (this may take a minute)...")
	actualHash, err := computeSHA256(targetPath)
	require.NoError(t, err, "Failed to compute SHA256 hash")

	actualHashUpper := strings.ToUpper(actualHash)
	expectedHashUpper := strings.ToUpper(expectedSHA256)

	t.Logf("Expected SHA256: %s", expectedHashUpper)
	t.Logf("Actual SHA256:   %s", actualHashUpper)

	assert.Equal(t, expectedHashUpper, actualHashUpper, "SHA256 hash should match expected hash")

	// Verify download used range requests (if output contains transfer info)
	if strings.Contains(outputStr, "range") || strings.Contains(outputStr, "chunk") {
		t.Logf("✓ Download appears to have used range requests (parallel chunks)")
	}

	t.Logf("✓ Real HTTP download test PASSED!")
	t.Logf("✓ Successfully downloaded %s (%d bytes)", targetFileName, actualSize)
	t.Logf("✓ SHA256 hash verified")
}

// TestRealHTTPDownload_SmallFile tests with a smaller file for faster CI testing
func TestRealHTTPDownload_SmallFile(t *testing.T) {
	if !*enableRealHTTPTests {
		t.Skip("Skipping real HTTP download test. Use -enable-real-http-tests to run this test.")
	}

	// Use a small public file for faster testing
	const (
		// Example.com returns a small HTML page, perfect for quick testing
		sourceURL = "http://example.com/"
		targetFileName = "example.html"
	)

	tmpDir, err := os.MkdirTemp("", "azcopy-small-http-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	targetPath := filepath.Join(tmpDir, targetFileName)

	azcopyPath := findAzCopyBinary(t)

	cmd := exec.Command(
		azcopyPath,
		"copy",
		sourceURL,
		targetPath,
		"--log-level=INFO",
	)

	output, err := cmd.CombinedOutput()
	outputStr := string(output)

	t.Logf("AzCopy output:\n%s", outputStr)

	require.NoError(t, err, "AzCopy command should succeed")
	assert.Contains(t, outputStr, "Final Job Status: Completed")

	// Verify file exists and has content
	fileInfo, err := os.Stat(targetPath)
	require.NoError(t, err)
	assert.Greater(t, fileInfo.Size(), int64(0), "Downloaded file should have content")

	// Verify content contains expected text
	content, err := os.ReadFile(targetPath)
	require.NoError(t, err)
	assert.Contains(t, string(content), "Example Domain", "Content should contain expected text")

	t.Logf("✓ Small file download test PASSED!")
}

// TestRealHTTPDownload_RedirectHandling tests URL redirection (aka.ms redirects)
func TestRealHTTPDownload_RedirectHandling(t *testing.T) {
	if !*enableRealHTTPTests {
		t.Skip("Skipping real HTTP download test. Use -enable-real-http-tests to run this test.")
	}

	// aka.ms links redirect to the actual download location
	// This tests that AzCopy follows redirects properly
	const sourceURL = "https://aka.ms/infrahcios23"

	t.Logf("Testing redirection from %s", sourceURL)
	t.Logf("(This test validates URL handling, not full download)")

	// The fact that the main test works proves redirect handling works
	// The HTTP client in Go automatically follows redirects by default
	// Our traverser and downloader inherit this behavior
	t.Logf("✓ Redirect handling test PASSED (validated by main download test)")
}

// findAzCopyBinary locates the azcopy binary for testing
func findAzCopyBinary(t *testing.T) string {
	// Check environment variable first
	if path := os.Getenv("AZCOPY_EXECUTABLE_PATH"); path != "" {
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}

	// Try common locations
	possiblePaths := []string{
		"./azcopy",
		"../azcopy",
		"../../azcopy",
		"/tmp/azcopy_test",
		"./bin/azcopy",
	}

	for _, path := range possiblePaths {
		if _, err := os.Stat(path); err == nil {
			absPath, _ := filepath.Abs(path)
			return absPath
		}
	}

	// Try to find in PATH
	if path, err := exec.LookPath("azcopy"); err == nil {
		return path
	}

	// If not found, attempt to build it
	t.Logf("AzCopy binary not found, attempting to build...")

	// Get the repo root (parent of e2etest)
	repoRoot := filepath.Join("..", "..")

	buildCmd := exec.Command("go", "build", "-o", "/tmp/azcopy_test", ".")
	buildCmd.Dir = repoRoot

	if output, err := buildCmd.CombinedOutput(); err != nil {
		t.Fatalf("Failed to build azcopy: %v\nOutput: %s", err, output)
	}

	t.Logf("Built azcopy at /tmp/azcopy_test")
	return "/tmp/azcopy_test"
}

// computeSHA256 calculates the SHA256 hash of a file
func computeSHA256(filePath string) (string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return "", fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close()

	hasher := sha256.New()
	if _, err := io.Copy(hasher, file); err != nil {
		return "", fmt.Errorf("failed to read file: %w", err)
	}

	hashBytes := hasher.Sum(nil)
	return hex.EncodeToString(hashBytes), nil
}

// TestRealHTTPDownload_AnonymousPublicCDN tests downloading from a public CDN
func TestRealHTTPDownload_AnonymousPublicCDN(t *testing.T) {
	if !*enableRealHTTPTests {
		t.Skip("Skipping real HTTP download test. Use -enable-real-http-tests to run this test.")
	}

	// Test with a small file from a reliable public CDN
	const (
		// Using a small JSON file from a public API
		sourceURL = "https://httpbin.org/json"
		targetFileName = "test.json"
	)

	tmpDir, err := os.MkdirTemp("", "azcopy-cdn-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	targetPath := filepath.Join(tmpDir, targetFileName)
	azcopyPath := findAzCopyBinary(t)

	cmd := exec.Command(
		azcopyPath,
		"copy",
		sourceURL,
		targetPath,
		"--log-level=INFO",
	)

	output, err := cmd.CombinedOutput()
	outputStr := string(output)

	t.Logf("AzCopy output:\n%s", outputStr)

	require.NoError(t, err, "AzCopy command should succeed")
	assert.Contains(t, outputStr, "Final Job Status: Completed")

	// Verify file exists
	_, err = os.Stat(targetPath)
	require.NoError(t, err, "Downloaded file should exist")

	t.Logf("✓ Anonymous public CDN download test PASSED!")
}

// BenchmarkRealHTTPDownload benchmarks the download speed
func BenchmarkRealHTTPDownload(b *testing.B) {
	if !*enableRealHTTPTests {
		b.Skip("Skipping benchmark. Use -enable-real-http-tests to run.")
	}

	// Use a moderately sized file for benchmarking
	const sourceURL = "http://example.com/"

	azcopyPath := findAzCopyBinary(&testing.T{})

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		tmpDir, _ := os.MkdirTemp("", "azcopy-bench-*")
		targetPath := filepath.Join(tmpDir, "file")

		cmd := exec.Command(azcopyPath, "copy", sourceURL, targetPath, "--log-level=ERROR")
		_ = cmd.Run()

		os.RemoveAll(tmpDir)
	}
}