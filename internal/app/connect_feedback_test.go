// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package app

import (
	"errors"
	"testing"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/pipeline"
)

func TestCrossPlatformCoverageConnectUnknownCommandOnlyPointsToHelp(t *testing.T) {
	for _, token := range []string{"bind", "rebind", "statrus", "unknown-action"} {
		t.Run(token, func(t *testing.T) {
			root := NewRootCommand()
			_, err := pipeline.RunPreParseArgs(root, newPipelineEngine(), []string{"dingtalk-tag", "connect", token, "--obsolete-flag", "value"})
			var typed *apperrors.Error
			if !errors.As(err, &typed) || typed.Reason != "unknown_subcommand" {
				t.Fatalf("未识别未知命令: %v", err)
			}
			if typed.Hint != "Run 'dws dingtalk-tag connect --help' for the full list" {
				t.Fatalf("未知命令应只引导帮助: %q", typed.Hint)
			}
			suggestions, ok := typed.Details["suggestions"].([]string)
			if !ok || len(suggestions) != 0 {
				t.Fatalf("存在误导性推荐: %#v", typed.Details)
			}
		})
	}
}
