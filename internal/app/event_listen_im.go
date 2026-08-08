// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.

package app

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/cli"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/event/personal"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/helpers"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut/targetresolver"
	"github.com/spf13/cobra"
)

type listenIMOptions struct {
	Kind             string
	Events           []string
	UserID           string
	OpenDingTalkID   string
	UserQuery        string
	ChatID           string
	ChatQuery        string
	QueryCSV         string
	MaxEvents        int
	Duration         time.Duration
	DryRun           bool
	ControlBaseURL   string
	StreamTicketMode string
	StreamTicketURL  string
	StreamSourceID   string
}

type listenIMPlan struct {
	EventKeys       []string
	UserID          string
	OpenDingTalkID  string
	GroupID         string
	ResolvedTargets []any
}

type eventTargetReader struct{}

func (eventTargetReader) CallMCPData(product, tool string, params map[string]any) (map[string]any, error) {
	text, err := helpers.CallMCPReadToolTextOnServer(product, tool, params)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(text) == "" {
		return map[string]any{}, nil
	}
	var data map[string]any
	if err := json.Unmarshal([]byte(text), &data); err != nil {
		return nil, apperrors.NewInternal(fmt.Sprintf("解析 %s 返回失败: %v", tool, err))
	}
	return data, nil
}

var eventListenIMReader = func() targetresolver.Reader { return eventTargetReader{} }

