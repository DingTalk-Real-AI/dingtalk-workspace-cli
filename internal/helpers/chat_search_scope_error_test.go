// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package helpers

import (
	"errors"
	"testing"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
)

func TestCrossPlatformCoverageNormalizeSearchConversationScopeError(t *testing.T) {
	if got := NormalizeSearchConversationScopeError("cid", nil); got != nil {
		t.Fatalf("nil error normalized to %#v", got)
	}

	invalidCases := []struct {
		name string
		err  error
	}{
		{
			name: "classified resource not found",
			err:  &CLIError{Code: CodeResourceNotFound, Message: "conversation not found"},
		},
		{
			name: "explicit conversation error code",
			err: &CLIError{
				Code:    CodeMCPToolError,
				Message: `{"success":false,"errorCode":"INVALID_OPEN_CONVERSATION_ID","errorMsg":"invalid conversation"}`,
			},
		},
		{
			name: "conversation specific parameter error",
			err: &CLIError{
				Code:    CodeMCPToolError,
				Message: `{"success":false,"errorCode":"PARAM_ERROR","errorMsg":"openConversationId is invalid"}`,
			},
		},
		{
			name: "transport diagnostic proves invalid conversation",
			err: apperrors.NewAPI(
				"conversation validation failed",
				apperrors.WithServerDiag(apperrors.ServerDiagnostics{
					ServerErrorCode: "INVALID_OPEN_CONVERSATION_ID",
				}),
			),
		},
	}
	for _, test := range invalidCases {
		t.Run(test.name, func(t *testing.T) {
			got := NormalizeSearchConversationScopeError("cid-invalid", test.err)
			var typed *apperrors.Error
			if !errors.As(got, &typed) || typed.Reason != "search_conversation_scope_invalid" {
				t.Fatalf("normalized error = %#v", got)
			}
			if !typed.RetryableSet || typed.Retryable {
				t.Fatalf("retryable = (%t, set=%t), want false and set", typed.Retryable, typed.RetryableSet)
			}
			if typed.Details["conversationId"] != "cid-invalid" || !errors.Is(got, test.err) {
				t.Fatalf("normalized error lost details or cause: %#v", typed)
			}
		})
	}

	preservedCases := []struct {
		name string
		err  error
	}{
		{
			name: "rate limit",
			err: &CLIError{
				Code:    CodeMCPToolError,
				Message: `{"success":false,"errorCode":"invalidRequest.rateLimited","errorMsg":"slow down","retryable":true}`,
			},
		},
		{
			name: "permission denied",
			err: &CLIError{
				Code:    CodeMCPToolError,
				Message: `{"success":false,"errorCode":"forbidden.noPermission","errorMsg":"permission denied"}`,
			},
		},
		{
			name: "generic parameter error without CID evidence",
			err: &CLIError{
				Code:    CodeMCPToolError,
				Message: `{"success":false,"errorCode":"PARAM_ERROR","errorMsg":"未找到指定工具"}`,
			},
		},
		{
			name: "structured error without recognized facts",
			err: &CLIError{
				Code:    CodeMCPToolError,
				Message: `{"success":false,"retryable":true}`,
			},
		},
		{
			name: "parameter error mentions CID without invalid evidence",
			err: &CLIError{
				Code:    CodeMCPToolError,
				Message: `{"success":false,"errorCode":"PARAM_ERROR","errorMsg":"openConversationId could not be processed"}`,
			},
		},
	}
	for _, test := range preservedCases {
		t.Run(test.name, func(t *testing.T) {
			if got := NormalizeSearchConversationScopeError("cid-target", test.err); got != test.err {
				t.Fatalf("error = %#v, want original %#v", got, test.err)
			}
		})
	}
}
