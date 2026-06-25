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
	"testing"

	authpkg "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/auth"
)

func TestWriteProfileUseJSONKeepsPrimaryAndCurrentDistinct(t *testing.T) {
	profile := &authpkg.Profile{
		Name:     "B Org",
		CorpID:   "corp_b",
		CorpName: "B Org",
		Status:   authpkg.ProfileStatusActive,
	}
	cfg := &authpkg.ProfilesConfig{
		PrimaryProfile: "corp_a",
		CurrentProfile: "corp_b",
	}
	var buf bytes.Buffer
	if err := writeProfileUseJSON(&buf, profile, cfg); err != nil {
		t.Fatalf("writeProfileUseJSON() error = %v", err)
	}
	var resp profileUseResponse
	if err := json.Unmarshal(buf.Bytes(), &resp); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if !resp.Profile.IsCurrent {
		t.Fatalf("isCurrent = false, want true")
	}
	if resp.Profile.IsPrimary {
		t.Fatalf("isPrimary = true, want false")
	}
}
