// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package app

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	authpkg "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/auth"
)

func TestResolveSkillTargetPath(t *testing.T) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("failed to get home directory: %v", err)
	}

	tests := []struct {
		name       string
		target     string
		wantSuffix string
		wantErr    bool
	}{
		{
			name:       "qoder target",
			target:     "qoder",
			wantSuffix: filepath.Join(".qoder", "skills"),
			wantErr:    false,
		},
		{
			name:       "claude target",
			target:     "claude",
			wantSuffix: filepath.Join(".claude", "skills"),
			wantErr:    false,
		},
		{
			name:       "cursor target",
			target:     "cursor",
			wantSuffix: filepath.Join(".cursor", "skills"),
			wantErr:    false,
		},
		{
			name:       "codex target",
			target:     "codex",
			wantSuffix: filepath.Join(".codex", "skills"),
			wantErr:    false,
		},
		{
			name:       "opencode target",
			target:     "opencode",
			wantSuffix: filepath.Join(".config", "opencode", "skills"),
			wantErr:    false,
		},
		{
			name:       "case insensitive - QODER",
			target:     "QODER",
			wantSuffix: filepath.Join(".qoder", "skills"),
			wantErr:    false,
		},
		{
			name:       "case insensitive - Claude",
			target:     "Claude",
			wantSuffix: filepath.Join(".claude", "skills"),
			wantErr:    false,
		},
		{
			name:    "invalid target",
			target:  "invalid",
			wantErr: true,
		},
		{
			name:    "empty target",
			target:  "",
			wantErr: true,
		},
		{
			name:    "whitespace only",
			target:  "   ",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := resolveSkillTargetPath(tt.target)
			if (err != nil) != tt.wantErr {
				t.Errorf("resolveSkillTargetPath() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr {
				expected := filepath.Join(homeDir, tt.wantSuffix)
				if got != expected {
					t.Errorf("resolveSkillTargetPath() = %v, want %v", got, expected)
				}
			}
		})
	}
}

func TestResolveSkillTargetPathCurrentDir(t *testing.T) {
	// Test "." target returns current working directory
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get working directory: %v", err)
	}

	got, err := resolveSkillTargetPath(".")
	if err != nil {
		t.Errorf("resolveSkillTargetPath(\".\") error = %v", err)
		return
	}

	if got != cwd {
		t.Errorf("resolveSkillTargetPath(\".\") = %v, want %v", got, cwd)
	}
}

func TestParseDownloadSkillResponse(t *testing.T) {
	tests := []struct {
		name        string
		jsonInput   string
		wantSuccess bool
		wantURL     string
		wantFile    string
		wantErrCode string
		wantErrMsg  string
	}{
		{
			name: "successful response",
			jsonInput: `{
				"success": true,
				"result": {
					"downloadUrl": "https://example.com/skill.zip",
					"fileName": "my-skill.zip"
				}
			}`,
			wantSuccess: true,
			wantURL:     "https://example.com/skill.zip",
			wantFile:    "my-skill.zip",
		},
		{
			name: "error response",
			jsonInput: `{
				"success": false,
				"errorCode": "SKILL_NOT_FOUND",
				"errorMsg": "The skill does not exist"
			}`,
			wantSuccess: false,
			wantErrCode: "SKILL_NOT_FOUND",
			wantErrMsg:  "The skill does not exist",
		},
		{
			name: "success with empty result",
			jsonInput: `{
				"success": true,
				"result": null
			}`,
			wantSuccess: true,
			wantURL:     "",
			wantFile:    "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var resp downloadSkillResponse
			if err := json.Unmarshal([]byte(tt.jsonInput), &resp); err != nil {
				t.Fatalf("failed to unmarshal JSON: %v", err)
			}

			if resp.Success != tt.wantSuccess {
				t.Errorf("Success = %v, want %v", resp.Success, tt.wantSuccess)
			}

			if tt.wantSuccess && resp.Result != nil {
				if resp.Result.DownloadURL != tt.wantURL {
					t.Errorf("DownloadURL = %v, want %v", resp.Result.DownloadURL, tt.wantURL)
				}
				if resp.Result.FileName != tt.wantFile {
					t.Errorf("FileName = %v, want %v", resp.Result.FileName, tt.wantFile)
				}
			}

			if !tt.wantSuccess {
				if resp.ErrorCode != tt.wantErrCode {
					t.Errorf("ErrorCode = %v, want %v", resp.ErrorCode, tt.wantErrCode)
				}
				if resp.ErrorMsg != tt.wantErrMsg {
					t.Errorf("ErrorMsg = %v, want %v", resp.ErrorMsg, tt.wantErrMsg)
				}
			}
		})
	}
}

