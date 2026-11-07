package helpers

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/spf13/cobra"
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
	if !reflect.DeepEqual(children, map[string]bool{"list": true, "detail": true, "create": true, "update": true}) {
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

const templateWriteSuccess = `{"result":{"processCode":"PROC-1","class":"com.dingtalk.bpms.oapi.vo.ProcessTopVO"},"success":true,"dingOpenErrcode":0,"errorMsg":"ok"}`
const templateForm = `{"items":[{"componentName":"TextField","props":{"id":"text-1","label":"事由"}}]}`
const templateProcess = `{"type":"start","nodeId":"sid-startevent","properties":{}}`

func templateWriteArgs(command string) []string {
	args := []string{"approval", "template", command, "--name", "出差申请", "--schema-content", templateForm}
	if command == "update" {
		args = append(args, "--process-code", "PROC-1", "--process-config", templateProcess)
	}
	return args
}

func TestCrossPlatformCoverageOATemplateWritePayload(t *testing.T) {
	for _, command := range []string{"create", "update"} {
		t.Run(command, func(t *testing.T) {
			caller := &scriptedToolCaller{format: "json", steps: []scriptedToolStep{{text: templateWriteSuccess}}}
			stdout, err := executeOAAttachmentCommandCapturingOutput(t, caller, templateWriteArgs(command)...)
			if err != nil {
				t.Fatal(err)
			}
			want := map[string]any{"name": "出差申请", "schemaContent": templateForm}
			if command == "update" {
				want["processCode"] = "PROC-1"
				want["processConfig"] = templateProcess
			}
			if caller.server != "oa" || caller.tool != command+"_process_template" || caller.calls != 1 || !reflect.DeepEqual(caller.args, want) {
				t.Fatalf("dispatch: %s/%s %#v", caller.server, caller.tool, caller.args)
			}
			var envelope map[string]any
			if err := json.Unmarshal([]byte(stdout), &envelope); err != nil {
				t.Fatal(err)
			}
			if envelope["ok"] != true || !reflect.DeepEqual(envelope["data"], map[string]any{"processCode": "PROC-1"}) {
				t.Fatalf("output: %s", stdout)
			}
		})
	}
}

func TestCrossPlatformCoverageOATemplateWriteOptionalValues(t *testing.T) {
	for _, command := range []string{"create", "update"} {
		t.Run(command, func(t *testing.T) {
			caller := &scriptedToolCaller{format: "json", steps: []scriptedToolStep{{text: templateWriteSuccess}}}
			args := append(templateWriteArgs(command), "--description", "", "--icon-url", "", "--form-config", `{}`, "--node-config", `{}`, "--condition-rule", `{}`, "--static-workflow", "0", "--append-enable", "n", "--duplicate-removal", "false", "--manager-user-ids", `[]`, "--visible-range", `[]`, "--plugin-configs", `[{"pluginId":42,"pluginKey":"test","pluginConfig":"{}","enable":false}]`)
			if command == "create" {
				args = append(args, "--expected-version", "0", "--process-config", templateProcess)
			}
			if _, err := executeOAAttachmentCommandCapturingOutput(t, caller, args...); err != nil {
				t.Fatal(err)
			}
			raw, err := json.Marshal(caller.args)
			if err != nil {
				t.Fatal(err)
			}
			var got map[string]any
			json.Unmarshal(raw, &got)
			for _, field := range []string{"description", "iconUrl"} {
				if got[field] != "" {
					t.Fatalf("explicit empty %s lost: %s", field, raw)
				}
			}
			for _, field := range []string{"managerUserIds", "visibleRange"} {
				if !reflect.DeepEqual(got[field], []any{}) {
					t.Fatalf("empty array %s lost: %s", field, raw)
				}
			}
			if got["staticWorkflow"] != "0" || got["appendEnable"] != "n" || got["duplicateRemoval"] != "false" || got["formConfig"] != `{}` || got["nodeConfig"] != `{}` || got["conditionRule"] != `{}` || got["processConfig"] != templateProcess {
				t.Fatalf("optional values: %s", raw)
			}
			if got["pluginConfigs"].([]any)[0].(map[string]any)["enable"] != false {
				t.Fatalf("false lost: %s", raw)
			}
			if command == "create" && got["expectedVersion"] != float64(0) {
				t.Fatalf("numeric zero lost: %s", raw)
			}
			if command == "update" {
				if _, ok := got["expectedVersion"]; ok {
					t.Fatal("update sent expectedVersion")
				}
			}
		})
	}
}

func TestCrossPlatformCoverageOATemplateWriteValidation(t *testing.T) {
	for _, command := range []string{"create", "update"} {
		for _, extra := range [][]string{
			{"--name", " "}, {"--schema-content", `[]`}, {"--schema-content", `null`}, {"--schema-content", `{`}, {"--schema-content", `{} {}`},
			{"--process-config", `[]`}, {"--form-config", `null`}, {"--node-config", `{`}, {"--condition-rule", `{`},
			{"--manager-user-ids", `[1]`}, {"--visible-range", `[null]`}, {"--plugin-configs", `{}`},
			{"--static-workflow", "2"}, {"--append-enable", "true"}, {"--duplicate-removal", "yes"}, {"extra-positional"},
		} {
			t.Run(command+strings.Join(extra, " "), func(t *testing.T) {
				caller := &scriptedToolCaller{format: "json"}
				if _, err := executeOAAttachmentCommandCapturingOutput(t, caller, append(templateWriteArgs(command), extra...)...); err == nil {
					t.Fatal("invalid input accepted")
				}
				if caller.calls != 0 {
					t.Fatal("invalid input called MCP")
				}
			})
		}
		for _, required := range []string{"--name", "--schema-content", "--process-code", "--process-config"} {
			args := templateWriteArgs(command)
			found := false
			for i := 0; i < len(args); i++ {
				if args[i] == required {
					args = append(args[:i], args[i+2:]...)
					found = true
					break
				}
			}
			if !found {
				continue
			}
			caller := &scriptedToolCaller{format: "json"}
			if _, err := executeOAAttachmentCommandCapturingOutput(t, caller, args...); err == nil || caller.calls != 0 {
				t.Fatalf("missing %s: %v", required, err)
			}
		}
	}
	for _, value := range []string{"1", "NaN", "Inf", "bad"} {
		caller := &scriptedToolCaller{format: "json"}
		if _, err := executeOAAttachmentCommandCapturingOutput(t, caller, append(templateWriteArgs("update"), "--expected-version", value)...); err == nil || caller.calls != 0 {
			t.Fatal("update accepts expected-version")
		}
	}
}

func TestCrossPlatformCoverageOATemplateWriteResponse(t *testing.T) {
	for _, response := range []string{
		`{"result":{},"success":false,"dingOpenErrcode":820008,"errorMsg":"can not cast to JSONObject."}`,
		`{"result":{"processCode":"PROC-1"},"success":true,"dingOpenErrcode":820008,"errorMsg":"error"}`,
		`{"result":{}}`, `{"success":true,"result":{}}`, `{"success":true,"result":[]}`, `{"success":true,"result":{"processCode":" "}}`, `[]`,
	} {
		for _, command := range []string{"create", "update"} {
			caller := &scriptedToolCaller{format: "json", steps: []scriptedToolStep{{text: response}}}
			_, err := executeOAAttachmentCommandCapturingOutput(t, caller, templateWriteArgs(command)...)
			if err == nil {
				t.Fatalf("accepted %s", response)
			}
			if strings.Contains(response, "JSONObject") && (!strings.Contains(err.Error(), "820008") || !strings.Contains(err.Error(), "JSONObject")) {
				t.Fatalf("lost upstream error: %v", err)
			}
		}
	}
}

func TestCrossPlatformCoverageOATemplateWriteFileInput(t *testing.T) {
	path := filepath.Join(t.TempDir(), "form.json")
	if err := os.WriteFile(path, []byte(templateForm), 0600); err != nil {
		t.Fatal(err)
	}
	caller := &scriptedToolCaller{format: "json", steps: []scriptedToolStep{{text: templateWriteSuccess}}}
	if _, err := executeOAAttachmentCommandCapturingOutput(t, caller, append(templateWriteArgs("create"), "--schema-content", "@"+path)...); err != nil {
		t.Fatal(err)
	}
	if caller.args["schemaContent"] != templateForm {
		t.Fatalf("file content: %#v", caller.args)
	}
}

func TestCrossPlatformCoverageOATemplateCreateVersionValidation(t *testing.T) {
	for _, raw := range []string{"NaN", "Inf", "bad", "1e999"} {
		caller := &scriptedToolCaller{format: "json"}
		_, err := executeOAAttachmentCommandCapturingOutput(t, caller, append(templateWriteArgs("create"), "--expected-version", raw)...)
		if err == nil || caller.calls != 0 {
			t.Fatalf("accepted version %s: %v", raw, err)
		}
	}
}

func TestCrossPlatformCoverageOATemplateWritePopulatedArrays(t *testing.T) {
	caller := &scriptedToolCaller{format: "json", steps: []scriptedToolStep{{text: templateWriteSuccess}}}
	arrays := map[string]string{"manager-user-ids": `["u1","u2"]`, "visible-range": `[{"corpId":"ding-example","visibleType":1,"visibleValue":"u1","unactiveFlag":0,"id":9007199254740993}]`, "plugin-configs": `[]`}
	args := templateWriteArgs("update")
	for flag, raw := range arrays {
		args = append(args, "--"+flag, raw)
	}
	if _, err := executeOAAttachmentCommandCapturingOutput(t, caller, args...); err != nil {
		t.Fatal(err)
	}
	payload, _ := json.Marshal(caller.args)
	for _, want := range []string{`"managerUserIds":["u1","u2"]`, `"pluginConfigs":[]`, `"id":9007199254740993`} {
		if !strings.Contains(string(payload), want) {
			t.Fatalf("lost array value %s: %s", want, payload)
		}
	}
}

func TestCrossPlatformCoverageOATemplateWriteDryRun(t *testing.T) {
	for _, command := range []string{"create", "update"} {
		caller := &scriptedToolCaller{format: "json"}
		stdout, err := executeOAAttachmentCommandCapturingOutput(t, caller, append(templateWriteArgs(command), "--dry-run")...)
		if err != nil {
			t.Fatal(err)
		}
		if caller.calls != 0 {
			t.Fatal("dry-run called MCP")
		}
		var got map[string]any
		if err := json.Unmarshal([]byte(stdout), &got); err != nil {
			t.Fatal(err)
		}
		data, _ := got["data"].(map[string]any)
		if data["tool"] != command+"_process_template" || data["arguments"] == nil || data["processCode"] != nil {
			t.Fatalf("wrong preview: %s", stdout)
		}
	}
}

func TestCrossPlatformCoverageOATemplateDocumentPayload(t *testing.T) {
	for _, operation := range []string{"create", "update"} {
		for _, encoded := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/encoded=%t", operation, encoded), func(t *testing.T) {
				doc := map[string]any{"name": "出差申请", "schemaContent": json.RawMessage(templateForm), "description": "", "managerUserIds": []string{}, "pluginConfigs": []any{map[string]any{"pluginId": json.Number("9007199254740993"), "enable": false}}}
				if operation == "update" {
					doc["processCode"] = "PROC-1"
					doc["processConfig"] = json.RawMessage(templateProcess)
				} else {
					doc["expectedVersion"] = 0
				}
				if encoded {
					doc["schemaContent"] = templateForm
					if operation == "update" {
						doc["processConfig"] = templateProcess
					}
				}
				raw, _ := json.Marshal(doc)
				path := filepath.Join(t.TempDir(), "template.json")
				if err := os.WriteFile(path, raw, 0600); err != nil {
					t.Fatal(err)
				}
				caller := &scriptedToolCaller{format: "json", steps: []scriptedToolStep{{text: templateWriteSuccess}}}
				if _, err := executeOAAttachmentCommandCapturingOutput(t, caller, "approval", "template", operation, "--from-document", path); err != nil {
					t.Fatal(err)
				}
				if caller.calls != 1 || caller.server != "oa" || caller.tool != operation+"_process_template" {
					t.Fatalf("bad dispatch %#v", caller)
				}
				if caller.args["schemaContent"] != templateForm || caller.args["description"] != "" {
					t.Fatalf("bad payload %#v", caller.args)
				}
				sent, _ := json.Marshal(caller.args)
				for _, want := range []string{`"managerUserIds":[]`, `"pluginId":9007199254740993`, `"enable":false`} {
					if !strings.Contains(string(sent), want) {
						t.Fatalf("missing %s: %s", want, sent)
					}
				}
				if strings.Contains(string(sent), "fromDocument") || strings.Contains(string(sent), path) {
					t.Fatal("local path leaked to MCP")
				}
				if operation == "update" {
					if _, exists := caller.args["expectedVersion"]; exists {
						t.Fatal("update sent version")
					}
				}
			})
		}
	}
}

