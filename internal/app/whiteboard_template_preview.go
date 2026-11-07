package app

import (
	"context"
	"fmt"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/executor"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut/usage"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/whiteboard"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
)

type whiteboardTemplatePreviewRunner interface {
	RunWhiteboardTemplatePreview(context.Context, executor.Invocation) (executor.Result, error)
}

func (a *toolCallerAdapter) CallWhiteboardTemplatePreview(ctx context.Context, tool string, args map[string]any) (*edition.ToolResult, error) {
	if a == nil || !a.DryRun() {
		return nil, fmt.Errorf("whiteboard template preview requires --dry-run")
	}
	if err := whiteboard.ValidateTemplatePreviewCall(whiteboard.ServerID, tool, args); err != nil {
		return nil, err
	}
	runner, ok := a.runner.(whiteboardTemplatePreviewRunner)
	if !ok {
		return nil, fmt.Errorf("runtime does not support whiteboard template remote preflight")
	}
	params := make(map[string]any, len(args))
	for k, v := range args {
		params[k] = v
	}
	inv := executor.NewHelperInvocation("overlay.whiteboard."+tool, whiteboard.ServerID, tool, params)
	result, err := runner.RunWhiteboardTemplatePreview(ctx, inv)
	traceWhiteboardResponse(inv, "template_preflight_before_conversion", result.Response, nil, err)
	if err != nil {
		return nil, err
	}
	return convertResult(result), nil
}

// The exception is checked again at the runner boundary. Only the cloned
// flags disable the local echo; the actual RPC still carries dryRun=true.
func (r *runtimeRunner) RunWhiteboardTemplatePreview(ctx context.Context, inv executor.Invocation) (executor.Result, error) {
	if r == nil || r.globalFlags == nil || !r.globalFlags.DryRun {
		return executor.Result{}, fmt.Errorf("whiteboard template preview requires --dry-run")
	}
	if err := whiteboard.ValidateTemplatePreviewCall(inv.CanonicalProduct, inv.Tool, inv.Params); err != nil {
		return executor.Result{}, err
	}
	clone := *r
	flags := *r.globalFlags
	flags.DryRun = false
	clone.globalFlags = &flags
	inv.DryRun = false
	return clone.Run(ctx, inv)
}

func (r recordingToolCaller) CallWhiteboardTemplatePreview(ctx context.Context, tool string, args map[string]any) (*edition.ToolResult, error) {
	inner, ok := r.inner.(edition.WhiteboardTemplatePreviewCaller)
	if !ok {
		return nil, fmt.Errorf("ToolCaller whiteboard template preflight is not configured")
	}
	recordedArgs := cloneToolArgs(args)
	result, err := inner.CallWhiteboardTemplatePreview(ctx, tool, args)
	usage.Append(whiteboard.ServerID, tool, recordedArgs, err == nil, false)
	return result, err
}
