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

// driveFolderStatefulExampleDispositions 明确记录文件夹镜像示例为什么只做契约校验：
// 预览必须读取用户指定的本地目录与真实钉盘目录，无法在隔离的 Agent 示例环境中伪造。
// DryRun 能力仍由 ContractFinal 正式发布，命令级测试负责执行并验证零写入预览。
func driveFolderStatefulExampleDispositions() []contract.ExampleDisposition {
	first, second := 0, 1
	return []contract.ExampleDisposition{
		{
			Index:      &first,
			Mode:       contract.ExampleDispositionModeContractOnly,
			ReasonCode: contract.ExampleDispositionReasonStatefulPreflight,
			Reason:     "预览需要读取用户指定的真实钉盘目录和本地文件系统状态",
			Reviewed:   true,
		},
		{
			Index:      &second,
			Mode:       contract.ExampleDispositionModeContractOnly,
			ReasonCode: contract.ExampleDispositionReasonStatefulPreflight,
			Reason:     "预览需要读取用户指定的真实钉盘目录和本地文件系统状态",
			Reviewed:   true,
		},
	}
}

func driveFolderStatusResultSpec() *contract.ResultSpec {
	return &contract.ResultSpec{
		Outcomes: []contract.ResultOutcome{
			contract.ResultOutcomeSuccess,
			contract.ResultOutcomeFailure,
		},
		DataSchema: json.RawMessage(`{
  "type":"object",
  "description":"本地文件夹与钉盘文件夹的差异",
  "properties":{
    "detection":{"type":"string","description":"差异检测模式","enum":["exact","quick"]},
    "new_local":{"type":"array","description":"仅本地存在的文件","items":{"type":"object","properties":{"rel_path":{"type":"string","description":"相对文件路径"}},"required":["rel_path"],"additionalProperties":false}},
    "new_remote":{"type":"array","description":"仅钉盘存在的文件","items":{"type":"object","properties":{"rel_path":{"type":"string","description":"相对文件路径"}},"required":["rel_path"],"additionalProperties":false}},
    "modified":{"type":"array","description":"双端都存在但内容或时间不同的文件","items":{"type":"object","properties":{"rel_path":{"type":"string","description":"相对文件路径"}},"required":["rel_path"],"additionalProperties":false}},
    "unchanged":{"type":"array","description":"双端一致的文件","items":{"type":"object","properties":{"rel_path":{"type":"string","description":"相对文件路径"}},"required":["rel_path"],"additionalProperties":false}},
    "unknown":{"type":"array","description":"exact 模式下因缺少可靠远端哈希而无法判定的文件","items":{"type":"object","properties":{"rel_path":{"type":"string","description":"相对文件路径"}},"required":["rel_path"],"additionalProperties":false}}
  },
  "required":["detection","new_local","new_remote","modified","unchanged","unknown"],
  "additionalProperties":false
}`),
	}
}

func driveFolderPullResultSpec() *contract.ResultSpec {
	return &contract.ResultSpec{
		Outcomes: []contract.ResultOutcome{
			contract.ResultOutcomeSuccess,
			contract.ResultOutcomePartialFailure,
			contract.ResultOutcomeFailure,
		},
		DataSchema: json.RawMessage(`{
  "type":"object",
  "description":"钉盘文件夹拉取的执行结果或预览计划",
  "properties":{
    "summary":{"type":"object","description":"真实执行的动作计数","properties":{"downloaded":{"type":"integer","description":"成功下载的文件数"},"skipped":{"type":"integer","description":"按策略跳过的文件数"},"failed":{"type":"integer","description":"失败的文件数"}},"required":["downloaded","skipped","failed"],"additionalProperties":false},
    "items":{"type":"array","description":"真实执行的逐文件结果","items":{"type":"object","properties":{"rel_path":{"type":"string","description":"相对文件路径"},"action":{"type":"string","description":"执行动作"},"error":{"type":"string","description":"失败原因"}},"required":["rel_path","action"],"additionalProperties":false}},
    "dry_run":{"type":"boolean","description":"是否为只读预览"},
    "executed":{"type":"boolean","description":"是否执行了写操作"},
    "preview_kind":{"type":"string","description":"预览类型","enum":["plan"]},
    "operation":{"type":"string","description":"预览的命令"},
    "if_exists":{"type":"string","description":"本地同名文件处理策略"},
    "plan":{"type":"object","description":"预览的拉取计划","properties":{"summary":{"type":"object","description":"计划动作计数","properties":{"downloaded":{"type":"integer","description":"计划下载的文件数"},"skipped":{"type":"integer","description":"计划跳过的文件数"},"failed":{"type":"integer","description":"预检失败的文件数"}},"required":["downloaded","skipped","failed"],"additionalProperties":false},"items":{"type":"array","description":"计划中的逐文件动作","items":{"type":"object","properties":{"rel_path":{"type":"string","description":"相对文件路径"},"action":{"type":"string","description":"计划动作"},"error":{"type":"string","description":"预检失败原因"}},"required":["rel_path","action"],"additionalProperties":false}}},"required":["summary","items"],"additionalProperties":false}
  },
  "additionalProperties":false
}`),
	}
}

