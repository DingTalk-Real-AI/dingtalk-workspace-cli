package recovery

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/transport"
)

type ToolInvoker interface {
	CallToolDirect(ctx context.Context, serverID, toolName string, args map[string]any) (*transport.ToolCallResult, error)
}

type Probe func(ctx context.Context, invoker ToolInvoker, rc RecoveryContext, replay Replay, plan RecoveryPlan) *ProbeResult

type Executor struct {
	planner *Planner
	invoker ToolInvoker
	probes  []Probe
}

func NewExecutor(planner *Planner, invoker ToolInvoker) *Executor {
	return &Executor{
		planner: planner,
		invoker: invoker,
		probes:  defaultProbes(),
	}
}

func (e *Executor) Execute(ctx context.Context, last LastError) RecoveryBundle {
	bundle := RecoveryBundle{
		Status:  "needs_agent_action",
		EventID: last.EventID,
		Context: cloneRecoveryContext(last.Context),
		Replay:  cloneReplay(last.Replay),
	}
	if e == nil || e.planner == nil {
		bundle.Status = "analysis_failed"
		bundle.AnalysisError = "recovery executor unavailable"
		return bundle
	}

	plan := e.planner.PlanWithOptions(ctx, last.Context, PlanOptions{
		EventID:         last.EventID,
		EnableDocSearch: true,
	})
	HydratePlanForEvent(last.EventID, last.Context, last.Replay, &plan)

	bundle.Plan = plan
	bundle.DocSearch = cloneDocSearch(plan.DocSearch)
	bundle.KBHits = cloneKBHits(plan.KBHits)
	bundle.DocActions = cloneDocActions(plan.DocActions)
	bundle.HumanActions = nonNilStrings(plan.HumanActions)
	bundle.ProbeResults = e.runProbes(ctx, last.Context, last.Replay, plan)
	if bundle.Plan.AgentRoute.Payload != nil {
		bundle.Plan.AgentRoute.Payload.ProbeResults = cloneProbeResults(bundle.ProbeResults)
	}
	bundle.AgentTask = buildAgentTask(last.EventID, last.Context, last.Replay, plan, bundle.ProbeResults)
	bundle.FinalizeHint = buildFinalizeHint(last.EventID)

	analysisErrors := make([]string, 0, 3)
	if isEmptyReplay(last.Replay) {
		analysisErrors = append(analysisErrors, "replay data missing")
	}
	if plan.DocSearch.Status == "error" {
		analysisErrors = append(analysisErrors, "doc search failed")
	}
	for _, probe := range bundle.ProbeResults {
		if probe.Status == "error" {
			analysisErrors = append(analysisErrors, fmt.Sprintf("probe %s failed", probe.Name))
		}
	}
	if len(analysisErrors) > 0 {
		bundle.Status = "analysis_failed"
		bundle.AnalysisError = strings.Join(uniqueStrings(analysisErrors), "; ")
	}
	return bundle
}

func (e *Executor) runProbes(ctx context.Context, rc RecoveryContext, replay Replay, plan RecoveryPlan) []ProbeResult {
	if len(e.probes) == 0 {
		return nil
	}
	results := make([]ProbeResult, 0, len(e.probes))
	for _, probe := range e.probes {
		result := probe(ctx, e.invoker, rc, replay, plan)
		if result == nil {
			continue
		}
		results = append(results, *result)
	}
	return results
}

func defaultProbes() []Probe {
	return []Probe{probeUnknownContextAudit, probeAITableBaseCatalog}
}

