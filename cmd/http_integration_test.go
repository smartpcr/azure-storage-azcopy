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

package cmd

import (
	"crypto/md5"
	"encoding/base64"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/Azure/azure-storage-azcopy/v10/common"
	"github.com/stretchr/testify/assert"
)

// Test data for HTTP tests
var httpTestData = []byte("AzCopy HTTP test data for range request validation")

// setupTestHTTPServer creates a test HTTP server with configurable behavior
func setupTestHTTPServer(t *testing.T, data []byte, requireAuth bool, authToken string, supportRanges bool) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Check authentication if required
		if requireAuth {
			auth := r.Header.Get("Authorization")
			expectedAuth := "Bearer " + authToken
			if auth != expectedAuth {
				t.Logf("Auth mismatch: got %q, expected %q", auth, expectedAuth)
				w.WriteHeader(http.StatusUnauthorized)
				w.Write([]byte("Unauthorized"))
				return
			}
		}

		// Handle HEAD requests
		if r.Method == "HEAD" {
			if supportRanges {
				w.Header().Set("Accept-Ranges", "bytes")
			}
			w.Header().Set("Content-Length", strconv.Itoa(len(data)))

			hash := md5.Sum(data)
			encoded := base64.StdEncoding.EncodeToString(hash[:])
			w.Header().Set("Content-MD5", encoded)
			w.Header().Set("ETag", `"test-etag"`)

			w.WriteHeader(http.StatusOK)
			return
		}

		// Handle GET requests with range support
		rangeHeader := r.Header.Get("Range")
		if rangeHeader != "" && supportRanges {
			// Parse range header
			rangeStr := strings.TrimPrefix(rangeHeader, "bytes=")
			parts := strings.Split(rangeStr, "-")

			if len(parts) == 2 {
				start, _ := strconv.ParseInt(parts[0], 10, 64)
				end, _ := strconv.ParseInt(parts[1], 10, 64)

				if start < 0 || start >= int64(len(data)) {
					w.WriteHeader(http.StatusRequestedRangeNotSatisfiable)
					return
				}

				if end >= int64(len(data)) {
					end = int64(len(data)) - 1
				}

				w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, end, len(data)))
				w.Header().Set("Content-Length", strconv.FormatInt(end-start+1, 10))
				w.Header().Set("Accept-Ranges", "bytes")
				w.WriteHeader(http.StatusPartialContent)
				w.Write(data[start : end+1])
				return
			}
		}

		// Full content response
		w.Header().Set("Content-Length", strconv.Itoa(len(data)))
		if supportRanges {
			w.Header().Set("Accept-Ranges", "bytes")
		}
		w.WriteHeader(http.StatusOK)
		w.Write(data)
	}))
}

// TestHTTPLocationDetection_GenericURLs verifies generic HTTP URLs are detected
func TestHTTPLocationDetection_GenericURLs(t *testing.T) {
	testCases := []struct {
		url      string
		expected common.Location
	}{
		{"https://api.example.com/files/data.bin", common.ELocation.Http()},
		{"http://download.example.com/file.tar.gz", common.ELocation.Http()},
		{"https://cdn.example.com/assets/video.mp4", common.ELocation.Http()},
		{"http://localhost:8080/test.txt", common.ELocation.Http()},
		{"http://192.168.1.100:9000/file.bin", common.ELocation.Http()},
		{"http://127.0.0.1:10000/data", common.ELocation.Http()},

		// Azure URLs should still be detected correctly
		{"https://account.blob.core.windows.net/container", common.ELocation.Blob()},
		{"https://account.file.core.windows.net/share", common.ELocation.File()},
		{"https://account.dfs.core.windows.net/filesystem", common.ELocation.BlobFS()},
	}

	for _, tc := range testCases {
		t.Run(tc.url, func(t *testing.T) {
			location := InferArgumentLocation(tc.url)
			assert.Equal(t, tc.expected, location, "URL should be detected as %s", tc.expected.String())
		})
	}
}

// TestHTTPServer_AnonymousAccess verifies server accepts requests without auth
func TestHTTPServer_AnonymousAccess(t *testing.T) {
	server := setupTestHTTPServer(t, httpTestData, false, "", true)
	defer server.Close()

	// Test HEAD request
	resp, err := http.Head(server.URL)
	assert.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "bytes", resp.Header.Get("Accept-Ranges"))

	// Test GET request
	resp, err = http.Get(server.URL)
	assert.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	defer resp.Body.Close()
}

