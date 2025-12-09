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
	"os"
	"testing"
)

// TestGetResumableDownloadConfig_Defaults tests default configuration values
func TestGetResumableDownloadConfig_Defaults(t *testing.T) {
	// Clear any environment variables
	clearResumableEnvVars()
	defer clearResumableEnvVars()

	config := GetResumableDownloadConfig()

	// Verify defaults
	if !config.Enabled {
		t.Error("Default Enabled should be true")
	}

	if config.Threshold != 268435456 { // 256MB
		t.Errorf("Default Threshold should be 268435456, got %d", config.Threshold)
	}

	if config.ChunkSize != 67108864 { // 64MB
		t.Errorf("Default ChunkSize should be 67108864, got %d", config.ChunkSize)
	}

	if config.SkipMD5 {
		t.Error("Default SkipMD5 should be false")
	}

	if config.ProgressDir != AzcopyJobPlanFolder {
		t.Errorf("Default ProgressDir should be %s, got %s", AzcopyJobPlanFolder, config.ProgressDir)
	}
}

// TestGetResumableDownloadConfig_EnabledParsing tests parsing of enabled flag
func TestGetResumableDownloadConfig_EnabledParsing(t *testing.T) {
	tests := []struct {
		name     string
		value    string
		expected bool
	}{
		{"true", "true", true},
		{"True", "True", true},
		{"TRUE", "TRUE", true},
		{"1", "1", true},
		{"yes", "yes", true},
		{"on", "on", true},
		{"false", "false", false},
		{"False", "False", false},
		{"FALSE", "FALSE", false},
		{"0", "0", false},
		{"no", "no", false},
		{"off", "off", false},
		{"invalid", "invalid", true}, // Invalid values use default (true)
		{"empty", "", true},           // Empty uses default (true)
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clearResumableEnvVars()
			defer clearResumableEnvVars()

			if tt.value != "" {
				os.Setenv("AZCOPY_RESUMABLE_DOWNLOAD", tt.value)
			}

			config := GetResumableDownloadConfig()

			if config.Enabled != tt.expected {
				t.Errorf("For value %q, expected Enabled=%v, got %v", tt.value, tt.expected, config.Enabled)
			}
		})
	}
}

// TestGetResumableDownloadConfig_ThresholdParsing tests parsing of threshold value
func TestGetResumableDownloadConfig_ThresholdParsing(t *testing.T) {
	tests := []struct {
		name     string
		value    string
		expected int64
	}{
		{"valid-512MB", "536870912", 536870912},       // 512MB
		{"valid-1GB", "1073741824", 1073741824},       // 1GB
		{"below-minimum", "1048576", MinResumableThreshold}, // 1MB -> clamped to 4MB minimum
		{"zero", "0", MinResumableThreshold},          // 0 -> clamped to minimum
		{"negative", "-1", MinResumableThreshold},     // negative -> parsed then clamped to minimum
		{"invalid", "invalid", 268435456},             // invalid -> default
		{"empty", "", 268435456},                      // empty -> default
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clearResumableEnvVars()
			defer clearResumableEnvVars()

			if tt.value != "" {
				os.Setenv("AZCOPY_RESUMABLE_THRESHOLD", tt.value)
			}

			config := GetResumableDownloadConfig()

			if config.Threshold != tt.expected {
				t.Errorf("For value %q, expected Threshold=%d, got %d", tt.value, tt.expected, config.Threshold)
			}
		})
	}
}

