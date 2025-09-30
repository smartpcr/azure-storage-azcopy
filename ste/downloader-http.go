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

package ste

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/http"
	"time"

	"github.com/Azure/azure-storage-azcopy/v10/common"
)

type httpDownloader struct {
	jptm            IJobPartTransferMgr
	sourceURL       string
	httpClient      *http.Client
	bearerToken     string
	supportsRange   bool
	contentLength   int64
	expectedMD5     []byte
	etag            string
}

func newHTTPDownloader(jptm IJobPartTransferMgr) (downloader, error) {
	info := jptm.Info()

	// Create HTTP client with appropriate timeouts
	client := &http.Client{
		Timeout: 30 * time.Minute, // Long timeout for large downloads
		Transport: &http.Transport{
			MaxIdleConns:        100,
			MaxIdleConnsPerHost: 100,
			IdleConnTimeout:     90 * time.Second,
			DisableCompression:  true, // Disable to get accurate byte counts
		},
	}

	// Get bearer token from credential if available
	// For Phase 3, we'll access it via jptm methods in Prologue
	// This will be enhanced in Phase 4 with CLI integration

	return &httpDownloader{
		sourceURL:  info.Source,
		httpClient: client,
	}, nil
}

func (hd *httpDownloader) Prologue(jptm IJobPartTransferMgr) {
	hd.jptm = jptm

	// Perform HEAD request to detect server capabilities
	err := hd.detectCapabilities()
	if err != nil {
		jptm.LogError(hd.sourceURL, "HEAD request", err)
		jptm.SetStatus(common.ETransferStatus.Failed())
		jptm.ReportTransferDone()
		return
	}

	// Validate content length matches what was enumerated
	info := jptm.Info()
	if hd.contentLength > 0 && info.SourceSize != hd.contentLength {
		err := fmt.Errorf("content length mismatch: enumerated=%d, actual=%d", info.SourceSize, hd.contentLength)
		jptm.LogError(hd.sourceURL, "Content length validation", err)
		jptm.SetStatus(common.ETransferStatus.Failed())
		jptm.ReportTransferDone()
	}
}

func (hd *httpDownloader) Epilogue() {
	// Cleanup - HTTP client doesn't need explicit cleanup
}

