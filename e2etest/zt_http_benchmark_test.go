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
	"bytes"
	"crypto/rand"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// findAzCopyBinaryForBenchmark finds the AzCopy binary for benchmarks
func findAzCopyBinaryForBenchmark(b *testing.B) string {
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
	b.Logf("AzCopy binary not found, attempting to build...")
	tmpBinary := filepath.Join(os.TempDir(), "azcopy_benchmark")

	buildCmd := exec.Command("go", "build", "-o", tmpBinary, ".")
	if err := buildCmd.Run(); err != nil {
		b.Fatalf("Failed to build AzCopy: %v", err)
	}

	return tmpBinary
}

// BenchmarkHTTPDownload_SmallFile benchmarks downloading a small file (1 MB)
func BenchmarkHTTPDownload_SmallFile(b *testing.B) {
	benchmarkHTTPDownload(b, 1*1024*1024, true) // 1 MB
}

// BenchmarkHTTPDownload_MediumFile benchmarks downloading a medium file (50 MB)
func BenchmarkHTTPDownload_MediumFile(b *testing.B) {
	benchmarkHTTPDownload(b, 50*1024*1024, true) // 50 MB
}

// BenchmarkHTTPDownload_LargeFile benchmarks downloading a large file (500 MB)
func BenchmarkHTTPDownload_LargeFile(b *testing.B) {
	benchmarkHTTPDownload(b, 500*1024*1024, true) // 500 MB
}

// BenchmarkHTTPDownload_SmallFile_NoRange benchmarks small file without range support
func BenchmarkHTTPDownload_SmallFile_NoRange(b *testing.B) {
	benchmarkHTTPDownload(b, 1*1024*1024, false) // 1 MB
}

// BenchmarkHTTPDownload_MediumFile_NoRange benchmarks medium file without range support
func BenchmarkHTTPDownload_MediumFile_NoRange(b *testing.B) {
	benchmarkHTTPDownload(b, 50*1024*1024, false) // 50 MB
}

// BenchmarkHTTPDownload_BlockSize_8MB benchmarks with 8 MB block size
func BenchmarkHTTPDownload_BlockSize_8MB(b *testing.B) {
	benchmarkHTTPDownloadWithBlockSize(b, 100*1024*1024, 8) // 100 MB file, 8 MB blocks
}

// BenchmarkHTTPDownload_BlockSize_16MB benchmarks with 16 MB block size
func BenchmarkHTTPDownload_BlockSize_16MB(b *testing.B) {
	benchmarkHTTPDownloadWithBlockSize(b, 100*1024*1024, 16) // 100 MB file, 16 MB blocks
}

// BenchmarkHTTPDownload_BlockSize_32MB benchmarks with 32 MB block size
func BenchmarkHTTPDownload_BlockSize_32MB(b *testing.B) {
	benchmarkHTTPDownloadWithBlockSize(b, 100*1024*1024, 32) // 100 MB file, 32 MB blocks
}

// BenchmarkHTTPDownload_BandwidthCap_100Mbps benchmarks with 100 Mbps cap
func BenchmarkHTTPDownload_BandwidthCap_100Mbps(b *testing.B) {
	benchmarkHTTPDownloadWithBandwidthCap(b, 50*1024*1024, 100) // 50 MB file, 100 Mbps cap
}

// BenchmarkHTTPDownload_BandwidthCap_500Mbps benchmarks with 500 Mbps cap
func BenchmarkHTTPDownload_BandwidthCap_500Mbps(b *testing.B) {
	benchmarkHTTPDownloadWithBandwidthCap(b, 50*1024*1024, 500) // 50 MB file, 500 Mbps cap
}

// BenchmarkHTTPDownload_Parallel benchmarks parallel downloads
func BenchmarkHTTPDownload_Parallel(b *testing.B) {
	// Generate test data
	testData := make([]byte, 10*1024*1024) // 10 MB
	_, err := rand.Read(testData)
	if err != nil {
		b.Fatalf("Failed to generate test data: %v", err)
	}

	// Create mock server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Accept-Ranges", "bytes")
		w.Header().Set("Content-Length", fmt.Sprintf("%d", len(testData)))

		rangeHeader := r.Header.Get("Range")
		if rangeHeader != "" {
			// Serve partial content
			w.WriteHeader(http.StatusPartialContent)
		}
		w.Write(testData)
	}))
	defer server.Close()

	azcopyPath := findAzCopyBinaryForBenchmark(b)

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			tmpDir, err := os.MkdirTemp("", "azcopy-bench-parallel-*")
			if err != nil {
				b.Fatalf("Failed to create temp dir: %v", err)
			}
			defer os.RemoveAll(tmpDir)

			targetPath := filepath.Join(tmpDir, "test.bin")

			cmd := exec.Command(azcopyPath, "copy", server.URL, targetPath, "--log-level=ERROR")
			if err := cmd.Run(); err != nil {
				b.Errorf("Download failed: %v", err)
			}
		}
	})
}

