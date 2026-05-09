// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package app

import (
	"bytes"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
)

// TestUpgradeCommand_BlockedInEmbeddedMode 验证嵌入模式（如 wukong real）下
// 直接调用 dws upgrade 会被 RunE 入口的守卫拦下，避免 CLI 自替换破坏宿主集成。
//
// 这是 wukong overlay 在 RegisterExtraCommands 阶段调用 RemoveCommand 之外
// 的"纵深防御"层：即便外部嵌入式发行版没有摘除该命令，仍能挡住自升级。
func TestUpgradeCommand_BlockedInEmbeddedMode(t *testing.T) {
	prev := edition.Get()
	edition.Override(&edition.Hooks{IsEmbedded: true, Name: "wukong"})
	t.Cleanup(func() { edition.Override(prev) })

	cases := []struct {
		name string
		args []string
	}{
		{"check", []string{"--check"}},
		{"list", []string{"--list"}},
		{"rollback", []string{"--rollback"}},
		{"plain", []string{}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cmd := newUpgradeCommand()
			var out, errBuf bytes.Buffer
			cmd.SetOut(&out)
			cmd.SetErr(&errBuf)
			cmd.SetArgs(tc.args)

			err := cmd.Execute()
			if err == nil {
				t.Fatalf("upgrade %v in embedded mode must return error, got nil", tc.args)
			}
			msg := err.Error()
			if !strings.Contains(msg, "嵌入模式") {
				t.Errorf("error message should mention 嵌入模式, got: %q", msg)
			}
			if !strings.Contains(msg, "wukong") {
				t.Errorf("error message should include edition name (wukong), got: %q", msg)
			}
			if !strings.Contains(msg, "dws upgrade") {
				t.Errorf("error message should reference dws upgrade for clarity, got: %q", msg)
			}
		})
	}
}

// TestUpgradeCommand_NotBlockedInOpenSourceMode 验证开源模式（IsEmbedded=false）
// 下守卫不会拦截 dws upgrade —— 命令应正常进入升级流程，错误（如有）来自
// 网络 / 版本比较等下游步骤，与守卫无关。
//
// 这里仅断言守卫文本不出现；不去打真实的 GitHub Releases，避免单测依赖网络。
// 选用 --check 的原因：路径最短，最快触发到守卫之后的代码。
func TestUpgradeCommand_NotBlockedInOpenSourceMode(t *testing.T) {
	prev := edition.Get()
	edition.Override(&edition.Hooks{IsEmbedded: false, Name: "open"})
	t.Cleanup(func() { edition.Override(prev) })

	cmd := newUpgradeCommand()
	var out, errBuf bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errBuf)
	cmd.SetArgs([]string{"--check"})

	err := cmd.Execute()
	// 网络可能失败也可能成功，这里只关心：错误不应是守卫产生的中文文案。
	if err != nil && strings.Contains(err.Error(), "嵌入模式") {
		t.Errorf("open-source mode must not be blocked by embedded guard, got: %v", err)
	}
}
