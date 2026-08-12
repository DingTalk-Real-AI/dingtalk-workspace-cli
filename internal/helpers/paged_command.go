package helpers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

const (
	defaultPagedCommandPageLimit = 50
	maxPagedCommandPageLimit     = 500
	defaultPagedCommandDelayMS   = 200
)

type PagedCursorKind int

const (
	PagedCursorString PagedCursorKind = iota
	PagedCursorInt64
)

type PagedAggregationMode int

const (
	PagedAggregationArray PagedAggregationMode = iota
	PagedAggregationConversationMessages
)

type PagedMCPCommandConfig struct {
	ServerID        string
	ToolName        string
	ItemPath        string
	CursorPath      string
	HasMorePath     string
	CursorArg       string
	CursorKind      PagedCursorKind
	AggregationMode PagedAggregationMode
	BuildArgs       func(*cobra.Command) (map[string]any, error)
	Fallback        func(map[string]any) error
	ProjectResult   func(map[string]any) map[string]any
}

type pagedCommandOptions struct {
	pageAll   bool
	pageLimit int
	maxItems  int
	delayMS   int
}

func AddPagedMCPFlags(cmd *cobra.Command) {
	cmd.Flags().Bool("page-all", false, "自动按 nextCursor 拉取所有分页；未设置时保持单页调用")
	cmd.Flags().Int("page-limit", defaultPagedCommandPageLimit, "自动翻页最多请求页数（默认 50，范围 1-500；仅 --page-all 生效）")
	cmd.Flags().Int("max-items", 0, "自动翻页最多返回条数（默认 0 表示不限制；仅 --page-all 生效）")
	cmd.Flags().Int("page-delay", defaultPagedCommandDelayMS, "自动翻页每页之间等待毫秒数（默认 200；0 表示不等待；仅 --page-all 生效）")
}

func RunPagedMCPCommand(cmd *cobra.Command, cfg PagedMCPCommandConfig) error {
	args, err := cfg.BuildArgs(cmd)
	if err != nil {
		return err
	}
	opts, err := readPagedCommandOptions(cmd)
	if err != nil {
		return err
	}
	if !opts.pageAll {
		return cfg.Fallback(args)
	}
	if err := validatePagedConfig(cfg); err != nil {
		return err
	}
	if deps.Caller.DryRun() {
		return deps.Out.PrintJSON(map[string]any{
			"dry_run": true,
			"request": map[string]any{
				"server": cfg.ServerID,
				"name":   cfg.ToolName,
				"args":   args,
			},
			"paging": map[string]any{
				"pageAll":   true,
				"pageLimit": opts.pageLimit,
				"maxItems":  opts.maxItems,
				"pageDelay": opts.delayMS,
			},
		})
	}
	return runPagedMCPCommand(cmd, cfg, opts, args)
}

func readPagedCommandOptions(cmd *cobra.Command) (pagedCommandOptions, error) {
	pageAll, _ := cmd.Flags().GetBool("page-all")
	opts := pagedCommandOptions{pageAll: pageAll}
	if !pageAll {
		return opts, nil
	}
	opts.pageLimit, _ = cmd.Flags().GetInt("page-limit")
	if opts.pageLimit < 1 || opts.pageLimit > maxPagedCommandPageLimit {
		return opts, fmt.Errorf("--page-limit must be between 1 and 500")
	}
	opts.maxItems, _ = cmd.Flags().GetInt("max-items")
	if opts.maxItems < 0 {
		return opts, fmt.Errorf("--max-items must be greater than or equal to 0")
	}
	opts.delayMS, _ = cmd.Flags().GetInt("page-delay")
	if opts.delayMS < 0 {
		return opts, fmt.Errorf("--page-delay must be greater than or equal to 0")
	}
	return opts, nil
}

func validatePagedConfig(cfg PagedMCPCommandConfig) error {
	switch {
	case strings.TrimSpace(cfg.ServerID) == "":
		return fmt.Errorf("paged command server is required")
	case strings.TrimSpace(cfg.ToolName) == "":
		return fmt.Errorf("paged command tool is required")
	case strings.TrimSpace(cfg.ItemPath) == "":
		return fmt.Errorf("paged command item path is required")
	case strings.TrimSpace(cfg.CursorPath) == "":
		return fmt.Errorf("paged command cursor path is required")
	case strings.TrimSpace(cfg.HasMorePath) == "":
		return fmt.Errorf("paged command hasMore path is required")
	case strings.TrimSpace(cfg.CursorArg) == "":
		return fmt.Errorf("paged command cursor arg is required")
	case cfg.BuildArgs == nil || cfg.Fallback == nil:
		return fmt.Errorf("paged command callbacks are required")
	default:
		return nil
	}
}

