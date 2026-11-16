// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.

package app

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/apiclient"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/helpers"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/config"
	"github.com/spf13/cobra"
)

const eventSendPayloadLimit = 32 * 1024

var (
	eventSendPush  = pushAppEvent
	eventSendURL   = appEventPushURL
	eventSendToken = func(ctx context.Context, clientID, clientSecret string) (string, error) {
		return resolveRawAPIToken(ctx, "", clientID, clientSecret)
	}
)

func newEventSendCommand(globalFlags ...*GlobalFlags) *cobra.Command {
	flags := &GlobalFlags{Timeout: 30}
	if len(globalFlags) > 0 && globalFlags[0] != nil {
		flags = globalFlags[0]
	}
	return helpers.NewLeafCommand(helpers.LeafSpec{
		Use:   "send",
		Short: "用企业自建应用身份上报三方事件",
		Long: "用企业自建应用 Token 调用现有事件上报接口，向指定企业内 userId 发送该应用的三方事件。" +
			"调用方提供已发布的 eventKey 和 payloadSchemaVersion，服务端校验事件归属、版本、接收人和业务 Schema。\n\n" +
			"应用凭证依次使用完整的应用凭证参数、成对环境变量、本机应用配置；具体参数见全局帮助和发送参考。" +
			"本命令仅使用应用凭证，无需用户登录或 Schema 预查。\n\n" +
			"--event-id 是本次发送的事件 ID。重发同一条事件时沿用原 ID 和原请求内容，发送新事件时使用新 ID。" +
			"每次调用只上报一次。ACCEPTED 表示本次接纳，DUPLICATE 表示同一事件此前已接纳；实际收件另行核对。\n\n" +
			"--dry-run 预览请求，内联 JSON 会做语法检查，@file 和 stdin 只展示来源，预览不读取文件、标准输入或凭证，不访问网络。",
		Example: "  dws event send --event-key app_user_order_created --event-id order-1001-created --payload-schema-version 3 --user-id user-1 --payload '{\"orderId\":\"order-1001\"}'\n" +
			"  dws event send --event-key app_user_order_created --event-id order-1002-created --payload-schema-version 3 --user-id user-1 --user-id user-2 --payload @payload.json --dry-run",
		OutputRollout: output.RolloutUnifiedActive,
		Flags: []helpers.LeafFlag{
			{Name: "event-key", Bind: "eventKey", Usage: "应用下已发布的三方事件键", Required: true, MarkRequired: true, Trim: true},
			{Name: "event-id", Bind: "eventId", Usage: "本次发送的事件 ID，最长 64 字符；重发沿用原 ID，新事件使用新 ID", Required: true, MarkRequired: true, Trim: true},
			{Name: "payload-schema-version", Bind: "payloadSchemaVersion", Usage: "payload 对应的已发布业务 Schema 版本", Required: true, MarkRequired: true, Trim: true},
			{Name: "user-id", Bind: "userIds", Kind: helpers.LeafStringSlice, Usage: "接收人的企业内 userId，可重复传入，去重后最多 100 个", Required: true, MarkRequired: true},
			{Name: "payload", Bind: "payload", Usage: "业务 JSON 对象，支持 @file 或 - 从 stdin 读取，最大 32KB", Required: true, MarkRequired: true, Trim: true},
			{Name: "occurred-at", Bind: "occurredAt", Usage: "可选事件发生时间，ISO 8601 格式且包含时区", Trim: true, OmitEmpty: true},
		},
		Safety:   contract.SafetySpec{Effect: "write", Risk: "medium", Confirmation: "not_required", Idempotency: "idempotent"},
		Validate: validateEventSend,
		Contract: helpers.LeafContract{
			Identity:    contract.ToolIdentitySpec{ProductID: "event", Name: "send", CanonicalPath: "event.send", CLIPath: "event send", PrimaryCLIPath: "event send"},
			Description: "使用企业自建应用凭证上报三方事件，返回服务端接纳或重复事件回执",
			Interface: &contract.InterfaceSpec{
				Mode: contract.InterfaceModeComposite, Availability: contract.InterfaceAvailable,
				Reason: "Application credentials are resolved and exchanged for an app token before one POST /v1.0/event/user/push request. The CLI supports file/stdin payload input and projects the validated admission receipt.",
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "用企业自建应用 Token 向指定企业用户发送应用三方事件",
				UseWhen:      []string{"已知企业自建应用凭证、应用事件及业务版本、事件 ID、接收人 userId 和 payload，需要通过 CLI 发送应用三方事件"},
				AvoidWhen:    []string{"监听未来事件用 event consume；给用户或群发送聊天消息用 chat"},
				Examples:     []string{"dws event send --event-key app_user_order_created --event-id order-1001-created --payload-schema-version 3 --user-id user-1 --payload '{\"orderId\":\"order-1001\"}' --dry-run"},
			},
			Parameters: []contract.ParamDecl{
				{Name: "event-key", Property: "eventKey"},
				{Name: "event-id", Property: "eventId"},
				{Name: "payload-schema-version", Property: "payloadSchemaVersion"},
				{Name: "user-id", Property: "userIds"},
				{Name: "payload", Property: "payload"},
				{Name: "occurred-at", Property: "occurredAt"},
			},
			Result: eventSendResultSpec(),
			DryRun: &contract.DryRunSpec{PreviewKind: "request"},
		},
		ResultCall: func(cmd *cobra.Command, _ string, args map[string]any) (output.CommandResult, error) {
			return runEventSend(cmd, flags, args)
		},
	})
}

