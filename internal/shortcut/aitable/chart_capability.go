// Copyright 2026 Alibaba Group
// SPDX-License-Identifier: Apache-2.0

package aitable

import (
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/helpers"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
)

// Explicit historical flags retain their meaning; omission now prefers a
// confirmed capability. This decision happens once, before any write, and is
// never revisited after an uncertain submission.
func preferServerChartTool(rt *shortcut.RuntimeContext, flag, tool string) (bool, error) {
	if rt.Changed(flag) {
		return rt.Bool(flag), nil
	}
	return helpers.MCPToolAvailable(rt.Command().Context(), serverMain, tool)
}
