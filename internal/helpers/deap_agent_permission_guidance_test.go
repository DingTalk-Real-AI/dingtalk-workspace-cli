// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package helpers

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
)

func TestCrossPlatformCoverageDeapAgentSkillPermissionGuidance(t *testing.T) {
	const guide = "当前账号暂无数字员工相关操作权限。请联系企业主管理员（张三），登录管理后台权限管理页（https://workforce.dingtalk.com/#/dai/role-manage），为你的账号添加对应权限后重试。"
	for _, tc := range []struct {
		name, detail string
		redacted     bool
	}{
		{"permission link", guide, false},
		{"ordinary error", "上传服务暂不可用", false},
		{"empty detail", "", false},
		{"signed URL", "https://oss.example/skill.zip?Signature=private-value", true},
		{"link with query", "https://workforce.dingtalk.com/#/dai/role-manage?key=private-value", true},
		{"link with suffix", "https://workforce.dingtalk.com/#/dai/role-manage/private-value", true},
		{"link plus credential", guide + " token=private-value", true},
		{"link plus other URL", guide + " https://oss.example/private-value", true},
		{"HTTP link", "http://workforce.dingtalk.com/#/dai/role-manage", true},
		{"secret", "secret=private-value", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := &deapAgentSkillStageError{Operation: "update", Stage: "upload", Err: errors.New(tc.detail)}
			got := err.Error()
			if tc.redacted {
				if !strings.Contains(got, "下游错误详情已脱敏") || strings.Contains(got, "private-value") {
					t.Fatalf("unsafe error: %s", got)
				}
			} else if !strings.Contains(got, tc.detail) || strings.Contains(got, "下游错误详情已脱敏") {
				t.Fatalf("safe guidance lost: %s", got)
			}
		})
	}
}

func TestCrossPlatformCoverageDeapAgentSkillUpdateMissingPermissionMessage(t *testing.T) {
	for _, mode := range []string{"enabled", "credential", "replace-package"} {
		t.Run(mode, func(t *testing.T) {
			caller, _ := newDeapAgentTestTree(t, false)
			original := apperrors.NewAPI("business error: code NO_PERMISSION",
				apperrors.WithServerKey(deapAgentServerID),
				apperrors.WithOperation("tools/call"),
				apperrors.WithServerDiag(apperrors.ServerDiagnostics{ServerErrorCode: "NO_PERMISSION", TraceID: "trace-test"}))
			caller.err = original
			args := map[string]any{"agentUuid": "agent", "skillId": "skill"}
			wantTool := deapAgentSkillUpdateTool
			if mode != "enabled" {
				t.Chdir(t.TempDir())
				args["file"] = employeeZIPFixture(t, zip.FileHeader{Name: "SKILL.md"})
				if mode == "credential" {
					wantTool = deapAgentSkillUploadCredentialTool
				} else {
					testseam.Swap(t, &deapAgentSkillUploader, deapAgentSkillPackageUploader(
						&deapAgentSkillUploaderStub{fileURL: "https://oss.example/fixture.zip"}))
				}
			} else {
				args["enabled"] = false
			}
			cmd := newDeapAgentSkillUpdateCommand()
			cmd.SetContext(context.Background())
			err := deapAgentCallSkillUpdate(cmd, deapAgentSkillUpdateTool, args)
			if err == nil || !strings.Contains(err.Error(), "https://workforce.dingtalk.com/#/dai/role-manage") ||
				!strings.Contains(err.Error(), "若已是") || strings.Contains(err.Error(), "下游错误详情已脱敏") {
				t.Fatalf("missing conditional permission guidance: %v", err)
			}
			var typed *apperrors.Error
			if !errors.As(err, &typed) || typed.ExitCode() != 1 || typed.ServerDiag.TraceID != "trace-test" ||
				typed.ServerDiag.ServerErrorCode != "NO_PERMISSION" || !errors.Is(err, original) {
				t.Fatalf("error contract lost: %#v / %v", typed, err)
			}
			if len(caller.calls) != 1 || caller.calls[0].toolName != wantTool {
				t.Fatalf("write replayed or continued after refusal: %#v", caller.calls)
			}
			var output bytes.Buffer
			if printErr := apperrors.PrintJSON(&output, err); printErr != nil ||
				!strings.Contains(output.String(), `"server_error_code": "NO_PERMISSION"`) ||
				!strings.Contains(output.String(), `"trace_id": "trace-test"`) ||
				!strings.Contains(output.String(), "role-manage") {
				t.Fatalf("JSON error lost guidance or diagnostics: %s / %v", output.String(), printErr)
			}
		})
	}
}

func TestCrossPlatformCoverageDeapAgentPermissionGuidancePreservesOtherErrors(t *testing.T) {
	var typedNil *apperrors.Error
	for _, original := range []error{
		nil, typedNil, errors.New("business error: code NO_PERMISSION"),
		apperrors.NewAPI("business error: code NO_PERMISSION", apperrors.WithServerKey("other-product"),
			apperrors.WithServerDiag(apperrors.ServerDiagnostics{ServerErrorCode: "NO_PERMISSION"})),
		apperrors.NewAPI("企业未开通", apperrors.WithServerDiag(apperrors.ServerDiagnostics{ServerErrorCode: "DIGITAL_EMPLOYEE_NOT_ENABLED"})),
		apperrors.NewAPI("数字员工权限校验已通过，但能力服务未允许本次操作，无需重复申请。",
			apperrors.WithServerDiag(apperrors.ServerDiagnostics{ServerErrorCode: "NO_PERMISSION"})),
	} {
		if got := deapAgentWithPermissionGuidance(original); got != original {
			t.Fatalf("unrelated or specific server error was replaced: %#v", got)
		}
	}
	for _, message := range []string{"", "NO_PERMISSION", "business error: code NO_PERMISSION", "business error: success=false"} {
		original := apperrors.NewAPI(message, apperrors.WithServerDiag(apperrors.ServerDiagnostics{ServerErrorCode: "NO_PERMISSION"}))
		if got := deapAgentWithPermissionGuidance(fmt.Errorf("wrapped: %w", original)); !strings.Contains(got.Error(), "服务端未返回具体原因") || original.Error() != message {
			t.Fatalf("missing guidance or original error mutated: %#v", got)
		}
	}
}