// GenerateDownloadFunc returns a chunk function for HTTP downloads
func (hd *httpDownloader) GenerateDownloadFunc(jptm IJobPartTransferMgr, destWriter common.ChunkedFileWriter, id common.ChunkID, length int64, pacer pacer) chunkFunc {
	return createDownloadChunkFunc(jptm, id, func() {
		// Download chunk from HTTP server
		jptm.LogChunkStatus(id, common.EWaitReason.HeaderResponse())

		// Create range request
		req, err := http.NewRequestWithContext(jptm.Context(), "GET", hd.sourceURL, nil)
		if err != nil {
			jptm.FailActiveDownload("Creating HTTP request", err)
			return
		}

		// Add range header if server supports it
		if hd.supportsRange {
			rangeHeader := fmt.Sprintf("bytes=%d-%d", id.OffsetInFile(), id.OffsetInFile()+length-1)
			req.Header.Set("Range", rangeHeader)
		} else if id.OffsetInFile() > 0 {
			// Server doesn't support range requests but we're trying to download a chunk
			// This should only happen for the first chunk in single-shot downloads
			err := fmt.Errorf("server does not support range requests, cannot download chunk at offset %d", id.OffsetInFile())
			jptm.FailActiveDownload("Range request validation", err)
			return
		}

		// Add authentication if available
		if hd.bearerToken != "" {
			req.Header.Set("Authorization", "Bearer "+hd.bearerToken)
		}

		// Add If-Match for consistency (use ETag if available)
		if hd.etag != "" {
			req.Header.Set("If-Match", hd.etag)
		}

		// Execute request with retries
		var resp *http.Response
		retries := 0
		maxRetries := destWriter.MaxRetryPerDownloadBody()

		for retries <= maxRetries {
			resp, err = hd.httpClient.Do(req)
			if err == nil && (resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusPartialContent) {
				break
			}

			// Log retry
			if err != nil {
				jptm.Log(common.LogWarning, fmt.Sprintf("HTTP request failed (attempt %d/%d): %v", retries+1, maxRetries+1, err))
			} else {
				jptm.Log(common.LogWarning, fmt.Sprintf("HTTP request failed with status %d (attempt %d/%d)", resp.StatusCode, retries+1, maxRetries+1))
				if resp.Body != nil {
					resp.Body.Close()
				}
			}

			retries++
			if retries <= maxRetries {
				// Exponential backoff
				backoff := time.Duration(retries) * time.Second
				if backoff > 30*time.Second {
					backoff = 30 * time.Second
				}
				time.Sleep(backoff)

				// Recreate request for retry
				req, err = http.NewRequestWithContext(jptm.Context(), "GET", hd.sourceURL, nil)
				if err != nil {
					jptm.FailActiveDownload("Creating HTTP request for retry", err)
					return
				}
				if hd.supportsRange {
					rangeHeader := fmt.Sprintf("bytes=%d-%d", id.OffsetInFile(), id.OffsetInFile()+length-1)
					req.Header.Set("Range", rangeHeader)
				}
				if hd.bearerToken != "" {
					req.Header.Set("Authorization", "Bearer "+hd.bearerToken)
				}
				if hd.etag != "" {
					req.Header.Set("If-Match", hd.etag)
				}
			}
		}

		if err != nil {
			jptm.FailActiveDownload("Downloading response body", err)
			return
		}

		if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusPartialContent {
			resp.Body.Close()
			err := fmt.Errorf("unexpected status code: %d", resp.StatusCode)
			jptm.FailActiveDownload("Downloading response body", err)
			return
		}

		defer resp.Body.Close()

		// Enqueue the response body to be written to disk
		jptm.LogChunkStatus(id, common.EWaitReason.Body())

		// Wrap the response body with pacing
		pacedBody := newPacedResponseBody(jptm.Context(), resp.Body, pacer)

		err = destWriter.EnqueueChunk(jptm.Context(), id, length, pacedBody, true)
		if err != nil {
			jptm.FailActiveDownload("Enqueuing chunk", err)
			return
		}
	})
}

// detectCapabilities performs HEAD request to detect server capabilities
func (hd *httpDownloader) detectCapabilities() error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "HEAD", hd.sourceURL, nil)
	if err != nil {
		return fmt.Errorf("failed to create HEAD request: %w", err)
	}

	// Add authentication
	if hd.bearerToken != "" {
		req.Header.Set("Authorization", "Bearer "+hd.bearerToken)
	}

	resp, err := hd.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("HEAD request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HEAD request returned status %d: %s", resp.StatusCode, resp.Status)
	}

	// Detect range support
	acceptRanges := resp.Header.Get("Accept-Ranges")
	hd.supportsRange = (acceptRanges == "bytes")

	// Get content length
	if resp.ContentLength > 0 {
		hd.contentLength = resp.ContentLength
	}

	// Parse Content-MD5 if available
	contentMD5 := resp.Header.Get("Content-MD5")
	if contentMD5 != "" {
		decoded, err := base64.StdEncoding.DecodeString(contentMD5)
		if err == nil {
			hd.expectedMD5 = decoded
		}
	}

	// Get ETag for consistency checks
	hd.etag = resp.Header.Get("ETag")

	return nil
}

// GetExpectedMD5 returns the expected MD5 hash from the server
func (hd *httpDownloader) GetExpectedMD5() []byte {
	return hd.expectedMD5
}

// GetSupportsRange returns whether the server supports range requests
func (hd *httpDownloader) GetSupportsRange() bool {
	return hd.supportsRange
}

func bytesEqual(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}