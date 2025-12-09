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

	"golang.org/x/sys/unix"
)

// Sync flags for Unix systems
const (
	msyncAsync = unix.MS_ASYNC
	msyncSync  = unix.MS_SYNC
)

// mmapFile creates a memory-mapped region for the given file on Unix systems
func mmapFile(file *os.File, size int) ([]byte, error) {
	return syscall.Mmap(
		int(file.Fd()),
		0,
		size,
		syscall.PROT_READ|syscall.PROT_WRITE,
		syscall.MAP_SHARED,
	)
}

// munmapFile unmaps the memory-mapped region on Unix systems
func munmapFile(data []byte) error {
	return syscall.Munmap(data)
}

// msyncFile synchronizes the memory-mapped region to disk on Unix systems
func msyncFile(data []byte, flags int) error {
	return unix.Msync(data, flags)
}
