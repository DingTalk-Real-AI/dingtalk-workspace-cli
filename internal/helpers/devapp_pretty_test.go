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

func TestDevAppPrettyAnnotateAppStatus(t *testing.T) {
	payload := map[string]any{
		"result": map[string]any{
			"items": []any{
				map[string]any{"status": float64(1)},
				map[string]any{"status": float64(0)},
			},
		},
	}
	devAppPrettyAnnotate("list_dev_app", payload)
	items := payload["result"].(map[string]any)["items"].([]any)
	if got := items[0].(map[string]any)["statusText"]; got != "ACTIVE 已激活" {
		t.Fatalf("statusText = %v, want ACTIVE 已激活", got)
	}
	if got := items[1].(map[string]any)["statusText"]; got != "IN_ACTIVE 已停用" {
		t.Fatalf("statusText = %v, want IN_ACTIVE 已停用", got)
	}
}

func TestDevAppPrettyAnnotateVersionStatusKindSeparation(t *testing.T) {
	// 版本工具只注解 versionStatus；旧 status 字段不再参与版本契约。
	version := map[string]any{"status": "INIT", "versionStatus": "RELEASE"}
	devAppPrettyAnnotate("get_dev_app_version_detail", version)
	if version["versionStatusText"] != "已发布生效" {
		t.Fatalf("version annotations wrong: %#v", version)
	}
	if _, exists := version["statusText"]; exists {
		t.Fatalf("version status should not be annotated: %#v", version)
	}

	// 应用工具的数字枚举不能套用版本语义；未登记的工具不动
	unknown := map[string]any{"status": float64(1)}
	devAppPrettyAnnotate("some_unknown_tool", unknown)
	if _, exists := unknown["statusText"]; exists {
		t.Fatalf("unknown tool should not be annotated: %#v", unknown)
	}
}
