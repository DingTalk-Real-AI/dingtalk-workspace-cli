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
	"reflect"
	"strings"
	"testing"
)

func TestWikiTopLevelListProxyExecutesSpaceList(t *testing.T) {
	t.Parallel()

	runner := &wikiCommandRunner{}
	cmd := wikiHandler{}.Command(runner)
	var out, errOut bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	cmd.SetArgs([]string{"list", "--format", "json"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v\nstderr:\n%s", err, errOut.String())
	}
	if !strings.Contains(strings.ToLower(errOut.String()), "redirecting to") {
		t.Fatalf("stderr = %q, want redirecting hint", errOut.String())
	}
	if runner.last.CanonicalProduct != "wiki" {
		t.Fatalf("product = %q, want wiki", runner.last.CanonicalProduct)
	}
	if runner.last.Tool != "list_wikiSpaces" {
		t.Fatalf("tool = %q, want list_wikiSpaces", runner.last.Tool)
	}
	if runner.last.LegacyPath != "wiki space list" {
		t.Fatalf("legacy path = %q, want wiki space list", runner.last.LegacyPath)
	}
}

func TestWikiNodeReadProxyExecutesDocRead(t *testing.T) {
	t.Parallel()

	runner := &wikiCommandRunner{}
	cmd := wikiHandler{}.Command(runner)
	var out, errOut bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	cmd.SetArgs([]string{"node", "read", "--node", "NODE_001", "--format", "json"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v\nstderr:\n%s", err, errOut.String())
	}
	if !strings.Contains(strings.ToLower(errOut.String()), "redirecting to") {
		t.Fatalf("stderr = %q, want redirecting hint", errOut.String())
	}
	if runner.last.CanonicalProduct != "doc" {
		t.Fatalf("product = %q, want doc", runner.last.CanonicalProduct)
	}
	if runner.last.Tool != "get_document_content" {
		t.Fatalf("tool = %q, want get_document_content", runner.last.Tool)
	}
	if got := runner.last.Params["nodeId"]; got != "NODE_001" {
		t.Fatalf("nodeId = %#v, want NODE_001", got)
	}
}

func TestWikiFileSearchProxyRenamesWorkspace(t *testing.T) {
	t.Parallel()

	runner := &wikiCommandRunner{}
	cmd := wikiHandler{}.Command(runner)
	var out, errOut bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	cmd.SetArgs([]string{
		"file", "search",
		"--query", "测试",
		"--workspace", "WS_001",
		"--format", "json",
	})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v\nstderr:\n%s", err, errOut.String())
	}
	if runner.last.CanonicalProduct != "doc" {
		t.Fatalf("product = %q, want doc", runner.last.CanonicalProduct)
	}
	if runner.last.Tool != "search_documents" {
		t.Fatalf("tool = %q, want search_documents", runner.last.Tool)
	}
	if got := runner.last.Params["keyword"]; got != "测试" {
		t.Fatalf("keyword = %#v, want 测试", got)
	}
	if got := runner.last.Params["workspaceIds"]; !reflect.DeepEqual(got, []string{"WS_001"}) {
		t.Fatalf("workspaceIds = %#v, want []string{WS_001}", got)
	}
}

func TestWikiProxyUnknownSubcommandsDoNotPanic(t *testing.T) {
	t.Parallel()

	cases := [][]string{
		{"node", "delete", "--node", "FAKE"},
		{"file", "read", "--node", "FAKE"},
	}
	for _, args := range cases {
		args := args
		t.Run(strings.Join(args[:2], " "), func(t *testing.T) {
			t.Parallel()

			runner := &wikiCommandRunner{}
			cmd := wikiHandler{}.Command(runner)
			var out, errOut bytes.Buffer
			cmd.SetOut(&out)
			cmd.SetErr(&errOut)
			cmd.SetArgs(args)

			err := cmd.Execute()
			if err == nil {
				t.Fatalf("Execute() error = nil, want unknown subcommand")
			}
			combined := strings.ToLower(out.String() + errOut.String() + err.Error())
			if strings.Contains(combined, "panic") || strings.Contains(combined, "runtime error") {
				t.Fatalf("unexpected panic output: %s", combined)
			}
		})
	}
}
