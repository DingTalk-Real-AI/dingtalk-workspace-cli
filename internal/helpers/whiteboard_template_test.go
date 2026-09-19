package helpers

import (
	"context"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contractfinal"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
	whiteboardcore "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/whiteboard"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
)

func prepareWhiteboardTemplateTestCommand() *cobra.Command {
	cmd := newWhiteboardCommand()
	cmd.PersistentFlags().Bool("yes", false, "")
	cmd.PersistentFlags().Bool("dry-run", false, "")
	ctx, _ := output.WithResultStore(context.Background())
	cmd.SetContext(ctx)
	return cmd
}

func TestCrossPlatformCoverageWhiteboardTemplateSaveDestination(t *testing.T) {
	caller := &whiteboardTestCaller{format: "json", dry: true, response: func(call whiteboardTestCall, _ int) string {
		if call.args["templateWorkspaceId"] != "target-ws" {
			t.Fatalf("destination = %#v", call.args)
		}
		return `{"success":true,"result":{"executed":false,"dryRun":true,"scope":"team","templateWorkspaceId":"target-ws","resourceType":9}}`
	}}
	installWhiteboardTestCaller(t, caller)
	cmd := prepareWhiteboardTemplateTestCommand()
	cmd.SetArgs([]string{"template", "team", "save", "--node", "wb", "--name", "n", "--request-id", "destination-1", "--template-workspace", " target-ws ", "--dry-run"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	result := map[string]any{"templateId": "tpl", "requestId": "r1", "scope": "team", "resourceType": 9, "templateWorkspaceId": "wrong-ws"}
	args := map[string]any{"requestId": "r1", "templateWorkspaceId": "target-ws"}
	if err := validateWhiteboardTemplateSaveResult(result, args, whiteboardcore.TeamTemplateSaveTool); err == nil {
		t.Fatal("accepted mismatched destination")
	}
	result["templateWorkspaceId"] = "target-ws"
	if err := validateWhiteboardTemplateSaveResult(result, args, whiteboardcore.TeamTemplateSaveTool); err != nil {
		t.Fatal(err)
	}
}

func TestCrossPlatformCoverageWhiteboardTemplateSaveLanguage(t *testing.T) {
	for _, scope := range []string{"personal", "team"} {
		for _, tc := range []struct {
			args []string
			want string
		}{
			{nil, "zh_CN"}, {[]string{"--language", " en_US "}, "en_US"}, {[]string{"--language", " "}, "zh_CN"},
		} {
			caller := &whiteboardTestCaller{format: "json", dry: true, response: func(call whiteboardTestCall, _ int) string {
				if call.args["language"] != tc.want {
					t.Fatalf("language = %#v, want %q", call.args["language"], tc.want)
				}
				return `{"success":true,"result":{"executed":false,"dryRun":true,"scope":"` + scope + `","templateWorkspaceId":"ws","resourceType":9}}`
			}}
			installWhiteboardTestCaller(t, caller)
			cmd := prepareWhiteboardTemplateTestCommand()
			args := []string{"template", scope, "save", "--node", "wb", "--name", "n", "--request-id", "language-1", "--dry-run"}
			cmd.SetArgs(append(args, tc.args...))
			if err := cmd.Execute(); err != nil {
				t.Fatal(err)
			}
			if len(caller.calls) != 1 {
				t.Fatalf("calls = %d", len(caller.calls))
			}
		}
	}
}

func TestCrossPlatformCoverageWhiteboardTemplateUnknownCommitDoesNotRetry(t *testing.T) {
	for _, scope := range []string{"personal", "team"} {
		t.Run(scope, func(t *testing.T) {
			caller := &whiteboardTestCaller{format: "json", response: func(whiteboardTestCall, int) string {
				return `{"success":false,"errorCode":"WHITEBOARD_TEMPLATE_IDEMPOTENCY_RESULT_UNKNOWN","errorMsg":"The template-center commit result is unknown.","result":{"logId":"trace-save-unknown"}}`
			}}
			installWhiteboardTestCaller(t, caller)
			cmd := prepareWhiteboardTemplateTestCommand()
			cmd.SetArgs([]string{"template", scope, "save", "--node", "wb", "--name", "n", "--request-id", "save-unknown", "--yes"})
			err := cmd.Execute()
			if err == nil {
				t.Fatal("unknown commit unexpectedly succeeded")
			}
			for _, want := range []string{"WHITEBOARD_TEMPLATE_IDEMPOTENCY_RESULT_UNKNOWN", "trace-save-unknown", "不要自动重试", "模板列表为空"} {
				if !strings.Contains(err.Error(), want) {
					t.Fatalf("error missing %q: %v", want, err)
				}
			}
			if len(caller.calls) != 1 || caller.calls[0].args["requestId"] != "save-unknown" {
				t.Fatalf("request retried or changed: %#v", caller.calls)
			}
		})
	}
}

func TestCrossPlatformCoverageWhiteboardTemplateCommandSurface(t *testing.T) {
	root := prepareWhiteboardTemplateTestCommand()
	paths := [][]string{
		{"template", "personal", "save"}, {"template", "personal", "list"}, {"template", "personal", "create"},
		{"template", "team", "save"}, {"template", "team", "list"}, {"template", "team", "create"},
		{"template", "public", "list"}, {"template", "public", "create"},
	}
	for _, path := range paths {
		leaf, _, err := root.Find(path)
		if err != nil || leaf == nil || !leaf.Runnable() {
			t.Fatalf("find %v: leaf=%v err=%v", path, leaf, err)
		}
		if _, ok := contractfinal.RuntimeContractFinal(leaf); !ok {
			t.Fatalf("%v missing ContractFinal", path)
		}
		if leaf.Flags().Lookup("type") != nil || leaf.Flags().Lookup("part-id") != nil {
			t.Fatalf("%v exposes type or part-id", path)
		}
		workspaceFlag := leaf.Flags().Lookup("template-workspace")
		wantWorkspace := path[1] == "team"
		if (workspaceFlag != nil) != wantWorkspace {
			t.Fatalf("%v template-workspace present=%v, want %v", path, workspaceFlag != nil, wantWorkspace)
		}
	}
	publicGroup, _, err := root.Find([]string{"template", "public"})
	if err != nil {
		t.Fatal(err)
	}
	for _, child := range publicGroup.Commands() {
		if child.Name() == "save" {
			t.Fatal("public template scope must not expose save")
		}
	}
}

func TestCrossPlatformCoverageWhiteboardPublicTemplateListAndCreate(t *testing.T) {
	caller := &whiteboardTestCaller{format: "json", response: func(call whiteboardTestCall, _ int) string {
		switch call.tool {
		case whiteboardcore.PublicTemplateListTool:
			if call.args["query"] != "复盘" {
				t.Fatalf("public list args=%#v", call.args)
			}
			if _, exists := call.args["templateWorkspaceId"]; exists {
				t.Fatalf("public list leaked template workspace: %#v", call.args)
			}
			return `{"success":true,"result":{"templates":[{"templateId":"public-tpl","resourceType":9,"scope":"public"}],"hasMore":false}}`
		case whiteboardcore.PublicTemplateCreateTool:
			if _, exists := call.args["templateWorkspaceId"]; exists {
				t.Fatalf("public create leaked template workspace: %#v", call.args)
			}
			return `{"success":true,"result":{"requestId":"public-create-1","nodeId":"wb-public","revision":1,"contentType":"WBD","requestMatched":true,"templateId":"public-tpl","templateScope":"public","resourceType":9,"verified":true}}`
		default:
			t.Fatalf("unexpected tool %s", call.tool)
			return ""
		}
	}}
	installWhiteboardTestCaller(t, caller)

	list := prepareWhiteboardTemplateTestCommand()
	list.SetArgs([]string{"template", "public", "list", "--query", " 复盘 "})
	if err := list.Execute(); err != nil {
		t.Fatal(err)
	}
	create := prepareWhiteboardTemplateTestCommand()
	create.SetArgs([]string{"template", "public", "create", "--template-id", "public-tpl", "--request-id", "public-create-1"})
	if err := create.Execute(); err != nil {
		t.Fatal(err)
	}
	if len(caller.calls) != 2 {
		t.Fatalf("calls=%#v", caller.calls)
	}
}

func TestCrossPlatformCoverageWhiteboardTemplateSaveRoutesScopesAndValidatesReceipt(t *testing.T) {
	caller := &whiteboardTestCaller{format: "json", response: func(call whiteboardTestCall, _ int) string {
		scope := "personal"
		workspace := ""
		if call.tool == whiteboardcore.TeamTemplateSaveTool {
			scope = "team"
			workspace = `,"templateWorkspaceId":"team-ws"`
		}
		return `{"success":true,"result":{"templateId":"tpl-1","name":"复盘","scope":"` + scope +
			`","resourceType":9,"sourceNodeId":"wb-1","requestId":"save-1","idempotentReplay":false` + workspace + `}}`
	}}
	installWhiteboardTestCaller(t, caller)
	for _, scope := range []string{"personal", "team"} {
		cmd := prepareWhiteboardTemplateTestCommand()
		cmd.SetArgs([]string{"template", scope, "save", "--node", "wb-1", "--name", "复盘", "--request-id", "save-1", "--yes"})
		if err := cmd.Execute(); err != nil {
			t.Fatalf("%s save: %v", scope, err)
		}
		call := caller.calls[len(caller.calls)-1]
		if call.args["sourceNodeId"] != "wb-1" || call.args["requestId"] != "save-1" {
			t.Fatalf("%s args=%#v", scope, call.args)
		}
		if _, exists := call.args["type"]; exists {
			t.Fatalf("%s args exposed type: %#v", scope, call.args)
		}
	}
	if caller.calls[0].tool != whiteboardcore.PersonalTemplateSaveTool || caller.calls[1].tool != whiteboardcore.TeamTemplateSaveTool {
		t.Fatalf("calls=%#v", caller.calls)
	}
}

func TestCrossPlatformCoverageWhiteboardTemplateSaveRequiresConfirmationAndRemoteDryRun(t *testing.T) {
	caller := &whiteboardTestCaller{format: "json"}
	installWhiteboardTestCaller(t, caller)
	cmd := prepareWhiteboardTemplateTestCommand()
	cmd.SetIn(strings.NewReader("no\n"))
	cmd.SetArgs([]string{"template", "personal", "save", "--node", "wb", "--name", "n", "--request-id", "save-1"})
	if err := cmd.Execute(); err == nil {
		t.Fatal("save without confirmation unexpectedly succeeded")
	}
	if len(caller.calls) != 0 {
		t.Fatalf("remote call before confirmation: %#v", caller.calls)
	}

	caller = &whiteboardTestCaller{dry: true, format: "json", response: func(call whiteboardTestCall, _ int) string {
		return `{"success":true,"result":{"executed":false,"dryRun":true,"scope":"team","templateWorkspaceId":"ws","sourceNodeId":"wb","resourceType":9}}`
	}}
	installWhiteboardTestCaller(t, caller)
	cmd = prepareWhiteboardTemplateTestCommand()
	cmd.SetArgs([]string{"template", "team", "save", "--node", "wb", "--name", "n", "--request-id", "save-1", "--dry-run"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if len(caller.calls) != 1 || caller.calls[0].args["dryRun"] != true {
		t.Fatalf("dry-run calls=%#v", caller.calls)
	}
}

func TestCrossPlatformCoverageWhiteboardTemplateListPaginationAndScope(t *testing.T) {
	caller := &whiteboardTestCaller{format: "json", response: func(call whiteboardTestCall, index int) string {
		if call.tool != whiteboardcore.TeamTemplateListTool || call.args["templateWorkspaceId"] != "ws-1" {
			t.Fatalf("call=%#v", call)
		}
		if index == 0 {
			return `{"success":true,"result":{"templates":[{"templateId":"t1","resourceType":9,"scope":"team","templateWorkspaceId":"ws-1"}],"nextCursor":"2","hasMore":true}}`
		}
		if call.args["nextCursor"] != "2" {
			t.Fatalf("second cursor args=%#v", call.args)
		}
		return `{"success":true,"result":{"templates":[{"templateId":"t2","resourceType":9,"scope":"team","templateWorkspaceId":"ws-1"}],"hasMore":false}}`
	}}
	installWhiteboardTestCaller(t, caller)
	cmd := prepareWhiteboardTemplateTestCommand()
	cmd.SetArgs([]string{"template", "team", "list", "--template-workspace", "ws-1", "--query", "复盘", "--page-all", "--max-pages", "2"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if len(caller.calls) != 2 {
		t.Fatalf("calls=%#v", caller.calls)
	}
}

func TestCrossPlatformCoverageWhiteboardTemplateRejectsInvalidPagingAndType(t *testing.T) {
	caller := &whiteboardTestCaller{format: "json", response: func(whiteboardTestCall, int) string {
		return `{"success":true,"result":{"templates":[{"templateId":"bad","resourceType":1}],"hasMore":false}}`
	}}
	installWhiteboardTestCaller(t, caller)
	for _, args := range [][]string{
		{"template", "personal", "list", "--limit", "51"},
		{"template", "personal", "list", "--cursor", "bad"},
		{"template", "personal", "list", "--max-pages", "2"},
		{"template", "personal", "list"},
	} {
		cmd := prepareWhiteboardTemplateTestCommand()
		cmd.SetArgs(args)
		if err := cmd.Execute(); err == nil {
			t.Fatalf("args %v unexpectedly succeeded", args)
		}
	}
}

func TestCrossPlatformCoverageWhiteboardTemplateCreateValidatesTemplateReceipt(t *testing.T) {
	caller := &whiteboardTestCaller{format: "json", response: func(call whiteboardTestCall, _ int) string {
		if call.tool != whiteboardcore.TeamTemplateCreateTool {
			t.Fatalf("tool=%s", call.tool)
		}
		return `{"success":true,"result":{"requestId":"create-1","nodeId":"wb-new","revision":1,"contentType":"WBD","requestMatched":true,"templateId":"tpl-1","templateScope":"team","templateWorkspaceId":"ws-1","resourceType":9,"verified":true}}`
	}}
	installWhiteboardTestCaller(t, caller)
	cmd := prepareWhiteboardTemplateTestCommand()
	cmd.SetArgs([]string{"template", "team", "create", "--template-workspace", "ws-1", "--template-id", "tpl-1", "--request-id", "create-1"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}

	caller.response = func(whiteboardTestCall, int) string {
		return `{"success":true,"result":{"requestId":"create-1","nodeId":"wb-new","revision":1,"templateId":"tpl-1","templateScope":"personal","resourceType":9}}`
	}
	cmd = prepareWhiteboardTemplateTestCommand()
	cmd.SetArgs([]string{"template", "team", "create", "--template-workspace", "ws-1", "--template-id", "tpl-1", "--request-id", "create-1"})
	if err := cmd.Execute(); err == nil {
		t.Fatal("cross-scope create receipt unexpectedly succeeded")
	}
}

func (c *whiteboardTestCaller) CallWhiteboardTemplatePreview(ctx context.Context, tool string, args map[string]any) (*edition.ToolResult, error) {
	if err := whiteboardcore.ValidateTemplatePreviewCall(whiteboardcore.ServerID, tool, args); err != nil {
		return nil, err
	}
	return c.CallTool(ctx, whiteboardcore.ServerID, tool, args)
}

func TestCrossPlatformCoverageWhiteboardTemplatePreviewRejectsInvalidReceipts(t *testing.T) {
	for _, response := range []string{
		`{"dry_run":true,"request":{}}`,
		`{"executed":true,"scope":"personal","resourceType":9}`,
		`{"executed":false,"scope":"team","resourceType":9}`,
		`{"executed":false,"scope":"personal","resourceType":1}`,
	} {
		t.Run(response, func(t *testing.T) {
			caller := &whiteboardTestCaller{dry: true, format: "json", response: func(whiteboardTestCall, int) string { return response }}
			installWhiteboardTestCaller(t, caller)
			cmd := prepareWhiteboardTemplateTestCommand()
			cmd.SetArgs([]string{"template", "personal", "save", "--node", "wb", "--name", "n", "--request-id", "preflight-invalid", "--dry-run"})
			if err := cmd.Execute(); err == nil {
				t.Fatal("invalid receipt accepted")
			}
			if len(caller.calls) != 1 || caller.calls[0].args["dryRun"] != true {
				t.Fatal("unexpected retry/write", caller.calls)
			}
		})
	}
}

func TestCrossPlatformCoverageWhiteboardTemplateListPreservesQueryAcrossPages(t *testing.T) {
	for _, scope := range []string{"personal", "team"} {
		for _, query := range []struct {
			name  string
			flags []string
			want  string
		}{
			{"omitted", nil, ""}, {"empty", []string{"--query", ""}, ""},
			{"whitespace", []string{"--query", "  "}, ""}, {"keyword", []string{"--query", "  课表  "}, "课表"},
		} {
			t.Run(scope+"/"+query.name, func(t *testing.T) {
				caller := &whiteboardTestCaller{format: "json", response: func(call whiteboardTestCall, index int) string {
					got, present := call.args["query"]
					if !present || got != query.want {
						t.Fatalf("page %d query = %#v, present=%v", index, got, present)
					}
					if scope == "team" && call.args["templateWorkspaceId"] != "ws" {
						t.Fatal(call.args)
					}
					if index == 0 {
						return `{"templates":[],"hasMore":true,"nextCursor":"20"}`
					}
					if call.args["nextCursor"] != "20" {
						t.Fatal(call.args)
					}
					return `{"templates":[],"hasMore":false}`
				}}
				installWhiteboardTestCaller(t, caller)
				cmd := prepareWhiteboardTemplateTestCommand()
				args := []string{"template", scope, "list", "--page-all"}
				if scope == "team" {
					args = append(args, "--template-workspace", "ws")
				}
				cmd.SetArgs(append(args, query.flags...))
				if err := cmd.Execute(); err != nil {
					t.Fatal(err)
				}
				if len(caller.calls) != 2 {
					t.Fatal(caller.calls)
				}
			})
		}
	}
}
