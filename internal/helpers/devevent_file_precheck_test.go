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
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
)

// devEventPrecheckError 断言一次预检失败：Message 带原因，Hint 带可恢复动作。
// Hint 不进 Error()（apperrors.Error.Error() 只返回 Message），只能解类型断言。
func devEventPrecheckError(t *testing.T, err error, want string) {
	t.Helper()
	if err == nil {
		t.Fatalf("want validation error containing %q, got nil", want)
	}
	if !strings.Contains(err.Error(), want) {
		t.Fatalf("error = %v, want containing %q", err, want)
	}
	var typed *apperrors.Error
	if !errors.As(err, &typed) {
		t.Fatalf("error type = %T, want *apperrors.Error", err)
	}
	if strings.TrimSpace(typed.Hint) == "" {
		t.Fatalf("error %q has empty Hint, want a recovery action", err)
	}
}

func TestDevEventApplyDefinitionFileForwardsOnlyDefinitionFields(t *testing.T) {
	path := writeDevEventFile(t, "hr-employee-joined.json", `{
	  "eventKey": "app_user_hr_employee_joined",
	  "name": "员工入职",
	  "description": "入职完成后上报",
	  "payloadSchema": {"type": "object"},
	  "samplePayload": {"employeeId": "E-1"},
	  "extra": "not forwarded"
	}`)
	params := map[string]any{"file": path, "unifiedAppId": "uapp_self_hr_A"}
	if err := devEventApplyDefinitionFile(params, false); err != nil {
		t.Fatalf("devEventApplyDefinitionFile() error = %v", err)
	}
	if _, present := params["file"]; present {
		t.Fatalf("file param present = %#v, want removed: the local path is not a business parameter", params["file"])
	}
	for _, field := range devEventDefinitionFields {
		if _, present := params[field]; !present {
			t.Fatalf("definition field %q not forwarded", field)
		}
	}
	if _, present := params["extra"]; present {
		t.Fatal("undeclared field was forwarded; only devEventDefinitionFields may leave the CLI")
	}
	if params["unifiedAppId"] != "uapp_self_hr_A" {
		t.Fatalf("unifiedAppId = %#v, want preserved", params["unifiedAppId"])
	}
}

func TestDevEventApplyDefinitionFileSerializesSchemaAsJsonText(t *testing.T) {
	path := writeDevEventFile(t, "schema-types.json", `{
	  "eventKey": "app_user_hr_employee_joined",
	  "name": "员工入职",
	  "payloadSchema": {
	    "type": "object",
	    "additionalProperties": false,
	    "properties": {"count": {"type": "integer", "description": "统计数量，非负整数"}}
	  },
	  "samplePayload": {"count": 1}
	}`)
	params := map[string]any{"file": path}
	if err := devEventApplyDefinitionFile(params, false); err != nil {
		t.Fatalf("devEventApplyDefinitionFile() error = %v", err)
	}
	for _, field := range []string{"payloadSchema", "samplePayload"} {
		value, ok := params[field].(string)
		if !ok {
			t.Fatalf("%s type = %T, want JSON text", field, params[field])
		}
		if strings.Contains(value, `"additionalProperties":"False"`) {
			t.Fatalf("%s converted boolean to string: %s", field, value)
		}
	}
	if !strings.Contains(params["payloadSchema"].(string), `"additionalProperties":false`) {
		t.Fatalf("payloadSchema = %s, want boolean additionalProperties", params["payloadSchema"])
	}
}

func TestDevEventApplyDefinitionFileRequiresPath(t *testing.T) {
	for name, params := range map[string]map[string]any{
		"absent":     {"unifiedAppId": "A"},
		"empty":      {"file": "   "},
		"wrong type": {"file": 7},
		"nil":        {"file": nil},
	} {
		t.Run(name, func(t *testing.T) {
			err := devEventApplyDefinitionFile(params, false)
			devEventPrecheckError(t, err, "--file 为必填")
			if _, present := params["file"]; present {
				t.Fatalf("file param still present = %#v", params["file"])
			}
		})
	}
}

