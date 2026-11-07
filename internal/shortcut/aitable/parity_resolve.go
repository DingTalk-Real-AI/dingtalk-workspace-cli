// Copyright 2026 Alibaba Group
// SPDX-License-Identifier: Apache-2.0
package aitable

import (
	"encoding/json"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut/aitabletarget"
)

func childResolver(kind, description string) shortcut.Shortcut {
	command := "+resolve-" + kind
	return shortcut.Shortcut{Service: "aitable", Command: command, Product: serverMain, Description: description, Intent: description + "；按完整目录精确匹配，零命中或多命中均失败。", Risk: shortcut.RiskRead,
		Safety:   contract.SafetySpec{Effect: "read", Risk: "low", Confirmation: "not_required", Idempotency: "idempotent"},
		Contract: aitableCompositeContractWithResult(command, description, description+"并取得唯一稳定 ID 时", "已有准确 ID 可直接调用目标操作；不做模糊自动选择", "dws aitable "+command+" --base-id B --table-id T --name 标题", &contract.ResultSpec{Outcomes: []contract.ResultOutcome{contract.ResultOutcomeSuccess, contract.ResultOutcomeFailure}, DataSchema: json.RawMessage(`{"type":"object","properties":{"status":{"type":"string","description":"唯一匹配状态"},"entityType":{"type":"string","description":"field 或 view"},"query":{"type":"string","description":"精确名称"},"matchType":{"type":"string","description":"匹配方式"},"selected":{"type":"object","description":"唯一选中对象","properties":{"id":{"type":"string","description":"稳定 ID"},"name":{"type":"string","description":"名称"}},"required":["id","name"]}},"required":["status","entityType","query","matchType","selected"]}`)}),
		Flags:    []shortcut.Flag{{Name: "base-id", Type: shortcut.FlagString, Desc: "Base ID", Required: true}, {Name: "table-id", Type: shortcut.FlagString, Desc: "Table ID", Required: true}, {Name: "name", Type: shortcut.FlagString, Desc: "精确名称，重名时拒绝选择", Required: true}},
		Execute: func(rt *shortcut.RuntimeContext) error {
			r, err := aitabletarget.ResolveChildName(rt, rt.Str("base-id"), rt.Str("table-id"), kind, rt.Str("name"))
			if err != nil {
				return err
			}
			return rt.Output(r)
		},
	}
}
func init() {
	shortcut.Register(withAITableParityAliases(childResolver("field", "按精确名称唯一定位字段")), withAITableParityAliases(childResolver("view", "按精确名称唯一定位视图")))
}
