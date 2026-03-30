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

package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeTestFile is a test helper that writes data to a file and fails the
// test immediately if the write fails, preventing confusing downstream errors.
func writeTestFile(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("failed to write test file %s: %v", path, err)
	}
}

// ---------------------------------------------------------------------------
// ReadFileArg
// ---------------------------------------------------------------------------

func TestReadFileArgPlainValue(t *testing.T) {
	t.Parallel()
	val, isFile, err := ReadFileArg("hello world")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if isFile {
		t.Error("plain value should not be detected as file")
	}
	if val != "hello world" {
		t.Errorf("got %q, want %q", val, "hello world")
	}
}

func TestReadFileArgReadsFile(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "input.txt")
	writeTestFile(t, path, []byte(`{"title":"test"}`))

	val, isFile, err := ReadFileArg("@" + path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !isFile {
		t.Error("@file value should be detected as file")
	}
	if val != `{"title":"test"}` {
		t.Errorf("got %q, want %q", val, `{"title":"test"}`)
	}
}

func TestReadFileArgEmptyFilename(t *testing.T) {
	t.Parallel()
	_, _, err := ReadFileArg("@")
	if err == nil {
		t.Fatal("expected error for empty filename")
	}
	if !strings.Contains(err.Error(), "must not be empty") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestReadFileArgMissingFile(t *testing.T) {
	t.Parallel()
	_, _, err := ReadFileArg("@/nonexistent/path/file.txt")
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestReadFileArgSizeLimit(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "huge.txt")
	// Create a file slightly over maxStdinSize (write 10MB + 1 byte)
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}
	data := strings.Repeat("x", maxStdinSize+1)
	if _, err := f.WriteString(data); err != nil {
		f.Close()
		t.Fatalf("failed to write test data: %v", err)
	}
	f.Close()

	_, _, err = ReadFileArg("@" + path)
	if err == nil {
		t.Fatal("expected error for oversized file")
	}
	if !strings.Contains(err.Error(), "10 MB") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestReadFileArgAtDashPassThrough(t *testing.T) {
	t.Parallel()
	// @- means stdin — ReadFileArg should NOT handle it, just pass through.
	val, isFile, err := ReadFileArg("@-")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if isFile {
		t.Error("@- should not be treated as a file by ReadFileArg")
	}
	if val != "@-" {
		t.Errorf("got %q, want %q", val, "@-")
	}
}

func TestReadStdinIfPipedReturnsEmptyForTerminal(t *testing.T) {
	// This test runs in a terminal context (go test), so stdin is a terminal.
	// ReadStdinIfPiped should return empty string.
	val, err := ReadStdinIfPiped()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if val != "" {
		t.Errorf("expected empty string for terminal stdin, got %q", val)
	}
}

func TestStdinIsPipeReturnsFalseForTerminal(t *testing.T) {
	// go test runs with stdin as a terminal, so StdinIsPipe should return false.
	if StdinIsPipe() {
		t.Error("expected StdinIsPipe() == false in terminal context")
	}
}

// ---------------------------------------------------------------------------
// readFileBounded (via ReadFileArg)
// ---------------------------------------------------------------------------

func TestReadFileBoundedEmptyFile(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "empty.txt")
	writeTestFile(t, path, []byte(""))

	val, isFile, err := ReadFileArg("@" + path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !isFile {
		t.Error("should be detected as file")
	}
	if val != "" {
		t.Errorf("expected empty content, got %q", val)
	}
}

func TestReadFileBoundedPermissionDenied(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "noperm.txt")
	writeTestFile(t, path, []byte("secret"))
	os.Chmod(path, 0o000)
	t.Cleanup(func() { os.Chmod(path, 0o644) })

	_, _, err := ReadFileArg("@" + path)
	if err == nil {
		t.Fatal("expected error for permission denied")
	}
}

// ---------------------------------------------------------------------------
// StdinGuard
// ---------------------------------------------------------------------------

func TestStdinGuardFirstClaimSucceeds(t *testing.T) {
	t.Parallel()
	guard := NewStdinGuard()
	if err := guard.Claim("--text @-"); err != nil {
		t.Fatalf("first claim should succeed: %v", err)
	}
	if !guard.Claimed() {
		t.Error("guard should report claimed after successful claim")
	}
}

