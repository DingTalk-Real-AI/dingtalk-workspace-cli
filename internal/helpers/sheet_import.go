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

import "github.com/spf13/cobra"

const sheetImportLong = `将本地表格文件导入为一个新的钉钉在线电子表格，与 dws sheet export（导出）对称。

支持的文件格式 (按扩展名):
  xlsx, xls   Microsoft Excel 表格

文件大小限制: 20MB
固定导入为电子表格类型；只新建文档，不操作已有表格。

CLI 内部自动完成全部流程:
  1. 创建导入会话（获取 OSS 上传凭证）
  2. 上传文件到 OSS
  3. 确认导入（触发格式转换）
  4. 渐进式退避轮询等待完成（最多约 5 分钟）

如果轮询超时仍未完成，会输出 taskId 供后续手动查询:
  dws sheet import get --task-id <taskId>`

func newSheetImportCmd() *cobra.Command {
	return newSheetImportCmdWithConfig(sheetImportFlowConfig())
}

func newSheetImportCmdWithConfig(cfg importFlowConfig) *cobra.Command {
	importCmd := &cobra.Command{
		Use:   "import",
		Short: "导入本地表格文件为在线电子表格 (xlsx / xls)",
		Long:  sheetImportLong,
		Example: `  # 导入 xlsx 为在线电子表格（默认表格名取文件名）
  dws sheet import --file ./quote.xlsx --folder-token <FOLDER_TOKEN>

  # 指定目标文件夹与导入后表格名称
  dws sheet import --file ./report.xls --folder-token <FOLDER_TOKEN> --name "月度报表"

  # 导入到指定知识库
  dws sheet import --file ./data.xls --workspace <WORKSPACE_ID>`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runImportCommand(cmd, args, cfg)
		},
	}
	addSheetImportFlags(importCmd)

	// Schema deliberately binds only runnable leaves. Keep the historical
	// runnable parent for CLI compatibility, and expose the same action through
	// a leaf so agents can discover and invoke the import operation.
	importCreateCmd := &cobra.Command{
		Use:   "create",
		Short: "导入本地表格文件为在线电子表格 (xlsx / xls)",
		Long:  sheetImportLong,
		Example: `  dws sheet import create --file ./quote.xlsx --folder-token <FOLDER_TOKEN>
  dws sheet import create --file ./data.xls --workspace <WORKSPACE_ID> --name "月度报表"`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runImportCommand(cmd, args, cfg)
		},
	}
	addSheetImportFlags(importCreateCmd)

	importGetCmd := &cobra.Command{
		Use:   "get",
		Short: "查询表格导入任务结果（手动兜底）",
		Long: `根据 taskId 查询表格导入任务的执行结果。
通常不需要手动调用，dws sheet import 会自动完成轮询。
仅在导入命令超时或中断后，用于手动查询任务状态。

任务状态:
  processing  转换中
  completed   导入成功，返回 documentUrl
  failed      导入失败`,
		Example: `  dws sheet import get --task-id <TASK_ID>`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runImportGetCommand(cmd, cfg)
		},
	}
	importGetCmd.Flags().String("task-id", "", "导入任务 ID (必填)")
	importCmd.AddCommand(importCreateCmd, importGetCmd)
	return importCmd
}

func addSheetImportFlags(cmd *cobra.Command) {
	cmd.Flags().String("file", "", "本地表格文件路径 (必填，支持 xlsx/xls)")
	cmd.Flags().String("folder-token", "", "目标文件夹 ID 或 URL (与 --workspace 至少传一个)")
	cmd.Flags().String("workspace", "", "目标知识库 ID 或 URL (与 --folder-token 至少传一个)")
	cmd.Flags().StringP("name", "n", "", "导入后表格名称 (可选，默认取文件名)")
	cmd.Flags().String("folder", "", "")
	_ = cmd.Flags().MarkHidden("folder")
}
