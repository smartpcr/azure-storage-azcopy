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
	"strconv"
	"strings"
)

const (
	// MinResumableChunkSize is the minimum chunk size (4MB)
	MinResumableChunkSize = 4 * 1024 * 1024
	// MaxResumableChunkSize is the maximum chunk size (100MB)
	MaxResumableChunkSize = 100 * 1024 * 1024
	// MinResumableThreshold is the minimum file size for resumable downloads (4MB)
	MinResumableThreshold = 4 * 1024 * 1024
)

// ResumableDownloadConfig holds configuration for resumable downloads
type ResumableDownloadConfig struct {
	Enabled       bool
	Threshold     int64
	ChunkSize     int64
	SkipMD5       bool
	ProgressDir   string
}

// GetResumableDownloadConfig reads and validates resumable download configuration from environment variables
func GetResumableDownloadConfig() ResumableDownloadConfig {
	config := ResumableDownloadConfig{
		Enabled:     parseBoolEnv(EEnvironmentVariable.ResumableDownloadEnabled(), true),
		Threshold:   parseInt64Env(EEnvironmentVariable.ResumableDownloadThreshold(), 268435456), // 256MB default
		ChunkSize:   parseInt64Env(EEnvironmentVariable.ResumableDownloadChunkSize(), 67108864),  // 64MB default
		SkipMD5:     parseBoolEnv(EEnvironmentVariable.ResumableDownloadSkipMD5(), false),
		ProgressDir: GetEnvironmentVariable(EEnvironmentVariable.ChunkProgressDir()),
	}

	// Validate and apply constraints
	config.Threshold = validateThreshold(config.Threshold)
	config.ChunkSize = validateChunkSize(config.ChunkSize)

	// If no custom progress dir, use default (job plan location)
	if config.ProgressDir == "" {
		config.ProgressDir = AzcopyJobPlanFolder
	}

	return config
}

// parseBoolEnv parses a boolean environment variable with a default value
func parseBoolEnv(env EnvironmentVariable, defaultValue bool) bool {
	value := GetEnvironmentVariable(env)
	if value == "" {
		return defaultValue
	}

	// Parse various boolean representations
	valueLower := strings.ToLower(value)
	switch valueLower {
	case "true", "1", "yes", "on":
		return true
	case "false", "0", "no", "off":
		return false
	default:
		return defaultValue
	}
}

// parseInt64Env parses an int64 environment variable with a default value
func parseInt64Env(env EnvironmentVariable, defaultValue int64) int64 {
	value := GetEnvironmentVariable(env)
	if value == "" {
		return defaultValue
	}

	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return defaultValue
	}

	return parsed
}

// validateThreshold ensures the threshold is within valid bounds
func validateThreshold(threshold int64) int64 {
	if threshold < MinResumableThreshold {
		return MinResumableThreshold
	}
	// No maximum threshold - users can set any large value
	return threshold
}

// validateChunkSize ensures the chunk size is within valid bounds
func validateChunkSize(chunkSize int64) int64 {
	if chunkSize < MinResumableChunkSize {
		return MinResumableChunkSize
	}
	if chunkSize > MaxResumableChunkSize {
		return MaxResumableChunkSize
	}
	return chunkSize
}
