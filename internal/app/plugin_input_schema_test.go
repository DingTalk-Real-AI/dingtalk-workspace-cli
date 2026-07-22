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
	"context"
	"errors"
	"reflect"
	"testing"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/executor"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/transport"
)

func TestPluginStdioExecutionNormalizesAndValidatesLiveSchema(t *testing.T) {
	isolatePluginRuntime(t)
	previousInit := runnerStdioEnsureInitialized
	previousList := runnerStdioListTools
	previousCall := runnerStdioCallTool
	t.Cleanup(func() {
		runnerStdioEnsureInitialized = previousInit
		runnerStdioListTools = previousList
		runnerStdioCallTool = previousCall
	})

	client := transport.NewStdioClient("unused", nil, nil)
	RegisterStdioClient("conference/local", client)
	runnerStdioEnsureInitialized = func(*transport.StdioClient, context.Context) error {
		return nil
	}
	runnerStdioListTools = func(*transport.StdioClient, context.Context) (transport.ToolsListResult, error) {
		return transport.ToolsListResult{
			Tools: []transport.ToolDescriptor{{
				Name: "create_conference",
				InputSchema: map[string]any{
					"type":     "object",
					"required": []any{"title"},
					"properties": map[string]any{
						"title":           map[string]any{"type": "string"},
						"capture_speaker": map[string]any{"type": "bool"},
					},
					"additionalProperties": false,
				},
			}},
		}, nil
	}
	var calledParams map[string]any
	runnerStdioCallTool = func(
		_ *transport.StdioClient,
		_ context.Context,
		_ string,
		params map[string]any,
	) (transport.ToolCallResult, error) {
		calledParams = params
		return transport.ToolCallResult{Content: map[string]any{"ok": true}}, nil
	}

	runner := &runtimeRunner{}
	invocation := executor.Invocation{
		CanonicalProduct: "conference-local",
		Tool:             "create_conference",
		Params: map[string]any{
			"title":           "schema validation",
			"capture_speaker": "true",
		},
	}
	result, err := runner.executeInvocation(
		context.Background(),
		"stdio://conference/local",
		invocation,
	)
	if err != nil {
		t.Fatalf("stdio plugin execution: %v", err)
	}
	wantParams := map[string]any{
		"title":           "schema validation",
		"capture_speaker": true,
	}
	if !reflect.DeepEqual(calledParams, wantParams) ||
		!reflect.DeepEqual(result.Invocation.Params, wantParams) {
		t.Fatalf("normalized wire params = %#v, result = %#v", calledParams, result.Invocation.Params)
	}

	calledParams = nil
	invocation.Params = map[string]any{"capture_speaker": "true"}
	_, err = runner.executeInvocation(
		context.Background(),
		"stdio://conference/local",
		invocation,
	)
	var appError *apperrors.Error
	if !errors.As(err, &appError) ||
		appError.Category != apperrors.CategoryValidation ||
		calledParams != nil {
		t.Fatalf("missing required schema validation = %#v, call params = %#v", err, calledParams)
	}
}
