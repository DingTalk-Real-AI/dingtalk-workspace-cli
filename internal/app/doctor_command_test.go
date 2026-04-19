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
	"encoding/json"
	"strings"
	"testing"
)

func TestCountResults(t *testing.T) {
	checks := []checkResult{
		{Status: statusPass},
		{Status: statusPass},
		{Status: statusWarn},
		{Status: statusFail},
	}
	pass, warn, fail := countResults(checks)
	if pass != 2 || warn != 1 || fail != 1 {
		t.Errorf("expected (2,1,1), got (%d,%d,%d)", pass, warn, fail)
	}
}

func TestCountResultsAllPass(t *testing.T) {
	checks := []checkResult{
		{Status: statusPass},
		{Status: statusPass},
	}
	pass, warn, fail := countResults(checks)
	if pass != 2 || warn != 0 || fail != 0 {
		t.Errorf("expected (2,0,0), got (%d,%d,%d)", pass, warn, fail)
	}
}

func TestStatusIcon(t *testing.T) {
	tests := []struct {
		status checkStatus
		want   string
	}{
		{statusPass, "✅"},
		{statusWarn, "⚠️"},
		{statusFail, "❌"},
	}
	for _, tc := range tests {
		got := statusIcon(tc.status)
		if got != tc.want {
			t.Errorf("statusIcon(%q) = %q, want %q", tc.status, got, tc.want)
		}
	}
}

func TestPrintCheckResult(t *testing.T) {
	var buf bytes.Buffer
	r := checkResult{
		Name:    "test",
		Status:  statusFail,
		Message: "something broke",
		Hint:    "try fixing it",
	}
	printCheckResult(&buf, r)

	out := buf.String()
	if !strings.Contains(out, "❌") {
		t.Error("expected fail icon")
	}
	if !strings.Contains(out, "something broke") {
		t.Error("expected message")
	}
	if !strings.Contains(out, "try fixing it") {
		t.Error("expected hint")
	}
}

func TestPrintCheckResultNoHint(t *testing.T) {
	var buf bytes.Buffer
	r := checkResult{
		Name:    "test",
		Status:  statusPass,
		Message: "all good",
	}
	printCheckResult(&buf, r)

	out := buf.String()
	if !strings.Contains(out, "✅") {
		t.Error("expected pass icon")
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) != 1 {
		t.Errorf("expected 1 line (no hint), got %d", len(lines))
	}
}

func TestDoctorCheckCacheEmpty(t *testing.T) {
	t.Setenv("DWS_CACHE_DIR", t.TempDir())

	var buf bytes.Buffer
	r := doctorCheckCache(&buf, false)

	if r.Status != statusWarn {
		t.Errorf("expected warn for empty cache, got %s", r.Status)
	}
	if !strings.Contains(r.Message, "缓存为空") {
		t.Errorf("expected empty cache message, got %q", r.Message)
	}
}

func TestDoctorCheckCacheEmptyJSON(t *testing.T) {
	t.Setenv("DWS_CACHE_DIR", t.TempDir())

	var buf bytes.Buffer
	r := doctorCheckCache(&buf, true)

	if r.Status != statusWarn {
		t.Errorf("expected warn for empty cache, got %s", r.Status)
	}
	if buf.Len() != 0 {
		t.Error("expected no output in JSON mode")
	}
}

func TestDoctorCommandStructure(t *testing.T) {
	cmd := newDoctorCommand()
	if cmd.Use != "doctor" {
		t.Errorf("Use = %q, want doctor", cmd.Use)
	}

	jsonFlag := cmd.Flags().Lookup("json")
	if jsonFlag == nil {
		t.Error("expected --json flag")
	}
	timeoutFlag := cmd.Flags().Lookup("timeout")
	if timeoutFlag == nil {
		t.Error("expected --timeout flag")
	}
}

func TestCheckResultJSONMarshal(t *testing.T) {
	r := checkResult{
		Name:    "auth",
		Status:  statusPass,
		Message: "已登录",
	}
	data, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	var parsed map[string]any
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatal(err)
	}
	if parsed["name"] != "auth" {
		t.Errorf("expected name=auth, got %v", parsed["name"])
	}
	if parsed["status"] != "pass" {
		t.Errorf("expected status=pass, got %v", parsed["status"])
	}
	if _, hasHint := parsed["hint"]; hasHint {
		t.Error("empty hint should be omitted")
	}
}