func driveFolderPushResultSpec() *contract.ResultSpec {
	return &contract.ResultSpec{
		Outcomes: []contract.ResultOutcome{
			contract.ResultOutcomeSuccess,
			contract.ResultOutcomePartialFailure,
			contract.ResultOutcomeFailure,
		},
		DataSchema: json.RawMessage(`{
  "type":"object",
  "description":"本地文件夹推送的执行结果或预览计划",
  "properties":{
    "summary":{"type":"object","description":"真实执行的动作计数","properties":{"uploaded":{"type":"integer","description":"成功上传或覆盖的文件数"},"skipped":{"type":"integer","description":"按策略跳过的文件数"},"failed":{"type":"integer","description":"失败的条目数"},"aborted":{"type":"boolean","description":"是否因失败中止后续上传"}},"required":["uploaded","skipped","failed","aborted"],"additionalProperties":false},
    "items":{"type":"array","description":"真实执行的逐条目结果","items":{"type":"object","properties":{"rel_path":{"type":"string","description":"相对文件或目录路径"},"action":{"type":"string","description":"执行动作"},"size_bytes":{"type":"integer","description":"文件字节数"},"error":{"type":"string","description":"失败原因"}},"required":["rel_path","action"],"additionalProperties":false}},
    "dry_run":{"type":"boolean","description":"是否为只读预览"},
    "executed":{"type":"boolean","description":"是否执行了写操作"},
    "preview_kind":{"type":"string","description":"预览类型","enum":["plan"]},
    "operation":{"type":"string","description":"预览的命令"},
    "if_exists":{"type":"string","description":"远端同名文件处理策略"},
    "plan":{"type":"object","description":"预览的推送计划","properties":{"summary":{"type":"object","description":"计划动作计数；兼容执行计数 uploaded/skipped 在预览中保持为零","properties":{"uploaded":{"type":"integer","description":"兼容字段；预览未执行上传，固定为零"},"skipped":{"type":"integer","description":"兼容字段；预览未执行跳过，固定为零"},"failed":{"type":"integer","description":"预检失败的条目数"},"aborted":{"type":"boolean","description":"计划是否会中止"},"planned_uploads":{"type":"integer","description":"计划上传或覆盖的文件数"},"planned_skips":{"type":"integer","description":"计划按策略跳过的文件数"},"planned_folders":{"type":"integer","description":"计划创建的远端目录数"}},"required":["uploaded","skipped","failed","aborted"],"additionalProperties":false},"items":{"type":"array","description":"计划中的逐条目动作","items":{"type":"object","properties":{"rel_path":{"type":"string","description":"相对文件或目录路径"},"action":{"type":"string","description":"未执行的 planned_* 计划动作或已确认的预检失败"},"size_bytes":{"type":"integer","description":"文件字节数"},"error":{"type":"string","description":"预检失败原因"}},"required":["rel_path","action"],"additionalProperties":false}}},"required":["summary","items"],"additionalProperties":false}
  },
  "additionalProperties":false
}`),
	}
}

