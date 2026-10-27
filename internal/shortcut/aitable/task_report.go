// Copyright 2026 Alibaba Group
// SPDX-License-Identifier: Apache-2.0

package aitable

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
)

// Model metrics are explicit client evidence. Missing values remain absent,
// never inferred from request time or encoded response bytes.
type aitableModelMetrics struct {
	Model        string `json:"model"`
	Environment  string `json:"environment"`
	InputDigest  string `json:"inputDigest"`
	ElapsedMS    *int64 `json:"elapsedMs,omitempty"`
	OutputBytes  *int64 `json:"outputBytes,omitempty"`
	Interactions *int64 `json:"interactions,omitempty"`
	ReworkCount  *int64 `json:"reworkCount,omitempty"`
}

type aitableTaskReport struct {
	Kind            string               `json:"kind"`
	RunID           string               `json:"runId"`
	BaseID          string               `json:"baseId"`
	InputDigest     string               `json:"inputDigest"`
	Status          string               `json:"status"`
	ElapsedMS       int64                `json:"elapsedMs"`
	Metrics         aitableTaskMetrics   `json:"metrics"`
	MetricsComplete bool                 `json:"metricsComplete"`
	ModelMetrics    *aitableModelMetrics `json:"modelMetrics,omitempty"`
	Comparison      map[string]any       `json:"comparison,omitempty"`
	identity        string
}

