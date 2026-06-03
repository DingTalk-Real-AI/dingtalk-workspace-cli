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

package output

import (
	"bytes"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestResolveFormatFallsBackWithoutFlag(t *testing.T) {
	cmd := &cobra.Command{Use: "child"}
	if got := ResolveFormat(cmd, FormatJSON); got != FormatJSON {
		t.Fatalf("ResolveFormat() = %q, want %q", got, FormatJSON)
	}
}

func TestResolveFormatReadsInheritedFlag(t *testing.T) {
	root := &cobra.Command{Use: "dws"}
	root.PersistentFlags().String("format", "table", "")
	child := &cobra.Command{Use: "message"}
	root.AddCommand(child)

	if err := root.PersistentFlags().Set("format", "raw"); err != nil {
		t.Fatalf("Set(format) error = %v", err)
	}

	if got := ResolveFormat(child, FormatJSON); got != FormatRaw {
		t.Fatalf("ResolveFormat() = %q, want %q", got, FormatRaw)
	}
}

func TestWriteTableishFlattensPrimaryInvocationObject(t *testing.T) {
	var out bytes.Buffer
	payload := map[string]any{
		"invocation": map[string]any{
			"canonical_product": "message",
			"tool":              "send_message_fallback",
			"legacy_path":       "message send",
		},
	}

	if err := Write(&out, FormatTable, payload); err != nil {
		t.Fatalf("Write(table) error = %v", err)
	}

	got := out.String()
	if strings.HasPrefix(strings.TrimSpace(got), "{") {
		t.Fatalf("table output should not be JSON:\n%s", got)
	}
	for _, want := range []string{"canonical_product", "message", "send_message_fallback"} {
		if !strings.Contains(got, want) {
			t.Fatalf("table output missing %q:\n%s", want, got)
		}
	}
}

func TestWriteRawUsesCompactJSONForStructuredPayload(t *testing.T) {
	var out bytes.Buffer
	payload := map[string]any{
		"kind": "compat_invocation",
		"params": map[string]any{
			"recipient": "user-1",
		},
	}

	if err := Write(&out, FormatRaw, payload); err != nil {
		t.Fatalf("Write(raw) error = %v", err)
	}

	got := strings.TrimSpace(out.String())
	if strings.Contains(got, "\n  ") {
		t.Fatalf("raw output should be compact JSON:\n%s", got)
	}
	if !strings.HasPrefix(got, "{\"kind\":\"compat_invocation\"") {
		t.Fatalf("raw output = %q, want compact JSON", got)
	}
}