func TestDevEventApplyDefinitionFilePropagatesReadError(t *testing.T) {
	params := map[string]any{"file": filepath.Join(t.TempDir(), "missing.json")}
	devEventPrecheckError(t, devEventApplyDefinitionFile(params, true), "指向的文件不可读")
}

func TestDevEventUpdateRejectsEventKeyMismatch(t *testing.T) {
	path := writeDevEventFile(t, "other.json", strings.Replace(devEventTestDefinition,
		"app_user_hr_employee_joined", "app_user_hr_employee_left", 1))
	params := map[string]any{"file": path, "eventKey": "app_user_hr_employee_joined"}
	err := devEventApplyDefinitionFile(params, true)
	devEventPrecheckError(t, err, `--file 中的 eventKey（"app_user_hr_employee_left"）与 --event-key（"app_user_hr_employee_joined"）不一致`)
	if _, present := params["name"]; present {
		t.Fatal("mismatched definition was still merged into params")
	}

	// 同一个文件在 create（matchEventKey=false）下不与 --event-key 比对。
	params = map[string]any{"file": path}
	if err := devEventApplyDefinitionFile(params, false); err != nil {
		t.Fatalf("create path error = %v, want no eventKey comparison", err)
	}
	if params["eventKey"] != "app_user_hr_employee_left" {
		t.Fatalf("eventKey = %#v, want the file value", params["eventKey"])
	}
}

func TestDevEventReadDefinitionFileRejectsControlledKeys(t *testing.T) {
	path := writeDevEventFile(t, "controlled.json", `{
	  "eventKey": "app_user_hr_employee_joined",
	  "name": "员工入职",
	  "payloadSchema": {"type": "object"},
	  "scope": "self",
	  "status": "ONLINE",
	  "configRevision": 9
	}`)
	_, err := devEventReadDefinitionFile(path)
	// 受控键按声明顺序回报，开发者一次就能删干净。
	devEventPrecheckError(t, err, "含不接受开发者自行指定的键：scope, status, configRevision")
}

func TestDevEventRejectedControlledKeysKeepsDeclarationOrder(t *testing.T) {
	rejected := devEventRejectedControlledKeys(map[string]any{"published": true, "defaultScope": "x", "name": "keep"})
	want := []string{"defaultScope", "published"}
	if len(rejected) != len(want) {
		t.Fatalf("rejected = %#v, want %#v", rejected, want)
	}
	for index, key := range want {
		if rejected[index] != key {
			t.Fatalf("rejected = %#v, want declaration order %#v", rejected, want)
		}
	}
	if got := devEventRejectedControlledKeys(map[string]any{"eventKey": "app_user_a_b_c"}); len(got) != 0 {
		t.Fatalf("rejected = %#v, want empty", got)
	}
}

