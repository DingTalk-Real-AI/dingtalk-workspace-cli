package app

import (
	"context"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/executor"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/whiteboard"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
)

func TestCrossPlatformCoverageWhiteboardTemplatePreviewBoundary(t *testing.T) {
	ctx := context.Background()
	fallback := &capturingSuccessRunner{}
	flags := &GlobalFlags{DryRun: true}
	runner := &runtimeRunner{globalFlags: flags, fallback: fallback}
	adapter := &toolCallerAdapter{flags: flags, runner: runner}
	caller := newRecordingToolCaller(adapter).(interface {
		edition.ToolCaller
		edition.WhiteboardTemplatePreviewCaller
	})
	for _, tool := range []string{whiteboard.PersonalTemplateSaveTool, whiteboard.TeamTemplateSaveTool, whiteboard.PersonalTemplateCreateTool, whiteboard.TeamTemplateCreateTool} {
		before := fallback.calls.Load()
		if _, err := caller.CallWhiteboardTemplatePreview(ctx, tool, map[string]any{"dryRun": true}); err != nil {
			t.Fatal(err)
		}
		if fallback.calls.Load() != before+1 || fallback.invocation.Params["dryRun"] != true || fallback.invocation.DryRun {
			t.Fatalf("preflight not dispatched correctly: %#v", fallback.invocation)
		}
		if !flags.DryRun {
			t.Fatal("global barrier changed")
		}
		if _, err := caller.CallTool(ctx, "whiteboard", tool, nil); err != nil {
			t.Fatal(err)
		}
		if fallback.calls.Load() != before+1 {
			t.Fatal("ordinary write escaped global dry-run")
		}
	}
	before := fallback.calls.Load()
	for _, args := range []map[string]any{nil, {"dryRun": false}, {"dryRun": "true"}} {
		if _, err := caller.CallWhiteboardTemplatePreview(ctx, whiteboard.PersonalTemplateSaveTool, args); err == nil {
			t.Fatal("unsafe preflight accepted")
		}
		inv := executor.NewHelperInvocation("test", "whiteboard", whiteboard.PersonalTemplateSaveTool, args)
		if _, err := runner.RunWhiteboardTemplatePreview(ctx, inv); err == nil {
			t.Fatal("runner accepted unsafe preflight")
		}
	}
	for _, serverTool := range [][2]string{{"whiteboard", "update_whiteboard"}, {"doc", whiteboard.PersonalTemplateSaveTool}} {
		inv := executor.NewHelperInvocation("test", serverTool[0], serverTool[1], map[string]any{"dryRun": true})
		if _, err := runner.RunWhiteboardTemplatePreview(ctx, inv); err == nil {
			t.Fatal("runner accepted unreviewed tool")
		}
	}
	if _, err := caller.CallWhiteboardTemplatePreview(ctx, "update_whiteboard", map[string]any{"dryRun": true}); err == nil {
		t.Fatal("adapter accepted write tool")
	}
	unsupported := &toolCallerAdapter{flags: flags, runner: fallback}
	if _, err := unsupported.CallWhiteboardTemplatePreview(ctx, whiteboard.PersonalTemplateSaveTool, map[string]any{"dryRun": true}); err == nil {
		t.Fatal("missing capability did not fail closed")
	}
	flags.DryRun = false
	if _, err := caller.CallWhiteboardTemplatePreview(ctx, whiteboard.PersonalTemplateSaveTool, map[string]any{"dryRun": true}); err == nil {
		t.Fatal("preflight accepted outside dry-run")
	}
	inv := executor.NewHelperInvocation("test", "whiteboard", whiteboard.PersonalTemplateSaveTool, map[string]any{"dryRun": true})
	if _, err := runner.RunWhiteboardTemplatePreview(ctx, inv); err == nil {
		t.Fatal("runner accepted outside dry-run")
	}
	if fallback.calls.Load() != before {
		t.Fatal("rejected request reached runner")
	}
}

func TestCrossPlatformCoverageWhiteboardTemplatePreviewErrorPropagation(t *testing.T) {
	flags := &GlobalFlags{DryRun: true}
	adapter := &toolCallerAdapter{flags: flags, runner: &runtimeRunner{globalFlags: flags, fallback: &countingErrorRunner{}}}
	if _, err := adapter.CallWhiteboardTemplatePreview(context.Background(), whiteboard.PersonalTemplateSaveTool, map[string]any{"dryRun": true}); err == nil {
		t.Fatal("runner failure lost")
	}
	caller := recordingToolCaller{inner: newToolCallerAdapter(&countingErrorRunner{}, flags)}
	// Hide the optional preview capability behind the ordinary caller interface.
	caller.inner = &ordinaryWhiteboardCaller{ToolCaller: caller.inner}
	if _, err := caller.CallWhiteboardTemplatePreview(context.Background(), whiteboard.PersonalTemplateSaveTool, nil); err == nil {
		t.Fatal("missing preflight capability accepted")
	}
}

type ordinaryWhiteboardCaller struct{ edition.ToolCaller }