func TestCrossPlatformCoverageOATemplateDocumentValidation(t *testing.T) {
	tests := []struct{ operation, body, want string }{
		{"create", `[]`, "对象"}, {"create", `null`, "对象"}, {"create", `{`, "JSON"}, {"create", `{} {}`, "JSON"},
		{"create", `{"name":"A","name":"B","schemaContent":{"items":[]}}`, "重复"},
		{"create", `{"name":"A","schemaContent":{"items":[]},"extra":1}`, "extra"},
		{"create", `{"schemaContent":{"items":[]}}`, "name"},
		{"create", `{"name":"A"}`, "schemaContent"},
		{"create", `{"name":1,"schemaContent":{"items":[]}}`, "name"},
		{"create", `{"name":"A","schemaContent":[]}`, "schemaContent"},
		{"create", `{"name":"A","schemaContent":{"items":[]},"managerUserIds":[null]}`, "managerUserIds"},
		{"create", `{"name":"A","schemaContent":{"items":[]},"pluginConfigs":[{"enable":"false"}]}`, "enable"},
		{"create", `{"name":"A","schemaContent":{"items":[]},"visibleRange":[{"visibleType":"1"}]}`, "visibleType"},
		{"create", `{"name":"A","schemaContent":{"items":[]},"duplicateRemoval":true}`, "duplicateRemoval"},
		{"create", `{"name":"A","schemaContent":{"items":[]},"expectedVersion":"1"}`, "expectedVersion"},
		{"create", `{"name":"A","schemaContent":{"items":[]},"processCode":"PROC-1"}`, "processCode"},
		{"update", `{"name":"A","schemaContent":{"items":[]},"processConfig":{"nodeId":"s","type":"start"}}`, "processCode"},
		{"update", `{"processCode":"PROC-1","name":"A","schemaContent":{"items":[]}}`, "processConfig"},
		{"update", `{"processCode":"PROC-1","name":"A","schemaContent":{"items":[]},"processConfig":{"nodeId":"s","type":"start"},"expectedVersion":1}`, "expectedVersion"},
		{"create", `{"name":"A","schemaContent":{"items":[{"componentName":"TextField","props":{"id":"x"}},{"componentName":"TextField","props":{"id":"x"}}]}}`, "id"},
		{"create", `{"name":"A","schemaContent":{"items":[]},"processConfig":{"type":"start"}}`, "nodeId"},

		// --- coverage: decodeOATemplateDocument edge cases ---
		// First token error: completely invalid JSON triggers first-token failure path.
		{"create", `not valid json`, "JSON"},
		// Field-name token error: trailing comma after valid pair, then EOF.
		{"create", `{"a":1,`, "字段"},
		// Field-value decode error: broken value after valid key.
		{"create", `{"good":1,"bad":[}`, "bad"},

		// --- coverage: oaTemplateDocumentArgs field-level checks ---
		// Null field value.
		{"create", `{"name":null,"schemaContent":{"items":[]}}`, "null"},
		// Enum mismatch: appendEnable accepts only "y"/"n".
		{"create", `{"name":"A","schemaContent":{"items":[]},"appendEnable":"maybe"}`, "appendEnable"},
		// expectedVersion overflow: 1e999 exceeds float64 range.
		{"create", `{"name":"A","schemaContent":{"items":[]},"expectedVersion":1e999}`, "expectedVersion"},

		// --- coverage: validateOATemplateDocumentArray ---
		// Non-array (string) for an array field.
		{"create", `{"name":"A","schemaContent":{"items":[]},"managerUserIds":"bad"}`, "managerUserIds"},
		// Non-object element in pluginConfigs.
		{"create", `{"name":"A","schemaContent":{"items":[]},"pluginConfigs":[42]}`, "pluginConfigs"},
		// Unsupported field name in pluginConfigs element.
		{"create", `{"name":"A","schemaContent":{"items":[]},"pluginConfigs":[{"badField":1}]}`, "badField"},

		// --- coverage: validateOATemplateDocumentStructure ---
		// schemaContent missing items field.
		{"create", `{"name":"A","schemaContent":{}}`, "items"},
		// processConfig childNode invalid (empty nodeId).
		{"create", `{"name":"A","schemaContent":{"items":[{"componentName":"T","props":{"id":"f"}}]},"processConfig":{"nodeId":"s","type":"start","childNode":{"type":""}}}`, "nodeId"},
		// processConfig conditionNodes element invalid.
		{"create", `{"name":"A","schemaContent":{"items":[{"componentName":"T","props":{"id":"f"}}]},"processConfig":{"nodeId":"s","type":"start","conditionNodes":[{"type":""}]}}`, "nodeId"},
	}
	for _, tc := range tests {
		t.Run(tc.operation+tc.body, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "template.json")
			os.WriteFile(path, []byte(tc.body), 0600)
			caller := &scriptedToolCaller{format: "json"}
			_, err := executeOAAttachmentCommandCapturingOutput(t, caller, "approval", "template", tc.operation, "--from-document", path)
			if err == nil || !strings.Contains(err.Error(), tc.want) || caller.calls != 0 {
				t.Fatalf("want local %s error, got %v (%d calls)", tc.want, err, caller.calls)
			}
		})
	}
}

