// Copyright 2026 Alibaba Group
// SPDX-License-Identifier: Apache-2.0

package aitable

import (
	"encoding/json"
	"fmt"
	"strconv"
	"sync"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
)

var aitableIndependentReads = map[string]bool{"get_base": true, "get_tables": true, "get_fields": true, "get_views": true, "get_dashboard": true, "get_chart": true, "query_records": true, "get_ai_field_task_status": true}

var ReadBatch = shortcut.Shortcut{
	Service: "aitable", Command: "+read-batch", Product: serverMain, Description: "集中执行 1～20 个独立只读查询，可显式启用最多 4 路并发", Risk: shortcut.RiskRead,
	Intent: "已有同一 Base 的 1～20 个无依赖只读请求时使用；每项 arguments 显式绑定同一 Base，按原输入顺序保留完整响应。默认串行，确认请求独立后可显式启用最多 4 路并发；并发减少等待，不减少远端请求总数。依赖前项 ID 的读取先完成绑定，条件写、写工具和本地汇总计算不进入此命令。",
	Safety: contract.SafetySpec{Effect: "read", Risk: "low", Confirmation: "not_required", Idempotency: "idempotent"}, OutputRollout: output.RolloutUnifiedActive,
	Contract: aitableCompositeContractWithResult("+read-batch", "集中读取独立对象，按输入顺序保留逐项结果", "已有同一 Base 的一组无依赖只读查询，需要集中返回并减少交互时", "依赖前项 ID 的读取先完成绑定；app get 等条件写与所有写工具禁止进入本命令", `dws aitable +read-batch --base-id B --requests '[{"tool":"get_tables","arguments":{"baseId":"B","tableIds":["T"]}}]' --concurrency 1`, &contract.ResultSpec{Outcomes: []contract.ResultOutcome{contract.ResultOutcomeSuccess, contract.ResultOutcomeFailure}, DataSchema: json.RawMessage(`{"type":"object","properties":{"baseId":{"type":"string","description":"固定目标 Base"},"items":{"type":"array","description":"按请求顺序返回的完整结果，不是查询摘要","items":{"type":"object","additionalProperties":true}},"requests":{"type":"integer","description":"远端请求数；并发不减少请求总数"},"concurrency":{"type":"integer","description":"最大并发数，1～4，默认串行"}},"required":["baseId","items","requests","concurrency"]}`)}),
	Flags:    []shortcut.Flag{{Name: "base-id", Type: shortcut.FlagString, Desc: "固定 Base ID", Required: true}, {Name: "requests", Type: shortcut.FlagString, Desc: "1～20 项只读请求 JSON，tool 和 arguments 必填", Required: true}, {Name: "concurrency", Type: shortcut.FlagString, Default: "1", Desc: "最大并发 1～4，确认独立读取后显式启用"}},
	Execute: func(rt *shortcut.RuntimeContext) error {
		if rt.DryRun() {
			return apperrors.NewValidation("批量读取需要真实只读查询；dry-run 不能产生读取证据")
		}
		var requests []struct {
			Tool      string         `json:"tool"`
			Arguments map[string]any `json:"arguments"`
		}
		if err := json.Unmarshal([]byte(rt.Str("requests")), &requests); err != nil || len(requests) == 0 || len(requests) > 20 {
			return apperrors.NewValidation("requests 必须包含 1～20 项 JSON 请求")
		}
		width, err := strconv.Atoi(rt.Str("concurrency"))
		if err != nil || width < 1 || width > 4 {
			return apperrors.NewValidation("concurrency 必须为 1～4")
		}
		for _, request := range requests {
			if !aitableIndependentReads[request.Tool] || request.Arguments["baseId"] != rt.Str("base-id") {
				return apperrors.NewValidation("只允许显式名单中的独立读取，且所有请求必须固定到同一 Base")
			}
		}
		items := make([]map[string]any, len(requests))
		errs := make([]error, len(requests))
		indices := make(chan int)
		var workers sync.WaitGroup
		for i := 0; i < min(width, len(requests)); i++ {
			workers.Add(1)
			go func() {
				defer workers.Done()
				for index := range indices {
					request := requests[index]
					raw, callErr := rt.CallMCPReadData(serverMain, request.Tool, request.Arguments)
					items[index] = map[string]any{"index": index, "tool": request.Tool, "result": raw}
					errs[index] = callErr
				}
			}()
		}
		for index := range requests {
			indices <- index
		}
		close(indices)
		workers.Wait()
		result := map[string]any{"baseId": rt.Str("base-id"), "items": items, "requests": len(requests), "concurrency": width}
		for index, callErr := range errs {
			if callErr != nil {
				return apperrors.NewAPI(fmt.Sprintf("只读查询 %d 失败；其余已读结果保留", index), apperrors.WithCause(callErr), apperrors.WithRetryable(false), apperrors.WithDetails(map[string]any{"result": result}))
			}
		}
		return rt.Output(result)
	},
}

func init() { shortcut.Register(ReadBatch) }
