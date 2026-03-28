package recovery

import (
	"context"
	"fmt"
	"strings"
)

type KnowledgeRetriever interface {
	Search(ctx context.Context, query string, rc RecoveryContext) (KnowledgeRetrieval, error)
}

type PlanOptions struct {
	EventID         string
	EnableDocSearch bool
}

type Planner struct {
	retriever KnowledgeRetriever
}

func NewPlanner(retriever KnowledgeRetriever) *Planner {
	return &Planner{retriever: retriever}
}

func (p *Planner) Plan(ctx context.Context, rc RecoveryContext) RecoveryPlan {
	return p.PlanWithOptions(ctx, rc, PlanOptions{EnableDocSearch: true})
}

func (p *Planner) PlanWithOptions(ctx context.Context, rc RecoveryContext, opts PlanOptions) RecoveryPlan {
	plan, exact := planExact(rc)
	if !exact {
		plan = RecoveryPlan{
			Category:      "unknown",
			DecisionOwner: DecisionOwnerAgent,
			Confidence:    0.35,
			Evidence:      []string{"未命中内置高频恢复规则"},
		}
	}

	routeReasons := buildAgentRouteReasons(plan)
	if opts.EnableDocSearch && len(routeReasons) > 0 {
		query := BuildFallbackQuery(rc)
		retrieval := p.searchKnowledge(ctx, query, rc)
		plan.DocSearch = retrieval.DocSearch
		if query != "" {
			plan.Evidence = append(plan.Evidence, fmt.Sprintf("fallback_query=%s", query))
		}
		switch plan.DocSearch.Status {
		case "success":
			plan.Evidence = append(plan.Evidence, "命中开放平台文档检索结果")
		case "empty":
			plan.Evidence = append(plan.Evidence, "开放平台文档检索无结果")
		case "error":
			if strings.TrimSpace(plan.DocSearch.Error) != "" {
				plan.Evidence = append(plan.Evidence, fmt.Sprintf("开放平台文档检索失败=%s", plan.DocSearch.Error))
			} else {
				plan.Evidence = append(plan.Evidence, "开放平台文档检索失败")
			}
		}
		if len(retrieval.KBHits) > 0 {
			plan.KBHits = append(plan.KBHits, retrieval.KBHits...)
			if plan.Category == "unknown" {
				plan.Confidence = 0.55
			}
		}
	} else {
		plan.DocSearch = DocSearch{Status: "skipped"}
	}

	plan.KBHits = dedupeKBHits(plan.KBHits)
	plan.DocActions = dedupeDocActions(extractDocActions(plan.KBHits))
	plan.SafeActions = nonNilStrings(normalizeSafeActions(plan.AutoActions))
	plan.AutoActions = nonNilStrings(plan.SafeActions)
	plan.DecisionOwner = normalizeDecisionOwner(plan)
	plan.DecisionHints = buildDecisionHints(plan, rc)
	plan.HumanActions = nonNilStrings(uniqueStrings(plan.HumanActions))
	plan.Evidence = nonNilStrings(uniqueStrings(plan.Evidence))
	plan.RuleHints = buildRuleHints(plan)
	plan.AgentRoute = buildAgentRoute(opts.EventID, rc, Replay{}, plan, routeReasons)
	return plan
}

func (p *Planner) searchKnowledge(ctx context.Context, query string, rc RecoveryContext) KnowledgeRetrieval {
	retrieval := KnowledgeRetrieval{DocSearch: DocSearch{Query: query, Status: "skipped"}}
	if query == "" || p == nil || p.retriever == nil {
		return retrieval
	}
	result, err := p.retriever.Search(ctx, query, rc)
	if result.DocSearch.Query == "" {
		result.DocSearch.Query = query
	}
	if err != nil {
		if result.DocSearch.Status == "" {
			result.DocSearch.Status = "error"
		}
		if strings.TrimSpace(result.DocSearch.Error) == "" {
			result.DocSearch.Error = err.Error()
		}
		return result
	}
	if result.DocSearch.Status == "" {
		if len(result.DocSearch.Items) > 0 {
			result.DocSearch.Status = "success"
		} else {
			result.DocSearch.Status = "empty"
		}
	}
	return result
}

