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
	"syscall"
	"time"
	"unsafe"
)

// This file implements file locking for Windows systems.
// It uses the LockFileEx/UnlockFileEx Windows API to prevent multiple AzCopy
// processes from simultaneously resuming the same download, which would cause
// data corruption.
//
// File locking behavior:
//   - LOCKFILE_EXCLUSIVE_LOCK: Exclusive lock (only one process can hold it)
//   - LOCKFILE_FAIL_IMMEDIATELY: Non-blocking (fail immediately if lock is held)
//   - Uses OVERLAPPED structure for specifying lock range (we lock 1 byte at offset 0)
//
// The lock is automatically released when:
//   - The file handle is closed
//   - The process terminates (gracefully or via crash)
//   - UnlockFileEx() is called explicitly
//
// Timeout handling:
//   - LockFileExclusiveWait() retries for up to 30 seconds (default)
//   - Polls every 100ms to avoid busy-waiting
//   - Returns FileLockTimeoutError if timeout is reached
//
// Platform-specific notes:
//   - Windows requires OVERLAPPED structure even for synchronous locks
//   - We lock 1 byte at offset 0 (the entire lock range is arbitrary)
//   - Lock granularity is per-handle, not per-process

var (
	kernel32    = syscall.NewLazyDLL("kernel32.dll")
	lockFileEx  = kernel32.NewProc("LockFileEx")
	unlockFileEx = kernel32.NewProc("UnlockFileEx")
)

const (
	LOCKFILE_EXCLUSIVE_LOCK   = 0x00000002
	LOCKFILE_FAIL_IMMEDIATELY = 0x00000001
)

// LockFileExclusive acquires an exclusive lock on the file
// This prevents other processes from locking the same file
func LockFileExclusive(file *os.File) error {
	var overlapped syscall.Overlapped

	ret, _, err := lockFileEx.Call(
		uintptr(syscall.Handle(file.Fd())),
		uintptr(LOCKFILE_EXCLUSIVE_LOCK|LOCKFILE_FAIL_IMMEDIATELY),
		uintptr(0),
		uintptr(1),     // Lock 1 byte
		uintptr(0),
		uintptr(unsafe.Pointer(&overlapped)),
	)

	if ret == 0 {
		return fmt.Errorf("LockFileEx failed: %w", err)
	}

	return nil
}

// LockFileExclusiveWait acquires an exclusive lock on the file, waiting if necessary
// with a timeout
func LockFileExclusiveWait(file *os.File, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)

	for {
		var overlapped syscall.Overlapped

		ret, _, err := lockFileEx.Call(
			uintptr(syscall.Handle(file.Fd())),
			uintptr(LOCKFILE_EXCLUSIVE_LOCK|LOCKFILE_FAIL_IMMEDIATELY),
			uintptr(0),
			uintptr(1), // Lock 1 byte
			uintptr(0),
			uintptr(unsafe.Pointer(&overlapped)),
		)

		if ret != 0 {
			return nil
		}

		// Check timeout
		if time.Now().After(deadline) {
			return &FileLockTimeoutError{
				Path:    file.Name(),
				Timeout: timeout,
			}
		}

		// Wait a bit before retrying
		time.Sleep(100 * time.Millisecond)
	}
}

// UnlockFile releases the lock on the file
func UnlockFile(file *os.File) error {
	var overlapped syscall.Overlapped

	ret, _, err := unlockFileEx.Call(
		uintptr(syscall.Handle(file.Fd())),
		uintptr(0),
		uintptr(1), // Unlock 1 byte
		uintptr(0),
		uintptr(unsafe.Pointer(&overlapped)),
	)

	if ret == 0 {
		return fmt.Errorf("UnlockFileEx failed: %w", err)
	}

	return nil
}

// FileLockTimeoutError indicates the file lock could not be acquired within the timeout
type FileLockTimeoutError struct {
	Path    string
	Timeout time.Duration
}

func (e *FileLockTimeoutError) Error() string {
	return fmt.Sprintf(`failed to acquire file lock after %v:
  File: %s

This usually means another AzCopy process is resuming the same download.

Suggested actions:
  1. Wait for the other process to complete
  2. Check Task Manager for running AzCopy processes
  3. If the other process crashed, manually delete the chunk progress file:
     del %USERPROFILE%\.azcopy\*.chunks
  4. Restart the download with 'azcopy jobs resume <jobID>'`,
		e.Timeout,
		e.Path)
}
