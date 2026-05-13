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

package compat

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// TestJSONParse_StrictJSON covers the primary path: callers passing
// canonical JSON (as generated programmatically or by agents).
func TestJSONParse_StrictJSON(t *testing.T) {
	t.Parallel()

	input := `[{"fieldName":"title","type":"text"},{"fieldName":"count","type":"number"}]`
	got, err := ApplyTransform(input, "json_parse", nil)
	if err != nil {
		t.Fatalf("strict JSON should parse, got err: %v", err)
	}
	arr, ok := got.([]any)
	if !ok || len(arr) != 2 {
		t.Fatalf("expected []any of length 2, got %T %v", got, got)
	}
}

// TestJSONParse_YAMLFlowFallback is the motivating case: a user types an
// ad-hoc JSON-shaped array without quoting every key and value. YAML flow
// syntax accepts it and the parsed output is indistinguishable from the
// strict-JSON equivalent.
func TestJSONParse_YAMLFlowFallback(t *testing.T) {
	t.Parallel()

	// Intentionally unquoted keys, unquoted string values, and Chinese
	// identifiers — typical of what humans type at a shell.
	input := `[{fieldName: 标题, type: text}, {fieldName: 数量, type: number, config: {formatter: INT}}, {fieldName: 状态, type: singleSelect, config: {options: [{name: 待办}, {name: 进行中}, {name: 已完成}]}}, {fieldName: 已确认, type: checkbox}]`

	got, err := ApplyTransform(input, "json_parse", nil)
	if err != nil {
		t.Fatalf("YAML-flow input should parse, got err: %v", err)
	}
	arr, ok := got.([]any)
	if !ok {
		t.Fatalf("expected []any, got %T", got)
	}
	if len(arr) != 4 {
		t.Fatalf("expected 4 field definitions, got %d", len(arr))
	}

	// Spot-check the third entry, which is the most deeply nested.
	third, ok := arr[2].(map[string]any)
	if !ok {
		t.Fatalf("arr[2] expected map[string]any, got %T", arr[2])
	}
	if third["fieldName"] != "状态" {
		t.Errorf("arr[2].fieldName: want 状态, got %v", third["fieldName"])
	}
	config, ok := third["config"].(map[string]any)
	if !ok {
		t.Fatalf("arr[2].config expected map, got %T", third["config"])
	}
	options, ok := config["options"].([]any)
	if !ok || len(options) != 3 {
		t.Fatalf("arr[2].config.options: want 3 items, got %v", config["options"])
	}
}

// TestJSONParse_EmptyString preserves the legacy behaviour of returning the
// original value untouched when the caller passes an empty / whitespace-only
// string, matching how other transforms treat empty input.
func TestJSONParse_EmptyString(t *testing.T) {
	t.Parallel()

	cases := []string{"", "   ", "\n\t"}
	for _, in := range cases {
		got, err := ApplyTransform(in, "json_parse", nil)
		if err != nil {
			t.Errorf("empty input %q should not error: %v", in, err)
			continue
		}
		if !reflect.DeepEqual(got, in) {
			t.Errorf("empty input %q should pass through, got %v", in, got)
		}
	}
}

// TestJSONParse_NonString passes through non-string inputs (already-parsed
// values flowing through the pipeline).
func TestJSONParse_NonString(t *testing.T) {
	t.Parallel()

	preParsed := []any{map[string]any{"k": "v"}}
	got, err := ApplyTransform(preParsed, "json_parse", nil)
	if err != nil {
		t.Fatalf("non-string should pass through: %v", err)
	}
	if !reflect.DeepEqual(got, preParsed) {
		t.Errorf("non-string should pass through unchanged, got %v", got)
	}
}

// TestJSONParse_InvalidInput verifies that genuine garbage is still rejected
// with a user-facing validation error that nudges towards `@file` syntax.
func TestJSONParse_InvalidInput(t *testing.T) {
	t.Parallel()

	// Unterminated bracket — neither valid JSON nor valid YAML flow.
	_, err := ApplyTransform("[{fieldName:", "json_parse", nil)
	if err == nil {
		t.Fatal("expected error for malformed input")
	}
	if msg := err.Error(); msg == "" {
		t.Fatal("error message should be non-empty")
	}
}

