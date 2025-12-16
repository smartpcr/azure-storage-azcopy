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

//go:build windows

package ste

import (
	"testing"
)

// TestFilesystemInfo_Methods_Windows tests the FilesystemInfo helper methods on Windows
func TestFilesystemInfo_Methods_Windows(t *testing.T) {
	tests := []struct {
		name     string
		info     FilesystemInfo
		wantMmap bool
		wantNet  bool
	}{
		{
			name: "local filesystem",
			info: FilesystemInfo{
				Type:              FilesystemTypeLocal,
				IsRemote:          false,
				SupportsMemoryMap: true,
			},
			wantMmap: true,
			wantNet:  false,
		},
		{
			name: "network filesystem (SMB)",
			info: FilesystemInfo{
				Type:              FilesystemTypeSMB,
				IsRemote:          true,
				SupportsMemoryMap: false,
			},
			wantMmap: false,
			wantNet:  true,
		},
		{
			name: "cluster filesystem (CSV)",
			info: FilesystemInfo{
				Type:              FilesystemTypeCSV,
				IsCluster:         true,
				SupportsMemoryMap: true,
			},
			wantMmap: true,
			wantNet:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.info.ShouldUseMmap(); got != tt.wantMmap {
				t.Errorf("ShouldUseMmap() = %v, want %v", got, tt.wantMmap)
			}

			if got := tt.info.IsNetworkFilesystem(); got != tt.wantNet {
				t.Errorf("IsNetworkFilesystem() = %v, want %v", got, tt.wantNet)
			}
		})
	}
}

// TestUnsupportedFilesystemError_Windows tests the error type on Windows
func TestUnsupportedFilesystemError_Windows(t *testing.T) {
	fsInfo := &FilesystemInfo{
		Type:     FilesystemTypeSMB,
		IsRemote: true,
	}

	err := &UnsupportedFilesystemError{
		Path:   `\\server\share\test.dat`,
		FSInfo: fsInfo,
		Err:    &FilesystemNotSupportedError{},
	}

	errStr := err.Error()
	if errStr == "" {
		t.Error("Error string should not be empty")
	}

	t.Logf("Error message: %s", errStr)
}
