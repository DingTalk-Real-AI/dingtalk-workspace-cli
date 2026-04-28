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
	"bytes"
	"strings"
	"testing"
)

func TestParseQueryStringToJSON(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name, raw, want string
	}{
		{
			name: "simple key-value",
			raw:  "timeMin=2026-04-01&maxResults=10",
			want: `{"maxResults":"10","timeMin":"2026-04-01"}`,
		},
		{
			name: "with special chars",
			raw:  "timeMin=2026-04-01T14:00:00+08:00&showDeleted=false",
			want: `{"showDeleted":"false","timeMin":"2026-04-01T14:00:00+08:00"}`,
		},
		{
			name: "empty value skipped",
			raw:  "nextToken=&syncToken=abc",
			want: `{"syncToken":"abc"}`,
		},
		{
			name: "all empty",
			raw:  "nextToken=&syncToken=",
			want: "{}",
		},
		{
			name: "empty string",
			raw:  "",
			want: "{}",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseQueryStringToJSON(tt.raw)
			if got != tt.want {
				t.Errorf("parseQueryStringToJSON(%q) = %s, want %s", tt.raw, got, tt.want)
			}
		})
	}
}

func TestRunAPI_QueryStringBlocked(t *testing.T) {
	t.Parallel()

	gf := &GlobalFlags{}
	cmd := newAPICommand(gf)

	var stdout, stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)

	cmd.SetArgs([]string{"GET", "/v1.0/calendar/users/me/events?timeMin=2026-04-01&maxResults=10"})
	err := cmd.Execute()

	if err == nil {
		t.Fatal("expected error when path contains query string, got nil")
	}
	errMsg := stderr.String()
	if !strings.Contains(errMsg, "--params") {
		t.Errorf("expected --params hint in error, got: %s", errMsg)
	}
	if !strings.Contains(errMsg, "maxResults") {
		t.Errorf("expected parsed query params in error, got: %s", errMsg)
	}
	if !strings.Contains(errMsg, "/v1.0/calendar/users/me/events") {
		t.Errorf("expected clean path in suggestion, got: %s", errMsg)
	}
}

func TestRunAPI_NoErrorWithoutQueryString(t *testing.T) {
	t.Parallel()

	gf := &GlobalFlags{}
	cmd := newAPICommand(gf)

	var stderr bytes.Buffer
	cmd.SetErr(&stderr)
	cmd.SetOut(&bytes.Buffer{})

	cmd.SetArgs([]string{"GET", "/v1.0/contact/users/me"})
	err := cmd.Execute()

	errMsg := stderr.String()
	if strings.Contains(errMsg, "查询参数") {
		t.Errorf("should not reject path without query string, got: %s", errMsg)
	}
	_ = err
}