func TestStdinGuardSecondClaimFails(t *testing.T) {
	t.Parallel()
	guard := NewStdinGuard()
	_ = guard.Claim("--text @-")

	err := guard.Claim("--body @-")
	if err == nil {
		t.Fatal("second claim should fail")
	}
	if !strings.Contains(err.Error(), "--text @-") {
		t.Errorf("error should mention first claimer, got: %v", err)
	}
	if !strings.Contains(err.Error(), "--body @-") {
		t.Errorf("error should mention second claimer, got: %v", err)
	}
}

func TestStdinGuardNotClaimedInitially(t *testing.T) {
	t.Parallel()
	guard := NewStdinGuard()
	if guard.Claimed() {
		t.Error("fresh guard should not be claimed")
	}
}

// ---------------------------------------------------------------------------
// ResolveInputSource
// ---------------------------------------------------------------------------

func TestResolveInputSourcePlainValue(t *testing.T) {
	t.Parallel()
	guard := NewStdinGuard()
	val, err := ResolveInputSource("hello", "text", guard)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if val != "hello" {
		t.Errorf("got %q, want %q", val, "hello")
	}
	if guard.Claimed() {
		t.Error("plain value should not claim stdin")
	}
}

func TestResolveInputSourceEmptyValue(t *testing.T) {
	t.Parallel()
	guard := NewStdinGuard()
	val, err := ResolveInputSource("", "text", guard)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if val != "" {
		t.Errorf("got %q, want empty", val)
	}
}

func TestResolveInputSourceAtFileReadsFile(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "msg.txt")
	writeTestFile(t, path, []byte("file content here"))

	guard := NewStdinGuard()
	val, err := ResolveInputSource("@"+path, "text", guard)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if val != "file content here" {
		t.Errorf("got %q, want %q", val, "file content here")
	}
	// @file should NOT claim stdin.
	if guard.Claimed() {
		t.Error("@file should not claim stdin")
	}
}

func TestResolveInputSourceAtFileEmptyName(t *testing.T) {
	t.Parallel()
	guard := NewStdinGuard()
	_, err := ResolveInputSource("@", "json", guard)
	if err == nil {
		t.Fatal("expected error for empty @file name")
	}
	if !strings.Contains(err.Error(), "--json") {
		t.Errorf("error should mention flag name, got: %v", err)
	}
}

func TestResolveInputSourceAtFileMissing(t *testing.T) {
	t.Parallel()
	guard := NewStdinGuard()
	_, err := ResolveInputSource("@/no/such/file.txt", "data", guard)
	if err == nil {
		t.Fatal("expected error for missing file")
	}
	if !strings.Contains(err.Error(), "--data") {
		t.Errorf("error should mention flag name, got: %v", err)
	}
}

func TestResolveInputSourceAtFileSizeLimit(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "huge.bin")
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}
	if _, err := f.WriteString(strings.Repeat("x", maxStdinSize+1)); err != nil {
		f.Close()
		t.Fatalf("failed to write test data: %v", err)
	}
	f.Close()

	guard := NewStdinGuard()
	_, resolveErr := ResolveInputSource("@"+path, "body", guard)
	if resolveErr == nil {
		t.Fatal("expected error for oversized file")
	}
	if !strings.Contains(resolveErr.Error(), "10 MB") {
		t.Errorf("error should mention size limit, got: %v", resolveErr)
	}
}

func TestResolveInputSourceAtDashNilGuardFails(t *testing.T) {
	t.Parallel()
	_, err := ResolveInputSource("@-", "text", nil)
	if err == nil {
		t.Fatal("expected error when guard is nil")
	}
	if !strings.Contains(err.Error(), "not available") {
		t.Errorf("error should mention stdin unavailability, got: %v", err)
	}
}

func TestResolveInputSourceAtDashDoubleClaimFails(t *testing.T) {
	t.Parallel()
	guard := NewStdinGuard()
	// Simulate first claim from another source.
	_ = guard.Claim("--json @-")

	_, err := ResolveInputSource("@-", "text", guard)
	if err == nil {
		t.Fatal("expected error for double stdin claim")
	}
	if !strings.Contains(err.Error(), "already consumed") {
		t.Errorf("error should mention stdin conflict, got: %v", err)
	}
}