func driveFolderSyncResultSpec() *contract.ResultSpec {
	return &contract.ResultSpec{
		Outcomes: []contract.ResultOutcome{
			contract.ResultOutcomeSuccess,
			contract.ResultOutcomePartialFailure,
			contract.ResultOutcomeFailure,
		},
		DataSchema: json.RawMessage(`{
  "type":"object",
  "description":"本地与钉盘双向同步的执行结果或预览计划",
  "properties":{
    "detection":{"type":"string","description":"差异检测模式","enum":["exact","quick"]},
    "diff":{"type":"object","description":"同步前的双端差异","properties":{"new_local":{"type":"array","description":"仅本地存在的文件","items":{"type":"object","properties":{"rel_path":{"type":"string","description":"相对文件路径"}},"required":["rel_path"],"additionalProperties":false}},"new_remote":{"type":"array","description":"仅钉盘存在的文件","items":{"type":"object","properties":{"rel_path":{"type":"string","description":"相对文件路径"}},"required":["rel_path"],"additionalProperties":false}},"modified":{"type":"array","description":"双端都存在但内容或时间不同的文件","items":{"type":"object","properties":{"rel_path":{"type":"string","description":"相对文件路径"}},"required":["rel_path"],"additionalProperties":false}},"unchanged":{"type":"array","description":"双端一致的文件","items":{"type":"object","properties":{"rel_path":{"type":"string","description":"相对文件路径"}},"required":["rel_path"],"additionalProperties":false}},"unknown":{"type":"array","description":"exact 模式下因缺少可靠远端哈希而无法判定的文件","items":{"type":"object","properties":{"rel_path":{"type":"string","description":"相对文件路径"}},"required":["rel_path"],"additionalProperties":false}}},"required":["new_local","new_remote","modified","unchanged","unknown"],"additionalProperties":false},
    "summary":{"type":"object","description":"真实执行的动作计数","properties":{"pulled":{"type":"integer","description":"成功拉取的条目数"},"pushed":{"type":"integer","description":"成功推送的条目数"},"skipped":{"type":"integer","description":"跳过的条目数"},"failed":{"type":"integer","description":"失败的条目数"}},"required":["pulled","pushed","skipped","failed"],"additionalProperties":false},
    "items":{"type":"array","description":"真实执行的逐条目结果","items":{"type":"object","properties":{"rel_path":{"type":"string","description":"相对文件或目录路径"},"action":{"type":"string","description":"执行动作"},"direction":{"type":"string","description":"同步方向"},"error":{"type":"string","description":"失败或跳过原因"}},"required":["rel_path","action"],"additionalProperties":false}},
    "dry_run":{"type":"boolean","description":"是否为只读预览","const":true},
    "executed":{"type":"boolean","description":"是否执行了写操作","const":false},
    "preview_kind":{"type":"string","description":"预览类型","enum":["plan"]},
    "operation":{"type":"string","description":"预览的命令","const":"drive sync"},
    "plan":{"type":"object","description":"预览的同步计划","properties":{"detection":{"type":"string","description":"计划使用的差异检测模式","enum":["exact","quick"]},"diff":{"type":"object","description":"计划执行前的双端差异","properties":{"new_local":{"type":"array","description":"仅本地存在的文件","items":{"type":"object","properties":{"rel_path":{"type":"string","description":"相对文件路径"}},"required":["rel_path"],"additionalProperties":false}},"new_remote":{"type":"array","description":"仅钉盘存在的文件","items":{"type":"object","properties":{"rel_path":{"type":"string","description":"相对文件路径"}},"required":["rel_path"],"additionalProperties":false}},"modified":{"type":"array","description":"双端都存在但内容或时间不同的文件","items":{"type":"object","properties":{"rel_path":{"type":"string","description":"相对文件路径"}},"required":["rel_path"],"additionalProperties":false}},"unchanged":{"type":"array","description":"双端一致的文件","items":{"type":"object","properties":{"rel_path":{"type":"string","description":"相对文件路径"}},"required":["rel_path"],"additionalProperties":false}},"unknown":{"type":"array","description":"exact 模式下因缺少可靠远端哈希而无法判定的文件","items":{"type":"object","properties":{"rel_path":{"type":"string","description":"相对文件路径"}},"required":["rel_path"],"additionalProperties":false}}},"required":["new_local","new_remote","modified","unchanged","unknown"],"additionalProperties":false},"summary":{"type":"object","description":"计划动作计数；兼容执行计数 pulled/pushed/skipped 在预览中保持为零","properties":{"pulled":{"type":"integer","description":"兼容字段；预览未执行拉取，固定为零"},"pushed":{"type":"integer","description":"兼容字段；预览未执行推送，固定为零"},"skipped":{"type":"integer","description":"兼容字段；预览未执行跳过，固定为零"},"failed":{"type":"integer","description":"预检失败的条目数"},"planned_pulls":{"type":"integer","description":"计划拉取的文件数"},"planned_pushes":{"type":"integer","description":"计划上传或覆盖的文件数"},"planned_skips":{"type":"integer","description":"计划按策略跳过的文件数"},"planned_folders":{"type":"integer","description":"计划创建的远端目录数"}},"required":["pulled","pushed","skipped","failed"],"additionalProperties":false},"items":{"type":"array","description":"计划中的逐条目动作","items":{"type":"object","properties":{"rel_path":{"type":"string","description":"相对文件或目录路径"},"action":{"type":"string","description":"未执行的 planned_* 计划动作、待决策动作或预检失败"},"direction":{"type":"string","description":"同步方向"},"error":{"type":"string","description":"失败或跳过原因"}},"required":["rel_path","action"],"additionalProperties":false}}},"required":["detection","diff","summary","items"],"additionalProperties":false}
  },
  "oneOf":[
    {"required":["detection","diff","summary","items"]},
    {"required":["dry_run","executed","preview_kind","operation","plan"]}
  ],
  "additionalProperties":false
}`),
	}
}