// TestGetResumableDownloadConfig_ChunkSizeParsing tests parsing of chunk size value
func TestGetResumableDownloadConfig_ChunkSizeParsing(t *testing.T) {
	tests := []struct {
		name     string
		value    string
		expected int64
	}{
		{"valid-32MB", "33554432", 33554432},         // 32MB
		{"valid-64MB", "67108864", 67108864},         // 64MB (default)
		{"valid-100MB", "104857600", 104857600},      // 100MB (maximum)
		{"below-minimum", "1048576", MinResumableChunkSize}, // 1MB -> clamped to 4MB
		{"above-maximum", "209715200", MaxResumableChunkSize}, // 200MB -> clamped to 100MB
		{"zero", "0", MinResumableChunkSize},         // 0 -> clamped to minimum
		{"negative", "-1", MinResumableChunkSize},    // negative -> parsed then clamped to minimum
		{"invalid", "invalid", 67108864},             // invalid -> default
		{"empty", "", 67108864},                      // empty -> default
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clearResumableEnvVars()
			defer clearResumableEnvVars()

			if tt.value != "" {
				os.Setenv("AZCOPY_RESUMABLE_CHUNK_SIZE", tt.value)
			}

			config := GetResumableDownloadConfig()

			if config.ChunkSize != tt.expected {
				t.Errorf("For value %q, expected ChunkSize=%d, got %d", tt.value, tt.expected, config.ChunkSize)
			}
		})
	}
}

// TestGetResumableDownloadConfig_SkipMD5Parsing tests parsing of skip MD5 flag
func TestGetResumableDownloadConfig_SkipMD5Parsing(t *testing.T) {
	tests := []struct {
		name     string
		value    string
		expected bool
	}{
		{"true", "true", true},
		{"false", "false", false},
		{"empty", "", false}, // Default
		{"invalid", "invalid", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clearResumableEnvVars()
			defer clearResumableEnvVars()

			if tt.value != "" {
				os.Setenv("AZCOPY_RESUME_SKIP_MD5", tt.value)
			}

			config := GetResumableDownloadConfig()

			if config.SkipMD5 != tt.expected {
				t.Errorf("For value %q, expected SkipMD5=%v, got %v", tt.value, tt.expected, config.SkipMD5)
			}
		})
	}
}

// TestGetResumableDownloadConfig_ProgressDir tests custom progress directory
func TestGetResumableDownloadConfig_ProgressDir(t *testing.T) {
	clearResumableEnvVars()
	defer clearResumableEnvVars()

	customDir := "/custom/progress/path"
	os.Setenv("AZCOPY_CHUNK_PROGRESS_DIR", customDir)

	config := GetResumableDownloadConfig()

	if config.ProgressDir != customDir {
		t.Errorf("Expected ProgressDir=%s, got %s", customDir, config.ProgressDir)
	}
}

// TestGetResumableDownloadConfig_AllEnvVars tests all environment variables together
func TestGetResumableDownloadConfig_AllEnvVars(t *testing.T) {
	clearResumableEnvVars()
	defer clearResumableEnvVars()

	// Set all environment variables
	os.Setenv("AZCOPY_RESUMABLE_DOWNLOAD", "false")
	os.Setenv("AZCOPY_RESUMABLE_THRESHOLD", "536870912") // 512MB
	os.Setenv("AZCOPY_RESUMABLE_CHUNK_SIZE", "33554432") // 32MB
	os.Setenv("AZCOPY_RESUME_SKIP_MD5", "true")
	os.Setenv("AZCOPY_CHUNK_PROGRESS_DIR", "/custom/path")

	config := GetResumableDownloadConfig()

	if config.Enabled {
		t.Error("Expected Enabled=false")
	}
	if config.Threshold != 536870912 {
		t.Errorf("Expected Threshold=536870912, got %d", config.Threshold)
	}
	if config.ChunkSize != 33554432 {
		t.Errorf("Expected ChunkSize=33554432, got %d", config.ChunkSize)
	}
	if !config.SkipMD5 {
		t.Error("Expected SkipMD5=true")
	}
	if config.ProgressDir != "/custom/path" {
		t.Errorf("Expected ProgressDir=/custom/path, got %s", config.ProgressDir)
	}
}

// clearResumableEnvVars clears all resumable download environment variables
func clearResumableEnvVars() {
	os.Unsetenv("AZCOPY_RESUMABLE_DOWNLOAD")
	os.Unsetenv("AZCOPY_RESUMABLE_THRESHOLD")
	os.Unsetenv("AZCOPY_RESUMABLE_CHUNK_SIZE")
	os.Unsetenv("AZCOPY_RESUME_SKIP_MD5")
	os.Unsetenv("AZCOPY_CHUNK_PROGRESS_DIR")
}
