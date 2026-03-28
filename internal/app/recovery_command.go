package app

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/cli"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/recovery"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/transport"
	"github.com/spf13/cobra"
)

func newRecoveryCommand(_ context.Context, loader cli.CatalogLoader, flags *GlobalFlags) *cobra.Command {
	var (
		planUseLast    bool
		planEventID    string
		executeUseLast bool
		executeEventID string
		finalEventID   string
		finalOutcome   string
		executionFile  string
	)

	runtime := newRecoveryRuntime(loader, flags)

	cmd := &cobra.Command{
		Use:               "recovery",
		Short:             "错误恢复辅助命令",
		Long:              "读取失败快照，生成恢复分析，并回写恢复结果。",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}

	planCmd := &cobra.Command{
		Use:               "plan",
		Short:             "基于失败快照生成恢复计划",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			store := recovery.NewStore(defaultConfigDir())
			last, err := loadRecoverySnapshot(store, planUseLast, planEventID)
			if err != nil {
				return err
			}

			planner := recovery.NewPlanner(runtime)
			plan := planner.PlanWithOptions(cmd.Context(), last.Context, recovery.PlanOptions{
				EventID:         last.EventID,
				EnableDocSearch: true,
			})
			recovery.HydratePlanForEvent(last.EventID, last.Context, last.Replay, &plan)
			if err := store.SavePlan(last.EventID, plan); err != nil {
				return fmt.Errorf("保存恢复计划失败: %w", err)
			}

			payload := map[string]any{
				"event_id": last.EventID,
				"context":  last.Context,
				"plan":     plan,
			}
			return output.WriteCommandPayload(cmd, payload, output.FormatJSON)
		},
	}
	planCmd.Flags().BoolVar(&planUseLast, "last", false, "读取最近一次失败快照")
	planCmd.Flags().StringVar(&planEventID, "event-id", "", "按 event_id 读取失败快照")

	executeCmd := &cobra.Command{
		Use:               "execute",
		Short:             "生成面向 Agent 的恢复分析包",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			store := recovery.NewStore(defaultConfigDir())
			last, err := loadRecoverySnapshot(store, executeUseLast, executeEventID)
			if err != nil {
				return err
			}

			planner := recovery.NewPlanner(runtime)
			executor := recovery.NewExecutor(planner, runtime)
			bundle := executor.Execute(cmd.Context(), *last)
			if err := store.SaveAnalysis(last.EventID, bundle.Plan, bundle); err != nil {
				return fmt.Errorf("保存恢复分析失败: %w", err)
			}

			return output.WriteCommandPayload(cmd, bundle, output.FormatJSON)
		},
	}
	executeCmd.Flags().BoolVar(&executeUseLast, "last", false, "读取最近一次失败快照")
	executeCmd.Flags().StringVar(&executeEventID, "event-id", "", "按 event_id 读取失败快照")

	finalizeCmd := &cobra.Command{
		Use:               "finalize",
		Short:             "回写恢复闭环结果",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if strings.TrimSpace(finalEventID) == "" {
				return fmt.Errorf("必须提供 --event-id")
			}
			if strings.TrimSpace(finalOutcome) == "" {
				return fmt.Errorf("必须提供 --outcome")
			}
			switch finalOutcome {
			case "recovered", "failed", "handoff":
			default:
				return fmt.Errorf("--outcome 仅支持 recovered|failed|handoff")
			}

			store := recovery.NewStore(defaultConfigDir())
			var execution *recovery.RecoveryExecution
			if strings.TrimSpace(executionFile) != "" {
				loaded, err := loadRecoveryExecution(executionFile)
				if err != nil {
					return err
				}
				execution = &loaded
			}
			if err := store.Finalize(finalEventID, finalOutcome, execution); err != nil {
				return fmt.Errorf("回写恢复结果失败: %w", err)
			}

			payload := map[string]any{
				"event_id": finalEventID,
				"outcome":  finalOutcome,
				"success":  true,
			}
			if execution != nil {
				payload["execution_recorded"] = true
			}
			return output.WriteCommandPayload(cmd, payload, output.FormatJSON)
		},
	}
	finalizeCmd.Flags().StringVar(&finalEventID, "event-id", "", "恢复事件 ID")
	finalizeCmd.Flags().StringVar(&finalOutcome, "outcome", "", "恢复结果: recovered|failed|handoff")
	finalizeCmd.Flags().StringVar(&executionFile, "execution-file", "", "Agent 执行详情 JSON 文件")

	cmd.AddCommand(planCmd, executeCmd, finalizeCmd)
	return cmd
}