func newEventListenIMCommand() *cobra.Command {
	var opts listenIMOptions
	cmd := &cobra.Command{
		Use:   "+listen-im",
		Short: "按 IM 意图解析目标并监听一个或多个个人消息事件",
		Long: "把 @我、指定发送人、指定群、全部单聊或全部群聊等用户意图确定性编译为个人 EventKey，" +
			"自然姓名/群名会先唯一解析，再复用 event consume 的订阅、ready marker、NDJSON、取消、回滚和清理生命周期；本命令只处理 IM，不接收 OA 审批事件。",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(c *cobra.Command, _ []string) error {
			plan, err := compileListenIMPlan(eventListenIMReader(), opts)
			if err != nil {
				return fmt.Errorf("event +listen-im: %w", err)
			}
			consumeOpts := personalConsumeOptions{
				EventKey:         firstArg(plan.EventKeys),
				EventKeys:        plan.EventKeys,
				Flatten:          true,
				UserID:           plan.UserID,
				OpenDingTalkID:   plan.OpenDingTalkID,
				GroupID:          plan.GroupID,
				QueryCSV:         opts.QueryCSV,
				ControlBaseURL:   opts.ControlBaseURL,
				StreamTicketMode: opts.StreamTicketMode,
				StreamTicketURL:  opts.StreamTicketURL,
				StreamSourceID:   opts.StreamSourceID,
				Common: commonConsumeOptions{
					FormatRaw: "ndjson",
					MaxEvents: opts.MaxEvents,
					Duration:  opts.Duration,
					DryRun:    opts.DryRun,
				},
			}
			return eventRunPersonalConsume(c, consumeOpts)
		},
	}
	f := cmd.Flags()
	f.StringVar(&opts.Kind, "kind", "at-me", "监听意图: at-me|sender|group|all-direct|all-group")
	f.StringSliceVar(&opts.Events, "events", []string{"message"}, "事件种类: message,reaction,read,recall")
	f.StringVar(&opts.UserID, "user", "", "指定发送人/单聊对端 userId")
	f.StringVar(&opts.OpenDingTalkID, "open-dingtalk-id", "", "指定发送人/单聊对端 openDingTalkId")
	f.StringVar(&opts.UserQuery, "user-query", "", "按姓名/花名唯一解析指定发送人")
	f.StringVar(&opts.ChatID, "chat-id", "", "指定群 openConversationId")
	f.StringVar(&opts.ChatQuery, "chat-query", "", "按群名唯一解析指定群")
	f.StringVar(&opts.QueryCSV, "query", "", "消息文本关键词过滤，逗号分隔；仅 message 事件")
	f.IntVar(&opts.MaxEvents, "max-events", 0, "收到 N 条后退出 (0 = 不限)")
	f.DurationVar(&opts.Duration, "duration", 0, "运行时长上限 (Go duration，如 30s/5m；0 = 不限)")
	f.BoolVar(&opts.DryRun, "dry-run", false, "解析目标并打印订阅计划，不创建订阅或连接 bus")
	f.StringVar(&opts.ControlBaseURL, "personal-event-base-url", "", "个人事件控制面 base URL；默认由 MCP base 派生 /dws")
	f.StringVar(&opts.StreamTicketMode, "stream-ticket-mode", strings.TrimSpace(os.Getenv("DWS_STREAM_TICKET_MODE")), "个人 Stream 建联模式；默认 normal")
	f.StringVar(&opts.StreamSourceID, "stream-source-id", strings.TrimSpace(os.Getenv("DWS_STREAM_SOURCE_ID")), "个人 Stream sourceId；开源版默认 open")
	f.StringVar(&opts.StreamTicketURL, "stream-ticket-url", strings.TrimSpace(os.Getenv("DWS_STREAM_TICKET_URL")), "个人 Stream 取票 URL")
	hideEventInternalFlags(cmd, "personal-event-base-url", "stream-ticket-mode", "stream-source-id", "stream-ticket-url")
	cli.AnnotateRuntimeFlagEnum(cmd, "kind", "at-me", "sender", "group", "all-direct", "all-group")
	cli.AnnotateRuntimeFlagEnum(cmd, "events", "message", "reaction", "read", "recall")
	cli.AnnotateRuntimeConstraints(cmd, cli.RuntimeSchemaConstraints{
		MutuallyExclusive: [][]string{{"user", "open-dingtalk-id", "user-query", "chat-id", "chat-query"}},
	})
	helpers.DeclareLeafMetadata(cmd, helpers.LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "write", Risk: "medium",
			Confirmation: "not_required", Idempotency: "non_idempotent",
		},
		Contract: helpers.LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "event",
				Name:           "listen_im",
				CanonicalPath:  "event.listen_im",
				CLIPath:        "event +listen-im",
				PrimaryCLIPath: "event +listen-im",
			},
			Description: "把 @我、指定发送人、指定群、全部单聊或全部群聊等用户意图确定性编译为个人 EventKey，自然姓名/群名会先唯一解析，再复用 event consume 的订阅、ready marker、NDJSON、取消、回滚和清理生命周期。",
			Interface: &contract.InterfaceSpec{
				Mode:         "composite",
				Availability: "available",
				Reason:       "Reviewed IM event facade: it deterministically maps kind/events to public personal EventKeys, resolves one natural user/chat target with the shared typed resolver, then delegates one single- or multi-event invocation to the existing subscription, bus, ready-marker, NDJSON, rollback, cancellation, and cleanup lifecycle.",
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "按 @我、发送人、群或全量范围监听普通 IM message/reaction/read/recall 事件",
				UseWhen: []string{
					"已知要监听 @我、指定发送人、指定群、全部单聊或全部群聊的 message/reaction/read/recall 事件时使用；姓名用 --user-query、群名用 --chat-query，CLI 会唯一解析目标并把多个兼容事件合并到一个消费生命周期。",
				},
				AvoidWhen: []string{
					"OA 审批事件、群标题/成员/解散等生命周期事件、显式 EventKey、复用 subscribe_id、Filter DSL、原始 transport envelope 或其它底层控制使用 event consume；只查历史消息使用 chat 查询入口",
				},
				Examples: []string{
					"dws event +listen-im --kind at-me --max-events 1",
					"dws event +listen-im --kind group --events message,reaction --chat-id <openConversationId> --duration 10m",
				},
			},
			Parameters: []contract.ParamDecl{
				{Name: "chat-id", Property: "chatId"},
				{Name: "chat-query", Property: "chatQuery"},
				{Name: "dry-run", Property: "dryRun"},
				{Name: "duration", Property: "duration"},
				{Name: "events", Property: "events"},
				{Name: "kind", Property: "kind"},
				{Name: "max-events", Property: "maxEvents"},
				{Name: "open-dingtalk-id", Property: "openDingtalkId"},
				{Name: "query", Property: "query"},
				{Name: "user", Property: "user"},
				{Name: "user-query", Property: "userQuery"},
			},
		},
	})
	return cmd
}

