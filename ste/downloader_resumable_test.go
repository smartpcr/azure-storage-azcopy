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
	"testing"

	"github.com/Azure/azure-storage-azcopy/v10/common"
)

// TestResumableDownloaderInterface tests that key downloaders implement the resumableDownloader interface
func TestResumableDownloaderInterface(t *testing.T) {
	tests := []struct {
		name           string
		downloader     interface{}
		shouldBeResumable bool
	}{
		{
			name:           "HTTP downloader should be resumable",
			downloader:     &httpDownloader{supportsRange: true},
			shouldBeResumable: true,
		},
		{
			name:           "Blob downloader should be resumable",
			downloader:     &blobDownloader{},
			shouldBeResumable: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, ok := tt.downloader.(resumableDownloader)
			if ok != tt.shouldBeResumable {
				if tt.shouldBeResumable {
					t.Errorf("%s does not implement resumableDownloader interface", tt.name)
				} else {
					t.Errorf("%s should not implement resumableDownloader interface", tt.name)
				}
			}

			if ok {
				resumable := tt.downloader.(resumableDownloader)

				// Test SupportsResume for HTTP
				if hd, isHTTP := tt.downloader.(*httpDownloader); isHTTP {
					if hd.supportsRange != resumable.SupportsResume() {
						t.Errorf("HTTP SupportsResume() should return %v, got %v", hd.supportsRange, resumable.SupportsResume())
					}
				}

				// Test SupportsResume for Blob (always true)
				if _, isBlob := tt.downloader.(*blobDownloader); isBlob {
					if !resumable.SupportsResume() {
						t.Error("Blob SupportsResume() should always return true")
					}
				}
			}
		})
	}
}

// TestHTTPDownloader_SupportsResume tests the SupportsResume method for HTTP downloader
func TestHTTPDownloader_SupportsResume(t *testing.T) {
	tests := []struct {
		name           string
		supportsRange  bool
		expectedResult bool
	}{
		{
			name:           "Server supports range requests",
			supportsRange:  true,
			expectedResult: true,
		},
		{
			name:           "Server does not support range requests",
			supportsRange:  false,
			expectedResult: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hd := &httpDownloader{
				supportsRange: tt.supportsRange,
			}

			result := hd.SupportsResume()
			if result != tt.expectedResult {
				t.Errorf("SupportsResume() = %v, want %v", result, tt.expectedResult)
			}

			// Also test via interface
			resumable, ok := interface{}(hd).(resumableDownloader)
			if !ok {
				t.Fatal("httpDownloader should implement resumableDownloader")
			}

			if resumable.SupportsResume() != tt.expectedResult {
				t.Errorf("Interface SupportsResume() = %v, want %v", resumable.SupportsResume(), tt.expectedResult)
			}
		})
	}
}

// TestBlobDownloader_SupportsResume tests the SupportsResume method for Blob downloader
func TestBlobDownloader_SupportsResume(t *testing.T) {
	bd := &blobDownloader{}

	// Blob should always support resume
	if !bd.SupportsResume() {
		t.Error("Blob downloader should always support resume")
	}

	// Also test via interface
	resumable, ok := interface{}(bd).(resumableDownloader)
	if !ok {
		t.Fatal("blobDownloader should implement resumableDownloader")
	}

	if !resumable.SupportsResume() {
		t.Error("Interface SupportsResume() should return true for blob")
	}
}

// TestResumableDownloadChunkFunc tests that createResumableDownloadChunkFunc creates the right type of chunk function
func TestResumableDownloadChunkFunc(t *testing.T) {
	// This test validates that we have the helper function for creating resumable download chunk funcs
	// The actual testing of the chunk func execution requires a full integration test with mocks

	// Just verify the function exists and can be called
	testBody := func() {
		// Empty test body
	}

	// This should not panic
	_ = createResumableDownloadChunkFunc(nil, common.NewChunkID("", 0, 0), testBody)
}

// TestDownloaderInterface ensures backward compatibility
func TestDownloaderInterface(t *testing.T) {
	// Ensure that resumableDownloader properly extends downloader
	var d downloader = &httpDownloader{}

	// Should still be assignable to base interface
	if d == nil {
		t.Error("downloader assignment failed")
	}

	// And should be upgradeable to resumableDownloader if it supports it
	if rd, ok := d.(resumableDownloader); ok {
		if rd == nil {
			t.Error("resumableDownloader upgrade failed")
		}
	}
}
