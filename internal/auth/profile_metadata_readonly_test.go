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

package auth

import (
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
)

func TestCrossPlatformCoverageResolveProfileMetadataReadOnly(t *testing.T) {
	const metadata = `{
  "version": 3,
  "currentProfile": "corp-a:user-a",
  "profiles": [
    {"name":"alpha","corpId":"corp-a","userId":"user-a","userName":"Alice"},
    {"name":"beta","corpId":"corp-b","userId":"user-b","userName":"Bob"}
  ]
}`
	reads := 0
	testseam.Swap(t, &profilesReadFile, func(path string) ([]byte, error) {
		reads++
		if !strings.HasSuffix(path, profilesJSONFile) {
			t.Fatalf("metadata path = %q", path)
		}
		return []byte(metadata), nil
	})

	current, err := ResolveProfileMetadataReadOnly("/config", "")
	if err != nil || current == nil || current.UserID != "user-a" || current.UserName != "Alice" || current.CorpID != "corp-a" {
		t.Fatalf("current metadata profile = %#v, %v", current, err)
	}
	explicit, err := ResolveProfileMetadataReadOnly("/config", "beta")
	if err != nil || explicit == nil || explicit.UserID != "user-b" || explicit.CorpID != "corp-b" {
		t.Fatalf("explicit metadata profile = %#v, %v", explicit, err)
	}
	if reads != 2 {
		t.Fatalf("profile metadata reads = %d, want 2", reads)
	}
}

func TestCrossPlatformCoverageResolveProfileMetadataReadOnlyFailsClosed(t *testing.T) {
	fail := errors.New("read failed")
	for _, tc := range []struct {
		name    string
		read    func(string) ([]byte, error)
		wantErr string
	}{
		{name: "missing", read: func(string) ([]byte, error) { return nil, os.ErrNotExist }},
		{name: "read error", read: func(string) ([]byte, error) { return nil, fail }, wantErr: "read profile metadata"},
		{name: "corrupt", read: func(string) ([]byte, error) { return []byte("{"), nil }, wantErr: "parse profile metadata"},
		{name: "forward version", read: func(string) ([]byte, error) { return []byte(`{"version":999}`), nil }, wantErr: "newer than supported"},
		{name: "no current", read: func(string) ([]byte, error) { return []byte(`{"version":3,"profiles":[]}`), nil }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			testseam.Swap(t, &profilesReadFile, tc.read)
			testseam.Swap(t, &profilesRename, func(string, string) error {
				t.Fatal("read-only metadata resolution attempted a quarantine rename")
				return nil
			})
			got, err := ResolveProfileMetadataReadOnly("/config", "")
			if tc.wantErr == "" {
				if err != nil || got != nil {
					t.Fatalf("read-only metadata = %#v, %v", got, err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) || got != nil {
				t.Fatalf("read-only metadata = %#v, %v; want %q", got, err, tc.wantErr)
			}
		})
	}
}

func TestCrossPlatformCoverageResolveProfileMetadataReadOnlyRejectsUnknownSelector(t *testing.T) {
	testseam.Swap(t, &profilesReadFile, func(string) ([]byte, error) {
		return []byte(`{"version":3,"currentProfile":"corp-a:user-a","profiles":[{"name":"alpha","corpId":"corp-a","userId":"user-a"}]}`), nil
	})
	profile, err := ResolveProfileMetadataReadOnly("/config", "missing")
	if err == nil || profile != nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("unknown read-only profile = %#v, %v", profile, err)
	}
}