func compileListenIMPlan(reader targetresolver.Reader, opts listenIMOptions) (listenIMPlan, error) {
	kind := strings.ToLower(strings.TrimSpace(opts.Kind))
	if kind == "" {
		kind = "at-me"
	}
	events := uniqueListenIMValues(opts.Events)
	if len(events) == 0 {
		return listenIMPlan{}, apperrors.NewValidation("--events 至少包含一个事件种类")
	}
	if strings.TrimSpace(opts.QueryCSV) != "" {
		for _, eventName := range events {
			if eventName != "message" {
				return listenIMPlan{}, apperrors.NewValidation("--query 只支持 message 事件")
			}
		}
	}

	plan := listenIMPlan{}
	var err error
	switch kind {
	case "at-me", "all-direct", "all-group":
		if listenIMTargetCount(opts) != 0 {
			return listenIMPlan{}, apperrors.NewValidation(fmt.Sprintf("--kind %s 不接受用户或群目标", kind))
		}
	case "sender":
		if listenIMUserTargetCount(opts) != 1 || listenIMChatTargetCount(opts) != 0 {
			return listenIMPlan{}, apperrors.NewValidation("--kind sender 必须且只能指定 --user、--open-dingtalk-id 或 --user-query 之一")
		}
		plan.UserID = strings.TrimSpace(opts.UserID)
		plan.OpenDingTalkID = strings.TrimSpace(opts.OpenDingTalkID)
		if query := strings.TrimSpace(opts.UserQuery); query != "" {
			resolved, resolveErr := targetresolver.ResolveUser(reader, query, targetresolver.IdentityAny)
			if resolveErr != nil {
				return listenIMPlan{}, resolveErr
			}
			plan.ResolvedTargets = append(plan.ResolvedTargets, resolved)
			plan.UserID = resolved.Selected.UserID
			if plan.UserID == "" {
				plan.OpenDingTalkID = resolved.Selected.OpenDingTalkID
			}
		}
	case "group":
		if listenIMChatTargetCount(opts) != 1 || listenIMUserTargetCount(opts) != 0 {
			return listenIMPlan{}, apperrors.NewValidation("--kind group 必须且只能指定 --chat-id 或 --chat-query 之一")
		}
		plan.GroupID = strings.TrimSpace(opts.ChatID)
		if query := strings.TrimSpace(opts.ChatQuery); query != "" {
			resolved, resolveErr := targetresolver.ResolveChat(reader, query)
			if resolveErr != nil {
				return listenIMPlan{}, resolveErr
			}
			plan.ResolvedTargets = append(plan.ResolvedTargets, resolved)
			plan.GroupID = resolved.Selected.OpenConversationID
		}
	default:
		return listenIMPlan{}, apperrors.NewValidation("--kind 必须是 at-me、sender、group、all-direct 或 all-group")
	}

	plan.EventKeys, err = listenIMEventKeys(kind, events)
	if err != nil {
		return listenIMPlan{}, err
	}
	return plan, nil
}

func listenIMEventKeys(kind string, events []string) ([]string, error) {
	mapping := map[string]map[string]string{
		"at-me":      {"message": personal.EventMention},
		"sender":     {"message": personal.EventFromUser, "reaction": personal.EventReactionO2O, "read": personal.EventReadO2O, "recall": personal.EventRecallO2O},
		"group":      {"message": personal.EventInChat, "reaction": personal.EventReactionGroup, "read": personal.EventReadGroup, "recall": personal.EventRecallGroup},
		"all-direct": {"message": personal.EventAllSingleChat},
		"all-group":  {"message": personal.EventAllGroupChat},
	}
	byEvent := mapping[kind]
	keys := make([]string, 0, len(events))
	for _, eventName := range events {
		key := byEvent[eventName]
		if key == "" {
			return nil, apperrors.NewValidation(fmt.Sprintf("--kind %s 不支持 event %s", kind, eventName))
		}
		keys = append(keys, key)
	}
	return keys, nil
}

func listenIMUserTargetCount(opts listenIMOptions) int {
	return nonEmptyListenIMCount(opts.UserID, opts.OpenDingTalkID, opts.UserQuery)
}

func listenIMChatTargetCount(opts listenIMOptions) int {
	return nonEmptyListenIMCount(opts.ChatID, opts.ChatQuery)
}

func listenIMTargetCount(opts listenIMOptions) int {
	return listenIMUserTargetCount(opts) + listenIMChatTargetCount(opts)
}

func nonEmptyListenIMCount(values ...string) int {
	count := 0
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			count++
		}
	}
	return count
}

func uniqueListenIMValues(values []string) []string {
	out := make([]string, 0, len(values))
	seen := map[string]bool{}
	for _, value := range values {
		value = strings.ToLower(strings.TrimSpace(value))
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	return out
}