func dedupeKBHits(hits []KBHit) []KBHit {
	if len(hits) <= 1 {
		return hits
	}
	seen := make(map[string]struct{}, len(hits))
	out := make([]KBHit, 0, len(hits))
	for _, hit := range hits {
		key := strings.TrimSpace(hit.Source) + "|" + strings.TrimSpace(hit.Title) + "|" + strings.TrimSpace(hit.URL)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, hit)
	}
	return out
}

func planExact(rc RecoveryContext) (RecoveryPlan, bool) {
	evidence := []string{}
	if rc.CLIErrorCode != "" {
		evidence = append(evidence, "cli_error_code="+rc.CLIErrorCode)
	}
	if rc.HTTPStatus != 0 {
		evidence = append(evidence, fmt.Sprintf("http_status=%d", rc.HTTPStatus))
	}
	if rc.CallStage != "" {
		evidence = append(evidence, "call_stage="+rc.CallStage)
	}

	switch {
	case rc.CLIErrorCode == cliAuthExpiredCode || strings.Contains(strings.ToLower(rc.RawError), "token验证失败") || strings.Contains(strings.ToLower(rc.RawError), "user_token_illegal"):
		shouldRetry := safeToRetry(rc, "auth")
		plan := RecoveryPlan{
			Category:      "auth",
			DecisionOwner: DecisionOwnerBuiltinRule,
			Confidence:    0.98,
			HumanActions:  []string{"如果重复执行两次仍失败，再运行 dws auth login 重新登录"},
			Evidence:      append(evidence, "命中认证失效规则"),
			KBHits:        []KBHit{{Source: "builtin", Title: "认证失效恢复规则", Score: 1}},
			ShouldRetry:   shouldRetry,
			ShouldStop:    !shouldRetry,
		}
		if shouldRetry {
			plan.AutoActions = []string{"rerun_original_command"}
		}
		return plan, true
	case rc.CLIErrorCode == cliInvalidJSONCode || rc.CLIErrorCode == cliMissingParamCode || strings.Contains(strings.ToLower(rc.RawError), "json 解析失败"):
		return RecoveryPlan{
			Category:      "input",
			DecisionOwner: DecisionOwnerBuiltinRule,
			Confidence:    0.96,
			HumanActions:  []string{"检查 JSON 结构、必填参数和字段名后再重试原命令"},
			Evidence:      append(evidence, "命中输入参数规则"),
			KBHits:        []KBHit{{Source: "builtin", Title: "输入参数修正规则", Score: 1}},
			ShouldStop:    true,
		}, true
	case strings.HasSuffix(rc.CLIErrorCode, "_NOT_FOUND") || rc.CLIErrorCode == cliResourceNotFoundCode || containsAny(rc.RawError, "not found", "资源不存在", "不存在", "does not exist", "has been deleted", "deleted", "base_not_found", "document.notfound", "resource.notfound", "specified base does not exist"):
		return RecoveryPlan{
			Category:      "resource",
			DecisionOwner: DecisionOwnerBuiltinRule,
			Confidence:    0.92,
			HumanActions:  []string{"先确认资源是否仍存在、ID 是否正确，再决定是否重试原命令"},
			Evidence:      append(evidence, "命中资源不存在规则"),
			KBHits:        []KBHit{{Source: "builtin", Title: "资源不存在恢复规则", Score: 1}},
			ShouldStop:    true,
		}, true
	case rc.CLIErrorCode == cliPermissionCode || rc.HTTPStatus == 403 || containsAny(rc.RawError, "forbidden", "permission", "权限不足", "无权限", "operationillegal"):
		return RecoveryPlan{
			Category:      "permission",
			DecisionOwner: DecisionOwnerBuiltinRule,
			Confidence:    0.95,
			HumanActions:  []string{"确认当前账号对目标资源有访问权限，必要时申请授权后再重试"},
			Evidence:      append(evidence, "命中权限规则"),
			KBHits:        []KBHit{{Source: "builtin", Title: "权限不足恢复规则", Score: 1}},
			ShouldStop:    true,
		}, true
	case rc.CLIErrorCode == cliRateLimitCode || rc.HTTPStatus == 429 || containsAny(rc.RawError, "too many requests", "rate limit"):
		shouldRetry := safeToRetry(rc, "rate_limit")
		plan := RecoveryPlan{
			Category:      "rate_limit",
			DecisionOwner: DecisionOwnerBuiltinRule,
			Confidence:    0.94,
			HumanActions:  []string{"如仍失败，请降低调用频率后再重试"},
			Evidence:      append(evidence, "命中限流规则"),
			KBHits:        []KBHit{{Source: "builtin", Title: "限流恢复规则", Score: 1}},
			ShouldRetry:   shouldRetry,
			ShouldStop:    !shouldRetry,
		}
		if shouldRetry {
			plan.AutoActions = []string{"wait_and_retry"}
		}
		return plan, true
	case rc.CLIErrorCode == cliTimeoutCode || rc.HTTPStatus >= 500 || containsAny(rc.RawError, "timeout", "deadline exceeded", "connection refused", "connection reset", "broken pipe", "502", "503", "504"):
		shouldRetry := safeToRetry(rc, "network")
		plan := RecoveryPlan{
			Category:      "network",
			DecisionOwner: DecisionOwnerBuiltinRule,
			Confidence:    0.9,
			HumanActions:  []string{"如果重试后仍失败，请检查网络或稍后再试"},
			Evidence:      append(evidence, "命中网络/服务异常规则"),
			KBHits:        []KBHit{{Source: "builtin", Title: "网络异常恢复规则", Score: 1}},
			ShouldRetry:   shouldRetry,
			ShouldStop:    !shouldRetry,
		}
		if shouldRetry {
			plan.AutoActions = []string{"retry_original_command"}
		}
		return plan, true
	}

	return RecoveryPlan{}, false
}