var TaskReport = shortcut.Shortcut{
	Service: "aitable", Command: "+task-report", Product: serverMain,
	Description: "离线汇总搭建或 AI 等待检查点的调用、等待和客户端模型统计", Risk: shortcut.RiskRead,
	Intent:        "已有本地搭建或 AI 等待检查点，需要离线查看已记录的耗时、调用数、退避和可选客户端模型统计时使用。可与 baseline 对照；只有同输入、身份、模型与环境且两次结果均 verified 才输出可比较差值。不会查询 MCP 或触发写入，不把 pending、旧数据或缺失统计当作性能验收。",
	Safety:        contract.SafetySpec{Effect: "read", Risk: "low", Confirmation: "not_required", Idempotency: "idempotent"},
	OutputRollout: output.RolloutUnifiedActive,
	Contract:      aitableCompositeContractWithResult("+task-report", "离线汇总检查点的已记录调用、等待及可选模型指标", "已有 build-plan 或 field-wait-ai 持久检查点，需要离线统计或比较相同输入的两次运行时", "没有检查点或需要查询真实任务状态时使用原只读核验入口；统计不能证明结果正确，未核验和缺失指标不参与提速结论", "dws aitable +task-report --checkpoint task.json", &contract.ResultSpec{Outcomes: []contract.ResultOutcome{contract.ResultOutcomeSuccess, contract.ResultOutcomeFailure}, DataSchema: json.RawMessage(`{"type":"object","properties":{"kind":{"type":"string","description":"build_plan 或 ai_wait"},"runId":{"type":"string","description":"计划运行 ID 或原始 AI 任务 ID"},"baseId":{"type":"string","description":"原目标 Base"},"inputDigest":{"type":"string","description":"原计划或固定等待目标的 SHA-256"},"status":{"type":"string","description":"检查点原状态；统计不提升业务核验状态"},"elapsedMs":{"type":"integer","description":"已持久化任务执行段时间，不包含 CLI 启动/参数解析、模型段或人工等待"},"metricsComplete":{"type":"boolean","description":"是否存在完整新统计且无未结算投递；中断前未落盘的区间仍不可推断"},"metrics":{"type":"object","description":"已记录 CLI 到 MCP 调用、请求耗时、退避及响应编码字节量；分量不与总时间重复相加，非内部 HSF 次数","additionalProperties":true},"modelMetrics":{"type":"object","description":"显式客户端文件提供的模型、环境和可选计时/输出/交互/返工指标，缺失不当作零","additionalProperties":true},"comparison":{"type":"object","description":"同输入、身份、模型、环境及两个 verified 结果才可比较 CLI 差值，不宣称端到端提速","additionalProperties":true}},"required":["kind","runId","baseId","inputDigest","status","elapsedMs","metricsComplete","metrics"]}`)}),
	Flags: []shortcut.Flag{
		{Name: "checkpoint", Type: shortcut.FlagString, Required: true, Desc: "本地 build-plan 或 AI 等待检查点"},
		{Name: "baseline", Type: shortcut.FlagString, Desc: "可选基线检查点，只读对照，不修改文件"},
		{Name: "model-metrics", Type: shortcut.FlagString, Desc: "可选客户端指标 JSON：model/environment/inputDigest 必填，elapsedMs/outputBytes/interactions/reworkCount 可选非负整数"},
		{Name: "baseline-model-metrics", Type: shortcut.FlagString, Desc: "基线对应的客户端指标 JSON，需同时提供 baseline"},
	},
	Execute: func(rt *shortcut.RuntimeContext) error {
		if rt.Str("baseline-model-metrics") != "" && rt.Str("baseline") == "" {
			return apperrors.NewValidation("baseline-model-metrics 需要 baseline")
		}
		report, err := loadAitableTaskReport(rt.Str("checkpoint"), rt.Str("model-metrics"))
		if err != nil {
			return apperrors.NewValidation(err.Error())
		}
		if path := rt.Str("baseline"); path != "" {
			baseline, err := loadAitableTaskReport(path, rt.Str("baseline-model-metrics"))
			if err != nil {
				return apperrors.NewValidation(err.Error())
			}
			comparable := report.Kind == baseline.Kind && report.InputDigest == baseline.InputDigest && report.BaseID == baseline.BaseID && report.identity == baseline.identity && report.Status == "verified" && baseline.Status == "verified" && report.MetricsComplete && baseline.MetricsComplete && report.ModelMetrics != nil && baseline.ModelMetrics != nil && report.ModelMetrics.Model == baseline.ModelMetrics.Model && report.ModelMetrics.Environment == baseline.ModelMetrics.Environment
			report.Comparison = map[string]any{"comparable": comparable, "baselineRunId": baseline.RunID, "reason": "仅同输入/身份/模型/环境且双方结果与统计完整时比较；模型与 CLI 时间不直接相加"}
			if comparable {
				report.Comparison["elapsedDeltaMs"] = report.ElapsedMS - baseline.ElapsedMS
				report.Comparison["remoteCallsDelta"] = report.Metrics.RemoteCalls - baseline.Metrics.RemoteCalls
				report.Comparison["backoffDeltaMs"] = report.Metrics.BackoffMS - baseline.Metrics.BackoffMS
				if report.ModelMetrics.ElapsedMS != nil && baseline.ModelMetrics.ElapsedMS != nil {
					report.Comparison["modelElapsedDeltaMs"] = *report.ModelMetrics.ElapsedMS - *baseline.ModelMetrics.ElapsedMS
				}
				if report.ModelMetrics.OutputBytes != nil && baseline.ModelMetrics.OutputBytes != nil {
					report.Comparison["modelOutputBytesDelta"] = *report.ModelMetrics.OutputBytes - *baseline.ModelMetrics.OutputBytes
				}
				if report.ModelMetrics.ReworkCount != nil && baseline.ModelMetrics.ReworkCount != nil {
					report.Comparison["reworkCountDelta"] = *report.ModelMetrics.ReworkCount - *baseline.ModelMetrics.ReworkCount
				}
				if report.ModelMetrics.Interactions != nil && baseline.ModelMetrics.Interactions != nil {
					report.Comparison["interactionsDelta"] = *report.ModelMetrics.Interactions - *baseline.ModelMetrics.Interactions
				}
			}
		}
		return rt.Output(report)
	},
}

func readAitableTaskFile(path string, target any) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	raw, err := io.ReadAll(io.LimitReader(f, (16<<20)+1))
	if err != nil {
		return err
	}
	if len(raw) > 16<<20 {
		return fmt.Errorf("任务证据文件超过 16 MiB")
	}
	return json.Unmarshal(raw, target)
}

