// Copyright 2026 Alibaba Group
// SPDX-License-Identifier: Apache-2.0

package aitable

import (
	"encoding/json"
	"strings"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
)

func aitableCompositeContractWithResult(command, description, useWhen, avoidWhen, example string, result *contract.ResultSpec) corecmd.ContractDecl {
	declaration := aitableCompositeContract(command, description, useWhen, avoidWhen, example)
	declaration.Result = result
	return declaration
}

func aitableTableBootstrapResultSpec() *contract.ResultSpec {
	return &contract.ResultSpec{
		Outcomes: []contract.ResultOutcome{
			contract.ResultOutcomeSuccess,
			contract.ResultOutcomeFailure,
		},
		DataSchema: json.RawMessage(`{
			"type":"object",
			"description":"AI Table 数据表初始化的执行计划、进度、验证证据和恢复信息",
			"properties":{
				"contractVersion":{"type":"string","description":"复合操作业务回执版本","const":"aitable.composite.v1"},
				"operation":{"type":"string","description":"复合操作标识","const":"table_bootstrap"},
				"status":{"type":"string","description":"业务执行状态；失败时同结构位于 error.details.result","enum":["success","planned","partial_success","unknown"]},
				"executed":{"type":"boolean","description":"是否已开始执行远端操作"},
				"retryable":{"type":"boolean","description":"当前回执是否允许直接重试"},
				"requestedCount":{"type":"integer","description":"请求创建的字段数量","minimum":0},
				"completedCount":{"type":"integer","description":"已完成并验证的字段数量","minimum":0},
				"failedCount":{"type":"integer","description":"已确认失败的字段数量","minimum":0},
				"resolved":{"type":"object","description":"已解析的 Base、数据表名称和稳定 ID","additionalProperties":true},
				"plan":{"type":"array","description":"dry-run 或执行前生成的有序操作计划","items":{"type":"object","description":"一个计划步骤","properties":{"index":{"type":"integer","description":"步骤序号"},"name":{"type":"string","description":"步骤名称"},"tool":{"type":"string","description":"步骤使用的 MCP 工具"},"status":{"type":"string","description":"步骤状态"},"offset":{"type":"integer","description":"分片起始偏移"},"count":{"type":"integer","description":"步骤处理数量"},"arguments":{"type":"object","description":"步骤参数摘要","additionalProperties":true},"result":{"type":"object","description":"步骤结果摘要","additionalProperties":true},"error":{"type":"string","description":"步骤错误摘要"}},"required":["index","name","status"],"additionalProperties":false}},
				"completedSteps":{"type":"array","description":"已完成步骤及其验证摘要","items":{"type":"object","description":"一个已完成步骤","properties":{"index":{"type":"integer","description":"步骤序号"},"name":{"type":"string","description":"步骤名称"},"tool":{"type":"string","description":"步骤使用的 MCP 工具"},"status":{"type":"string","description":"步骤状态"},"offset":{"type":"integer","description":"分片起始偏移"},"count":{"type":"integer","description":"步骤处理数量"},"arguments":{"type":"object","description":"步骤参数摘要","additionalProperties":true},"result":{"type":"object","description":"步骤结果摘要","additionalProperties":true},"error":{"type":"string","description":"步骤错误摘要"}},"required":["index","name","status"],"additionalProperties":false}},
				"verification":{"type":"object","description":"创建后读回验证证据","additionalProperties":true},
				"checkpoint":{"type":"object","description":"失败或部分成功后的恢复检查点","additionalProperties":true},
				"nextCommand":{"type":"string","description":"用于核验或恢复的下一条可执行命令"},
				"knownSideEffects":{"type":"array","description":"已确认发生的远端副作用","items":{"type":"object","description":"一个已确认副作用","additionalProperties":true}},
				"warnings":{"type":"array","description":"不阻断最终验证的警告信息","items":{"type":"string"}},
				"result":{"type":"object","description":"成功时创建并验证的数据表结构","properties":{"baseId":{"type":"string","description":"目标 Base ID"},"tableId":{"type":"string","description":"新数据表稳定 ID"},"tableName":{"type":"string","description":"新数据表名称"},"fields":{"type":"array","description":"读回验证后的字段结构","items":{"type":"object","description":"一个已验证字段","additionalProperties":true}}},"required":["baseId","tableId","tableName","fields"],"additionalProperties":true}
			},
			"required":["contractVersion","operation","status","executed","retryable"],
			"additionalProperties":false
		}`),
	}
}

func aitableCompositeContract(command, description, useWhen, avoidWhen, example string) corecmd.ContractDecl {
	name := strings.TrimPrefix(command, "+")
	name = strings.ReplaceAll(name, "-", "_")
	cliPath := "aitable " + command
	return corecmd.ContractDecl{
		Identity: contract.ToolIdentitySpec{
			ProductID:      "aitable",
			Name:           "shortcut_" + name,
			CanonicalPath:  "aitable.shortcut_" + name,
			CLIPath:        cliPath,
			PrimaryCLIPath: cliPath,
		},
		Description: description,
		Interface: &contract.InterfaceSpec{
			Mode:         "composite",
			Availability: "available",
			Reason:       "DWS reviewed composite orchestration with explicit read-back success predicates, partial-effect reporting, and resumable checkpoints; no single MCP operation represents the full command contract.",
		},
		Selection: contract.SelectionSpec{
			AgentSummary: description,
			UseWhen:      []string{useWhen},
			AvoidWhen:    []string{avoidWhen},
			Examples:     []string{example},
		},
	}
}