func TestDevEventReadDefinitionFileRequiredAndTypedFields(t *testing.T) {
	cases := []struct {
		name    string
		content string
		want    string
	}{
		{"missing eventKey", `{"name":"n","payloadSchema":{"type":"object"}}`, `缺少必填键 "eventKey"`},
		{"blank eventKey", `{"eventKey":"  ","name":"n","payloadSchema":{"type":"object"}}`, `缺少必填键 "eventKey"`},
		{"non-string eventKey", `{"eventKey":7,"name":"n","payloadSchema":{"type":"object"}}`, `缺少必填键 "eventKey"`},
		{"missing name", `{"eventKey":"app_user_a_b_c","payloadSchema":{"type":"object"}}`, `缺少必填键 "name"`},
		{"missing payloadSchema", `{"eventKey":"app_user_a_b_c","name":"n"}`, `缺少必填键 "payloadSchema"`},
		{"null payloadSchema", `{"eventKey":"app_user_a_b_c","name":"n","payloadSchema":null}`, "的 payloadSchema 是 null"},
		{"array payloadSchema", `{"eventKey":"app_user_a_b_c","name":"n","payloadSchema":[]}`, `payloadSchema 必须是 JSON 对象`},
		{"string payloadSchema", `{"eventKey":"app_user_a_b_c","name":"n","payloadSchema":"object"}`, `payloadSchema 必须是 JSON 对象`},
		{"payloadSchema type not object", `{"eventKey":"app_user_a_b_c","name":"n","payloadSchema":{"type":"array"}}`, `payloadSchema.type 必须是 "object"（当前 "array"）`},
		{"payloadSchema without type", `{"eventKey":"app_user_a_b_c","name":"n","payloadSchema":{"properties":{}}}`, `payloadSchema.type 必须是 "object"（当前 ""）`},
		{"array samplePayload", `{"eventKey":"app_user_a_b_c","name":"n","payloadSchema":{"type":"object"},"samplePayload":[]}`, `samplePayload 必须是 JSON 对象`},
		{"null samplePayload is rejected, not treated as absent", `{"eventKey":"app_user_a_b_c","name":"n","payloadSchema":{"type":"object"},"samplePayload":null}`, "的 samplePayload 是 null"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			path := writeDevEventFile(t, "definition.json", c.content)
			definition, err := devEventReadDefinitionFile(path)
			if c.want == "" {
				if err != nil {
					t.Fatalf("devEventReadDefinitionFile() error = %v, want nil", err)
				}
				if definition == nil {
					t.Fatal("definition == nil, want the parsed object")
				}
				return
			}
			devEventPrecheckError(t, err, c.want)
			if definition != nil {
				t.Fatalf("definition = %#v, want nil on validation failure", definition)
			}
		})
	}
}

func TestDevEventReadDefinitionFileAcceptsDocumentedKeys(t *testing.T) {
	accepted := []string{
		"app_user_hr_employee_joined",
		"app_user_finance_invoice_paid",
		"app_user_project_task_assigned",
	}
	for _, eventKey := range accepted {
		t.Run("accept "+eventKey, func(t *testing.T) {
			path := writeDevEventFile(t, "definition.json", strings.Replace(devEventTestDefinition,
				"app_user_hr_employee_joined", eventKey, 1))
			if _, err := devEventReadDefinitionFile(path); err != nil {
				t.Fatalf("devEventReadDefinitionFile() error = %v", err)
			}
		})
	}
}

func TestDevEventReadDefinitionFileRejectsKeyShape(t *testing.T) {
	rejected := []string{
		"acme_hr_employee_joined",                 // 缺 app_user_ 前缀
		"user_",                                   // 前缀后为空
		"app_user_1hr_employee_joined",            // 业务系统首字符不是小写字母
		"app_user_HR_employee_joined",             // 含大写
		"app_user_hr-employee_joined",             // 含连字符
		"app_user_hr_employee joined",             // 含空格
		"USER_app_hr_employee_joined",             // 前缀大写
		"app_user_hr_employee",                    // 缺少动作段
		strings.Repeat("a", devEventKeyMaxLength), // 超长（同时不符合前缀规则）
	}
	for _, eventKey := range rejected {
		t.Run("reject "+eventKey, func(t *testing.T) {
			content := strings.Replace(devEventTestDefinition, "app_user_hr_employee_joined", eventKey, 1)
			path := writeDevEventFile(t, "definition.json", content)
			_, err := devEventReadDefinitionFile(path)
			if err == nil {
				t.Fatalf("eventKey %q was accepted, want rejection", eventKey)
			}
			if !strings.Contains(err.Error(), "不符合命名规则") {
				t.Fatalf("eventKey %q error = %v, want 不符合命名规则", eventKey, err)
			}
		})
	}
}

func TestDevEventReadDefinitionFileRejectsOverlongKey(t *testing.T) {
	// 合法前缀 + 合法字符，但超过 event_key VARCHAR(128)。
	eventKey := "app_user_hr_employee_" + strings.Repeat("a", devEventKeyMaxLength)
	path := writeDevEventFile(t, "definition.json", strings.Replace(devEventTestDefinition,
		"app_user_hr_employee_joined", eventKey, 1))
	_, err := devEventReadDefinitionFile(path)
	devEventPrecheckError(t, err, "的 eventKey 超过 128 字符")
}