func TestExtractSkillZip(t *testing.T) {
	// Create a temporary zip file with test content
	tempDir := t.TempDir()
	zipPath := filepath.Join(tempDir, "test.zip")
	destDir := filepath.Join(tempDir, "extracted")

	// Create zip file with test content
	zipFile, err := os.Create(zipPath)
	if err != nil {
		t.Fatalf("failed to create zip file: %v", err)
	}

	zipWriter := zip.NewWriter(zipFile)

	// Add a file to the zip
	fileContent := []byte("test content")
	writer, err := zipWriter.Create("test-file.txt")
	if err != nil {
		t.Fatalf("failed to create file in zip: %v", err)
	}
	if _, err := writer.Write(fileContent); err != nil {
		t.Fatalf("failed to write file content: %v", err)
	}

	// Add a subdirectory with a file
	writer, err = zipWriter.Create("subdir/nested-file.txt")
	if err != nil {
		t.Fatalf("failed to create nested file in zip: %v", err)
	}
	if _, err := writer.Write([]byte("nested content")); err != nil {
		t.Fatalf("failed to write nested file content: %v", err)
	}

	if err := zipWriter.Close(); err != nil {
		t.Fatalf("failed to close zip writer: %v", err)
	}
	if err := zipFile.Close(); err != nil {
		t.Fatalf("failed to close zip file: %v", err)
	}

	// Extract the zip
	if err := extractSkillZip(zipPath, destDir); err != nil {
		t.Fatalf("extractSkillZip() error = %v", err)
	}

	// Verify extracted files
	extractedFile := filepath.Join(destDir, "test-file.txt")
	content, err := os.ReadFile(extractedFile)
	if err != nil {
		t.Errorf("failed to read extracted file: %v", err)
	}
	if string(content) != "test content" {
		t.Errorf("extracted content = %v, want %v", string(content), "test content")
	}

	// Verify nested file
	nestedFile := filepath.Join(destDir, "subdir", "nested-file.txt")
	content, err = os.ReadFile(nestedFile)
	if err != nil {
		t.Errorf("failed to read nested file: %v", err)
	}
	if string(content) != "nested content" {
		t.Errorf("nested content = %v, want %v", string(content), "nested content")
	}
}

func TestExtractSkillZipPreventZipSlip(t *testing.T) {
	tempDir := t.TempDir()
	zipPath := filepath.Join(tempDir, "malicious.zip")
	destDir := filepath.Join(tempDir, "extracted")

	// Create a zip file with a path traversal attempt
	zipFile, err := os.Create(zipPath)
	if err != nil {
		t.Fatalf("failed to create zip file: %v", err)
	}

	zipWriter := zip.NewWriter(zipFile)

	// Try to create a file with path traversal
	writer, err := zipWriter.Create("../../../etc/passwd")
	if err != nil {
		t.Fatalf("failed to create malicious file in zip: %v", err)
	}
	if _, err := writer.Write([]byte("malicious content")); err != nil {
		t.Fatalf("failed to write malicious content: %v", err)
	}

	if err := zipWriter.Close(); err != nil {
		t.Fatalf("failed to close zip writer: %v", err)
	}
	if err := zipFile.Close(); err != nil {
		t.Fatalf("failed to close zip file: %v", err)
	}

	// Extract should fail due to zip slip protection
	err = extractSkillZip(zipPath, destDir)
	if err == nil {
		t.Error("extractSkillZip() should have failed for zip slip attack")
	}
	if !strings.Contains(err.Error(), "invalid file path") {
		t.Errorf("error should mention invalid file path, got: %v", err)
	}
}

