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
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestHTTPDownloader_DetectCapabilities(t *testing.T) {
	t.Run("SupportsRange", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, "HEAD", r.Method)
			w.Header().Set("Accept-Ranges", "bytes")
			w.Header().Set("Content-Length", "1000")
			w.Header().Set("ETag", "\"abc123\"")
			w.Header().Set("Content-MD5", base64.StdEncoding.EncodeToString([]byte("test-md5")))
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		httpDl := &httpDownloader{
			sourceURL:  server.URL,
			httpClient: server.Client(),
		}

		err := httpDl.detectCapabilities()
		assert.NoError(t, err)
		assert.True(t, httpDl.supportsRange)
		assert.Equal(t, int64(1000), httpDl.contentLength)
		assert.Equal(t, "\"abc123\"", httpDl.etag)
		assert.NotEmpty(t, httpDl.expectedMD5)
	})

	t.Run("NoRangeSupport", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Accept-Ranges", "none")
			w.Header().Set("Content-Length", "500")
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		httpDl := &httpDownloader{
			sourceURL:  server.URL,
			httpClient: server.Client(),
		}

		err := httpDl.detectCapabilities()
		assert.NoError(t, err)
		assert.False(t, httpDl.supportsRange)
		assert.Equal(t, int64(500), httpDl.contentLength)
	})

	t.Run("NoAcceptRangesHeader", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// No Accept-Ranges header
			w.Header().Set("Content-Length", "1000")
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		httpDl := &httpDownloader{
			sourceURL:  server.URL,
			httpClient: server.Client(),
		}

		err := httpDl.detectCapabilities()
		assert.NoError(t, err)
		assert.False(t, httpDl.supportsRange)
	})

	t.Run("ServerError", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		}))
		defer server.Close()

		httpDl := &httpDownloader{
			sourceURL:  server.URL,
			httpClient: server.Client(),
		}

		err := httpDl.detectCapabilities()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "500")
	})

	t.Run("NotFound", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
		}))
		defer server.Close()

		httpDl := &httpDownloader{
			sourceURL:  server.URL,
			httpClient: server.Client(),
		}

		err := httpDl.detectCapabilities()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "404")
	})

	t.Run("WithBearerToken", func(t *testing.T) {
		tokenReceived := false
		expectedToken := "test-bearer-token"

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			auth := r.Header.Get("Authorization")
			if auth == "Bearer "+expectedToken {
				tokenReceived = true
			}
			w.Header().Set("Content-Length", "1000")
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		httpDl := &httpDownloader{
			sourceURL:   server.URL,
			httpClient:  server.Client(),
			bearerToken: expectedToken,
		}

		err := httpDl.detectCapabilities()
		assert.NoError(t, err)
		assert.True(t, tokenReceived)
	})

	t.Run("InvalidMD5", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-MD5", "not-valid-base64!!!")
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		httpDl := &httpDownloader{
			sourceURL:  server.URL,
			httpClient: server.Client(),
		}

		err := httpDl.detectCapabilities()
		assert.NoError(t, err)
		assert.Empty(t, httpDl.expectedMD5) // Invalid MD5 should be ignored
	})

	t.Run("AllMetadata", func(t *testing.T) {
		expectedMD5 := []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Accept-Ranges", "bytes")
			w.Header().Set("Content-Length", "2048")
			w.Header().Set("ETag", "\"etag-value\"")
			w.Header().Set("Content-MD5", base64.StdEncoding.EncodeToString(expectedMD5))
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		httpDl := &httpDownloader{
			sourceURL:  server.URL,
			httpClient: server.Client(),
		}

		err := httpDl.detectCapabilities()
		assert.NoError(t, err)
		assert.True(t, httpDl.supportsRange)
		assert.Equal(t, int64(2048), httpDl.contentLength)
		assert.Equal(t, "\"etag-value\"", httpDl.etag)
		assert.Equal(t, expectedMD5, httpDl.expectedMD5)
	})

	t.Run("Timeout", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Simulate slow server
			time.Sleep(5 * time.Second)
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		httpDl := &httpDownloader{
			sourceURL: server.URL,
			httpClient: &http.Client{
				Timeout: 100 * time.Millisecond,
			},
		}

		err := httpDl.detectCapabilities()
		assert.Error(t, err)
	})

	t.Run("NetworkFailure", func(t *testing.T) {
		httpDl := &httpDownloader{
			sourceURL: "http://localhost:1/nonexistent",
			httpClient: &http.Client{
				Timeout: 1 * time.Second,
			},
		}

		err := httpDl.detectCapabilities()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "HEAD request failed")
	})
}