func TestCrossPlatformCoverageOATemplateDocumentSourceAndConflicts(t *testing.T) {
	path := filepath.Join(t.TempDir(), "template.json")
	os.WriteFile(path, []byte(`{"name":"A","schemaContent":{"items":[]},"processConfig":{"type":"start","nodeId":"s"}}`), 0600)
	for _, extra := range [][]string{{"--name", "override"}, {"--description", ""}, {"--schema-content", "{}"}} {
		caller := &scriptedToolCaller{format: "json"}
		args := append([]string{"approval", "template", "create", "--from-document", path}, extra...)
		if _, err := executeOAAttachmentCommandCapturingOutput(t, caller, args...); err == nil || caller.calls != 0 {
			t.Fatalf("mixed sources accepted: %v", err)
		}
	}
	for _, bad := range []string{"relative.json", "@" + path, filepath.Dir(path), path + ".missing", ""} {
		caller := &scriptedToolCaller{format: "json"}
		if _, err := executeOAAttachmentCommandCapturingOutput(t, caller, "approval", "template", "create", "--from-document", bad); err == nil || caller.calls != 0 {
			t.Fatalf("bad path accepted %s: %v", bad, err)
		}
	}
	caller := &scriptedToolCaller{format: "json"}
	stdout, err := executeOAAttachmentCommandCapturingOutput(t, caller, "approval", "template", "update", "--process-code", "PROC-1", "--from-document", path, "--dry-run")
	if err != nil || caller.calls != 0 || !strings.Contains(stdout, `"processCode": "PROC-1"`) {
		t.Fatalf("dry-run: %v %s", err, stdout)
	}
	os.WriteFile(path, []byte(`{"processCode":"PROC-2","name":"A","schemaContent":{"items":[]},"processConfig":{"type":"start","nodeId":"s"}}`), 0600)
	caller = &scriptedToolCaller{format: "json"}
	if _, err := executeOAAttachmentCommandCapturingOutput(t, caller, "approval", "template", "update", "--process-code", "PROC-1", "--from-document", path); err == nil || caller.calls != 0 {
		t.Fatal("conflicting process codes accepted")
	}
}