// TestHTTPServer_Authentication verifies bearer token auth works
func TestHTTPServer_Authentication(t *testing.T) {
	validToken := "test-token-12345"
	server := setupTestHTTPServer(t, httpTestData, true, validToken, true)
	defer server.Close()

	// Test without token - should fail
	resp, err := http.Head(server.URL)
	assert.NoError(t, err)
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)

	// Test with correct token - should succeed
	req, _ := http.NewRequest("HEAD", server.URL, nil)
	req.Header.Set("Authorization", "Bearer "+validToken)
	resp, err = http.DefaultClient.Do(req)
	assert.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	// Test with wrong token - should fail
	req, _ = http.NewRequest("HEAD", server.URL, nil)
	req.Header.Set("Authorization", "Bearer wrong-token")
	resp, err = http.DefaultClient.Do(req)
	assert.NoError(t, err)
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
}

// TestHTTPServer_RangeRequests verifies range request handling
func TestHTTPServer_RangeRequests(t *testing.T) {
	server := setupTestHTTPServer(t, httpTestData, false, "", true)
	defer server.Close()

	dataLen := len(httpTestData)
	testCases := []struct {
		name          string
		rangeHeader   string
		expectedCode  int
		expectedStart int
		expectedEnd   int
	}{
		{"First10Bytes", "bytes=0-9", http.StatusPartialContent, 0, 9},
		{"Middle10Bytes", "bytes=10-19", http.StatusPartialContent, 10, 19},
		{"Last8Bytes", fmt.Sprintf("bytes=%d-%d", dataLen-8, dataLen-1), http.StatusPartialContent, dataLen - 8, dataLen - 1},
		{"FromMiddleToEnd", fmt.Sprintf("bytes=25-%d", dataLen-1), http.StatusPartialContent, 25, dataLen - 1},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			req, _ := http.NewRequest("GET", server.URL, nil)
			req.Header.Set("Range", tc.rangeHeader)

			resp, err := http.DefaultClient.Do(req)
			assert.NoError(t, err)
			defer resp.Body.Close()

			assert.Equal(t, tc.expectedCode, resp.StatusCode)

			if resp.StatusCode == http.StatusPartialContent {
				expectedLen := tc.expectedEnd - tc.expectedStart + 1
				assert.Equal(t, strconv.Itoa(expectedLen), resp.Header.Get("Content-Length"))
				assert.Contains(t, resp.Header.Get("Content-Range"), fmt.Sprintf("bytes %d-%d", tc.expectedStart, tc.expectedEnd))
			}
		})
	}
}

// TestHTTPServer_NoRangeSupport verifies fallback when ranges not supported
func TestHTTPServer_NoRangeSupport(t *testing.T) {
	server := setupTestHTTPServer(t, httpTestData, false, "", false)
	defer server.Close()

	// HEAD request should not indicate range support
	resp, err := http.Head(server.URL)
	assert.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.NotEqual(t, "bytes", resp.Header.Get("Accept-Ranges"))

	// GET request with Range header should still return full content
	req, _ := http.NewRequest("GET", server.URL, nil)
	req.Header.Set("Range", "bytes=0-9")
	resp, err = http.DefaultClient.Do(req)
	assert.NoError(t, err)
	defer resp.Body.Close()

	// Should return 200 OK (full content), not 206 Partial Content
	assert.Equal(t, http.StatusOK, resp.StatusCode)
}

// TestHTTPFromTo_Validation verifies HTTP to Local is valid
func TestHTTPFromTo_Validation(t *testing.T) {
	testCases := []struct {
		name     string
		src      string
		dst      string
		expected common.FromTo
	}{
		{
			name:     "HTTPToLocal",
			src:      "https://api.example.com/file.bin",
			dst:      "/local/path",
			expected: common.EFromTo.HttpLocal(),
		},
		{
			name:     "HTTPSToLocal",
			src:      "http://localhost:8080/data.txt",
			dst:      "./downloads/",
			expected: common.EFromTo.HttpLocal(),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			fromTo := inferFromTo(tc.src, tc.dst)
			assert.Equal(t, tc.expected, fromTo)
		})
	}
}

// TestHTTPAuthentication_BearerTokenFormat verifies proper Authorization header format
func TestHTTPAuthentication_BearerTokenFormat(t *testing.T) {
	token := "eyJhbGciOiJSUzI1NiIsInR5cCI6IkpXVCJ9..."
	expectedHeader := "Bearer " + token

	server := setupTestHTTPServer(t, httpTestData, true, token, true)
	defer server.Close()

	req, _ := http.NewRequest("HEAD", server.URL, nil)
	req.Header.Set("Authorization", expectedHeader)

	resp, err := http.DefaultClient.Do(req)
	assert.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode, "Should accept Bearer token")
}