func loadRecoverySnapshot(store *recovery.Store, useLast bool, eventID string) (*recovery.LastError, error) {
	if useLast && strings.TrimSpace(eventID) != "" {
		return nil, fmt.Errorf("--last 和 --event-id 不能同时使用")
	}
	switch {
	case useLast:
		last, err := store.LoadLastError()
		if err != nil {
			return nil, fmt.Errorf("读取失败快照失败: %w", err)
		}
		return last, nil
	case strings.TrimSpace(eventID) != "":
		last, err := store.LoadErrorByEvent(strings.TrimSpace(eventID))
		if err != nil {
			return nil, fmt.Errorf("读取失败快照失败: %w", err)
		}
		return last, nil
	default:
		return nil, fmt.Errorf("必须通过 --last 或 --event-id 指定失败快照")
	}
}

func loadRecoveryExecution(path string) (recovery.RecoveryExecution, error) {
	var execution recovery.RecoveryExecution
	data, err := os.ReadFile(path)
	if err != nil {
		return execution, fmt.Errorf("读取恢复执行详情失败: %w", err)
	}
	var payload recoveryExecutionPayload
	if err := json.Unmarshal(data, &payload); err != nil {
		return execution, fmt.Errorf("解析恢复执行详情失败: %w", err)
	}
	execution.Actions = append([]string(nil), payload.Actions...)
	if len(execution.Actions) == 0 && strings.TrimSpace(payload.Action) != "" {
		execution.Actions = []string{strings.TrimSpace(payload.Action)}
	}
	execution.Result = strings.TrimSpace(payload.Result)
	execution.ErrorSummary = strings.TrimSpace(payload.ErrorSummary)
	if execution.ErrorSummary == "" {
		execution.ErrorSummary = strings.TrimSpace(payload.Error)
	}

	attempts, err := decodeRecoveryAttempts(payload.Attempts, execution.Actions, execution.Result, execution.ErrorSummary)
	if err != nil {
		return execution, fmt.Errorf("解析恢复执行详情失败: %w", err)
	}
	if len(attempts) == 0 && payload.Attempt > 0 {
		attempts = legacyRecoveryAttempts(payload.Attempt, execution.Actions, execution.Result, execution.ErrorSummary)
	}
	execution.Attempts = attempts
	return execution, nil
}

type recoveryExecutionPayload struct {
	Action       string          `json:"action,omitempty"`
	Actions      []string        `json:"actions,omitempty"`
	Attempt      int             `json:"attempt,omitempty"`
	Attempts     json.RawMessage `json:"attempts,omitempty"`
	Result       string          `json:"result,omitempty"`
	Error        string          `json:"error,omitempty"`
	ErrorSummary string          `json:"error_summary,omitempty"`
}

func decodeRecoveryAttempts(raw json.RawMessage, actions []string, result, errorSummary string) ([]recovery.RecoveryAttempt, error) {
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" || trimmed == "null" {
		return nil, nil
	}
	if strings.HasPrefix(trimmed, "[") {
		var attempts []recovery.RecoveryAttempt
		if err := json.Unmarshal(raw, &attempts); err != nil {
			return nil, err
		}
		return attempts, nil
	}

	var count int
	if err := json.Unmarshal(raw, &count); err != nil {
		return nil, err
	}
	return legacyRecoveryAttempts(count, actions, result, errorSummary), nil
}

