package recovery

import (
	"crypto/sha1"
	"encoding/hex"
	stderrors "errors"
	"fmt"
	"regexp"
	"sort"
	"strings"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/platform/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/runtime/transport"
)

type OperationKind string

const (
	OperationRead    OperationKind = "read"
	OperationWrite   OperationKind = "write"
	OperationUnknown OperationKind = "unknown"
)

type DecisionOwner string

const (
	DecisionOwnerBuiltinRule DecisionOwner = "builtin_rule"
	DecisionOwnerAgent       DecisionOwner = "agent"
)

type RecoveryContext struct {
	CommandPath   []string       `json:"command_path"`
	ServerID      string         `json:"server_id,omitempty"`
	ToolName      string         `json:"tool_name,omitempty"`
	OperationKind OperationKind  `json:"operation_kind"`
	CLIErrorCode  string         `json:"cli_error_code,omitempty"`
	RawError      string         `json:"raw_error"`
	CallStage     string         `json:"call_stage,omitempty"`
	HTTPStatus    int            `json:"http_status,omitempty"`
	RetryAfter    string         `json:"retry_after,omitempty"`
	TraceID       string         `json:"trace_id,omitempty"`
	RequestID     string         `json:"request_id,omitempty"`
	ArgsSummary   map[string]any `json:"args_summary,omitempty"`
	Fingerprint   string         `json:"fingerprint"`
}

type Replay struct {
	ServerID        string         `json:"server_id,omitempty"`
	ToolName        string         `json:"tool_name,omitempty"`
	OperationKind   OperationKind  `json:"operation_kind"`
	ToolArgs        map[string]any `json:"tool_args,omitempty"`
	RedactedArgv    []string       `json:"redacted_argv,omitempty"`
	RedactedCommand string         `json:"redacted_command,omitempty"`
}

type RecoveryPlan struct {
	Category      string        `json:"category"`
	DecisionOwner DecisionOwner `json:"decision_owner,omitempty"`
	Confidence    float64       `json:"confidence"`
	AutoActions   []string      `json:"auto_actions"`
	SafeActions   []string      `json:"safe_actions"`
	DocActions    []DocAction   `json:"doc_actions,omitempty"`
	DocSearch     DocSearch     `json:"doc_search"`
	DecisionHints DecisionHints `json:"decision_hints"`
	HumanActions  []string      `json:"human_actions"`
	Evidence      []string      `json:"evidence"`
	KBHits        []KBHit       `json:"kb_hits"`
	ShouldRetry   bool          `json:"should_retry"`
	ShouldStop    bool          `json:"should_stop"`
	RuleHints     RuleHints     `json:"rule_hints"`
	AgentRoute    AgentRoute    `json:"agent_route"`
}

type KBHit struct {
	Source  string  `json:"source"`
	Title   string  `json:"title"`
	URL     string  `json:"url,omitempty"`
	Snippet string  `json:"snippet,omitempty"`
	Score   float64 `json:"score,omitempty"`
}

type DocAction struct {
	Action      string `json:"action"`
	Reason      string `json:"reason,omitempty"`
	SourceTitle string `json:"source_title,omitempty"`
	SourceURL   string `json:"source_url,omitempty"`
}

type DocSearch struct {
	Provider    string          `json:"provider,omitempty"`
	Query       string          `json:"query,omitempty"`
	Page        int             `json:"page,omitempty"`
	Size        int             `json:"size,omitempty"`
	CurrentPage int             `json:"current_page,omitempty"`
	TotalCount  int             `json:"total_count,omitempty"`
	HasMore     bool            `json:"has_more"`
	Status      string          `json:"status,omitempty"`
	Error       string          `json:"error,omitempty"`
	Request     *ToolCallRecord `json:"request,omitempty"`
	Response    *ToolResponse   `json:"response,omitempty"`
	Items       []DocSearchItem `json:"items,omitempty"`
}

