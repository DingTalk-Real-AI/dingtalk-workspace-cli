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
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/spf13/cobra"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/executor"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
)

// V5 契约（2026-07-21）：hsf 型工具三件套 + 凭证解绑。
// hsf 与 http 的关键差异：apiInputs/apiOutputs 无需提供（服务端按 HSF 方法
// schema 自动生成）；映射 target 为 $.<DTO简名>.<字段>（非 Pascal 位置名）、
// 出参 source 无 .Body. 前缀——因此不走 http 侧的映射静态互验（lint）。

func newDevMCPHsfMethodListCommand(runner executor.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:               "method-list",
		Short:             "查询 HSF 接口的方法清单（建 hsf 工具前的方法发现，含每方法出入参 schema）",
		Example:           "  dws dev mcp hsf method-list --interface-name com.dingtalk.open.connect.workbench.api.service.hsf.MCPHsfService --format json",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			interfaceName, err := devMCPRequiredString(cmd, "interface-name")
			if err != nil {
				return err
			}
			params := map[string]any{"interfaceName": interfaceName}
			devMCPPutString(params, "version", devMCPStringFlag(cmd, "version"))
			return runDevMCPTool(runner, cmd, devMCPHsfMethodListTool, params)
		},
	}
	cmd.Flags().String("interface-name", "", "必填。HSF 接口全限定名（不能只传简名），如 com.dingtalk.open.connect.workbench.api.service.hsf.MCPHsfService")
	cmd.Flags().String("version", "", "可选。HSF 服务版本号，缺省 1.0.0")
	preferLegacyLeaf(cmd)
	annotateDevMCPTool(cmd, devMCPHsfMethodListTool)
	DeclareLeafMetadata(cmd, LeafSpec{
		OutputRollout: output.RolloutUnifiedActive,
		Safety:        devMCPReadSafety(),
		Contract:      devMCPContract(cmd, devMCPHsfMethodListTool, "dev mcp hsf method-list", "查询 HSF 接口方法和参数 Schema", false),
	})
	return cmd
}

func newDevMCPToolCreateHsfCommand(runner executor.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:               "create-hsf",
		Short:             "新建 HSF 型 MCP 工具草稿（apiInputs/apiOutputs 由服务端按方法 schema 自动生成）",
		Example:           `  dws dev mcp tool create-hsf --mcp-id 10520 --name search_mcp_services --title 搜索MCP服务 --description 按名称关键词搜索MCP服务 --hsf-info '{"interfaceName":"com.dingtalk.open.connect.workbench.api.service.hsf.MCPHsfService","methodName":"searchMCPs","version":"1.0.0"}' --tool-inputs '[]' --input-mappings '[]' --tool-outputs '[]' --output-mappings '[]' --dry-run --format json`,
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			params, err := devMCPToolHsfCreateParams(cmd)
			if err != nil {
				return err
			}
			return runDevMCPTool(runner, cmd, devMCPToolCreateHsfTool, params)
		},
	}
	addDevMCPMCPIDFlag(cmd)
	cmd.Flags().String("name", "", "必填。工具唯一标识，snake_case、动词开头、语义明确")
	cmd.Flags().String("title", "", "必填。工具中文标题")
	cmd.Flags().String("description", "", "必填。工具功能完整描述（何时用+返回什么）")
	cmd.Flags().String("hsf-info", "", "必填。HSF 三元组 JSON 对象 {interfaceName,methodName,version(可省，缺省1.0.0)}——先用 hsf method-list 发现方法；与 http 版的 --http-info 对仗")
	cmd.Flags().String("tool-inputs", "", "必填。暴露给 LLM 的入参字段树 JSON 数组（与 http 版同构）")
	cmd.Flags().String("input-mappings", "", "必填。toolInputs→DTO 映射 JSON 数组；target=$.<DTO简名>.<字段>（DTO 字段名以 hsf method-list 的 inputSchema 为准，写错=静默忽略）；⚠️DTO 含 corpId/userId 时必须显式加两条系统注入映射（$.system_node.ddDataCorpId / $.system_node.operateUserId），服务端不自动补，漏写=运行时参数错误")
	cmd.Flags().String("tool-outputs", "", "必填。对外出参字段树 JSON 数组；不做精修显式传 '[]'")
	cmd.Flags().String("output-mappings", "", "必填。出参映射 JSON 数组；⚠️HSF 侧 source 无 .Body. 前缀：$.node_service_activator.result…（写操作类最简=只透 success/errorCode/errorMsg 三条）")
	addDevMCPTimeoutOOKFlags(cmd, false)
	preferLegacyLeaf(cmd)
	annotateDevMCPTool(cmd, devMCPToolCreateHsfTool)
	DeclareLeafMetadata(cmd, LeafSpec{
		OutputRollout: output.RolloutUnifiedActive,
		Safety:        devMCPWriteSafety(),
		Validate:      validateDevMCPHsfCreate,
		Contract:      devMCPContract(cmd, devMCPToolCreateHsfTool, "dev mcp tool create-hsf", "创建 HSF 类型的 MCP 工具草稿", true),
	})
	return cmd
}

