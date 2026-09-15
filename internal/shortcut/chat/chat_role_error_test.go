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
	"errors"
	"strings"
	"testing"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
)

func TestCrossPlatformCoverageChatRoleErrorConvergence(t *testing.T) {
	t.Run("unknown typed retryability fails closed", func(t *testing.T) {
		err := convergeChatRoleError("query_custom_user_roles", apperrors.NewAPI("opaque 1001"))
		var typed *apperrors.Error
		if !errors.As(err, &typed) {
			t.Fatalf("error type = %T, want *errors.Error", err)
		}
		if typed.Operation != "im/query_custom_user_roles" ||
			typed.Reason != "chat_role_operation_failed" ||
			!typed.RetryableSet || typed.Retryable {
			t.Fatalf("converged error = %#v", typed)
		}
		if !strings.Contains(typed.Hint, "Help") || !strings.Contains(typed.Hint, "Shortcut/atomic") {
			t.Fatalf("hint = %q, want bounded recovery guidance", typed.Hint)
		}
	})

	t.Run("explicit retryability is preserved", func(t *testing.T) {
		err := convergeChatRoleError(
			"add_custom_group_role",
			apperrors.NewAPI("busy", apperrors.WithRetryable(true)),
		)
		var typed *apperrors.Error
		if !errors.As(err, &typed) || !typed.RetryableSet || !typed.Retryable {
			t.Fatalf("retryability = %#v, want explicit true", typed)
		}
	})

	t.Run("resolution categories keep their precise contract", func(t *testing.T) {
		original := apperrors.NewDiscovery("group is ambiguous", apperrors.WithReason("ambiguous_group"))
		if got := convergeChatRoleError("list_custom_group_roles", original); got != original {
			t.Fatalf("error identity changed: got %p want %p", got, original)
		}
		var typed *apperrors.Error
		if !errors.As(original, &typed) || typed.Reason != "ambiguous_group" || typed.RetryableSet {
			t.Fatalf("discovery error mutated = %#v", typed)
		}
	})

	t.Run("untyped failures become non-retryable internal errors", func(t *testing.T) {
		cause := errors.New("opaque transport failure")
		err := convergeChatRoleError("remove_custom_group_role", cause)
		var typed *apperrors.Error
		if !errors.As(err, &typed) || typed.Category != apperrors.CategoryInternal ||
			!typed.RetryableSet || typed.Retryable || !errors.Is(err, cause) {
			t.Fatalf("internal convergence = %#v, err=%v", typed, err)
		}
	})
}
