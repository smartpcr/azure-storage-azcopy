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

func TestHTTPURLParts_SimpleHTTPSURL(t *testing.T) {
	url := "https://api.example.com/files/data.bin"
	parts, err := NewHTTPURLParts(url)

	assert.NoError(t, err)
	assert.Equal(t, "https", parts.Scheme)
	assert.Equal(t, "api.example.com", parts.Host)
	assert.Equal(t, "", parts.Port)
	assert.Equal(t, "/files/data.bin", parts.Path)
	assert.Equal(t, "", parts.Query)
	assert.Equal(t, "", parts.Fragment)
	assert.Equal(t, url, parts.URL)
}

func TestHTTPURLParts_SimpleHTTPURL(t *testing.T) {
	url := "http://example.com/file.txt"
	parts, err := NewHTTPURLParts(url)

	assert.NoError(t, err)
	assert.Equal(t, "http", parts.Scheme)
	assert.Equal(t, "example.com", parts.Host)
	assert.Equal(t, "", parts.Port)
	assert.Equal(t, "/file.txt", parts.Path)
}

func TestHTTPURLParts_URLWithPort(t *testing.T) {
	url := "http://localhost:8080/download"
	parts, err := NewHTTPURLParts(url)

	assert.NoError(t, err)
	assert.Equal(t, "http", parts.Scheme)
	assert.Equal(t, "localhost", parts.Host)
	assert.Equal(t, "8080", parts.Port)
	assert.Equal(t, "/download", parts.Path)
}

func TestHTTPURLParts_URLWithQuery(t *testing.T) {
	url := "https://api.example.com/files?version=2&format=json"
	parts, err := NewHTTPURLParts(url)

	assert.NoError(t, err)
	assert.Equal(t, "https", parts.Scheme)
	assert.Equal(t, "api.example.com", parts.Host)
	assert.Equal(t, "/files", parts.Path)
	assert.Equal(t, "version=2&format=json", parts.Query)
}

func TestHTTPURLParts_URLWithFragment(t *testing.T) {
	url := "https://docs.example.com/page#section1"
	parts, err := NewHTTPURLParts(url)

	assert.NoError(t, err)
	assert.Equal(t, "https", parts.Scheme)
	assert.Equal(t, "docs.example.com", parts.Host)
	assert.Equal(t, "/page", parts.Path)
	assert.Equal(t, "section1", parts.Fragment)
}

func TestHTTPURLParts_ComplexURL(t *testing.T) {
	url := "https://api.example.com:8443/files/data.bin?version=2&format=json#metadata"
	parts, err := NewHTTPURLParts(url)

	assert.NoError(t, err)
	assert.Equal(t, "https", parts.Scheme)
	assert.Equal(t, "api.example.com", parts.Host)
	assert.Equal(t, "8443", parts.Port)
	assert.Equal(t, "/files/data.bin", parts.Path)
	assert.Equal(t, "version=2&format=json", parts.Query)
	assert.Equal(t, "metadata", parts.Fragment)
}

func TestHTTPURLParts_RootPath(t *testing.T) {
	url := "https://example.com/"
	parts, err := NewHTTPURLParts(url)

	assert.NoError(t, err)
	assert.Equal(t, "https", parts.Scheme)
	assert.Equal(t, "example.com", parts.Host)
	assert.Equal(t, "/", parts.Path)
}

func TestHTTPURLParts_NoPath(t *testing.T) {
	url := "https://example.com"
	parts, err := NewHTTPURLParts(url)

	assert.NoError(t, err)
	assert.Equal(t, "https", parts.Scheme)
	assert.Equal(t, "example.com", parts.Host)
	assert.Equal(t, "", parts.Path)
}

func TestHTTPURLParts_URLWithSpecialChars(t *testing.T) {
	url := "https://example.com/path%20with%20spaces/file%2Bname.txt"
	parts, err := NewHTTPURLParts(url)

	assert.NoError(t, err)
	// Go's URL parser automatically decodes the path
	assert.Equal(t, "/path with spaces/file+name.txt", parts.Path)
}