func newDevMCPToolUpdateHsfCommand(runner executor.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:               "update-hsf",
		Short:             "编辑 HSF 型工具（⚠️部分更新语义：只传要改的字段，未传保持原值——与 http 版全量提交完全相反）",
		Example:           "  dws dev mcp tool update-hsf --mcp-id 10520 --tool-id G-ACT-xxx --description 更准确的新描述 --dry-run --format json",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			params, err := devMCPToolHsfUpdateParams(cmd)
			if err != nil {
				return err
			}
			return runDevMCPTool(runner, cmd, devMCPToolUpdateHsfTool, params)
		},
	}
	addDevMCPToolLocatorFlags(cmd)
	cmd.Flags().String("name", "", "可选。改工具标识名（snake_case）；不传=保持原值")
	cmd.Flags().String("title", "", "可选。改标题；不传=保持原值")
	cmd.Flags().String("description", "", "可选。改工具描述；不传=保持原值")
	cmd.Flags().String("hsf-info", "", "可选。改 HSF 三元组 JSON 对象；⚠️interfaceName 与 methodName 必须同时给才会切换方法，只给其一会被静默忽略（照样返回 success 但方法没换）；不传=保持原值")
	cmd.Flags().String("tool-inputs", "", "可选。改入参字段树；不传=保持原值，传了=整块替换")
	cmd.Flags().String("input-mappings", "", "可选。改入参映射；不传=保持原值，传了=整块替换（target=$.<DTO简名>.<字段>，字段名以 hsf method-list 为准）")
	cmd.Flags().String("tool-outputs", "", "可选。改出参字段树；不传=保持原值，传了=整块替换")
	cmd.Flags().String("output-mappings", "", "可选。改出参映射；不传=保持原值，传数组=整块替换；⚠️传 []=清空既有映射导致工具出参失效，勿用 [] 表达「不改」")
	addDevMCPTimeoutOOKFlags(cmd, true)
	preferLegacyLeaf(cmd)
	annotateDevMCPTool(cmd, devMCPToolUpdateHsfTool)
	DeclareLeafMetadata(cmd, LeafSpec{
		OutputRollout: output.RolloutUnifiedActive,
		Safety:        devMCPWriteSafety(),
		Validate:      validateDevMCPHsfUpdate,
		Contract:      devMCPContract(cmd, devMCPToolUpdateHsfTool, "dev mcp tool update-hsf", "更新 HSF 类型的 MCP 工具草稿", true),
	})
	return cmd
}

func devMCPToolHsfCreateParams(cmd *cobra.Command) (map[string]any, error) {
	mcpID, err := devMCPRequiredInt(cmd, "mcp-id")
	if err != nil {
		return nil, err
	}
	name, err := devMCPRequiredString(cmd, "name")
	if err != nil {
		return nil, err
	}
	params := map[string]any{"mcpId": mcpID, "name": name}
	if err := devMCPValidateToolName(name); err != nil {
		return nil, err
	}
	hsfInfo, err := devMCPRequiredJSONObjectFlag(cmd, "hsf-info")
	if err != nil {
		return nil, err
	}
	params["hsfInfo"] = hsfInfo
	devMCPPutString(params, "title", devMCPStringFlag(cmd, "title"))
	devMCPPutString(params, "description", devMCPStringFlag(cmd, "description"))
	for _, f := range []struct{ flag, key string }{
		{"tool-inputs", "toolInputs"},
		{"input-mappings", "inputMappings"},
		{"tool-outputs", "toolOutputs"},
		{"output-mappings", "outputMappings"},
	} {
		if err := devMCPPutJSONArrayFlag(cmd, params, f.flag, f.key); err != nil {
			return nil, err
		}
		if _, ok := params[f.key]; !ok {
			return nil, apperrors.NewValidation("--" + f.flag + " 为必填（hsf 建造四件套；--tool-outputs 不精修显式传 '[]'；apiInputs/apiOutputs 无需提供，服务端按 HSF 方法 schema 自动生成）")
		}
	}
	if fields, ok := params["toolInputs"].([]any); ok {
		if err := devMCPValidateFieldsFlag("tool-inputs", fields, true); err != nil {
			return nil, err
		}
	}
	if fields, ok := params["toolOutputs"].([]any); ok {
		if err := devMCPValidateFieldsFlag("tool-outputs", fields, true); err != nil {
			return nil, err
		}
	}
	if mappings, ok := params["inputMappings"].([]any); ok {
		if err := devMCPValidateMappingsFlag("input-mappings", mappings); err != nil {
			return nil, err
		}
	}
	if mappings, ok := params["outputMappings"].([]any); ok {
		if err := devMCPValidateMappingsFlag("output-mappings", mappings); err != nil {
			return nil, err
		}
	}
	if err := devMCPPutTimeoutOOK(cmd, params); err != nil {
		return nil, err
	}
	return params, nil
}

