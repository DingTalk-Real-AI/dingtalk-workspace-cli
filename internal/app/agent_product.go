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
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/agentproduct"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/configmeta"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
)

func init() {
	configmeta.Register(configmeta.ConfigItem{
		Name:         agentproduct.EnvName,
		Category:     configmeta.CategoryExternal,
		Description:  "调用方声明的 Agent 产品标识；覆盖 HTTP claw-type，但不是认证凭据",
		DefaultValue: "由当前发行版决定",
		Example:      "qwenwork",
	})
}

// parseAgentProduct converts the reusable package error into the CLI's stable
// structured validation error without exposing the untrusted raw value.
func parseAgentProduct(raw string) (string, error) {
	value, err := agentproduct.Parse(raw)
	if err != nil {
		return "", invalidAgentProductError()
	}
	return value, nil
}

func invalidAgentProductError() error {
	return apperrors.NewValidation(
		"DWS_AGENT_PRODUCT must be at most 64 bytes and match ^[A-Za-z0-9][A-Za-z0-9_-]*$",
		apperrors.WithReason("invalid_agent_product"),
	)
}

// resolveEffectiveAgentProduct resolves the request-header identity with one
// shared precedence rule: a valid non-empty runtime override wins, otherwise
// the edition's MergeHeaders value wins, otherwise the OSS default is used.
// Invalid runtime input falls back here for library callers that bypass root
// validation; normal CLI execution rejects it before network access.
func resolveEffectiveAgentProduct(headers map[string]string) string {
	fallback := edition.DefaultOSSClawType
	if value := headers[agentproduct.HeaderName]; value != "" {
		fallback = value
	}
	value, err := agentproduct.ResolveFromEnv(fallback)
	if err != nil {
		return fallback
	}
	return value
}

func applyAgentProductOverride(headers map[string]string) map[string]string {
	value := resolveEffectiveAgentProduct(headers)
	if headers == nil {
		headers = make(map[string]string)
	}
	headers[agentproduct.HeaderName] = value
	return headers
}