func TestResolveInputSourceAtFileMultiline(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "multiline.md")
	content := "# Title\n\nLine 1\nLine 2\nLine 3\n"
	writeTestFile(t, path, []byte(content))

	guard := NewStdinGuard()
	val, err := ResolveInputSource("@"+path, "text", guard)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if val != content {
		t.Errorf("multiline content mismatch:\ngot:  %q\nwant: %q", val, content)
	}
}

func TestResolveInputSourceAtFileUTF8(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "chinese.txt")
	content := "你好世界 🌍"
	writeTestFile(t, path, []byte(content))

	guard := NewStdinGuard()
	val, err := ResolveInputSource("@"+path, "text", guard)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if val != content {
		t.Errorf("UTF-8 content mismatch:\ngot:  %q\nwant: %q", val, content)
	}
}

func TestResolveInputSourceAtFileJSON(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "payload.json")
	content := `{"title":"meeting","startTime":"2026-03-29T10:00:00Z"}`
	writeTestFile(t, path, []byte(content))

	guard := NewStdinGuard()
	val, err := ResolveInputSource("@"+path, "json", guard)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if val != content {
		t.Errorf("JSON content mismatch:\ngot:  %q\nwant: %q", val, content)
	}
}

func TestResolveInputSourceValueStartingWithAtSign(t *testing.T) {
	t.Parallel()
	// A value like "@mention" that looks like @file but the file doesn't exist
	// should return an error (user likely intended file input).
	guard := NewStdinGuard()
	_, err := ResolveInputSource("@mention_someone", "text", guard)
	if err == nil {
		t.Fatal("expected error for non-existent @file path")
	}
}

// ---------------------------------------------------------------------------
// ResolveInputSource: table-driven edge cases
// ---------------------------------------------------------------------------

