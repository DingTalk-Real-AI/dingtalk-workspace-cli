// Copyright 2026 Alibaba Group
// SPDX-License-Identifier: Apache-2.0

package aitable

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/aitableprotocol"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/auth"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	filelock "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/event/lock"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/helpers"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/config"
	"github.com/google/uuid"
)

// The plan is deliberately limited to AI-table construction. A readback is an
// explicit assertion on full data, not a model-written shell script or summary.
type buildPlan struct {
	Version   int                       `json:"version"`
	BaseID    string                    `json:"baseId"`
	Steps     []buildPlanStep           `json:"steps"`
	Templates map[string]map[string]any `json:"templates,omitempty"`
}
type buildPlanStep struct {
	ID            string            `json:"id"`
	DependsOn     []string          `json:"dependsOn,omitempty"`
	Tool          string            `json:"tool"`
	Arguments     map[string]any    `json:"arguments"`
	Verify        buildPlanReadback `json:"verify"`
	Template      string            `json:"template,omitempty"`
	FieldBindings map[string]any    `json:"fieldBindings,omitempty"`
}
type buildPlanReadback struct {
	Tool      string         `json:"tool"`
	Arguments map[string]any `json:"arguments"`
	Expected  map[string]any `json:"expected"`
}
type buildPlanState struct {
	Version            int                  `json:"version"`
	Digest             string               `json:"planDigest"`
	Identity           string               `json:"identity"`
	BaseID             string               `json:"baseId"`
	RunID              string               `json:"runId"`
	Status             string               `json:"status"`
	Steps              []buildPlanStepState `json:"steps"`
	Calls              []buildPlanCall      `json:"calls"`
	ElapsedMS          int64                `json:"elapsedMs"`
	Metrics            aitableTaskMetrics   `json:"metrics"`
	MetricsPartial     bool                 `json:"metricsPartial"`
	InvocationComplete bool                 `json:"invocationComplete"`
}
type buildPlanStepState struct {
	ID             string          `json:"id"`
	Status         string          `json:"status"`
	Resources      map[string]any  `json:"resources,omitempty"`
	Receipt        map[string]any  `json:"receipt,omitempty"`
	Arguments      map[string]any  `json:"arguments,omitempty"`
	Reconciliation map[string]any  `json:"reconciliationReceipt,omitempty"`
	AIProgress     *aiTaskEvidence `json:"aiProgress,omitempty"`
}
type buildPlanCall struct {
	Step          string `json:"step"`
	Tool          string `json:"tool"`
	Phase         string `json:"phase"`
	ElapsedMS     int64  `json:"elapsedMs"`
	ResponseBytes int    `json:"responseBytes"`
	Succeeded     bool   `json:"succeeded"`
}

