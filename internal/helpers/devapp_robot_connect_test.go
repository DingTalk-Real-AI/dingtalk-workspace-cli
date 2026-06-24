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

package helpers

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

// TestDevAppRobotConnectRegistered confirms `dev connect` is wired into
// the robot subtree alongside the (reused) provisioning commands.
func TestDevAppRobotConnectRegistered(t *testing.T) {
	root := newDevAppCommand(&captureRunner{})
	if _, _, err := root.Find([]string{"robot", "connect"}); err != nil {
		t.Fatalf("dev connect not registered: %v", err)
	}
	// Provisioning stays under robot (reused, not duplicated by connect).
	for _, name := range []string{"create", "submit", "result"} {
		if _, _, err := root.Find([]string{"robot", name}); err != nil {
			t.Fatalf("devapp robot %s missing: %v", name, err)
		}
	}
}

// TestDevAppRobotConnectValidation covers the credential / channel guards on the
// dry-run path (which never launches the Stream connector).
func TestDevAppRobotConnectValidation(t *testing.T) {
	cases := []struct {
		name     string
		args     []string
		wantErr  string
		wantJSON []string // substrings expected in successful dry-run output
	}{
		{
			name:    "no credentials and no unified-app-id",
			args:    []string{"--channel", "claudecode", "--dry-run"},
			wantErr: "需要 --robot-client-id/--robot-client-secret",
		},
		{
			name:    "unknown channel",
			args:    []string{"--channel", "nope", "--robot-client-id", "a", "--robot-client-secret", "b", "--dry-run"},
			wantErr: "未知渠道",
		},
		{
			name:     "explicit credentials dry-run emits plan",
			args:     []string{"--channel", "claudecode", "--robot-client-id", "id1", "--robot-client-secret", "sec1", "--dry-run"},
			wantJSON: []string{"\"credentialSource\"", "flag:--robot-client-id/--robot-client-secret", "stream-bridge", "\"clientId\"", "\"completionState\": \"LOCAL_DEBUG_ONLY\"", "\"doesNotPublish\": true", "\"scope\": \"local_debug_only\""},
		},
		{
			name:     "unified-app-id dry-run skips credentials get",
			args:     []string{"--channel", "qoderwork", "--unified-app-id", "UAID", "--dry-run"},
			wantJSON: []string{"credentials get, skipped in dry-run", "\"unifiedAppId\"", "\"completionState\": \"LOCAL_DEBUG_ONLY\"", "\"doesNotPublish\": true", "\"scope\": \"local_debug_only\""},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := newDevAppTestRoot(&captureRunner{})
			var out bytes.Buffer
			root.SetOut(&out)
			root.SetErr(&out)
			root.SetArgs(append([]string{"dev", "connect"}, tc.args...))
			err := root.Execute()

			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("want error containing %q, got err=%v out=%s", tc.wantErr, err, out.String())
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v\noutput:\n%s", err, out.String())
			}
			for _, sub := range tc.wantJSON {
				if !strings.Contains(out.String(), sub) {
					t.Fatalf("output missing %q:\n%s", sub, out.String())
				}
			}
		})
	}
}

func TestDevAppRobotConnectLocalDebugNotice(t *testing.T) {
	notice := connectLocalDebugNotice()
	for _, sub := range []string{"本地调试", "不代表线上发布完成", "不会提交版本发布", "APPROVAL_REQUIRED"} {
		if !strings.Contains(notice, sub) {
			t.Fatalf("notice = %q, want substring %q", notice, sub)
		}
	}
}

// TestDevAppFetchCredentials checks that --unified-app-id credential resolution
// reuses get_dev_app_credentials and extracts clientId/clientSecret from the
// MCP envelope.
func TestDevAppFetchCredentials(t *testing.T) {
	runner := &devAppResponseRunner{response: map[string]any{
		"content": map[string]any{
			"result": map[string]any{
				"clientId":     "ck-123",
				"clientSecret": "cs-456",
			},
		},
	}}
	root := newDevAppTestRoot(runner)
	connectCmd, _, err := root.Find([]string{"dev", "connect"})
	if err != nil {
		t.Fatalf("find connect: %v", err)
	}
	connectCmd.SetContext(context.Background())

	id, secret, err := devAppFetchCredentials(runner, connectCmd, "UAID-1")
	if err != nil {
		t.Fatalf("devAppFetchCredentials error: %v", err)
	}
	if id != "ck-123" || secret != "cs-456" {
		t.Fatalf("got id=%q secret=%q, want ck-123/cs-456", id, secret)
	}
	if runner.last.Tool != "get_dev_app_credentials" {
		t.Fatalf("Tool = %q, want get_dev_app_credentials", runner.last.Tool)
	}
	if runner.last.CanonicalProduct != devAppProduct {
		t.Fatalf("CanonicalProduct = %q, want %q", runner.last.CanonicalProduct, devAppProduct)
	}
	if got := runner.last.Params["unifiedAppId"]; got != "UAID-1" {
		t.Fatalf("unifiedAppId param = %v, want UAID-1", got)
	}
}

// TestDevAppConnectUnwrap covers the envelope descent and the appKey/appSecret
// field fallbacks.
func TestDevAppConnectUnwrap(t *testing.T) {
	// appKey/appSecret fallback when clientId/clientSecret absent.
	payload := devAppConnectUnwrap(map[string]any{
		"result": map[string]any{"appKey": "ak", "appSecret": "as"},
	})
	if got := devAppConnectFirst(payload, "clientId", "appKey"); got != "ak" {
		t.Fatalf("clientId fallback = %q, want ak", got)
	}
	if got := devAppConnectFirst(payload, "clientSecret", "appSecret"); got != "as" {
		t.Fatalf("clientSecret fallback = %q, want as", got)
	}
	// nil / missing tolerated.
	if got := devAppConnectFirst(nil, "clientId"); got != "" {
		t.Fatalf("nil map = %q, want empty", got)
	}
}
