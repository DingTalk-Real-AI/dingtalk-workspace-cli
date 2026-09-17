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
	"io"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contractfinal"
)

func executeMarkdownCommentCommand(t *testing.T, caller *docCommentMutationCaller, args ...string) error {
	t.Helper()
	previousDeps := deps
	previousArgs := os.Args
	t.Cleanup(func() {
		deps = previousDeps
		os.Args = previousArgs
	})

	InitDeps(caller)
	deps.Out.w = io.Discard
	os.Args = []string{"dws", "markdown"}
	cmd := newMarkdownCommand()
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
	cmd.SetArgs(args)
	return cmd.Execute()
}

func TestCrossPlatformCoverageMarkdownCommentExposesListOnly(t *testing.T) {
	group := newMarkdownCommentCmd()
	if len(group.Commands()) != 1 || group.Commands()[0].Name() != "list" {
		t.Fatalf("markdown comment commands = %#v, want list only", group.Commands())
	}
	for _, unsupported := range []string{
		"create", "reply", "update", "delete", "batch-query",
		"list-replies", "resolve", "restore", "react-reply",
	} {
		if cmd, _, err := group.Find([]string{unsupported}); err == nil && cmd != group {
			t.Fatalf("markdown comment unexpectedly exposes %q", unsupported)
		}
	}

	listCmd, remaining, err := group.Find([]string{"list"})
	if err != nil || len(remaining) != 0 {
		t.Fatalf("markdown comment list lookup: cmd=%v remaining=%v err=%v", listCmd, remaining, err)
	}
	final, ok := contractfinal.RuntimeContractFinal(listCmd)
	if !ok || final.Identity == nil || final.Interface == nil || final.Interface.Ref == nil {
		t.Fatalf("markdown comment list incomplete ContractFinal: %#v", final)
	}
	if final.Identity.ProductID != "markdown" || final.Identity.PrimaryCLIPath != "markdown comment list" {
		t.Fatalf("markdown comment list identity = %#v", final.Identity)
	}
	if final.Interface.Ref.ProductID != commentServer || final.Interface.Ref.RPCName != "list_comments" {
		t.Fatalf("markdown comment list interface = %#v", final.Interface.Ref)
	}
	if final.Selection == nil ||
		!strings.Contains(strings.Join(final.Selection.AvoidWhen, "\n"), "drive comment list-v2") {
		t.Fatalf("markdown comment routing does not send ordinary files to Drive list-v2: %#v", final.Selection)
	}
}

func TestCrossPlatformCoverageMarkdownCommentListUsesDocCommentRPCAndInlineFilter(t *testing.T) {
	caller := &docCommentMutationCaller{}
	err := executeMarkdownCommentCommand(t, caller,
		"comment", "list", "--node", "markdown-1", "--limit", "20",
		"--cursor", "opaque-2", "--type", "inline", "--resolve-status", "unresolved")
	if err != nil {
		t.Fatal(err)
	}
	if len(caller.calls) != 1 {
		t.Fatalf("calls = %#v", caller.calls)
	}
	want := map[string]any{
		"nodeId": "markdown-1", "pageSize": 20, "nextToken": "opaque-2",
		"resolveStatus": "unresolved", "commentType": "inline",
	}
	call := caller.calls[0]
	if call.productID != commentServer || call.toolName != "list_comments" || !reflect.DeepEqual(call.args, want) {
		t.Fatalf("call = %#v, want server=%q tool=list_comments args=%#v", call, commentServer, want)
	}
}

func TestCrossPlatformCoverageMarkdownCommentListRejectsInvalidLimit(t *testing.T) {
	caller := &docCommentMutationCaller{}
	err := executeMarkdownCommentCommand(t, caller,
		"comment", "list", "--node", "markdown-1", "--limit", "51")
	if err == nil || !strings.Contains(err.Error(), "1-50") {
		t.Fatalf("invalid limit error = %v", err)
	}
	if len(caller.calls) != 0 {
		t.Fatalf("invalid input reached MCP: %#v", caller.calls)
	}
}