func TestDevEventReadJSONObjectBoundaries(t *testing.T) {
	dir := t.TempDir()

	t.Run("missing", func(t *testing.T) {
		_, err := devEventReadJSONObject(filepath.Join(dir, "missing.json"), "--file")
		devEventPrecheckError(t, err, "指向的文件不可读")
	})

	t.Run("directory", func(t *testing.T) {
		_, err := devEventReadJSONObject(dir, "--payload-file")
		devEventPrecheckError(t, err, "指向的是目录而不是文件")
	})

	t.Run("oversize", func(t *testing.T) {
		path := filepath.Join(dir, "big.json")
		if err := os.WriteFile(path, make([]byte, devEventDefinitionFileMaxBytes+1), 0o600); err != nil {
			t.Fatal(err)
		}
		_, err := devEventReadJSONObject(path, "--file")
		devEventPrecheckError(t, err, "超过 1048576 字节上限")
	})

	t.Run("read failure after stat", func(t *testing.T) {
		path := writeDevEventFile(t, "definition.json", devEventTestDefinition)
		testseam.Swap(t, &devEventReadFile, func(string) ([]byte, error) {
			return nil, errors.New("permission denied")
		})
		_, err := devEventReadJSONObject(path, "--file")
		devEventPrecheckError(t, err, "读取文件")
	})

	t.Run("read limit is enforced after stat", func(t *testing.T) {
		path := writeDevEventFile(t, "grown.json", "{}")
		testseam.Swap(t, &devEventReadFile, func(string) ([]byte, error) {
			return make([]byte, devEventDefinitionFileMaxBytes+1), nil
		})
		_, err := devEventReadJSONObject(path, "--file")
		devEventPrecheckError(t, err, "超过 1048576 字节上限")
	})

	t.Run("empty and whitespace only", func(t *testing.T) {
		for name, content := range map[string]string{"empty": "", "whitespace": "  \n\t "} {
			path := writeDevEventFile(t, name+".json", content)
			_, err := devEventReadJSONObject(path, "--file")
			devEventPrecheckError(t, err, "指向的文件为空")
		}
	})

	t.Run("bom is tolerated", func(t *testing.T) {
		path := writeDevEventFile(t, "bom.json", "\ufeff  "+devEventTestDefinition+"\n")
		definition, err := devEventReadJSONObject(path, "--file")
		if err != nil {
			t.Fatalf("devEventReadJSONObject() error = %v", err)
		}
		if definition["eventKey"] != "app_user_hr_employee_joined" {
			t.Fatalf("eventKey = %#v, want the BOM stripped before parsing", definition["eventKey"])
		}
	})

	t.Run("not an object", func(t *testing.T) {
		for name, content := range map[string]string{
			"array":  `[{"eventKey":"app_user_a_b_c"}]`,
			"string": `"app_user_a_b_c"`,
			"broken": `{"eventKey":`,
		} {
			path := writeDevEventFile(t, name+".json", content)
			_, err := devEventReadJSONObject(path, "--file")
			devEventPrecheckError(t, err, "不是合法 JSON 对象")
		}
	})

	t.Run("top level null", func(t *testing.T) {
		path := writeDevEventFile(t, "null.json", "null")
		_, err := devEventReadJSONObject(path, "--file")
		devEventPrecheckError(t, err, "顶层是 null，必须是 JSON 对象")
	})
}