func legacyRecoveryAttempts(count int, actions []string, result, errorSummary string) []recovery.RecoveryAttempt {
	if count <= 0 {
		return nil
	}
	summary := strings.TrimSpace(strings.Join(actions, ", "))
	if summary == "" {
		summary = "legacy execution attempt"
	}
	attempts := make([]recovery.RecoveryAttempt, 0, count)
	for i := 0; i < count; i++ {
		attempts = append(attempts, recovery.RecoveryAttempt{
			CommandSummary: summary,
			Result:         result,
			ErrorSummary:   errorSummary,
			Source:         "legacy_execution_file",
		})
	}
	return attempts
}

type recoveryRuntime struct {
	loader    cli.CatalogLoader
	transport *transport.Client
	flags     *GlobalFlags
}

func newRecoveryRuntime(loader cli.CatalogLoader, flags *GlobalFlags) *recoveryRuntime {
	var httpClient *http.Client
	if flags != nil && flags.Timeout > 0 {
		httpClient = &http.Client{Timeout: time.Duration(flags.Timeout) * time.Second}
	}
	client := transport.NewClient(httpClient)
	client.ExtraHeaders = resolveIdentityHeaders()
	return &recoveryRuntime{
		loader:    loader,
		transport: client,
		flags:     flags,
	}
}

func (r *recoveryRuntime) Search(ctx context.Context, query string, rc recovery.RecoveryContext) (recovery.KnowledgeRetrieval, error) {
	const (
		searchPage = 1
		searchSize = 5
	)
	requestArgs := map[string]any{
		"keyword": query,
		"page":    searchPage,
		"size":    searchSize,
	}

	retrieval := recovery.KnowledgeRetrieval{
		DocSearch: recovery.DocSearch{
			Provider: "open_platform_docs",
			Query:    query,
			Page:     searchPage,
			Size:     searchSize,
			Status:   "empty",
			Request: &recovery.ToolCallRecord{
				ServerID:  "devdoc",
				ToolName:  "search_open_platform_docs",
				Arguments: cloneRecoveryArgs(requestArgs),
			},
		},
	}
	if r == nil || strings.TrimSpace(query) == "" {
		retrieval.DocSearch.Status = "skipped"
		return retrieval, nil
	}
	result, err := r.CallToolDirect(ctx, "devdoc", "search_open_platform_docs", requestArgs)
	if result != nil {
		retrieval.DocSearch.Response = toRecoveryToolResponse(result)
	}
	if err != nil {
		retrieval.DocSearch.Status = "error"
		retrieval.DocSearch.Error = err.Error()
		return retrieval, err
	}

	retrieval.DocSearch.Items = parseDocSearchItems(result)
	if len(retrieval.DocSearch.Items) > 0 {
		retrieval.DocSearch.Status = "success"
		retrieval.KBHits = rerankDocSearchHits(query, rc, retrieval.DocSearch.Items)
	}
	return retrieval, nil
}

func (r *recoveryRuntime) CallToolDirect(ctx context.Context, serverID, toolName string, args map[string]any) (*transport.ToolCallResult, error) {
	if r == nil || r.transport == nil {
		return nil, fmt.Errorf("recovery runtime not initialized")
	}
	endpoint, err := r.resolveEndpoint(ctx, serverID)
	if err != nil {
		return nil, err
	}
	tc := r.transport.WithAuth(resolveRuntimeAuthToken(ctx, recoveryRuntimeToken(r.flags)), resolveIdentityHeaders())
	result, err := tc.CallTool(ctx, endpoint, toolName, args)
	if err != nil {
		return nil, err
	}
	if result.IsError {
		return &result, apperrors.NewAPI(
			extractMCPErrorMessage(result),
			apperrors.WithOperation("tools/call"),
			apperrors.WithReason("mcp_tool_error"),
			apperrors.WithServerKey(serverID),
		)
	}
	return &result, nil
}

func (r *recoveryRuntime) resolveEndpoint(ctx context.Context, productID string) (string, error) {
	if endpoint, ok := directRuntimeEndpoint(productID); ok {
		return endpoint, nil
	}
	if r == nil || r.loader == nil {
		return "", fmt.Errorf("未找到服务 %s 的 endpoint", productID)
	}
	catalog, err := r.loader.Load(ctx)
	if err != nil {
		return "", err
	}
	product, ok := catalog.FindProduct(productID)
	if !ok || strings.TrimSpace(product.Endpoint) == "" {
		return "", fmt.Errorf("未找到服务 %s 的 endpoint", productID)
	}
	return strings.TrimSpace(product.Endpoint), nil
}

