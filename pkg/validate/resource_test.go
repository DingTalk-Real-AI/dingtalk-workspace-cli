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

package validate

import (
	"strings"
	"testing"
)

func TestResourceName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		value      string
		wantErr    bool
		errContain string
	}{
		{"valid simple ID", "user123", false, ""},
		{"valid UUID", "550e8400-e29b-41d4-a716-446655440000", false, ""},
		{"valid with slash", "org/team/member", false, ""},
		{"empty", "", true, "must not be empty"},
		{"path traversal", "../admin", true, "path traversal"},
		{"path traversal mid", "a/../b", true, "path traversal"},
		{"query injection", "user?admin=true", true, "invalid characters"},
		{"fragment injection", "user#section", true, "invalid characters"},
		{"percent encoding bypass", "user%2e%2e", true, "invalid characters"},
		{"null byte", "user\x00id", true, "invalid characters"},
		{"control char", "user\x1fid", true, "invalid characters"},
		{"zero-width space", "user\u200Bid", true, "dangerous Unicode"},
		{"Bidi override", "user\u202Eid", true, "dangerous Unicode"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := ResourceName(tt.value, "--id")
			if (err != nil) != tt.wantErr {
				t.Errorf("ResourceName(%q) error = %v, wantErr %v", tt.value, err, tt.wantErr)
			}
			if err != nil && tt.errContain != "" && !strings.Contains(err.Error(), tt.errContain) {
				t.Errorf("ResourceName(%q) error = %q, want containing %q", tt.value, err.Error(), tt.errContain)
			}
		})
	}
}

func TestEncodePathSegment(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input string
		want  string
	}{
		{"simple", "simple"},
		{"with/slash", "with%2Fslash"},
		{"with?query", "with%3Fquery"},
		{"with#frag", "with%23frag"},
		{"with space", "with%20space"},
		{"../traversal", "..%2Ftraversal"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			t.Parallel()
			got := EncodePathSegment(tt.input)
			if got != tt.want {
				t.Errorf("EncodePathSegment(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}