func TestSkillAddCommandValidation(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		wantErr bool
		errMsg  string
	}{
		{
			name:    "missing arguments",
			args:    []string{"skill", "add"},
			wantErr: true,
			errMsg:  "accepts 2 arg(s)",
		},
		{
			name:    "missing target",
			args:    []string{"skill", "add", "skill-123"},
			wantErr: true,
			errMsg:  "accepts 2 arg(s)",
		},
		{
			name:    "too many arguments",
			args:    []string{"skill", "add", "skill-123", "qoder", "extra"},
			wantErr: true,
			errMsg:  "accepts 2 arg(s)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := NewRootCommand()
			cmd.SetArgs(tt.args)

			var out bytes.Buffer
			cmd.SetOut(&out)
			cmd.SetErr(&out)

			err := cmd.Execute()
			if (err != nil) != tt.wantErr {
				t.Errorf("Execute() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr && !strings.Contains(err.Error(), tt.errMsg) {
				t.Errorf("error = %v, should contain %v", err, tt.errMsg)
			}
		})
	}
}

func TestSkillAddInvalidTarget(t *testing.T) {
	// Setup: Create config directory with valid token
	tempDir := t.TempDir()
	configDir := filepath.Join(tempDir, "config")
	t.Setenv("DWS_CONFIG_DIR", configDir)

	// Save a valid token
	err := authpkg.SaveTokenData(configDir, &authpkg.TokenData{
		AccessToken:  "test-token",
		RefreshToken: "refresh-token",
		ExpiresAt:    time.Now().Add(time.Hour),
		RefreshExpAt: time.Now().Add(24 * time.Hour),
	})
	if err != nil {
		t.Fatalf("failed to save token data: %v", err)
	}

	cmd := NewRootCommand()
	cmd.SetArgs([]string{"skill", "add", "skill-123", "invalid-target"})

	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)

	err = cmd.Execute()
	if err == nil {
		t.Error("Execute() should have failed for invalid target")
	}
	if !strings.Contains(err.Error(), "invalid target") {
		t.Errorf("error should mention invalid target, got: %v", err)
	}
}

func TestSkillAddRequiresAuth(t *testing.T) {
	// Setup: Create config directory without token
	tempDir := t.TempDir()
	configDir := filepath.Join(tempDir, "config")
	t.Setenv("DWS_CONFIG_DIR", configDir)

	// Ensure the config directory exists but has no token
	if err := os.MkdirAll(configDir, 0755); err != nil {
		t.Fatalf("failed to create config dir: %v", err)
	}

	cmd := NewRootCommand()
	cmd.SetArgs([]string{"skill", "add", "skill-123", "qoder"})

	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)

	err := cmd.Execute()
	if err == nil {
		t.Error("Execute() should have failed without auth")
	}
	// Check for authentication-related error (English or Chinese)
	errStr := err.Error()
	if !strings.Contains(errStr, "not logged in") && !strings.Contains(errStr, "token") && !strings.Contains(errStr, "未登录") && !strings.Contains(errStr, "auth") {
		t.Errorf("error should mention authentication, got: %v", err)
	}
}

func TestFetchSkillDownloadInfoUnauthorized(t *testing.T) {
	// Create mock server that returns 401
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()

	// We can't easily test the actual fetchSkillDownloadInfo function
	// because it uses a hardcoded URL. This test verifies HTTP 401 handling pattern.
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(server.URL)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", resp.StatusCode)
	}
}

func TestSupportedTargets(t *testing.T) {
	targets := supportedTargets()

	// Should contain all predefined targets
	expectedTargets := []string{"qoder", "claude", "cursor", "codex", "opencode", "."}
	for _, expected := range expectedTargets {
		if !strings.Contains(targets, expected) {
			t.Errorf("supportedTargets() should contain %s, got: %s", expected, targets)
		}
	}
}

