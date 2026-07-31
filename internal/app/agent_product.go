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
		Description:  "调用方声明的 Agent 产品标识；作为 x-dws-agent-product 发送并用于 IM 小尾巴，本客户端不使用该值改变 HTTP claw-type/PAT",
		DefaultValue: "未设置（请求头省略，IM 使用当前发行版默认值）",
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

// resolveEditionClawType resolves the fixed routing/PAT identity supplied by
// the active edition. DWS_AGENT_PRODUCT is deliberately not consulted.
func resolveEditionClawType(headers map[string]string) string {
	if value := headers["claw-type"]; value != "" {
		return value
	}
	return edition.DefaultOSSClawType
}

// applyAgentProductHeader injects only a valid, non-empty caller-declared
// product. Invalid values are omitted on library paths that bypass root
// validation; normal CLI execution rejects them before network access.
func applyAgentProductHeader(headers map[string]string) map[string]string {
	value, err := agentproduct.ResolveFromEnv("")
	if err != nil || value == "" {
		if headers != nil {
			delete(headers, agentproduct.HeaderName)
		}
		return headers
	}
	if headers == nil {
		headers = make(map[string]string)
	}
	headers[agentproduct.HeaderName] = value
	return headers
}