type DocSearchItem struct {
	Title string `json:"title"`
	URL   string `json:"url,omitempty"`
	Desc  string `json:"desc,omitempty"`
}

type ToolCallRecord struct {
	ServerID  string         `json:"server_id,omitempty"`
	ToolName  string         `json:"tool_name,omitempty"`
	Arguments map[string]any `json:"arguments,omitempty"`
}

type ToolResponse struct {
	IsError bool                `json:"is_error,omitempty"`
	Content []ToolResponseBlock `json:"content,omitempty"`
}

type ToolResponseBlock struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
}

type RuleHints struct {
	Category      string        `json:"category"`
	DecisionOwner DecisionOwner `json:"decision_owner,omitempty"`
	Confidence    float64       `json:"confidence"`
	SafeActions   []string      `json:"safe_actions"`
	HumanActions  []string      `json:"human_actions"`
	Evidence      []string      `json:"evidence"`
	ShouldRetry   bool          `json:"should_retry"`
	ShouldStop    bool          `json:"should_stop"`
}

type DecisionHints struct {
	Retryable            bool `json:"retryable"`
	PermissionSensitive  bool `json:"permission_sensitive"`
	AuthRelated          bool `json:"auth_related"`
	ResourceStateRelated bool `json:"resource_state_related"`
}

type AgentRoute struct {
	Required bool               `json:"required"`
	Target   string             `json:"target,omitempty"`
	Executor string             `json:"executor,omitempty"`
	Reasons  []string           `json:"reasons"`
	Payload  *AgentRoutePayload `json:"payload,omitempty"`
}

type AgentRoutePayload struct {
	EventID       string          `json:"event_id"`
	Context       RecoveryContext `json:"context"`
	Replay        Replay          `json:"replay"`
	RawError      string          `json:"raw_error"`
	Category      string          `json:"category"`
	DecisionOwner DecisionOwner   `json:"decision_owner,omitempty"`
	Confidence    float64         `json:"confidence"`
	ShouldRetry   bool            `json:"should_retry"`
	ShouldStop    bool            `json:"should_stop"`
	SafeActions   []string        `json:"safe_actions"`
	DocActions    []DocAction     `json:"doc_actions,omitempty"`
	KBHits        []KBHit         `json:"kb_hits"`
	DocSearch     DocSearch       `json:"doc_search"`
	HumanActions  []string        `json:"human_actions"`
	DecisionHints DecisionHints   `json:"decision_hints"`
	Evidence      []string        `json:"evidence"`
	RuleHints     RuleHints       `json:"rule_hints"`
	ProbeResults  []ProbeResult   `json:"probe_results,omitempty"`
}

type ProbeResult struct {
	Name        string         `json:"name"`
	Status      string         `json:"status"`
	ServerID    string         `json:"server_id,omitempty"`
	ToolName    string         `json:"tool_name,omitempty"`
	ArgsSummary map[string]any `json:"args_summary,omitempty"`
	Summary     string         `json:"summary,omitempty"`
	Output      any            `json:"output,omitempty"`
	Error       string         `json:"error,omitempty"`
}

type AgentTask struct {
	Goal                string   `json:"goal"`
	Why                 string   `json:"why"`
	MustReadRefs        []string `json:"must_read_refs,omitempty"`
	AllowedActions      []string `json:"allowed_actions,omitempty"`
	ForbiddenActions    []string `json:"forbidden_actions,omitempty"`
	StopConditions      []string `json:"stop_conditions,omitempty"`
	FinalizeRequirement string   `json:"finalize_requirement,omitempty"`
}

type FinalizeHint struct {
	Required            bool     `json:"required"`
	Command             string   `json:"command,omitempty"`
	ExecutionFileFields []string `json:"execution_file_fields,omitempty"`
	AllowedOutcomes     []string `json:"allowed_outcomes,omitempty"`
}

