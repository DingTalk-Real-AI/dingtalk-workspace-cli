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

package pipeline

import "testing"

func TestNormalizeFlagTokenDirectContract(t *testing.T) {
	known := map[string]bool{"user-id": true}
	tests := []struct {
		argument string
		want     string
		ok       bool
	}{
		{argument: "-u"},
		{argument: "--"},
		{argument: "--user-id"},
		{argument: "--unknown"},
		{argument: "--unknownFlag"},
		{argument: "--userId=42", want: "--user-id=42", ok: true},
	}
	for _, test := range tests {
		got, ok := NormalizeFlagToken(test.argument, known)
		if got != test.want || ok != test.ok {
			t.Errorf("NormalizeFlagToken(%q) = %q, %v; want %q, %v", test.argument, got, ok, test.want, test.ok)
		}
	}
}

func TestSplitStickyFlagDirectContract(t *testing.T) {
	specs := map[string]FlagInfo{
		"limit":     {Name: "limit", Type: "int"},
		"page-size": {Name: "page-size", Type: "int"},
		"yes":       {Name: "yes", Type: "bool"},
	}
	tests := []struct {
		argument string
		want     StickyFlagPair
		ok       bool
	}{
		{argument: "-limit100"},
		{argument: "--limit=100"},
		{argument: "--"},
		{argument: "--limit"},
		{argument: "--pageSize"},
		{argument: "--unknown100"},
		{argument: "--limitabc"},
		{argument: "--yesfalse", want: StickyFlagPair{Flag: "--yes", Value: "false", Inline: true}, ok: true},
		{argument: "--yestrue", want: StickyFlagPair{Flag: "--yes", Value: "true", Inline: true}, ok: true},
		{argument: "--yesno", want: StickyFlagPair{Flag: "--yes", Value: "false", Inline: true}, ok: true},
		{argument: "--yesmaybe"},
		{argument: "--limit100", want: StickyFlagPair{Flag: "--limit", Value: "100"}, ok: true},
		{argument: "--pageSize50", want: StickyFlagPair{Flag: "--page-size", Value: "50"}, ok: true},
	}
	for _, test := range tests {
		got, ok := SplitStickyFlag(test.argument, specs)
		if got != test.want || ok != test.ok {
			t.Errorf("SplitStickyFlag(%q) = %#v, %v; want %#v, %v", test.argument, got, ok, test.want, test.ok)
		}
	}
}

func TestFuzzyMatchFlagDirectContract(t *testing.T) {
	known := map[string]bool{"limit": true, "name": true, "nave": true, "id": true}
	candidates := []string{"limit", "name", "nave", "id"}
	tests := []struct {
		argument      string
		candidates    []string
		useCandidates bool
		want          string
		ok            bool
	}{
		{argument: "-limt"},
		{argument: "--"},
		{argument: "--limit"},
		{argument: "--xy"},
		{argument: "--nae"},
		{argument: "--nothing", useCandidates: true},
		{argument: "--limt=10", want: "--limit=10", ok: true},
	}
	for _, test := range tests {
		caseCandidates := candidates
		if test.useCandidates {
			caseCandidates = test.candidates
		}
		got, ok := FuzzyMatchFlag(test.argument, known, caseCandidates)
		if got != test.want || ok != test.ok {
			t.Errorf("FuzzyMatchFlag(%q) = %q, %v; want %q, %v", test.argument, got, ok, test.want, test.ok)
		}
	}
}
