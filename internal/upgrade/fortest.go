// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package upgrade

import (
	"context"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
)

// SwapUserHomeDirForTest swaps the home-dir seam used by UpgradeSkillLocations
// and related upgrade path helpers, isolating inherited Agent configuration
// roots as well. Restored via t.Cleanup; not safe with
// t.Parallel.
func SwapUserHomeDirForTest(t *testing.T, fn func() (string, error)) {
	t.Helper()
	testseam.Swap(t, &upgradeUserHomeDir, fn)
	testseam.Swap(t, &upgradeGetenv, func(string) string { return "" })
}

// SwapPlatformGitHubTokenForTest 隔离升级测试与本机登录态；不可用于并行测试。
func SwapPlatformGitHubTokenForTest(t *testing.T, fn func(context.Context) string) {
	t.Helper()
	testseam.Swap(t, &platformGitHubToken, fn)
}