func devMCPToolHsfUpdateParams(cmd *cobra.Command) (map[string]any, error) {
	params, err := devMCPToolLocatorParams(cmd)
	if err != nil {
		return nil, err
	}
	if name := devMCPStringFlag(cmd, "name"); name != "" {
		if err := devMCPValidateToolName(name); err != nil {
			return nil, err
		}
		params["name"] = name
	}
	devMCPPutString(params, "title", devMCPStringFlag(cmd, "title"))
	devMCPPutString(params, "description", devMCPStringFlag(cmd, "description"))
	if err := devMCPPutJSONObjectFlag(cmd, params, "hsf-info", "hsfInfo"); err != nil {
		return nil, err
	}
	for _, f := range []struct{ flag, key string }{
		{"tool-inputs", "toolInputs"},
		{"input-mappings", "inputMappings"},
		{"tool-outputs", "toolOutputs"},
		{"output-mappings", "outputMappings"},
	} {
		if err := devMCPPutJSONArrayFlag(cmd, params, f.flag, f.key); err != nil {
			return nil, err
		}
	}
	if fields, ok := params["toolInputs"].([]any); ok {
		if err := devMCPValidateFieldsFlag("tool-inputs", fields, true); err != nil {
			return nil, err
		}
	}
	if fields, ok := params["toolOutputs"].([]any); ok {
		if err := devMCPValidateFieldsFlag("tool-outputs", fields, true); err != nil {
			return nil, err
		}
	}
	if mappings, ok := params["inputMappings"].([]any); ok {
		if err := devMCPValidateMappingsFlag("input-mappings", mappings); err != nil {
			return nil, err
		}
	}
	if mappings, ok := params["outputMappings"].([]any); ok {
		if err := devMCPValidateMappingsFlag("output-mappings", mappings); err != nil {
			return nil, err
		}
	}
	if err := devMCPPutTimeoutOOK(cmd, params); err != nil {
		return nil, err
	}
	if len(params) == 2 {
		return nil, apperrors.NewValidation("至少提供一项待更新字段（部分更新语义：只传要改的，未传保持原值）")
	}
	return params, nil
}

func newDevMCPCredentialUnbindCommand(runner executor.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:               "unbind",
		Short:             "解绑发布实例的生效凭证（bind 的逆操作；credential delete 报 credential_in_use 时先走本命令）",
		Example:           "  dws dev mcp credential unbind --mcp-id 10520 --dry-run --format json",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			mcpID, err := devMCPRequiredInt(cmd, "mcp-id")
			if err != nil {
				return err
			}
			return runDevMCPTool(runner, cmd, devMCPCredentialUnbindTool, map[string]any{"mcpId": mcpID})
		},
	}
	addDevMCPMCPIDFlag(cmd)
	preferLegacyLeaf(cmd)
	annotateDevMCPTool(cmd, devMCPCredentialUnbindTool)
	DeclareLeafMetadata(cmd, LeafSpec{
		OutputRollout: output.RolloutUnifiedActive,
		Safety:        devMCPWriteSafety(),
		Validate:      validateDevMCPRequiredIntFlag("mcp-id"),
		Contract:      devMCPContract(cmd, devMCPCredentialUnbindTool, "dev mcp credential unbind", "解绑 MCP 发布实例当前使用的凭证账号", true),
	})
	return cmd
}
