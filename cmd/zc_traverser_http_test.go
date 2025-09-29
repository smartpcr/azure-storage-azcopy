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
	"context"
	"encoding/base64"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Azure/azure-storage-azcopy/v10/common"
	"github.com/stretchr/testify/assert"
)

func TestHTTPTraverser_Creation(t *testing.T) {
	t.Run("ValidURL", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Accept-Ranges", "bytes")
			w.Header().Set("Content-Length", "1000")
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		ctx := context.Background()
		opts := &InitResourceTraverserOptions{
			Credential: &common.CredentialInfo{
				CredentialType: common.ECredentialType.OAuthToken(),
				OAuthTokenInfo: common.OAuthTokenInfo{
					Token: common.Token{
						AccessToken: "test-token",
					},
				},
			},
		}

		traverser, err := newHTTPTraverser(server.URL, ctx, opts)
		assert.NoError(t, err)
		assert.NotNil(t, traverser)
		assert.Equal(t, "test-token", traverser.bearerToken)
	})

	t.Run("InvalidURL", func(t *testing.T) {
		ctx := context.Background()
		opts := &InitResourceTraverserOptions{}

		_, err := newHTTPTraverser("not a url", ctx, opts)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "invalid HTTP URL")
	})

	t.Run("ServerNotResponding", func(t *testing.T) {
		ctx := context.Background()
		opts := &InitResourceTraverserOptions{}

		// Use invalid port
		_, err := newHTTPTraverser("http://localhost:99999/file", ctx, opts)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to detect HTTP capabilities")
	})

	t.Run("NilIncrementFunc", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		ctx := context.Background()
		opts := &InitResourceTraverserOptions{
			IncrementEnumeration: nil,
		}

		traverser, err := newHTTPTraverser(server.URL, ctx, opts)
		assert.NoError(t, err)
		assert.NotNil(t, traverser.incrementEnumerationCounter)
	})
}

func TestHTTPTraverser_RangeDetection(t *testing.T) {
	t.Run("ServerSupportsRanges", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, "HEAD", r.Method)
			w.Header().Set("Accept-Ranges", "bytes")
			w.Header().Set("Content-Length", "5000")
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		ctx := context.Background()
		opts := &InitResourceTraverserOptions{}

		traverser, err := newHTTPTraverser(server.URL, ctx, opts)
		assert.NoError(t, err)
		assert.True(t, traverser.supportsRange)
		assert.Equal(t, int64(5000), traverser.contentLength)
	})

	t.Run("ServerDoesNotSupportRanges", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Accept-Ranges", "none")
			w.Header().Set("Content-Length", "1000")
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		ctx := context.Background()
		opts := &InitResourceTraverserOptions{}

		traverser, err := newHTTPTraverser(server.URL, ctx, opts)
		assert.NoError(t, err)
		assert.False(t, traverser.supportsRange)
	})

	t.Run("ServerNoAcceptRangesHeader", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Don't set Accept-Ranges header
			w.Header().Set("Content-Length", "1000")
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		ctx := context.Background()
		opts := &InitResourceTraverserOptions{}

		traverser, err := newHTTPTraverser(server.URL, ctx, opts)
		assert.NoError(t, err)
		assert.False(t, traverser.supportsRange)
	})
}

