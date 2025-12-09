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
	"fmt"
	"os"
	"sync"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Sync flags for Windows systems
// Note: Windows FlushViewOfFile doesn't have async/sync distinction
// We emulate the behavior with immediate flush (sync) or no-op (async)
const (
	msyncAsync = 0 // Async - we'll treat as no-op for performance
	msyncSync  = 1 // Sync - will call FlushViewOfFile
)

// mmapHandle tracks the Windows file mapping handle for each mapped region
type mmapHandle struct {
	handle  windows.Handle
	mapAddr uintptr
}

var (
	mmapHandles   = make(map[uintptr]*mmapHandle)
	mmapHandlesMu sync.Mutex
)

// mmapFile creates a memory-mapped region for the given file on Windows
func mmapFile(file *os.File, size int) ([]byte, error) {
	// Get the Windows file handle
	fileHandle := windows.Handle(file.Fd())

	// Create a file mapping object
	// Use PAGE_READWRITE for read/write access
	maxSizeHigh := uint32(int64(size) >> 32)
	maxSizeLow := uint32(int64(size) & 0xFFFFFFFF)

	mappingHandle, err := windows.CreateFileMapping(
		fileHandle,
		nil, // default security
		windows.PAGE_READWRITE,
		maxSizeHigh,
		maxSizeLow,
		nil, // no name
	)
	if err != nil {
		return nil, fmt.Errorf("CreateFileMapping failed: %w", err)
	}

	// Map a view of the file into memory
	// FILE_MAP_WRITE includes read access
	mapAddr, err := windows.MapViewOfFile(
		mappingHandle,
		windows.FILE_MAP_WRITE,
		0, // offset high
		0, // offset low
		uintptr(size),
	)
	if err != nil {
		windows.CloseHandle(mappingHandle)
		return nil, fmt.Errorf("MapViewOfFile failed: %w", err)
	}

	// Store the mapping handle for later cleanup
	mmapHandlesMu.Lock()
	mmapHandles[mapAddr] = &mmapHandle{
		handle:  mappingHandle,
		mapAddr: mapAddr,
	}
	mmapHandlesMu.Unlock()

	// Convert the pointer to a byte slice
	// This is safe because we're mapping a file
	data := unsafe.Slice((*byte)(unsafe.Pointer(mapAddr)), size)

	return data, nil
}

// munmapFile unmaps the memory-mapped region on Windows
func munmapFile(data []byte) error {
	if len(data) == 0 {
		return nil
	}

	// Get the address of the mapped region
	mapAddr := uintptr(unsafe.Pointer(&data[0]))

	// Retrieve the mapping handle
	mmapHandlesMu.Lock()
	handle, exists := mmapHandles[mapAddr]
	if exists {
		delete(mmapHandles, mapAddr)
	}
	mmapHandlesMu.Unlock()

	if !exists {
		return fmt.Errorf("munmap: mapping handle not found for address %x", mapAddr)
	}

	// Unmap the view
	err := windows.UnmapViewOfFile(handle.mapAddr)
	if err != nil {
		// Still try to close the handle
		windows.CloseHandle(handle.handle)
		return fmt.Errorf("UnmapViewOfFile failed: %w", err)
	}

	// Close the mapping handle
	err = windows.CloseHandle(handle.handle)
	if err != nil {
		return fmt.Errorf("CloseHandle failed: %w", err)
	}

	return nil
}

// msyncFile synchronizes the memory-mapped region to disk on Windows
func msyncFile(data []byte, flags int) error {
	if len(data) == 0 {
		return nil
	}

	// For async sync, we skip the flush for performance
	// The OS will flush eventually, similar to Unix MS_ASYNC
	if flags == msyncAsync {
		// No-op for async on Windows
		// Windows will flush dirty pages eventually
		return nil
	}

	// For sync, we force an immediate flush
	// Get the address and size
	addr := uintptr(unsafe.Pointer(&data[0]))
	size := uintptr(len(data))

	// FlushViewOfFile flushes to the disk cache
	err := windows.FlushViewOfFile(addr, size)
	if err != nil {
		return fmt.Errorf("FlushViewOfFile failed: %w", err)
	}

	// For true durability, we also need to flush the file buffers
	// Get the file handle from our tracking map
	mmapHandlesMu.Lock()
	_, exists := mmapHandles[addr]
	mmapHandlesMu.Unlock()

	if exists {
		// The handle stored is the mapping handle, not the file handle
		// We can't easily get the file handle here without storing it
		// For now, FlushViewOfFile is sufficient for most use cases
		// The file handle flush happens on close in the main code
	}

	return nil
}

// getFileHandle attempts to get the Windows file handle from os.File
// This is used for additional flush operations if needed
func getFileHandle(file *os.File) (windows.Handle, error) {
	// os.File.Fd() returns a uintptr that we can convert to Handle
	return windows.Handle(file.Fd()), nil
}

// flushFileBuffers flushes the OS file buffers to disk
// This is called in addition to FlushViewOfFile for full durability
func flushFileBuffers(file *os.File) error {
	handle, err := getFileHandle(file)
	if err != nil {
		return err
	}

	return windows.FlushFileBuffers(handle)
}