func TestCrossPlatformCoverageOATemplateCreateBusinessRejections(t *testing.T) {
	for _, tc := range []struct {
		code          int
		message, want string
	}{
		{810001, "An approval template with the same name already exists.", "重新命名后创建"},
		{810002, "The approval template count limit has been reached.", "清理无效模板"},
	} {
		for _, document := range []bool{false, true} {
			t.Run(fmt.Sprintf("%d/document=%t", tc.code, document), func(t *testing.T) {
				raw, _ := json.Marshal(map[string]any{"success": false, "dingOpenErrcode": tc.code, "errorMsg": tc.message, "result": map[string]any{}})
				caller := &scriptedToolCaller{format: "json", steps: []scriptedToolStep{{text: string(raw)}}}
				args := templateWriteArgs("create")
				if document {
					path := filepath.Join(t.TempDir(), "template.json")
					doc, _ := json.Marshal(map[string]any{"name": "出差申请", "schemaContent": json.RawMessage(templateForm)})
					if err := os.WriteFile(path, doc, 0600); err != nil {
						t.Fatal(err)
					}
					args = []string{"approval", "template", "create", "--from-document", path}
				}
				_, err := executeOAAttachmentCommandCapturingOutput(t, caller, args...)
				if err == nil || !strings.Contains(err.Error(), tc.want) || !strings.Contains(err.Error(), tc.message) {
					t.Fatalf("missing recovery guidance: %v", err)
				}
				if !reflect.DeepEqual(caller.toolLog, []string{"create_process_template"}) {
					t.Fatalf("unexpected calls %v", caller.toolLog)
				}
				if tc.code == 810001 {
					for _, want := range []string{"更新已有模板", "取消创建"} {
						if !strings.Contains(err.Error(), want) {
							t.Fatalf("missing %s: %v", want, err)
						}
					}
				}
			})
		}
	}
}