func TestHTTPTraverser_MetadataExtraction(t *testing.T) {
	t.Run("AllMetadataPresent", func(t *testing.T) {
		expectedMD5 := base64.StdEncoding.EncodeToString([]byte("test-md5-hash"))
		expectedETag := `"abc123"`
		expectedLastMod := time.Now().UTC().Format(http.TimeFormat)

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Accept-Ranges", "bytes")
			w.Header().Set("Content-Length", "12345")
			w.Header().Set("Content-MD5", expectedMD5)
			w.Header().Set("ETag", expectedETag)
			w.Header().Set("Last-Modified", expectedLastMod)
			w.Header().Set("Content-Type", "application/octet-stream")
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		ctx := context.Background()
		opts := &InitResourceTraverserOptions{}

		traverser, err := newHTTPTraverser(server.URL, ctx, opts)
		assert.NoError(t, err)
		assert.Equal(t, int64(12345), traverser.contentLength)
		assert.NotNil(t, traverser.contentMD5)
		assert.Equal(t, expectedETag, traverser.etag)
		assert.False(t, traverser.lastModified.IsZero())
		assert.Equal(t, "application/octet-stream", traverser.contentType)
	})

	t.Run("PartialMetadata", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Length", "100")
			// Only some headers
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		ctx := context.Background()
		opts := &InitResourceTraverserOptions{}

		traverser, err := newHTTPTraverser(server.URL, ctx, opts)
		assert.NoError(t, err)
		assert.Equal(t, int64(100), traverser.contentLength)
		assert.Nil(t, traverser.contentMD5)
		assert.Empty(t, traverser.etag)
		assert.True(t, traverser.lastModified.IsZero())
	})

	t.Run("InvalidContentLength", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Invalid Content-Length causes HTTP client to fail
			// This is correct behavior - server is sending malformed HTTP
			w.Header().Set("Content-Length", "not-a-number")
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		ctx := context.Background()
		opts := &InitResourceTraverserOptions{}

		_, err := newHTTPTraverser(server.URL, ctx, opts)
		assert.Error(t, err, "Should fail with invalid Content-Length")
	})

	t.Run("InvalidMD5", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-MD5", "not-base64!!!!")
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		ctx := context.Background()
		opts := &InitResourceTraverserOptions{}

		traverser, err := newHTTPTraverser(server.URL, ctx, opts)
		assert.NoError(t, err)
		assert.Nil(t, traverser.contentMD5)
	})

	t.Run("InvalidLastModified", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Last-Modified", "not-a-date")
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		ctx := context.Background()
		opts := &InitResourceTraverserOptions{}

		traverser, err := newHTTPTraverser(server.URL, ctx, opts)
		assert.NoError(t, err)
		assert.True(t, traverser.lastModified.IsZero())
	})
}

func TestHTTPTraverser_Authentication(t *testing.T) {
	t.Run("BearerTokenSent", func(t *testing.T) {
		expectedToken := "test-bearer-token"
		tokenReceived := false

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			auth := r.Header.Get("Authorization")
			if auth == "Bearer "+expectedToken {
				tokenReceived = true
			}
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		ctx := context.Background()
		opts := &InitResourceTraverserOptions{
			Credential: &common.CredentialInfo{
				CredentialType: common.ECredentialType.OAuthToken(),
				OAuthTokenInfo: common.OAuthTokenInfo{
					Token: common.Token{
						AccessToken: expectedToken,
					},
				},
			},
		}

		_, err := newHTTPTraverser(server.URL, ctx, opts)
		assert.NoError(t, err)
		assert.True(t, tokenReceived, "Bearer token should be sent")
	})

	t.Run("UnauthorizedResponse", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusUnauthorized)
		}))
		defer server.Close()

		ctx := context.Background()
		opts := &InitResourceTraverserOptions{}

		_, err := newHTTPTraverser(server.URL, ctx, opts)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "401")
	})

	t.Run("NoCredentials", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			auth := r.Header.Get("Authorization")
			assert.Empty(t, auth, "No auth header should be sent")
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		ctx := context.Background()
		opts := &InitResourceTraverserOptions{}

		traverser, err := newHTTPTraverser(server.URL, ctx, opts)
		assert.NoError(t, err)
		assert.Empty(t, traverser.bearerToken)
	})
}