func TestHTTPURLParts_URLWithAuthentication(t *testing.T) {
	// URL parser should handle but we ignore user:pass in our logic
	url := "https://user:pass@example.com/file"
	parts, err := NewHTTPURLParts(url)

	assert.NoError(t, err)
	assert.Equal(t, "https", parts.Scheme)
	assert.Equal(t, "example.com", parts.Host)
	assert.Equal(t, "/file", parts.Path)
}

func TestHTTPURLParts_IPv4Host(t *testing.T) {
	url := "http://192.168.1.1:8080/file"
	parts, err := NewHTTPURLParts(url)

	assert.NoError(t, err)
	assert.Equal(t, "192.168.1.1", parts.Host)
	assert.Equal(t, "8080", parts.Port)
}

func TestHTTPURLParts_IPv6Host(t *testing.T) {
	url := "https://[2001:db8::1]:8080/file"
	parts, err := NewHTTPURLParts(url)

	assert.NoError(t, err)
	assert.Equal(t, "2001:db8::1", parts.Host)
	assert.Equal(t, "8080", parts.Port)
}

func TestHTTPURLParts_IPv6HostNoPort(t *testing.T) {
	url := "https://[2001:db8::1]/file"
	parts, err := NewHTTPURLParts(url)

	assert.NoError(t, err)
	assert.Equal(t, "2001:db8::1", parts.Host)
	assert.Equal(t, "", parts.Port)
}

func TestHTTPURLParts_InvalidScheme(t *testing.T) {
	url := "ftp://example.com/file"
	_, err := NewHTTPURLParts(url)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "expected http or https")
}

func TestHTTPURLParts_NoScheme(t *testing.T) {
	url := "example.com/file"
	_, err := NewHTTPURLParts(url)

	// This should parse but scheme will be empty, so should fail
	assert.Error(t, err)
}

func TestHTTPURLParts_EmptyURL(t *testing.T) {
	url := ""
	_, err := NewHTTPURLParts(url)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "cannot be empty")
}

func TestHTTPURLParts_MalformedURL(t *testing.T) {
	url := "ht!tp://bad url with spaces"
	_, err := NewHTTPURLParts(url)

	assert.Error(t, err)
}

func TestHTTPURLParts_OnlyScheme(t *testing.T) {
	url := "https://"
	parts, err := NewHTTPURLParts(url)

	// This should parse without error (empty host is valid in URL parsing)
	assert.NoError(t, err)
	assert.Equal(t, "https", parts.Scheme)
	assert.Equal(t, "", parts.Host)
}

func TestHTTPURLParts_String(t *testing.T) {
	originalURL := "https://api.example.com:8443/files/data.bin?version=2"
	parts, err := NewHTTPURLParts(originalURL)

	assert.NoError(t, err)
	assert.Equal(t, originalURL, parts.String())
}

func TestHTTPURLParts_IsSecure_HTTPS(t *testing.T) {
	url := "https://example.com/file"
	parts, err := NewHTTPURLParts(url)

	assert.NoError(t, err)
	assert.True(t, parts.IsSecure())
}

func TestHTTPURLParts_IsSecure_HTTP(t *testing.T) {
	url := "http://example.com/file"
	parts, err := NewHTTPURLParts(url)

	assert.NoError(t, err)
	assert.False(t, parts.IsSecure())
}

func TestHTTPURLParts_URLWithMultipleQueryParams(t *testing.T) {
	url := "https://example.com/api?param1=value1&param2=value2&param3=value3"
	parts, err := NewHTTPURLParts(url)

	assert.NoError(t, err)
	assert.Equal(t, "param1=value1&param2=value2&param3=value3", parts.Query)
}

