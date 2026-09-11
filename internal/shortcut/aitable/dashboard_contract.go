// Copyright 2026 Alibaba Group
// SPDX-License-Identifier: Apache-2.0

package aitable

import (
	"encoding/json"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
)

// dashboardGetResultSpec makes the schemaVersion type evidence part of the
// reviewed Shortcut result contract. Runtime output remains the unmodified MCP
// response so JSON number 2 and string "2" stay distinguishable.
func dashboardGetResultSpec() *contract.ResultSpec {
	return &contract.ResultSpec{
		Outcomes: []contract.ResultOutcome{
			contract.ResultOutcomeSuccess,
			contract.ResultOutcomeFailure,
		},
		DataSchema: json.RawMessage(`{
  "type":"object",
  "description":"get_dashboard 的 MCP 响应信封；Dashboard 元信息保持 MCP 返回的 JSON 类型",
  "properties":{
    "data":{
      "type":"object",
      "description":"Dashboard 详情",
      "properties":{
        "meta":{
          "description":"决定根布局协议的只读 Dashboard 元信息",
          "oneOf":[
            {
              "type":"object",
              "properties":{
                "schemaVersion":{"description":"MCP 返回的原始 JSON 标量；历史 Dashboard 可省略"},
                "schemaVersionTypeVerified":{"type":"boolean","description":"schemaVersion 是否保留了 storage 中的原始 JSON 标量类型"}
              },
              "required":["schemaVersionTypeVerified"],
              "additionalProperties":true
            },
            {"type":"null"}
          ]
        }
      },
      "required":["meta"],
      "additionalProperties":true
    }
  },
  "required":["data"],
  "additionalProperties":true
}`),
	}
}
