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
	"strconv"
	"strings"
	"time"

	"github.com/Azure/azure-storage-azcopy/v10/common"
)

// httpTraverser enumerates a single HTTP file
type httpTraverser struct {
	rawURL              string
	ctx                 context.Context
	httpClient          *http.Client
	bearerToken         string
	customHeaders       map[string]string

	// Capabilities detected via HEAD request
	supportsRange       bool
	contentLength       int64
	contentMD5          []byte
	etag                string
	lastModified        time.Time
	contentType         string

	incrementEnumerationCounter enumerationCounterFunc
}

func newHTTPTraverser(rawURL string, ctx context.Context, opts *InitResourceTraverserOptions) (*httpTraverser, error) {
	// Parse URL
	_, err := common.NewHTTPURLParts(rawURL)
	if err != nil {
		return nil, fmt.Errorf("invalid HTTP URL: %w", err)
	}

	// Create HTTP client with timeout
	client := &http.Client{
		Timeout: 30 * time.Second,
		Transport: &http.Transport{
			MaxIdleConns:        100,
			MaxIdleConnsPerHost: 100,
			IdleConnTimeout:     90 * time.Second,
		},
	}

	// Extract credentials
	bearerToken := ""
	if opts.Credential != nil && opts.Credential.OAuthTokenInfo.AccessToken != "" {
		bearerToken = opts.Credential.OAuthTokenInfo.AccessToken
	}

	// Get custom headers if provided
	customHeaders := make(map[string]string)
	// Note: HTTPHeaders will be added in Phase 4 CLI integration

	incrementFunc := opts.IncrementEnumeration
	if incrementFunc == nil {
		incrementFunc = enumerationCounterFuncNoop
	}

	t := &httpTraverser{
		rawURL:                      rawURL,
		ctx:                         ctx,
		httpClient:                  client,
		bearerToken:                 bearerToken,
		customHeaders:               customHeaders,
		incrementEnumerationCounter: incrementFunc,
	}

	// Perform HEAD request to detect capabilities
	if err := t.detectCapabilities(); err != nil {
		return nil, fmt.Errorf("failed to detect HTTP capabilities: %w", err)
	}

	return t, nil
}

// IsDirectory always returns false (HTTP endpoints are files)
func (t *httpTraverser) IsDirectory(isSource bool) (bool, error) {
	return false, nil
}

// Traverse enumerates the single HTTP file
func (t *httpTraverser) Traverse(
	preprocessor objectMorpher,
	processor objectProcessor,
	filters []ObjectFilter,
) error {
	// Create StoredObject representing this HTTP file
	object := &StoredObject{
		name:             t.getFileName(),
		entityType:       common.EEntityType.File(),
		lastModifiedTime: t.lastModified,
		size:             t.contentLength,
		md5:              t.contentMD5,
		contentType:      t.contentType,
		Metadata:         common.Metadata{},
		relativePath:     "",
	}

	// Apply preprocessor
	if preprocessor != nil {
		preprocessor(object)
	}

	// Apply filters
	for _, filter := range filters {
		_, supported := filter.DoesSupportThisOS()
		if !supported {
			continue
		}
		if !filter.DoesPass(*object) {
			glcm.Info(fmt.Sprintf("Skipping %s due to filter", object.name))
			return nil
		}
	}

	// Process the object
	err := processor(*object)
	if err != nil {
		return fmt.Errorf("failed to process HTTP file: %w", err)
	}

	t.incrementEnumerationCounter(common.EEntityType.File())

	return nil
}

// detectCapabilities performs HEAD request to detect server capabilities
func (t *httpTraverser) detectCapabilities() error {
	req, err := http.NewRequestWithContext(t.ctx, "HEAD", t.rawURL, nil)
	if err != nil {
		return fmt.Errorf("failed to create HEAD request: %w", err)
	}

	// Add authentication
	if t.bearerToken != "" {
		req.Header.Set("Authorization", "Bearer "+t.bearerToken)
	}

	// Add custom headers
	for k, v := range t.customHeaders {
		req.Header.Set(k, v)
	}

	resp, err := t.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("HEAD request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HEAD request returned status %d: %s", resp.StatusCode, resp.Status)
	}

	// Detect range support
	acceptRanges := resp.Header.Get("Accept-Ranges")
	t.supportsRange = (acceptRanges == "bytes")

	// Parse Content-Length
	contentLength := resp.Header.Get("Content-Length")
	if contentLength != "" {
		length, err := strconv.ParseInt(contentLength, 10, 64)
		if err == nil {
			t.contentLength = length
		}
	}

	// Parse Content-MD5 (optional)
	contentMD5 := resp.Header.Get("Content-MD5")
	if contentMD5 != "" {
		// Base64 decode
		decoded, err := base64.StdEncoding.DecodeString(contentMD5)
		if err == nil {
			t.contentMD5 = decoded
		}
	}

	// Parse ETag
	t.etag = resp.Header.Get("ETag")

	// Parse Last-Modified
	lastMod := resp.Header.Get("Last-Modified")
	if lastMod != "" {
		parsedTime, err := http.ParseTime(lastMod)
		if err == nil {
			t.lastModified = parsedTime
		}
	}

	// Parse Content-Type
	t.contentType = resp.Header.Get("Content-Type")

	return nil
}

// getFileName extracts filename from URL
func (t *httpTraverser) getFileName() string {
	urlParts, err := common.NewHTTPURLParts(t.rawURL)
	if err != nil {
		return "downloaded_file"
	}

	// Extract last segment of path
	path := strings.TrimSuffix(urlParts.Path, "/")
	parts := strings.Split(path, "/")

	// Find the last non-empty segment
	for i := len(parts) - 1; i >= 0; i-- {
		if parts[i] != "" {
			return parts[i]
		}
	}

	return "downloaded_file"
}

// GetSupportsRange returns whether the server supports range requests
func (t *httpTraverser) GetSupportsRange() bool {
	return t.supportsRange
}

// GetContentLength returns the content length from HEAD response
func (t *httpTraverser) GetContentLength() int64 {
	return t.contentLength
}

// GetETag returns the ETag from HEAD response
func (t *httpTraverser) GetETag() string {
	return t.etag
}