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

package apiclient

import (
	"strings"
	"testing"
)

func TestParseJSONMap_Empty(t *testing.T) {
	result, err := ParseJSONMap("", "--params", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != nil {
		t.Errorf("expected nil, got %v", result)
	}
}

func TestParseJSONMap_ValidJSON(t *testing.T) {
	result, err := ParseJSONMap(`{"key":"value","num":42}`, "--params", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result["key"] != "value" {
		t.Errorf("expected key=value, got %v", result["key"])
	}
}

func TestParseJSONMap_SingleQuotes(t *testing.T) {
	result, err := ParseJSONMap(`'{"key":"value"}'`, "--params", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result["key"] != "value" {
		t.Errorf("expected key=value, got %v", result["key"])
	}
}

func TestParseJSONMap_Stdin(t *testing.T) {
	stdin := strings.NewReader(`{"from":"stdin"}`)
	result, err := ParseJSONMap("-", "--params", stdin)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result["from"] != "stdin" {
		t.Errorf("expected from=stdin, got %v", result["from"])
	}
}

func TestParseJSONMap_InvalidJSON(t *testing.T) {
	_, err := ParseJSONMap("not json", "--params", nil)
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestParseOptionalBody_Empty(t *testing.T) {
	result, err := ParseOptionalBody("POST", "", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != nil {
		t.Errorf("expected nil, got %v", result)
	}
}

func TestParseOptionalBody_GETNotAllowed(t *testing.T) {
	_, err := ParseOptionalBody("GET", `{"data":true}`, nil)
	if err == nil {
		t.Error("expected error for GET with body")
	}
}

func TestParseOptionalBody_ValidPOST(t *testing.T) {
	result, err := ParseOptionalBody("POST", `{"key":"value"}`, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	m, ok := result.(map[string]any)
	if !ok {
		t.Fatalf("expected map, got %T", result)
	}
	if m["key"] != "value" {
		t.Errorf("expected key=value, got %v", m["key"])
	}
}

func TestStripSingleQuotes(t *testing.T) {
	tests := []struct{ in, want string }{
		{`'hello'`, `hello`},
		{`"hello"`, `"hello"`},
		{`hello`, `hello`},
		{`''`, ``},
		{`'`, `'`},
	}
	for _, tt := range tests {
		got := stripSingleQuotes(tt.in)
		if got != tt.want {
			t.Errorf("stripSingleQuotes(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestTruncate(t *testing.T) {
	if got := truncate("hello", 10); got != "hello" {
		t.Errorf("expected hello, got %q", got)
	}
	if got := truncate("hello world", 5); got != "hello..." {
		t.Errorf("expected hello..., got %q", got)
	}
}
