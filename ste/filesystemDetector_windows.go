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
	"os"
	"path/filepath"
	"strings"
	"syscall"

	"golang.org/x/sys/windows"
)

// FilesystemType represents the type of filesystem
type FilesystemType int

const (
	FilesystemTypeLocal   FilesystemType = iota // Local disk (NTFS, ReFS, etc.)
	FilesystemTypeCSV                           // Cluster Shared Volume
	FilesystemTypeSMB                           // SMB/CIFS network share
	FilesystemTypeUnknown                       // Unknown or unsupported
)

// FilesystemInfo contains detailed information about the filesystem
type FilesystemInfo struct {
	Type                FilesystemType
	IsRemote            bool
	IsCluster           bool
	SupportsMemoryMap   bool
	RequiresWriteThrough bool
	FileSystemName      string
	VolumeName          string
}

// detectFilesystem determines the filesystem type for a given path
func detectFilesystem(path string) (*FilesystemInfo, error) {
	info := &FilesystemInfo{
		Type:              FilesystemTypeUnknown,
		SupportsMemoryMap: true, // Assume supported unless we find otherwise
	}

	// Get absolute path
	absPath, err := filepath.Abs(path)
	if err != nil {
		return info, err
	}

	// Check for UNC path (network share)
	if strings.HasPrefix(absPath, `\\`) {
		info.Type = FilesystemTypeSMB
		info.IsRemote = true
		info.SupportsMemoryMap = false // Disable mmap on SMB by default
		return info, nil
	}

	// Check for CSV path (Cluster Shared Volumes)
	// CSV volumes are typically mounted at C:\ClusterStorage\...
	if strings.Contains(absPath, `\ClusterStorage\`) {
		info.Type = FilesystemTypeCSV
		info.IsCluster = true
		info.RequiresWriteThrough = true
		info.SupportsMemoryMap = true // mmap works on CSV with proper flags
		return info, nil
	}

	// Get the volume path (e.g., "C:\")
	volumePath := filepath.VolumeName(absPath) + `\`

	// Get volume information
	volumeNameBuf := make([]uint16, windows.MAX_PATH+1)
	fileSystemNameBuf := make([]uint16, windows.MAX_PATH+1)
	var volumeSerialNumber uint32
	var maxComponentLength uint32
	var fileSystemFlags uint32

	volumePathPtr, err := syscall.UTF16PtrFromString(volumePath)
	if err != nil {
		return info, err
	}

	err = windows.GetVolumeInformation(
		volumePathPtr,
		&volumeNameBuf[0],
		uint32(len(volumeNameBuf)),
		&volumeSerialNumber,
		&maxComponentLength,
		&fileSystemFlags,
		&fileSystemNameBuf[0],
		uint32(len(fileSystemNameBuf)),
	)

	if err != nil {
		// If we can't get volume info, assume local and allow mmap
		info.Type = FilesystemTypeLocal
		return info, nil
	}

	// Convert to strings
	info.VolumeName = syscall.UTF16ToString(volumeNameBuf)
	info.FileSystemName = syscall.UTF16ToString(fileSystemNameBuf)

	// Check if volume is remote using GetDriveType
	driveType := windows.GetDriveType(volumePathPtr)
	if driveType == windows.DRIVE_REMOTE {
		info.Type = FilesystemTypeSMB
		info.IsRemote = true
		info.SupportsMemoryMap = false
		return info, nil
	}

	// Check for CSV by examining filesystem characteristics
	// CSV volumes may have specific flags or characteristics
	// Additional heuristic: Check if it's a ReFS volume in cluster storage
	if info.FileSystemName == "CSVFS" || info.FileSystemName == "CSVFS_ReFS" {
		info.Type = FilesystemTypeCSV
		info.IsCluster = true
		info.RequiresWriteThrough = true
		info.SupportsMemoryMap = true
		return info, nil
	}

	// If we detect ReFS and volume name suggests clustering
	if info.FileSystemName == "ReFS" && strings.Contains(strings.ToUpper(info.VolumeName), "CLUSTER") {
		info.Type = FilesystemTypeCSV
		info.IsCluster = true
		info.RequiresWriteThrough = true
		info.SupportsMemoryMap = true
		return info, nil
	}

	// Default to local filesystem
	info.Type = FilesystemTypeLocal
	info.IsRemote = false
	info.IsCluster = false
	info.SupportsMemoryMap = true

	return info, nil
}

// openFileForMmap opens a file with appropriate flags for memory mapping
// based on the filesystem type
func openFileForMmap(path string, size int64) (*os.File, *FilesystemInfo, error) {
	// Detect filesystem type
	fsInfo, err := detectFilesystem(path)
	if err != nil {
		// If detection fails, proceed with default flags
		file, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0644)
		return file, fsInfo, err
	}

	// Determine flags based on filesystem type
	var createFlags uint32 = windows.FILE_ATTRIBUTE_NORMAL

	if fsInfo.Type == FilesystemTypeCSV {
		// For CSV, use write-through to ensure cache coherency across nodes
		createFlags |= windows.FILE_FLAG_WRITE_THROUGH
	}

	if !fsInfo.SupportsMemoryMap {
		// For filesystems that don't support mmap well (SMB),
		// we should fall back to regular I/O
		// For now, return error - caller should handle fallback
		return nil, fsInfo, &FilesystemNotSupportedError{
			Path:       path,
			FSType:     fsInfo.Type,
			Reason:     "Memory mapping not recommended on this filesystem type",
			Suggestion: "Use regular file I/O instead",
		}
	}

	// Convert path to UTF16
	pathPtr, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return nil, fsInfo, err
	}

	// Create file with Windows API for precise control
	handle, err := windows.CreateFile(
		pathPtr,
		windows.GENERIC_READ|windows.GENERIC_WRITE,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE,
		nil,
		windows.CREATE_ALWAYS, // Equivalent to O_CREATE|O_TRUNC
		createFlags,
		0,
	)

	if err != nil {
		return nil, fsInfo, err
	}

	// Convert Windows handle to os.File
	file := os.NewFile(uintptr(handle), path)

	// Pre-allocate space
	if err := file.Truncate(size); err != nil {
		file.Close()
		return nil, fsInfo, err
	}

	return file, fsInfo, nil
}

// FilesystemNotSupportedError indicates the filesystem doesn't support mmap well
type FilesystemNotSupportedError struct {
	Path       string
	FSType     FilesystemType
	Reason     string
	Suggestion string
}

func (e *FilesystemNotSupportedError) Error() string {
	fsTypeName := "unknown"
	switch e.FSType {
	case FilesystemTypeLocal:
		fsTypeName = "local"
	case FilesystemTypeCSV:
		fsTypeName = "CSV"
	case FilesystemTypeSMB:
		fsTypeName = "SMB"
	}
	return "filesystem not supported for mmap: path=" + e.Path +
		", type=" + fsTypeName +
		", reason=" + e.Reason +
		", suggestion=" + e.Suggestion
}

// IsNetworkFilesystem returns true if the filesystem is network-based
func (info *FilesystemInfo) IsNetworkFilesystem() bool {
	return info.IsRemote || info.Type == FilesystemTypeSMB
}

// ShouldUseMmap returns true if memory mapping is recommended for this filesystem
func (info *FilesystemInfo) ShouldUseMmap() bool {
	return info.SupportsMemoryMap
}

// GetRecommendedFlags returns recommended file open flags for this filesystem
func (info *FilesystemInfo) GetRecommendedFlags() uint32 {
	var flags uint32 = windows.FILE_ATTRIBUTE_NORMAL

	if info.RequiresWriteThrough {
		flags |= windows.FILE_FLAG_WRITE_THROUGH
	}

	return flags
}