func TestResolveInputSourceTable(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	existingFile := filepath.Join(dir, "exists.txt")
	writeTestFile(t, existingFile, []byte("file-data"))

	tests := []struct {
		name      string
		value     string
		flagName  string
		guard     *StdinGuard
		wantVal   string
		wantErr   string // substring to match in error, "" means no error
		wantClaim bool   // expect guard to be claimed after call
	}{
		{
			name:     "plain string unchanged",
			value:    "hello",
			flagName: "text",
			guard:    NewStdinGuard(),
			wantVal:  "hello",
		},
		{
			name:     "empty string unchanged",
			value:    "",
			flagName: "text",
			guard:    NewStdinGuard(),
			wantVal:  "",
		},
		{
			name:     "plain string with special chars",
			value:    "hello@world.com",
			flagName: "email",
			guard:    NewStdinGuard(),
			wantVal:  "hello@world.com",
		},
		{
			name:     "@file reads content",
			value:    "@" + existingFile,
			flagName: "data",
			guard:    NewStdinGuard(),
			wantVal:  "file-data",
		},
		{
			name:      "@file does not claim stdin",
			value:     "@" + existingFile,
			flagName:  "data",
			guard:     NewStdinGuard(),
			wantVal:   "file-data",
			wantClaim: false,
		},
		{
			name:     "bare @ is error",
			value:    "@",
			flagName: "body",
			guard:    NewStdinGuard(),
			wantErr:  "must not be empty",
		},
		{
			name:     "missing file is error",
			value:    "@/tmp/does-not-exist-" + t.Name(),
			flagName: "file",
			guard:    NewStdinGuard(),
			wantErr:  "--file",
		},
		{
			name:     "@- with nil guard is error",
			value:    "@-",
			flagName: "text",
			guard:    nil,
			wantErr:  "not available",
		},
		{
			name:     "@- with pre-claimed guard is error",
			value:    "@-",
			flagName: "body",
			guard: func() *StdinGuard {
				g := NewStdinGuard()
				_ = g.Claim("--json @-")
				return g
			}(),
			wantErr: "already consumed",
		},
		{
			name:     "error message includes flag name",
			value:    "@/nonexistent",
			flagName: "my-flag",
			guard:    NewStdinGuard(),
			wantErr:  "--my-flag",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			val, err := ResolveInputSource(tt.value, tt.flagName, tt.guard)
			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tt.wantErr)
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Errorf("error = %q, want substring %q", err.Error(), tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if val != tt.wantVal {
				t.Errorf("value = %q, want %q", val, tt.wantVal)
			}
			if tt.guard != nil && tt.wantClaim != tt.guard.Claimed() {
				t.Errorf("guard.Claimed() = %v, want %v", tt.guard.Claimed(), tt.wantClaim)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// ReadFileArg: table-driven edge cases
// ---------------------------------------------------------------------------

func TestReadFileArgTable(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	existingFile := filepath.Join(dir, "data.txt")
	writeTestFile(t, existingFile, []byte("content"))
	emptyFile := filepath.Join(dir, "empty.txt")
	writeTestFile(t, emptyFile, []byte(""))

	tests := []struct {
		name       string
		value      string
		wantVal    string
		wantIsFile bool
		wantErr    string
	}{
		{"plain string", "hello", "hello", false, ""},
		{"empty string", "", "", false, ""},
		{"email-like value", "user@domain.com", "user@domain.com", false, ""},
		{"@file reads content", "@" + existingFile, "content", true, ""},
		{"@file empty content", "@" + emptyFile, "", true, ""},
		{"@- passes through", "@-", "@-", false, ""},
		{"bare @ is error", "@", "", false, "must not be empty"},
		{"missing file is error", "@/nonexistent", "", false, "@file"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			val, isFile, err := ReadFileArg(tt.value)
			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tt.wantErr)
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Errorf("error = %q, want substring %q", err.Error(), tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if val != tt.wantVal {
				t.Errorf("value = %q, want %q", val, tt.wantVal)
			}
			if isFile != tt.wantIsFile {
				t.Errorf("isFile = %v, want %v", isFile, tt.wantIsFile)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// StdinGuard: table-driven claim sequences
// ---------------------------------------------------------------------------

func TestStdinGuardClaimSequences(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		claims     []string // sources to claim in order
		wantFails  int      // how many claims should fail
		wantErrSub string   // substring expected in first failure
	}{
		{
			name:      "single claim succeeds",
			claims:    []string{"--json @-"},
			wantFails: 0,
		},
		{
			name:       "second claim fails",
			claims:     []string{"--json @-", "--text @-"},
			wantFails:  1,
			wantErrSub: "already consumed",
		},
		{
			name:       "third claim also fails",
			claims:     []string{"--json @-", "--text @-", "--body @-"},
			wantFails:  2,
			wantErrSub: "already consumed",
		},
		{
			name:       "error names both sources",
			claims:     []string{"implicit stdin (pipe)", "--text @-"},
			wantFails:  1,
			wantErrSub: "implicit stdin (pipe)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			guard := NewStdinGuard()
			fails := 0
			var firstErr error
			for _, source := range tt.claims {
				if err := guard.Claim(source); err != nil {
					fails++
					if firstErr == nil {
						firstErr = err
					}
				}
			}
			if fails != tt.wantFails {
				t.Errorf("failures = %d, want %d", fails, tt.wantFails)
			}
			if tt.wantErrSub != "" && firstErr != nil {
				if !strings.Contains(firstErr.Error(), tt.wantErrSub) {
					t.Errorf("first error = %q, want substring %q", firstErr.Error(), tt.wantErrSub)
				}
			}
		})
	}
}

// ---------------------------------------------------------------------------
// StdinGuard concurrency safety
// ---------------------------------------------------------------------------

func TestStdinGuardConcurrentClaims(t *testing.T) {
	t.Parallel()
	guard := NewStdinGuard()

	const goroutines = 50
	results := make(chan error, goroutines)
	for i := 0; i < goroutines; i++ {
		go func(id int) {
			results <- guard.Claim("goroutine")
		}(i)
	}

	successCount := 0
	for i := 0; i < goroutines; i++ {
		if err := <-results; err == nil {
			successCount++
		}
	}
	if successCount != 1 {
		t.Errorf("exactly one goroutine should succeed, got %d", successCount)
	}
}
