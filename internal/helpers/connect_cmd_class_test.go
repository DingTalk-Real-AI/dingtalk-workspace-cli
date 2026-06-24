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

import "testing"

func TestClassifyDwsCommandReadVerbs(t *testing.T) {
	readLeaves := []string{
		"list", "get", "search", "read", "view", "query", "show",
		"detail", "status", "download", "export", "fetch", "info",
		"find", "summary",
	}
	for _, v := range readLeaves {
		if got := ClassifyDwsCommand("todo", "task", v); got != CmdClassRead {
			t.Errorf("verb %q: got %v, want CmdClassRead", v, got)
		}
	}
}

func TestClassifyDwsCommandWriteVerbs(t *testing.T) {
	writeLeaves := []string{
		"create", "update", "delete", "submit", "send", "done",
		"cancel", "offline", "enable", "disable", "remove", "add",
		"set", "approve", "reject", "write", "upload", "move", "copy",
	}
	for _, v := range writeLeaves {
		if got := ClassifyDwsCommand("approval", "instance", v); got != CmdClassWrite {
			t.Errorf("verb %q: got %v, want CmdClassWrite", v, got)
		}
	}
}

func TestClassifyDwsCommandUnknown(t *testing.T) {
	cases := [][]string{
		{},
		{"todo"},                   // container only, no action verb
		{"todo", "task"},           // still only containers
		{"frobnicate"},             // nonsense verb
		{"todo", "task", "wibble"}, // unknown leaf, no known ancestor
		{"   ", ""},                // all empty after trim
	}
	for _, c := range cases {
		if got := ClassifyDwsCommand(c...); got != CmdClassUnknown {
			t.Errorf("parts %v: got %v, want CmdClassUnknown", c, got)
		}
	}
}

func TestClassifyDwsCommandRealSamples(t *testing.T) {
	cases := []struct {
		parts []string
		want  CmdClass
	}{
		// Real dws command paths sampled from the repo's cobra Use: defs.
		{[]string{"todo", "task", "create"}, CmdClassWrite},
		{[]string{"todo", "task", "list"}, CmdClassRead},
		{[]string{"todo", "task", "get"}, CmdClassRead},
		{[]string{"todo", "task", "update"}, CmdClassWrite},
		{[]string{"todo", "task", "delete"}, CmdClassWrite},
		{[]string{"todo", "task", "done"}, CmdClassWrite},
		{[]string{"approval", "instance", "submit"}, CmdClassWrite},
		{[]string{"attendance", "get"}, CmdClassRead},
		{[]string{"chat", "message", "send"}, CmdClassWrite},
		{[]string{"chat", "message", "list"}, CmdClassRead},
		{[]string{"drive", "file", "download"}, CmdClassRead},
		{[]string{"drive", "file", "upload"}, CmdClassWrite},
		{[]string{"drive", "dir", "mkdir"}, CmdClassWrite},
		{[]string{"doc", "export"}, CmdClassRead},
		{[]string{"contact", "user", "search"}, CmdClassRead},
		{[]string{"report", "status"}, CmdClassRead},
	}
	for _, c := range cases {
		if got := ClassifyDwsCommand(c.parts...); got != c.want {
			t.Errorf("cmd %v: got %v, want %v", c.parts, got, c.want)
		}
	}
}

func TestClassifyDwsCommandCompoundVerbs(t *testing.T) {
	cases := []struct {
		parts []string
		want  CmdClass
	}{
		// Compound leaf verbs that exist in the repo.
		{[]string{"chat", "send-by-bot"}, CmdClassWrite},
		{[]string{"chat", "send-by-webhook"}, CmdClassWrite},
		{[]string{"chat", "recall-by-bot"}, CmdClassWrite},
		{[]string{"aitable", "form", "list-forms"}, CmdClassRead},
		{[]string{"wiki", "space", "list-spaces"}, CmdClassRead},
		{[]string{"record", "batch-update"}, CmdClassWrite},
		{[]string{"group", "add-bot"}, CmdClassWrite},
		{[]string{"group", "remove-bot"}, CmdClassWrite},
	}
	for _, c := range cases {
		if got := ClassifyDwsCommand(c.parts...); got != c.want {
			t.Errorf("compound %v: got %v, want %v", c.parts, got, c.want)
		}
	}
}

func TestClassifyDwsCommandLeafFallbackToAncestor(t *testing.T) {
	// When the true leaf is a placeholder/id segment we don't recognise,
	// classification falls back to the nearest known ancestor verb.
	if got := ClassifyDwsCommand("todo", "task", "get", "abc123"); got != CmdClassRead {
		t.Errorf("trailing id segment: got %v, want CmdClassRead", got)
	}
	if got := ClassifyDwsCommand("todo", "task", "create", "xyz"); got != CmdClassWrite {
		t.Errorf("trailing id segment: got %v, want CmdClassWrite", got)
	}
}

func TestCmdClassOverrideTable(t *testing.T) {
	// Suppose "download" should require confirmation for some tenant: a
	// full-path override flips a normally-read command to write without
	// touching the heuristic tables.
	overrides := map[string]CmdClass{
		"drive file download": CmdClassWrite, // full-path override
		"export":              CmdClassWrite, // single-verb override
	}

	if got := ClassifyDwsCommandWith(overrides, "drive", "file", "download"); got != CmdClassWrite {
		t.Errorf("full-path override: got %v, want CmdClassWrite", got)
	}
	// A different path whose leaf is "download" is NOT affected by the
	// full-path override and stays read.
	if got := ClassifyDwsCommandWith(overrides, "media", "download"); got != CmdClassRead {
		t.Errorf("unrelated download path: got %v, want CmdClassRead", got)
	}
	// Single-verb override flips every "export" leaf.
	if got := ClassifyDwsCommandWith(overrides, "doc", "export"); got != CmdClassWrite {
		t.Errorf("single-verb override: got %v, want CmdClassWrite", got)
	}
}

func TestSetCmdClassOverrideProcessWide(t *testing.T) {
	defer SetCmdClassOverride("status", CmdClassUnknown) // cleanup

	// Baseline: status is read.
	if got := ClassifyDwsCommand("report", "status"); got != CmdClassRead {
		t.Fatalf("baseline status: got %v, want CmdClassRead", got)
	}
	// Register a process-wide override making status write.
	SetCmdClassOverride("STATUS", CmdClassWrite) // case-insensitive key
	if got := ClassifyDwsCommand("report", "status"); got != CmdClassWrite {
		t.Errorf("after override: got %v, want CmdClassWrite", got)
	}
	// Removing the override restores the heuristic result.
	SetCmdClassOverride("status", CmdClassUnknown)
	if got := ClassifyDwsCommand("report", "status"); got != CmdClassRead {
		t.Errorf("after override removal: got %v, want CmdClassRead", got)
	}
}

func TestCmdClassString(t *testing.T) {
	cases := map[CmdClass]string{
		CmdClassRead:    "read",
		CmdClassWrite:   "write",
		CmdClassUnknown: "unknown",
	}
	for c, want := range cases {
		if got := c.String(); got != want {
			t.Errorf("String(%d): got %q, want %q", c, got, want)
		}
	}
}