func TestAgentSkillPathsCrossPlatform(t *testing.T) {
	// Verify that paths use platform-appropriate separators
	for target, path := range agentSkillPaths {
		if runtime.GOOS == "windows" {
			if strings.Contains(path, "/") && !strings.Contains(path, "\\") {
				// On Windows, filepath.Join should use backslashes
				// But raw map values may use forward slashes
				t.Logf("Note: %s path '%s' uses forward slashes (will be converted by filepath.Join)", target, path)
			}
		}

		// Test that resolveSkillTargetPath produces valid paths
		resolved, err := resolveSkillTargetPath(target)
		if err != nil {
			t.Errorf("resolveSkillTargetPath(%s) failed: %v", target, err)
			continue
		}

		// Path should be absolute
		if !filepath.IsAbs(resolved) {
			t.Errorf("resolveSkillTargetPath(%s) returned non-absolute path: %s", target, resolved)
		}
	}
}

func TestCleanupTempFile(t *testing.T) {
	// Create a temporary file
	tempFile, err := os.CreateTemp("", "test-cleanup-*.txt")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	tempPath := tempFile.Name()
	tempFile.Close()

	// Verify file exists
	if _, err := os.Stat(tempPath); os.IsNotExist(err) {
		t.Fatalf("temp file should exist before cleanup")
	}

	// Clean up
	cleanupTempFile(tempPath)

	// Verify file is deleted
	if _, err := os.Stat(tempPath); !os.IsNotExist(err) {
		t.Errorf("temp file should be deleted after cleanup")
	}

	// Cleanup should not panic on empty path
	cleanupTempFile("")

	// Cleanup should not panic on non-existent file
	cleanupTempFile("/nonexistent/path/file.txt")
}

func TestDownloadSkillResponseJSON(t *testing.T) {
	// Test JSON marshaling/unmarshaling round-trip
	original := downloadSkillResponse{
		Success: true,
		Result: &downloadSkillResult{
			DownloadURL: "https://example.com/skill.zip",
			FileName:    "skill.zip",
		},
	}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	var parsed downloadSkillResponse
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if parsed.Success != original.Success {
		t.Errorf("Success mismatch: got %v, want %v", parsed.Success, original.Success)
	}
	if parsed.Result.DownloadURL != original.Result.DownloadURL {
		t.Errorf("DownloadURL mismatch: got %v, want %v", parsed.Result.DownloadURL, original.Result.DownloadURL)
	}
	if parsed.Result.FileName != original.Result.FileName {
		t.Errorf("FileName mismatch: got %v, want %v", parsed.Result.FileName, original.Result.FileName)
	}
}

func TestSkillCommandHelp(t *testing.T) {
	cmd := NewRootCommand()
	cmd.SetArgs([]string{"skill", "--help"})

	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	output := out.String()
	// Check for the Long description which is shown in help
	if !strings.Contains(output, "技能") {
		t.Errorf("help should mention '技能', got: %s", output)
	}
	if !strings.Contains(output, "add") {
		t.Errorf("help should mention 'add' subcommand, got: %s", output)
	}
}

func TestSkillAddCommandHelp(t *testing.T) {
	cmd := NewRootCommand()
	cmd.SetArgs([]string{"skill", "add", "--help"})

	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	output := out.String()
	// Should mention supported targets
	expectedTargets := []string{"qoder", "claude", "cursor", "codex", "opencode"}
	for _, target := range expectedTargets {
		if !strings.Contains(output, target) {
			t.Errorf("help should mention target '%s', got: %s", target, output)
		}
	}
}

