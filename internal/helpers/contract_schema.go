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
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
	"github.com/spf13/cobra"
)

// contractSchemaRefs holds pointers to all 44 contract leaf commands so
// declareContractSchema can attach ContractDecl metadata without inflating
// contract.go with inline declarations.
type contractSchemaRefs struct {
	RecordList            *cobra.Command
	RecordGet             *cobra.Command
	RecordQuantityByType  *cobra.Command
	RecordCreate          *cobra.Command
	ImportBatch           *cobra.Command
	ImportBatchResult     *cobra.Command
	ProcessTemplates      *cobra.Command
	FileDirectories       *cobra.Command
	Draft                 *cobra.Command
	ReviewBenefit         *cobra.Command
	ReviewCreate          *cobra.Command
	ReviewAnalysis        *cobra.Command
	ReviewResult          *cobra.Command
	AccountCreate         *cobra.Command
	AccountUpdate         *cobra.Command
	AccountGet            *cobra.Command
	AccountList           *cobra.Command
	AccountDelete         *cobra.Command
	Archive               *cobra.Command
	ProjectAdd            *cobra.Command
	ProjectDelete         *cobra.Command
	ProjectUpdate         *cobra.Command
	ProjectSetStatus      *cobra.Command
	ProjectList           *cobra.Command
	ProjectDigests        *cobra.Command
	ProjectDetail         *cobra.Command
	ProjectExport         *cobra.Command
	ProjectImportTemplate *cobra.Command
	ProjectImport         *cobra.Command
	ProjectImportResult   *cobra.Command
	SubjectAdd            *cobra.Command
	SubjectList           *cobra.Command
	SubjectDetail         *cobra.Command
	SubjectUpdate         *cobra.Command
	SubjectDelete         *cobra.Command
	SubjectBatchDelete    *cobra.Command
	SubjectSort           *cobra.Command
	SubjectDetectRisk     *cobra.Command
	SubjectBaseInfo       *cobra.Command
	SubjectAutoFill       *cobra.Command
	SubjectExport         *cobra.Command
	SubjectImportTemplate *cobra.Command
	SubjectImport         *cobra.Command
	SubjectImportResult   *cobra.Command
}

// contractCompositeIface is the shared interface disposition for all contract
// leaves: commands dispatch through the contract MCP server and wrap payloads
// in *OpenRequest structures, so no single interface_ref can represent them.
var contractCompositeIface = &contract.InterfaceSpec{
	Mode:         contract.InterfaceModeComposite,
	Availability: contract.InterfaceAvailable,
	Reason:       "命令通过智能合同 MCP 服务器分派并包裹在 *OpenRequest 结构中，不能绑定为单一 interface_ref",
}

var (
	safetyRead        = contract.SafetySpec{Effect: "read", Risk: "low", Confirmation: "not_required", Idempotency: "idempotent"}
	safetyWrite       = contract.SafetySpec{Effect: "write", Risk: "medium", Confirmation: "not_required", Idempotency: "unknown"}
	safetyDestructive = contract.SafetySpec{Effect: "destructive", Risk: "high", Confirmation: "not_required", Idempotency: "unknown"}
)