func runPagedMCPCommand(cmd *cobra.Command, cfg PagedMCPCommandConfig, opts pagedCommandOptions, args map[string]any) error {
	var envelope map[string]any
	ctx := cmd.Context()
	items := newPagedCollection(cfg)
	seenCursors := map[string]bool{}
	currentCursor := cursorValueKey(args[cfg.CursorArg], cfg.CursorKind)
	lastCursor := args[cfg.CursorArg]
	hasMore := true

	for page := 1; page <= opts.pageLimit && hasMore; page++ {
		pageCursor := args[cfg.CursorArg]
		seenCursors[currentCursor] = true
		text, err := callMCPToolReturnTextOnServer(ctx, cfg.ServerID, cfg.ToolName, args)
		if err != nil {
			return handlePagedCommandError(cmd, envelope, cfg, items, page, currentCursor, err)
		}
		parsed, pageItems, nextCursor, more, err := parsePagedCommandPage(text, cfg)
		if err != nil {
			return handlePagedCommandError(cmd, envelope, cfg, items, page, currentCursor, err)
		}
		if envelope == nil {
			envelope = parsed
		}
		if err := items.Add(pageItems); err != nil {
			return handlePagedCommandError(cmd, envelope, cfg, items, page, currentCursor, err)
		}
		hasMore = more

		if opts.maxItems > 0 && items.Total() > opts.maxItems {
			items.Truncate(opts.maxItems)
			return writePagedCommandResult(envelope, cfg, items, pagingMetadata{
				Truncated:           true,
				HasMore:             true,
				LastCursor:          pageCursor,
				Pages:               page,
				Total:               items.Total(),
				TruncatedWithinPage: true,
			})
		}
		lastCursor = nextCursor
		if opts.maxItems > 0 && items.Total() == opts.maxItems && hasMore {
			return writePagedCommandResult(envelope, cfg, items, pagingMetadata{
				Truncated:  true,
				HasMore:    true,
				LastCursor: lastCursor,
				Pages:      page,
				Total:      items.Total(),
			})
		}
		if !hasMore {
			return writePagedCommandResult(envelope, cfg, items, pagingMetadata{
				Truncated:  false,
				HasMore:    false,
				LastCursor: lastCursor,
				Pages:      page,
				Total:      items.Total(),
			})
		}
		nextKey := cursorValueKey(nextCursor, cfg.CursorKind)
		if nextKey == "" || nextKey == currentCursor || seenCursors[nextKey] {
			err := fmt.Errorf("pagination cursor did not advance: %s", nextKey)
			return handlePagedCommandError(cmd, envelope, cfg, items, page+1, nextKey, err)
		}
		normalizedCursor, err := normalizeCursorArg(nextCursor, cfg.CursorKind)
		if err != nil {
			return handlePagedCommandError(cmd, envelope, cfg, items, page+1, nextKey, err)
		}
		currentCursor = nextKey
		args[cfg.CursorArg] = normalizedCursor
		if opts.delayMS > 0 {
			if err := sleepPagedCommandDelay(ctx, time.Duration(opts.delayMS)*time.Millisecond); err != nil {
				return handlePagedCommandError(cmd, envelope, cfg, items, page+1, currentCursor, err)
			}
		}
	}

	return writePagedCommandResult(envelope, cfg, items, pagingMetadata{
		Truncated:  hasMore,
		HasMore:    hasMore,
		LastCursor: lastCursor,
		Pages:      opts.pageLimit,
		Total:      items.Total(),
	})
}

