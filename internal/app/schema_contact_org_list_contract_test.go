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

package app

import (
	"bytes"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
)

// schemaString normalizes a schema payload value to string for assertions.
func schemaString(v any) string {
	s, _ := v.(string)
	return s
}

func contactOrgListSchemaLeaf(t *testing.T, canonical string, compact bool) map[string]any {
	t.Helper()
	root := NewRootCommand()
	var stdout, stderr bytes.Buffer
	root.SetOut(&stdout)
	root.SetErr(&stderr)
	args := []string{"schema", canonical, "--format", "json"}
	if compact {
		args = append(args, "--compact")
	}
	root.SetArgs(args)
	if err := root.Execute(); err != nil {
		t.Fatalf("execute schema %s compact=%v: %v; stderr=%s", canonical, compact, err, stderr.String())
	}
	var payload map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatalf("decode schema %s compact=%v: %v", canonical, compact, err)
	}
	return payload
}

// TestContactOrgListSchemaPublishesPaginationAndStripsCursorFields 验证
// contact.list_team_invite 与 contact.query_org_apply_list 的 full/compact
// 最终投影都发布统一 Pagination 契约，且业务 Result.DataSchema 不再包含
// hasMore/nextCursor 等内部分页字段。
func TestContactOrgListSchemaPublishesPaginationAndStripsCursorFields(t *testing.T) {
	for _, canonical := range []string{"contact.list_team_invite", "contact.query_org_apply_list"} {
		t.Run(canonical, func(t *testing.T) {
			full := contactOrgListSchemaLeaf(t, canonical, false)
			compact := contactOrgListSchemaLeaf(t, canonical, true)

			if full["result"] == nil || compact["result"] == nil {
				t.Fatalf("result missing: full=%#v compact=%#v", full["result"], compact["result"])
			}
			if !strings.EqualFold(schemaString(full["result"].(map[string]any)["data_schema"].(map[string]any)["type"]), "object") {
				t.Fatalf("full result data_schema is not object: %#v", full["result"])
			}
			if !strings.EqualFold(schemaString(compact["result"].(map[string]any)["data_schema"].(map[string]any)["type"]), "object") {
				t.Fatalf("compact result data_schema is not object: %#v", compact["result"])
			}

			fullResult := full["result"].(map[string]any)
			compactResult := compact["result"].(map[string]any)
			fullDataSchema, _ := json.Marshal(fullResult["data_schema"])
			compactDataSchema, _ := json.Marshal(compactResult["data_schema"])
			for _, s := range []string{string(fullDataSchema), string(compactDataSchema)} {
				if strings.Contains(s, "hasMore") || strings.Contains(s, "nextCursor") {
					t.Fatalf("%s data_schema must not contain pagination fields: %s", canonical, s)
				}
			}

			if full["pagination"] == nil || compact["pagination"] == nil {
				t.Fatalf("pagination missing: full=%#v compact=%#v", full["pagination"], compact["pagination"])
			}
			if !reflect.DeepEqual(full["pagination"], compact["pagination"]) {
				t.Fatalf("full/compact pagination mismatch\nfull=%#v\ncompact=%#v", full["pagination"], compact["pagination"])
			}
			pg := full["pagination"].(map[string]any)
			if schemaString(pg["kind"]) != contract.PaginationKindCursor {
				t.Fatalf("%s pagination.kind = %v, want %s", canonical, pg["kind"], contract.PaginationKindCursor)
			}
			if schemaString(pg["cursor_parameter"]) != "cursor" {
				t.Fatalf("%s pagination.cursor_parameter = %v, want cursor", canonical, pg["cursor_parameter"])
			}
			if schemaString(pg["meta_path"]) != contract.PaginationMetaPath {
				t.Fatalf("%s pagination.meta_path = %v, want %s", canonical, pg["meta_path"], contract.PaginationMetaPath)
			}
		})
	}
}
