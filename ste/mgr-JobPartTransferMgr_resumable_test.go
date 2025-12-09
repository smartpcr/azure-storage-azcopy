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
)

// TestIsResumableDownload tests the getter and setter for resumable download flag
func TestIsResumableDownload(t *testing.T) {
	jptm := &jobPartTransferMgr{
		isResumableDownload: false,
	}

	// Test default value
	if jptm.IsResumableDownload() {
		t.Error("IsResumableDownload() should default to false")
	}

	// Test setter with true
	jptm.SetResumableDownload(true)
	if !jptm.IsResumableDownload() {
		t.Error("IsResumableDownload() should return true after SetResumableDownload(true)")
	}

	// Test setter with false
	jptm.SetResumableDownload(false)
	if jptm.IsResumableDownload() {
		t.Error("IsResumableDownload() should return false after SetResumableDownload(false)")
	}
}

// TestIsResumableDownload_Interface tests that the interface methods work correctly
func TestIsResumableDownload_Interface(t *testing.T) {
	jptm := &jobPartTransferMgr{
		isResumableDownload: false,
	}

	// Test via interface
	var ijptm IJobPartTransferMgr = jptm

	// Verify default value via interface
	if ijptm.IsResumableDownload() {
		t.Error("Interface IsResumableDownload() should default to false")
	}

	// Set via interface
	ijptm.SetResumableDownload(true)
	if !ijptm.IsResumableDownload() {
		t.Error("Interface IsResumableDownload() should return true after SetResumableDownload(true)")
	}

	// Verify the underlying struct was modified
	if !jptm.isResumableDownload {
		t.Error("Underlying struct field should be modified when setting via interface")
	}
}
