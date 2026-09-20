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

package shortcut

import (
	"os"
	"path/filepath"
	"testing"
)

// runInputShortcut mounts a shortcut declaring an Input flag and runs it, so the
// framework's @file resolution happens exactly as in production. It reports what
// Execute observed through the typed accessors.
func runInputShortcut(t *testing.T, args []string, flags []Flag, read func(rt *RuntimeContext)) {
	t.Helper()
	s := Shortcut{
		Service: "doc",
		Command: "+input-probe",
		Flags:   flags,
		Execute: func(rt *RuntimeContext) error {
			read(rt)
			return nil
		},
	}
	cmd := mount(s)
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
	cmd.SetArgs(args)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute %v: %v", args, err)
	}
}

func writeShortcutPayload(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "payload.md")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write payload: %v", err)
	}
	return path
}

// A source-loaded payload must reach Execute byte-exact: RuntimeContext.Str
// trims every inline value, and trimming a payload would silently drop content
// a caller may depend on (a trailing newline covered by a checksum).
func TestCrossPlatformCoverageRuntimeStrKeepsPayloadVerbatim(t *testing.T) {
	flags := []Flag{{Name: "markdown", Desc: "内容（支持 @文件路径）", Input: []string{"file"}}}
	path := writeShortcutPayload(t, "  body  \n")

	var got string
	var resolved bool
	runInputShortcut(t, []string{"--markdown", "@" + path}, flags, func(rt *RuntimeContext) {
		got = rt.Str("markdown")
		resolved = rt.InputResolvedFromSource("markdown")
	})
	if got != "  body  \n" {
		t.Fatalf("Str() = %q, want the payload byte-exact", got)
	}
	if !resolved {
		t.Fatal("InputResolvedFromSource() = false for an @file value")
	}
}

// An inline value keeps the historical unconditional trim, so the 376 existing
// shortcut commands are unaffected by the payload exemption.
func TestCrossPlatformCoverageRuntimeStrTrimsInlineValue(t *testing.T) {
	flags := []Flag{{Name: "markdown", Desc: "内容（支持 @文件路径）", Input: []string{"file"}}}

	var got string
	var resolved bool
	runInputShortcut(t, []string{"--markdown", "  x  "}, flags, func(rt *RuntimeContext) {
		got = rt.Str("markdown")
		resolved = rt.InputResolvedFromSource("markdown")
	})
	if got != "x" {
		t.Fatalf("Str() = %q, want the trimmed inline value", got)
	}
	if resolved {
		t.Fatal("InputResolvedFromSource() = true for an inline value")
	}
}

// StrFirst must stop at an explicitly supplied payload even when it is empty;
// falling through to the alias would ship a value the caller did not ask for.
func TestCrossPlatformCoverageRuntimeStrFirstHonoursEmptyPayload(t *testing.T) {
	flags := []Flag{
		{Name: "markdown", Desc: "内容（支持 @文件路径）", Input: []string{"file"}},
		{Name: "text", Desc: "备选内容"},
	}
	path := writeShortcutPayload(t, "")

	var got string
	runInputShortcut(t, []string{"--markdown", "@" + path, "--text", "ALIAS"}, flags, func(rt *RuntimeContext) {
		got = rt.StrFirst("markdown", "text")
	})
	if got != "" {
		t.Fatalf("StrFirst() = %q, want the empty payload to win", got)
	}
}

// Without a source the existing first-non-empty behaviour is unchanged.
func TestCrossPlatformCoverageRuntimeStrFirstFallsBackWithoutPayload(t *testing.T) {
	flags := []Flag{
		{Name: "markdown", Desc: "内容（支持 @文件路径）", Input: []string{"file"}},
		{Name: "text", Desc: "备选内容"},
	}

	var got string
	runInputShortcut(t, []string{"--text", "ALIAS"}, flags, func(rt *RuntimeContext) {
		got = rt.StrFirst("markdown", "text")
	})
	if got != "ALIAS" {
		t.Fatalf("StrFirst() = %q, want the alias value", got)
	}
}
