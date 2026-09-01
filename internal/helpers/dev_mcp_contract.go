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

package helpers

import (
	"strings"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/spf13/cobra"
)

func devMCPReadSafety() contract.SafetySpec {
	return contract.SafetySpec{
		Effect: "read", Risk: "medium",
		Confirmation: "not_required", Idempotency: "idempotent",
	}
}

func devMCPWriteSafety() contract.SafetySpec {
	return contract.SafetySpec{
		Effect: "write", Risk: "medium",
		Confirmation: "user_required", Idempotency: "unknown",
	}
}

func devMCPDestructiveSafety() contract.SafetySpec {
	return contract.SafetySpec{
		Effect: "destructive", Risk: "high",
		Confirmation: "user_required", Idempotency: "unknown",
	}
}

func devMCPContract(cmd *cobra.Command, tool, cliPath, description string, dryRun bool) LeafContract {
	example := ""
	if cmd != nil {
		for _, line := range strings.Split(cmd.Example, "\n") {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "dws ") {
				example = line
				break
			}
		}
	}
	if example == "" {
		example = "dws " + cliPath
	}
	decl := LeafContract{
		Identity: contract.ToolIdentitySpec{
			ProductID:      "dev",
			Name:           tool,
			CanonicalPath:  "dev." + tool,
			CLIPath:        cliPath,
			PrimaryCLIPath: cliPath,
		},
		Description: description,
		Interface: &contract.InterfaceSpec{
			Mode:         contract.InterfaceModeComposite,
			Availability: contract.InterfaceAvailable,
			Reason:       "Reviewed unpinned adapter: this static developer command calls the helper-only mcpdev/" + tool + " management endpoint, which is intentionally absent from pinned public MCP metadata.",
		},
		Selection: contract.SelectionSpec{
			AgentSummary: description,
			UseWhen:      []string{description},
			AvoidWhen:    []string{"只需调用已发布 MCP 工具时使用 dws mcp published，而不是开发配置命令"},
			Examples:     []string{example},
		},
	}
	if dryRun {
		decl.DryRun = &contract.DryRunSpec{
			PreviewKind: contract.DryRunPreviewInvocation,
			RemoteReads: false,
		}
	}
	return decl
}

func validateDevMCPRequiredIntFlag(flag string) func(*cobra.Command, []string) error {
	return func(cmd *cobra.Command, _ []string) error {
		_, err := devMCPRequiredInt(cmd, flag)
		return err
	}
}

func validateDevMCPToolLocator(cmd *cobra.Command, _ []string) error {
	_, err := devMCPToolLocatorParams(cmd)
	return err
}

func validateDevMCPServiceCreate(cmd *cobra.Command, _ []string) error {
	if _, err := devMCPRequiredString(cmd, "name"); err != nil {
		return err
	}
	if _, err := devMCPRequiredString(cmd, "description"); err != nil {
		return err
	}
	_, err := devMCPServerNameFlag(cmd)
	return err
}

func validateDevMCPServiceUpdate(cmd *cobra.Command, _ []string) error {
	if _, err := devMCPRequiredInt(cmd, "mcp-id"); err != nil {
		return err
	}
	serverName, err := devMCPServerNameFlag(cmd)
	if err != nil {
		return err
	}
	updates := 0
	for _, flag := range []string{"name", "description", "icon-url", "introduction"} {
		if devMCPStringFlag(cmd, flag) != "" {
			updates++
		}
	}
	if serverName != "" {
		updates++
	}
	if updates == 0 {
		return apperrors.NewValidation("至少提供一项待更新字段：--name、--description、--icon-url、--introduction 或 --server-name")
	}
	return nil
}

func validateDevMCPToolCreate(cmd *cobra.Command, _ []string) error {
	_, err := devMCPToolUpsertParams(cmd, false)
	return err
}

func validateDevMCPToolUpdate(cmd *cobra.Command, _ []string) error {
	_, err := devMCPToolUpsertParams(cmd, true)
	return err
}

func validateDevMCPToolDebug(cmd *cobra.Command, _ []string) error {
	if _, err := devMCPToolLocatorParams(cmd); err != nil {
		return err
	}
	if _, err := devMCPRequiredJSONObjectFlag(cmd, "value"); err != nil {
		return err
	}
	credentialID := devMCPIntFlag(cmd, "credential-id")
	if credentialID == 0 && !commandBoolFlag(cmd, "no-credential") {
		return apperrors.NewValidation("调试需指定本次运行时凭证，二选一：--credential-id <id> 或 --no-credential")
	}
	if credentialID != 0 && commandBoolFlag(cmd, "no-credential") {
		return apperrors.NewValidation("--credential-id 与 --no-credential 只能使用一个")
	}
	return nil
}

func validateDevMCPAuthSave(cmd *cobra.Command, _ []string) error {
	if _, err := devMCPRequiredInt(cmd, "mcp-id"); err != nil {
		return err
	}
	authType, err := devMCPRequiredString(cmd, "auth-type")
	if err != nil {
		return err
	}
	if !devMCPValidAuthType(strings.ToUpper(authType)) {
		return apperrors.NewValidation("--auth-type 只支持 NO_AUTH、BASIC、TOKEN 或 SIGNATURE（静态 API key 场景用 SIGNATURE 自定义字段+直引）")
	}
	for _, flag := range []string{"basic-auth-config", "api-secret-auth-config", "token-auth-config", "signature-auth-config"} {
		if value := devMCPStringFlag(cmd, flag); value != "" {
			params := map[string]any{}
			if err := devMCPPutJSONObjectFlag(cmd, params, flag, flag); err != nil {
				return err
			}
		}
	}
	return nil
}

func validateDevMCPCredentialLocator(cmd *cobra.Command, _ []string) error {
	_, err := devMCPCredentialLocatorParams(cmd)
	return err
}

func validateDevMCPCredentialSave(cmd *cobra.Command, _ []string) error {
	if _, err := devMCPRequiredInt(cmd, "mcp-id"); err != nil {
		return err
	}
	if _, err := devMCPRequiredString(cmd, "name"); err != nil {
		return err
	}
	content := devMCPStringFlag(cmd, "content")
	path := devMCPStringFlag(cmd, "content-file")
	if content != "" && path != "" {
		return apperrors.NewValidation("--content 与 --content-file 只能使用一个")
	}
	if content == "" && path == "" {
		return apperrors.NewValidation("--content 或 --content-file 为必填")
	}
	if path == "-" {
		return nil
	}
	_, err := devMCPCredentialContent(cmd)
	return err
}

func validateDevMCPMemberMutation(cmd *cobra.Command, _ []string) error {
	if _, err := devMCPRequiredInt(cmd, "mcp-id"); err != nil {
		return err
	}
	if len(splitDevAppList(devMCPStringFlag(cmd, "user-ids"))) == 0 {
		return apperrors.NewValidation("--user-ids 至少包含一个成员 staffId")
	}
	return nil
}

func validateDevMCPHsfCreate(cmd *cobra.Command, _ []string) error {
	_, err := devMCPToolHsfCreateParams(cmd)
	return err
}

func validateDevMCPHsfUpdate(cmd *cobra.Command, _ []string) error {
	_, err := devMCPToolHsfUpdateParams(cmd)
	return err
}
