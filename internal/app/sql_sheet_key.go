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

package app

import (
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/aitableprotocol"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/executor"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/jsonutil"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/transport"
)

// sqlSheetKeyToolResultError validates the original text before using the parsed business envelope.
func sqlSheetKeyToolResultError(tool string, result transport.ToolCallResult) error {
	if result.StructuredContent == nil && len(result.Blocks) > 0 {
		if len(result.Blocks) != 1 || result.Blocks[0].Type != "text" {
			return aitableprotocol.SQLSheetKeyFailure(tool, "")
		}
		raw := []byte(result.Blocks[0].Text)
		if jsonutil.RejectDuplicateObjectKeys(raw) != nil || jsonutil.RejectNonCanonicalObjectKeys(raw, "status", "data", "error") != nil {
			return aitableprotocol.SQLSheetKeyFailure(tool, "")
		}
	}
	return aitableprotocol.SQLSheetKeyResponseError(tool, result.Content, result.IsError)
}

// sqlSheetKeyResult preserves content scanning without the authentication replay path.
func (r *runtimeRunner) sqlSheetKeyResult(endpoint string, invocation executor.Invocation, content map[string]any) (executor.Result, error) {
	report, err := r.scanContent(content)
	if err != nil {
		return executor.Result{}, err
	}
	invocation.Implemented = true
	response := map[string]any{"endpoint": transport.RedactURL(endpoint), "content": content}
	if r.includeScanReport && report.Scanned {
		// Scan findings retain location and severity, not copies of credential text.
		for i := range report.Findings {
			report.Findings[i].Snippet = ""
		}
		response["safety"] = report
	}
	return executor.Result{Invocation: invocation, Response: response}, nil
}
