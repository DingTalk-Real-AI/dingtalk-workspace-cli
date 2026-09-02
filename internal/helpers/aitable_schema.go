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
	// Historical DWS helper commands are now published by the public aitable
	// snapshot. Keep the constructor name to avoid churn at every call site.
	return aitableMCPInterface(rpc)
}

func aitableCompositeInterface(reason string) *contract.InterfaceSpec {
	return &contract.InterfaceSpec{
		Mode:         "composite",
		Availability: "available",
		Reason:       reason,
	}
}

func aitableDashboardGetResultSpec() *contract.ResultSpec {
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

func aitableRunAIFieldResultSpec() *contract.ResultSpec {
	return &contract.ResultSpec{
		Outcomes: []contract.ResultOutcome{contract.ResultOutcomeSuccess, contract.ResultOutcomeFailure},
		DataSchema: json.RawMessage(`{
  "type":"object","description":"run_ai_field 的 MCP 响应",
  "properties":{"data":{"type":"object","description":"AI 字段任务提交结果","properties":{
    "tasks":{"type":"array","description":"各 AI 字段的独立提交结果","items":{"type":"object","properties":{
      "fieldId":{"type":"string","description":"AI 字段 ID"},
      "taskId":{"type":"string","description":"提交成功后的任务 ID"},
      "status":{"type":"string","description":"submitted 或 conflict"},
      "total":{"type":"integer","description":"任务涉及的记录数"},
      "error":{"type":"string","description":"冲突或提交失败时的错误信息"}
    },"required":["fieldId","status"],"additionalProperties":true}},
    "documentUrl":{"type":"string","description":"查看运行进度和结果的文档链接"}
  },"required":["tasks"],"additionalProperties":true}},
  "required":["data"],"additionalProperties":true
}`),
	}
}

func aitableRecordIDsResultSpec() *contract.ResultSpec {
	return &contract.ResultSpec{
		Outcomes: []contract.ResultOutcome{contract.ResultOutcomeSuccess, contract.ResultOutcomeFailure},
		DataSchema: json.RawMessage(`{
  "type":"object","description":"query_record_ids 的 MCP 响应",
  "properties":{"data":{"type":"object","description":"当前页记录 ID 与续传游标","properties":{
    "recordIds":{"type":"array","description":"按表内顺序返回的记录 ID","items":{"type":"string"}},
    "nextCursor":{"type":"string","description":"下一页游标；为空表示扫描完成"}
  },"required":["recordIds"],"additionalProperties":true}},
  "required":["data"],"additionalProperties":true
}`),
	}
}

func aitableCreateSubRecordsResultSpec() *contract.ResultSpec {
	return &contract.ResultSpec{
		Outcomes: []contract.ResultOutcome{contract.ResultOutcomeSuccess, contract.ResultOutcomeFailure},
		DataSchema: json.RawMessage(`{
  "type":"object","description":"create_sub_records 的 MCP 响应",
  "properties":{"data":{"type":"object","description":"子记录创建结果","properties":{
    "recordIds":{"type":"array","description":"按输入顺序返回的子记录 ID","items":{"type":"string"}},
    "hierarchyFieldId":{"type":"string","description":"实际使用的层级关联字段 ID"},
    "parentRecordId":{"type":"string","description":"父记录 ID 回显"}
  },"required":["recordIds","hierarchyFieldId","parentRecordId"],"additionalProperties":true}},
  "required":["data"],"additionalProperties":true
}`),
	}
}

func aitableSubmitFormResultSpec() *contract.ResultSpec {
	return &contract.ResultSpec{
		Outcomes: []contract.ResultOutcome{contract.ResultOutcomeSuccess, contract.ResultOutcomeFailure},
		DataSchema: json.RawMessage(`{
  "type":"object","description":"submit_form 的 MCP 响应",
  "properties":{"data":{"type":"object","description":"表单提交后生成的记录标识","properties":{
    "baseId":{"type":"string","description":"Base ID 回显"},
    "tableId":{"type":"string","description":"Table ID 回显"},
    "viewId":{"type":"string","description":"表单视图 ID 回显"},
    "rowId":{"type":"string","description":"提交后生成的记录 ID"},
    "version":{"type":"integer","description":"提交后的数据版本号"}
  },"required":["baseId","tableId","viewId","rowId","version"],"additionalProperties":true}},
  "required":["data"],"additionalProperties":true
}`),
	}
}

func aitableRemoveAttachmentsResultSpec() *contract.ResultSpec {
	return &contract.ResultSpec{
		Outcomes: []contract.ResultOutcome{contract.ResultOutcomeSuccess, contract.ResultOutcomeFailure},
		DataSchema: json.RawMessage(`{
  "type":"object","description":"remove_attachments 的 MCP 响应",
  "properties":{"data":{"type":"object","description":"附件删除结果","properties":{
    "removedCount":{"type":"integer","description":"本次删除的附件总数"},
    "affectedFieldIds":{"type":"array","description":"发生删除或清空的附件字段 ID","items":{"type":"string"}}
  },"required":["removedCount","affectedFieldIds"],"additionalProperties":true}},
  "required":["data"],"additionalProperties":true
}`),
	}
}

func aitableCreateFormResultSpec() *contract.ResultSpec {
	return &contract.ResultSpec{
		Outcomes: []contract.ResultOutcome{contract.ResultOutcomeSuccess, contract.ResultOutcomeFailure},
		DataSchema: json.RawMessage(`{
  "type":"object","description":"create_form_view 的 MCP 响应",
  "properties":{"data":{"type":"object","description":"新建表单视图的标识","properties":{
    "baseId":{"type":"string","description":"Base ID 回显"},
    "tableId":{"type":"string","description":"Table ID 回显"},
    "viewId":{"type":"string","description":"新建的表单视图 ID"}
  },"required":["baseId","tableId","viewId"],"additionalProperties":true}},
  "required":["data"],"additionalProperties":true
}`),
	}
}

func aitableNotifyShareFormResultSpec() *contract.ResultSpec {
	return &contract.ResultSpec{
		Outcomes: []contract.ResultOutcome{contract.ResultOutcomeSuccess, contract.ResultOutcomeFailure},
		DataSchema: json.RawMessage(`{
  "type":"object","description":"notify_share_form_recipients 的 MCP 响应",
  "properties":{"data":{"type":"object","description":"表单通知发送结果","properties":{
    "baseId":{"type":"string","description":"Base ID 回显"},
    "tableId":{"type":"string","description":"Table ID 回显"},
    "viewId":{"type":"string","description":"表单视图 ID 回显"},
    "recipients":{"type":"array","description":"实际接收通知的用户 ID","items":{"type":"string"}},
    "sentAt":{"type":"string","description":"服务端记录的发送时间"}
  },"required":["baseId","tableId","viewId","recipients","sentAt"],"additionalProperties":true}},
  "required":["data"],"additionalProperties":true
}`),
	}
}

func aitableHiddenFieldsResultSpec() *contract.ResultSpec {
	return &contract.ResultSpec{
		Outcomes: []contract.ResultOutcome{contract.ResultOutcomeSuccess, contract.ResultOutcomeFailure},
		DataSchema: json.RawMessage(`{
  "type":"object","description":"get_hidden_fields_of_view 的 MCP 响应",
  "properties":{"data":{"type":"object","description":"视图隐藏字段结果","properties":{
    "baseId":{"type":"string","description":"Base ID 回显"},
    "tableId":{"type":"string","description":"Table ID 回显"},
    "viewId":{"type":"string","description":"视图 ID 回显"},
    "hiddenFieldIds":{"type":"array","description":"当前视图隐藏的字段 ID","items":{"type":"string"}}
  },"required":["baseId","tableId","viewId","hiddenFieldIds"],"additionalProperties":true}},
  "required":["data"],"additionalProperties":true
}`),
	}
}