func validateEventSend(cmd *cobra.Command, _ []string) error {
	for _, field := range []struct {
		name string
		max  int
	}{
		{"event-key", 128}, {"event-id", 64}, {"payload-schema-version", 64},
	} {
		value, _ := cmd.Flags().GetString(field.name)
		if len([]rune(strings.TrimSpace(value))) > field.max {
			return apperrors.NewValidation(fmt.Sprintf("--%s 最长 %d 字符", field.name, field.max))
		}
	}
	users, _ := cmd.Flags().GetStringSlice("user-id")
	users = normalizeEventSendUserIDs(users)
	if len(users) == 0 || len(users) > 100 {
		return apperrors.NewValidation("--user-id 去重后需要 1-100 个企业内 userId")
	}
	for _, userID := range users {
		if len([]rune(userID)) > 64 {
			return apperrors.NewValidation("--user-id 单个值最长 64 字符")
		}
	}
	occurredAt, _ := cmd.Flags().GetString("occurred-at")
	if strings.TrimSpace(occurredAt) != "" {
		when, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(occurredAt))
		if err != nil || when.UnixMilli() <= 0 {
			return apperrors.NewValidation("--occurred-at 需要带时区的 ISO 8601 时间且晚于 Unix 纪元")
		}
	}
	return nil
}

func runEventSend(cmd *cobra.Command, flags *GlobalFlags, args map[string]any) (output.CommandResult, error) {
	req := appEventPushRequest{
		EventKey: args["eventKey"].(string), EventID: args["eventId"].(string),
		PayloadSchemaVersion: args["payloadSchemaVersion"].(string),
		UserIDs:              normalizeEventSendUserIDs(args["userIds"].([]string)),
	}
	req.OccurredAt, _ = args["occurredAt"].(string)
	payloadSource := args["payload"].(string)
	endpoint, err := eventSendURL(defaultConfigDir())
	if err != nil {
		return nil, apperrors.NewValidation(err.Error())
	}
	if flags.DryRun {
		preview := map[string]any{"method": "POST", "url": endpoint, "request": req}
		if apiclient.IsDeferredInput(payloadSource) {
			preview["payloadSource"] = payloadSource
		} else {
			req.Payload, err = readEventSendPayload(payloadSource, nil)
			if err != nil {
				return nil, err
			}
			preview["request"] = req
		}
		return output.Success(preview, output.WithDryRun()), nil
	}
	req.Payload, err = readEventSendPayload(payloadSource, cmd.InOrStdin())
	if err != nil {
		return nil, err
	}
	timeout := 30 * time.Second
	if flags.Timeout > 0 {
		timeout = time.Duration(flags.Timeout) * time.Second
	}
	ctx, cancel := context.WithTimeout(cmd.Context(), timeout)
	defer cancel()
	token, err := eventSendToken(ctx, flags.ClientID, flags.ClientSecret)
	if err != nil {
		return nil, err
	}
	result, err := eventSendPush(ctx, endpoint, token, req)
	if err != nil {
		return nil, err
	}
	return output.Success(result), nil
}

