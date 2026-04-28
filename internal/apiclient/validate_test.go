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

func TestValidateMethod(t *testing.T) {
	valid := []string{"GET", "get", "Post", "put", "PATCH", "delete"}
	for _, m := range valid {
		got, err := ValidateMethod(m)
		if err != nil {
			t.Errorf("ValidateMethod(%q) unexpected error: %v", m, err)
		}
		if got != strings.ToUpper(m) {
			t.Errorf("ValidateMethod(%q) = %q, want %q", m, got, strings.ToUpper(m))
		}
	}

	invalid := []string{"HEAD", "OPTIONS", "TRACE", "CONNECT", "INVALID", ""}
	for _, m := range invalid {
		_, err := ValidateMethod(m)
		if err == nil {
			t.Errorf("ValidateMethod(%q) expected error, got nil", m)
		}
	}
}

func TestValidatePath(t *testing.T) {
	// Valid paths
	for _, p := range []string{"/v1.0/users", "/v2.0/calendar/events", "v1.0/contact/users/me"} {
		if err := ValidatePath(p); err != nil {
			t.Errorf("ValidatePath(%q) unexpected error: %v", p, err)
		}
	}

	// Empty path
	if err := ValidatePath(""); err == nil {
		t.Error("ValidatePath(\"\") expected error")
	}

	// Path traversal
	if err := ValidatePath("/v1.0/../secret"); err == nil {
		t.Error("ValidatePath with .. expected error")
	}

	// Control character
	if err := ValidatePath("/v1.0/\x00test"); err == nil {
		t.Error("ValidatePath with null byte expected error")
	}
}

func TestRejectDangerousUnicode(t *testing.T) {
	// Zero-width space
	if err := ValidateUserInput("hello\u200Bworld", "test"); err == nil {
		t.Error("expected error for zero-width space")
	}
	// BOM
	if err := ValidateUserInput("\uFEFFhello", "test"); err == nil {
		t.Error("expected error for BOM")
	}
	// Bidi override
	if err := ValidateUserInput("hello\u202Aworld", "test"); err == nil {
		t.Error("expected error for bidi override")
	}
	// Normal string should pass
	if err := ValidateUserInput("hello world 你好", "test"); err != nil {
		t.Errorf("unexpected error for normal string: %v", err)
	}
}

func TestValidateStdinExclusion(t *testing.T) {
	if err := ValidateStdinExclusion("-", "-"); err == nil {
		t.Error("expected error when both params and data read from stdin")
	}
	if err := ValidateStdinExclusion("-", "{}"); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if err := ValidateStdinExclusion("{}", "-"); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestValidateFlagExclusion(t *testing.T) {
	if err := ValidateFlagExclusion("output.json", true); err == nil {
		t.Error("expected error when --output and --page-all both set")
	}
	if err := ValidateFlagExclusion("output.json", false); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if err := ValidateFlagExclusion("", true); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestMaskToken(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{"", "****"},
		{"abc", "****"},
		{"abcd", "****"},
		{"abcde", "abcd****"},
		{"abcdefghij", "abcd****"},
	}
	for _, tt := range tests {
		got := MaskToken(tt.in)
		if got != tt.want {
			t.Errorf("MaskToken(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestValidateTargetHost(t *testing.T) {
	// Allowed hosts
	allowed := []string{
		"https://api.dingtalk.com/v1.0/contact/users/me",
		"https://oapi.dingtalk.com/topapi/v2/user/get",
		"https://API.DINGTALK.COM/v1.0/test",
		"https://OAPI.DINGTALK.COM/topapi/test",
	}
	for _, u := range allowed {
		if err := ValidateTargetHost(u); err != nil {
			t.Errorf("ValidateTargetHost(%q) unexpected error: %v", u, err)
		}
	}

	// Blocked hosts
	blocked := []string{
		"https://oapi.dingtalk.fakedomain.com/topapi/v2/user/get",
		"https://fake.com/v1.0/test",
		"https://api.dingtalk.com.evil.com/v1.0/test",
		"https://evil.com/redirect?url=https://api.dingtalk.com",
		"http://localhost:8080/v1.0/test",
		"https://dingtalk.com/v1.0/test",
	}
	for _, u := range blocked {
		if err := ValidateTargetHost(u); err == nil {
			t.Errorf("ValidateTargetHost(%q) expected error, got nil", u)
		}
	}
}