func TestHTTPTraverser_IsDirectory(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	ctx := context.Background()
	opts := &InitResourceTraverserOptions{}

	traverser, err := newHTTPTraverser(server.URL, ctx, opts)
	assert.NoError(t, err)

	isDir, err := traverser.IsDirectory(true)
	assert.NoError(t, err)
	assert.False(t, isDir, "HTTP endpoints should always be files")

	isDir, err = traverser.IsDirectory(false)
	assert.NoError(t, err)
	assert.False(t, isDir, "HTTP endpoints should always be files")
}

func TestHTTPTraverser_Traverse(t *testing.T) {
	t.Run("SuccessfulEnumeration", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Length", "5000")
			w.Header().Set("Accept-Ranges", "bytes")
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		ctx := context.Background()
		enumCounterCalled := false
		opts := &InitResourceTraverserOptions{
			IncrementEnumeration: func(entityType common.EntityType) {
				enumCounterCalled = true
				assert.Equal(t, common.EEntityType.File(), entityType)
			},
		}

		traverser, err := newHTTPTraverser(server.URL, ctx, opts)
		assert.NoError(t, err)

		processed := false
		processor := func(obj StoredObject) error {
			processed = true
			assert.Equal(t, common.EEntityType.File(), obj.entityType)
			assert.Equal(t, int64(5000), obj.size)
			return nil
		}

		err = traverser.Traverse(nil, processor, nil)
		assert.NoError(t, err)
		assert.True(t, processed, "Processor should be called")
		assert.True(t, enumCounterCalled, "Enum counter should be called")
	})

	t.Run("WithPreprocessor", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Length", "100")
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		ctx := context.Background()
		opts := &InitResourceTraverserOptions{}

		traverser, err := newHTTPTraverser(server.URL, ctx, opts)
		assert.NoError(t, err)

		preprocessorCalled := false
		preprocessor := func(obj *StoredObject) {
			preprocessorCalled = true
			obj.ContainerName = "test-container"
		}

		processed := false
		processor := func(obj StoredObject) error {
			processed = true
			assert.Equal(t, "test-container", obj.ContainerName)
			return nil
		}

		err = traverser.Traverse(preprocessor, processor, nil)
		assert.NoError(t, err)
		assert.True(t, preprocessorCalled)
		assert.True(t, processed)
	})

	t.Run("WithFilters", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Length", "100")
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		ctx := context.Background()
		opts := &InitResourceTraverserOptions{}

		traverser, err := newHTTPTraverser(server.URL, ctx, opts)
		assert.NoError(t, err)

		processed := false
		processor := func(obj StoredObject) error {
			processed = true
			return nil
		}

		// Filter that blocks everything
		blockFilter := &mockFilter{
			doesPass:   false,
			supportsOS: true,
		}

		err = traverser.Traverse(nil, processor, []ObjectFilter{blockFilter})
		assert.NoError(t, err)
		assert.False(t, processed, "Processor should not be called when filtered")
	})

	t.Run("ProcessorError", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Length", "100")
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		ctx := context.Background()
		opts := &InitResourceTraverserOptions{}

		traverser, err := newHTTPTraverser(server.URL, ctx, opts)
		assert.NoError(t, err)

		expectedErr := fmt.Errorf("processor error")
		processor := func(obj StoredObject) error {
			return expectedErr
		}

		err = traverser.Traverse(nil, processor, nil)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "processor error")
	})

	t.Run("UnsupportedOSFilter", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Length", "100")
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		ctx := context.Background()
		opts := &InitResourceTraverserOptions{}

		traverser, err := newHTTPTraverser(server.URL, ctx, opts)
		assert.NoError(t, err)

		processed := false
		processor := func(obj StoredObject) error {
			processed = true
			return nil
		}

		// Filter that doesn't support current OS
		unsupportedFilter := &mockFilter{
			doesPass:   true,
			supportsOS: false,
		}

		err = traverser.Traverse(nil, processor, []ObjectFilter{unsupportedFilter})
		assert.NoError(t, err)
		assert.True(t, processed, "Should skip unsupported filter and still process")
	})
}

