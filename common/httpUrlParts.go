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
	"fmt"
	"net/url"
)

// HTTPURLParts represents a parsed HTTP URL
type HTTPURLParts struct {
	Scheme   string // "http" or "https"
	Host     string // "api.example.com"
	Port     string // "8080" or ""
	Path     string // "/files/data.bin"
	Query    string // "version=2&format=json"
	Fragment string // "section1"
	URL      string // Full URL
}

// NewHTTPURLParts parses an HTTP URL
func NewHTTPURLParts(rawURL string) (HTTPURLParts, error) {
	if rawURL == "" {
		return HTTPURLParts{}, fmt.Errorf("URL cannot be empty")
	}

	parsed, err := url.Parse(rawURL)
	if err != nil {
		return HTTPURLParts{}, fmt.Errorf("invalid HTTP URL: %w", err)
	}

	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return HTTPURLParts{}, fmt.Errorf("expected http or https, got: %s", parsed.Scheme)
	}

	return HTTPURLParts{
		Scheme:   parsed.Scheme,
		Host:     parsed.Hostname(),
		Port:     parsed.Port(),
		Path:     parsed.Path,
		Query:    parsed.RawQuery,
		Fragment: parsed.Fragment,
		URL:      rawURL,
	}, nil
}

// String reconstructs the URL
func (h HTTPURLParts) String() string {
	return h.URL
}

// IsSecure returns true if HTTPS
func (h HTTPURLParts) IsSecure() bool {
	return h.Scheme == "https"
}