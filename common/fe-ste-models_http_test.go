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

	"github.com/stretchr/testify/assert"
)

func TestHTTPLocation_Value(t *testing.T) {
	httpLoc := ELocation.Http()
	assert.Equal(t, Location(11), httpLoc, "HTTP location should have value 11")
	assert.NotEqual(t, Location(0), httpLoc, "HTTP location should not be zero")
}

func TestHTTPLocation_String(t *testing.T) {
	httpLoc := ELocation.Http()
	assert.Equal(t, "Http", httpLoc.String(), "HTTP location string representation should be 'Http'")
}

func TestHTTPLocation_IsRemote(t *testing.T) {
	httpLoc := ELocation.Http()
	assert.True(t, httpLoc.IsRemote(), "HTTP location should be remote")
}

func TestHTTPLocation_IsLocal(t *testing.T) {
	httpLoc := ELocation.Http()
	assert.False(t, httpLoc.IsLocal(), "HTTP location should not be local")
}

func TestHTTPLocation_IsAzure(t *testing.T) {
	httpLoc := ELocation.Http()
	assert.False(t, httpLoc.IsAzure(), "HTTP location should not be Azure")
}

func TestHTTPLocation_IsFolderAware(t *testing.T) {
	httpLoc := ELocation.Http()
	assert.False(t, httpLoc.IsFolderAware(), "HTTP location should not be folder-aware")
}

func TestHTTPLocation_CanForwardOAuthTokens(t *testing.T) {
	httpLoc := ELocation.Http()
	assert.False(t, httpLoc.CanForwardOAuthTokens(), "HTTP location should not forward OAuth tokens automatically")
}

func TestHTTPLocation_SupportsHnsACLs(t *testing.T) {
	httpLoc := ELocation.Http()
	assert.False(t, httpLoc.SupportsHnsACLs(), "HTTP location should not support HNS ACLs")
}

func TestHTTPLocation_IsFile(t *testing.T) {
	httpLoc := ELocation.Http()
	assert.False(t, httpLoc.IsFile(), "HTTP location should not be File")
}

func TestHTTPLocation_SupportsTrailingDot(t *testing.T) {
	httpLoc := ELocation.Http()
	assert.False(t, httpLoc.SupportsTrailingDot(), "HTTP location should not support trailing dots")
}

func TestHTTPLocation_Parse(t *testing.T) {
	var loc Location
	err := loc.Parse("Http")
	assert.NoError(t, err, "Should parse 'Http' string")
	assert.Equal(t, ELocation.Http(), loc, "Parsed location should be Http")
}

func TestHTTPLocation_Uniqueness(t *testing.T) {
	// Ensure HTTP location is unique and doesn't conflict with other locations
	httpLoc := ELocation.Http()

	allLocations := []Location{
		ELocation.Unknown(),
		ELocation.Local(),
		ELocation.Pipe(),
		ELocation.Blob(),
		ELocation.File(),
		ELocation.BlobFS(),
		ELocation.S3(),
		ELocation.Benchmark(),
		ELocation.GCP(),
		ELocation.None(),
		ELocation.FileNFS(),
	}

	for _, loc := range allLocations {
		assert.NotEqual(t, httpLoc, loc, "HTTP location should be unique from %s", loc.String())
	}
}

func TestHttpLocalFromTo_Creation(t *testing.T) {
	ft := EFromTo.HttpLocal()
	assert.NotEqual(t, FromTo(0), ft, "HttpLocal FromTo should have non-zero value")
}

func TestHttpLocalFromTo_Components(t *testing.T) {
	ft := EFromTo.HttpLocal()

	// Extract from and to using bit operations (reverse of FromToValue)
	from := Location(ft >> 8)
	to := Location(ft & 0xFF)

	assert.Equal(t, ELocation.Http(), from, "From should be HTTP")
	assert.Equal(t, ELocation.Local(), to, "To should be Local")
}

func TestHttpLocalFromTo_String(t *testing.T) {
	ft := EFromTo.HttpLocal()
	str := ft.String()
	assert.Contains(t, str, "Http", "String representation should contain 'Http'")
	assert.Contains(t, str, "Local", "String representation should contain 'Local'")
}

func TestHttpLocalFromTo_Uniqueness(t *testing.T) {
	httpLocal := EFromTo.HttpLocal()

	// Verify it's different from other FromTo combinations
	assert.NotEqual(t, EFromTo.LocalBlob(), httpLocal)
	assert.NotEqual(t, EFromTo.BlobLocal(), httpLocal)
	assert.NotEqual(t, EFromTo.Unknown(), httpLocal)
}

func TestFromToValue_HttpLocal(t *testing.T) {
	// Test that FromToValue creates correct HttpLocal value
	fromTo := FromToValue(ELocation.Http(), ELocation.Local())
	assert.Equal(t, EFromTo.HttpLocal(), fromTo, "FromToValue should create correct HttpLocal")
}

func TestHttpLocation_AllRemoteLocations(t *testing.T) {
	// Verify HTTP is included in remote locations
	remoteLocations := []Location{
		ELocation.Blob(),
		ELocation.File(),
		ELocation.BlobFS(),
		ELocation.S3(),
		ELocation.GCP(),
		ELocation.FileNFS(),
		ELocation.Http(),
	}

	for _, loc := range remoteLocations {
		assert.True(t, loc.IsRemote(), "%s should be remote", loc.String())
	}
}

func TestHttpLocation_AllLocalLocations(t *testing.T) {
	// Verify HTTP is not in local locations
	localLocations := []Location{
		ELocation.Local(),
		ELocation.Benchmark(),
		ELocation.Pipe(),
		ELocation.Unknown(),
		ELocation.None(),
	}

	for _, loc := range localLocations {
		assert.False(t, loc.IsRemote(), "%s should not be remote", loc.String())
	}

	// Ensure HTTP is not in this list
	httpLoc := ELocation.Http()
	for _, loc := range localLocations {
		assert.NotEqual(t, httpLoc, loc, "HTTP should not be in local locations")
	}
}