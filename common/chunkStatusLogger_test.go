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

package common

import (
	"testing"
)

// Test ChunkID backward compatibility and new chunk index functionality
func TestChunkID_ChunkIndex(t *testing.T) {
	// Create a ChunkID without chunk index (backward compatible)
	chunkID := ChunkID{
		Name: "test-chunk-1",
	}

	// Verify HasChunkIndex returns false when not set
	if chunkID.HasChunkIndex() {
		t.Error("Expected HasChunkIndex to return false for new ChunkID")
	}

	// Verify ChunkIndex returns 0 when not set
	if idx := chunkID.ChunkIndex(); idx != 0 {
		t.Errorf("Expected ChunkIndex to return 0, got %d", idx)
	}

	// Set chunk index
	chunkID.SetChunkIndex(42)

	// Verify HasChunkIndex returns true after setting
	if !chunkID.HasChunkIndex() {
		t.Error("Expected HasChunkIndex to return true after SetChunkIndex")
	}

	// Verify ChunkIndex returns correct value
	if idx := chunkID.ChunkIndex(); idx != 42 {
		t.Errorf("Expected ChunkIndex to return 42, got %d", idx)
	}
}

func TestChunkID_ChunkIndexZero(t *testing.T) {
	// Test that index 0 is valid
	chunkID := ChunkID{
		Name: "chunk-0",
	}

	chunkID.SetChunkIndex(0)

	if !chunkID.HasChunkIndex() {
		t.Error("Expected HasChunkIndex to return true for index 0")
	}

	if idx := chunkID.ChunkIndex(); idx != 0 {
		t.Errorf("Expected ChunkIndex to return 0, got %d", idx)
	}
}

func TestChunkID_ChunkIndexMultipleSet(t *testing.T) {
	// Test setting chunk index multiple times
	chunkID := ChunkID{
		Name: "chunk",
	}

	chunkID.SetChunkIndex(10)
	if idx := chunkID.ChunkIndex(); idx != 10 {
		t.Errorf("Expected ChunkIndex to return 10, got %d", idx)
	}

	chunkID.SetChunkIndex(20)
	if idx := chunkID.ChunkIndex(); idx != 20 {
		t.Errorf("Expected ChunkIndex to return 20, got %d", idx)
	}

	chunkID.SetChunkIndex(30)
	if idx := chunkID.ChunkIndex(); idx != 30 {
		t.Errorf("Expected ChunkIndex to return 30, got %d", idx)
	}
}
