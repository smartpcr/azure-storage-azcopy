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
	"fmt"
	"os"
	"syscall"
	"time"
)

// LockFileExclusive acquires an exclusive lock on the file
// This prevents other processes from locking the same file
func LockFileExclusive(file *os.File) error {
	return syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
}

// LockFileExclusiveWait acquires an exclusive lock on the file, waiting if necessary
// with a timeout
func LockFileExclusiveWait(file *os.File, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)

	for {
		err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
		if err == nil {
			return nil
		}

		// Check if it's a "would block" error
		if err != syscall.EWOULDBLOCK && err != syscall.EAGAIN {
			return fmt.Errorf("flock failed: %w", err)
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
	return syscall.Flock(int(file.Fd()), syscall.LOCK_UN)
}

// FileLockTimeoutError indicates the file lock could not be acquired within the timeout
type FileLockTimeoutError struct {
	Path    string
	Timeout time.Duration
}

func (e *FileLockTimeoutError) Error() string {
	return fmt.Sprintf("failed to acquire file lock on %s after %v (another process may be using this file)",
		e.Path, e.Timeout)
}