func probeUnknownContextAudit(ctx context.Context, invoker ToolInvoker, rc RecoveryContext, replay Replay, plan RecoveryPlan) *ProbeResult {
	if normalizeDecisionOwner(plan) != DecisionOwnerAgent {
		return nil
	}

	sourcesChecked := []string{
		"recovery_context.command_path",
		"recovery_context.args_summary",
		"replay.tool_args",
		"replay.redacted_command",
		"plan.doc_search",
		"plan.kb_hits",
		"plan.doc_actions",
	}
	availableSources := make([]string, 0, len(sourcesChecked))
	missingSources := make([]string, 0, len(sourcesChecked))
	candidateIdentifiers := make([]map[string]any, 0)

	appendCandidates := func(source string, values map[string]any) {
		for key, value := range values {
			text := strings.TrimSpace(fmt.Sprint(value))
			if text == "" {
				continue
			}
			lowerKey := strings.ToLower(key)
			if !strings.Contains(lowerKey, "id") && !strings.Contains(lowerKey, "uuid") {
				continue
			}
			candidateIdentifiers = append(candidateIdentifiers, map[string]any{
				"source": source,
				"field":  key,
				"value":  text,
			})
		}
	}

	appendCandidates("context.args_summary", rc.ArgsSummary)
	appendCandidates("replay.tool_args", replay.ToolArgs)

	if len(rc.CommandPath) > 0 {
		availableSources = append(availableSources, "recovery_context.command_path")
	} else {
		missingSources = append(missingSources, "recovery_context.command_path")
	}
	if len(rc.ArgsSummary) > 0 {
		availableSources = append(availableSources, "recovery_context.args_summary")
	} else {
		missingSources = append(missingSources, "recovery_context.args_summary")
	}
	if len(replay.ToolArgs) > 0 {
		availableSources = append(availableSources, "replay.tool_args")
	} else {
		missingSources = append(missingSources, "replay.tool_args")
	}
	if strings.TrimSpace(replay.RedactedCommand) != "" {
		availableSources = append(availableSources, "replay.redacted_command")
	} else {
		missingSources = append(missingSources, "replay.redacted_command")
	}
	if plan.DocSearch.Status != "" && plan.DocSearch.Status != "skipped" {
		availableSources = append(availableSources, "plan.doc_search")
	} else {
		missingSources = append(missingSources, "plan.doc_search")
	}
	if len(plan.KBHits) > 0 {
		availableSources = append(availableSources, "plan.kb_hits")
	} else {
		missingSources = append(missingSources, "plan.kb_hits")
	}
	if len(plan.DocActions) > 0 {
		availableSources = append(availableSources, "plan.doc_actions")
	} else {
		missingSources = append(missingSources, "plan.doc_actions")
	}

	summary := fmt.Sprintf("audited %d local recovery source(s)", len(sourcesChecked))
	if len(candidateIdentifiers) == 0 {
		summary += "; no identifier candidates found"
	} else {
		summary += fmt.Sprintf("; found %d identifier candidate(s)", len(candidateIdentifiers))
	}

	return &ProbeResult{
		Name:    "unknown_context_audit",
		Status:  "success",
		Summary: summary,
		Output: map[string]any{
			"sources_checked":       sourcesChecked,
			"available_sources":     availableSources,
			"missing_sources":       missingSources,
			"candidate_identifiers": candidateIdentifiers,
			"decision_owner":        string(normalizeDecisionOwner(plan)),
		},
	}
}

func probeAITableBaseCatalog(ctx context.Context, invoker ToolInvoker, rc RecoveryContext, replay Replay, plan RecoveryPlan) *ProbeResult {
	if len(rc.CommandPath) < 2 || rc.CommandPath[0] != "aitable" || rc.CommandPath[1] != "base" {
		return nil
	}
	if len(rc.CommandPath) < 3 || rc.CommandPath[2] != "get" {
		return &ProbeResult{Name: "aitable_base_catalog_probe", Status: "skipped", Summary: "no registered read-only probe for this aitable command"}
	}
	if invoker == nil {
		return &ProbeResult{Name: "aitable_base_catalog_probe", Status: "skipped", Summary: "tool invoker unavailable"}
	}
	result, err := invoker.CallToolDirect(ctx, replay.ServerID, "list_bases", map[string]any{"limit": 5})
	if err != nil {
		return &ProbeResult{
			Name:     "aitable_base_catalog_probe",
			Status:   "error",
			ServerID: replay.ServerID,
			ToolName: "list_bases",
			Error:    err.Error(),
			Summary:  "failed to run read-only aitable base catalog probe",
		}
	}
	payload, summary := summarizeProbeOutput(result)
	return &ProbeResult{
		Name:        "aitable_base_catalog_probe",
		Status:      "success",
		ServerID:    replay.ServerID,
		ToolName:    "list_bases",
		ArgsSummary: SummarizeArgs(map[string]any{"limit": 5}),
		Summary:     summary,
		Output:      payload,
	}
}

