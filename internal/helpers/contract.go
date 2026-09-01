// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package helpers

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/cmdutil"
	"github.com/spf13/cobra"
)

// ──────────────────────────────────────────────────────────
// dws contract — 智能合同（vendor extension，Agent Schema 内暴露）
//
// ProductDecl 在 newContractCommand 顶部注册，产品 ID 为 "contract"；
// 每个叶子必须由 declareContractSchema 声明 ContractDecl（Identity /
// Description / Interface / Selection / Parameters / Safety），否则装配
// 会拒绝该叶子。非叶子节点由 newGroupCommand 提供 GroupPolicy。
//
// MCP: queryContracts, createContract, draft_contract_by_minutes,
//      queryContractDetails, queryContractQuantityByType,
//      batchImportContractAsync, getBatchImportContractResult,
//      queryContractProcessContent, getAllFileDirectory,
//      queryContractReviewBenefit, createContractReviewTask,
//      contractAnalysis, queryContractReviewResult
//      createAccountInfo, updateAccountInfo, getAccountEntryInfo,
//      listAccountInfo, deleteAccountEntryInfo, contractOpenArchive
//      addProject, deleteProject, updateProject, setProjectStatus,
//      queryProjects, queryProjectDigests, queryProjectDetail,
//      exportProject, getImportProjectTemplate, importProject, getImportProjectResult,
//      addSubject, querySubjects, querySubjectDetail, updateSubject,
//      deleteSubject, batchDeleteSubject, sortSubjects,
//      detectSubjectRisk, querySubjectBaseInfo, autoFillSubjectInfo,
//      exportSubject, getImportSubjectTemplate, importSubject, getImportSubjectResult
// ──────────────────────────────────────────────────────────