func normalizeEventSendUserIDs(values []string) []string {
	users := make([]string, 0, len(values))
	seen := make(map[string]bool, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		users = append(users, value)
	}
	return users
}

func readEventSendPayload(source string, stdin io.Reader) (json.RawMessage, error) {
	reader := io.Reader(strings.NewReader(source))
	if source == "-" {
		if stdin == nil {
			return nil, apperrors.NewValidation("--payload 指定 stdin，但标准输入不可用")
		}
		reader = stdin
	} else if strings.HasPrefix(source, "@") {
		path := strings.TrimSpace(strings.TrimPrefix(source, "@"))
		if path == "" {
			return nil, apperrors.NewValidation("--payload 的 @file 路径不能为空")
		}
		file, err := os.Open(path)
		if err != nil {
			return nil, apperrors.NewValidation("无法读取 --payload 文件", apperrors.WithReason("payload_file_unreadable"))
		}
		defer file.Close()
		reader = file
	}
	data, err := io.ReadAll(io.LimitReader(reader, config.MaxResponseBodySize+1))
	if err != nil {
		return nil, apperrors.NewValidation("无法读取 --payload 内容")
	}
	if len(data) > config.MaxResponseBodySize {
		return nil, apperrors.NewValidation("--payload 输入超过 10MB 读取上限")
	}
	data = bytes.TrimSpace(data)
	var compact bytes.Buffer
	if len(data) == 0 || data[0] != '{' || json.Compact(&compact, data) != nil {
		return nil, apperrors.NewValidation("--payload 必须是有效的 JSON 对象")
	}
	if compact.Len() > eventSendPayloadLimit {
		return nil, apperrors.NewValidation("--payload 超过 32KB 上限")
	}
	return json.RawMessage(compact.Bytes()), nil
}

func eventSendResultSpec() *contract.ResultSpec {
	return &contract.ResultSpec{
		Outcomes: []contract.ResultOutcome{contract.ResultOutcomeSuccess, contract.ResultOutcomeFailure},
		DataSchema: json.RawMessage(`{
		"oneOf":[{
			"type":"object","description":"事件上报接纳回执",
			"properties":{
				"eventId":{"type":"string","description":"本次上报的事件 ID"},
				"status":{"type":"string","enum":["ACCEPTED","DUPLICATE"],"description":"首次接纳或此前已接纳的重复事件"},
				"duplicate":{"type":"boolean","description":"服务端是否识别为同键同内容的重复事件"}
			},
			"required":["eventId","status","duplicate"],"additionalProperties":false
		},{
			"type":"object","description":"dry-run 请求预览",
			"properties":{
				"method":{"type":"string","const":"POST","description":"HTTP 方法"},
				"url":{"type":"string","description":"当前环境的事件上报地址"},
				"request":{"type":"object","description":"上报字段，文件或 stdin payload 在预览阶段延迟读取","additionalProperties":true},
				"payloadSource":{"type":"string","description":"延迟读取的 @file 或 stdin 来源"}
			},
			"required":["method","url","request"],"additionalProperties":false
		}]
		}`),
	}
}