func BuildFallbackQuery(rc RecoveryContext) string {
	parts := make([]string, 0, 6)
	if rc.CLIErrorCode != "" {
		parts = append(parts, rc.CLIErrorCode)
	}
	if rc.ToolName != "" {
		parts = append(parts, rc.ToolName)
	}
	if len(rc.CommandPath) > 0 {
		parts = append(parts, strings.Join(rc.CommandPath, " "))
	}
	parts = append(parts, stableKeywords(rc.RawError)...)
	return strings.Join(uniqueStrings(parts), " ")
}

func safeToRetry(rc RecoveryContext, category string) bool {
	switch rc.OperationKind {
	case OperationRead:
		return category == "auth" || category == "rate_limit" || category == "network"
	case OperationWrite:
		if rc.CallStage != "http" {
			return false
		}
		return category == "auth" || category == "rate_limit"
	default:
		return false
	}
}

func normalizeDecisionOwner(plan RecoveryPlan) DecisionOwner {
	if plan.DecisionOwner != "" {
		return plan.DecisionOwner
	}
	if plan.Category == "unknown" {
		return DecisionOwnerAgent
	}
	return DecisionOwnerBuiltinRule
}

func stableKeywords(raw string) []string {
	normalized := normalizeRawError(raw)
	if normalized == "" {
		return nil
	}
	parts := strings.FieldsFunc(normalized, func(r rune) bool {
		return !(r >= 'a' && r <= 'z') && !(r >= '0' && r <= '9') && r != '_'
	})
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		if len(part) < 3 || strings.HasPrefix(part, "<") {
			continue
		}
		out = append(out, part)
		if len(out) >= 6 {
			break
		}
	}
	return uniqueStrings(out)
}

func uniqueStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}