func TestDevEventCheckObjectField(t *testing.T) {
	definition := map[string]any{"present": map[string]any{"a": 1}, "nullish": nil, "wrong": "text"}

	if err := devEventCheckObjectField("f.json", definition, "present", true); err != nil {
		t.Fatalf("present object error = %v", err)
	}
	if err := devEventCheckObjectField("f.json", definition, "absent", false); err != nil {
		t.Fatalf("absent optional error = %v, want nil", err)
	}
	devEventPrecheckError(t, devEventCheckObjectField("f.json", definition, "absent", true), `缺少必填键 "absent"`)
	// 显式 null 与键缺席是两回事：null 会被原样下发，必填与可选都拒绝。
	devEventPrecheckError(t, devEventCheckObjectField("f.json", definition, "nullish", true), "的 nullish 是 null")
	devEventPrecheckError(t, devEventCheckObjectField("f.json", definition, "nullish", false), "的 nullish 是 null")
	devEventPrecheckError(t, devEventCheckObjectField("f.json", definition, "wrong", false), "wrong 必须是 JSON 对象")
	devEventPrecheckError(t, devEventCheckObjectField("f.json", definition, "wrong", true), "wrong 必须是 JSON 对象")
}

func TestDevEventParamString(t *testing.T) {
	if got := devEventParamString(nil, "eventKey"); got != "" {
		t.Fatalf("nil source = %q, want empty", got)
	}
	if got := devEventParamString(map[string]any{"eventKey": 7, "name": "n"}, "eventKey"); got != "" {
		t.Fatalf("non-string value = %q, want empty", got)
	}
	if got := devEventParamString(map[string]any{"name": "n"}, "name"); got != "n" {
		t.Fatalf("string value = %q, want n", got)
	}
}

// TestDevEventDebugPayloadFile 覆盖 debug 的 --payload-file 两条分支：缺省时不
// 注入 samplePayload（沿用已保存样例），提供时覆盖为文件内容，且路径本身不下发。
func TestDevEventDebugPayloadFile(t *testing.T) {
	t.Run("absent keeps the saved sample", func(t *testing.T) {
		runner := &fakeDevEventRunner{}
		cmd, _ := prepareDevEventLeaf(t, newDevEventDebugCommand(runner))
		_ = cmd.Flags().Set("yes", "true")
		_ = cmd.Flags().Set("unified-app-id", "uapp_self_hr_A")
		_ = cmd.Flags().Set("event-key", "app_user_hr_employee_joined")
		_ = cmd.Flags().Set("expected-revision", "1")
		if err := cmd.RunE(cmd, nil); err != nil {
			t.Fatalf("RunE() error = %v", err)
		}
		if _, present := runner.got.Params["samplePayload"]; present {
			t.Fatalf("samplePayload = %#v, want absent so the saved sample is used", runner.got.Params["samplePayload"])
		}
		if _, present := runner.got.Params["payloadFile"]; present {
			t.Fatal("payloadFile path was forwarded as a business parameter")
		}
	})

	t.Run("invalid payload file fails before dispatch", func(t *testing.T) {
		runner := &fakeDevEventRunner{}
		cmd, _ := prepareDevEventLeaf(t, newDevEventDebugCommand(runner))
		_ = cmd.Flags().Set("yes", "true")
		_ = cmd.Flags().Set("unified-app-id", "uapp_self_hr_A")
		_ = cmd.Flags().Set("event-key", "app_user_hr_employee_joined")
		_ = cmd.Flags().Set("expected-revision", "1")
		_ = cmd.Flags().Set("payload-file", writeDevEventFile(t, "payload.json", "[1,2]"))
		err := cmd.RunE(cmd, nil)
		devEventPrecheckError(t, err, "--payload-file 指向的文件")
		if runner.calls != 0 {
			t.Fatal("invalid payload file reached the backend")
		}
	})

	t.Run("blank payload file is ignored", func(t *testing.T) {
		params := map[string]any{"payloadFile": "   "}
		runner := &fakeDevEventRunner{}
		cmd, _ := prepareDevEventLeaf(t, newDevEventDebugCommand(runner))
		call := devEventCallWithPayloadFile(runner)
		if err := call(cmd, devEventDebugTool, params); err != nil {
			t.Fatalf("call error = %v", err)
		}
		if _, present := runner.got.Params["payloadFile"]; present {
			t.Fatal("payloadFile was forwarded")
		}
		if _, present := runner.got.Params["samplePayload"]; present {
			t.Fatal("blank payload file injected samplePayload")
		}
	})
}
