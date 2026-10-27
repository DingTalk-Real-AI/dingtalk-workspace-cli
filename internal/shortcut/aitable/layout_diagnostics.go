// Copyright 2026 Alibaba Group
// SPDX-License-Identifier: Apache-2.0

package aitable

import (
	"encoding/json"
	"fmt"
	"strings"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/aitableprotocol"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
)

var DashboardDiagnose = shortcut.Shortcut{
	Service: "aitable", Command: "+dashboard-diagnose", Product: serverMain,
	Description: "只读检查根布局几何、指定区域覆盖及声明设计差异", Risk: shortcut.RiskRead,
	Intent:        "需要只读检查当前仪表盘根布局越界、重叠和布局中已声明的 minW/minH 时使用。geometryVerified 仅表示这些几何检查通过，仅在显式 expected-layout 时比较完整设计，regions 时核验区域覆盖；不证明组件最小尺寸规则或页面文字和图例可读；pageReadableVerified 始终为 false。该命令不修改布局。",
	Safety:        contract.SafetySpec{Effect: "read", Risk: "low", Confirmation: "not_required", Idempotency: "idempotent"},
	OutputRollout: output.RolloutUnifiedActive,
	Contract:      aitableCompositeContractWithResult("+dashboard-diagnose", "只读检查根布局几何与已声明 minW/minH；不证明符合设计或页面可读", "需要在当前仪表盘完整根布局上只读检查越界、重叠或已声明 minW/minH 时", "组件最小尺寸、子容器布局及页面文字和图例可读性需单独验收；几何通过不能替代这些结论；保留设计时不调用 arrange", `dws aitable +dashboard-diagnose --base-id B --dashboard-id D`, &contract.ResultSpec{Outcomes: []contract.ResultOutcome{contract.ResultOutcomeSuccess, contract.ResultOutcomeFailure}, DataSchema: json.RawMessage(`{"type":"object","description":"当前根布局几何、完整声明设计差异与指定区域覆盖；不证明页面可读","properties":{"baseId":{"type":"string","description":"目标 Base"},"dashboardId":{"type":"string","description":"目标仪表盘"},"totalColumns":{"type":"integer","description":"经原始类型核准的根网格列数，不推测子容器列数"},"geometryVerified":{"type":"boolean","description":"根布局越界、重叠及已声明 minW/minH 检查是否通过；不证明组件尺寸规则、设计符合性或页面可读"},"pageReadableVerified":{"type":"boolean","const":false,"description":"未执行页面文字和图例可读性验收，始终 false"},"issues":{"type":"array","description":"根几何或已声明约束问题","items":{"type":"object","properties":{"chartId":{"type":"string","description":"问题布局项 ID；缺失身份时为空"},"otherId":{"type":"string","description":"重叠的另一布局项 ID，非重叠问题可省略"},"code":{"type":"string","description":"根布局身份、几何、约束或重叠问题代码"},"message":{"type":"string","description":"问题说明"}},"required":["chartId","code","message"],"additionalProperties":true}},"designVerified":{"type":"boolean","description":"仅传 expected-layout 时返回，真实全部根卡片与完整声明几何一致"},"designIssues":{"type":"array","description":"实际布局与设计之间的缺失、额外、重复或几何差异","items":{"type":"object","additionalProperties":true}},"regionCoverageVerified":{"type":"boolean","description":"仅传 regions 时返回，所选合法根卡片并集是否覆盖全部指定区域"},"regions":{"type":"array","description":"逐区域覆盖证据，重叠面积不重复计数","items":{"type":"object","properties":{"id":{"type":"string","description":"输入区域 ID"},"covered":{"type":"boolean","description":"选中卡片并集是否完整覆盖区域"},"uncoveredColumns":{"type":"array","description":"存在未覆盖垂直区间的根网格列","items":{"type":"integer"}}},"required":["id","covered","uncoveredColumns"]}},"excludedChildItems":{"type":"integer","description":"未套用根坐标规则的子容器布局项数量，未核验子布局"}},"required":["baseId","dashboardId","totalColumns","geometryVerified","pageReadableVerified","issues"]}`)}),
	Flags:         []shortcut.Flag{{Name: "base-id", Type: shortcut.FlagString, Desc: "Base ID", Required: true}, {Name: "dashboard-id", Type: shortcut.FlagString, Desc: "Dashboard ID", Required: true}, {Name: "expected-layout", Type: shortcut.FlagString, Desc: "可选完整设计 JSON 数组，每项 i/x/y/w/h 必填；比较全部根卡片，保持叠放设计"}, {Name: "regions", Type: shortcut.FlagString, Desc: "可选区域 JSON 数组，每项 id/x/y/w/h 必填，chartIds 可选；核验选中根卡片的并集覆盖"}, {Name: "is-app-mode", Type: shortcut.FlagBool, Desc: "已确认的应用模式上下文"}},
	Execute: func(rt *shortcut.RuntimeContext) error {
		b, d := rt.Str("base-id"), rt.Str("dashboard-id")
		raw, err := rt.CallMCPData(serverMain, "get_dashboard", map[string]any{"baseId": b, "dashboardId": d})
		if err != nil {
			return err
		}
		columns, err := aitableprotocol.ResolveDashboardRootColumns(raw, b, d, rt.Bool("is-app-mode"))
		if err != nil {
			return err
		}
		items, found := findNamedObjectList(raw, "layout")
		if !found {
			return fmt.Errorf("get_dashboard 未返回完整 layout，不能诊断")
		}
		rootItems := make([]map[string]any, 0, len(items))
		for _, item := range items {
			parent, _ := item["parentId"].(string)
			if parent == "" || parent == aitableprotocol.RootResponsiveLayoutParent {
				rootItems = append(rootItems, item)
			}
		}
		excluded := len(items) - len(rootItems)
		items = rootItems
		issues := aitableprotocol.DiagnoseRootLayout(items, columns)
		result := map[string]any{"baseId": b, "dashboardId": d, "totalColumns": columns, "geometryVerified": len(issues) == 0, "pageReadableVerified": false, "issues": issues}
		result["excludedChildItems"] = excluded
		if rt.Command().Flags().Changed("expected-layout") {
			var expected []map[string]any
			if err := json.Unmarshal([]byte(rt.Str("expected-layout")), &expected); err != nil || expected == nil || len(expected) > 500 {
				return apperrors.NewValidation("expected-layout 必须为最多 500 项的完整设计数组")
			}
			differences, err := aitableprotocol.CompareRootLayout(items, expected, columns)
			if err != nil {
				return apperrors.NewValidation(err.Error())
			}
			result["designVerified"], result["designIssues"] = len(differences) == 0, differences
		}
		if rt.Command().Flags().Changed("regions") {
			var input []map[string]any
			var regions []aitableprotocol.LayoutRegion
			raw := []byte(rt.Str("regions"))
			if err := json.Unmarshal(raw, &input); err != nil || len(input) == 0 || len(input) > 20 {
				return apperrors.NewValidation("regions 必须为 1～20 项区域数组")
			}
			for _, region := range input {
				if err := aitableprotocol.ValidateRootChartLayout(region, columns); err != nil {
					return apperrors.NewValidation(err.Error())
				}
			}
			decoder := json.NewDecoder(strings.NewReader(string(raw)))
			decoder.DisallowUnknownFields()
			if err := decoder.Decode(&regions); err != nil {
				return apperrors.NewValidation("regions 字段类型无效")
			}
			coverage, err := aitableprotocol.CheckLayoutRegions(items, regions, columns)
			if err != nil {
				return apperrors.NewValidation(err.Error())
			}
			covered := true
			for _, region := range coverage {
				covered = covered && region.Covered
			}
			result["regionCoverageVerified"], result["regions"] = covered, coverage
		}
		return rt.Output(result)
	},
}

func init() { shortcut.Register(DashboardDiagnose) }
