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

//go:build linux || darwin || freebsd || openbsd || netbsd

package ste

import (
	"os"
	"syscall"
)

// FilesystemType represents the type of filesystem
type FilesystemType int

const (
	FilesystemTypeLocal   FilesystemType = iota // Local disk (ext4, xfs, btrfs, etc.)
	FilesystemTypeNFS                           // NFS network filesystem
	FilesystemTypeCIFS                          // CIFS/SMB network filesystem
	FilesystemTypeUnknown                       // Unknown or unsupported
)

// FilesystemInfo contains detailed information about the filesystem
type FilesystemInfo struct {
	Type              FilesystemType
	IsRemote          bool
	SupportsMemoryMap bool
	FileSystemType    int64 // Filesystem type constant from syscall
	FileSystemName    string
}

// Filesystem type constants for common filesystems
const (
	// Linux filesystem magic numbers
	NFS_SUPER_MAGIC   = 0x6969     // NFS
	SMB_SUPER_MAGIC   = 0x517B      // SMB/CIFS
	SMB2_MAGIC_NUMBER = 0xFE534D42  // SMB2
	CIFS_MAGIC_NUMBER = 0xFF534D42  // CIFS
	EXT4_SUPER_MAGIC  = 0xEF53      // ext4
	XFS_SUPER_MAGIC   = 0x58465342  // XFS
	BTRFS_SUPER_MAGIC = 0x9123683E  // btrfs
	TMPFS_MAGIC       = 0x01021994  // tmpfs
)

// detectFilesystem determines the filesystem type for a given path on Unix systems
func detectFilesystem(path string) (*FilesystemInfo, error) {
	info := &FilesystemInfo{
		Type:              FilesystemTypeUnknown,
		SupportsMemoryMap: true, // Assume supported unless we find otherwise
	}

	// Use statfs to get filesystem information
	var stat syscall.Statfs_t
	err := syscall.Statfs(path, &stat)
	if err != nil {
		// If statfs fails, assume local and allow mmap
		info.Type = FilesystemTypeLocal
		return info, nil
	}

	info.FileSystemType = int64(stat.Type)

	// Detect filesystem type based on magic number
	switch stat.Type {
	case NFS_SUPER_MAGIC:
		info.Type = FilesystemTypeNFS
		info.IsRemote = true
		info.FileSystemName = "NFS"
		// NFS and mmap can be problematic
		// Cache coherency issues similar to CSV
		// Recommend against mmap on NFS
		info.SupportsMemoryMap = false

	case SMB_SUPER_MAGIC, SMB2_MAGIC_NUMBER, CIFS_MAGIC_NUMBER:
		info.Type = FilesystemTypeCIFS
		info.IsRemote = true
		info.FileSystemName = "CIFS/SMB"
		// CIFS/SMB has known issues with mmap
		// Particularly on Linux clients
		info.SupportsMemoryMap = false

	case EXT4_SUPER_MAGIC:
		info.Type = FilesystemTypeLocal
		info.FileSystemName = "ext4"
		info.SupportsMemoryMap = true

	case XFS_SUPER_MAGIC:
		info.Type = FilesystemTypeLocal
		info.FileSystemName = "XFS"
		info.SupportsMemoryMap = true

	case BTRFS_SUPER_MAGIC:
		info.Type = FilesystemTypeLocal
		info.FileSystemName = "btrfs"
		info.SupportsMemoryMap = true

	case TMPFS_MAGIC:
		info.Type = FilesystemTypeLocal
		info.FileSystemName = "tmpfs"
		info.SupportsMemoryMap = true

	default:
		// Unknown filesystem - assume local and allow mmap
		// Most local filesystems support mmap well
		info.Type = FilesystemTypeLocal
		info.FileSystemName = "unknown"
		info.SupportsMemoryMap = true
	}

	return info, nil
}

// openFileForMmap opens a file with appropriate flags for memory mapping
// based on the filesystem type (Unix version)
func openFileForMmap(path string, size int64) (*os.File, *FilesystemInfo, error) {
	// Detect filesystem type
	fsInfo, err := detectFilesystem(path)
	if err != nil {
		// If detection fails, proceed with default flags
		file, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0644)
		return file, fsInfo, err
	}

	// Check if mmap is recommended
	if !fsInfo.SupportsMemoryMap {
		// For network filesystems that don't support mmap well,
		// return error - caller should handle fallback
		return nil, fsInfo, &FilesystemNotSupportedError{
			Path:       path,
			FSType:     fsInfo.Type,
			Reason:     "Memory mapping not recommended on " + fsInfo.FileSystemName,
			Suggestion: "Use regular file I/O instead",
		}
	}

	// Create file with standard flags
	file, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0644)
	if err != nil {
		return nil, fsInfo, err
	}

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
	case FilesystemTypeNFS:
		fsTypeName = "NFS"
	case FilesystemTypeCIFS:
		fsTypeName = "CIFS/SMB"
	}
	return "filesystem not supported for mmap: path=" + e.Path +
		", type=" + fsTypeName +
		", reason=" + e.Reason +
		", suggestion=" + e.Suggestion
}

// IsNetworkFilesystem returns true if the filesystem is network-based
func (info *FilesystemInfo) IsNetworkFilesystem() bool {
	return info.IsRemote
}

// ShouldUseMmap returns true if memory mapping is recommended for this filesystem
func (info *FilesystemInfo) ShouldUseMmap() bool {
	return info.SupportsMemoryMap
}