// benchmarkHTTPDownload is a helper function for HTTP download benchmarks
func benchmarkHTTPDownload(b *testing.B, fileSize int64, supportsRange bool) {
	// Generate test data
	testData := make([]byte, fileSize)
	_, err := rand.Read(testData)
	if err != nil {
		b.Fatalf("Failed to generate test data: %v", err)
	}

	// Create mock HTTP server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if supportsRange {
			w.Header().Set("Accept-Ranges", "bytes")
		}
		w.Header().Set("Content-Length", fmt.Sprintf("%d", len(testData)))

		rangeHeader := r.Header.Get("Range")
		if rangeHeader != "" && supportsRange {
			// Serve partial content
			w.WriteHeader(http.StatusPartialContent)
			// For simplicity in benchmark, just serve full content
		}

		// Stream data to avoid memory spikes
		chunk := 8 * 1024 * 1024 // 8 MB chunks
		for offset := 0; offset < len(testData); offset += chunk {
			end := offset + chunk
			if end > len(testData) {
				end = len(testData)
			}
			w.Write(testData[offset:end])
		}
	}))
	defer server.Close()

	azcopyPath := findAzCopyBinaryForBenchmark(b)

	// Create temp directory
	tmpDir, err := os.MkdirTemp("", "azcopy-benchmark-*")
	if err != nil {
		b.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	b.ResetTimer()
	b.SetBytes(fileSize)

	for i := 0; i < b.N; i++ {
		targetPath := filepath.Join(tmpDir, fmt.Sprintf("test_%d.bin", i))

		cmd := exec.Command(
			azcopyPath,
			"copy",
			server.URL,
			targetPath,
			"--log-level=ERROR",
			"--output-type=text",
		)

		output, err := cmd.CombinedOutput()
		if err != nil {
			b.Fatalf("Download failed: %v\nOutput: %s", err, string(output))
		}

		// Verify file size
		fileInfo, err := os.Stat(targetPath)
		if err != nil {
			b.Fatalf("Failed to stat file: %v", err)
		}
		if fileInfo.Size() != fileSize {
			b.Fatalf("File size mismatch: expected %d, got %d", fileSize, fileInfo.Size())
		}

		// Clean up after each iteration to avoid disk space issues
		os.Remove(targetPath)
	}
}

// benchmarkHTTPDownloadWithBlockSize benchmarks downloads with specific block sizes
func benchmarkHTTPDownloadWithBlockSize(b *testing.B, fileSize int64, blockSizeMB int) {
	// Generate test data
	testData := make([]byte, fileSize)
	_, err := rand.Read(testData)
	if err != nil {
		b.Fatalf("Failed to generate test data: %v", err)
	}

	// Create mock HTTP server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Accept-Ranges", "bytes")
		w.Header().Set("Content-Length", fmt.Sprintf("%d", len(testData)))

		rangeHeader := r.Header.Get("Range")
		if rangeHeader != "" {
			w.WriteHeader(http.StatusPartialContent)
		}

		// Stream data
		chunk := 8 * 1024 * 1024
		for offset := 0; offset < len(testData); offset += chunk {
			end := offset + chunk
			if end > len(testData) {
				end = len(testData)
			}
			w.Write(testData[offset:end])
		}
	}))
	defer server.Close()

	azcopyPath := findAzCopyBinaryForBenchmark(b)

	tmpDir, err := os.MkdirTemp("", "azcopy-benchmark-blocksize-*")
	if err != nil {
		b.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	b.ResetTimer()
	b.SetBytes(fileSize)

	for i := 0; i < b.N; i++ {
		targetPath := filepath.Join(tmpDir, fmt.Sprintf("test_%d.bin", i))

		cmd := exec.Command(
			azcopyPath,
			"copy",
			server.URL,
			targetPath,
			"--log-level=ERROR",
			fmt.Sprintf("--block-size-mb=%d", blockSizeMB),
		)

		if err := cmd.Run(); err != nil {
			b.Fatalf("Download failed: %v", err)
		}

		os.Remove(targetPath)
	}
}

// benchmarkHTTPDownloadWithBandwidthCap benchmarks downloads with bandwidth limits
func benchmarkHTTPDownloadWithBandwidthCap(b *testing.B, fileSize int64, capMbps int) {
	// Generate test data
	testData := make([]byte, fileSize)
	_, err := rand.Read(testData)
	if err != nil {
		b.Fatalf("Failed to generate test data: %v", err)
	}

	// Create mock HTTP server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Accept-Ranges", "bytes")
		w.Header().Set("Content-Length", fmt.Sprintf("%d", len(testData)))

		rangeHeader := r.Header.Get("Range")
		if rangeHeader != "" {
			w.WriteHeader(http.StatusPartialContent)
		}

		w.Write(testData)
	}))
	defer server.Close()

	azcopyPath := findAzCopyBinaryForBenchmark(b)

	tmpDir, err := os.MkdirTemp("", "azcopy-benchmark-bandwidth-*")
	if err != nil {
		b.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	b.ResetTimer()
	b.SetBytes(fileSize)

	for i := 0; i < b.N; i++ {
		targetPath := filepath.Join(tmpDir, fmt.Sprintf("test_%d.bin", i))

		cmd := exec.Command(
			azcopyPath,
			"copy",
			server.URL,
			targetPath,
			"--log-level=ERROR",
			fmt.Sprintf("--cap-mbps=%d", capMbps),
		)

		if err := cmd.Run(); err != nil {
			b.Fatalf("Download failed: %v", err)
		}

		os.Remove(targetPath)
	}
}