func loadAitableTaskReport(path, modelPath string) (aitableTaskReport, error) {
	var raw map[string]json.RawMessage
	report := aitableTaskReport{}
	if err := readAitableTaskFile(path, &raw); err != nil {
		return report, err
	}
	if kind := string(raw["kind"]); kind != "" && kind != `"ai_wait"` && kind != `"build_plan"` {
		return report, fmt.Errorf("不支持的任务检查点类型")
	}
	encoded, _ := json.Marshal(raw)
	if string(raw["kind"]) == `"ai_wait"` {
		var state aiWaitCheckpoint
		if err := json.Unmarshal(encoded, &state); err != nil {
			return report, err
		}
		if state.Version != 1 || state.Identity == "" || state.BaseID == "" || state.TableID == "" || len(state.Targets) == 0 {
			return report, fmt.Errorf("AI 等待检查点身份或目标无效")
		}
		if _, err := restoreAITaskPoll(state.BaseID, state.TableID, state.Targets, state.Progress); err != nil {
			return report, err
		}
		targets, _ := json.Marshal(map[string]any{"baseId": state.BaseID, "tableId": state.TableID, "tasks": state.Targets})
		digest := sha256.Sum256(targets)
		report = aitableTaskReport{Kind: "ai_wait", RunID: stringValue(state.Targets[0], "taskId"), BaseID: state.BaseID, InputDigest: hex.EncodeToString(digest[:]), Status: "pending", ElapsedMS: state.ElapsedMS, Metrics: state.Metrics, MetricsComplete: raw["metrics"] != nil && !state.MetricsPartial && state.InvocationComplete, identity: state.Identity}
		if aiTaskTerminalFailure(state.Progress.Body) != nil {
			report.Status = "failed"
		} else if state.Progress.Body["terminalVerified"] == true {
			report.Status = "terminal_results_unverified"
		}

	} else {
		var state buildPlanState
		if err := json.Unmarshal(encoded, &state); err != nil {
			return report, err
		}
		if state.Version != 1 || state.RunID == "" || state.Identity == "" || state.Digest == "" || state.BaseID == "" {
			return report, fmt.Errorf("搭建检查点身份或计划无效")
		}
		report = aitableTaskReport{Kind: "build_plan", RunID: state.RunID, BaseID: state.BaseID, InputDigest: state.Digest, Status: state.Status, ElapsedMS: state.ElapsedMS, Metrics: state.Metrics, MetricsComplete: raw["metrics"] != nil && !state.MetricsPartial && state.InvocationComplete, identity: state.Identity}
		if len(state.Steps) == 0 {
			return report, fmt.Errorf("搭建检查点缺少步骤")
		}
		for _, step := range state.Steps {
			if step.Status == "started" || step.Status == "unknown" || (state.Status == "verified" && step.Status != "verified") {
				report.MetricsComplete = false
			}
		}
		if raw["metrics"] == nil {
			report.Metrics = metricsFromPlanCalls(state.Calls)
		}

	}
	if report.ElapsedMS < 0 || report.Metrics.Invocations < 0 || report.Metrics.RemoteCalls < 0 || report.Metrics.ReadCalls < 0 || report.Metrics.WriteCalls < 0 || report.Metrics.RequestMS < 0 || report.Metrics.BackoffMS < 0 || report.Metrics.ResponseBytes < 0 || report.Metrics.RemoteCalls != report.Metrics.ReadCalls+report.Metrics.WriteCalls {
		return report, fmt.Errorf("检查点统计无效")
	}
	if modelPath != "" {
		var model aitableModelMetrics
		if err := readAitableTaskFile(modelPath, &model); err != nil {
			return report, err
		}
		if model.Model == "" || model.Environment == "" || model.InputDigest != report.InputDigest {
			return report, fmt.Errorf("客户端模型指标必须绑定同一输入，包含 model/environment/inputDigest")
		}
		for _, value := range []*int64{model.ElapsedMS, model.OutputBytes, model.Interactions, model.ReworkCount} {
			if value != nil && *value < 0 {
				return report, fmt.Errorf("客户端模型指标必须为非负整数")
			}
		}
		report.ModelMetrics = &model
	}
	return report, nil
}

func init() { shortcut.Register(TaskReport) }