func containsAny(value string, targets ...string) bool {
	value = strings.ToLower(value)
	for _, target := range targets {
		if strings.Contains(value, strings.ToLower(target)) {
			return true
		}
	}
	return false
}

const (
	cliAuthExpiredCode      = "AUTH_TOKEN_EXPIRED"
	cliInvalidJSONCode      = "INPUT_INVALID_JSON"
	cliMissingParamCode     = "INPUT_MISSING_PARAM"
	cliPermissionCode       = "AUTH_PERMISSION_DENIED"
	cliResourceNotFoundCode = "RESOURCE_NOT_FOUND"
	cliRateLimitCode        = "NETWORK_RATE_LIMITED"
	cliTimeoutCode          = "NETWORK_TIMEOUT"
)

func normalizeSafeActions(actions []string) []string {
	allowed := make([]string, 0, len(actions))
	for _, action := range actions {
		switch action {
		case "rerun_original_command", "retry_original_command", "wait_and_retry":
			allowed = append(allowed, action)
		}
	}
	return uniqueStrings(allowed)
}

func buildDecisionHints(plan RecoveryPlan, rc RecoveryContext) DecisionHints {
	signals := []string{rc.CLIErrorCode, rc.RawError}
	for _, hit := range plan.KBHits {
		signals = append(signals, hit.Title, hit.Snippet)
	}
	for _, action := range plan.DocActions {
		signals = append(signals, action.Action, action.Reason)
	}
	text := strings.Join(signals, " ")
	return DecisionHints{
		Retryable:            plan.ShouldRetry || len(plan.SafeActions) > 0,
		PermissionSensitive:  plan.Category == "permission" || containsAny(text, "permission", "forbidden", "无权限", "operatorid has permission", "有访问权限", "inaccessible"),
		AuthRelated:          plan.Category == "auth" || containsAny(text, "auth_token_expired", "token验证失败", "user_token_illegal", "认证失效", "token 已过期"),
		ResourceStateRelated: plan.Category == "resource" || containsAny(text, "not found", "does not exist", "deleted", "document.notfound", "resource.notfound", "资源不存在", "文档不存在", "彻底删除", "base_not_found"),
	}
}

func buildRuleHints(plan RecoveryPlan) RuleHints {
	return RuleHints{
		Category:      plan.Category,
		DecisionOwner: normalizeDecisionOwner(plan),
		Confidence:    plan.Confidence,
		SafeActions:   nonNilStrings(plan.SafeActions),
		HumanActions:  nonNilStrings(plan.HumanActions),
		Evidence:      nonNilStrings(plan.Evidence),
		ShouldRetry:   plan.ShouldRetry,
		ShouldStop:    plan.ShouldStop,
	}
}

func buildAgentRouteReasons(plan RecoveryPlan) []string {
	reasons := make([]string, 0, 2)
	if plan.Category == "unknown" {
		reasons = append(reasons, "unknown_category")
	}
	return nonNilStrings(uniqueStrings(reasons))
}

func buildAgentRoute(eventID string, rc RecoveryContext, replay Replay, plan RecoveryPlan, reasons []string) AgentRoute {
	required := len(reasons) > 0
	var payload *AgentRoutePayload
	decisionOwner := normalizeDecisionOwner(plan)
	if required {
		payload = &AgentRoutePayload{
			EventID:       eventID,
			Context:       cloneRecoveryContext(rc),
			Replay:        cloneReplay(replay),
			RawError:      rc.RawError,
			Category:      plan.Category,
			DecisionOwner: decisionOwner,
			Confidence:    plan.Confidence,
			ShouldRetry:   plan.ShouldRetry,
			ShouldStop:    plan.ShouldStop,
			SafeActions:   nonNilStrings(plan.SafeActions),
			DocActions:    cloneDocActions(plan.DocActions),
			KBHits:        cloneKBHits(plan.KBHits),
			DocSearch:     cloneDocSearch(plan.DocSearch),
			HumanActions:  nonNilStrings(plan.HumanActions),
			DecisionHints: plan.DecisionHints,
			Evidence:      nonNilStrings(plan.Evidence),
			RuleHints:     plan.RuleHints,
		}
	}
	return AgentRoute{
		Required: required,
		Target:   ternaryString(required, "agent_llm", ""),
		Executor: ternaryString(required, "dingtalk-workspace", ""),
		Reasons:  nonNilStrings(reasons),
		Payload:  payload,
	}
}