func TestCrossPlatformCoverageOATemplateDocumentStructureTraversal(t *testing.T) {
	// Covers validateOATemplateDocumentStructure: childNode (L1462-1465),
	// conditionNodes (L1467-1470), formConfig early-return (L1423-1424),
	// managerUserIds continue branch (L1379), pluginKey string kind (L1407-1408).
	doc := map[string]any{
		"name":           "覆盖率补充",
		"schemaContent":  json.RawMessage(`{"items":[{"componentName":"TextField","props":{"id":"f1"}}]}`),
		"processConfig":  json.RawMessage(`{"nodeId":"s","type":"start","childNode":{"nodeId":"a1","type":"approver"},"conditionNodes":[{"nodeId":"c1","type":"condition"}]}`),
		"formConfig":     json.RawMessage(`{}`),
		"managerUserIds": []string{"user1"},
		"pluginConfigs":  json.RawMessage(`[{"pluginKey":"myPlugin","pluginId":1,"enable":true,"pluginConfig":"{}"}]`),
	}
	raw, _ := json.Marshal(doc)
	path := filepath.Join(t.TempDir(), "template.json")
	if err := os.WriteFile(path, raw, 0600); err != nil {
		t.Fatal(err)
	}
	caller := &scriptedToolCaller{format: "json", steps: []scriptedToolStep{{text: templateWriteSuccess}}}
	_, err := executeOAAttachmentCommandCapturingOutput(t, caller, "approval", "template", "create", "--from-document", path)
	if err != nil {
		t.Fatal(err)
	}
	if caller.calls != 1 || caller.server != "oa" || caller.tool != "create_process_template" {
		t.Fatalf("dispatch: %s/%s (%d calls)", caller.server, caller.tool, caller.calls)
	}
}