type RecoveryBundle struct {
	Status        string          `json:"status"`
	EventID       string          `json:"event_id"`
	Context       RecoveryContext `json:"context"`
	Replay        Replay          `json:"replay"`
	Plan          RecoveryPlan    `json:"plan"`
	DocSearch     DocSearch       `json:"doc_search"`
	KBHits        []KBHit         `json:"kb_hits,omitempty"`
	DocActions    []DocAction     `json:"doc_actions,omitempty"`
	HumanActions  []string        `json:"human_actions,omitempty"`
	ProbeResults  []ProbeResult   `json:"probe_results,omitempty"`
	AgentTask     AgentTask       `json:"agent_task"`
	FinalizeHint  FinalizeHint    `json:"finalize_hint"`
	AnalysisError string          `json:"analysis_error,omitempty"`
}

type LastError struct {
	EventID    string          `json:"event_id"`
	RecordedAt string          `json:"recorded_at"`
	Context    RecoveryContext `json:"context"`
	Replay     Replay          `json:"replay,omitempty"`
}

type RecoveryEvent struct {
	EventID    string             `json:"event_id"`
	Phase      string             `json:"phase"`
	RecordedAt string             `json:"recorded_at"`
	Context    *RecoveryContext   `json:"context,omitempty"`
	Replay     *Replay            `json:"replay,omitempty"`
	Plan       *RecoveryPlan      `json:"plan,omitempty"`
	Bundle     *RecoveryBundle    `json:"bundle,omitempty"`
	Execution  *RecoveryExecution `json:"execution,omitempty"`
	Outcome    string             `json:"outcome,omitempty"`
}

type RecoveryAttempt struct {
	CommandSummary string `json:"command_summary,omitempty"`
	Result         string `json:"result,omitempty"`
	ErrorSummary   string `json:"error_summary,omitempty"`
	Source         string `json:"source,omitempty"`
}

type RecoveryExecution struct {
	Actions      []string          `json:"actions,omitempty"`
	Attempts     []RecoveryAttempt `json:"attempts,omitempty"`
	Result       string            `json:"result,omitempty"`
	ErrorSummary string            `json:"error_summary,omitempty"`
}

type CaptureInput struct {
	CommandPath   []string
	ServerID      string
	ToolName      string
	OperationKind OperationKind
	Args          map[string]any
	Argv          []string
	RawErr        error
	WrappedErr    error
}

type KnowledgeRetrieval struct {
	KBHits    []KBHit   `json:"kb_hits"`
	DocSearch DocSearch `json:"doc_search"`
}

func BuildContext(input CaptureInput) RecoveryContext {
	operationKind := input.OperationKind
	if operationKind == "" || operationKind == OperationUnknown {
		operationKind = InferOperationKind(input.ToolName)
	}

	rawError := ""
	if input.RawErr != nil {
		rawError = input.RawErr.Error()
	}

	ctx := RecoveryContext{
		CommandPath:   append([]string(nil), input.CommandPath...),
		ServerID:      input.ServerID,
		ToolName:      input.ToolName,
		OperationKind: operationKind,
		RawError:      rawError,
		ArgsSummary:   SummarizeArgs(input.Args),
	}

	if code := canonicalCLIErrorCode(input.WrappedErr, rawError); code != "" {
		ctx.CLIErrorCode = code
	}

	var callErr *transport.CallError
	if stderrors.As(input.RawErr, &callErr) || stderrors.As(input.WrappedErr, &callErr) {
		ctx.CallStage = string(callErr.Stage)
		ctx.HTTPStatus = callErr.HTTPStatus
		ctx.RetryAfter = callErr.RetryAfter
		ctx.TraceID = callErr.TraceID
		ctx.RequestID = callErr.RequestID
	}

	ctx.Fingerprint = ComputeFingerprint(ctx)
	return ctx
}

