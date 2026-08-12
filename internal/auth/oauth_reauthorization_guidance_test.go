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
	"strconv"
	"strings"
	"testing"
)

func TestLegacyRefreshReauthorizationGuidanceTreatsProfileAsDisplayData(t *testing.T) {
	tests := []struct {
		name     string
		selector string
	}{
		{name: "command substitution", selector: `external-$(touch marker)`},
		{name: "backticks", selector: "external-`touch marker`"},
		{name: "newline", selector: "external\ndws auth reset"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			guidance := legacyRefreshReauthorizationGuidance(tt.selector)

			if !strings.Contains(guidance, "dws auth login") ||
				!strings.Contains(guidance, "--profile") ||
				!strings.Contains(guidance, "profile 标识") {
				t.Fatalf("guidance lacks stable reauthorization instructions: %q", guidance)
			}
			if strings.Contains(guidance, "dws auth login --profile") ||
				strings.Contains(guidance, "--profile "+strconv.Quote(tt.selector)) {
				t.Fatalf("guidance embeds untrusted selector in an executable command: %q", guidance)
			}
			if !strings.Contains(guidance, "profile: "+strconv.Quote(tt.selector)) {
				t.Fatalf("guidance does not preserve selector as display data: %q", guidance)
			}
			if strings.Contains(tt.selector, "\n") && strings.Contains(guidance, tt.selector) {
				t.Fatalf("guidance retained a raw selector newline: %q", guidance)
			}
		})
	}
}

func TestLegacyRefreshReauthorizationGuidanceWithoutProfileStillExplainsLogin(t *testing.T) {
	guidance := legacyRefreshReauthorizationGuidance("")
	if !strings.Contains(guidance, "dws auth login") {
		t.Fatalf("guidance = %q, want login instruction", guidance)
	}
	if strings.Contains(guidance, "--profile") || strings.Contains(guidance, "profile:") {
		t.Fatalf("guidance = %q, should not invent an empty profile value", guidance)
	}
}
