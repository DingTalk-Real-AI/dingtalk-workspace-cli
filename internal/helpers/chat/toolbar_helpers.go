// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package chat

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
)

func hasIntersection(a, b []int64) bool {
	if len(a) == 0 || len(b) == 0 {
		return false
	}
	seen := make(map[int64]struct{}, len(a))
	for _, v := range a {
		seen[v] = struct{}{}
	}
	for _, v := range b {
		if _, ok := seen[v]; ok {
			return true
		}
	}
	return false
}

func isSystemBusy(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), "SYSTEM_BUSY")
}

func parseExtension(cmd *cobra.Command) (map[string]string, error) {
	raw, _ := cmd.Flags().GetStringArray("extension")
	if len(raw) == 0 {
		return nil, nil
	}
	result := make(map[string]string, len(raw))
	for _, pair := range raw {
		idx := strings.Index(pair, "=")
		if idx <= 0 {
			return nil, fmt.Errorf("--extension 格式错误，期望 key=value，实际 %q", pair)
		}
		key := strings.TrimSpace(pair[:idx])
		value := strings.TrimSpace(pair[idx+1:])
		if key == "" {
			return nil, fmt.Errorf("--extension key 不能为空: %q", pair)
		}
		if _, exists := result[key]; exists {
			return nil, fmt.Errorf("--extension 重复 key %q，每个 key 只能指定一次", key)
		}
		result[key] = value
	}
	return result, nil
}

func toolbarConversationID(cmd *cobra.Command) (string, error) {
	cid, _ := cmd.Flags().GetString("conversation-id")
	cid = strings.TrimSpace(cid)
	if cid == "" {
		return "", apperrors.NewValidation(
			"flag --conversation-id is required",
			apperrors.WithReason("missing_required_flag"),
			apperrors.WithHint("使用 --conversation-id 指定会话 openConversationId"),
			apperrors.WithActions("通过 dws chat search 或 dws chat group list 获取会话 ID"),
		)
	}
	return cid, nil
}

func toolbarNewSystemBusyError() error {
	return apperrors.NewValidation(
		"服务端繁忙（SYSTEM_BUSY），请稍后重试",
		apperrors.WithReason("system_busy"),
		apperrors.WithHint("服务端当前负载较高，建议等待几秒后重试"),
		apperrors.WithActions("等待后使用相同参数重试"),
	)
}