func recoveryRuntimeToken(flags *GlobalFlags) string {
	if flags == nil {
		return ""
	}
	return strings.TrimSpace(flags.Token)
}

func toRecoveryToolResponse(result *transport.ToolCallResult) *recovery.ToolResponse {
	if result == nil {
		return nil
	}
	response := &recovery.ToolResponse{IsError: result.IsError}
	if len(result.Blocks) > 0 {
		response.Content = make([]recovery.ToolResponseBlock, 0, len(result.Blocks))
		for _, block := range result.Blocks {
			response.Content = append(response.Content, recovery.ToolResponseBlock{
				Type: block.Type,
				Text: block.Text,
			})
		}
	}
	return response
}

func parseDocSearchItems(result *transport.ToolCallResult) []recovery.DocSearchItem {
	if result == nil {
		return nil
	}
	if items := parseDocSearchItemsFromMap(result.Content); len(items) > 0 {
		return items
	}
	for _, block := range result.Blocks {
		var payload map[string]any
		if err := json.Unmarshal([]byte(block.Text), &payload); err == nil {
			if items := parseDocSearchItemsFromMap(payload); len(items) > 0 {
				return items
			}
		}
	}
	return nil
}

func parseDocSearchItemsFromMap(payload map[string]any) []recovery.DocSearchItem {
	if len(payload) == 0 {
		return nil
	}
	if items := toDocSearchItems(payload["items"]); len(items) > 0 {
		return items
	}
	if data, ok := payload["data"].(map[string]any); ok {
		if items := toDocSearchItems(data["items"]); len(items) > 0 {
			return items
		}
	}
	if result, ok := payload["result"].(map[string]any); ok {
		if items := toDocSearchItems(result["items"]); len(items) > 0 {
			return items
		}
	}
	return nil
}

func toDocSearchItems(raw any) []recovery.DocSearchItem {
	list, ok := raw.([]any)
	if !ok {
		return nil
	}
	items := make([]recovery.DocSearchItem, 0, len(list))
	for _, entry := range list {
		object, ok := entry.(map[string]any)
		if !ok {
			continue
		}
		item := recovery.DocSearchItem{}
		if title, ok := object["title"].(string); ok {
			item.Title = title
		}
		if url, ok := object["url"].(string); ok {
			item.URL = url
		}
		if desc, ok := object["desc"].(string); ok {
			item.Desc = desc
		}
		if item.Title != "" || item.URL != "" || item.Desc != "" {
			items = append(items, item)
		}
	}
	return items
}

func rerankDocSearchHits(query string, rc recovery.RecoveryContext, items []recovery.DocSearchItem) []recovery.KBHit {
	if len(items) == 0 {
		return nil
	}
	keywords := strings.Fields(strings.ToLower(strings.TrimSpace(query)))
	type scoredHit struct {
		hit   recovery.KBHit
		score float64
	}
	scored := make([]scoredHit, 0, len(items))
	for _, item := range items {
		text := strings.ToLower(strings.Join(append([]string{
			item.Title,
			item.URL,
			item.Desc,
			rc.ToolName,
		}, rc.CommandPath...), " "))
		score := 0.0
		for _, keyword := range keywords {
			if strings.Contains(text, keyword) {
				score += 1
			}
		}
		scored = append(scored, scoredHit{
			hit: recovery.KBHit{
				Source:  "open_platform_docs",
				Title:   item.Title,
				URL:     item.URL,
				Snippet: item.Desc,
				Score:   score,
			},
			score: score,
		})
	}
	sort.SliceStable(scored, func(i, j int) bool {
		return scored[i].score > scored[j].score
	})
	limit := len(scored)
	if limit > 3 {
		limit = 3
	}
	hits := make([]recovery.KBHit, 0, limit)
	for _, item := range scored[:limit] {
		hits = append(hits, item.hit)
	}
	return hits
}