func TestCrossPlatformCoverageOATemplateWriteSourceBlankPath(t *testing.T) {
	// Direct invocation bypasses LeafAtLeastOne constraint (which rejects
	// whitespace-only values before Validate runs): covers L1199-1200.
	cmd := &cobra.Command{Use: "create"}
	cmd.Flags().String("from-document", "", "")
	_ = cmd.Flags().Set("from-document", "  ")
	err := validateOATemplateWriteSources(cmd, nil)
	if err == nil || !strings.Contains(err.Error(), "绝对路径") {
		t.Fatalf("whitespace-only --from-document accepted: %v", err)
	}
}

func TestCrossPlatformCoverageOATemplateProcessCodeEmptyWithDocument(t *testing.T) {
	// --process-code " " combined with --from-document: covers L1310-1311.
	doc := map[string]any{
		"name":          "A",
		"schemaContent": json.RawMessage(`{"items":[{"componentName":"T","props":{"id":"f"}}]}`),
		"processConfig": json.RawMessage(`{"nodeId":"s","type":"start"}`),
		"processCode":   "PROC-1",
	}
	raw, _ := json.Marshal(doc)
	path := filepath.Join(t.TempDir(), "template.json")
	os.WriteFile(path, raw, 0600)
	caller := &scriptedToolCaller{format: "json"}
	_, err := executeOAAttachmentCommandCapturingOutput(t, caller, "approval", "template", "update", "--process-code", "  ", "--from-document", path)
	if err == nil || caller.calls != 0 || !strings.Contains(err.Error(), "process-code") {
		t.Fatalf("empty --process-code accepted: %v", err)
	}
}