// TestFileRead_BasicFile exercises the happy path: a UTF-8 file on disk is
// read in full and surfaced as a string value. This is the contract the
// `--content-file ./a.md` flag relies on so the upstream MCP tool sees the
// file contents in place of the path.
func TestFileRead_BasicFile(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "note.md")
	contents := "# Heading\n\n- bullet one\n- bullet two\n"
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatalf("setup: %v", err)
	}

	got, err := ApplyTransform(path, "file_read", nil)
	if err != nil {
		t.Fatalf("file_read should succeed, got err: %v", err)
	}
	if got != contents {
		t.Errorf("file_read should return file contents verbatim; got %q want %q", got, contents)
	}
}

// TestFileRead_EmptyPath rejects empty input with a validation error rather
// than silently reading "" / cwd. The dispatcher maps validation errors to
// exit code 2 so the user sees a usage problem.
func TestFileRead_EmptyPath(t *testing.T) {
	t.Parallel()

	_, err := ApplyTransform("", "file_read", nil)
	if err == nil {
		t.Fatal("expected validation error for empty path")
	}
	if !strings.Contains(err.Error(), "file_read") {
		t.Errorf("error should mention the transform name, got %q", err.Error())
	}
}

// TestFileRead_MissingFile surfaces a clear validation error when the path
// doesn't exist. The previous `os.ReadFile` error is wrapped so the user
// sees what they passed.
func TestFileRead_MissingFile(t *testing.T) {
	t.Parallel()

	missing := filepath.Join(t.TempDir(), "definitely-not-here.md")
	_, err := ApplyTransform(missing, "file_read", nil)
	if err == nil {
		t.Fatal("expected error for missing file")
	}
	if !strings.Contains(err.Error(), "definitely-not-here.md") {
		t.Errorf("error should mention the missing path, got %q", err.Error())
	}
}

// TestFileRead_InvalidUTF8 rejects binary input. Upstream tools expect text
// content and silently shipping a corrupted byte string would mask a real
// user error.
func TestFileRead_InvalidUTF8(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "binary.dat")
	if err := os.WriteFile(path, []byte{0xff, 0xfe, 0x00, 0x01}, 0o600); err != nil {
		t.Fatalf("setup: %v", err)
	}
	_, err := ApplyTransform(path, "file_read", nil)
	if err == nil {
		t.Fatal("expected UTF-8 validation error for binary input")
	}
	if !strings.Contains(err.Error(), "UTF-8") {
		t.Errorf("error should mention UTF-8, got %q", err.Error())
	}
}

// TestFileRead_NonString rejects non-string flag values. CLI flags resolve to
// string by default but a misconfigured envelope (e.g. Type: int) shouldn't
// silently no-op.
func TestFileRead_NonString(t *testing.T) {
	t.Parallel()

	_, err := ApplyTransform(123, "file_read", nil)
	if err == nil {
		t.Fatal("expected validation error for non-string value")
	}
}

// TestFileRead_StdinDashIsAccepted documents the contract: the special value
// "-" is reserved for stdin. We don't test stdin redirection here (that
// requires plumbing os.Stdin replacement which complicates the test) — this
// is a compile-time signal that "-" doesn't path-resolve to a file named "-"
// in the current directory. The end-to-end stdin path is covered in
// test/cli_compat once the envelope ships.
func TestFileRead_StdinDashIsAccepted(t *testing.T) {
	t.Parallel()

	// Run with stdin redirected from an empty pipe so we don't hang.
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	defer r.Close()
	if _, err := w.Write([]byte("piped content")); err != nil {
		t.Fatalf("setup: %v", err)
	}
	w.Close()

	origStdin := os.Stdin
	os.Stdin = r
	defer func() { os.Stdin = origStdin }()

	got, err := ApplyTransform("-", "file_read", nil)
	if err != nil {
		t.Fatalf("file_read with '-' should read stdin, got err: %v", err)
	}
	if got != "piped content" {
		t.Errorf("expected stdin contents, got %q", got)
	}
}

// TestFileRead_UnknownTransformPassThrough double-checks that the new case
// is gated by name and doesn't regress when the transform name is missing.
func TestFileRead_UnknownTransformPassThrough(t *testing.T) {
	t.Parallel()

	got, err := ApplyTransform("./some-path", "", nil)
	if err != nil {
		t.Fatalf("empty transform should pass through, got err: %v", err)
	}
	if !reflect.DeepEqual(got, "./some-path") {
		t.Errorf("expected pass-through, got %v", got)
	}
}
