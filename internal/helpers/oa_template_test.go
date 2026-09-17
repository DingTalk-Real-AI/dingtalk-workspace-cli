package helpers

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestCrossPlatformCoverageOATemplateCommands(t *testing.T) {
	for _, tc := range []struct {
		command, tool, result string
		args                  []string
		want                  map[string]any
	}{
		{"list", "list_manage_templates", `[{"processCode":"PROC-1","flowTitle":"请假"}]`, nil, map[string]any{}},
		{"detail", "get_template_detail", `[{"processCode":"PROC-1","schemaContent":"{\"items\":[]}","processConfig":"{\"type\":\"start\"}"}]`, []string{"--process-code", " PROC-1 "}, map[string]any{"processCodes": []string{"PROC-1"}}},
	} {
		t.Run(tc.command, func(t *testing.T) {
			caller := &scriptedToolCaller{format: "json", steps: []scriptedToolStep{{text: `{"success":true,"dingOpenErrcode":0,"result":` + tc.result + `}`}}}
			stdout, err := executeOAAttachmentCommandCapturingOutput(t, caller, append([]string{"approval", "template", tc.command}, tc.args...)...)
			if err != nil {
				t.Fatal(err)
			}
			if caller.calls != 1 || caller.server != "oa" || caller.tool != tc.tool || !reflect.DeepEqual(caller.args, tc.want) {
				t.Fatalf("call: %s/%s %#v (%d)", caller.server, caller.tool, caller.args, caller.calls)
			}
			var got map[string]any
			if err := json.Unmarshal([]byte(stdout), &got); err != nil {
				t.Fatal(err)
			}
			var want any
			json.Unmarshal([]byte(tc.result), &want)
			if got["ok"] != true || !reflect.DeepEqual(got["data"], map[string]any{"templates": want}) {
				t.Fatalf("output: %s", stdout)
			}
		})
		for _, response := range []string{`{"success":false,"dingOpenErrcode":830001,"result":{}}`, `{"success":true,"result":{}}`, `{"result":[]}`} {
			t.Run(tc.command+"/failure/"+response, func(t *testing.T) {
				caller := &scriptedToolCaller{format: "json", steps: []scriptedToolStep{{text: response}}}
				_, err := executeOAAttachmentCommandCapturingOutput(t, caller, append([]string{"approval", "template", tc.command}, tc.args...)...)
				if err == nil {
					t.Fatal("expected failure")
				}
				if strings.Contains(response, "830001") && !strings.Contains(err.Error(), "830001") {
					t.Fatalf("business error code lost: %v", err)
				}
			})
		}
	}
}

func TestCrossPlatformCoverageOATemplateDetailRequiresOneCode(t *testing.T) {
	for _, args := range [][]string{nil, {"--template-code", "PROC-1"}, {"--process-code", " "}, {"--process-codes", "PROC-1,PROC-2"}, {"--process-code", "PROC-1", "PROC-2"}} {
		caller := &scriptedToolCaller{format: "json"}
		if _, err := executeOAAttachmentCommandCapturingOutput(t, caller, append([]string{"approval", "template", "detail"}, args...)...); err == nil {
			t.Fatal("expected validation failure")
		}
		if caller.calls != 0 {
			t.Fatal("invalid input called MCP")
		}
	}
}

func TestCrossPlatformCoverageOATemplateResponseValidation(t *testing.T) {
	for _, tc := range []struct {
		response string
		fail     bool
	}{
		{response: `{"success":true,"result":[]}`},
		{response: `{"success":true,"result":[null]}`, fail: true},
		{response: `{"success":true,"result":[{},{}]}`, fail: true},
		{response: `{"success":true,"dingOpenErrcode":830001,"result":[]}`, fail: true},
		{response: `[]`, fail: true},
	} {
		t.Run(tc.response, func(t *testing.T) {
			caller := &scriptedToolCaller{format: "json", steps: []scriptedToolStep{{text: tc.response}}}
			_, err := executeOAAttachmentCommandCapturingOutput(t, caller, "approval", "template", "detail", "--process-code", "PROC-1")
			if (err != nil) != tc.fail {
				t.Fatalf("error = %v, want failure %v", err, tc.fail)
			}
		})
	}
}

func TestCrossPlatformCoverageOATemplateGroupHelp(t *testing.T) {
	caller := &scriptedToolCaller{format: "json"}
	stdout, err := executeOAAttachmentCommandCapturingOutput(t, caller, "approval", "template", "--help")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"审批模板管理", "list", "detail"} {
		if !strings.Contains(stdout, want) {
			t.Fatalf("group help missing %q: %s", want, stdout)
		}
	}
	if caller.calls != 0 {
		t.Fatal("group help called MCP")
	}
	root := newOaCommand()
	group, _, err := root.Find([]string{"approval", "template"})
	if err != nil || group.Name() != "template" {
		t.Fatalf("template group missing: %v", err)
	}
	children := map[string]bool{}
	for _, child := range group.Commands() {
		children[child.Name()] = true
	}
	if !reflect.DeepEqual(children, map[string]bool{"list": true, "detail": true}) {
		t.Fatalf("template leaves = %#v", children)
	}
}

func TestCrossPlatformCoverageOAApprovalHelpIncludesTemplate(t *testing.T) {
	caller := &scriptedToolCaller{format: "json"}
	stdout, err := executeOAAttachmentCommandCapturingOutput(t, caller, "approval", "--help")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout, "template") || !strings.Contains(stdout, "审批模板管理") {
		t.Fatalf("approval help does not expose template management: %s", stdout)
	}
	if caller.calls != 0 {
		t.Fatal("approval help called MCP")
	}
}