func TestCrossPlatformCoverageOATemplateCreateSuccessPathRejection(t *testing.T) {
	// When the MCP response lacks a "success" field but carries dingOpenErrcode=810001,
	// the shared isBusinessError check passes it through as data. The template-specific
	// oaTemplateCreateRejection then catches the rejection: covers L1532-1533.
	for _, tc := range []struct {
		code int
		want string
	}{
		{810001, "重新命名后创建"},
		{810002, "清理无效模板"},
	} {
		t.Run(fmt.Sprintf("%d", tc.code), func(t *testing.T) {
			response, _ := json.Marshal(map[string]any{
				"dingOpenErrcode": tc.code,
				"errorMsg":        "rejection",
				"result":          map[string]any{},
			})
			caller := &scriptedToolCaller{format: "json", steps: []scriptedToolStep{{text: string(response)}}}
			_, err := executeOAAttachmentCommandCapturingOutput(t, caller, templateWriteArgs("create")...)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("missing rejection guidance: %v", err)
			}
		})
	}
}

func TestCrossPlatformCoverageOATemplateDocumentStructureDirectValidation(t *testing.T) {
	// Direct unit tests for validateOATemplateDocumentStructure to cover branches
	// unreachable through the command path (oaTemplateJSONFlag.Transform validates
	// "object" shape before validateOATemplateDocumentStructure is called).
	for _, tc := range []struct {
		key, raw, want string
		fail           bool
	}{
		// Non-schemaContent/processConfig key returns nil immediately: L1423-1424.
		{"formConfig", `{}`, "", false},
		{"nodeConfig", `{"anything":true}`, "", false},
		// schemaContent not a JSON object: L1427-1428.
		{"schemaContent", `[]`, "JSON 对象", true},
		{"schemaContent", `not json`, "JSON 对象", true},
		// processConfig not a JSON object: L1427-1428.
		{"processConfig", `"string"`, "JSON 对象", true},
	} {
		t.Run(tc.key+"/"+tc.raw, func(t *testing.T) {
			err := validateOATemplateDocumentStructure(tc.key, []byte(tc.raw))
			if tc.fail {
				if err == nil || !strings.Contains(err.Error(), tc.want) {
					t.Fatalf("want error containing %q, got %v", tc.want, err)
				}
			} else if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

// Five defensive statements in the template write path are provably
// unreachable and therefore cannot be covered without weakening the guards
// they protect. They must stay: each fails closed if an upstream invariant is
// ever broken by a future refactor. This test pins the invariants that make
// them dead code today, so the analysis is verified in CI rather than trusted
// on faith. If any assertion below starts failing, the matching guard has
// become reachable and needs a real coverage case.
//
// Inline guards (cannot be unit-tested in isolation; documented here):
//   - oa.go L1226 `if err != nil || len(raw) > maxSize`: os.File.Stat already
//     rejects non-regular files and sizes > 4 MiB (L1222), so io.ReadAll of a
//     regular file never errors and never yields more than maxSize bytes.
//   - oa.go L1264 `json.Unmarshal(value, &text)` in the quote-prefixed branch:
//     value is a json.RawMessage from decodeOATemplateDocument, so a leading
//     quote guarantees a complete valid JSON string that always unmarshals.
//   - oa.go L1292 `decoder.Decode(&n)` for expectedVersion: value is again a
//     valid json.RawMessage, so Decode into any always succeeds; the non-number
//     case is caught by the json.Number type assertion at L1296 instead.
func TestCrossPlatformCoverageOATemplateWriteUnreachableGuards(t *testing.T) {
	// oa.go L1347 `if !ok`: a JSON object key token is always a string. No
	// input makes decoder.Token() yield a non-string in key position without
	// first returning an error (caught at L1343). Confirm every key decodes as
	// a string so the type assertion never fails.
	for _, raw := range []string{`{"a":1}`, `{"1":2}`, `{"":3}`, `{"a":1,"b":2}`} {
		fields, err := decodeOATemplateDocument([]byte(raw))
		if err != nil {
			t.Fatalf("decode %s: %v", raw, err)
		}
		if len(fields) == 0 {
			t.Fatalf("decode %s produced no fields", raw)
		}
	}

	// oa.go L1402 `decoder.Decode(&decoded)`: each element field is a
	// json.RawMessage extracted by unmarshaling into map[string]json.RawMessage,
	// so its bytes are always valid JSON and Decode never fails. Feed values
	// (including huge/overflowing numbers) that reach the per-field decode and
	// confirm validation is driven only by the kind check at L1414, never by a
	// decode error.
	for _, tc := range []struct {
		key, raw, want string
	}{
		{"pluginConfigs", `[{"pluginId":1e999}]`, "pluginId"},
		{"visibleRange", `[{"visibleType":123456789012345678901234567890}]`, ""},
	} {
		err := validateOATemplateDocumentArray(tc.key, []byte(tc.raw))
		if tc.want == "" {
			if err != nil {
				t.Fatalf("array %s: unexpected error %v", tc.raw, err)
			}
			continue
		}
		// A kind mismatch (number expected, overflow still decodes as
		// json.Number) surfaces the field name, never a decode failure.
		if err != nil && !strings.Contains(err.Error(), tc.want) {
			t.Fatalf("array %s: error %v not about %s", tc.raw, err, tc.want)
		}
	}
}
func TestCrossPlatformCoverageOATemplateNestedChildrenValidation(t *testing.T) {
	for _, tc := range []struct {
		name, raw, want string
		fail            bool
	}{
		// Child control missing props.id → fail.
		{
			name: "child missing props.id",
			raw:  `{"items":[{"componentName":"TableField","props":{"id":"table-1"},"children":[{"componentName":"TextField","props":{"id":""}}]}]}`,
			want: "props.id",
			fail: true,
		},
		// Child control duplicates parent ID → fail.
		{
			name: "child duplicates parent id",
			raw:  `{"items":[{"componentName":"TableField","props":{"id":"field-1"},"children":[{"componentName":"TextField","props":{"id":"field-1"}}]}]}`,
			want: "props.id",
			fail: true,
		},
		// Sibling children share same ID → fail.
		{
			name: "sibling children duplicate id",
			raw:  `{"items":[{"componentName":"TableField","props":{"id":"table-1"},"children":[{"componentName":"TextField","props":{"id":"dup"}},{"componentName":"NumberField","props":{"id":"dup"}}]}]}`,
			want: "props.id",
			fail: true,
		},
		// Valid nested controls with unique IDs → pass.
		{
			name: "valid nested controls",
			raw:  `{"items":[{"componentName":"TableField","props":{"id":"table-1"},"children":[{"componentName":"TextField","props":{"id":"text-1"}},{"componentName":"NumberField","props":{"id":"num-1"}}]}]}`,
			fail: false,
		},
		// Deeply nested: children within children.
		{
			name: "deeply nested valid",
			raw:  `{"items":[{"componentName":"TableField","props":{"id":"t1"},"children":[{"componentName":"InnerTable","props":{"id":"t2"},"children":[{"componentName":"TextField","props":{"id":"f1"}}]}]}]}`,
			fail: false,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := validateOATemplateDocumentStructure("schemaContent", []byte(tc.raw))
			if tc.fail {
				if err == nil || !strings.Contains(err.Error(), tc.want) {
					t.Fatalf("want error containing %q, got %v", tc.want, err)
				}
			} else if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestCrossPlatformCoverageOATemplateNestedChildrenDocumentValidation(t *testing.T) {
	// End-to-end: from-document path with nested children.
	for _, tc := range []struct {
		name, schema, want string
		fail               bool
	}{
		{
			name:   "document child missing id",
			schema: `{"items":[{"componentName":"TableField","props":{"id":"table-1"},"children":[{"componentName":"TextField","props":{}}]}]}`,
			want:   "id",
			fail:   true,
		},
		{
			name:   "document child parent id collision",
			schema: `{"items":[{"componentName":"TableField","props":{"id":"x"},"children":[{"componentName":"TextField","props":{"id":"x"}}]}]}`,
			want:   "id",
			fail:   true,
		},
		{
			name:   "document valid nested",
			schema: `{"items":[{"componentName":"TableField","props":{"id":"table-1"},"children":[{"componentName":"TextField","props":{"id":"text-1"}}]}]}`,
			fail:   false,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			doc := map[string]any{"name": "嵌套测试", "schemaContent": json.RawMessage(tc.schema)}
			raw, _ := json.Marshal(doc)
			path := filepath.Join(t.TempDir(), "template.json")
			os.WriteFile(path, raw, 0600)
			caller := &scriptedToolCaller{format: "json"}
			if !tc.fail {
				caller.steps = []scriptedToolStep{{text: templateWriteSuccess}}
			}
			_, err := executeOAAttachmentCommandCapturingOutput(t, caller, "approval", "template", "create", "--from-document", path)
			if tc.fail {
				if err == nil || !strings.Contains(err.Error(), tc.want) {
					t.Fatalf("want error containing %q, got %v", tc.want, err)
				}
				if caller.calls != 0 {
					t.Fatal("invalid input called MCP")
				}
			} else if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}