func canonicalCLIErrorCode(err error, rawError string) string {
	var typed *apperrors.Error
	if stderrors.As(err, &typed) {
		reason := strings.ToLower(strings.TrimSpace(typed.Reason))
		message := strings.ToLower(strings.TrimSpace(typed.Message + " " + rawError))
		switch typed.Category {
		case apperrors.CategoryValidation:
			if strings.Contains(message, "required") || strings.Contains(message, "missing") {
				return cliMissingParamCode
			}
			return cliInvalidJSONCode
		case apperrors.CategoryAuth:
			if reason == "http_403" || strings.Contains(message, "forbidden") || strings.Contains(message, "permission") || strings.Contains(message, "无权限") {
				return cliPermissionCode
			}
			return cliAuthExpiredCode
		case apperrors.CategoryAPI, apperrors.CategoryDiscovery:
			switch {
			case reason == "http_429" || typed.Retryable && strings.Contains(message, "rate limit"):
				return cliRateLimitCode
			case strings.Contains(reason, "timeout") || strings.Contains(message, "timeout") || strings.Contains(message, "deadline exceeded") || strings.Contains(message, "connection refused") || strings.Contains(message, "connection reset"):
				return cliTimeoutCode
			case strings.Contains(reason, "invalid_params"):
				return cliInvalidJSONCode
			case strings.Contains(reason, "method_not_found"):
				return cliResourceNotFoundCode
			case strings.Contains(reason, "http_404"):
				return cliResourceNotFoundCode
			}
		}
	}

	normalized := strings.ToLower(strings.TrimSpace(rawError))
	switch {
	case strings.Contains(normalized, "token验证失败") || strings.Contains(normalized, "user_token_illegal"):
		return cliAuthExpiredCode
	case strings.Contains(normalized, "forbidden") || strings.Contains(normalized, "permission") || strings.Contains(normalized, "无权限"):
		return cliPermissionCode
	case strings.Contains(normalized, "too many requests") || strings.Contains(normalized, "rate limit"):
		return cliRateLimitCode
	case strings.Contains(normalized, "timeout") || strings.Contains(normalized, "deadline exceeded") || strings.Contains(normalized, "connection refused") || strings.Contains(normalized, "connection reset"):
		return cliTimeoutCode
	case strings.Contains(normalized, "not found") || strings.Contains(normalized, "资源不存在") || strings.Contains(normalized, "base_not_found"):
		return cliResourceNotFoundCode
	}

	return ""
}

func BuildReplay(input CaptureInput) Replay {
	operationKind := input.OperationKind
	if operationKind == "" || operationKind == OperationUnknown {
		operationKind = InferOperationKind(input.ToolName)
	}

	replay := Replay{
		ServerID:      input.ServerID,
		ToolName:      input.ToolName,
		OperationKind: operationKind,
		ToolArgs:      sanitizeReplayMap(input.Args),
	}
	if len(input.Argv) > 0 {
		replay.RedactedArgv = sanitizeArgv(input.Argv)
		replay.RedactedCommand = strings.Join(replay.RedactedArgv, " ")
	}
	return replay
}

func InferOperationKind(toolName string) OperationKind {
	toolName = strings.ToLower(strings.TrimSpace(toolName))
	if toolName == "" {
		return OperationUnknown
	}

	readPrefixes := []string{"get_", "list_", "search_", "query_", "status_", "download_"}
	for _, prefix := range readPrefixes {
		if strings.HasPrefix(toolName, prefix) {
			return OperationRead
		}
	}

	writePrefixes := []string{
		"create_", "update_", "delete_", "send_", "approve_", "reject_", "revoke_",
		"upload_", "import_", "commit_", "add_", "remove_", "modify_", "set_",
		"assign_", "insert_", "recall_", "batch_send_", "batch_recall_", "save_",
		"generate", "edit", "upscale", "isolate",
	}
	for _, prefix := range writePrefixes {
		if strings.HasPrefix(toolName, prefix) {
			return OperationWrite
		}
	}
	return OperationUnknown
}