func parsePagedCommandPage(text string, cfg PagedMCPCommandConfig) (map[string]any, []any, any, bool, error) {
	var parsed map[string]any
	if err := json.Unmarshal([]byte(text), &parsed); err != nil {
		return nil, nil, nil, false, fmt.Errorf("parse paged response JSON: %w", err)
	}
	rawHasMore, ok := getJSONPath(parsed, cfg.HasMorePath)
	if !ok {
		return nil, nil, nil, false, fmt.Errorf("paged response missing %s", cfg.HasMorePath)
	}
	hasMore, ok := rawHasMore.(bool)
	if !ok {
		return nil, nil, nil, false, fmt.Errorf("paged response %s must be boolean", cfg.HasMorePath)
	}
	rawItems, ok := getJSONPath(parsed, cfg.ItemPath)
	if !ok && cfg.AggregationMode == PagedAggregationConversationMessages && !hasMore {
		rawItems = []any{}
		ok = true
	}
	if !ok {
		return nil, nil, nil, false, fmt.Errorf("paged response missing %s", cfg.ItemPath)
	}
	items, ok := rawItems.([]any)
	if !ok {
		return nil, nil, nil, false, fmt.Errorf("paged response %s must be array", cfg.ItemPath)
	}
	nextCursor, ok := getJSONPath(parsed, cfg.CursorPath)
	if hasMore && !ok {
		return nil, nil, nil, false, fmt.Errorf("paged response missing %s", cfg.CursorPath)
	}
	return parsed, items, nextCursor, hasMore, nil
}

type pagingMetadata struct {
	Truncated           bool
	HasMore             bool
	LastCursor          any
	Pages               int
	Total               int
	TruncatedWithinPage bool
	Partial             bool
	FailedPage          int
	FailedCursor        string
	PagesFetched        int
	ItemsFetched        int
	Error               string
}

func handlePagedCommandError(cmd *cobra.Command, envelope map[string]any, cfg PagedMCPCommandConfig, items *pagedCollection, failedPage int, failedCursor string, err error) error {
	if envelope == nil {
		return err
	}
	fmt.Fprintf(cmd.ErrOrStderr(), "pagination stopped at page %d: %v\n", failedPage, err)
	if outputErr := writePagedCommandResult(envelope, cfg, items, pagingMetadata{
		Truncated:    true,
		HasMore:      true,
		LastCursor:   failedCursor,
		Pages:        failedPage - 1,
		Total:        items.Total(),
		Partial:      true,
		FailedPage:   failedPage,
		FailedCursor: failedCursor,
		PagesFetched: failedPage - 1,
		ItemsFetched: items.Total(),
		Error:        err.Error(),
	}); outputErr != nil {
		return errors.Join(err, outputErr)
	}
	return err
}

func writePagedCommandResult(envelope map[string]any, cfg PagedMCPCommandConfig, items *pagedCollection, meta pagingMetadata) error {
	_ = setJSONPath(envelope, cfg.ItemPath, items.Values())
	_ = setJSONPath(envelope, cfg.HasMorePath, meta.HasMore)
	_ = setJSONPath(envelope, cfg.CursorPath, meta.LastCursor)
	paging := map[string]any{
		"truncated":  meta.Truncated,
		"hasMore":    meta.HasMore,
		"lastCursor": meta.LastCursor,
		"pages":      meta.Pages,
		"total":      meta.Total,
	}
	if meta.Partial {
		paging["partial"] = true
		paging["failedPage"] = meta.FailedPage
		paging["failedCursor"] = meta.FailedCursor
		paging["pagesFetched"] = meta.PagesFetched
		paging["itemsFetched"] = meta.ItemsFetched
		paging["error"] = meta.Error
	}
	if meta.TruncatedWithinPage {
		paging["truncatedWithinPage"] = true
		paging["resumeCursorReliable"] = false
	}
	envelope["paging"] = paging
	if cfg.ProjectResult != nil {
		envelope = cfg.ProjectResult(envelope)
	}
	return deps.Out.PrintJSON(envelope)
}

func sleepPagedCommandDelay(ctx context.Context, delay time.Duration) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-helperAfter(delay):
		return nil
	}
}

type pagedCollection struct {
	mode              PagedAggregationMode
	items             []any
	conversationIndex map[string]int
	total             int
}

func newPagedCollection(cfg PagedMCPCommandConfig) *pagedCollection {
	return &pagedCollection{
		mode:              cfg.AggregationMode,
		conversationIndex: map[string]int{},
	}
}

func (c *pagedCollection) Add(items []any) error {
	if c.mode != PagedAggregationConversationMessages {
		c.items = append(c.items, items...)
		c.total = len(c.items)
		return nil
	}
	for _, item := range items {
		if err := c.addConversation(item); err != nil {
			return err
		}
	}
	return nil
}

