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
	os.WriteFile(path, []byte(`{"title":"test"}`), 0o644)

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
	f, _ := os.Create(path)
	data := strings.Repeat("x", maxStdinSize+1)
	f.WriteString(data)
	f.Close()

	_, _, err := ReadFileArg("@" + path)
	if err == nil {
		t.Fatal("expected error for oversized file")
	}
	if !strings.Contains(err.Error(), "10 MB") {
		t.Errorf("unexpected error: %v", err)
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
