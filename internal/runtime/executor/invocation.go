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

package executor

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/platform/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/runtime/ir"
)

type Invocation struct {
	Kind             string         `json:"kind"`
	Stage            string         `json:"stage"`
	Implemented      bool           `json:"implemented"`
	DryRun           bool           `json:"dry_run,omitempty"`
	CanonicalProduct string         `json:"canonical_product"`
	Tool             string         `json:"tool"`
	CanonicalPath    string         `json:"canonical_path"`
	LegacyPath       string         `json:"legacy_path,omitempty"`
	Params           map[string]any `json:"params"`
}

type Result struct {
	Invocation Invocation     `json:"invocation"`
	Response   map[string]any `json:"response,omitempty"`
}

type Runner interface {
	Run(context.Context, Invocation) (Result, error)
}

type EchoRunner struct{}

func (EchoRunner) Run(_ context.Context, invocation Invocation) (Result, error) {
	if invocation.DryRun {
		return Result{
			Invocation: invocation,
			Response: map[string]any{
				"dry_run": true,
				"request": ToolCallRequest(invocation.Tool, invocation.Params),
				"note":    "execution skipped by --dry-run",
			},
		}, nil
	}
	return Result{Invocation: invocation}, nil
}

func NewInvocation(product ir.CanonicalProduct, tool ir.ToolDescriptor, params map[string]any) Invocation {
	if params == nil {
		params = map[string]any{}
	}
	return Invocation{
		Kind:             "canonical_invocation",
		Stage:            "canonical_cli",
		Implemented:      false,
		CanonicalProduct: product.ID,
		Tool:             tool.RPCName,
		CanonicalPath:    tool.CanonicalPath,
		Params:           params,
	}
}

func NewCompatInvocation(legacyPath, canonicalProduct, tool string, params map[string]any) Invocation {
	if params == nil {
		params = map[string]any{}
	}
	return Invocation{
		Kind:             "compat_invocation",
		Stage:            "compat_cli",
		Implemented:      false,
		CanonicalProduct: canonicalProduct,
		Tool:             tool,
		CanonicalPath:    canonicalProduct + "." + tool,
		LegacyPath:       legacyPath,
		Params:           params,
	}
}

func NewHelperInvocation(legacyPath, canonicalProduct, tool string, params map[string]any) Invocation {
	if params == nil {
		params = map[string]any{}
	}
	return Invocation{
		Kind:             "helper_invocation",
		Stage:            "helper_override",
		Implemented:      false,
		CanonicalProduct: canonicalProduct,
		Tool:             tool,
		CanonicalPath:    canonicalProduct + "." + tool,
		LegacyPath:       legacyPath,
		Params:           params,
	}
}

func NewWorkflowInvocation(legacyPath, workflowName string, steps []Invocation) Invocation {
	stepDescriptions := make([]any, 0, len(steps))
	for _, step := range steps {
		stepDescriptions = append(stepDescriptions, map[string]any{
			"product": step.CanonicalProduct,
			"tool":    step.Tool,
		})
	}
	return Invocation{
		Kind:             "workflow_invocation",
		Stage:            "workflow",
		Implemented:      false,
		CanonicalProduct: "workflow",
		Tool:             workflowName,
		CanonicalPath:    "workflow." + workflowName,
		LegacyPath:       legacyPath,
		Params: map[string]any{
			"steps": stepDescriptions,
		},
	}
}

func MergePayloads(jsonPayload, paramsPayload string, overrides map[string]any) (map[string]any, error) {
	merged := make(map[string]any)

	for label, payload := range map[string]string{
		"--json":   jsonPayload,
		"--params": paramsPayload,
	} {
		value, err := parseJSONObject(label, payload)
		if err != nil {
			return nil, err
		}
		for key, raw := range value {
			merged[key] = raw
		}
	}

	for key, value := range overrides {
		merged[key] = value
	}
	return merged, nil
}

func parseJSONObject(label, raw string) (map[string]any, error) {
	if strings.TrimSpace(raw) == "" {
		return map[string]any{}, nil
	}

	var value any
	if err := json.Unmarshal([]byte(raw), &value); err != nil {
		return nil, apperrors.NewValidation(fmt.Sprintf("%s must be valid JSON: %v", label, err))
	}

	object, ok := value.(map[string]any)
	if !ok {
		return nil, apperrors.NewValidation(fmt.Sprintf("%s must decode to a JSON object", label))
	}
	return object, nil
}

func ToolCallRequest(tool string, params map[string]any) map[string]any {
	if params == nil {
		params = map[string]any{}
	}
	return map[string]any{
		"jsonrpc": "2.0",
		"id":      3,
		"method":  "tools/call",
		"params": map[string]any{
			"name":      tool,
			"arguments": params,
		},
	}
}