func HydratePlanForEvent(eventID string, rc RecoveryContext, replay Replay, plan *RecoveryPlan) {
	if plan == nil {
		return
	}
	plan.RuleHints = buildRuleHints(*plan)
	plan.AgentRoute = buildAgentRoute(eventID, rc, replay, *plan, buildAgentRouteReasons(*plan))
}

func extractDocActions(hits []KBHit) []DocAction {
	out := make([]DocAction, 0)
	for _, hit := range hits {
		if strings.TrimSpace(hit.Snippet) == "" {
			continue
		}
		lines := strings.Split(hit.Snippet, "\n")
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if !strings.HasPrefix(line, "|") {
				continue
			}
			cells := splitTableRow(line)
			if len(cells) < 5 {
				continue
			}
			if strings.EqualFold(cells[0], "httpcode") || strings.HasPrefix(cells[0], "--") {
				continue
			}
			action := strings.TrimSpace(cells[len(cells)-1])
			if action == "" || action == "%s" {
				continue
			}
			reasonParts := make([]string, 0, 2)
			if len(cells) > 1 && cells[1] != "" {
				reasonParts = append(reasonParts, cells[1])
			}
			if len(cells) > 3 && cells[3] != "" && cells[3] != "%s" {
				reasonParts = append(reasonParts, cells[3])
			}
			out = append(out, DocAction{
				Action:      action,
				Reason:      strings.Join(reasonParts, ": "),
				SourceTitle: hit.Title,
				SourceURL:   hit.URL,
			})
		}
	}
	return out
}

func splitTableRow(line string) []string {
	parts := strings.Split(line, "|")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		out = append(out, part)
	}
	return out
}

func dedupeDocActions(actions []DocAction) []DocAction {
	if len(actions) <= 1 {
		return actions
	}
	seen := make(map[string]struct{}, len(actions))
	out := make([]DocAction, 0, len(actions))
	for _, action := range actions {
		key := strings.TrimSpace(action.Action) + "|" + strings.TrimSpace(action.Reason) + "|" + strings.TrimSpace(action.SourceTitle) + "|" + strings.TrimSpace(action.SourceURL)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, action)
	}
	return out
}

func cloneDocActions(actions []DocAction) []DocAction {
	if len(actions) == 0 {
		return nil
	}
	out := make([]DocAction, len(actions))
	copy(out, actions)
	return out
}

func cloneKBHits(hits []KBHit) []KBHit {
	if len(hits) == 0 {
		return nil
	}
	out := make([]KBHit, len(hits))
	copy(out, hits)
	return out
}

func cloneDocSearch(search DocSearch) DocSearch {
	cloned := search
	if search.Request != nil {
		cloned.Request = &ToolCallRecord{
			ServerID:  search.Request.ServerID,
			ToolName:  search.Request.ToolName,
			Arguments: cloneMap(search.Request.Arguments),
		}
	}
	if search.Response != nil {
		cloned.Response = &ToolResponse{IsError: search.Response.IsError}
		if len(search.Response.Content) > 0 {
			cloned.Response.Content = make([]ToolResponseBlock, len(search.Response.Content))
			copy(cloned.Response.Content, search.Response.Content)
		}
	}
	if len(search.Items) > 0 {
		cloned.Items = make([]DocSearchItem, len(search.Items))
		copy(cloned.Items, search.Items)
	}
	return cloned
}

func ternaryString(ok bool, yes, no string) string {
	if ok {
		return yes
	}
	return no
}

func nonNilStrings(values []string) []string {
	if len(values) == 0 {
		return []string{}
	}
	return append([]string{}, values...)
}
