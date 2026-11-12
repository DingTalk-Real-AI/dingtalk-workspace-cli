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

package publishedmcp

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"time"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/mcpschema"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/transport"
	"github.com/google/uuid"
)

// Operation mode is opt-in for the CC HTTP operation contract, never inferred
// from a tool's name. The immutable journal grants exactly one initial send.
// Reopening it performs authenticated lookups only, even after a lost response.
type OperationTransport interface {
	Tools(context.Context, string) (transport.ToolsListResult, error)
	InvokeValidated(context.Context, string, string, map[string]any) (ValidatedInvocationResult, error)
}
type operationJournal struct {
	Version   int    `json:"version"`
	MCPID     string `json:"mcpId"`
	Tool      string `json:"tool"`
	ArgsHash  string `json:"argsHash"`
	RequestID string `json:"requestId"`
}

func operationError(message string) error {
	return apperrors.NewValidation(message, apperrors.WithReason("published_mcp_operation_contract"))
}

func InvokeOperation(ctx context.Context, client OperationTransport, endpoint, mcpID, tool string, args map[string]any, file string) (ValidatedInvocationResult, error) {
	if tool == "operation_get" {
		return ValidatedInvocationResult{}, operationError("operation-file starts a business tool; use invoke operation_get directly for explicit queries")
	}
	if _, has := args["requestId"]; has {
		return ValidatedInvocationResult{}, operationError("operation mode owns requestId; omit it from --params")
	}
	raw, err := json.Marshal(args)
	if err != nil {
		return ValidatedInvocationResult{}, err
	}
	wanted := operationJournal{Version: 1, MCPID: mcpID, Tool: tool, ArgsHash: fmt.Sprintf("%x", sha256.Sum256(raw)), RequestID: uuid.NewString()}
	// Verify both declarations before the first write. A missing recovery tool
	// must not leave a freshly dispatched write without a recovery path.
	list, err := client.Tools(ctx, endpoint)
	if err != nil {
		return ValidatedInvocationResult{}, err
	}
	initial := map[string]any{}
	for k, v := range args {
		initial[k] = v
	}
	initial["requestId"] = wanted.RequestID
	checks := map[string]map[string]any{"operation_get": {"requestId": wanted.RequestID}}
	if _, statErr := os.Lstat(file); errors.Is(statErr, os.ErrNotExist) {
		checks[tool] = initial
	}
	for name, values := range checks {
		schema, n := exactToolInputSchema(list, name)
		if n != 1 {
			return ValidatedInvocationResult{}, operationError("operation mode requires unique tool and operation_get declarations")
		}
		props, _ := schema["properties"].(map[string]any)
		if props["requestId"] == nil {
			return ValidatedInvocationResult{}, operationError("operation contract must declare requestId")
		}
		if err := mcpschema.ValidateInputSchema(values, schema); err != nil {
			return ValidatedInvocationResult{}, err
		}
	}
	record, created, err := openOperation(file, wanted)
	if err != nil {
		return ValidatedInvocationResult{}, err
	}
	var out ValidatedInvocationResult
	if created {
		initial["requestId"] = record.RequestID
		out, err = client.InvokeValidated(ctx, endpoint, tool, initial)
		out.ResultTool = tool
	} else {
		out, err = client.InvokeValidated(ctx, endpoint, "operation_get", map[string]any{"requestId": record.RequestID})
		out.ResultTool = "operation_get"
	}
	// Never repeat a failed tools/call. The durable journal permits a later
	// authenticated lookup without remembering a response's operationId.
	if err != nil {
		return out, err
	}
	for {
		if out.Result.IsError {
			return out, nil
		}
		value := out.Result.Content
		if out.Result.StructuredContent != nil {
			value = out.Result.StructuredContent
		}
		success, ok := value["success"].(bool)
		if !ok {
			return out, operationError("operation response lacks success boolean")
		}
		state, _ := value["executionState"].(string)
		switch state {
		case "pending", "completed", "applied", "no_change", "partial", "rejected", "unknown":
		default:
			return out, operationError("unrecognized executionState")
		}
		// Preserve business refusals/partial/unknown in the original result.
		if !success {
			return out, nil
		}
		if state != "pending" {
			return out, nil
		}
		id, _ := value["operationId"].(string)
		if _, err := uuid.Parse(id); err != nil {
			return out, operationError("pending operation has no valid operationId")
		}
		timer := time.NewTimer(time.Second)
		select {
		case <-ctx.Done():
			timer.Stop()
			return out, nil
		case <-timer.C:
		}
		next, err := client.InvokeValidated(ctx, endpoint, "operation_get", map[string]any{"requestId": record.RequestID})
		if err != nil {
			return out, err
		}
		next.ResultTool = "operation_get"
		out = next
	}
}

func openOperation(file string, wanted operationJournal) (operationJournal, bool, error) {
	f, err := os.OpenFile(file, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err == nil {
		// Publish the request identity durably before any network write.
		raw, e := json.Marshal(wanted)
		if e == nil {
			_, e = f.Write(raw)
		}
		if e == nil {
			e = f.Sync()
		}
		if e == nil && runtime.GOOS != "windows" {
			dir, openErr := os.Open(filepath.Dir(file))
			if openErr != nil {
				e = openErr
			} else {
				e = dir.Sync()
				_ = dir.Close()
			}
		}
		closeErr := f.Close()
		if e == nil {
			e = closeErr
		}
		if e != nil {
			return operationJournal{}, false, e
		}
		return wanted, true, nil
	}
	if !errors.Is(err, os.ErrExist) {
		return operationJournal{}, false, err
	}
	info, err := os.Lstat(file)
	if err != nil {
		return operationJournal{}, false, err
	}
	if !info.Mode().IsRegular() || (runtime.GOOS != "windows" && info.Mode().Perm()&0077 != 0) {
		return operationJournal{}, false, operationError("operation file must be a private regular file")
	}
	f, err = os.Open(file)
	if err != nil {
		return operationJournal{}, false, err
	}
	defer f.Close()
	raw, err := io.ReadAll(io.LimitReader(f, 4097))
	if err != nil {
		return operationJournal{}, false, err
	}
	var old operationJournal
	if len(raw) > 4096 || json.Unmarshal(raw, &old) != nil || old.Version != 1 {
		return old, false, operationError("invalid operation file; do not recreate it to retry a write")
	}
	if _, err := uuid.Parse(old.RequestID); err != nil {
		return old, false, operationError("invalid persisted requestId")
	}
	if old.MCPID != wanted.MCPID || old.Tool != wanted.Tool || old.ArgsHash != wanted.ArgsHash {
		return old, false, operationError("operation file belongs to different service, tool or parameters")
	}
	return old, false, nil
}