func TestHTTPTraverser_GetFileName(t *testing.T) {
	tests := []struct {
		url          string
		expectedName string
	}{
		{
			url:          "https://example.com/files/data.bin",
			expectedName: "data.bin",
		},
		{
			url:          "https://example.com/path/to/archive.tar.gz",
			expectedName: "archive.tar.gz",
		},
		{
			url:          "https://example.com/file",
			expectedName: "file",
		},
		{
			url:          "https://example.com/",
			expectedName: "downloaded_file",
		},
		{
			url:          "https://example.com",
			expectedName: "downloaded_file",
		},
	}

	for _, tt := range tests {
		t.Run(tt.url, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			}))
			defer server.Close()

			// Create traverser with test URL for filename extraction
			ctx := context.Background()
			opts := &InitResourceTraverserOptions{}

			// We need to create a server first to pass capability check
			traverser, err := newHTTPTraverser(server.URL, ctx, opts)
			if err != nil {
				t.Skip("Could not create traverser")
				return
			}

			// Now test getFileName with the actual test URL
			traverser.rawURL = tt.url
			filename := traverser.getFileName()
			assert.Equal(t, tt.expectedName, filename)
		})
	}
}

func TestHTTPTraverser_GetSupportsRange(t *testing.T) {
	t.Run("SupportsRange", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Accept-Ranges", "bytes")
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		ctx := context.Background()
		opts := &InitResourceTraverserOptions{}

		traverser, err := newHTTPTraverser(server.URL, ctx, opts)
		assert.NoError(t, err)
		assert.True(t, traverser.GetSupportsRange())
	})

	t.Run("DoesNotSupportRange", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		ctx := context.Background()
		opts := &InitResourceTraverserOptions{}

		traverser, err := newHTTPTraverser(server.URL, ctx, opts)
		assert.NoError(t, err)
		assert.False(t, traverser.GetSupportsRange())
	})
}

func TestHTTPTraverser_GetContentLength(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "123456")
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	ctx := context.Background()
	opts := &InitResourceTraverserOptions{}

	traverser, err := newHTTPTraverser(server.URL, ctx, opts)
	assert.NoError(t, err)
	assert.Equal(t, int64(123456), traverser.GetContentLength())
}

func TestHTTPTraverser_GetETag(t *testing.T) {
	expectedETag := `"test-etag-value"`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("ETag", expectedETag)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	ctx := context.Background()
	opts := &InitResourceTraverserOptions{}

	traverser, err := newHTTPTraverser(server.URL, ctx, opts)
	assert.NoError(t, err)
	assert.Equal(t, expectedETag, traverser.GetETag())
}

func TestHTTPTraverser_ContextCancellation(t *testing.T) {
	slowServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(5 * time.Second)
		w.WriteHeader(http.StatusOK)
	}))
	defer slowServer.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	opts := &InitResourceTraverserOptions{}

	_, err := newHTTPTraverser(slowServer.URL, ctx, opts)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to detect HTTP capabilities")
}

func TestHTTPTraverser_ServerErrors(t *testing.T) {
	t.Run("404NotFound", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
		}))
		defer server.Close()

		ctx := context.Background()
		opts := &InitResourceTraverserOptions{}

		_, err := newHTTPTraverser(server.URL, ctx, opts)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "404")
	})

	t.Run("500InternalServerError", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		}))
		defer server.Close()

		ctx := context.Background()
		opts := &InitResourceTraverserOptions{}

		_, err := newHTTPTraverser(server.URL, ctx, opts)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "500")
	})
}

// Mock filter for testing
type mockFilter struct {
	doesPass   bool
	supportsOS bool
}

func (m *mockFilter) DoesPass(obj StoredObject) bool {
	return m.doesPass
}

func (m *mockFilter) DoesSupportThisOS() (msg string, supported bool) {
	return "", m.supportsOS
}

func (m *mockFilter) AppliesOnlyToFiles() bool {
	return false
}