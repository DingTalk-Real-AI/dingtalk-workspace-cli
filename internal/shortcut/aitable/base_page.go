// Copyright 2026 Alibaba Group
// SPDX-License-Identifier: Apache-2.0

package aitable

import (
	"strings"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut/aitabletarget"
)

// Keep the established collection projection and retain the service's paging
// evidence even when deleted or inaccessible objects make this page empty.
func outputBasePage(rt *shortcut.RuntimeContext, bases []map[string]any, data map[string]any) error {
	next, more, known := aitabletarget.Pagination(data)
	if known && !more {
		next = ""
	}
	if next != "" {
		more, known = true, true
		if next == strings.TrimSpace(rt.Str("cursor")) {
			return apperrors.NewAPI("Base 列表返回了未前进的分页游标", apperrors.WithReason("aitable_cursor_stalled"))
		}
	}
	if known && more && next == "" {
		return apperrors.NewAPI("Base 列表还有后续数据但未返回游标", apperrors.WithReason("aitable_cursor_missing"))
	}
	out := map[string]any{"count": len(bases), "bases": bases}
	if known {
		out["hasMore"] = more
	}
	if next != "" {
		out["nextCursor"] = next
	}
	return rt.Output(out)
}
