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

package agentproduct

import (
	"errors"
	"strings"
	"testing"
)

func TestParse(t *testing.T) {
	valid := []struct {
		name string
		raw  string
		want string
	}{
		{name: "unset", raw: "", want: ""},
		{name: "ASCII whitespace only", raw: " \t ", want: ""},
		{name: "qwenwork", raw: "qwenwork", want: "qwenwork"},
		{name: "legacy open claw", raw: "openClaw", want: "openClaw"},
		{name: "trim", raw: " \tqwenwork\t ", want: "qwenwork"},
		{name: "generic", raw: "agent-2_alpha", want: "agent-2_alpha"},
		{name: "leading digit", raw: "2nd_product", want: "2nd_product"},
		{name: "maximum length", raw: strings.Repeat("a", MaxValueBytes), want: strings.Repeat("a", MaxValueBytes)},
	}
	for _, tc := range valid {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Parse(tc.raw)
			if err != nil {
				t.Fatalf("Parse() error = %v", err)
			}
			if got != tc.want {
				t.Fatalf("Parse() = %q, want %q", got, tc.want)
			}
		})
	}

	invalid := []struct {
		name string
		raw  string
	}{
		{name: "carriage return", raw: "qwenwork\r"},
		{name: "line feed", raw: "\nqwenwork"},
		{name: "internal space", raw: "qwen work"},
		{name: "internal tab", raw: "qwen\twork"},
		{name: "unicode", raw: "千问办公"},
		{name: "leading dash", raw: "-qwenwork"},
		{name: "leading underscore", raw: "_qwenwork"},
		{name: "control character", raw: "qwenwork\x00cloud"},
		{name: "vertical tab", raw: "\vqwenwork"},
		{name: "form feed", raw: "qwenwork\f"},
		{name: "next line", raw: "qwenwork\u0085"},
		{name: "non-breaking space", raw: "\u00a0qwenwork"},
		{name: "ideographic space", raw: "qwenwork\u3000"},
		{name: "too long", raw: strings.Repeat("a", MaxValueBytes+1)},
	}
	for _, tc := range invalid {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Parse(tc.raw)
			if !errors.Is(err, ErrInvalid) {
				t.Fatalf("Parse(%q) = %q, %v; want ErrInvalid", tc.raw, got, err)
			}
			if strings.Contains(err.Error(), tc.raw) {
				t.Fatalf("error must not echo invalid value %q: %v", tc.raw, err)
			}
		})
	}
}

func TestResolveFromEnv(t *testing.T) {
	t.Run("unset uses fallback", func(t *testing.T) {
		t.Setenv(EnvName, "")
		got, err := ResolveFromEnv("openClaw")
		if err != nil || got != "openClaw" {
			t.Fatalf("ResolveFromEnv() = %q, %v; want openClaw, nil", got, err)
		}
	})

	t.Run("valid environment wins", func(t *testing.T) {
		t.Setenv(EnvName, " qwenwork ")
		got, err := ResolveFromEnv("openClaw")
		if err != nil || got != "qwenwork" {
			t.Fatalf("ResolveFromEnv() = %q, %v; want qwenwork, nil", got, err)
		}
	})

	t.Run("invalid environment does not return fallback", func(t *testing.T) {
		t.Setenv(EnvName, "qwen work")
		got, err := ResolveFromEnv("openClaw")
		if got != "" || !errors.Is(err, ErrInvalid) {
			t.Fatalf("ResolveFromEnv() = %q, %v; want empty, ErrInvalid", got, err)
		}
	})
}