func summarizeProbeOutput(result *transport.ToolCallResult) (any, string) {
	if result == nil {
		return nil, "probe returned no output"
	}
	for _, block := range result.Blocks {
		if block.Type != "text" || strings.TrimSpace(block.Text) == "" {
			continue
		}
		var parsed any
		if err := json.Unmarshal([]byte(block.Text), &parsed); err == nil {
			return parsed, "probe returned JSON payload"
		}
		return block.Text, "probe returned text payload"
	}
	if len(result.Content) > 0 {
		return result.Content, "probe returned content payload"
	}
	return result, "probe returned non-text payload"
}

func buildAgentTask(eventID string, rc RecoveryContext, replay Replay, plan RecoveryPlan, probes []ProbeResult) AgentTask {
	whyParts := []string{fmt.Sprintf("recovery category=%s", plan.Category)}
	if len(probes) > 0 {
		whyParts = append(whyParts, fmt.Sprintf("probe results=%d", len(probes)))
	}
	if plan.DocSearch.Status != "" && plan.DocSearch.Status != "skipped" {
		whyParts = append(whyParts, "doc search results included")
	}
	if replay.RedactedCommand != "" {
		whyParts = append(whyParts, "failed command="+replay.RedactedCommand)
	}

	return AgentTask{
		Goal:         "阅读完整 RecoveryBundle，并基于 dws skill 对 unknown 恢复场景做整体分析后再决定是否发起 grounded 的下一次 dws 尝试",
		Why:          strings.Join(whyParts, "; "),
		MustReadRefs: buildMustReadRefs(rc.CommandPath),
		AllowedActions: []string{
			"读取完整 recovery bundle、recovery-guide 和对应产品参考文档",
			"基于 doc_actions、kb_hits、probe_results 与 replay/context 整体判断下一步",
			"仅在修复依据明确时重新发起新的 dws 命令",
		},
		ForbiddenActions: []string{
			"编造 ID、UUID、token、URL 或其他业务参数",
			"绕过 dws 直接调用 HTTP API、curl 或浏览器",
			"未确认前把失败命令替换成另一套业务流程",
		},
		StopConditions: []string{
			"新的 dws 命令已经成功并可回写 recovered",
			"没有 grounded 修复依据，只能回写 handoff",
			"继续尝试会要求猜测或扩大副作用边界",
		},
		FinalizeRequirement: buildFinalizeHint(eventID).Command,
	}
}

func buildFinalizeHint(eventID string) FinalizeHint {
	return FinalizeHint{
		Required: true,
		Command: fmt.Sprintf(
			"dws recovery finalize --event-id %s --outcome <recovered|failed|handoff> --execution-file <file.json> --format json",
			eventID,
		),
		ExecutionFileFields: []string{"actions", "attempts", "result", "error_summary"},
		AllowedOutcomes:     []string{"recovered", "failed", "handoff"},
	}
}

func buildMustReadRefs(commandPath []string) []string {
	refs := []string{
		"skills/references/recovery-guide.md",
		"skills/references/error-codes.md",
		"skills/references/global-reference.md",
	}
	if len(commandPath) > 0 {
		if ref := productReferenceFor(commandPath[0]); ref != "" {
			refs = append(refs, ref)
		}
	}
	return refs
}

func productReferenceFor(commandRoot string) string {
	refs := map[string]string{
		"aitable":    "skills/references/products/aitable.md",
		"attendance": "skills/references/products/attendance.md",
		"calendar":   "skills/references/products/calendar.md",
		"chat":       "skills/references/products/chat.md",
		"contact":    "skills/references/products/contact.md",
		"devdoc":     "skills/references/products/simple.md",
		"ding":       "skills/references/products/ding.md",
		"report":     "skills/references/products/report.md",
		"todo":       "skills/references/products/todo.md",
		"workbench":  "skills/references/products/workbench.md",
		"approval":   "skills/references/products/simple.md",
	}
	return refs[commandRoot]
}

func cloneProbeResults(results []ProbeResult) []ProbeResult {
	if len(results) == 0 {
		return nil
	}
	out := make([]ProbeResult, len(results))
	for i, result := range results {
		out[i] = result
		out[i].ArgsSummary = cloneMap(result.ArgsSummary)
	}
	return out
}