func SummarizeArgs(args map[string]any) map[string]any {
	if len(args) == 0 {
		return nil
	}
	keys := make([]string, 0, len(args))
	for key := range args {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	summary := make(map[string]any, len(keys))
	for _, key := range keys {
		if isSensitiveKey(key) {
			continue
		}
		summary[key] = summarizeValue(key, args[key])
	}
	return summary
}

func sanitizeReplayMap(args map[string]any) map[string]any {
	if len(args) == 0 {
		return nil
	}
	out := make(map[string]any, len(args))
	for key, value := range args {
		if isSensitiveKey(key) {
			continue
		}
		out[key] = sanitizeReplayField(key, value)
	}
	return out
}

func sanitizeReplayField(key string, value any) any {
	lowerKey := strings.ToLower(key)
	if isContentKey(lowerKey) {
		return summarizeValue(lowerKey, value)
	}
	switch v := value.(type) {
	case string:
		if len(v) > 120 {
			return summarizeValue(lowerKey, v)
		}
		return v
	case map[string]string:
		return sanitizeReplayStringMap(v)
	case []map[string]any:
		out := make([]map[string]any, 0, len(v))
		for _, item := range v {
			out = append(out, sanitizeReplayMap(item))
		}
		return out
	default:
		return sanitizeReplayValue(v)
	}
}

func sanitizeReplayValue(value any) any {
	switch v := value.(type) {
	case map[string]any:
		return sanitizeReplayMap(v)
	case map[string]string:
		return sanitizeReplayStringMap(v)
	case []any:
		out := make([]any, 0, len(v))
		for _, item := range v {
			out = append(out, sanitizeReplayValue(item))
		}
		return out
	case []string:
		out := make([]string, len(v))
		copy(out, v)
		return out
	case []map[string]any:
		out := make([]map[string]any, 0, len(v))
		for _, item := range v {
			out = append(out, sanitizeReplayMap(item))
		}
		return out
	default:
		return v
	}
}

func sanitizeReplayStringMap(args map[string]string) map[string]any {
	if len(args) == 0 {
		return nil
	}
	out := make(map[string]any, len(args))
	for key, value := range args {
		if isSensitiveKey(key) {
			continue
		}
		out[key] = sanitizeReplayField(key, value)
	}
	return out
}

func sanitizeArgv(argv []string) []string {
	if len(argv) == 0 {
		return nil
	}
	sensitiveFlags := map[string]struct{}{
		"--token":         {},
		"--auth-code":     {},
		"--refresh-token": {},
	}
	out := make([]string, 0, len(argv))
	skipValue := false
	for _, arg := range argv {
		if skipValue {
			out = append(out, "<redacted>")
			skipValue = false
			continue
		}
		if value, ok := redactSensitiveArg(arg); ok {
			out = append(out, value)
			continue
		}
		if _, ok := sensitiveFlags[arg]; ok {
			out = append(out, arg)
			skipValue = true
			continue
		}
		out = append(out, arg)
	}
	return out
}

func redactSensitiveArg(arg string) (string, bool) {
	for _, prefix := range []string{"--token=", "--auth-code=", "--refresh-token="} {
		if strings.HasPrefix(arg, prefix) {
			return prefix + "<redacted>", true
		}
	}
	return "", false
}

func summarizeValue(key string, value any) any {
	lowerKey := strings.ToLower(key)
	switch v := value.(type) {
	case string:
		if isContentKey(lowerKey) || len(v) > 120 {
			return map[string]any{"kind": "string", "length": float64(len(v))}
		}
		return v
	case []string:
		if isContentKey(lowerKey) {
			return map[string]any{"kind": "array", "count": float64(len(v))}
		}
		out := make([]string, len(v))
		copy(out, v)
		return out
	case []any:
		return map[string]any{"kind": "array", "count": float64(len(v))}
	case []map[string]any:
		if isContentKey(lowerKey) {
			return map[string]any{"kind": "array", "count": float64(len(v))}
		}
		out := make([]map[string]any, 0, len(v))
		for _, item := range v {
			nested := make(map[string]any, len(item))
			for nestedKey, nestedValue := range item {
				if isSensitiveKey(nestedKey) {
					continue
				}
				nested[nestedKey] = summarizeValue(nestedKey, nestedValue)
			}
			out = append(out, nested)
		}
		return out
	case map[string]any:
		if isContentKey(lowerKey) {
			return map[string]any{"kind": "object", "keys": sortedKeys(v)}
		}
		out := make(map[string]any, len(v))
		for nestedKey, nestedValue := range v {
			if isSensitiveKey(nestedKey) {
				continue
			}
			out[nestedKey] = summarizeValue(nestedKey, nestedValue)
		}
		return out
	case map[string]string:
		if isContentKey(lowerKey) {
			return map[string]any{"kind": "object", "keys": sortedStringKeys(v)}
		}
		out := make(map[string]any, len(v))
		for nestedKey, nestedValue := range v {
			if isSensitiveKey(nestedKey) {
				continue
			}
			out[nestedKey] = summarizeValue(nestedKey, nestedValue)
		}
		return out
	default:
		return value
	}
}

func ComputeFingerprint(ctx RecoveryContext) string {
	base := strings.Join([]string{
		ctx.ServerID,
		ctx.ToolName,
		ctx.CLIErrorCode,
		fmt.Sprintf("%d", ctx.HTTPStatus),
		normalizeRawError(ctx.RawError),
	}, "|")
	sum := sha1.Sum([]byte(base))
	return hex.EncodeToString(sum[:])
}

var (
	reURLQuery        = regexp.MustCompile(`https?://[^\s?]+(?:\?[^\s]+)?`)
	reUUID            = regexp.MustCompile(`\b[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}\b`)
	reLongDigits      = regexp.MustCompile(`\b\d{6,}\b`)
	reTimestamp       = regexp.MustCompile(`\b\d{4}-\d{2}-\d{2}[t\s]\d{2}:\d{2}:\d{2}(?:\.\d+)?(?:z|[+-]\d{2}:?\d{2})?\b`)
	reLongToken       = regexp.MustCompile(`\b[a-z0-9_-]{18,}\b`)
	reResourceID      = regexp.MustCompile(`\b(?:base|tbl|fld|rec|conv|user|task)_[a-z0-9_-]+\b`)
	reMultiWhitespace = regexp.MustCompile(`\s+`)
)

func normalizeRawError(raw string) string {
	normalized := strings.ToLower(strings.TrimSpace(raw))
	replacements := []struct {
		re   *regexp.Regexp
		repl string
	}{
		{reURLQuery, "<url>"},
		{reUUID, "<uuid>"},
		{reTimestamp, "<ts>"},
		{reResourceID, "<id>"},
		{reLongDigits, "<num>"},
		{reLongToken, "<token>"},
	}
	for _, item := range replacements {
		normalized = item.re.ReplaceAllString(normalized, item.repl)
	}
	normalized = reMultiWhitespace.ReplaceAllString(normalized, " ")
	return strings.TrimSpace(normalized)
}

func sortedKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func sortedStringKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func isSensitiveKey(key string) bool {
	key = strings.ToLower(key)
	sensitive := []string{"token", "authcode", "refresh", "cookie", "header", "authorization", "x-user-access-token"}
	for _, token := range sensitive {
		if strings.Contains(key, token) {
			return true
		}
	}
	return false
}

func isContentKey(key string) bool {
	key = strings.ToLower(key)
	content := []string{"text", "body", "markdown", "json", "records", "fields"}
	for _, token := range content {
		if strings.Contains(key, token) {
			return true
		}
	}
	return false
}
