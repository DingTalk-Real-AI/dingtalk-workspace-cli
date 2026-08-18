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
	"encoding/json"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
)

// aitable_schema.go holds shared Safety / Interface factories for aitable's
// DeclareLeafMetadata declarations (metadata-only mode). Selection prose and
// per-command payloads live in aitable.go alongside their leaf definitions.

func aitableSafetyRead() contract.SafetySpec {
	return contract.SafetySpec{
		Effect: "read", Risk: "low",
		Confirmation: "not_required", Idempotency: "idempotent",
	}
}

func aitableSafetyWrite() contract.SafetySpec {
	return contract.SafetySpec{
		Effect: "write", Risk: "medium",
		Confirmation: "not_required", Idempotency: "unknown",
	}
}

func aitableSafetyWriteConfirm() contract.SafetySpec {
	return contract.SafetySpec{
		Effect: "write", Risk: "medium",
		Confirmation: "user_required", Idempotency: "unknown",
	}
}

func aitableSafetyDestructive() contract.SafetySpec {
	return contract.SafetySpec{
		Effect: "destructive", Risk: "high",
		Confirmation: "user_required", Idempotency: "unknown",
	}
}

func aitableMCPInterface(rpc string) *contract.InterfaceSpec {
	return &contract.InterfaceSpec{
		Mode:         "mcp",
		Availability: "available",
		Ref:          &contract.InterfaceRefSpec{ProductID: "aitable", RPCName: rpc},
	}
}

func aitableHelperMCPInterface(rpc string) *contract.InterfaceSpec {
	return &contract.InterfaceSpec{
		Mode:         "mcp",
		Availability: "available",
		Ref:          &contract.InterfaceRefSpec{ProductID: "aitable-helper", RPCName: rpc},
	}
}

func aitableCompositeInterface(reason string) *contract.InterfaceSpec {
	return &contract.InterfaceSpec{
		Mode:         "composite",
		Availability: "available",
		Reason:       reason,
	}
}

func aitableRecordsStatsResultSpec() *contract.ResultSpec {
	return &contract.ResultSpec{
		Outcomes: []contract.ResultOutcome{
			contract.ResultOutcomeSuccess,
			contract.ResultOutcomeFailure,
		},
		DataSchema: json.RawMessage(`{
  "type":"object",
  "description":"不分组的字段聚合结果",
  "properties":{
    "results":{
      "type":"array",
      "description":"按数据版本返回的聚合结果批次",
      "items":{
        "type":"object",
        "properties":{
          "dataVersion":{"description":"参与统计的数据版本"},
          "deltaVersion":{"description":"聚合结果的增量版本"},
          "results":{
            "type":"array",
            "description":"请求中各统计项的结果",
            "items":{
              "type":"object",
              "properties":{
                "fieldId":{"type":"string","description":"被统计的字段 ID"},
                "statsType":{"type":"string","description":"实际执行的统计类型"},
                "value":{"description":"统计值；具体 JSON 类型取决于统计类型和字段类型"}
              },
              "required":["fieldId","statsType","value"],
              "additionalProperties":true
            }
          }
        },
        "required":["results"],
        "additionalProperties":true
      }
    }
  },
  "required":["results"],
  "additionalProperties":true
}`),
	}
}

func aitableGroupedStatsResultSpec() *contract.ResultSpec {
	return &contract.ResultSpec{
		Outcomes: []contract.ResultOutcome{
			contract.ResultOutcomeSuccess,
			contract.ResultOutcomeFailure,
		},
		DataSchema: json.RawMessage(`{
  "type":"object",
  "description":"分组或高级字段聚合结果",
  "properties":{
    "dataVersion":{"description":"参与统计的数据版本"},
    "results":{
      "type":"array",
      "description":"每个分组一条结果；未分组时通常只有一条",
      "items":{
        "type":"object",
        "properties":{
          "groupKeys":{
            "type":"array",
            "description":"当前结果的分组键",
            "items":{
              "type":"object",
              "properties":{
                "fieldId":{"type":"string","description":"分组字段 ID"},
                "value":{"description":"分组值；编码取决于字段类型"},
                "recordCount":{"type":"integer","description":"该分组包含的记录数"}
              },
              "additionalProperties":true
            }
          },
          "fieldStatsMap":{
            "type":"object",
            "description":"按字段 ID 索引的聚合值",
            "additionalProperties":{
              "type":"object",
              "properties":{
                "action":{"type":"string","description":"实际执行的统计动作"},
                "value":{"description":"统计值；具体 JSON 类型取决于统计动作和字段类型"}
              },
              "required":["action","value"],
              "additionalProperties":true
            }
          }
        },
        "required":["fieldStatsMap"],
        "additionalProperties":true
      }
    }
  },
  "required":["results"],
  "additionalProperties":true
}`),
	}
}