// BenchmarkHTTPDownload_RealWorld_SmallFile benchmarks downloading real small file
func BenchmarkHTTPDownload_RealWorld_SmallFile(b *testing.B) {
	if !*enableRealHTTPTests {
		b.Skip("Skipping real-world benchmark. Use -enable-real-http-tests to run.")
	}

	const sourceURL = "https://aka.ms/downloadazcopy-v10-linux"

	azcopyPath := findAzCopyBinaryForBenchmark(b)

	tmpDir, err := os.MkdirTemp("", "azcopy-benchmark-real-*")
	if err != nil {
		b.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		targetPath := filepath.Join(tmpDir, fmt.Sprintf("azcopy_%d.tar.gz", i))

		cmd := exec.Command(
			azcopyPath,
			"copy",
			sourceURL,
			targetPath,
			"--log-level=ERROR",
		)

		if err := cmd.Run(); err != nil {
			b.Fatalf("Download failed: %v", err)
		}

		fileInfo, err := os.Stat(targetPath)
		if err != nil {
			b.Fatalf("Failed to stat file: %v", err)
		}

		b.SetBytes(fileInfo.Size())
		os.Remove(targetPath)
	}
}

// BenchmarkHTTPTraverser_HEAD benchmarks HTTP traverser HEAD request performance
func BenchmarkHTTPTraverser_HEAD(b *testing.B) {
	// Create mock server with minimal response
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "HEAD" {
			w.Header().Set("Accept-Ranges", "bytes")
			w.Header().Set("Content-Length", "10485760") // 10 MB
			w.Header().Set("Content-Type", "application/octet-stream")
			w.Header().Set("ETag", "abc123")
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer server.Close()

	client := &http.Client{}

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		req, _ := http.NewRequest("HEAD", server.URL, nil)
		resp, err := client.Do(req)
		if err != nil {
			b.Fatalf("HEAD request failed: %v", err)
		}
		resp.Body.Close()
	}
}

// BenchmarkHTTPDownload_ChunkWrite benchmarks chunk writing performance
func BenchmarkHTTPDownload_ChunkWrite(b *testing.B) {
	chunkSize := 8 * 1024 * 1024 // 8 MB
	chunk := make([]byte, chunkSize)
	_, err := rand.Read(chunk)
	if err != nil {
		b.Fatalf("Failed to generate chunk: %v", err)
	}

	tmpDir, err := os.MkdirTemp("", "azcopy-benchmark-chunk-*")
	if err != nil {
		b.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	b.ResetTimer()
	b.SetBytes(int64(chunkSize))

	for i := 0; i < b.N; i++ {
		targetPath := filepath.Join(tmpDir, fmt.Sprintf("chunk_%d.bin", i))

		file, err := os.Create(targetPath)
		if err != nil {
			b.Fatalf("Failed to create file: %v", err)
		}

		_, err = file.Write(chunk)
		if err != nil {
			b.Fatalf("Failed to write chunk: %v", err)
		}

		file.Close()
		os.Remove(targetPath)
	}
}

// BenchmarkHTTPDownload_MemoryAllocation benchmarks memory allocation patterns
func BenchmarkHTTPDownload_MemoryAllocation(b *testing.B) {
	sizes := []int{
		1 * 1024 * 1024,      // 1 MB
		8 * 1024 * 1024,      // 8 MB
		16 * 1024 * 1024,     // 16 MB
		32 * 1024 * 1024,     // 32 MB
	}

	for _, size := range sizes {
		b.Run(fmt.Sprintf("Alloc_%dMB", size/(1024*1024)), func(b *testing.B) {
			b.ReportAllocs()
			b.SetBytes(int64(size))

			for i := 0; i < b.N; i++ {
				buffer := make([]byte, size)
				_ = buffer
			}
		})
	}
}

// BenchmarkHTTPDownload_BufferPooling benchmarks buffer reuse
func BenchmarkHTTPDownload_BufferPooling(b *testing.B) {
	chunkSize := 8 * 1024 * 1024 // 8 MB

	b.Run("WithoutPool", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			buffer := make([]byte, chunkSize)
			_ = buffer
		}
	})

	b.Run("WithPool", func(b *testing.B) {
		b.ReportAllocs()
		bufferPool := &bytes.Buffer{}

		for i := 0; i < b.N; i++ {
			bufferPool.Reset()
			bufferPool.Grow(chunkSize)
			_ = bufferPool.Bytes()
		}
	})
}