// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0.
package app

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"time"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/executor"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/helpers"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/publishedmcp"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut/usage"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/transport"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
)

var _ helpers.AicardTemplateCaller = (*toolCallerAdapter)(nil)
var _ helpers.AicardTemplateCaller = recordingToolCaller{}

// InvokeAicardTemplate uses the same invocation identity and transport as the
// published-MCP command without recursively executing the CLI or mutating deps.
func (a *toolCallerAdapter) InvokeAicardTemplate(ctx context.Context, mcpID, tool string, args map[string]any) (publishedmcp.ValidatedInvocationResult, error) {
	if a == nil || a.DryRun() {
		return publishedmcp.ValidatedInvocationResult{}, apperrors.NewValidation("Published template invocation is unavailable during dry-run")
	}
	if a.flags != nil {
		if a.flags.Timeout <= 0 || int64(a.flags.Timeout) > math.MaxInt64/int64(time.Second) {
			return publishedmcp.ValidatedInvocationResult{}, apperrors.NewValidation("--timeout must be a positive number of seconds within the supported range", apperrors.WithReason("invalid_timeout"))
		}
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, time.Duration(a.flags.Timeout)*time.Second)
		defer cancel()
	}
	return invokeAicardTemplate(ctx, a, newAicardTemplateTransportFactory(a.runner, a.flags), mcpID, tool, args)
}

// newAicardTemplateTransportFactory matches the published-MCP factory but logs
// through aicardTemplateTransport, so template bodies never reach the file log.
func newAicardTemplateTransportFactory(runner executor.Runner, flags *GlobalFlags) mcpPublishedTransportFactory {
	return func(ctx context.Context) (mcpPublishedTransport, error) {
		base, token, headers, err := resolveMCPPublishedClientConfig(ctx, runner, flags)
		if err != nil {
			return nil, err
		}
		return publishedmcp.New(aicardTemplateTransport(base), token, headers), nil
	}
}

// aicardTemplateTransport returns a copy of base whose diagnostic file log hides
// tool arguments. The template travels as one JSON string under "file", and the
// shared redaction works by key, so it would otherwise log the template body.
// The request itself and the shared runner transport are left unchanged.
func aicardTemplateTransport(base *transport.Client) *transport.Client {
	client := base.WithAuth(base.AuthToken, base.ExtraHeaders)
	// Snapshots contain both request and response bodies, including retrieved files.
	client.SnapshotRecorder = nil
	if base.FileLogger != nil {
		client.FileLogger = slog.New(aicardTemplateLogHandler{inner: base.FileLogger.Handler()})
	}
	return client
}

const aicardTemplateArgumentsAttr = "arguments_summary"

type aicardTemplateLogHandler struct {
	inner slog.Handler
}

func (h aicardTemplateLogHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.inner.Enabled(ctx, level)
}

func (h aicardTemplateLogHandler) Handle(ctx context.Context, record slog.Record) error {
	clean := slog.NewRecord(record.Time, record.Level, record.Message, record.PC)
	record.Attrs(func(attr slog.Attr) bool {
		clean.AddAttrs(redactAicardTemplateAttr(attr))
		return true
	})
	return h.inner.Handle(ctx, clean)
}

func (h aicardTemplateLogHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	clean := make([]slog.Attr, len(attrs))
	for index, attr := range attrs {
		clean[index] = redactAicardTemplateAttr(attr)
	}
	return aicardTemplateLogHandler{inner: h.inner.WithAttrs(clean)}
}

func (h aicardTemplateLogHandler) WithGroup(name string) slog.Handler {
	return aicardTemplateLogHandler{inner: h.inner.WithGroup(name)}
}

func redactAicardTemplateAttr(attr slog.Attr) slog.Attr {
	if attr.Key == aicardTemplateArgumentsAttr || attr.Key == "body" {
		return slog.String(attr.Key, "***")
	}
	return attr
}

func invokeAicardTemplate(ctx context.Context, caller edition.ToolCaller, factory mcpPublishedTransportFactory, mcpID, tool string, args map[string]any) (publishedmcp.ValidatedInvocationResult, error) {
	endpoint, err := resolvePublishedMCPEndpoint(ctx, mcpPublishedCallerWithDeadline(caller, ctx), mcpID)
	if err != nil {
		return publishedmcp.ValidatedInvocationResult{}, err
	}
	client, err := factory(ctx)
	if err != nil {
		return publishedmcp.ValidatedInvocationResult{}, err
	}
	return client.InvokeValidated(ctx, endpoint, tool, args)
}

func (r recordingToolCaller) InvokeAicardTemplate(ctx context.Context, mcpID, tool string, args map[string]any) (publishedmcp.ValidatedInvocationResult, error) {
	inner, ok := r.inner.(helpers.AicardTemplateCaller)
	if !ok {
		return publishedmcp.ValidatedInvocationResult{}, fmt.Errorf("Published template caller is not configured")
	}
	recordedArgs := cloneToolArgs(args)
	result, err := inner.InvokeAicardTemplate(ctx, mcpID, tool, args)
	usage.Append("aicard-template", tool, recordedArgs, err == nil && !result.Result.IsError, r.inner.DryRun())
	return result, err
}