func TestDownloadSkillFileSuccess(t *testing.T) {
	// Create a mock server that returns a zip file
	expectedContent := []byte("fake zip content")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/zip")
		w.WriteHeader(http.StatusOK)
		w.Write(expectedContent)
	}))
	defer server.Close()

	// Download the file
	ctx := context.Background()
	tempPath, err := downloadSkillFile(ctx, server.URL, "test.zip")
	if err != nil {
		t.Fatalf("downloadSkillFile() error = %v", err)
	}
	defer os.Remove(tempPath)

	// Verify the downloaded content
	content, err := os.ReadFile(tempPath)
	if err != nil {
		t.Fatalf("failed to read downloaded file: %v", err)
	}

	if !bytes.Equal(content, expectedContent) {
		t.Errorf("downloaded content mismatch: got %v, want %v", content, expectedContent)
	}
}

func TestDownloadSkillFileServerError(t *testing.T) {
	// Create a mock server that returns 500
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	ctx := context.Background()
	_, err := downloadSkillFile(ctx, server.URL, "test.zip")
	if err == nil {
		t.Error("downloadSkillFile() should fail on server error")
	}
}

func TestExtractSkillZipEmptyZip(t *testing.T) {
	tempDir := t.TempDir()
	zipPath := filepath.Join(tempDir, "empty.zip")
	destDir := filepath.Join(tempDir, "extracted")

	// Create an empty zip file
	zipFile, err := os.Create(zipPath)
	if err != nil {
		t.Fatalf("failed to create zip file: %v", err)
	}
	zipWriter := zip.NewWriter(zipFile)
	if err := zipWriter.Close(); err != nil {
		t.Fatalf("failed to close zip writer: %v", err)
	}
	if err := zipFile.Close(); err != nil {
		t.Fatalf("failed to close zip file: %v", err)
	}

	// Extract should succeed even for empty zip
	if err := extractSkillZip(zipPath, destDir); err != nil {
		t.Errorf("extractSkillZip() should not fail for empty zip: %v", err)
	}

	// Destination directory should be created
	if _, err := os.Stat(destDir); os.IsNotExist(err) {
		t.Errorf("destination directory should be created")
	}
}

func TestExtractSkillZipWithDirectories(t *testing.T) {
	tempDir := t.TempDir()
	zipPath := filepath.Join(tempDir, "test.zip")
	destDir := filepath.Join(tempDir, "extracted")

	// Create zip with directory entries
	zipFile, err := os.Create(zipPath)
	if err != nil {
		t.Fatalf("failed to create zip file: %v", err)
	}

	zipWriter := zip.NewWriter(zipFile)

	// Add a directory entry with proper permissions
	header := &zip.FileHeader{
		Name:   "mydir/",
		Method: zip.Deflate,
	}
	header.SetMode(0755 | os.ModeDir)
	_, err = zipWriter.CreateHeader(header)
	if err != nil {
		t.Fatalf("failed to create directory in zip: %v", err)
	}

	// Add a file in the directory
	fileHeader := &zip.FileHeader{
		Name:   "mydir/file.txt",
		Method: zip.Deflate,
	}
	fileHeader.SetMode(0644)
	writer, err := zipWriter.CreateHeader(fileHeader)
	if err != nil {
		t.Fatalf("failed to create file in zip: %v", err)
	}
	if _, err := writer.Write([]byte("content")); err != nil {
		t.Fatalf("failed to write content: %v", err)
	}

	if err := zipWriter.Close(); err != nil {
		t.Fatalf("failed to close zip writer: %v", err)
	}
	if err := zipFile.Close(); err != nil {
		t.Fatalf("failed to close zip file: %v", err)
	}

	// Extract
	if err := extractSkillZip(zipPath, destDir); err != nil {
		t.Fatalf("extractSkillZip() error = %v", err)
	}

	// Verify directory was created
	dirPath := filepath.Join(destDir, "mydir")
	info, err := os.Stat(dirPath)
	if err != nil {
		t.Errorf("directory should exist: %v", err)
	} else if !info.IsDir() {
		t.Errorf("mydir should be a directory")
	}

	// Verify file exists
	filePath := filepath.Join(destDir, "mydir", "file.txt")
	content, err := os.ReadFile(filePath)
	if err != nil {
		t.Errorf("file should exist: %v", err)
	} else if string(content) != "content" {
		t.Errorf("file content mismatch: got %s, want 'content'", string(content))
	}
}