func (c *pagedCollection) Values() []any {
	if c.items == nil {
		return []any{}
	}
	return c.items
}

func (c *pagedCollection) Total() int {
	return c.total
}

func (c *pagedCollection) Truncate(maxItems int) bool {
	if maxItems <= 0 || c.total <= maxItems {
		return false
	}
	if c.mode != PagedAggregationConversationMessages {
		c.items = c.items[:maxItems]
		c.total = len(c.items)
		return true
	}
	c.truncateConversationMessages(maxItems)
	return true
}

func (c *pagedCollection) addConversation(item any) error {
	conversation, ok := item.(map[string]any)
	if !ok {
		return fmt.Errorf("paged response conversation item must be object")
	}
	key, _ := conversation["openConversationId"].(string)
	if strings.TrimSpace(key) == "" {
		return fmt.Errorf("paged response conversation item missing openConversationId")
	}
	messages, err := conversationMessages(conversation)
	if err != nil {
		return err
	}
	if idx, ok := c.conversationIndex[key]; ok {
		existing := c.items[idx].(map[string]any)
		existingMessages, err := conversationMessages(existing)
		if err != nil {
			return err
		}
		existing["messages"] = append(existingMessages, messages...)
		c.total += len(messages)
		return nil
	}
	c.conversationIndex[key] = len(c.items)
	c.items = append(c.items, conversation)
	c.total += len(messages)
	return nil
}

func (c *pagedCollection) truncateConversationMessages(maxItems int) {
	remaining := maxItems
	for i, item := range c.items {
		conversation := item.(map[string]any)
		messages, _ := conversationMessages(conversation)
		if remaining >= len(messages) {
			remaining -= len(messages)
			continue
		}
		if remaining == 0 {
			c.items = c.items[:i]
			c.total = maxItems
			return
		}
		conversation["messages"] = messages[:remaining]
		c.items = c.items[:i+1]
		c.total = maxItems
		return
	}
}

func conversationMessages(conversation map[string]any) ([]any, error) {
	raw, ok := conversation["messages"]
	if !ok {
		return []any{}, nil
	}
	messages, ok := raw.([]any)
	if !ok {
		return nil, fmt.Errorf("paged response conversation messages must be array")
	}
	return messages, nil
}

func getJSONPath(root map[string]any, path string) (any, bool) {
	var current any = root
	for _, part := range strings.Split(path, ".") {
		obj, ok := current.(map[string]any)
		if !ok {
			return nil, false
		}
		current, ok = obj[part]
		if !ok {
			return nil, false
		}
	}
	return current, true
}

func setJSONPath(root map[string]any, path string, value any) bool {
	parts := strings.Split(path, ".")
	current := root
	for _, part := range parts[:len(parts)-1] {
		next, ok := current[part].(map[string]any)
		if !ok {
			return false
		}
		current = next
	}
	current[parts[len(parts)-1]] = value
	return true
}

func cursorValueKey(value any, kind PagedCursorKind) string {
	switch kind {
	case PagedCursorInt64:
		switch v := value.(type) {
		case int64:
			return strconv.FormatInt(v, 10)
		case int:
			return strconv.Itoa(v)
		case float64:
			return strconv.FormatInt(int64(v), 10)
		case string:
			return strings.TrimSpace(v)
		default:
			return ""
		}
	default:
		if value == nil {
			return ""
		}
		return strings.TrimSpace(fmt.Sprint(value))
	}
}

func normalizeCursorArg(value any, kind PagedCursorKind) (any, error) {
	if kind != PagedCursorInt64 {
		if value == nil {
			return "", nil
		}
		return fmt.Sprint(value), nil
	}
	switch v := value.(type) {
	case int64:
		return v, nil
	case int:
		return int64(v), nil
	case float64:
		converted := int64(v)
		if float64(converted) != v {
			return nil, fmt.Errorf("paged response cursor must be an integer, got %v", v)
		}
		return converted, nil
	case string:
		parsed, err := strconv.ParseInt(strings.TrimSpace(v), 10, 64)
		if err == nil {
			return parsed, nil
		}
		return nil, fmt.Errorf("paged response cursor must be a base-10 int64 string, got %q", v)
	}
	return nil, fmt.Errorf("paged response cursor must be int64-compatible, got %T", value)
}