func newContractCommand() *cobra.Command {
	contract.RegisterProductDecl(contract.ProductDecl{
		ID: "contract",
		HelpReferences: contract.HelpReferences{
			RelatedSkills: []string{"dingtalk-contract"},
			Documentation: []contract.HelpDocumentation{
				contract.SkillDocumentation("智能合同命令参考", "dingtalk-contract", "references/contract.md"),
			},
		},
		Selection: contract.ProductSelectionDecl{
			AgentSummary: "钉钉智能合同：台账查询、批量导入、听记+模版起草、审查、归档、项目、相对方、账款管理",
			UseWhen: []string{
				"用户要查合同台账列表、详情或状态统计，创建、批量导入、归档合同",
				"用户要按 AI 听记和模版起草合同",
				"用户要发起合同审查（权益、解析、任务、结果）",
				"用户要管理合同项目、相对方、工商信息或收付款账款",
			},
			AvoidWhen: []string{
				"通用钉盘文件查找、上传、下载走 drive；合同文件的钉盘元数据也从 drive 取",
				"AI 听记内容查询走 minutes；仅从听记获取 taskUuid 后回本产品调用 draft",
				"OA 审批实例的查询、同意、拒绝、转交、撤销走 misc，不要与 process-templates 混淆",
			},
		},
	})
	root := newGroupCommand(&cobra.Command{
		Use:   "contract",
		Short: "智能合同管理",
		Long:  `智能合同：台账查询/详情/分类统计、批量导入、审批模板与台账分类、听记+模版起草、合同审查（权益、任务、解析、结果）、项目管理、相对方管理。`,
		RunE:  groupRunE,
	})

	// ── record ────────────────────────────────────────────────

	recordCmd := newGroupCommand(&cobra.Command{Use: "record", Short: "合同记录", RunE: groupRunE})

	recordListCmd := &cobra.Command{
		Use:   "list",
		Short: "查询合同列表",
		Long: `按合同创建时间范围与状态筛选（与 MCP queryContracts 入参一致）。
创建时间范围使用 --start / --end，须为 ISO-8601 字符串（如 2026-03-10T14:00:00+08:00）；CLI 会换算为 MCP 所需的毫秒时间戳。禁止使用毫秒时间戳作为 CLI 入参。
合同状态传英文枚举，可多选：approving, signing, canceled, withdraw, refused, not-archive, archive-confirming, archived。

--type 表示台账查询维度（与 MCP queryContracts 的 type 一致），取值：
  self              我的
  participation     我参与的
  department        我部门的
  all               全部（默认）
  unassigned        待分配的`,
		Example: `  dws contract record list --format json
  dws contract record list --start "2026-03-10T00:00:00+08:00" --end "2026-03-11T23:59:59+08:00" --status approving,signing --format json
  dws contract record list --type participation --format json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			req := map[string]any{}
			if err := appendContractCreateTimeFromISO(req, cmd, "start", "createStartTime"); err != nil {
				return err
			}
			if err := appendContractCreateTimeFromISO(req, cmd, "end", "createEndTime"); err != nil {
				return err
			}
			if cs, ok := req["createStartTime"].(int64); ok {
				if ce, ok := req["createEndTime"].(int64); ok {
					if err := cmdutil.ValidateTimeRange(cs, ce); err != nil {
						return err
					}
				}
			}
			if raw, _ := cmd.Flags().GetString("status"); strings.TrimSpace(raw) != "" {
				if statuses := parseCSVValues(raw); len(statuses) > 0 {
					req["contractStatusList"] = statuses
				}
			}
			rawType, _ := cmd.Flags().GetString("type")
			scope, err := parseContractRecordQueryScope(rawType)
			if err != nil {
				return err
			}
			req["type"] = scope
			return callMCPToolOnServer("contract", "queryContracts", req)
		},
	}

	recordGetCmd := &cobra.Command{
		Use:     "get",
		Aliases: []string{"detail"},
		Short:   "查询合同详情",
		Long: `按合同 ID 查询单份合同详情（MCP queryContractDetails）。
必填：--contract-id（与台账列表/详情页中的合同主键一致，传至 MCP 的 contractId）。`,
		Example: `  dws contract record get --contract-id "c_xxx" --format json
  dws contract record detail --contract-id "c_xxx" --format json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			id := strings.TrimSpace(MustGetStringFlag(cmd, "contract-id"))
			if id == "" {
				return fmt.Errorf("--contract-id 为必填参数")
			}
			return callMCPToolOnServer("contract", "queryContractDetails", map[string]any{
				"contractId": id,
			})
		},
	}

	recordQuantityByTypeCmd := &cobra.Command{
		Use:   "quantity-by-type",
		Short: "按查询维度统计各状态合同数量",
		Long: `按查询维度返回各合同状态下的台账条数（MCP queryContractQuantityByType）。

--type 与 record list 相同，表示台账查询维度（与 queryContracts / queryContractQuantityByType 的 type 一致）：
  self, participation, department, all（默认）, unassigned`,
		Example: `  dws contract record quantity-by-type --format json
  dws contract record quantity-by-type --type department --format json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			rawType, _ := cmd.Flags().GetString("type")
			scope, err := parseContractRecordQueryScope(rawType)
			if err != nil {
				return err
			}
			return callMCPToolOnServer("contract", "queryContractQuantityByType", map[string]any{
				"type": scope,
			})
		},
	}

	recordCreateCmd := &cobra.Command{
		Use:   "create",
		Short: "创建合同台账",
		Long: `将合同文件与关键信息写入台账（MCP createContract）。
JSON 须符合 ImportContractInfoRequest 结构；必填字段：contentFiles, name, effectiveStatus, signStatus, ownerDeptNo。

【枚举值说明】
effectiveStatus（履约状态）: not-effective(未生效), pre-effective(待生效), effective(生效中), expired(已到期), ineffective(已完结), canceled(已作废)
signStatus（签署状态）: signing(签订中), not-archive(待归档), archived(已归档)
amountType（金额类型）: payment_party_other(收入), payment_party_our(支出), none(无金额)
signType（签署方式）: entity_seal(纸质签署), electronic_seal(电子签署)
termType（期限类型）: accurate_end_date(固定期限), perform_finished(无固定期限)
sealTypes（印章类型）: contract_seal(合同章), common_seal(公章), legal_seal(法人章)`,
		Example: `  dws contract record create --file ./contract.json --format json
  cat contract.json | dws contract record create --file - --format json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			payload, err := readContractJSONPayload(cmd)
			if err != nil {
				return err
			}
			return callMCPToolOnServer("contract", "createContract", map[string]any{
				"ImportContractInfoRequest": payload,
			})
		},
	}

	// ── import（批量导入）──────────────────────────────────────

	importCmd := newGroupCommand(&cobra.Command{
		Use:   "import",
		Short: "批量导入合同",
		Long:  `批量导入：按钉盘模版 fileId+spaceId 创建异步任务、按任务 ID 查结果（MCP batchImportContractAsync / getBatchImportContractResult）。`,
		RunE:  groupRunE,
	})

	importBatchCmd := &cobra.Command{
		Use:   "batch",
		Short: "从钉盘模版文件创建批量导入任务",
		Long: `基于钉盘中的合同批量导入模版文件创建异步导入任务（MCP batchImportContractAsync）。
必填：模版文件在钉盘中的 fileId、以及该文件所在空间的 spaceId；入参仅这两项，对应 MCP 的 fileId、spaceId。`,
		Example: `  dws contract import batch --file-id "123456" --space-id "7890" --format json
  dws contract import batch --file-id "123456" -s "7890" --format json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			fileID := strings.TrimSpace(MustGetStringFlag(cmd, "file-id"))
			spaceID := strings.TrimSpace(MustGetStringFlag(cmd, "space-id"))
			if fileID == "" {
				return fmt.Errorf("--file-id 为必填（钉盘模版文件的 fileId）")
			}
			if spaceID == "" {
				return fmt.Errorf("--space-id 为必填（模版文件所在钉盘空间的 spaceId）")
			}
			return callMCPToolOnServer("contract", "batchImportContractAsync", map[string]any{
				"fileId":  fileID,
				"spaceId": spaceID,
			})
		},
	}

	importBatchResultCmd := &cobra.Command{
		Use:   "batch-result",
		Short: "获取批量合同导入任务结果",
		Long: `按任务 ID 查询批量导入执行结果（MCP getBatchImportContractResult）。
必填：--task-id（创建批量导入任务接口返回的任务 ID，对应 MCP taskId）。`,
		Example: `  dws contract import batch-result --task-id "task_xxx" --format json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			taskID := strings.TrimSpace(MustGetStringFlag(cmd, "task-id"))
			if taskID == "" {
				return fmt.Errorf("--task-id 为必填参数")
			}
			return callMCPToolOnServer("contract", "getBatchImportContractResult", map[string]any{
				"taskId": taskID,
			})
		},
	}

	// ── process-templates / file-directories（元数据）──────────

	processTemplatesCmd := &cobra.Command{
		Use:   "process-templates",
		Short: "查询当前用户可见审批模板",
		Long: `列出当前登录用户可见的合同审批模板/流程内容（MCP queryContractProcessContent）。
无额外必填参数，与 MCP 空对象或默认入参一致。`,
		Example: `  dws contract process-templates --format json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return callMCPToolOnServer("contract", "queryContractProcessContent", map[string]any{})
		},
	}

	fileDirectoriesCmd := &cobra.Command{
		Use:     "file-directories",
		Aliases: []string{"directories"},
		Short:   "查询所有合同台账分类",
		Long: `返回全部合同台账目录/分类（MCP getAllFileDirectory）。
无额外必填参数，与 MCP 空对象或默认入参一致。`,
		Example: `  dws contract file-directories --format json
  dws contract directories --format json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return callMCPToolOnServer("contract", "getAllFileDirectory", map[string]any{})
		},
	}

	// ── draft ─────────────────────────────────────────────────

	draftCmd := &cobra.Command{
		Use:   "draft",
		Short: "根据听记和模版起草合同",
		Long: `根据 AI 听记任务与合同模版起草合同（MCP 与 tools/list 一致）。
听记 id 取自 bizType 为 flashMinutes 的 fileUri 或 id；支持多听记合并。

必填：--task-uuids。
模版（二选一，至少一项，对应 MCP templateUrl / templateContent）：
  --template-url   模版文件 URL
  --template-content   模版全文`,
		Example: `  dws contract draft --task-uuids uuid1,uuid2 --template-url "https://..." --format json
  dws contract draft --task-uuids uuid1 --template-content "$(cat 模版.txt)" --format json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			rawUuids := strings.TrimSpace(MustGetStringFlag(cmd, "task-uuids"))
			if rawUuids == "" {
				return fmt.Errorf("--task-uuids 为必填参数")
			}
			taskUuids := parseCSVValues(rawUuids)
			if len(taskUuids) == 0 {
				return fmt.Errorf("--task-uuids 为必填参数")
			}

			templateURL := strings.TrimSpace(MustGetStringFlag(cmd, "template-url"))
			templateContent := strings.TrimSpace(MustGetStringFlag(cmd, "template-content"))
			if templateURL == "" && templateContent == "" {
				return fmt.Errorf("缺少合同模版：请指定 --template-url（MCP templateUrl）或 --template-content（MCP templateContent），至少一项")
			}

			toolArgs := map[string]any{"taskUuids": taskUuids}
			if templateURL != "" {
				toolArgs["templateUrl"] = templateURL
			}
			if templateContent != "" {
				toolArgs["templateContent"] = templateContent
			}

			return callMCPToolOnServer("contract", "draft_contract_by_minutes", toolArgs)
		},
	}

	// ── review ────────────────────────────────────────────────

	reviewCmd := newGroupCommand(&cobra.Command{
		Use:   "review",
		Short: "合同审查",
		Long:  `合同审查相关操作：权益查询、创建审查任务、解析合同文件、查询审查结果。`,
		RunE:  groupRunE,
	})

	reviewBenefitCmd := &cobra.Command{
		Use:     "benefit",
		Short:   "查询合同审查权益",
		Long:    `查询用户组织的合同审查的权益数据（MCP queryContractReviewBenefit）。`,
		Example: `  dws contract review benefit --format json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return callMCPToolOnServer("contract", "queryContractReviewBenefit", map[string]any{})
		},
	}

	reviewCreateCmd := &cobra.Command{
		Use:   "create",
		Short: "创建合同审查任务",
		Long: `创建合同审查任务（MCP createContractReviewTask）。
JSON 须符合 IntelligentContractReviewClientRequest 结构。

【字段说明】
source              来源标识（字符串，可选）
fileInfo            文件信息对象（可选，与 fileId/spaceId 方式二选一）
  fileId            云盘文件 ID
  spaceId           云盘空间 ID
  fileName          文件名（须带扩展名，如 合同.pdf）
  fileSize          文件大小（字节数，整数）
  fileType          文件类型（如 pdf、docx）
reviewType          审查类型标识（如 AI_REVIEW，可选）
companyList         审查方公司列表（数组，可选）
  reviewPosition    审查方在合同中的位置（字符串）
reviewPosition      默认审查位置（字符串，可选）
reviewResultType    审查结果类型（字符串，可选）
customReviewRules   自定义审查规则（字符串，可选）`,
		Example: `  dws contract review create --file ./review_request.json --format json
  cat review_request.json | dws contract review create --file - --format json

示例 review_request.json：
{
  "source": "OPEN_CLAW",
  "fileInfo": {
    "fileId": "xxx",
    "spaceId": "yyy",
    "fileName": "采购合同.pdf",
    "fileSize": "102400",
    "fileType": "pdf"
  },
  "reviewType": "AI_REVIEW",
  "reviewPosition": "甲方",
  "reviewResultType": "standard",
  "companyList": [{"reviewPosition": "乙方"}]
}`,
		RunE: func(cmd *cobra.Command, args []string) error {
			path, _ := cmd.Flags().GetString("file")
			if strings.TrimSpace(path) == "" {
				return fmt.Errorf("--file 为必填（JSON 路径，或 \"-\" 表示 stdin）")
			}
			var r io.Reader
			if path == "-" {
				r = cmd.InOrStdin()
			} else {
				f, err := os.Open(path)
				if err != nil {
					return fmt.Errorf("打开 JSON 文件: %w", err)
				}
				defer f.Close()
				r = f
			}
			b, err := io.ReadAll(r)
			if err != nil {
				return fmt.Errorf("读取 JSON: %w", err)
			}
			var payload map[string]any
			if err := json.Unmarshal(b, &payload); err != nil {
				return fmt.Errorf("JSON 解析失败: %w", err)
			}
			return callMCPToolOnServer("contract", "createContractReviewTask", map[string]any{
				"IntelligentContractReviewClientRequest": payload,
			})
		},
	}

	reviewAnalysisCmd := &cobra.Command{
		Use:   "analysis",
		Short: "解析合同文件",
		Long: `解析合同文件，返回合同摘要和审查推荐模型（MCP contractAnalysis）。
JSON 须包含文件信息，可包括 fileInfo（fileId/spaceId/fileName/fileSize/fileType）或直接传文件字段。

【字段说明】
fileInfo            文件信息对象（可选）
  fileId            云盘文件 ID
  spaceId           云盘空间 ID
  fileName          文件名（须带扩展名，如 合同.pdf）
  fileSize          文件大小（字节数，整数）
  fileType          文件类型（如 pdf、docx）
source              来源标识（字符串，可选）`,
		Example: `  dws contract review analysis --file ./analysis_request.json --format json
  cat analysis_request.json | dws contract review analysis --file - --format json

示例 analysis_request.json：
{
  "fileInfo": {
    "fileId": "xxx",
    "spaceId": "yyy",
    "fileName": "采购合同.pdf",
    "fileSize": "102400",
    "fileType": "pdf"
  }
}`,
		RunE: func(cmd *cobra.Command, args []string) error {
			path, _ := cmd.Flags().GetString("file")
			if strings.TrimSpace(path) == "" {
				return fmt.Errorf("--file 为必填（JSON 路径，或 \"-\" 表示 stdin）")
			}
			var r io.Reader
			if path == "-" {
				r = cmd.InOrStdin()
			} else {
				f, err := os.Open(path)
				if err != nil {
					return fmt.Errorf("打开 JSON 文件: %w", err)
				}
				defer f.Close()
				r = f
			}
			b, err := io.ReadAll(r)
			if err != nil {
				return fmt.Errorf("读取 JSON: %w", err)
			}
			var payload map[string]any
			if err := json.Unmarshal(b, &payload); err != nil {
				return fmt.Errorf("JSON 解析失败: %w", err)
			}
			return callMCPToolOnServer("contract", "contractAnalysis", map[string]any{
				"AnalysisContractApiRequest": payload,
			})
		},
	}

	reviewResultCmd := &cobra.Command{
		Use:   "result",
		Short: "查询合同审查结果",
		Long: `查询合同审查结果（MCP queryContractReviewResult）。
必填：--task-id（审查任务 ID，由 review create 返回）、--review-type（审查类型，如 AI_REVIEW）。
入参包裹在 IntelligentLegalContractReviewClientRequest 下。`,
		Example: `  dws contract review result --task-id "MjIzODAwMkFJX1JFVklFVw==" --review-type AI_REVIEW --format json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			taskID := strings.TrimSpace(MustGetStringFlag(cmd, "task-id"))
			if taskID == "" {
				return fmt.Errorf("--task-id 为必填参数")
			}
			reviewType := strings.TrimSpace(MustGetStringFlag(cmd, "review-type"))
			if reviewType == "" {
				return fmt.Errorf("--review-type 为必填参数")
			}
			return callMCPToolOnServer("contract", "queryContractReviewResult", map[string]any{
				"IntelligentLegalContractReviewClientRequest": map[string]any{
					"taskId":     taskID,
					"reviewType": reviewType,
				},
			})
		},
	}

	// flags
	recordListCmd.Flags().String("start", "", "合同创建时间范围起点（ISO-8601，如 2026-03-10T14:00:00+08:00）")
	recordListCmd.Flags().String("end", "", "合同创建时间范围终点（ISO-8601，须晚于 --start）")
	recordListCmd.Flags().String("status", "", "合同状态，英文枚举，逗号分隔")
	recordListCmd.Flags().String("type", "all", "查询维度: self|participation|department|all|unassigned（默认 all，与 MCP queryContracts 的 type 一致）")

	recordGetCmd.Flags().String("contract-id", "", "合同 ID（必填，对应 MCP queryContractDetails 的 contractId）")

	recordQuantityByTypeCmd.Flags().String("type", "all", "查询维度: self|participation|department|all|unassigned（默认 all，与 MCP queryContractQuantityByType 的 type 一致）")

	recordCreateCmd.Flags().String("file", "", "ImportContractInfoRequest JSON 文件路径，\"-\" 表示 stdin（必填）")

	importBatchCmd.Flags().String("file-id", "", "钉盘批量导入模版文件的 fileId（必填）；勿使用 -f 简写，与全局 --format/-f 冲突")
	importBatchCmd.Flags().StringP("space-id", "s", "", "模版文件所在钉盘空间的 spaceId（必填）")
	importBatchResultCmd.Flags().String("task-id", "", "批量导入任务 ID（必填，MCP getBatchImportContractResult 的 taskId）")
	importCmd.AddCommand(importBatchCmd, importBatchResultCmd)

	draftCmd.Flags().String("task-uuids", "", "听记任务 id 列表，逗号分隔 (必填)")
	draftCmd.Flags().String("template-url", "", "合同模版 URL（与 --template-content 至少填一项；对应 MCP templateUrl）")
	draftCmd.Flags().String("template-content", "", "合同模版全文（与 --template-url 至少填一项；对应 MCP templateContent）")

	reviewCreateCmd.Flags().String("file", "", "IntelligentContractReviewClientRequest JSON 文件路径，\"-\" 表示 stdin（必填）")
	reviewAnalysisCmd.Flags().String("file", "", "contractAnalysis 请求 JSON 文件路径，\"-\" 表示 stdin（必填）")
	reviewResultCmd.Flags().String("task-id", "", "审查任务 ID（必填）")
	reviewResultCmd.Flags().String("review-type", "", "审查类型，如 AI_REVIEW（必填）")

	recordCmd.AddCommand(
		recordListCmd,
		recordGetCmd,
		recordQuantityByTypeCmd,
		recordCreateCmd,
	)
	reviewCmd.AddCommand(reviewBenefitCmd, reviewCreateCmd, reviewAnalysisCmd, reviewResultCmd)

	// ── account ───────────────────────────────────────────────

	accountCmd := newGroupCommand(&cobra.Command{
		Use:   "account",
		Short: "合同账款管理",
		Long:  `合同账款相关操作：创建、更新、查询、列举、删除账款信息。`,
		RunE:  groupRunE,
	})

	accountCreateCmd := &cobra.Command{
		Use:   "create",
		Short: "创建账款信息",
		Long: `创建合同账款信息（MCP createAccountInfo）。
JSON 须符合 CreateContractAccountRequest 结构。

【字段说明】
contractId        合同唯一标识（Long，必填）
amount            账款金额，格式 xxxx.xx，以元为单位（String(32)，必填）
transactionNo     单据号，账款唯一标识，不能重复（String(64)，必填）
executionDate     账款实际入账时间，Unix 时间戳，单位毫秒（Long，必填）
status            账款状态（String(32)，必填）
                  approving: 审批中 / withdraw: 已撤销 / refused: 已拒绝
                  confirming: 确认中 / canceled: 已作废 / finished: 已完成
reimbursementNo   报销单号，待确认长度（String(64)，可选）
currencyCode      币种，不填默认 "CNY"（String(16)，可选）
source            来源
remark            账款备注（String(64)，可选）`,
		Example: `  dws contract account create --file ./account.json --format json
  cat account.json | dws contract account create --file - --format json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			payload, err := readContractJSONPayload(cmd)
			if err != nil {
				return err
			}
			return callMCPToolOnServer("contract", "createAccountInfo", map[string]any{
				"CreateContractAccountRequest": payload,
			})
		},
	}
	accountCreateCmd.Flags().String("file", "", "CreateContractAccountRequest JSON 文件路径，\"-\" 表示 stdin（必填）")

	accountUpdateCmd := &cobra.Command{
		Use:   "update",
		Short: "更新账款信息",
		Long: `更新合同账款信息（MCP updateAccountInfo）。
JSON 须符合 UpdateContractAccountRequest 结构，建议先用 account get 获取原数据后修改再提交。

【字段说明】
accountEntryId    账款条目 ID（数字，必填，指定要更新的账款）
contractId        合同唯一标识（Long，必填）
amount            账款金额，格式 xxxx.xx，以元为单位（String(32)，必填）
transactionNo     单据号，账款唯一标识，不能重复（String(64)，必填）
executionDate     账款实际入账时间，Unix 时间戳，单位毫秒（Long，必填）
status            账款状态（String(32)，必填）
                  approving: 审批中 / withdraw: 已撤销 / refused: 已拒绝
                  confirming: 确认中 / canceled: 已作废 / finished: 已完成
reimbursementNo   报销单号，待确认长度（String(64)，可选）
currencyCode      币种，不填默认 "CNY"（String(16)，可选）
source            来源
remark            账款备注（String(64)，可选）`,
		Example: `  dws contract account update --file ./account_update.json --format json
  cat account_update.json | dws contract account update --file - --format json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			payload, err := readContractJSONPayload(cmd)
			if err != nil {
				return err
			}
			return callMCPToolOnServer("contract", "updateAccountInfo", map[string]any{
				"UpdateContractAccountRequest": payload,
			})
		},
	}
	accountUpdateCmd.Flags().String("file", "", "UpdateContractAccountRequest JSON 文件路径，\"-\" 表示 stdin（必填）")

	accountGetCmd := &cobra.Command{
		Use:     "get",
		Short:   "获取账款信息",
		Long:    `按账款 ID 获取单条账款详情（MCP getAccountEntryInfo）。`,
		Example: `  dws contract account get --account-id 12345 --format json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			accountEntryID, _ := cmd.Flags().GetInt64("account-id")
			if accountEntryID == 0 {
				return fmt.Errorf("--account-id 为必填参数")
			}
			return callMCPToolOnServer("contract", "getAccountEntryInfo", map[string]any{
				"QueryContractAccountEntryRequest": map[string]any{
					"accountEntryId": accountEntryID,
				},
			})
		},
	}
	accountGetCmd.Flags().Int64("account-id", 0, "账款 ID（必填）")

	accountListCmd := &cobra.Command{
		Use:   "list",
		Short: "列举账款信息",
		Long: `按多条件筛选查询账款列表（MCP listAccountInfo）。

【参数说明】
--scope           查询范围: self / department / all
--query-status    收付款状态: all / pay / receive
--amount-type     金额类型: payment_party_other(收入) / payment_party_our(支出) / none(无金额)
--status          账款状态
--source          来源
--contract-code   合同代码
--contract-name   合同名称
--transaction-no  单据号
--exec-start      执行开始时间（毫秒时间戳）
--exec-end        执行结束时间（毫秒时间戳）
--page            当前页码（默认 1）
--page-size       每页条数`,
		Example: `  dws contract account list --scope self --format json
  dws contract account list --scope all --query-status pay --exec-start 1700000000000 --format json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			req := map[string]any{}
			if v, _ := cmd.Flags().GetString("scope"); strings.TrimSpace(v) != "" {
				req["scope"] = strings.TrimSpace(v)
			}
			if v, _ := cmd.Flags().GetString("query-status"); strings.TrimSpace(v) != "" {
				req["queryStatus"] = strings.TrimSpace(v)
			}
			if v, _ := cmd.Flags().GetString("amount-type"); strings.TrimSpace(v) != "" {
				req["amountType"] = strings.TrimSpace(v)
			}
			if v, _ := cmd.Flags().GetString("status"); strings.TrimSpace(v) != "" {
				req["status"] = strings.TrimSpace(v)
			}
			if v, _ := cmd.Flags().GetString("source"); strings.TrimSpace(v) != "" {
				req["source"] = strings.TrimSpace(v)
			}
			if v, _ := cmd.Flags().GetString("contract-code"); strings.TrimSpace(v) != "" {
				req["contractCode"] = strings.TrimSpace(v)
			}
			if v, _ := cmd.Flags().GetString("contract-name"); strings.TrimSpace(v) != "" {
				req["contractName"] = strings.TrimSpace(v)
			}
			if v, _ := cmd.Flags().GetString("transaction-no"); strings.TrimSpace(v) != "" {
				req["transactionNo"] = strings.TrimSpace(v)
			}
			if err := appendContractCreateTimeFromISO(req, cmd, "exec-start", "executionDateBegin"); err != nil {
				return err
			}
			if err := appendContractCreateTimeFromISO(req, cmd, "exec-end", "executionDateEnd"); err != nil {
				return err
			}
			if v, _ := cmd.Flags().GetInt("page"); v > 0 {
				req["currentPage"] = v
			}
			if v, _ := cmd.Flags().GetInt("page-size"); v > 0 {
				req["pageSize"] = v
			}
			return callMCPToolOnServer("contract", "listAccountInfo", map[string]any{
				"QueryContractAccountListRequest": req,
			})
		},
	}
	accountListCmd.Flags().String("scope", "", "查询范围: self / department / all")
	accountListCmd.Flags().String("query-status", "", "收付款状态: all / pay / receive")
	accountListCmd.Flags().String("amount-type", "", "金额类型: payment_party_other / payment_party_our / none")
	accountListCmd.Flags().String("status", "", "账款状态")
	accountListCmd.Flags().String("source", "", "来源")
	accountListCmd.Flags().String("contract-code", "", "合同代码")
	accountListCmd.Flags().String("contract-name", "", "合同名称")
	accountListCmd.Flags().String("transaction-no", "", "单据号")
	accountListCmd.Flags().String("exec-start", "", "执行开始时间（毫秒时间戳）")
	accountListCmd.Flags().String("exec-end", "", "执行结束时间（毫秒时间戳）")
	accountListCmd.Flags().Int("page", 0, "当前页码")
	accountListCmd.Flags().Int("page-size", 0, "每页条数")

	accountDeleteCmd := &cobra.Command{
		Use:     "delete",
		Short:   "删除账款信息",
		Long:    `按账款 ID 删除账款信息（MCP deleteAccountEntryInfo）。`,
		Example: `  dws contract account delete --account-id 12345 --format json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			accountEntryID, _ := cmd.Flags().GetInt64("account-id")
			if accountEntryID == 0 {
				return fmt.Errorf("--account-id 为必填参数")
			}
			return callMCPToolOnServer("contract", "deleteAccountEntryInfo", map[string]any{
				"DeleteContractAccountEntryRequest": map[string]any{
					"accountEntryId": accountEntryID,
				},
			})
		},
	}
	accountDeleteCmd.Flags().Int64("account-id", 0, "账款 ID（必填）")

	accountCmd.AddCommand(accountCreateCmd, accountUpdateCmd, accountGetCmd, accountListCmd, accountDeleteCmd)

	// ── arch ───────────────────────────────────────────────

	archCmd := &cobra.Command{
		Use:   "archive",
		Short: "合同文档归档",
		Long: `对合同进行归档操作（MCP contractOpenArchive）。
必须通过 --file 传入完整 JSON，因 archiveFiles 为必填数组且包含嵌套结构。

JSON 须符合 ContractOpenArchiveRequest 结构：

【字段说明】
bizId             合同唯一标识（必填）
archiveTime       归档时间，Unix 毫秒时间戳（必填）
archiveFiles      归档文件列表（必填，数组）
  spaceId         钉盘空间 ID
  fileId          钉盘文件 ID
  fileName        文件名
  fileType        文件种类
  fileSize        文件大小（数字）
archiveCode       归档编号（可选）
archiveComment    归档备注信息（可选）
fileDirectoryId   归档目录 ID（数字，可选）`,
		Example: `  dws contract archive --file ./archive_request.json --format json
  cat archive_request.json | dws contract archive --file - --format json

示例 archive_request.json：
{
  "bizId": "abc123",
  "archiveTime": 1700000000000,
  "archiveFiles": [
    {
      "spaceId": "xxx",
      "fileId": "yyy",
      "fileName": "合同.pdf",
      "fileType": "pdf",
      "fileSize": 102400
    }
  ],
  "archiveCode": "ARCH-2024-001",
  "archiveComment": "已审核完毕",
  "fileDirectoryId": 10
}`,
		RunE: func(cmd *cobra.Command, args []string) error {
			payload, err := readContractJSONPayload(cmd)
			if err != nil {
				return err
			}
			return callMCPToolOnServer("contract", "contractOpenArchive", map[string]any{
				"ContractOpenArchiveRequest": payload,
			})
		},
	}
	archCmd.Flags().String("file", "", "ContractOpenArchiveRequest JSON 文件路径，\"-\" 表示 stdin（必填）")

	// ── project ───────────────────────────────────────────────

	projectCmd := newGroupCommand(&cobra.Command{Use: "project", Short: "合同项目管理", RunE: groupRunE})

	projectAddCmd := &cobra.Command{
		Use:   "add",
		Short: "新增项目",
		Long: `创建合同项目（MCP addProject）。
必填：--name；可选：--code, --owners, --start-date, --end-date, --remark, --contract-ids, --source。
--start-date / --end-date 须为 ISO-8601 字符串（如 2026-03-10T14:00:00+08:00），CLI 换算为 MCP 毫秒时间戳。禁止将毫秒时间戳作为 CLI 入参。`,
		Example: `  dws contract project add --name "2024采购项目" --format json
  dws contract project add --name "Q1项目" --code "PRJ-001" --owners "staff1,staff2" --start-date "2026-01-01T00:00:00+08:00" --end-date "2026-12-31T23:59:59+08:00" --format json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			name := strings.TrimSpace(MustGetStringFlag(cmd, "name"))
			if name == "" {
				return fmt.Errorf("--name 为必填参数")
			}
			req := map[string]any{"name": name}
			if v := strings.TrimSpace(MustGetStringFlag(cmd, "code")); v != "" {
				req["code"] = v
			}
			if raw := strings.TrimSpace(MustGetStringFlag(cmd, "owners")); raw != "" {
				req["ownerList"] = parseCSVValues(raw)
			}
			if err := appendContractCreateTimeFromISO(req, cmd, "start-date", "startDate"); err != nil {
				return err
			}
			if err := appendContractCreateTimeFromISO(req, cmd, "end-date", "endDate"); err != nil {
				return err
			}
			if v := strings.TrimSpace(MustGetStringFlag(cmd, "remark")); v != "" {
				req["remark"] = v
			}
			if raw := strings.TrimSpace(MustGetStringFlag(cmd, "contract-ids")); raw != "" {
				ids, err := parseContractInt64CSV(raw)
				if err != nil {
					return fmt.Errorf("--contract-ids 须为逗号分隔的整数: %w", err)
				}
				req["contractIds"] = ids
			}
			if v := strings.TrimSpace(MustGetStringFlag(cmd, "source")); v != "" {
				req["source"] = v
			}
			return callMCPToolOnServer("contract", "addProject", map[string]any{
				"AddProjectOpenRequest": req,
			})
		},
	}

	projectDeleteCmd := &cobra.Command{
		Use:   "delete",
		Short: "删除项目（支持批量）",
		Long: `按项目 ID 列表删除项目（MCP deleteProject）。
必填：--project-ids（逗号分隔的项目 ID）。`,
		Example: `  dws contract project delete --project-ids "1001,1002" --format json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			raw := strings.TrimSpace(MustGetStringFlag(cmd, "project-ids"))
			if raw == "" {
				return fmt.Errorf("--project-ids 为必填参数")
			}
			ids, err := parseContractInt64CSV(raw)
			if err != nil {
				return fmt.Errorf("--project-ids 须为逗号分隔的整数: %w", err)
			}
			return callMCPToolOnServer("contract", "deleteProject", map[string]any{
				"DeleteProjectOpenRequest": map[string]any{
					"projectIds": ids,
				},
			})
		},
	}

	projectUpdateCmd := &cobra.Command{
		Use:   "update",
		Short: "更新项目信息",
		Long: `更新已有项目（MCP updateProject）。
必填：--project-id, --name；可选：--code, --owners, --start-date, --end-date, --remark, --contract-ids。
--start-date / --end-date 须为 ISO-8601 字符串（如 2026-03-10T14:00:00+08:00），CLI 换算为 MCP 毫秒时间戳。禁止将毫秒时间戳作为 CLI 入参。`,
		Example: `  dws contract project update --project-id 1001 --name "更新后的名称" --format json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			projectID, err := cmd.Flags().GetInt64("project-id")
			if err != nil || projectID == 0 {
				return fmt.Errorf("--project-id 为必填参数（整数）")
			}
			name := strings.TrimSpace(MustGetStringFlag(cmd, "name"))
			if name == "" {
				return fmt.Errorf("--name 为必填参数")
			}
			req := map[string]any{"projectId": projectID, "name": name}
			if v := strings.TrimSpace(MustGetStringFlag(cmd, "code")); v != "" {
				req["code"] = v
			}
			if raw := strings.TrimSpace(MustGetStringFlag(cmd, "owners")); raw != "" {
				req["ownerList"] = parseCSVValues(raw)
			}
			if err := appendContractCreateTimeFromISO(req, cmd, "start-date", "startDate"); err != nil {
				return err
			}
			if err := appendContractCreateTimeFromISO(req, cmd, "end-date", "endDate"); err != nil {
				return err
			}
			if v := strings.TrimSpace(MustGetStringFlag(cmd, "remark")); v != "" {
				req["remark"] = v
			}
			if raw := strings.TrimSpace(MustGetStringFlag(cmd, "contract-ids")); raw != "" {
				ids, err := parseContractInt64CSV(raw)
				if err != nil {
					return fmt.Errorf("--contract-ids 须为逗号分隔的整数: %w", err)
				}
				req["contractIds"] = ids
			}
			return callMCPToolOnServer("contract", "updateProject", map[string]any{
				"UpdateProjectOpenRequest": req,
			})
		},
	}

	projectSetStatusCmd := &cobra.Command{
		Use:   "set-status",
		Short: "更新项目状态",
		Long: `更新项目状态（MCP setProjectStatus）。
必填：--project-id, --status。`,
		Example: `  dws contract project set-status --project-id 1001 --status "active" --format json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			projectID, err := cmd.Flags().GetInt64("project-id")
			if err != nil || projectID == 0 {
				return fmt.Errorf("--project-id 为必填参数（整数）")
			}
			status := strings.TrimSpace(MustGetStringFlag(cmd, "status"))
			if status == "" {
				return fmt.Errorf("--status 为必填参数")
			}
			return callMCPToolOnServer("contract", "setProjectStatus", map[string]any{
				"UpdateProjectStatusOpenRequest": map[string]any{
					"projectId": projectID,
					"status":    status,
				},
			})
		},
	}

	projectListCmd := &cobra.Command{
		Use:   "list",
		Short: "分页查询项目列表",
		Long: `分页查询合同项目（MCP queryProjects）。
必填：--current-page, --page-size, --scope（self/all）；可选筛选条件。`,
		Example: `  dws contract project list --current-page 1 --page-size 20 --scope all --format json
  dws contract project list --current-page 1 --page-size 10 --scope self --name "采购" --status active --format json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			currentPage, err := cmd.Flags().GetInt64("current-page")
			if err != nil || currentPage == 0 {
				return fmt.Errorf("--current-page 为必填参数（正整数）")
			}
			pageSize, err := cmd.Flags().GetInt64("page-size")
			if err != nil || pageSize == 0 {
				return fmt.Errorf("--page-size 为必填参数（正整数）")
			}
			scope := strings.TrimSpace(MustGetStringFlag(cmd, "scope"))
			if scope == "" {
				return fmt.Errorf("--scope 为必填参数（self/all）")
			}
			req := map[string]any{
				"currentPage": currentPage,
				"pageSize":    pageSize,
				"scope":       scope,
			}
			if v := strings.TrimSpace(MustGetStringFlag(cmd, "name")); v != "" {
				req["name"] = v
			}
			if v := strings.TrimSpace(MustGetStringFlag(cmd, "code")); v != "" {
				req["code"] = v
			}
			if raw := strings.TrimSpace(MustGetStringFlag(cmd, "owners")); raw != "" {
				req["ownerList"] = parseCSVValues(raw)
			}
			if v := strings.TrimSpace(MustGetStringFlag(cmd, "status")); v != "" {
				req["status"] = v
			}
			if err := appendContractCreateTimeFromISO(req, cmd, "start-date-left", "startDateLeft"); err != nil {
				return err
			}
			if err := appendContractCreateTimeFromISO(req, cmd, "start-date-right", "startDateRight"); err != nil {
				return err
			}
			if err := appendContractCreateTimeFromISO(req, cmd, "end-date-left", "endDateLeft"); err != nil {
				return err
			}
			if err := appendContractCreateTimeFromISO(req, cmd, "end-date-right", "endDateRight"); err != nil {
				return err
			}
			return callMCPToolOnServer("contract", "queryProjects", map[string]any{
				"QueryProjectOpenRequest": req,
			})
		},
	}

	projectDigestsCmd := &cobra.Command{
		Use:   "digests",
		Short: "分页查询项目摘要列表",
		Long: `分页查询合同项目摘要（MCP queryProjectDigests）。
参数同 project list。`,
		Example: `  dws contract project digests --current-page 1 --page-size 20 --scope all --format json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			currentPage, err := cmd.Flags().GetInt64("current-page")
			if err != nil || currentPage == 0 {
				return fmt.Errorf("--current-page 为必填参数（正整数）")
			}
			pageSize, err := cmd.Flags().GetInt64("page-size")
			if err != nil || pageSize == 0 {
				return fmt.Errorf("--page-size 为必填参数（正整数）")
			}
			scope := strings.TrimSpace(MustGetStringFlag(cmd, "scope"))
			if scope == "" {
				return fmt.Errorf("--scope 为必填参数（self/all）")
			}
			req := map[string]any{
				"currentPage": currentPage,
				"pageSize":    pageSize,
				"scope":       scope,
			}
			if v := strings.TrimSpace(MustGetStringFlag(cmd, "name")); v != "" {
				req["name"] = v
			}
			if v := strings.TrimSpace(MustGetStringFlag(cmd, "code")); v != "" {
				req["code"] = v
			}
			if raw := strings.TrimSpace(MustGetStringFlag(cmd, "owners")); raw != "" {
				req["ownerList"] = parseCSVValues(raw)
			}
			if v := strings.TrimSpace(MustGetStringFlag(cmd, "status")); v != "" {
				req["status"] = v
			}
			if err := appendContractCreateTimeFromISO(req, cmd, "start-date-left", "startDateLeft"); err != nil {
				return err
			}
			if err := appendContractCreateTimeFromISO(req, cmd, "start-date-right", "startDateRight"); err != nil {
				return err
			}
			if err := appendContractCreateTimeFromISO(req, cmd, "end-date-left", "endDateLeft"); err != nil {
				return err
			}
			if err := appendContractCreateTimeFromISO(req, cmd, "end-date-right", "endDateRight"); err != nil {
				return err
			}
			return callMCPToolOnServer("contract", "queryProjectDigests", map[string]any{
				"QueryProjectOpenRequest": req,
			})
		},
	}

	projectDetailCmd := &cobra.Command{
		Use:   "detail",
		Short: "查询项目详情",
		Long: `按项目 ID 查询详情（MCP queryProjectDetail）。
必填：--project-id。`,
		Example: `  dws contract project detail --project-id 1001 --format json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			projectID, err := cmd.Flags().GetInt64("project-id")
			if err != nil || projectID == 0 {
				return fmt.Errorf("--project-id 为必填参数（整数）")
			}
			return callMCPToolOnServer("contract", "queryProjectDetail", map[string]any{
				"QueryProjectDetailOpenRequest": map[string]any{
					"projectId": projectID,
				},
			})
		},
	}

	projectExportCmd := &cobra.Command{
		Use:   "export",
		Short: "项目导出到 Excel",
		Long: `导出指定项目到 Excel（MCP exportProject）。
必填：--project-ids（逗号分隔）；可选：--process-code。`,
		Example: `  dws contract project export --project-ids "1001,1002" --format json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			raw := strings.TrimSpace(MustGetStringFlag(cmd, "project-ids"))
			if raw == "" {
				return fmt.Errorf("--project-ids 为必填参数")
			}
			ids, err := parseContractInt64CSV(raw)
			if err != nil {
				return fmt.Errorf("--project-ids 须为逗号分隔的整数: %w", err)
			}
			req := map[string]any{"projectIds": ids}
			if v := strings.TrimSpace(MustGetStringFlag(cmd, "process-code")); v != "" {
				req["processCode"] = v
			}
			return callMCPToolOnServer("contract", "exportProject", map[string]any{
				"ExportProjectOpenRequest": req,
			})
		},
	}

	projectImportTemplateCmd := &cobra.Command{
		Use:     "import-template",
		Short:   "获取批量导入项目模板",
		Long:    `获取项目批量导入的 Excel 模板下载链接（MCP getImportProjectTemplate）。`,
		Example: `  dws contract project import-template --format json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return callMCPToolOnServer("contract", "getImportProjectTemplate", map[string]any{
				"BaseProjectOpenRequest": map[string]any{},
			})
		},
	}

	projectImportCmd := &cobra.Command{
		Use:   "import",
		Short: "批量导入项目",
		Long: `从钉盘文件批量导入项目（MCP importProject）。
必填：--file-id；可选：--space-id, --file-name, --file-type, --file-size。`,
		Example: `  dws contract project import --file-id "abc123" --space-id 7890 --format json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			fileID := strings.TrimSpace(MustGetStringFlag(cmd, "file-id"))
			if fileID == "" {
				return fmt.Errorf("--file-id 为必填参数")
			}
			req := map[string]any{"fileId": fileID}
			if spaceID, err := cmd.Flags().GetInt64("space-id"); err == nil && spaceID != 0 {
				req["spaceId"] = spaceID
			}
			if v := strings.TrimSpace(MustGetStringFlag(cmd, "file-name")); v != "" {
				req["fileName"] = v
			}
			if v := strings.TrimSpace(MustGetStringFlag(cmd, "file-type")); v != "" {
				req["fileType"] = v
			}
			if fileSize, err := cmd.Flags().GetInt64("file-size"); err == nil && fileSize != 0 {
				req["fileSize"] = fileSize
			}
			return callMCPToolOnServer("contract", "importProject", map[string]any{
				"ImportProjectOpenRequest": req,
			})
		},
	}

	projectImportResultCmd := &cobra.Command{
		Use:   "import-result",
		Short: "获取项目批量导入结果",
		Long: `按任务 ID 查询批量导入执行结果（MCP getImportProjectResult）。
必填：--task-id。`,
		Example: `  dws contract project import-result --task-id "task_xxx" --format json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			taskID := strings.TrimSpace(MustGetStringFlag(cmd, "task-id"))
			if taskID == "" {
				return fmt.Errorf("--task-id 为必填参数")
			}
			return callMCPToolOnServer("contract", "getImportProjectResult", map[string]any{
				"GetImportProjectResultOpenRequest": map[string]any{
					"taskId": taskID,
				},
			})
		},
	}

	// project flags
	projectAddCmd.Flags().String("name", "", "项目名称（必填）")
	projectAddCmd.Flags().String("code", "", "项目编码")
	projectAddCmd.Flags().String("owners", "", "负责人 staffId 列表，逗号分隔")
	projectAddCmd.Flags().String("start-date", "", "开始日期（ISO-8601，如 2026-03-10T14:00:00+08:00）")
	projectAddCmd.Flags().String("end-date", "", "结束日期（ISO-8601，须晚于 --start-date）")
	projectAddCmd.Flags().String("remark", "", "备注")
	projectAddCmd.Flags().String("contract-ids", "", "关联合同 ID 列表，逗号分隔")
	projectAddCmd.Flags().String("source", "", "来源")

	projectDeleteCmd.Flags().String("project-ids", "", "项目 ID 列表，逗号分隔（必填）")

	projectUpdateCmd.Flags().Int64("project-id", 0, "项目 ID（必填）")
	projectUpdateCmd.Flags().String("name", "", "项目名称（必填）")
	projectUpdateCmd.Flags().String("code", "", "项目编码")
	projectUpdateCmd.Flags().String("owners", "", "负责人 staffId 列表，逗号分隔")
	projectUpdateCmd.Flags().String("start-date", "", "开始日期（ISO-8601，如 2026-03-10T14:00:00+08:00）")
	projectUpdateCmd.Flags().String("end-date", "", "结束日期（ISO-8601，须晚于 --start-date）")
	projectUpdateCmd.Flags().String("remark", "", "备注")
	projectUpdateCmd.Flags().String("contract-ids", "", "关联合同 ID 列表，逗号分隔")

	projectSetStatusCmd.Flags().Int64("project-id", 0, "项目 ID（必填）")
	projectSetStatusCmd.Flags().String("status", "", "项目状态（必填）")

	projectListCmd.Flags().Int64("current-page", 0, "当前页码（必填）")
	projectListCmd.Flags().Int64("page-size", 0, "每页条数（必填）")
	projectListCmd.Flags().String("scope", "", "查询范围：self(我负责的)/all(所有项目)（必填）")
	projectListCmd.Flags().String("name", "", "项目名称（模糊搜索）")
	projectListCmd.Flags().String("code", "", "项目编码")
	projectListCmd.Flags().String("owners", "", "负责人 staffId 列表，逗号分隔")
	projectListCmd.Flags().String("status", "", "项目状态")
	projectListCmd.Flags().String("start-date-left", "", "开始日期左区间（ISO-8601，如 2026-01-01T00:00:00+08:00）")
	projectListCmd.Flags().String("start-date-right", "", "开始日期右区间（ISO-8601）")
	projectListCmd.Flags().String("end-date-left", "", "结束日期左区间（ISO-8601）")
	projectListCmd.Flags().String("end-date-right", "", "结束日期右区间（ISO-8601）")

	projectDigestsCmd.Flags().Int64("current-page", 0, "当前页码（必填）")
	projectDigestsCmd.Flags().Int64("page-size", 0, "每页条数（必填）")
	projectDigestsCmd.Flags().String("scope", "", "查询范围：self/all（必填）")
	projectDigestsCmd.Flags().String("name", "", "项目名称（模糊搜索）")
	projectDigestsCmd.Flags().String("code", "", "项目编码")
	projectDigestsCmd.Flags().String("owners", "", "负责人 staffId 列表，逗号分隔")
	projectDigestsCmd.Flags().String("status", "", "项目状态")
	projectDigestsCmd.Flags().String("start-date-left", "", "开始日期左区间（ISO-8601，如 2026-01-01T00:00:00+08:00）")
	projectDigestsCmd.Flags().String("start-date-right", "", "开始日期右区间（ISO-8601）")
	projectDigestsCmd.Flags().String("end-date-left", "", "结束日期左区间（ISO-8601）")
	projectDigestsCmd.Flags().String("end-date-right", "", "结束日期右区间（ISO-8601）")

	projectDetailCmd.Flags().Int64("project-id", 0, "项目 ID（必填）")

	projectExportCmd.Flags().String("project-ids", "", "项目 ID 列表，逗号分隔（必填）")
	projectExportCmd.Flags().String("process-code", "", "审批模板 code（可选）")

	projectImportCmd.Flags().String("file-id", "", "钉盘文件 ID（必填）")
	projectImportCmd.Flags().Int64("space-id", 0, "钉盘空间 ID")
	projectImportCmd.Flags().String("file-name", "", "文件名称")
	projectImportCmd.Flags().String("file-type", "", "文件类型")
	projectImportCmd.Flags().Int64("file-size", 0, "文件大小（字节）")

	projectImportResultCmd.Flags().String("task-id", "", "导入任务 ID（必填）")

	projectCmd.AddCommand(
		projectAddCmd,
		projectDeleteCmd,
		projectUpdateCmd,
		projectSetStatusCmd,
		projectListCmd,
		projectDigestsCmd,
		projectDetailCmd,
		projectExportCmd,
		projectImportTemplateCmd,
		projectImportCmd,
		projectImportResultCmd,
	)
	// ── subject ───────────────────────────────────────────────

	subjectCmd := newGroupCommand(&cobra.Command{Use: "subject", Short: "合同相对方管理", RunE: groupRunE})

	subjectAddCmd := &cobra.Command{
		Use:   "add",
		Short: "添加相对方",
		Long: `创建合同相对方（MCP addSubject）。字段较多，推荐通过 --file 传入 JSON。
必填：partyType, name；其余可选。

【枚举值说明】
partyType: other(对方), our(己方)
bankAccountType: BUSINESS_ACCOUNT(对公), PERSONAL_ACCOUNT(个人)
source: contract(智能合同), oa(OA审批)`,
		Example: `  dws contract subject add --file ./subject.json --format json
  cat subject.json | dws contract subject add --file - --format json

示例 subject.json：
{
  "partyType": "other",
  "name": "北京示例科技有限公司",
  "ucsi": "91110108MA01XXXXX",
  "legalPerson": "张三",
  "tags": ["供应商", "战略合作"]
}`,
		RunE: func(cmd *cobra.Command, args []string) error {
			payload, err := readContractJSONPayload(cmd)
			if err != nil {
				return err
			}
			return callMCPToolOnServer("contract", "addSubject", map[string]any{"AddSubjectOpenRequest": payload})
		},
	}

	subjectListCmd := &cobra.Command{
		Use:   "list",
		Short: "查询相对方列表",
		Long: `分页查询相对方（MCP querySubjects）。
必填：--current-page, --page-size；可选：--party-type, --name, --code, --source。`,
		Example: `  dws contract subject list --current-page 1 --page-size 20 --format json
  dws contract subject list --current-page 1 --page-size 10 --party-type other --name "科技" --format json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			currentPage, err := cmd.Flags().GetInt64("current-page")
			if err != nil || currentPage == 0 {
				return fmt.Errorf("--current-page 为必填参数（正整数）")
			}
			pageSize, err := cmd.Flags().GetInt64("page-size")
			if err != nil || pageSize == 0 {
				return fmt.Errorf("--page-size 为必填参数（正整数）")
			}
			req := map[string]any{
				"currentPage": currentPage,
				"pageSize":    pageSize,
			}
			if v := strings.TrimSpace(MustGetStringFlag(cmd, "party-type")); v != "" {
				req["partyType"] = v
			}
			if v := strings.TrimSpace(MustGetStringFlag(cmd, "name")); v != "" {
				req["name"] = v
			}
			if v := strings.TrimSpace(MustGetStringFlag(cmd, "code")); v != "" {
				req["code"] = v
			}
			if v := strings.TrimSpace(MustGetStringFlag(cmd, "source")); v != "" {
				req["source"] = v
			}
			return callMCPToolOnServer("contract", "querySubjects", map[string]any{
				"QuerySubjectOpenRequest": req,
			})
		},
	}

	subjectDetailCmd := &cobra.Command{
		Use:   "detail",
		Short: "查询相对方详情",
		Long: `按相对方 ID 查询详情（MCP querySubjectDetail）。
必填：--subject-id。`,
		Example: `  dws contract subject detail --subject-id 2001 --format json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			subjectID, err := cmd.Flags().GetInt64("subject-id")
			if err != nil || subjectID == 0 {
				return fmt.Errorf("--subject-id 为必填参数（整数）")
			}
			return callMCPToolOnServer("contract", "querySubjectDetail", map[string]any{
				"QuerySubjectDetailOpenRequest": map[string]any{
					"subjectId": subjectID,
				},
			})
		},
	}

	subjectUpdateCmd := &cobra.Command{
		Use:   "update",
		Short: "修改相对方",
		Long: `修改已有相对方信息（MCP updateSubject）。字段较多，推荐通过 --file 传入 JSON。
JSON 中须包含 subjectId, partyType, name 等必填字段。`,
		Example: `  dws contract subject update --file ./subject_update.json --format json

示例 subject_update.json：
{
  "subjectId": 2001,
  "partyType": "other",
  "name": "北京示例科技有限公司（更新）",
  "remark": "信息变更"
}`,
		RunE: func(cmd *cobra.Command, args []string) error {
			payload, err := readContractJSONPayload(cmd)
			if err != nil {
				return err
			}
			return callMCPToolOnServer("contract", "updateSubject", map[string]any{
				"UpdateSubjectOpenRequest": payload,
			})
		},
	}

	subjectDeleteCmd := &cobra.Command{
		Use:   "delete",
		Short: "删除相对方（单个）",
		Long: `按相对方 ID 删除（MCP deleteSubject）。
必填：--subject-id。`,
		Example: `  dws contract subject delete --subject-id 2001 --format json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			subjectID, err := cmd.Flags().GetInt64("subject-id")
			if err != nil || subjectID == 0 {
				return fmt.Errorf("--subject-id 为必填参数（整数）")
			}
			return callMCPToolOnServer("contract", "deleteSubject", map[string]any{
				"DeleteSubjectOpenRequest": map[string]any{
					"subjectId": subjectID,
				},
			})
		},
	}

	subjectBatchDeleteCmd := &cobra.Command{
		Use:   "batch-delete",
		Short: "批量删除相对方",
		Long: `按相对方 ID 列表批量删除（MCP batchDeleteSubject），一次最多 1000 个。
必填：--subject-ids（逗号分隔）。`,
		Example: `  dws contract subject batch-delete --subject-ids "2001,2002,2003" --format json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			raw := strings.TrimSpace(MustGetStringFlag(cmd, "subject-ids"))
			if raw == "" {
				return fmt.Errorf("--subject-ids 为必填参数")
			}
			ids, err := parseContractInt64CSV(raw)
			if err != nil {
				return fmt.Errorf("--subject-ids 须为逗号分隔的整数: %w", err)
			}
			return callMCPToolOnServer("contract", "batchDeleteSubject", map[string]any{
				"BatchDeleteSubjectOpenRequest": map[string]any{
					"subjectIdList": ids,
				},
			})
		},
	}

	subjectSortCmd := &cobra.Command{
		Use:   "sort",
		Short: "己方主体排序",
		Long: `设置己方主体的展示顺序（MCP sortSubjects）。
必填：--subject-ids（逗号分隔，按期望顺序排列）。`,
		Example: `  dws contract subject sort --subject-ids "2001,2003,2002" --format json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			raw := strings.TrimSpace(MustGetStringFlag(cmd, "subject-ids"))
			if raw == "" {
				return fmt.Errorf("--subject-ids 为必填参数")
			}
			ids, err := parseContractInt64CSV(raw)
			if err != nil {
				return fmt.Errorf("--subject-ids 须为逗号分隔的整数: %w", err)
			}
			return callMCPToolOnServer("contract", "sortSubjects", map[string]any{
				"SortSubjectOpenRequest": map[string]any{
					"subjectIdList": ids,
				},
			})
		},
	}

	subjectRiskCmd := &cobra.Command{
		Use:   "detect-risk",
		Short: "检测相对方风险",
		Long: `检测相对方风险信息（MCP detectSubjectRisk）。
必填：--subject-name；可选：--subject-id。`,
		Example: `  dws contract subject detect-risk --subject-name "北京示例科技有限公司" --format json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			subjectName := strings.TrimSpace(MustGetStringFlag(cmd, "subject-name"))
			if subjectName == "" {
				return fmt.Errorf("--subject-name 为必填参数")
			}
			req := map[string]any{"subjectName": subjectName}
			if subjectID, err := cmd.Flags().GetInt64("subject-id"); err == nil && subjectID != 0 {
				req["subjectId"] = subjectID
			}
			return callMCPToolOnServer("contract", "detectSubjectRisk", map[string]any{
				"SubjectRiskOpenRequest": req,
			})
		},
	}

	subjectBaseInfoCmd := &cobra.Command{
		Use:   "base-info",
		Short: "查询相对方工商基本信息",
		Long: `查询相对方工商信息（MCP querySubjectBaseInfo）。
必填：--subject-name；可选：--subject-id。`,
		Example: `  dws contract subject base-info --subject-name "北京示例科技有限公司" --format json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			subjectName := strings.TrimSpace(MustGetStringFlag(cmd, "subject-name"))
			if subjectName == "" {
				return fmt.Errorf("--subject-name 为必填参数")
			}
			req := map[string]any{"subjectName": subjectName}
			if subjectID, err := cmd.Flags().GetInt64("subject-id"); err == nil && subjectID != 0 {
				req["subjectId"] = subjectID
			}
			return callMCPToolOnServer("contract", "querySubjectBaseInfo", map[string]any{
				"SubjectRiskOpenRequest": req,
			})
		},
	}

	subjectAutoFillCmd := &cobra.Command{
		Use:   "auto-fill",
		Short: "相对方信息智能填充",
		Long: `根据相对方名称智能填充详细信息（MCP autoFillSubjectInfo）。
必填：--subject-name；可选：--subject-id。`,
		Example: `  dws contract subject auto-fill --subject-name "北京示例科技有限公司" --format json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			subjectName := strings.TrimSpace(MustGetStringFlag(cmd, "subject-name"))
			if subjectName == "" {
				return fmt.Errorf("--subject-name 为必填参数")
			}
			req := map[string]any{"subjectName": subjectName}
			if subjectID, err := cmd.Flags().GetInt64("subject-id"); err == nil && subjectID != 0 {
				req["subjectId"] = subjectID
			}
			return callMCPToolOnServer("contract", "autoFillSubjectInfo", map[string]any{
				"SubjectRiskOpenRequest": req,
			})
		},
	}

	subjectExportCmd := &cobra.Command{
		Use:   "export",
		Short: "导出相对方到 Excel",
		Long: `导出指定相对方到 Excel（MCP exportSubject）。
必填：--subject-ids（逗号分隔）；可选：--process-code。`,
		Example: `  dws contract subject export --subject-ids "2001,2002" --format json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			raw := strings.TrimSpace(MustGetStringFlag(cmd, "subject-ids"))
			if raw == "" {
				return fmt.Errorf("--subject-ids 为必填参数")
			}
			ids, err := parseContractInt64CSV(raw)
			if err != nil {
				return fmt.Errorf("--subject-ids 须为逗号分隔的整数: %w", err)
			}
			req := map[string]any{"subjectIds": ids}
			if v := strings.TrimSpace(MustGetStringFlag(cmd, "process-code")); v != "" {
				req["processCode"] = v
			}
			return callMCPToolOnServer("contract", "exportSubject", map[string]any{
				"ExportSubjectOpenRequest": req,
			})
		},
	}

	subjectImportTemplateCmd := &cobra.Command{
		Use:   "import-template",
		Short: "获取相对方批量导入模板",
		Long: `获取相对方批量导入的 Excel 模板下载链接（MCP getImportSubjectTemplate）。
可选：--type（other/our）。`,
		Example: `  dws contract subject import-template --format json
  dws contract subject import-template --type other --format json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			req := map[string]any{}
			if v := strings.TrimSpace(MustGetStringFlag(cmd, "type")); v != "" {
				req["type"] = v
			}
			return callMCPToolOnServer("contract", "getImportSubjectTemplate", map[string]any{
				"GetImportSubjectTemplateOpenRequest": req,
			})
		},
	}

	subjectImportCmd := &cobra.Command{
		Use:   "import",
		Short: "批量导入相对方",
		Long: `从钉盘文件批量导入相对方（MCP importSubject）。
必填：--file-id；可选：--space-id, --file-name, --file-type, --file-size。`,
		Example: `  dws contract subject import --file-id "abc123" --space-id 7890 --format json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			fileID := strings.TrimSpace(MustGetStringFlag(cmd, "file-id"))
			if fileID == "" {
				return fmt.Errorf("--file-id 为必填参数")
			}
			req := map[string]any{"fileId": fileID}
			if spaceID, err := cmd.Flags().GetInt64("space-id"); err == nil && spaceID != 0 {
				req["spaceId"] = spaceID
			}
			if v := strings.TrimSpace(MustGetStringFlag(cmd, "file-name")); v != "" {
				req["fileName"] = v
			}
			if v := strings.TrimSpace(MustGetStringFlag(cmd, "file-type")); v != "" {
				req["fileType"] = v
			}
			if fileSize, err := cmd.Flags().GetInt64("file-size"); err == nil && fileSize != 0 {
				req["fileSize"] = fileSize
			}
			return callMCPToolOnServer("contract", "importSubject", map[string]any{
				"ImportSubjectOpenRequest": req,
			})
		},
	}

	subjectImportResultCmd := &cobra.Command{
		Use:   "import-result",
		Short: "查询相对方批量导入结果",
		Long: `按任务 ID 查询导入结果（MCP getImportSubjectResult）。
必填：--task-id。`,
		Example: `  dws contract subject import-result --task-id "task_xxx" --format json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			taskID := strings.TrimSpace(MustGetStringFlag(cmd, "task-id"))
			if taskID == "" {
				return fmt.Errorf("--task-id 为必填参数")
			}
			return callMCPToolOnServer("contract", "getImportSubjectResult", map[string]any{
				"GetImportSubjectResultOpenRequest": map[string]any{
					"taskId": taskID,
				},
			})
		},
	}

	// subject flags
	subjectAddCmd.Flags().String("file", "", "AddSubjectOpenRequest JSON 文件路径，\"-\" 表示 stdin（必填）")
	subjectUpdateCmd.Flags().String("file", "", "UpdateSubjectOpenRequest JSON 文件路径，\"-\" 表示 stdin（必填）")

	subjectListCmd.Flags().Int64("current-page", 0, "当前页码（必填）")
	subjectListCmd.Flags().Int64("page-size", 0, "每页条数（必填）")
	subjectListCmd.Flags().String("party-type", "", "相对方类型：other(对方)/our(己方)")
	subjectListCmd.Flags().String("name", "", "相对方名称（模糊匹配）")
	subjectListCmd.Flags().String("code", "", "主体编号")
	subjectListCmd.Flags().String("source", "", "来源：contract/oa")

	subjectDetailCmd.Flags().Int64("subject-id", 0, "相对方 ID（必填）")

	subjectDeleteCmd.Flags().Int64("subject-id", 0, "相对方 ID（必填）")

	subjectBatchDeleteCmd.Flags().String("subject-ids", "", "相对方 ID 列表，逗号分隔（必填，最多 1000 个）")

	subjectSortCmd.Flags().String("subject-ids", "", "己方主体 ID 列表，逗号分隔，按期望顺序（必填）")

	subjectRiskCmd.Flags().String("subject-name", "", "相对方名称（必填）")
	subjectRiskCmd.Flags().Int64("subject-id", 0, "相对方 ID（可选）")

	subjectBaseInfoCmd.Flags().String("subject-name", "", "相对方名称（必填）")
	subjectBaseInfoCmd.Flags().Int64("subject-id", 0, "相对方 ID（可选）")

	subjectAutoFillCmd.Flags().String("subject-name", "", "相对方名称（必填）")
	subjectAutoFillCmd.Flags().Int64("subject-id", 0, "相对方 ID（可选）")

	subjectExportCmd.Flags().String("subject-ids", "", "相对方 ID 列表，逗号分隔（必填）")
	subjectExportCmd.Flags().String("process-code", "", "审批模板 code（可选）")

	subjectImportTemplateCmd.Flags().String("type", "", "相对方类型：other/our（可选）")

	subjectImportCmd.Flags().String("file-id", "", "钉盘文件 ID（必填）")
	subjectImportCmd.Flags().Int64("space-id", 0, "钉盘空间 ID")
	subjectImportCmd.Flags().String("file-name", "", "文件名称")
	subjectImportCmd.Flags().String("file-type", "", "文件类型")
	subjectImportCmd.Flags().Int64("file-size", 0, "文件大小（字节）")

	subjectImportResultCmd.Flags().String("task-id", "", "导入任务 ID（必填）")

	subjectCmd.AddCommand(
		subjectAddCmd,
		subjectListCmd,
		subjectDetailCmd,
		subjectUpdateCmd,
		subjectDeleteCmd,
		subjectBatchDeleteCmd,
		subjectSortCmd,
		subjectRiskCmd,
		subjectBaseInfoCmd,
		subjectAutoFillCmd,
		subjectExportCmd,
		subjectImportTemplateCmd,
		subjectImportCmd,
		subjectImportResultCmd,
	)
	// ── assemble tree ─────────────────────────────────────────
	root.AddCommand(
		recordCmd,
		importCmd,
		processTemplatesCmd,
		fileDirectoriesCmd,
		draftCmd,
		reviewCmd,
		accountCmd,
		archCmd,
		projectCmd,
		subjectCmd,
	)

	// Attach ContractDecl metadata to all leaf commands so the contract tree
	// enters the Agent Schema surface (identity, safety, interface, selection,
	// parameters). Must run after flags are registered and before return.
	declareContractSchema(&contractSchemaRefs{
		RecordList:            recordListCmd,
		RecordGet:             recordGetCmd,
		RecordQuantityByType:  recordQuantityByTypeCmd,
		RecordCreate:          recordCreateCmd,
		ImportBatch:           importBatchCmd,
		ImportBatchResult:     importBatchResultCmd,
		ProcessTemplates:      processTemplatesCmd,
		FileDirectories:       fileDirectoriesCmd,
		Draft:                 draftCmd,
		ReviewBenefit:         reviewBenefitCmd,
		ReviewCreate:          reviewCreateCmd,
		ReviewAnalysis:        reviewAnalysisCmd,
		ReviewResult:          reviewResultCmd,
		AccountCreate:         accountCreateCmd,
		AccountUpdate:         accountUpdateCmd,
		AccountGet:            accountGetCmd,
		AccountList:           accountListCmd,
		AccountDelete:         accountDeleteCmd,
		Archive:               archCmd,
		ProjectAdd:            projectAddCmd,
		ProjectDelete:         projectDeleteCmd,
		ProjectUpdate:         projectUpdateCmd,
		ProjectSetStatus:      projectSetStatusCmd,
		ProjectList:           projectListCmd,
		ProjectDigests:        projectDigestsCmd,
		ProjectDetail:         projectDetailCmd,
		ProjectExport:         projectExportCmd,
		ProjectImportTemplate: projectImportTemplateCmd,
		ProjectImport:         projectImportCmd,
		ProjectImportResult:   projectImportResultCmd,
		SubjectAdd:            subjectAddCmd,
		SubjectList:           subjectListCmd,
		SubjectDetail:         subjectDetailCmd,
		SubjectUpdate:         subjectUpdateCmd,
		SubjectDelete:         subjectDeleteCmd,
		SubjectBatchDelete:    subjectBatchDeleteCmd,
		SubjectSort:           subjectSortCmd,
		SubjectDetectRisk:     subjectRiskCmd,
		SubjectBaseInfo:       subjectBaseInfoCmd,
		SubjectAutoFill:       subjectAutoFillCmd,
		SubjectExport:         subjectExportCmd,
		SubjectImportTemplate: subjectImportTemplateCmd,
		SubjectImport:         subjectImportCmd,
		SubjectImportResult:   subjectImportResultCmd,
	})

	return root
}

// ── shared helpers ────────────────────────────────────────

// readContractJSONPayload reads the path from flag "file" (file path or "-" for stdin) into a JSON object.
func readContractJSONPayload(cmd *cobra.Command) (map[string]any, error) {
	path, _ := cmd.Flags().GetString("file")
	if strings.TrimSpace(path) == "" {
		return nil, fmt.Errorf(`--file 为必填（JSON 路径，或 "-" 表示 stdin）`)
	}
	var r io.Reader
	if path == "-" {
		r = cmd.InOrStdin()
	} else {
		f, err := os.Open(path)
		if err != nil {
			return nil, fmt.Errorf("打开 JSON 文件: %w", err)
		}
		defer f.Close()
		r = f
	}
	b, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("读取 JSON: %w", err)
	}
	var payload map[string]any
	if err := json.Unmarshal(b, &payload); err != nil {
		return nil, fmt.Errorf("JSON 解析失败: %w", err)
	}
	return payload, nil
}

// parseContractRecordQueryScope 解析 record list / quantity-by-type 的 --type（与 MCP 的查询维度一致）。空值按 all。
func parseContractRecordQueryScope(raw string) (string, error) {
	t := strings.TrimSpace(strings.ToLower(raw))
	if t == "" {
		return "all", nil
	}
	switch t {
	case "self", "participation", "department", "all", "unassigned":
		return t, nil
	default:
		return "", fmt.Errorf(
			`--type 须为查询维度之一: self（我的）、participation（我参与的）、department（我部门的）、all（全部）、unassigned（待分配）；收到 %q`,
			strings.TrimSpace(raw),
		)
	}
}

// appendContractCreateTimeFromISO reads an optional CLI time flag, parses ISO-8601 to milliseconds,
// and sets jsonKey on req for MCP queryContracts (createStartTime / createEndTime).
func appendContractCreateTimeFromISO(req map[string]any, cmd *cobra.Command, flagName, jsonKey string) error {
	raw, _ := cmd.Flags().GetString(flagName)
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	ms, err := cmdutil.ParseISOTimeToMillis(flagName, raw)
	if err != nil {
		return err
	}
	req[jsonKey] = ms
	return nil
}

func parseContractInt64CSV(raw string) ([]int64, error) {
	parts := strings.Split(raw, ",")
	out := make([]int64, 0, len(parts))
	for _, p := range parts {
		v := strings.TrimSpace(p)
		if v == "" {
			continue
		}
		n, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("无法解析 %q 为整数: %w", v, err)
		}
		out = append(out, n)
	}
	return out, nil
}