var buildPlanID = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_-]{0,63}$`)
var buildPlanWriters = map[string]string{
	"create_table": "get_tables", "create_fields": "get_fields", "create_view": "get_views",
	"update_view": "get_views", "create_records": "query_records", "update_records": "query_records",
	"create_dashboard": "get_dashboard", "create_chart": "get_chart",
	"run_ai_field": "get_ai_field_task_status",
}

func buildPlanResultSpec() *contract.ResultSpec {
	return &contract.ResultSpec{Outcomes: []contract.ResultOutcome{contract.ResultOutcomeSuccess, contract.ResultOutcomePending, contract.ResultOutcomeFailure}, DataSchema: json.RawMessage(`{"type":"object","description":"计划执行与只读续验的检查点；异常时已保留的同结构进度位于 error.details.result，不授权重放未知写入","properties":{"version":{"type":"integer","description":"检查点格式版本"},"planDigest":{"type":"string","description":"规范化计划的 SHA-256，恢复时必须匹配原计划"},"identity":{"type":"string","description":"所选账号组织、显式认证覆盖与网关上下文的 SHA-256，恢复时必须匹配；不存储令牌，未执行预演时为空"},"baseId":{"type":"string","description":"写入与回读共同绑定的固定目标 Base"},"runId":{"type":"string","description":"跨调用复用的任务关联 ID"},"status":{"type":"string","enum":["planned","verified","unknown","pending"],"description":"planned=尚未执行；verified=全部步骤的独立回读断言匹配；unknown=效果或核验未知；pending=仍有步骤待执行或核验"},"steps":{"type":"array","description":"逐步状态、真实资源 ID 与原始回执；回执和摘要不能代替独立完整数据断言","items":{"type":"object","properties":{"id":{"type":"string","description":"计划中稳定的步骤 ID"},"status":{"type":"string","description":"planned/started/accepted/verified/unknown；accepted 仅收到回执，verified 需要独立回读断言匹配"},"resources":{"type":"object","description":"该步真实回执中的资源事实，用于按依赖绑定后续 ID","additionalProperties":true},"receipt":{"type":"object","description":"原始写入回执，保留作核验依据，不是完成证明","additionalProperties":true},"arguments":{"type":"object","description":"该步实际已绑定的写入参数，包含需保留的原记录写入 token","additionalProperties":true},"aiProgress":{"type":"object","description":"本轮固定 AI 目标、逐行终态与查询证据；等待跨进程保留，不能证明结果可读","additionalProperties":true}},"required":["id","status"],"additionalProperties":true}},"calls":{"type":"array","description":"结构写入/回读/准备请求的耗时和响应量；AI 等待调用汇总见 metrics、行证据见 aiProgress。所有分量已包含在总耗时中，不重复相加","items":{"type":"object","properties":{"step":{"type":"string","description":"对应步骤 ID"},"tool":{"type":"string","description":"实际调用的 MCP 工具"},"phase":{"type":"string","description":"write=写入，verify=独立核验，prepare=写前目标检查；AI 等待次数及退避见 metrics"},"elapsedMs":{"type":"integer","description":"该次请求耗时"},"responseBytes":{"type":"integer","description":"该次响应重新编码后的字节数，不表示网络传输体积"},"succeeded":{"type":"boolean","description":"该次调用是否返回错误；true 不单独证明该步骤已核验"}},"required":["step","tool","phase","elapsedMs","responseBytes","succeeded"],"additionalProperties":true}},"elapsedMs":{"type":"integer","description":"各次任务执行段累计耗时，已包含请求及核验，不包含 CLI 启动、模型交互或人工等待"},"metrics":{"type":"object","description":"累计 CLI 到 MCP 调用、请求与退避统计；分量已包含在 elapsedMs，缺失旧统计不推断为零","additionalProperties":true}},"required":["version","planDigest","identity","baseId","runId","status","steps","calls","elapsedMs"]}`)}
}

func buildPlanShortcut(verifyOnly bool) shortcut.Shortcut {
	command, description, use, avoid := "+build-plan", "按依赖计划串行搭建 AI 表格，逐步独立回读断言并保存检查点", "在同一 Base 搭建多表、字段、视图、记录、显式图表布局及固定记录 AI 任务，已声明顺序、依赖以及每一步的独立完整数据回读断言，需要真实 ID 绑定和中断接续时", "单表结构优先 +table-bootstrap；摘要、success 信封或图表列表不能替代完整配置与数据断言；未知写入先用 +build-plan-verify 只读续验，缺少可靠回执或 ID 时不重建；不默认追加 arrange"
	risk := shortcut.RiskWrite
	safety := contract.SafetySpec{Effect: "write", Risk: "medium", Confirmation: "user_required", Idempotency: "non_idempotent"}
	if verifyOnly {
		command, description, use, avoid = "+build-plan-verify", "按原计划和检查点只读续验真实对象，重新匹配逐步完整数据断言", "搭建任务中断或结果未知，原计划、账号组织身份与 Base 都未变化，需要对检查点中已有真实 ID 的步骤独立只读续验时", "尚未执行的步骤不会通过续验创建；缺少可靠回执或 ID 时停止，不按摘要、数量差额或输入位置补写；接续未执行写入需 +build-plan --resume"
		risk = shortcut.RiskRead
		safety = contract.SafetySpec{Effect: "read", Risk: "low", Confirmation: "not_required", Idempotency: "idempotent"}
	}
	return shortcut.Shortcut{Service: "aitable", Command: command, Product: serverMain, Description: description, Intent: use + "。每一步 verify 必须独立读取完整业务 data，并断言本步声明的全部配置、布局或目标数据，不能用摘要证明完成。检查点绑定计划摘要、实际账号组织身份及原 Base；恢复先核验已有步骤，未知写入不自动重放。", Risk: risk, Safety: safety, OutputRollout: output.RolloutUnifiedActive,
		Contract: aitableCompositeContractWithResult(command, description, use, avoid, "dws aitable "+command+" --plan plan.json --checkpoint task.json", buildPlanResultSpec()),
		Flags:    []shortcut.Flag{{Name: "plan", Type: shortcut.FlagString, Desc: "版本 1 JSON 文件，固定 baseId，1～100 个按依赖排序的 steps；AI 步骤需固定 fieldIds/recordIds；verify.tasks 绑定本步 taskTargets，结果未证实可读时停为 pending。每步 verify.tool/arguments/expected.data 必填，expected 必须覆盖本步完整配置、布局或数据，不能只写摘要", Required: true}, {Name: "checkpoint", Type: shortcut.FlagString, Desc: "私有持久检查点文件；新任务拒绝覆盖旧文件，恢复须匹配原计划、账号组织和 Base", Required: true}, {Name: "resume", Type: shortcut.FlagBool, Desc: "写入口先重新只读核验已有步骤，再继续未执行步骤；verify 入口始终读取已有检查点"}, {Name: "timeout", Type: shortcut.FlagString, Default: "2m", Desc: "本次请求与核验共同的总时限，须为正时长且不超过 30m，如 2m"}},
		Execute:  func(rt *shortcut.RuntimeContext) error { return executeBuildPlan(rt, verifyOnly) },
	}
}

func readBuildPlan(path string) (buildPlan, string, error) {
	var plan buildPlan
	f, err := os.Open(path)
	if err != nil {
		return plan, "", err
	}
	defer f.Close()
	raw, err := io.ReadAll(io.LimitReader(f, 8<<20+1))
	if err != nil {
		return plan, "", err
	}
	if len(raw) > 8<<20 {
		return plan, "", fmt.Errorf("计划超过 8 MiB")
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err = decoder.Decode(&plan); err != nil {
		return plan, "", err
	}
	if decoder.Decode(new(any)) != io.EOF {
		return plan, "", fmt.Errorf("计划包含多余 JSON")
	}
	if plan.Version != 1 || plan.BaseID == "" || len(plan.Steps) == 0 || len(plan.Steps) > 100 {
		return plan, "", fmt.Errorf("计划 version 必须为 1，baseId 非空，steps 为 1～100 项")
	}
	seen := map[string]bool{}
	for index, step := range plan.Steps {
		if step.Template != "" {
			if step.Tool != "create_chart" {
				return plan, "", fmt.Errorf("template 仅用于 create_chart")
			}
			parameters, ok := step.Arguments["config"].(map[string]any)
			if !ok {
				return plan, "", fmt.Errorf("template 参数必须是 config 对象")
			}
			cfg, expandErr := aitableprotocol.ExpandChartTemplate(plan.Templates[step.Template], parameters)
			if expandErr != nil {
				return plan, "", expandErr
			}
			step.Arguments["config"] = cfg
			plan.Steps[index] = step
		}
		if !buildPlanID.MatchString(step.ID) || seen[step.ID] {
			return plan, "", fmt.Errorf("步骤 ID 无效或重复: %q", step.ID)
		}
		if buildPlanWriters[step.Tool] == "" || step.Verify.Tool != buildPlanWriters[step.Tool] || len(step.Arguments) == 0 || len(step.Verify.Arguments) == 0 || len(step.Verify.Expected) == 0 {
			return plan, "", fmt.Errorf("步骤 %s 必须使用受支持的构建工具和对应完整数据回读断言", step.ID)
		}
		for _, dependency := range step.DependsOn {
			if !seen[dependency] {
				return plan, "", fmt.Errorf("步骤 %s 依赖 %q 必须在此前声明", step.ID, dependency)
			}
		}
		if step.Arguments["baseId"] != plan.BaseID || step.Verify.Arguments["baseId"] != plan.BaseID {
			return plan, "", fmt.Errorf("步骤 %s 的写入及回读必须显式绑定同一 baseId", step.ID)
		}
		expected, ok := step.Verify.Expected["data"].(map[string]any)
		if !ok || len(expected) == 0 {
			return plan, "", fmt.Errorf("步骤 %s 必须断言完整业务 data，不能只比较 success 信封", step.ID)
		}
		allowed := map[string]bool{}
		for _, dep := range step.DependsOn {
			allowed[dep] = true
		}
		if err := validatePlanReferences(step.Arguments, allowed); err != nil {
			return plan, "", err
		}
		if len(step.FieldBindings) != 0 {
			if step.Tool != "create_records" && step.Tool != "update_records" {
				return plan, "", fmt.Errorf("fieldBindings 只用于记录步骤")
			}
			if err := validatePlanReferences(step.FieldBindings, allowed); err != nil {
				return plan, "", err
			}
		}
		allowed[step.ID] = true
		if err := validatePlanReferences(step.Verify.Arguments, allowed); err != nil {
			return plan, "", err
		}
		if err := validatePlanReferences(step.Verify.Expected, allowed); err != nil {
			return plan, "", err
		}
		if err := preflightPlanStep(step); err != nil {
			return plan, "", err
		}
		seen[step.ID] = true
	}
	canonical, _ := json.Marshal(plan)
	sum := sha256.Sum256(canonical)
	return plan, hex.EncodeToString(sum[:]), nil
}

func validatePlanReferences(value any, allowed map[string]bool) error {
	switch v := value.(type) {
	case map[string]any:
		if ref, exists := v["$ref"]; exists {
			name, ok := ref.(string)
			parts := strings.Split(name, ".")
			if !ok || len(v) != 1 || len(parts) < 2 || !allowed[parts[0]] {
				return fmt.Errorf("引用必须对应显式 dependsOn 或本步回执: %v", ref)
			}
			return nil
		}
		for _, child := range v {
			if err := validatePlanReferences(child, allowed); err != nil {
				return err
			}
		}
	case []any:
		for _, child := range v {
			if err := validatePlanReferences(child, allowed); err != nil {
				return err
			}
		}
	}
	return nil
}

func preflightPlanStep(step buildPlanStep) error {
	args := step.Arguments
	require := func(key string) error {
		if v, ok := args[key]; !ok || v == nil {
			return fmt.Errorf("步骤 %s 缺少 %s", step.ID, key)
		}
		return nil
	}
	for _, key := range map[string][]string{"create_table": {"tableName", "fields"}, "create_fields": {"tableId", "fields"}, "create_view": {"tableId", "viewType"}, "update_view": {"tableId", "viewId"}, "create_records": {"tableId", "records"}, "update_records": {"tableId", "records"}, "create_dashboard": {"config"}, "create_chart": {"dashboardId", "config", "layout"}, "run_ai_field": {"tableId", "fieldIds", "recordIds"}}[step.Tool] {
		if err := require(key); err != nil {
			return err
		}
	}
	if step.Tool == "create_table" || step.Tool == "create_fields" {
		fields, ok := args["fields"].([]any)
		if !ok || len(fields) > 15 || (step.Tool == "create_fields" && len(fields) == 0) {
			return fmt.Errorf("步骤 %s fields 最多 15 项；大结构优先 bootstrap", step.ID)
		}
		for i, field := range fields {
			if _, err := validateBootstrapField(field, fmt.Sprintf("%s.fields[%d]", step.ID, i)); err != nil {
				return err
			}
		}
		if step.Tool == "create_table" {
			if err := validateCreateTableCapabilities(fields, step.ID+".fields"); err != nil {
				return err
			}
		}
	}
	if step.Tool == "run_ai_field" {
		if err := validateAIPlanArguments(args, true); err != nil {
			return err
		}
		data, _ := step.Verify.Expected["data"].(map[string]any)
		if data["terminalVerified"] != true || data["resultReadableVerified"] != true {
			return fmt.Errorf("AI 计划 verify.expected.data 必须要求终态和本轮结果可读，不能绕过证据")
		}
	}
	if step.Tool == "create_chart" {
		encoded, _ := json.Marshal([]any{map[string]any{"config": args["config"], "layout": args["layout"]}})
		if _, err := parseChartBatch(string(encoded)); err != nil {
			return err
		}
	}
	if step.Tool == "create_records" || step.Tool == "update_records" {
		records, ok := args["records"].([]any)
		if !ok || len(records) == 0 || len(records) > 100 {
			return fmt.Errorf("计划记录步骤需 1～100 项，请显式分片")
		}
		for _, v := range records {
			row, ok := v.(map[string]any)
			if !ok {
				return fmt.Errorf("records 必须包含对象")
			}
			if _, ok := row["cells"].(map[string]any); !ok {
				return fmt.Errorf("record.cells 必须是对象")
			}
			if step.Tool == "update_records" && stringValue(row, "recordId") == "" {
				return fmt.Errorf("更新记录必须提供真实 recordId")
			}
		}
	}

	if step.Tool == "create_dashboard" {
		cfg, ok := args["config"].(map[string]any)
		if !ok || stringValue(cfg, "name") == "" {
			return fmt.Errorf("步骤 %s 的 dashboard config.name 必填", step.ID)
		}
	}
	return nil
}

func buildPlanIdentity(rt *shortcut.RuntimeContext) (string, error) {
	selector, _ := rt.Command().Flags().GetString("profile")
	if selector == "" {
		selector = auth.RuntimeProfile()
	}
	identity, err := auth.ResolveProfileMetadataReadOnly(config.DefaultConfigDir(), selector)
	if err != nil {
		return "", err
	}
	if identity == nil || identity.UserID == "" || identity.CorpID == "" {
		return "", fmt.Errorf("无法绑定实际执行账号与组织，请先登录并选择明确 profile")
	}
	// An explicit token bypasses the profile credential resolver. Bind that
	// override and the endpoint context as well, without persisting either secret.
	token, _ := rt.Command().Flags().GetString("token")
	scope := []string{identity.CorpID, identity.UserID, strings.TrimSpace(token), auth.MCPBaseURLOverride(), os.Getenv("DINGTALK_AITABLE_MCP_URL")}
	encoded, _ := json.Marshal(scope)
	sum := sha256.Sum256(encoded)
	return hex.EncodeToString(sum[:]), nil
}

func saveBuildPlanState(path string, state *buildPlanState) error {
	raw, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	return aitableTaskWriteJSON(path, append(raw, '\n'))
}

func loadBuildPlanState(path string, plan buildPlan, digest, identity string) (buildPlanState, error) {
	var state buildPlanState
	raw, err := os.ReadFile(path)
	if err != nil {
		return state, err
	}
	if err = json.Unmarshal(raw, &state); err != nil {
		return state, err
	}
	if state.Version != 1 || state.Digest != digest || state.Identity != identity || state.BaseID != plan.BaseID || state.RunID == "" || len(state.Steps) != len(plan.Steps) {
		return state, fmt.Errorf("检查点计划、身份或目标不匹配，不能复用")
	}
	var marker struct {
		Metrics *json.RawMessage `json:"metrics"`
	}
	_ = json.Unmarshal(raw, &marker)
	if marker.Metrics == nil {
		state.Metrics = metricsFromPlanCalls(state.Calls)
	}
	state.MetricsPartial = state.MetricsPartial || !state.InvocationComplete || marker.Metrics == nil
	for i, step := range plan.Steps {
		if state.Steps[i].ID != step.ID {
			return state, fmt.Errorf("检查点步骤不匹配")
		}
		switch state.Steps[i].Status {
		case "planned", "started", "unknown", "accepted", "verified", "pending":
		default:
			return state, fmt.Errorf("未知检查点状态")
		}
	}
	return state, nil
}

func executeBuildPlan(rt *shortcut.RuntimeContext, verifyOnly bool) error {
	plan, digest, err := readBuildPlan(rt.Str("plan"))
	if err != nil {
		return apperrors.NewValidation(err.Error())
	}
	duration, err := time.ParseDuration(rt.Str("timeout"))
	if err != nil || duration <= 0 || duration > 30*time.Minute {
		return apperrors.NewValidation("timeout 必须为正时长且不超过 30m")
	}
	state := buildPlanState{Version: 1, Digest: digest, BaseID: plan.BaseID, Status: "planned", RunID: uuid.NewString(), Steps: []buildPlanStepState{}, Calls: []buildPlanCall{}}
	for _, step := range plan.Steps {
		state.Steps = append(state.Steps, buildPlanStepState{ID: step.ID, Status: "planned"})
	}
	if rt.DryRun() {
		return rt.Output(state)
	}
	identity, err := buildPlanIdentity(rt)
	if err != nil {
		return err
	}
	state.Identity = identity
	path, err := aitableTaskAbsPath(rt.Str("checkpoint"))
	if err != nil {
		return err
	}
	planPath, err := aitableTaskAbsPath(rt.Str("plan"))
	if err != nil {
		return err
	}
	if path == planPath {
		return apperrors.NewValidation("检查点不能覆盖计划文件")
	}
	if err = os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	// OS-backed locks release on process death, so an interrupted task can resume
	// without deleting a stale marker and risking a concurrent live owner.
	checkpointLock, err := filelock.TryAcquire(path + ".lock")
	if err != nil {
		return fmt.Errorf("检查点正在使用或无法加锁: %w", err)
	}
	defer checkpointLock.Close()

	if verifyOnly || rt.Bool("resume") {
		state, err = loadBuildPlanState(path, plan, digest, identity)
		if err != nil {
			return err
		}
	} else {
		f, createErr := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if createErr != nil {
			return fmt.Errorf("新任务拒绝覆盖已有检查点: %w", createErr)
		}
		f.Close()
		if err = saveBuildPlanState(path, &state); err != nil {
			return err
		}
	}
	started := time.Now()
	previousElapsed := state.ElapsedMS
	state.Metrics.Invocations++
	state.InvocationComplete = false
	persist := func() error {
		state.ElapsedMS = previousElapsed + time.Since(started).Milliseconds()
		return saveBuildPlanState(path, &state)
	}
	ctx, cancel := context.WithTimeout(rt.Command().Context(), duration)
	defer cancel()
	previous := rt.Command().Context()
	rt.Command().SetContext(ctx)
	defer rt.Command().SetContext(previous)
	resources := map[string]map[string]any{}
	finish := func(cause error) error {
		state.InvocationComplete = true
		if saveErr := persist(); saveErr != nil {
			return saveErr
		}
		if cause != nil {
			if errors.Is(cause, context.Canceled) || errors.Is(cause, context.DeadlineExceeded) {
				return fmt.Errorf("任务中断；检查点 %s: %w", path, cause)
			}
			var classified *apperrors.Error
			if errors.As(cause, &classified) {
				copy := *classified
				copy.Details = make(map[string]any, len(classified.Details)+2)
				for key, value := range classified.Details {
					copy.Details[key] = value
				}
				copy.Details["result"], copy.Details["checkpoint"] = state, path
				return &copy
			}
			var legacy *helpers.CLIError
			if errors.As(cause, &legacy) {
				copy := *legacy
				copy.Details = make(map[string]any, len(legacy.Details)+2)
				for key, value := range legacy.Details {
					copy.Details[key] = value
				}
				copy.Details["result"], copy.Details["checkpoint"] = state, path
				return &copy
			}
			return apperrors.NewAPI("搭建计划尚未完整核实，保留检查点并先只读续验", apperrors.WithCause(cause), apperrors.WithRetryable(false), apperrors.WithExecutionStarted(true), apperrors.WithDetails(map[string]any{"result": state, "checkpoint": path}))
		}
		if state.Status == "pending" {
			return output.StoreResult(rt.Command().Context(), output.Pending(state, &output.OperationInfo{ID: state.RunID, State: "pending", NextCommand: aitableRecoveryCommand("dws", "aitable", "+build-plan-verify", "--plan", rt.Str("plan"), "--checkpoint", path, "--format", "json")}))
		}
		return rt.Output(state)
	}
	for i, step := range plan.Steps {
		current := &state.Steps[i]
		if ctx.Err() != nil {
			state.Status = "pending"
			return finish(ctx.Err())
		}
		if current.Resources != nil {
			resources[step.ID] = current.Resources
		}
		if current.Status == "planned" && verifyOnly {
			continue
		}
		if current.Status == "planned" {
			arguments, bindErr := aitableprotocol.BindReferences(step.Arguments, resources)
			if bindErr != nil {
				return finish(bindErr)
			}
			args := arguments.(map[string]any)
			if len(step.FieldBindings) != 0 {
				bindings, bindErr := resolvePlanFieldBindings(step.FieldBindings, resources)
				if bindErr != nil {
					return finish(bindErr)
				}
				args["records"], bindErr = bindPlanRecordCells(args["records"], bindings)
				if bindErr != nil {
					return finish(bindErr)
				}
			}
			// readBuildPlan pins baseId to the literal plan Base before any
			// dispatch; BindReferences leaves this scalar unchanged.
			if step.Tool == "run_ai_field" {
				if err := validateAIPlanArguments(args, false); err != nil {
					return finish(apperrors.NewValidation(err.Error()))
				}
			}
			if step.Tool == "create_chart" {
				if err := preflightBoundPlanChart(rt, &state, step.ID, args); err != nil {
					return finish(err)
				}
			}
			// Persist before dispatch. A crash after remote acceptance never becomes a
			// fresh planned write on resume, even if the response has been lost.
			if step.Tool == "create_records" {
				if _, exists := args["clientToken"]; !exists {
					args["clientToken"] = uuid.NewString()
				}
			}
			current.Arguments = args
			current.Status = "started"
			state.Status = "pending"
			if err = persist(); err != nil {
				return err
			}
			raw, callErr := buildPlanCallTool(rt, &state, step.ID, step.Tool, "write", args, true)
			current.Receipt = raw
			// A proven pre-dispatch refusal is not an uncertain update. Reading
			// an already matching old value must not turn that refusal into
			// verified, nor authorize a resume to repeat the write.
			var refused *apperrors.Error
			if errors.As(callErr, &refused) && refused.ExecutionStarted != nil && !*refused.ExecutionStarted {
				current.Status, state.Status = "unknown", "unknown"
				current.Resources = nil
				return finish(callErr)
			}
			current.Resources = parityResponseObject(raw)
			if len(current.Resources) == 0 {
				if candidate := buildPlanCreationCandidate(step.Tool, state.BaseID, args, callErr); candidate != nil {
					current.Resources = candidate
					// The transport returns structured errors separately from raw data.
					// Persist only the allowlisted unknown receipt, never a success or
					// arbitrary upstream diagnostics, so a later process can read this ID.
					current.Receipt = map[string]any{"status": "error", "error": map[string]any{"details": cloneAnyMap(candidate)}}
				}
			}
			if len(current.Resources) == 0 && step.Tool == "update_view" {
				current.Resources = map[string]any{"viewId": args["viewId"]}
			}
			if len(current.Resources) == 0 && step.Tool == "update_records" {
				// Update targets already have real IDs. They can be read after an
				// uncertain receipt without inventing any created resource identity.
				current.Resources = map[string]any{"tableId": args["tableId"]}
			}
			resources[step.ID] = current.Resources
			current.Status = "accepted"
			if callErr != nil {
				current.Status = "unknown"
			}
			if err = persist(); err != nil {
				return err
			}
			if callErr != nil && len(current.Resources) == 0 {
				state.Status = "unknown"
				return finish(callErr)
			}
		}
		if step.Tool == "create_records" && current.Status != "planned" {
			if err = reconcileBuildPlanRecords(rt, &state, step, current); err != nil {
				state.Status = "unknown"
				return finish(err)
			}
			resources[step.ID] = current.Resources
		}
		if current.Resources == nil && current.Status != "planned" {
			state.Status = "unknown"
			return finish(fmt.Errorf("步骤 %s 无可靠回执；先按原目标/记录 token 对账，不重发写入", step.ID))
		}
		if step.Tool == "run_ai_field" {
			targets, targetErr := aiPlanTargets(current)
			if targetErr != nil {
				state.Status = "unknown"
				return finish(targetErr)
			}
			current.Resources["taskTargets"] = targets
			bound, bindErr := aitableprotocol.BindReferences(step.Verify.Arguments, resources)
			if bindErr != nil {
				return finish(bindErr)
			}
			query := bound.(map[string]any)
			rawTargets, _ := json.Marshal(targets)
			queryTargets, _ := json.Marshal(query["tasks"])
			if query["baseId"] != state.BaseID || query["tableId"] != current.Arguments["tableId"] || string(rawTargets) != string(queryTargets) {
				return finish(fmt.Errorf("AI 回读必须绑定原 Base/Table 和本轮 taskTargets"))
			}
			if err := waitAIPlanStep(rt, &state, current, persist); err != nil {
				return finish(err)
			}
			return finish(nil)
		}
		boundArgs, bindErr := aitableprotocol.BindReferences(step.Verify.Arguments, resources)
		if bindErr != nil {
			state.Status = "unknown"
			return finish(bindErr)
		}
		boundExpected, bindErr := aitableprotocol.BindReferences(step.Verify.Expected, resources)
		if bindErr != nil {
			state.Status = "unknown"
			return finish(bindErr)
		}
		if len(step.FieldBindings) != 0 {
			bindings, bindErr := resolvePlanFieldBindings(step.FieldBindings, resources)
			if bindErr != nil {
				return finish(bindErr)
			}
			// A whole-data $ref may resolve to a scalar even though the declared
			// assertion was an object. Keep the accepted receipt and fail validation
			// without issuing another write, including on resume.
			expected, _ := boundExpected.(map[string]any)
			expectedData, ok := expected["data"].(map[string]any)
			if !ok {
				state.Status = "unknown"
				return finish(apperrors.NewValidation("verify.expected.data 引用解析后必须是对象"))
			}
			if expectedRecords, exists := expectedData["records"]; exists {
				expectedData["records"], bindErr = bindPlanRecordCells(expectedRecords, bindings)
				if bindErr != nil {
					return finish(bindErr)
				}
			}
		}
		args := boundArgs.(map[string]any)
		if current.Arguments["tableId"] != nil && !declaredValueMatches(args["tableId"], current.Arguments["tableId"]) {
			return finish(fmt.Errorf("步骤 %s 回读目标跨 Table", step.ID))
		}
		raw, readErr := buildPlanCallTool(rt, &state, step.ID, step.Verify.Tool, "verify", args, false)
		if readErr == nil && declaredValueMatches(raw, boundExpected) {
			readErr = verifyBuildPlanIntent(rt, &state, step, current, raw)
		}
		if readErr != nil || !declaredValueMatches(raw, boundExpected) {
			current.Status = "unknown"
			state.Status = "unknown"
			if readErr == nil {
				readErr = fmt.Errorf("步骤 %s 的完整读回断言不匹配", step.ID)
			}
			return finish(readErr)
		}
		current.Status = "verified"
		if err = persist(); err != nil {
			return err
		}
	}
	state.Status = "verified"
	for _, step := range state.Steps {
		if step.Status != "verified" {
			state.Status = "pending"
		}
	}
	return finish(nil)
}

func buildPlanCallTool(rt *shortcut.RuntimeContext, state *buildPlanState, step, tool, phase string, args map[string]any, write bool) (map[string]any, error) {
	started := time.Now()
	var raw map[string]any
	var err error
	if write {
		raw, err = rt.CallMCPWriteDataStrict(serverMain, tool, args)
	} else {
		raw, err = rt.CallMCPData(serverMain, tool, args)
	}
	state.Metrics.recordCall(write, started, raw)
	encoded, _ := json.Marshal(raw)
	state.Calls = append(state.Calls, buildPlanCall{Step: step, Tool: tool, Phase: phase, ElapsedMS: time.Since(started).Milliseconds(), ResponseBytes: len(encoded), Succeeded: err == nil})
	return raw, err
}

// A lost create receipt can be recovered only through the existing token ledger.
// The original receipt is preserved; partial IDs never authorize filling a gap.
func reconcileBuildPlanRecords(rt *shortcut.RuntimeContext, state *buildPlanState, step buildPlanStep, current *buildPlanStepState) error {
	records, _ := current.Arguments["records"].([]any)
	if validUniqueRecordIDs(createdRecordIDs(current.Receipt), len(records)) {
		return nil
	}
	table, token := stringValue(current.Arguments, "tableId"), stringValue(current.Arguments, "clientToken")
	if table == "" || token == "" {
		return fmt.Errorf("创建记录缺少原表或 clientToken，不能推断已创建 ID")
	}
	if current.Reconciliation != nil {
		ids, err := reconciledRecordIDs(parityResponseObject(current.Reconciliation), state.BaseID, table, token)
		if err == nil && validUniqueRecordIDs(ids, len(records)) {
			return nil
		}
	}
	raw, err := buildPlanCallTool(rt, state, step.ID, "get_record_write_result", "reconcile", map[string]any{"baseId": state.BaseID, "tableId": table, "clientToken": token}, false)
	current.Reconciliation = raw
	if err != nil {
		return err
	}
	ids, err := reconciledRecordIDs(parityResponseObject(raw), state.BaseID, table, token)
	if err != nil || !validUniqueRecordIDs(ids, len(records)) {
		return fmt.Errorf("原 token 的创建记录 ID 集合未完整确认；保留原任务，不按数量补写")
	}
	wire := make([]any, len(ids))
	for i, id := range ids {
		wire[i] = id
	}
	current.Resources = map[string]any{"newRecordIds": wire, "recordIds": wire, "clientToken": token, "reconciled": true}
	return saveBuildPlanState(rt.Str("checkpoint"), state)
}

// Cell keys cannot use value references. A plan-only alias dictionary maps them
// to verified field IDs before dispatch, without adding an MCP wire parameter.
// Whole-object references must still resolve to an object; reject scalars both
// before a write and when validating an already accepted step on resume.
func resolvePlanFieldBindings(declared map[string]any, resources map[string]map[string]any) (map[string]string, error) {
	bound, err := aitableprotocol.BindReferences(declared, resources)
	if err != nil {
		return nil, err
	}
	object, ok := bound.(map[string]any)
	if !ok {
		return nil, apperrors.NewValidation("fieldBindings 引用解析后必须是对象")
	}
	result, seen := map[string]string{}, map[string]bool{}
	for alias, value := range object {
		id, ok := value.(string)
		if strings.TrimSpace(alias) == "" || !ok || strings.TrimSpace(id) == "" || id != strings.TrimSpace(id) || seen[id] {
			return nil, fmt.Errorf("fieldBindings 必须为非空逻辑名到不同真实字段 ID 的映射")
		}
		seen[id], result[alias] = true, id
	}
	return result, nil
}

func bindPlanRecordCells(value any, bindings map[string]string) ([]any, error) {
	rows, ok := value.([]any)
	if !ok {
		return nil, fmt.Errorf("records 必须是数组")
	}
	result := make([]any, len(rows))
	for i, value := range rows {
		row, ok := value.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("records 项必须是对象")
		}
		cells, ok := row["cells"].(map[string]any)
		if !ok {
			return nil, fmt.Errorf("record.cells 必须是对象")
		}
		actual := map[string]any{}
		for alias, value := range cells {
			id, found := bindings[alias]
			if !found {
				return nil, fmt.Errorf("记录字段逻辑名 %q 未声明 fieldBindings", alias)
			}
			actual[id] = value
		}
		copy := cloneAnyMap(row)
		copy["cells"] = actual
		result[i] = copy
	}
	return result, nil
}

func init() { shortcut.Register(buildPlanShortcut(false), buildPlanShortcut(true)) }

// Independently verify the writer's declared intent. A user-authored assertion
// on baseId or a success envelope cannot replace these configuration checks.
func verifyBuildPlanIntent(rt *shortcut.RuntimeContext, state *buildPlanState, step buildPlanStep, current *buildPlanStepState, raw map[string]any) error {
	args := current.Arguments
	if len(args) == 0 {
		return fmt.Errorf("检查点缺少实际写入参数，不能核验完整配置")
	}
	body := parityResponseObject(raw)
	if base, exists := body["baseId"]; exists && base != state.BaseID {
		return fmt.Errorf("独立回读的 Base 不匹配")
	}
	if table, exists := body["tableId"]; exists && args["tableId"] != nil && table != args["tableId"] {
		return fmt.Errorf("独立回读的数据表不匹配")
	}
	switch step.Tool {
	case "create_chart":
		cfg, _ := args["config"].(map[string]any)
		layout, _ := args["layout"].(map[string]any)
		id := stringValue(current.Resources, "chartId")
		if id == "" || body["baseId"] != state.BaseID || stringValue(body, "chartId") != id || body["dashboardId"] != args["dashboardId"] || !verifyChartConfiguration(body, cfg, layout) {
			return fmt.Errorf("图表完整配置、布局或所属目标未匹配")
		}
	case "create_dashboard":
		cfg, _ := args["config"].(map[string]any)
		id := stringValue(current.Resources, "dashboardId")
		if id == "" || stringValue(body, "dashboardId") != id {
			return fmt.Errorf("仪表盘身份未匹配")
		}
		for key, value := range cfg {
			if key == "name" {
				if stringValue(body, "dashboardName", "name") != value {
					return fmt.Errorf("仪表盘名称不匹配")
				}
			} else if !declaredValueMatches(body[key], value) {
				return fmt.Errorf("仪表盘配置 %s 不匹配", key)
			}
		}
	case "create_table", "create_fields":
		tableID := stringValue(args, "tableId")
		if step.Tool == "create_table" {
			tableID = stringValue(current.Resources, "tableId", "sheetId")
		}
		if tableID == "" {
			return fmt.Errorf("字段回读缺少已创建表 ID")
		}
		if step.Tool == "create_table" {
			tables, ok := findNamedObjectList(raw, "tables")
			matched := false
			for _, table := range tables {
				if stringValue(table, "tableId") == tableID && stringValue(table, "tableName", "name") == args["tableName"] {
					matched = true
				}
			}
			if !ok || !matched {
				return fmt.Errorf("已创建表的 ID 或名称未匹配")
			}
		}
		fieldsRaw := raw
		if step.Tool == "create_table" {
			var err error
			fieldsRaw, err = buildPlanCallTool(rt, state, step.ID, "get_fields", "verify_fields", map[string]any{"baseId": state.BaseID, "tableId": tableID}, false)
			if err != nil {
				return err
			}
			fieldBody := parityResponseObject(fieldsRaw)
			if base, exists := fieldBody["baseId"]; exists && base != state.BaseID {
				return fmt.Errorf("创建表的字段回读 Base 不匹配")
			}
			if table, exists := fieldBody["tableId"]; exists && table != tableID {
				return fmt.Errorf("创建表的字段回读 Table 不匹配")
			}
		}
		actual, ok := findNamedObjectList(fieldsRaw, "fields")
		if !ok {
			return fmt.Errorf("字段回读缺少完整 fields")
		}
		declared, _ := args["fields"].([]any)
		if step.Tool == "create_fields" {
			// Names and existing schema do not prove this batch created anything.
			// Bind each accepted receipt ID to its exact independently read field.
			entries, found := findNamedObjectList(current.Receipt, "results")
			if !found || len(entries) != len(declared) {
				return fmt.Errorf("创建字段缺少完整逐项回执，不凭同名旧字段确认创建")
			}
			created := []map[string]any{}
			seen := map[string]bool{}
			for _, entry := range entries {
				id := stringValue(entry, "fieldId")
				if id == "" || seen[id] || entry["success"] != true {
					return fmt.Errorf("创建字段的受理 ID 缺失或重复，先只读对账，不重建")
				}
				seen[id] = true
				matches := 0
				for _, field := range actual {
					if stringValue(field, "fieldId") == id && stringValue(field, "fieldName", "name") == stringValue(entry, "fieldName") {
						created = append(created, field)
						matches++
					}
				}
				if matches != 1 {
					return fmt.Errorf("创建字段的真实 ID 尚未完整独立回读")
				}
			}
			actual = created
		}
		if err := verifyDeclaredFieldStructures(actual, declared); err != nil {
			return err
		}
		byName := map[string]any{}
		for _, field := range actual {
			name := stringValue(field, "fieldName", "name")
			id := stringValue(field, "fieldId")
			if name == "" || id == "" {
				continue
			}
			if _, exists := byName[name]; exists {
				return fmt.Errorf("字段名称重复，不能自动绑定")
			}
			byName[name] = id
		}
		current.Resources["fieldsByName"] = byName
	case "create_view", "update_view":
		id := stringValue(args, "viewId")
		if step.Tool == "create_view" {
			id = stringValue(current.Resources, "viewId")
		}
		views, ok := findNamedObjectList(raw, "views")
		if !ok || id == "" {
			return fmt.Errorf("视图缺少完整回读或真实 ID")
		}
		var target map[string]any
		for _, view := range views {
			if stringValue(view, "viewId") == id {
				if target != nil {
					return fmt.Errorf("视图身份重复")
				}
				target = view
			}
		}
		if target == nil {
			return fmt.Errorf("视图身份未匹配")
		}
		for key, expected := range map[string]any{"baseId": state.BaseID, "tableId": args["tableId"]} {
			if actual, exists := target[key]; exists && actual != expected {
				return fmt.Errorf("视图 %s 不匹配", key)
			}
		}
		for key, value := range args {
			if key == "baseId" || key == "tableId" || key == "viewId" {
				continue
			}
			actual := target[key]
			switch key {
			case "newViewName", "viewName":
				actual = stringValue(target, "viewName", "name")
			case "viewDescription":
				actual = target["description"]
			case "config":
				config, ok := value.(map[string]any)
				if !ok || !presetViewMatches(target, stringValue(target, "viewType", "type"), config) {
					return fmt.Errorf("视图完整配置的公开投影不匹配")
				}
				continue
			}
			if !declaredValueMatches(actual, value) {
				return fmt.Errorf("视图配置 %s 不匹配", key)
			}
		}
	case "create_records", "update_records":
		actual, ok := findNamedObjectList(raw, "records")
		declared, _ := args["records"].([]any)
		if !ok || len(declared) == 0 {
			return fmt.Errorf("记录缺少完整回读")
		}
		declaredRecords := make([]map[string]any, 0, len(declared))
		for _, value := range declared {
			record, ok := value.(map[string]any)
			if !ok {
				return fmt.Errorf("记录输入必须是对象")
			}
			declaredRecords = append(declaredRecords, record)
		}
		candidateIDs := recordIDs(declaredRecords)
		if step.Tool == "create_records" {
			candidateIDs = createdRecordIDs(current.Receipt)
			if current.Reconciliation != nil {
				candidateIDs, _ = reconciledRecordIDs(parityResponseObject(current.Reconciliation), state.BaseID, stringValue(args, "tableId"), stringValue(args, "clientToken"))
			}
		}
		if !validUniqueRecordIDs(candidateIDs, len(declared)) {
			return fmt.Errorf("记录回执 ID 集合不完整；按原 token 对账，不能凭旧值确认创建")
		}
		return verifyPlanRecordCandidates(actual, declaredRecords, candidateIDs)
	}
	return nil
}