func TestHTTPURLParts_URLWithEncodedQuery(t *testing.T) {
	url := "https://example.com/search?q=hello%20world&filter=%2Fpath%2Fto%2Ffile"
	parts, err := NewHTTPURLParts(url)

	assert.NoError(t, err)
	assert.Contains(t, parts.Query, "hello%20world")
	assert.Contains(t, parts.Query, "%2Fpath%2Fto%2Ffile")
}

func TestHTTPURLParts_LongPath(t *testing.T) {
	url := "https://example.com/very/long/path/with/many/segments/leading/to/a/file.txt"
	parts, err := NewHTTPURLParts(url)

	assert.NoError(t, err)
	assert.Equal(t, "/very/long/path/with/many/segments/leading/to/a/file.txt", parts.Path)
}

func TestHTTPURLParts_StandardHTTPSPort(t *testing.T) {
	// Port 443 is standard for HTTPS, but might not be included in parsed output
	url := "https://example.com:443/file"
	parts, err := NewHTTPURLParts(url)

	assert.NoError(t, err)
	assert.Equal(t, "example.com", parts.Host)
	assert.Equal(t, "443", parts.Port)
}

func TestHTTPURLParts_StandardHTTPPort(t *testing.T) {
	// Port 80 is standard for HTTP, but might not be included in parsed output
	url := "http://example.com:80/file"
	parts, err := NewHTTPURLParts(url)

	assert.NoError(t, err)
	assert.Equal(t, "example.com", parts.Host)
	assert.Equal(t, "80", parts.Port)
}

func TestHTTPURLParts_PathWithDots(t *testing.T) {
	url := "https://example.com/path/./to/../file.txt"
	parts, err := NewHTTPURLParts(url)

	assert.NoError(t, err)
	// URL parser preserves the path as-is
	assert.Equal(t, "/path/./to/../file.txt", parts.Path)
}

func TestHTTPURLParts_TrailingSlash(t *testing.T) {
	url := "https://example.com/directory/"
	parts, err := NewHTTPURLParts(url)

	assert.NoError(t, err)
	assert.Equal(t, "/directory/", parts.Path)
}

func TestHTTPURLParts_QueryWithEquals(t *testing.T) {
	url := "https://example.com/api?token=abc=def=ghi"
	parts, err := NewHTTPURLParts(url)

	assert.NoError(t, err)
	assert.Equal(t, "token=abc=def=ghi", parts.Query)
}

func TestHTTPURLParts_EmptyQueryParam(t *testing.T) {
	url := "https://example.com/api?param1=&param2=value"
	parts, err := NewHTTPURLParts(url)

	assert.NoError(t, err)
	assert.Equal(t, "param1=&param2=value", parts.Query)
}

func TestHTTPURLParts_CaseSensitivity(t *testing.T) {
	// Scheme should be lowercase after parsing
	url1 := "HTTPS://EXAMPLE.COM/FILE"
	parts1, err1 := NewHTTPURLParts(url1)
	assert.NoError(t, err1)
	assert.Equal(t, "https", parts1.Scheme)
	assert.Equal(t, "EXAMPLE.COM", parts1.Host) // Host case is preserved

	url2 := "HtTpS://example.com/file"
	parts2, err2 := NewHTTPURLParts(url2)
	assert.NoError(t, err2)
	assert.Equal(t, "https", parts2.Scheme)
}

func TestHTTPURLParts_SubdomainURL(t *testing.T) {
	url := "https://api.subdomain.example.com/endpoint"
	parts, err := NewHTTPURLParts(url)

	assert.NoError(t, err)
	assert.Equal(t, "api.subdomain.example.com", parts.Host)
	assert.Equal(t, "/endpoint", parts.Path)
}

func TestHTTPURLParts_Localhost(t *testing.T) {
	url := "http://localhost:3000/api"
	parts, err := NewHTTPURLParts(url)

	assert.NoError(t, err)
	assert.Equal(t, "localhost", parts.Host)
	assert.Equal(t, "3000", parts.Port)
	assert.Equal(t, "/api", parts.Path)
}