func TestHTTPDownloader_GetMethods(t *testing.T) {
	t.Run("GetExpectedMD5", func(t *testing.T) {
		expectedMD5 := []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}
		httpDl := &httpDownloader{
			expectedMD5: expectedMD5,
		}

		md5 := httpDl.GetExpectedMD5()
		assert.Equal(t, expectedMD5, md5)
	})

	t.Run("GetExpectedMD5Empty", func(t *testing.T) {
		httpDl := &httpDownloader{}

		md5 := httpDl.GetExpectedMD5()
		assert.Empty(t, md5)
	})

	t.Run("GetSupportsRange", func(t *testing.T) {
		httpDl := &httpDownloader{supportsRange: true}
		assert.True(t, httpDl.GetSupportsRange())

		httpDl.supportsRange = false
		assert.False(t, httpDl.GetSupportsRange())
	})
}

func TestHTTPDownloader_BytesEqual(t *testing.T) {
	t.Run("EqualBytes", func(t *testing.T) {
		a := []byte{1, 2, 3, 4, 5}
		b := []byte{1, 2, 3, 4, 5}
		assert.True(t, bytesEqual(a, b))
	})

	t.Run("DifferentBytes", func(t *testing.T) {
		a := []byte{1, 2, 3, 4, 5}
		b := []byte{1, 2, 3, 4, 6}
		assert.False(t, bytesEqual(a, b))
	})

	t.Run("DifferentLengths", func(t *testing.T) {
		a := []byte{1, 2, 3}
		b := []byte{1, 2, 3, 4, 5}
		assert.False(t, bytesEqual(a, b))
	})

	t.Run("EmptyArrays", func(t *testing.T) {
		a := []byte{}
		b := []byte{}
		assert.True(t, bytesEqual(a, b))
	})

	t.Run("OneEmpty", func(t *testing.T) {
		a := []byte{1, 2, 3}
		b := []byte{}
		assert.False(t, bytesEqual(a, b))
	})
}

func TestHTTPDownloader_HTTPClient(t *testing.T) {
	t.Run("ClientConfiguration", func(t *testing.T) {
		client := &http.Client{
			Timeout: 30 * time.Minute,
			Transport: &http.Transport{
				MaxIdleConns:        100,
				MaxIdleConnsPerHost: 100,
				IdleConnTimeout:     90 * time.Second,
				DisableCompression:  true,
			},
		}

		httpDl := &httpDownloader{
			httpClient: client,
		}

		assert.Equal(t, 30*time.Minute, httpDl.httpClient.Timeout)
		transport := httpDl.httpClient.Transport.(*http.Transport)
		assert.Equal(t, 100, transport.MaxIdleConns)
		assert.Equal(t, 100, transport.MaxIdleConnsPerHost)
		assert.Equal(t, 90*time.Second, transport.IdleConnTimeout)
		assert.True(t, transport.DisableCompression)
	})
}

func TestHTTPDownloader_Various(t *testing.T) {
	t.Run("MultipleCapabilityChecks", func(t *testing.T) {
		callCount := 0
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			callCount++
			w.Header().Set("Accept-Ranges", "bytes")
			w.Header().Set("Content-Length", "1000")
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		httpDl := &httpDownloader{
			sourceURL:  server.URL,
			httpClient: server.Client(),
		}

		// First check
		err := httpDl.detectCapabilities()
		assert.NoError(t, err)
		firstCallCount := callCount

		// Second check should update state
		err = httpDl.detectCapabilities()
		assert.NoError(t, err)
		assert.Equal(t, firstCallCount+1, callCount)
	})

	t.Run("ContentLengthZero", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Length", "0")
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		httpDl := &httpDownloader{
			sourceURL:  server.URL,
			httpClient: server.Client(),
		}

		err := httpDl.detectCapabilities()
		assert.NoError(t, err)
		assert.Equal(t, int64(0), httpDl.contentLength)
	})

	t.Run("NoContentLength", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// No Content-Length header
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		httpDl := &httpDownloader{
			sourceURL:  server.URL,
			httpClient: server.Client(),
		}

		err := httpDl.detectCapabilities()
		assert.NoError(t, err)
		assert.Equal(t, int64(0), httpDl.contentLength)
	})
}