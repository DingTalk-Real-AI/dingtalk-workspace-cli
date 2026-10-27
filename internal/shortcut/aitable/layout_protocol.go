// Copyright 2026 Alibaba Group
// SPDX-License-Identifier: Apache-2.0

package aitable

import (
	"encoding/json"
	"fmt"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
)

var DashboardPreview = shortcut.Shortcut{
	Service: "aitable", Command: "+dashboard-preview", Product: serverMain, Description: "读取真实布局，预览服务端按行重排的逐卡位置和尺寸变化", Risk: shortcut.RiskRead,
	Intent: "仅在已确认 preview_dashboard_layout 正式部署，且用户明确考虑统一按行重排时使用；读取真实当前布局并返回同源计算的逐卡前后位置、尺寸和源布局摘要，未执行写入。参数回显不能代替真实预览，预览也不能证明页面可读性或原子 CAS；保留已有设计和混排时继续使用显式布局。",
	Safety: contract.SafetySpec{Effect: "read", Risk: "low", Confirmation: "not_required", Idempotency: "idempotent"}, OutputRollout: output.RolloutUnifiedActive,
	Contract: aitableCompositeContractWithResult("+dashboard-preview", "预览真实布局的按行重排计算", "用户明确考虑统一按行重排，且 preview_dashboard_layout 已正式部署，需要先看逐卡前后差异时", "保留已有设计或混排用显式布局；预览几何不能证明页面可读性或原子 CAS", `dws aitable +dashboard-preview --base-id B --dashboard-id D`, &contract.ResultSpec{Outcomes: []contract.ResultOutcome{contract.ResultOutcomeSuccess, contract.ResultOutcomeFailure}, DataSchema: json.RawMessage(`{"type":"object","properties":{"baseId":{"type":"string","description":"所属 Base"},"dashboardId":{"type":"string","description":"所属仪表盘"},"layoutFingerprint":{"type":"string","description":"真实源布局摘要，执行时传 expectedLayoutFingerprint"},"beforeLayout":{"type":"array","description":"计算前真实逐卡位置尺寸","items":{"type":"object","additionalProperties":true}},"layout":{"type":"array","description":"与执行同源计算的新位置尺寸","items":{"type":"object","additionalProperties":true}},"preview":{"type":"boolean","const":true,"description":"仅计算，未写入"}},"required":["baseId","dashboardId","layoutFingerprint","beforeLayout","layout","preview"]}`)}),
	Flags:    []shortcut.Flag{{Name: "base-id", Type: shortcut.FlagString, Desc: "Base ID", Required: true}, {Name: "dashboard-id", Type: shortcut.FlagString, Desc: "Dashboard ID", Required: true}, {Name: "is-app-mode", Type: shortcut.FlagBool, Desc: "已确认的应用模式只读上下文"}},
	Execute: func(rt *shortcut.RuntimeContext) error {
		if rt.DryRun() {
			return apperrors.NewValidation("真实预览需要读取当前布局，请正常调用只读 +dashboard-preview")
		}
		b, d := rt.Str("base-id"), rt.Str("dashboard-id")
		raw, err := rt.CallMCPData(serverHelper, "preview_dashboard_layout", map[string]any{"baseId": b, "dashboardId": d, "isAppMode": rt.Bool("is-app-mode")})
		if err != nil {
			return err
		}
		body := parityResponseObject(raw)
		if body["baseId"] != b || body["dashboardId"] != d || body["preview"] != true || stringValue(body, "layoutFingerprint") == "" {
			return fmt.Errorf("预览缺少真实 source 证据或目标不匹配")
		}
		if _, ok := findNamedObjectList(body, "beforeLayout"); !ok {
			return fmt.Errorf("预览缺少逐卡 beforeLayout")
		}
		if _, ok := findNamedObjectList(body, "layout"); !ok {
			return fmt.Errorf("预览缺少计算后的 layout")
		}
		return rt.Output(body)
	},
}

func init() { shortcut.Register(DashboardPreview) }
