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
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/keychain"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
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

	if r.Status != statusPass {
		t.Errorf("expected pass for static endpoint mode, got %s", r.Status)
	}
}

func TestDoctorCheckCacheEmptyJSON(t *testing.T) {
	t.Setenv("DWS_CACHE_DIR", t.TempDir())

	var buf bytes.Buffer
	r := doctorCheckCache(&buf, true)

	if r.Status != statusPass {
		t.Errorf("expected pass for static endpoint mode, got %s", r.Status)
	}
	if buf.Len() != 0 {
		t.Error("expected no output in JSON mode")
	}
}

func TestDoctorCheckAuthReportsKeychainUnavailable(t *testing.T) {
	t.Setenv("DWS_CONFIG_DIR", filepath.Join(t.TempDir(), "config"))

	prev := edition.Get()
	edition.Override(&edition.Hooks{
		LoadToken: func(configDir string) ([]byte, error) {
			return nil, keychain.NewUnavailableError("read DEK from macOS Keychain", errors.New("default keychain missing"))
		},
	})
	t.Cleanup(func() {
		edition.Override(prev)
	})

	var buf bytes.Buffer
	r := doctorCheckAuth(context.Background(), &buf, false)

	if r.Name != "auth" {
		t.Fatalf("name = %q, want auth", r.Name)
	}
	if r.Status != statusFail {
		t.Fatalf("status = %q, want fail", r.Status)
	}
	if !strings.Contains(r.Message, "Keychain") && !strings.Contains(r.Message, "钥匙串") {
		t.Fatalf("message should mention Keychain/钥匙串; result=%+v", r)
	}
	if !strings.Contains(r.Hint, keychain.DisableKeychainEnv) {
		t.Fatalf("hint should mention %s; result=%+v", keychain.DisableKeychainEnv, r)
	}
}

func TestDoctorCheckAuthReportsDEKMissing(t *testing.T) {
	t.Setenv("DWS_CONFIG_DIR", filepath.Join(t.TempDir(), "config"))

	prev := edition.Get()
	edition.Override(&edition.Hooks{
		LoadToken: func(configDir string) ([]byte, error) {
			return nil, fmt.Errorf("load from keychain: %w", keychain.ErrDEKMissing)
		},
	})
	t.Cleanup(func() {
		edition.Override(prev)
	})

	var buf bytes.Buffer
	r := doctorCheckAuth(context.Background(), &buf, false)

	if r.Name != "auth" {
		t.Fatalf("name = %q, want auth", r.Name)
	}
	if r.Status != statusFail {
		t.Fatalf("status = %q, want fail", r.Status)
	}
	if !strings.Contains(r.Message, "登录密钥") {
		t.Fatalf("message should mention 登录密钥; result=%+v", r)
	}
	if !strings.Contains(r.Hint, "重新登录") {
		t.Fatalf("hint should mention 重新登录; result=%+v", r)
	}
	detail, ok := r.Detail.(map[string]string)
	if !ok || detail["reason"] != "dek_missing" {
		t.Fatalf("detail = %#v, want reason=dek_missing", r.Detail)
	}
}

func TestDoctorCheckKeychainReportsUnavailable(t *testing.T) {
	prev := doctorKeychainDiagnose
	doctorKeychainDiagnose = func() keychain.Diagnostic {
		return keychain.Diagnostic{
			OK:      false,
			Reason:  "keychain_unavailable",
			Message: "macOS 默认钥匙串不存在",
			Hint:    "恢复默认钥匙串后重试",
			Detail: map[string]string{
				"default_keychain": "/tmp/missing.keychain-db",
			},
		}
	}
	t.Cleanup(func() {
		doctorKeychainDiagnose = prev
	})

	var buf bytes.Buffer
	r := doctorCheckKeychain(&buf, false)

	if r.Name != "keychain" {
		t.Fatalf("name = %q, want keychain", r.Name)
	}
	if r.Status != statusFail {
		t.Fatalf("status = %q, want fail", r.Status)
	}
	if r.Message != "macOS 默认钥匙串不存在" {
		t.Fatalf("message = %q", r.Message)
	}
	if r.Hint == "" {
		t.Fatalf("hint is empty; result=%+v", r)
	}
	detail, ok := r.Detail.(map[string]string)
	if !ok || detail["default_keychain"] == "" {
		t.Fatalf("detail = %#v, want default_keychain", r.Detail)
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