// declareContractSchema attaches ContractDecl metadata to all 44 contract
// leaf commands. It must be called after all flags are registered but before
// the root command is returned, so that assembly can collect identity and
// resolve the full Schema.
func declareContractSchema(r *contractSchemaRefs) {
	// ── record ──────────────────────────────────────────────
	DeclareLeafMetadata(r.RecordList, LeafSpec{
		Safety: safetyRead,
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID: "contract", Name: "record_list", CanonicalPath: "contract.record_list",
				CLIPath: "contract record list", PrimaryCLIPath: "contract record list",
			},
			Description: "按创建时间范围与状态筛选查询合同列表。",
			Interface:   contractCompositeIface,
			Selection: contract.SelectionSpec{
				AgentSummary: "查询合同列表，按创建时间、状态和查询维度筛选。",
				UseWhen:      []string{"用户要查看合同台账列表或按状态/时间/维度筛选合同"},
				AvoidWhen:    []string{"查单份合同详情用 record get；按维度统计数量用 record quantity-by-type"},
				Examples:     []string{"dws contract record list --format json"},
			},
			Parameters: []contract.ParamDecl{
				{Name: "start", Property: "createStartTime"},
				{Name: "end", Property: "createEndTime"},
				{Name: "status", Property: "contractStatusList"},
				{Name: "type", Property: "type"},
			},
		},
	})

	DeclareLeafMetadata(r.RecordGet, LeafSpec{
		Safety: safetyRead,
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID: "contract", Name: "record_get", CanonicalPath: "contract.record_get",
				CLIPath: "contract record get", PrimaryCLIPath: "contract record get",
				Aliases: []string{"contract record detail"},
			},
			Description: "按合同 ID 查询单份合同详情。",
			Interface:   contractCompositeIface,
			Selection: contract.SelectionSpec{
				AgentSummary: "按合同 ID 查询单份合同详情。",
				UseWhen:      []string{"用户要查看某份合同的完整详情"},
				AvoidWhen:    []string{"查合同列表用 record list；按维度统计用 record quantity-by-type"},
				Examples:     []string{`dws contract record get --contract-id "c_xxx" --format json`},
			},
			Parameters: []contract.ParamDecl{
				{Name: "contract-id", Property: "contractId", Required: boolPtr(true)},
			},
		},
	})

	DeclareLeafMetadata(r.RecordQuantityByType, LeafSpec{
		Safety: safetyRead,
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID: "contract", Name: "record_quantity_by_type", CanonicalPath: "contract.record_quantity_by_type",
				CLIPath: "contract record quantity-by-type", PrimaryCLIPath: "contract record quantity-by-type",
			},
			Description: "按查询维度返回各合同状态下的台账条数。",
			Interface:   contractCompositeIface,
			Selection: contract.SelectionSpec{
				AgentSummary: "按查询维度统计各状态合同数量。",
				UseWhen:      []string{"用户要按维度统计各状态的合同数量"},
				AvoidWhen:    []string{"要查具体合同列表用 record list"},
				Examples:     []string{"dws contract record quantity-by-type --format json"},
			},
			Parameters: []contract.ParamDecl{
				{Name: "type", Property: "type"},
			},
		},
	})

	DeclareLeafMetadata(r.RecordCreate, LeafSpec{
		Safety: safetyWrite,
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID: "contract", Name: "record_create", CanonicalPath: "contract.record_create",
				CLIPath: "contract record create", PrimaryCLIPath: "contract record create",
			},
			Description: "将合同文件与关键信息写入台账。",
			Interface:   contractCompositeIface,
			Selection: contract.SelectionSpec{
				AgentSummary: "创建合同台账记录。",
				UseWhen:      []string{"用户要将合同文件与关键信息写入台账"},
				AvoidWhen:    []string{"批量导入用 import batch；起草合同用 draft"},
				Examples:     []string{"dws contract record create --file ./contract.json --format json"},
			},
			Parameters: []contract.ParamDecl{
				{Name: "file", Property: "ImportContractInfoRequest", Required: boolPtr(true)},
			},
		},
	})

	// ── import ─────────────────────────────────────────────
	DeclareLeafMetadata(r.ImportBatch, LeafSpec{
		Safety: safetyWrite,
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID: "contract", Name: "import_batch", CanonicalPath: "contract.import_batch",
				CLIPath: "contract import batch", PrimaryCLIPath: "contract import batch",
			},
			Description: "基于钉盘模版文件创建异步批量导入任务。",
			Interface:   contractCompositeIface,
			Selection: contract.SelectionSpec{
				AgentSummary: "从钉盘模版文件创建合同批量导入任务。",
				UseWhen:      []string{"用户要批量导入合同并已准备好钉盘模版文件"},
				AvoidWhen:    []string{"单条创建用 record create；查导入结果用 import batch-result"},
				Examples:     []string{`dws contract import batch --file-id "123456" --space-id "7890" --format json`},
			},
			Parameters: []contract.ParamDecl{
				{Name: "file-id", Property: "fileId", Required: boolPtr(true)},
				{Name: "space-id", Property: "spaceId", Required: boolPtr(true)},
			},
		},
	})

	DeclareLeafMetadata(r.ImportBatchResult, LeafSpec{
		Safety: safetyRead,
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID: "contract", Name: "import_batch_result", CanonicalPath: "contract.import_batch_result",
				CLIPath: "contract import batch-result", PrimaryCLIPath: "contract import batch-result",
			},
			Description: "按任务 ID 查询批量导入执行结果。",
			Interface:   contractCompositeIface,
			Selection: contract.SelectionSpec{
				AgentSummary: "查询合同批量导入任务结果。",
				UseWhen:      []string{"用户已创建批量导入任务后要查执行结果"},
				AvoidWhen:    []string{"创建导入任务用 import batch"},
				Examples:     []string{`dws contract import batch-result --task-id "task_xxx" --format json`},
			},
			Parameters: []contract.ParamDecl{
				{Name: "task-id", Property: "taskId", Required: boolPtr(true)},
			},
		},
	})

	// ── process-templates / file-directories ───────────────
	DeclareLeafMetadata(r.ProcessTemplates, LeafSpec{
		Safety: safetyRead,
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID: "contract", Name: "process_templates", CanonicalPath: "contract.process_templates",
				CLIPath: "contract process-templates", PrimaryCLIPath: "contract process-templates",
			},
			Description: "列出当前登录用户可见的合同审批模板。",
			Interface:   contractCompositeIface,
			Selection: contract.SelectionSpec{
				AgentSummary: "查询当前用户可见的合同审批模板。",
				UseWhen:      []string{"用户要查看可用的合同审批模板"},
				AvoidWhen:    []string{"查台账分类用 file-directories；OA 审批处理走 misc"},
				Examples:     []string{"dws contract process-templates --format json"},
			},
		},
	})

	DeclareLeafMetadata(r.FileDirectories, LeafSpec{
		Safety: safetyRead,
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID: "contract", Name: "file_directories", CanonicalPath: "contract.file_directories",
				CLIPath: "contract file-directories", PrimaryCLIPath: "contract file-directories",
				Aliases: []string{"contract directories"},
			},
			Description: "返回全部合同台账目录/分类。",
			Interface:   contractCompositeIface,
			Selection: contract.SelectionSpec{
				AgentSummary: "查询所有合同台账分类。",
				UseWhen:      []string{"用户要查看合同台账的分类目录"},
				AvoidWhen:    []string{"查审批模板用 process-templates"},
				Examples:     []string{"dws contract file-directories --format json"},
			},
		},
	})

	// ── draft ───────────────────────────────────────────────
	DeclareLeafMetadata(r.Draft, LeafSpec{
		Safety: safetyWrite,
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID: "contract", Name: "draft", CanonicalPath: "contract.draft",
				CLIPath: "contract draft", PrimaryCLIPath: "contract draft",
			},
			Description: "根据 AI 听记任务与合同模版起草合同。",
			Interface:   contractCompositeIface,
			Selection: contract.SelectionSpec{
				AgentSummary: "根据听记和模版起草合同。",
				UseWhen:      []string{"用户要根据 AI 听记和合同模版起草合同"},
				AvoidWhen:    []string{"听记内容查询走 minutes；仅从听记获取 taskUuid 后回本产品调用 draft"},
				Examples:     []string{`dws contract draft --task-uuids uuid1,uuid2 --template-url "https://..." --format json`},
			},
			Parameters: []contract.ParamDecl{
				{Name: "task-uuids", Property: "taskUuids", Required: boolPtr(true)},
				{Name: "template-url", Property: "templateUrl"},
				{Name: "template-content", Property: "templateContent"},
			},
		},
	})

	// ── review ──────────────────────────────────────────────
	DeclareLeafMetadata(r.ReviewBenefit, LeafSpec{
		Safety: safetyRead,
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID: "contract", Name: "review_benefit", CanonicalPath: "contract.review_benefit",
				CLIPath: "contract review benefit", PrimaryCLIPath: "contract review benefit",
			},
			Description: "查询用户组织的合同审查权益数据。",
			Interface:   contractCompositeIface,
			Selection: contract.SelectionSpec{
				AgentSummary: "查询合同审查权益。",
				UseWhen:      []string{"用户要查看合同审查的权益额度或使用情况"},
				AvoidWhen:    []string{"创建审查任务用 review create；查审查结果用 review result"},
				Examples:     []string{"dws contract review benefit --format json"},
			},
		},
	})

	DeclareLeafMetadata(r.ReviewCreate, LeafSpec{
		Safety: safetyWrite,
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID: "contract", Name: "review_create", CanonicalPath: "contract.review_create",
				CLIPath: "contract review create", PrimaryCLIPath: "contract review create",
			},
			Description: "创建合同审查任务。",
			Interface:   contractCompositeIface,
			Selection: contract.SelectionSpec{
				AgentSummary: "创建合同审查任务，提交合同文件进行 AI 审查。",
				UseWhen:      []string{"用户要对合同文件发起 AI 审查"},
				AvoidWhen:    []string{"解析合同文件用 review analysis；查审查结果用 review result"},
				Examples:     []string{"dws contract review create --file ./review_request.json --format json"},
			},
			Parameters: []contract.ParamDecl{
				{Name: "file", Property: "IntelligentContractReviewClientRequest", Required: boolPtr(true)},
			},
		},
	})

	DeclareLeafMetadata(r.ReviewAnalysis, LeafSpec{
		Safety: safetyRead,
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID: "contract", Name: "review_analysis", CanonicalPath: "contract.review_analysis",
				CLIPath: "contract review analysis", PrimaryCLIPath: "contract review analysis",
			},
			Description: "解析合同文件，返回合同摘要和审查推荐模型。",
			Interface:   contractCompositeIface,
			Selection: contract.SelectionSpec{
				AgentSummary: "解析合同文件并返回摘要和审查推荐。",
				UseWhen:      []string{"用户要解析合同文件获取摘要和审查建议"},
				AvoidWhen:    []string{"创建正式审查任务用 review create；查审查结果用 review result"},
				Examples:     []string{"dws contract review analysis --file ./analysis_request.json --format json"},
			},
			Parameters: []contract.ParamDecl{
				{Name: "file", Property: "AnalysisContractApiRequest", Required: boolPtr(true)},
			},
		},
	})

	DeclareLeafMetadata(r.ReviewResult, LeafSpec{
		Safety: safetyRead,
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID: "contract", Name: "review_result", CanonicalPath: "contract.review_result",
				CLIPath: "contract review result", PrimaryCLIPath: "contract review result",
			},
			Description: "查询合同审查结果。",
			Interface:   contractCompositeIface,
			Selection: contract.SelectionSpec{
				AgentSummary: "按任务 ID 查询合同审查结果。",
				UseWhen:      []string{"用户已创建审查任务后要查询审查结果"},
				AvoidWhen:    []string{"创建审查任务用 review create"},
				Examples:     []string{`dws contract review result --task-id "MjIzODAwMkFJX1JFVklFVw==" --review-type AI_REVIEW --format json`},
			},
			Parameters: []contract.ParamDecl{
				{Name: "task-id", Property: "taskId", Required: boolPtr(true)},
				{Name: "review-type", Property: "reviewType", Required: boolPtr(true)},
			},
		},
	})

	// ── account ─────────────────────────────────────────────
	DeclareLeafMetadata(r.AccountCreate, LeafSpec{
		Safety: safetyWrite,
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID: "contract", Name: "account_create", CanonicalPath: "contract.account_create",
				CLIPath: "contract account create", PrimaryCLIPath: "contract account create",
			},
			Description: "创建合同账款信息。",
			Interface:   contractCompositeIface,
			Selection: contract.SelectionSpec{
				AgentSummary: "创建合同账款记录。",
				UseWhen:      []string{"用户要为合同创建收付款账款信息"},
				AvoidWhen:    []string{"更新账款用 account update；查账款列表用 account list"},
				Examples:     []string{"dws contract account create --file ./account.json --format json"},
			},
			Parameters: []contract.ParamDecl{
				{Name: "file", Property: "CreateContractAccountRequest", Required: boolPtr(true)},
			},
		},
	})

	DeclareLeafMetadata(r.AccountUpdate, LeafSpec{
		Safety: safetyWrite,
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID: "contract", Name: "account_update", CanonicalPath: "contract.account_update",
				CLIPath: "contract account update", PrimaryCLIPath: "contract account update",
			},
			Description: "更新合同账款信息。",
			Interface:   contractCompositeIface,
			Selection: contract.SelectionSpec{
				AgentSummary: "更新已有合同账款信息。",
				UseWhen:      []string{"用户要修改已有账款记录"},
				AvoidWhen:    []string{"创建账款用 account create；查账款详情用 account get"},
				Examples:     []string{"dws contract account update --file ./account_update.json --format json"},
			},
			Parameters: []contract.ParamDecl{
				{Name: "file", Property: "UpdateContractAccountRequest", Required: boolPtr(true)},
			},
		},
	})

	DeclareLeafMetadata(r.AccountGet, LeafSpec{
		Safety: safetyRead,
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID: "contract", Name: "account_get", CanonicalPath: "contract.account_get",
				CLIPath: "contract account get", PrimaryCLIPath: "contract account get",
			},
			Description: "按账款 ID 获取单条账款详情。",
			Interface:   contractCompositeIface,
			Selection: contract.SelectionSpec{
				AgentSummary: "获取账款详情。",
				UseWhen:      []string{"用户要查看某条账款的详细信息"},
				AvoidWhen:    []string{"查账款列表用 account list；创建账款用 account create"},
				Examples:     []string{"dws contract account get --account-id 12345 --format json"},
			},
			Parameters: []contract.ParamDecl{
				{Name: "account-id", Property: "accountEntryId", Required: boolPtr(true)},
			},
		},
	})

	DeclareLeafMetadata(r.AccountList, LeafSpec{
		Safety: safetyRead,
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID: "contract", Name: "account_list", CanonicalPath: "contract.account_list",
				CLIPath: "contract account list", PrimaryCLIPath: "contract account list",
			},
			Description: "按多条件筛选查询账款列表。",
			Interface:   contractCompositeIface,
			Selection: contract.SelectionSpec{
				AgentSummary: "按条件筛选查询账款列表。",
				UseWhen:      []string{"用户要查询合同账款列表"},
				AvoidWhen:    []string{"查单条账款详情用 account get；创建账款用 account create"},
				Examples:     []string{"dws contract account list --scope self --format json"},
			},
			Parameters: []contract.ParamDecl{
				{Name: "scope", Property: "scope"},
				{Name: "query-status", Property: "queryStatus"},
				{Name: "amount-type", Property: "amountType"},
				{Name: "status", Property: "status"},
				{Name: "source", Property: "source"},
				{Name: "contract-code", Property: "contractCode"},
				{Name: "contract-name", Property: "contractName"},
				{Name: "transaction-no", Property: "transactionNo"},
				{Name: "exec-start", Property: "executionDateBegin"},
				{Name: "exec-end", Property: "executionDateEnd"},
				{Name: "page", Property: "currentPage"},
				{Name: "page-size", Property: "pageSize"},
			},
		},
	})

	DeclareLeafMetadata(r.AccountDelete, LeafSpec{
		Safety: safetyDestructive,
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID: "contract", Name: "account_delete", CanonicalPath: "contract.account_delete",
				CLIPath: "contract account delete", PrimaryCLIPath: "contract account delete",
			},
			Description: "按账款 ID 删除账款信息。",
			Interface:   contractCompositeIface,
			Selection: contract.SelectionSpec{
				AgentSummary: "删除合同账款信息。",
				UseWhen:      []string{"用户要删除某条账款记录"},
				AvoidWhen:    []string{"更新账款用 account update；查账款列表用 account list"},
				Examples:     []string{"dws contract account delete --account-id 12345 --format json"},
			},
			Parameters: []contract.ParamDecl{
				{Name: "account-id", Property: "accountEntryId", Required: boolPtr(true)},
			},
		},
	})

	// ── archive ─────────────────────────────────────────────
	DeclareLeafMetadata(r.Archive, LeafSpec{
		Safety: safetyWrite,
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID: "contract", Name: "archive", CanonicalPath: "contract.archive",
				CLIPath: "contract archive", PrimaryCLIPath: "contract archive",
			},
			Description: "对合同进行归档操作。",
			Interface:   contractCompositeIface,
			Selection: contract.SelectionSpec{
				AgentSummary: "将合同文档归档。",
				UseWhen:      []string{"用户要对合同进行归档操作"},
				AvoidWhen:    []string{"创建合同用 record create；查合同详情用 record get"},
				Examples:     []string{"dws contract archive --file ./archive_request.json --format json"},
			},
			Parameters: []contract.ParamDecl{
				{Name: "file", Property: "ContractOpenArchiveRequest", Required: boolPtr(true)},
			},
		},
	})

	// ── project ─────────────────────────────────────────────
	DeclareLeafMetadata(r.ProjectAdd, LeafSpec{
		Safety: safetyWrite,
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID: "contract", Name: "project_add", CanonicalPath: "contract.project_add",
				CLIPath: "contract project add", PrimaryCLIPath: "contract project add",
			},
			Description: "创建合同项目。",
			Interface:   contractCompositeIface,
			Selection: contract.SelectionSpec{
				AgentSummary: "新增合同项目。",
				UseWhen:      []string{"用户要创建新的合同项目"},
				AvoidWhen:    []string{"更新项目用 project update；查项目列表用 project list"},
				Examples:     []string{`dws contract project add --name "2024采购项目" --format json`},
			},
			Parameters: []contract.ParamDecl{
				{Name: "name", Property: "name", Required: boolPtr(true)},
				{Name: "code", Property: "code"},
				{Name: "owners", Property: "ownerList"},
				{Name: "start-date", Property: "startDate"},
				{Name: "end-date", Property: "endDate"},
				{Name: "remark", Property: "remark"},
				{Name: "contract-ids", Property: "contractIds"},
				{Name: "source", Property: "source"},
			},
		},
	})

	DeclareLeafMetadata(r.ProjectDelete, LeafSpec{
		Safety: safetyDestructive,
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID: "contract", Name: "project_delete", CanonicalPath: "contract.project_delete",
				CLIPath: "contract project delete", PrimaryCLIPath: "contract project delete",
			},
			Description: "按项目 ID 列表删除项目（支持批量）。",
			Interface:   contractCompositeIface,
			Selection: contract.SelectionSpec{
				AgentSummary: "删除合同项目，支持批量。",
				UseWhen:      []string{"用户要删除一个或多个合同项目"},
				AvoidWhen:    []string{"更新项目状态用 project set-status；查项目列表用 project list"},
				Examples:     []string{`dws contract project delete --project-ids "1001,1002" --format json`},
			},
			Parameters: []contract.ParamDecl{
				{Name: "project-ids", Property: "projectIds", Required: boolPtr(true)},
			},
		},
	})

	DeclareLeafMetadata(r.ProjectUpdate, LeafSpec{
		Safety: safetyWrite,
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID: "contract", Name: "project_update", CanonicalPath: "contract.project_update",
				CLIPath: "contract project update", PrimaryCLIPath: "contract project update",
			},
			Description: "更新已有项目信息。",
			Interface:   contractCompositeIface,
			Selection: contract.SelectionSpec{
				AgentSummary: "更新合同项目信息。",
				UseWhen:      []string{"用户要修改已有项目的名称、负责人、日期等"},
				AvoidWhen:    []string{"创建项目用 project add；仅改状态用 project set-status"},
				Examples:     []string{`dws contract project update --project-id 1001 --name "更新后的名称" --format json`},
			},
			Parameters: []contract.ParamDecl{
				{Name: "project-id", Property: "projectId", Required: boolPtr(true)},
				{Name: "name", Property: "name", Required: boolPtr(true)},
				{Name: "code", Property: "code"},
				{Name: "owners", Property: "ownerList"},
				{Name: "start-date", Property: "startDate"},
				{Name: "end-date", Property: "endDate"},
				{Name: "remark", Property: "remark"},
				{Name: "contract-ids", Property: "contractIds"},
			},
		},
	})

	DeclareLeafMetadata(r.ProjectSetStatus, LeafSpec{
		Safety: safetyWrite,
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID: "contract", Name: "project_set_status", CanonicalPath: "contract.project_set_status",
				CLIPath: "contract project set-status", PrimaryCLIPath: "contract project set-status",
			},
			Description: "更新项目状态。",
			Interface:   contractCompositeIface,
			Selection: contract.SelectionSpec{
				AgentSummary: "更新合同项目状态。",
				UseWhen:      []string{"用户要变更项目的状态"},
				AvoidWhen:    []string{"更新项目详情用 project update；查项目列表用 project list"},
				Examples:     []string{`dws contract project set-status --project-id 1001 --status "active" --format json`},
			},
			Parameters: []contract.ParamDecl{
				{Name: "project-id", Property: "projectId", Required: boolPtr(true)},
				{Name: "status", Property: "status", Required: boolPtr(true)},
			},
		},
	})

	DeclareLeafMetadata(r.ProjectList, LeafSpec{
		Safety: safetyRead,
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID: "contract", Name: "project_list", CanonicalPath: "contract.project_list",
				CLIPath: "contract project list", PrimaryCLIPath: "contract project list",
			},
			Description: "分页查询合同项目列表。",
			Interface:   contractCompositeIface,
			Selection: contract.SelectionSpec{
				AgentSummary: "分页查询合同项目列表。",
				UseWhen:      []string{"用户要查询合同项目列表"},
				AvoidWhen:    []string{"查项目摘要用 project digests；查项目详情用 project detail"},
				Examples:     []string{"dws contract project list --current-page 1 --page-size 20 --scope all --format json"},
			},
			Parameters: []contract.ParamDecl{
				{Name: "current-page", Property: "currentPage", Required: boolPtr(true)},
				{Name: "page-size", Property: "pageSize", Required: boolPtr(true)},
				{Name: "scope", Property: "scope", Required: boolPtr(true)},
				{Name: "name", Property: "name"},
				{Name: "code", Property: "code"},
				{Name: "owners", Property: "ownerList"},
				{Name: "status", Property: "status"},
				{Name: "start-date-left", Property: "startDateLeft"},
				{Name: "start-date-right", Property: "startDateRight"},
				{Name: "end-date-left", Property: "endDateLeft"},
				{Name: "end-date-right", Property: "endDateRight"},
			},
		},
	})

	DeclareLeafMetadata(r.ProjectDigests, LeafSpec{
		Safety: safetyRead,
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID: "contract", Name: "project_digests", CanonicalPath: "contract.project_digests",
				CLIPath: "contract project digests", PrimaryCLIPath: "contract project digests",
			},
			Description: "分页查询合同项目摘要列表。",
			Interface:   contractCompositeIface,
			Selection: contract.SelectionSpec{
				AgentSummary: "分页查询合同项目摘要。",
				UseWhen:      []string{"用户要查看项目摘要列表而非完整列表"},
				AvoidWhen:    []string{"要完整项目列表用 project list；要单项目详情用 project detail"},
				Examples:     []string{"dws contract project digests --current-page 1 --page-size 20 --scope all --format json"},
			},
			Parameters: []contract.ParamDecl{
				{Name: "current-page", Property: "currentPage", Required: boolPtr(true)},
				{Name: "page-size", Property: "pageSize", Required: boolPtr(true)},
				{Name: "scope", Property: "scope", Required: boolPtr(true)},
				{Name: "name", Property: "name"},
				{Name: "code", Property: "code"},
				{Name: "owners", Property: "ownerList"},
				{Name: "status", Property: "status"},
				{Name: "start-date-left", Property: "startDateLeft"},
				{Name: "start-date-right", Property: "startDateRight"},
				{Name: "end-date-left", Property: "endDateLeft"},
				{Name: "end-date-right", Property: "endDateRight"},
			},
		},
	})

	DeclareLeafMetadata(r.ProjectDetail, LeafSpec{
		Safety: safetyRead,
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID: "contract", Name: "project_detail", CanonicalPath: "contract.project_detail",
				CLIPath: "contract project detail", PrimaryCLIPath: "contract project detail",
			},
			Description: "按项目 ID 查询项目详情。",
			Interface:   contractCompositeIface,
			Selection: contract.SelectionSpec{
				AgentSummary: "查询合同项目详情。",
				UseWhen:      []string{"用户要查看某项目的完整信息"},
				AvoidWhen:    []string{"查项目列表用 project list；查摘要用 project digests"},
				Examples:     []string{"dws contract project detail --project-id 1001 --format json"},
			},
			Parameters: []contract.ParamDecl{
				{Name: "project-id", Property: "projectId", Required: boolPtr(true)},
			},
		},
	})

	DeclareLeafMetadata(r.ProjectExport, LeafSpec{
		Safety: safetyRead,
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID: "contract", Name: "project_export", CanonicalPath: "contract.project_export",
				CLIPath: "contract project export", PrimaryCLIPath: "contract project export",
			},
			Description: "导出指定项目到 Excel。",
			Interface:   contractCompositeIface,
			Selection: contract.SelectionSpec{
				AgentSummary: "导出合同项目到 Excel。",
				UseWhen:      []string{"用户要导出项目数据到 Excel"},
				AvoidWhen:    []string{"查项目列表用 project list；查项目详情用 project detail"},
				Examples:     []string{`dws contract project export --project-ids "1001,1002" --format json`},
			},
			Parameters: []contract.ParamDecl{
				{Name: "project-ids", Property: "projectIds", Required: boolPtr(true)},
				{Name: "process-code", Property: "processCode"},
			},
		},
	})

	DeclareLeafMetadata(r.ProjectImportTemplate, LeafSpec{
		Safety: safetyRead,
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID: "contract", Name: "project_import_template", CanonicalPath: "contract.project_import_template",
				CLIPath: "contract project import-template", PrimaryCLIPath: "contract project import-template",
			},
			Description: "获取项目批量导入的 Excel 模板下载链接。",
			Interface:   contractCompositeIface,
			Selection: contract.SelectionSpec{
				AgentSummary: "获取项目批量导入模板。",
				UseWhen:      []string{"用户要获取项目批量导入的 Excel 模板"},
				AvoidWhen:    []string{"执行批量导入用 project import；查导入结果用 project import-result"},
				Examples:     []string{"dws contract project import-template --format json"},
			},
		},
	})

	DeclareLeafMetadata(r.ProjectImport, LeafSpec{
		Safety: safetyWrite,
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID: "contract", Name: "project_import", CanonicalPath: "contract.project_import",
				CLIPath: "contract project import", PrimaryCLIPath: "contract project import",
			},
			Description: "从钉盘文件批量导入项目。",
			Interface:   contractCompositeIface,
			Selection: contract.SelectionSpec{
				AgentSummary: "批量导入合同项目。",
				UseWhen:      []string{"用户要从钉盘文件批量导入项目"},
				AvoidWhen:    []string{"单条创建项目用 project add；查导入结果用 project import-result"},
				Examples:     []string{`dws contract project import --file-id "abc123" --space-id 7890 --format json`},
			},
			Parameters: []contract.ParamDecl{
				{Name: "file-id", Property: "fileId", Required: boolPtr(true)},
				{Name: "space-id", Property: "spaceId"},
				{Name: "file-name", Property: "fileName"},
				{Name: "file-type", Property: "fileType"},
				{Name: "file-size", Property: "fileSize"},
			},
		},
	})

	DeclareLeafMetadata(r.ProjectImportResult, LeafSpec{
		Safety: safetyRead,
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID: "contract", Name: "project_import_result", CanonicalPath: "contract.project_import_result",
				CLIPath: "contract project import-result", PrimaryCLIPath: "contract project import-result",
			},
			Description: "按任务 ID 查询项目批量导入结果。",
			Interface:   contractCompositeIface,
			Selection: contract.SelectionSpec{
				AgentSummary: "查询项目批量导入结果。",
				UseWhen:      []string{"用户已执行项目批量导入后要查结果"},
				AvoidWhen:    []string{"执行批量导入用 project import"},
				Examples:     []string{`dws contract project import-result --task-id "task_xxx" --format json`},
			},
			Parameters: []contract.ParamDecl{
				{Name: "task-id", Property: "taskId", Required: boolPtr(true)},
			},
		},
	})

	// ── subject ─────────────────────────────────────────────
	DeclareLeafMetadata(r.SubjectAdd, LeafSpec{
		Safety: safetyWrite,
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID: "contract", Name: "subject_add", CanonicalPath: "contract.subject_add",
				CLIPath: "contract subject add", PrimaryCLIPath: "contract subject add",
			},
			Description: "创建合同相对方。",
			Interface:   contractCompositeIface,
			Selection: contract.SelectionSpec{
				AgentSummary: "添加合同相对方。",
				UseWhen:      []string{"用户要创建新的合同相对方"},
				AvoidWhen:    []string{"修改相对方用 subject update；查相对方列表用 subject list"},
				Examples:     []string{"dws contract subject add --file ./subject.json --format json"},
			},
			Parameters: []contract.ParamDecl{
				{Name: "file", Property: "AddSubjectOpenRequest", Required: boolPtr(true)},
			},
		},
	})

	DeclareLeafMetadata(r.SubjectList, LeafSpec{
		Safety: safetyRead,
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID: "contract", Name: "subject_list", CanonicalPath: "contract.subject_list",
				CLIPath: "contract subject list", PrimaryCLIPath: "contract subject list",
			},
			Description: "分页查询相对方列表。",
			Interface:   contractCompositeIface,
			Selection: contract.SelectionSpec{
				AgentSummary: "分页查询合同相对方列表。",
				UseWhen:      []string{"用户要查询合同相对方列表"},
				AvoidWhen:    []string{"查单条相对方详情用 subject detail；添加相对方用 subject add"},
				Examples:     []string{"dws contract subject list --current-page 1 --page-size 20 --format json"},
			},
			Parameters: []contract.ParamDecl{
				{Name: "current-page", Property: "currentPage", Required: boolPtr(true)},
				{Name: "page-size", Property: "pageSize", Required: boolPtr(true)},
				{Name: "party-type", Property: "partyType"},
				{Name: "name", Property: "name"},
				{Name: "code", Property: "code"},
				{Name: "source", Property: "source"},
			},
		},
	})

	DeclareLeafMetadata(r.SubjectDetail, LeafSpec{
		Safety: safetyRead,
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID: "contract", Name: "subject_detail", CanonicalPath: "contract.subject_detail",
				CLIPath: "contract subject detail", PrimaryCLIPath: "contract subject detail",
			},
			Description: "按相对方 ID 查询详情。",
			Interface:   contractCompositeIface,
			Selection: contract.SelectionSpec{
				AgentSummary: "查询合同相对方详情。",
				UseWhen:      []string{"用户要查看某相对方的完整信息"},
				AvoidWhen:    []string{"查相对方列表用 subject list"},
				Examples:     []string{"dws contract subject detail --subject-id 2001 --format json"},
			},
			Parameters: []contract.ParamDecl{
				{Name: "subject-id", Property: "subjectId", Required: boolPtr(true)},
			},
		},
	})

	DeclareLeafMetadata(r.SubjectUpdate, LeafSpec{
		Safety: safetyWrite,
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID: "contract", Name: "subject_update", CanonicalPath: "contract.subject_update",
				CLIPath: "contract subject update", PrimaryCLIPath: "contract subject update",
			},
			Description: "修改已有相对方信息。",
			Interface:   contractCompositeIface,
			Selection: contract.SelectionSpec{
				AgentSummary: "修改合同相对方信息。",
				UseWhen:      []string{"用户要修改已有相对方的信息"},
				AvoidWhen:    []string{"添加相对方用 subject add；删除相对方用 subject delete"},
				Examples:     []string{"dws contract subject update --file ./subject_update.json --format json"},
			},
			Parameters: []contract.ParamDecl{
				{Name: "file", Property: "UpdateSubjectOpenRequest", Required: boolPtr(true)},
			},
		},
	})

	DeclareLeafMetadata(r.SubjectDelete, LeafSpec{
		Safety: safetyDestructive,
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID: "contract", Name: "subject_delete", CanonicalPath: "contract.subject_delete",
				CLIPath: "contract subject delete", PrimaryCLIPath: "contract subject delete",
			},
			Description: "按相对方 ID 删除单个相对方。",
			Interface:   contractCompositeIface,
			Selection: contract.SelectionSpec{
				AgentSummary: "删除合同相对方。",
				UseWhen:      []string{"用户要删除单个相对方"},
				AvoidWhen:    []string{"批量删除用 subject batch-delete；修改相对方用 subject update"},
				Examples:     []string{"dws contract subject delete --subject-id 2001 --format json"},
			},
			Parameters: []contract.ParamDecl{
				{Name: "subject-id", Property: "subjectId", Required: boolPtr(true)},
			},
		},
	})

	DeclareLeafMetadata(r.SubjectBatchDelete, LeafSpec{
		Safety: safetyDestructive,
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID: "contract", Name: "subject_batch_delete", CanonicalPath: "contract.subject_batch_delete",
				CLIPath: "contract subject batch-delete", PrimaryCLIPath: "contract subject batch-delete",
			},
			Description: "按相对方 ID 列表批量删除，一次最多 1000 个。",
			Interface:   contractCompositeIface,
			Selection: contract.SelectionSpec{
				AgentSummary: "批量删除合同相对方。",
				UseWhen:      []string{"用户要一次删除多个相对方"},
				AvoidWhen:    []string{"删除单个用 subject delete；修改用 subject update"},
				Examples:     []string{`dws contract subject batch-delete --subject-ids "2001,2002,2003" --format json`},
			},
			Parameters: []contract.ParamDecl{
				{Name: "subject-ids", Property: "subjectIdList", Required: boolPtr(true)},
			},
		},
	})

	DeclareLeafMetadata(r.SubjectSort, LeafSpec{
		Safety: safetyWrite,
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID: "contract", Name: "subject_sort", CanonicalPath: "contract.subject_sort",
				CLIPath: "contract subject sort", PrimaryCLIPath: "contract subject sort",
			},
			Description: "设置己方主体的展示顺序。",
			Interface:   contractCompositeIface,
			Selection: contract.SelectionSpec{
				AgentSummary: "对己方主体进行排序。",
				UseWhen:      []string{"用户要调整己方主体的展示顺序"},
				AvoidWhen:    []string{"查相对方列表用 subject list；修改相对方用 subject update"},
				Examples:     []string{`dws contract subject sort --subject-ids "2001,2003,2002" --format json`},
			},
			Parameters: []contract.ParamDecl{
				{Name: "subject-ids", Property: "subjectIdList", Required: boolPtr(true)},
			},
		},
	})

	DeclareLeafMetadata(r.SubjectDetectRisk, LeafSpec{
		Safety: safetyRead,
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID: "contract", Name: "subject_detect_risk", CanonicalPath: "contract.subject_detect_risk",
				CLIPath: "contract subject detect-risk", PrimaryCLIPath: "contract subject detect-risk",
			},
			Description: "检测相对方风险信息。",
			Interface:   contractCompositeIface,
			Selection: contract.SelectionSpec{
				AgentSummary: "检测合同相对方风险。",
				UseWhen:      []string{"用户要检测相对方的风险信息"},
				AvoidWhen:    []string{"查工商基本信息用 subject base-info；智能填充用 subject auto-fill"},
				Examples:     []string{`dws contract subject detect-risk --subject-name "北京示例科技有限公司" --format json`},
			},
			Parameters: []contract.ParamDecl{
				{Name: "subject-name", Property: "subjectName", Required: boolPtr(true)},
				{Name: "subject-id", Property: "subjectId"},
			},
		},
	})

	DeclareLeafMetadata(r.SubjectBaseInfo, LeafSpec{
		Safety: safetyRead,
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID: "contract", Name: "subject_base_info", CanonicalPath: "contract.subject_base_info",
				CLIPath: "contract subject base-info", PrimaryCLIPath: "contract subject base-info",
			},
			Description: "查询相对方工商基本信息。",
			Interface:   contractCompositeIface,
			Selection: contract.SelectionSpec{
				AgentSummary: "查询相对方工商基本信息。",
				UseWhen:      []string{"用户要查相对方的工商信息"},
				AvoidWhen:    []string{"检测风险用 subject detect-risk；智能填充用 subject auto-fill"},
				Examples:     []string{`dws contract subject base-info --subject-name "北京示例科技有限公司" --format json`},
			},
			Parameters: []contract.ParamDecl{
				{Name: "subject-name", Property: "subjectName", Required: boolPtr(true)},
				{Name: "subject-id", Property: "subjectId"},
			},
		},
	})

	DeclareLeafMetadata(r.SubjectAutoFill, LeafSpec{
		Safety: safetyRead,
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID: "contract", Name: "subject_auto_fill", CanonicalPath: "contract.subject_auto_fill",
				CLIPath: "contract subject auto-fill", PrimaryCLIPath: "contract subject auto-fill",
			},
			Description: "根据相对方名称智能填充详细信息。",
			Interface:   contractCompositeIface,
			Selection: contract.SelectionSpec{
				AgentSummary: "智能填充相对方详细信息。",
				UseWhen:      []string{"用户要根据相对方名称自动填充详细信息"},
				AvoidWhen:    []string{"查工商基本信息用 subject base-info；检测风险用 subject detect-risk"},
				Examples:     []string{`dws contract subject auto-fill --subject-name "北京示例科技有限公司" --format json`},
			},
			Parameters: []contract.ParamDecl{
				{Name: "subject-name", Property: "subjectName", Required: boolPtr(true)},
				{Name: "subject-id", Property: "subjectId"},
			},
		},
	})

	DeclareLeafMetadata(r.SubjectExport, LeafSpec{
		Safety: safetyRead,
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID: "contract", Name: "subject_export", CanonicalPath: "contract.subject_export",
				CLIPath: "contract subject export", PrimaryCLIPath: "contract subject export",
			},
			Description: "导出指定相对方到 Excel。",
			Interface:   contractCompositeIface,
			Selection: contract.SelectionSpec{
				AgentSummary: "导出合同相对方到 Excel。",
				UseWhen:      []string{"用户要导出相对方数据到 Excel"},
				AvoidWhen:    []string{"查相对方列表用 subject list；查详情用 subject detail"},
				Examples:     []string{`dws contract subject export --subject-ids "2001,2002" --format json`},
			},
			Parameters: []contract.ParamDecl{
				{Name: "subject-ids", Property: "subjectIds", Required: boolPtr(true)},
				{Name: "process-code", Property: "processCode"},
			},
		},
	})

	DeclareLeafMetadata(r.SubjectImportTemplate, LeafSpec{
		Safety: safetyRead,
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID: "contract", Name: "subject_import_template", CanonicalPath: "contract.subject_import_template",
				CLIPath: "contract subject import-template", PrimaryCLIPath: "contract subject import-template",
			},
			Description: "获取相对方批量导入的 Excel 模板下载链接。",
			Interface:   contractCompositeIface,
			Selection: contract.SelectionSpec{
				AgentSummary: "获取相对方批量导入模板。",
				UseWhen:      []string{"用户要获取相对方批量导入的 Excel 模板"},
				AvoidWhen:    []string{"执行批量导入用 subject import；查导入结果用 subject import-result"},
				Examples:     []string{"dws contract subject import-template --format json"},
			},
			Parameters: []contract.ParamDecl{
				{Name: "type", Property: "type"},
			},
		},
	})

	DeclareLeafMetadata(r.SubjectImport, LeafSpec{
		Safety: safetyWrite,
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID: "contract", Name: "subject_import", CanonicalPath: "contract.subject_import",
				CLIPath: "contract subject import", PrimaryCLIPath: "contract subject import",
			},
			Description: "从钉盘文件批量导入相对方。",
			Interface:   contractCompositeIface,
			Selection: contract.SelectionSpec{
				AgentSummary: "批量导入合同相对方。",
				UseWhen:      []string{"用户要从钉盘文件批量导入相对方"},
				AvoidWhen:    []string{"单条添加用 subject add；查导入结果用 subject import-result"},
				Examples:     []string{`dws contract subject import --file-id "abc123" --space-id 7890 --format json`},
			},
			Parameters: []contract.ParamDecl{
				{Name: "file-id", Property: "fileId", Required: boolPtr(true)},
				{Name: "space-id", Property: "spaceId"},
				{Name: "file-name", Property: "fileName"},
				{Name: "file-type", Property: "fileType"},
				{Name: "file-size", Property: "fileSize"},
			},
		},
	})

	DeclareLeafMetadata(r.SubjectImportResult, LeafSpec{
		Safety: safetyRead,
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID: "contract", Name: "subject_import_result", CanonicalPath: "contract.subject_import_result",
				CLIPath: "contract subject import-result", PrimaryCLIPath: "contract subject import-result",
			},
			Description: "按任务 ID 查询相对方批量导入结果。",
			Interface:   contractCompositeIface,
			Selection: contract.SelectionSpec{
				AgentSummary: "查询相对方批量导入结果。",
				UseWhen:      []string{"用户已执行相对方批量导入后要查结果"},
				AvoidWhen:    []string{"执行批量导入用 subject import"},
				Examples:     []string{`dws contract subject import-result --task-id "task_xxx" --format json`},
			},
			Parameters: []contract.ParamDecl{
				{Name: "task-id", Property: "taskId", Required: boolPtr(true)},
			},
		},
	})
}
