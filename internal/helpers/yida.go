package helpers

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
)

// splitYidaList parses a comma-separated CLI value into a []string for MCP
// payloads that expect array fields (e.g. task list --apps appTypes,
// --process-codes processCodes). Trims whitespace and drops empties.
func splitYidaList(v string) []string {
	parts := strings.Split(v, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if s := strings.TrimSpace(p); s != "" {
			out = append(out, s)
		}
	}
	return out
}

// ──────────────────────────────────────────────────────────
// dws yida — 宜搭
// MCP tools（tools/list）：
//   search_form_data, get_inst_detail, create_form_data, update_form_data,
//   get_form_components,
//   list_apps, list_app_forms, create_app,
//   get_todo_tasks, get_process_running_tasks, get_process_op_records,
//   execute_task, redirect_task, start_process_instance,
//   create_form, get_form_info, get_form_schema,
//   save_form_schema, update_form_title, preview_form,
//   create_process_flow, save_process, publish_process,
//   get_process, list_process_versions
// 	 create_manage_automation, update_manage_automation,update_app_permission,get_form_components
//
// CLI ↔ MCP 工具名映射（部分非对称，CLI 跟 cr-rules 推荐动词，MCP 跟上游不变）：
//   dws yida design form update-schema      → save_form_schema
//   dws yida design process update          → save_process
//   dws yida design process versions list   → list_process_versions
// ──────────────────────────────────────────────────────────

// validateYidaEnum 校验 string 值是否在允许集合内；不在则返回友好错误。
func validateYidaEnum(flagName, val string, allowed ...string) error {
	for _, a := range allowed {
		if val == a {
			return nil
		}
	}
	return fmt.Errorf("--%s must be one of: %s; got: %q",
		flagName, strings.Join(allowed, " / "), val)
}

// isAllDigits 检查纯数字（用于 process-id 校验）。
func isAllDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

// confirmYidaAction 是 yida 写命令的兜底确认。复用 helpers.go::confirmDelete 的 --yes / TTY
// prompt / 非 TTY 拒绝逻辑，把 verb 塞进 resourceType 让 prompt 文案匹配「保存 / 发布」等非
// delete 语义。仅用于 update-schema / process update / process publish 三个覆盖式写。
func confirmYidaAction(cmd *cobra.Command, verb, target string) bool {
	return confirmDelete(verb, target)
}

// yidaParsePageSize parses and validates pagination flags with defaults (page=1, size=20).
// page must be > 0, size must be between 1 and 100.
func yidaParsePageSize(cmd *cobra.Command, pageFlag, sizeFlag string) (float64, float64, error) {
	page, size := 1.0, 20.0
	if v, _ := cmd.Flags().GetString(pageFlag); v != "" {
		if n, err := strconv.ParseFloat(v, 64); err != nil || n < 1 {
			return 0, 0, fmt.Errorf("--%s must be greater than 0", pageFlag)
		} else {
			page = n
		}
	}
	if v, _ := cmd.Flags().GetString(sizeFlag); v != "" {
		if n, err := strconv.ParseFloat(v, 64); err != nil || n < 1 || n > 100 {
			return 0, 0, fmt.Errorf("--%s must be between 1 and 100", sizeFlag)
		} else {
			size = n
		}
	}
	return page, size, nil
}

func newYidaCommand() *cobra.Command {
	root := &cobra.Command{
		Use:   "yida",
		Short: "宜搭（应用 / 表单 / 流程审批）",
		Long:  `管理宜搭：应用列表、表单查询、数据详情、流程审批（待办、执行、转交）、审批记录。`,
		RunE:  groupRunE,
	}

	// ── app 子命令 ────────────────────────────────────────────
	appCmd := &cobra.Command{Use: "app", Short: "应用管理", RunE: groupRunE}

	appListCmd := &cobra.Command{
		Use:     "list",
		Short:   "获取宜搭应用列表",
		Example: `  dws yida app list --keyword "项目管理" --page 1 --size 20`,
		RunE: func(cmd *cobra.Command, args []string) error {
			argsMap := map[string]any{}
			if v, _ := cmd.Flags().GetString("keyword"); v != "" {
				argsMap["keyword"] = v
			}
			if v, _ := cmd.Flags().GetString("filter"); v != "" {
				argsMap["filter"] = v
			}
			page, size, err := yidaParsePageSize(cmd, "page", "size")
			if err != nil {
				return err
			}
			argsMap["pageNumber"] = page
			argsMap["pageSize"] = size
			if v, _ := cmd.Flags().GetString("language"); v != "" {
				argsMap["language"] = v
			}
			return callMCPTool("list_apps", argsMap)
		},
	}

	appListFormsCmd := &cobra.Command{
		Use:     "list-forms",
		Short:   "获取应用内表单列表",
		Example: `  dws yida app list-forms --app <appType> --form-types receipt --page 1 --size 20`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "app"); err != nil {
				return err
			}
			argsMap := map[string]any{
				"appType": mustGetFlag(cmd, "app"),
			}
			if v, _ := cmd.Flags().GetString("form-types"); v != "" {
				argsMap["formTypes"] = v
			}
			page, size, err := yidaParsePageSize(cmd, "page", "size")
			if err != nil {
				return err
			}
			argsMap["currentPage"] = page
			argsMap["pageSize"] = size
			if v, _ := cmd.Flags().GetString("language"); v != "" {
				argsMap["language"] = v
			}
			return callMCPTool("list_app_forms", argsMap)
		},
	}

	// ── form 子命令（定义态）─────────────────────────────────
	formCmd := &cobra.Command{Use: "form", Short: "表单定义管理", RunE: groupRunE}

	// ── data 子命令（实例态）─────────────────────────────────
	dataCmd := &cobra.Command{Use: "data", Short: "表单数据管理", RunE: groupRunE}

	formComponentsCmd := &cobra.Command{
		Use:     "components",
		Short:   "获取表单字段定义",
		Example: `  dws yida form components --app <appType> --form <formUuid>`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "app", "form"); err != nil {
				return err
			}
			argsMap := map[string]any{
				"appType":  mustGetFlag(cmd, "app"),
				"formUuid": mustGetFlag(cmd, "form"),
			}
			if v, _ := cmd.Flags().GetString("language"); v != "" {
				argsMap["language"] = v
			}
			if v, _ := cmd.Flags().GetString("version"); v != "" {
				argsMap["version"] = v
			}
			return callMCPTool("get_form_components", argsMap)
		},
	}

	dataSearchCmd := &cobra.Command{
		Use:   "search",
		Short: "表单数据条件查询",
		Long:  `按条件筛选并分页查询宜搭表单的数据记录，支持多字段组合过滤、排序和模糊搜索。`,
		Example: `  dws yida data search --app <appType> --form <formUuid>
  dws yida data search --app <appType> --form <formUuid> --page 1 --size 20
  dws yida data search --app <appType> --form <formUuid> --search-field '{"textField_abc":"hello"}'`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "app", "form"); err != nil {
				return err
			}
			argsMap := map[string]any{
				"appType":  mustGetFlag(cmd, "app"),
				"formUuid": mustGetFlag(cmd, "form"),
			}
			page, size, err := yidaParsePageSize(cmd, "page", "size")
			if err != nil {
				return err
			}
			argsMap["currentPage"] = page
			argsMap["pageSize"] = size
			if v, _ := cmd.Flags().GetString("search-field"); v != "" {
				argsMap["searchFieldJson"] = v
			}
			if v, _ := cmd.Flags().GetBool("use-alias"); v {
				argsMap["useAlias"] = true
			}
			if v, _ := cmd.Flags().GetString("originator-id"); v != "" {
				argsMap["originatorId"] = v
			}
			if v, _ := cmd.Flags().GetString("create-from"); v != "" {
				if ms, err := parseISOTimeToMillis("create-from", v); err == nil {
					argsMap["createFrom"] = float64(ms)
				} else {
					return err
				}
			}
			if v, _ := cmd.Flags().GetString("create-to"); v != "" {
				if ms, err := parseISOTimeToMillis("create-to", v); err == nil {
					argsMap["createTo"] = float64(ms)
				} else {
					return err
				}
			}
			if v, _ := cmd.Flags().GetString("modified-from"); v != "" {
				if ms, err := parseISOTimeToMillis("modified-from", v); err == nil {
					argsMap["modifiedFrom"] = float64(ms)
				} else {
					return err
				}
			}
			if v, _ := cmd.Flags().GetString("modified-to"); v != "" {
				if ms, err := parseISOTimeToMillis("modified-to", v); err == nil {
					argsMap["modifiedTo"] = float64(ms)
				} else {
					return err
				}
			}
			if v, _ := cmd.Flags().GetString("language"); v != "" {
				argsMap["language"] = v
			}
			return callMCPTool("search_form_data", argsMap)
		},
	}

	dataDetailCmd := &cobra.Command{
		Use:     "detail",
		Short:   "获取单条记录详情",
		Example: `  dws yida data detail --app <appType> --instance-id <formInstId>`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "app", "instance-id"); err != nil {
				return err
			}
			argsMap := map[string]any{
				"appType":    mustGetFlag(cmd, "app"),
				"formInstId": mustGetFlag(cmd, "instance-id"),
			}
			if v, _ := cmd.Flags().GetString("form"); v != "" {
				argsMap["formUuid"] = v
			}
			if v, _ := cmd.Flags().GetString("need-inst-value"); v != "" {
				argsMap["needInstValue"] = v
			}
			return callMCPTool("get_inst_detail", argsMap)
		},
	}

	dataCreateCmd := &cobra.Command{
		Use:   "create",
		Short: "新增表单实例",
		Long:  `新增一条普通表单实例，返回新实例 ID。流程表单发起审批请使用 process start。`,
		Example: `  dws yida data create --app <appType> --form <formUuid> --form-data '{"textField_name":"张三","numberField_amount":1280}'
  dws yida data create --app <appType> --form <formUuid> --form-data '{"name":"张三","amount":1280}' --use-alias`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "app", "form", "form-data"); err != nil {
				return err
			}
			argsMap := map[string]any{
				"appType":      mustGetFlag(cmd, "app"),
				"formUuid":     mustGetFlag(cmd, "form"),
				"formDataJson": mustGetFlag(cmd, "form-data"),
			}
			if v, _ := cmd.Flags().GetString("instance-id"); v != "" {
				argsMap["formInstId"] = v
			}
			if v, _ := cmd.Flags().GetBool("use-alias"); v {
				argsMap["useAlias"] = true
			}
			if v, _ := cmd.Flags().GetString("language"); v != "" {
				argsMap["language"] = v
			}
			return callMCPTool("create_form_data", argsMap)
		},
	}

	dataUpdateCmd := &cobra.Command{
		Use:   "update",
		Short: "修改表单实例",
		Long:  `按实例 ID 修改一条普通表单实例。该命令不是 upsert，不会在实例不存在时新增数据。`,
		Example: `  dws yida data update --app <appType> --form <formUuid> --instance-id <formInstId> --form-data '{"numberField_amount":1680}'
  dws yida data update --app <appType> --form <formUuid> --instance-id <formInstId> --form-data '{"amount":1680}' --use-alias`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "app", "form", "instance-id", "form-data"); err != nil {
				return err
			}
			argsMap := map[string]any{
				"appType":            mustGetFlag(cmd, "app"),
				"formUuid":           mustGetFlag(cmd, "form"),
				"formInstId":         mustGetFlag(cmd, "instance-id"),
				"updateFormDataJson": mustGetFlag(cmd, "form-data"),
			}
			if v, _ := cmd.Flags().GetBool("use-alias"); v {
				argsMap["useAlias"] = true
			}
			if v, _ := cmd.Flags().GetString("language"); v != "" {
				argsMap["language"] = v
			}
			return callMCPTool("update_form_data", argsMap)
		},
	}

	dataExportStartCmd := &cobra.Command{
		Use:   "export-start",
		Short: "触发异步导出表单数据",
		Long:  `异步触发导出表单数据任务，返回 sequence 用于后续查询导出结果。`,
		Example: `  dws yida data export-start --app <appType> --form <formUuid> --fields '["processInstanceTitle","textField_abc","numberField_def","originator","createTime"]'
  dws yida data export-start --app <appType> --form <formUuid> --fields '["textField_abc"]' --filter '{"key":"textField_abc","value":"hello","type":"TEXT","operator":"like","componentName":"TextField"}'`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "app", "form", "fields"); err != nil {
				return err
			}
			argsMap := map[string]any{
				"appType":        mustGetFlag(cmd, "app"),
				"formUuid":       mustGetFlag(cmd, "form"),
				"fieldFilterVal": mustGetFlag(cmd, "fields"),
			}
			if v, _ := cmd.Flags().GetString("view"); v != "" {
				argsMap["viewUuid"] = v
			}
			if v, _ := cmd.Flags().GetString("filter"); v != "" {
				argsMap["filterRule"] = v
			}
			if v, _ := cmd.Flags().GetString("sort"); v != "" {
				argsMap["sortRule"] = v
			}
			if v, _ := cmd.Flags().GetString("language"); v != "" {
				argsMap["language"] = v
			}
			return callMCPTool("export_form_data_start", argsMap)
		},
	}

	dataExportQueryCmd := &cobra.Command{
		Use:     "export-query",
		Short:   "查询导出任务结果（返回下载 URL）",
		Long:    `通过 export-start 返回的 sequence 查询导出进度，完成后返回 OSS 下载直链。`,
		Example: `  dws yida data export-query --app <appType> --sequence <sequence>`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "app", "sequence"); err != nil {
				return err
			}
			argsMap := map[string]any{
				"appType":  mustGetFlag(cmd, "app"),
				"sequence": mustGetFlag(cmd, "sequence"),
			}
			if v, _ := cmd.Flags().GetString("language"); v != "" {
				argsMap["language"] = v
			}
			return callMCPTool("export_form_data_query", argsMap)
		},
	}

	// ── task 子命令（待办任务）─────────────────────────────────
	taskCmd := &cobra.Command{Use: "task", Short: "待办任务管理", RunE: groupRunE}

	taskListCmd := &cobra.Command{
		Use:   "list",
		Short: "查询当前用户待办任务",
		Long:  `查询当前用户在宜搭中的待办审批任务列表，支持按应用、流程、时间范围、关键词等多维过滤。`,
		Example: `  dws yida task list
  dws yida task list --apps <appType> --keyword "报销"
  dws yida task list --page 1 --size 20`,
		RunE: func(cmd *cobra.Command, args []string) error {
			argsMap := map[string]any{}
			if v, _ := cmd.Flags().GetString("apps"); v != "" {
				argsMap["appTypes"] = splitYidaList(v)
			}
			if v, _ := cmd.Flags().GetString("process-codes"); v != "" {
				argsMap["processCodes"] = splitYidaList(v)
			}
			if v, _ := cmd.Flags().GetString("keyword"); v != "" {
				argsMap["keyword"] = v
			}
			if v, _ := cmd.Flags().GetString("status"); v != "" {
				argsMap["status"] = v
			}
			page, size, err := yidaParsePageSize(cmd, "page", "size")
			if err != nil {
				return err
			}
			argsMap["page"] = page
			argsMap["limit"] = size
			if v, _ := cmd.Flags().GetString("create-from"); v != "" {
				if ms, err := parseISOTimeToMillis("create-from", v); err == nil {
					argsMap["createFrom"] = float64(ms)
				} else {
					return err
				}
			}
			if v, _ := cmd.Flags().GetString("create-to"); v != "" {
				if ms, err := parseISOTimeToMillis("create-to", v); err == nil {
					argsMap["createTo"] = float64(ms)
				} else {
					return err
				}
			}
			if v, _ := cmd.Flags().GetString("instance-create-from"); v != "" {
				if ms, err := parseISOTimeToMillis("instance-create-from", v); err == nil {
					argsMap["instanceCreateFrom"] = float64(ms)
				} else {
					return err
				}
			}
			if v, _ := cmd.Flags().GetString("instance-create-to"); v != "" {
				if ms, err := parseISOTimeToMillis("instance-create-to", v); err == nil {
					argsMap["instanceCreateTo"] = float64(ms)
				} else {
					return err
				}
			}
			if v, _ := cmd.Flags().GetString("task-finish-from"); v != "" {
				if ms, err := parseISOTimeToMillis("task-finish-from", v); err == nil {
					argsMap["taskFinishFrom"] = float64(ms)
				} else {
					return err
				}
			}
			if v, _ := cmd.Flags().GetString("task-finish-to"); v != "" {
				if ms, err := parseISOTimeToMillis("task-finish-to", v); err == nil {
					argsMap["taskFinishTo"] = float64(ms)
				} else {
					return err
				}
			}
			return callMCPTool("get_todo_tasks", argsMap)
		},
	}

	// ── process 子命令（流程审批）───────────────────────────────
	processCmd := &cobra.Command{Use: "process", Short: "流程审批管理", RunE: groupRunE}

	processRunningTasksCmd := &cobra.Command{
		Use:     "running-tasks",
		Short:   "查询流程运行中的任务节点",
		Example: `  dws yida process running-tasks --app <appType> --instance-id <processInstanceId>`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "app", "instance-id"); err != nil {
				return err
			}
			argsMap := map[string]any{
				"appType":           mustGetFlag(cmd, "app"),
				"processInstanceId": mustGetFlag(cmd, "instance-id"),
			}
			if v, _ := cmd.Flags().GetString("language"); v != "" {
				argsMap["language"] = v
			}
			return callMCPTool("get_process_running_tasks", argsMap)
		},
	}

	processRecordsCmd := &cobra.Command{
		Use:     "records",
		Short:   "获取流程审批操作记录",
		Example: `  dws yida process records --app <appType> --instance-id <processInstanceId>`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "app", "instance-id"); err != nil {
				return err
			}
			argsMap := map[string]any{
				"appType":           mustGetFlag(cmd, "app"),
				"processInstanceId": mustGetFlag(cmd, "instance-id"),
			}
			if v, _ := cmd.Flags().GetString("language"); v != "" {
				argsMap["language"] = v
			}
			return callMCPTool("get_process_op_records", argsMap)
		},
	}

	processExecuteCmd := &cobra.Command{
		Use:   "execute",
		Short: "执行审批（同意/拒绝）",
		Long: `对指定任务执行审批操作，支持同意或拒绝，可附带审批意见和表单数据修改。
**CAUTION:** 审批决策不可撤回 — 执行前必须向用户确认。`,
		Example: `  dws yida process execute --app <appType> --form <formUuid> --task <taskId> --result "agree"
  dws yida process execute --app <appType> --form <formUuid> --task <taskId> --result "disagree" --remark "不符合要求"`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "app", "form", "task", "result"); err != nil {
				return err
			}
			taskIdNum, _ := strconv.ParseFloat(mustGetFlag(cmd, "task"), 64)
			argsMap := map[string]any{
				"appType":   mustGetFlag(cmd, "app"),
				"formUuid":  mustGetFlag(cmd, "form"),
				"taskId":    taskIdNum,
				"outResult": mustGetFlag(cmd, "result"),
			}
			if v, _ := cmd.Flags().GetString("remark"); v != "" {
				argsMap["remark"] = v
			} else {
				argsMap["remark"] = mustGetFlag(cmd, "result")
			}
			if v, _ := cmd.Flags().GetString("instance-id"); v != "" {
				argsMap["procInstId"] = v
			}
			if v, _ := cmd.Flags().GetString("form-data"); v != "" {
				argsMap["formDataJson"] = v
			}
			if v, _ := cmd.Flags().GetString("digital-sign-url"); v != "" {
				argsMap["digitalSignUrl"] = v
			}
			if v, _ := cmd.Flags().GetBool("no-execute-expressions"); v {
				argsMap["noExecuteExpressions"] = true
			}
			return callMCPTool("execute_task", argsMap)
		},
	}

	processRedirectCmd := &cobra.Command{
		Use:   "redirect",
		Short: "转交审批任务",
		Long: `将当前审批任务转交给其他处理人。
**CAUTION:** 转交操作不可撤回 — 执行前必须向用户确认。`,
		Example: `  dws yida process redirect --app <appType> --instance-id <processInstanceId> --task <taskId>  --to-user <userId>
  dws yida process redirect --app <appType> --instance-id <processInstanceId> --task <taskId>  --to-user <userId> --remark "请帮忙处理"`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "app", "task", "instance-id", "to-user"); err != nil {
				return err
			}
			taskIdNum, _ := strconv.ParseFloat(mustGetFlag(cmd, "task"), 64)
			argsMap := map[string]any{
				"appType":           mustGetFlag(cmd, "app"),
				"taskId":            taskIdNum,
				"processInstanceId": mustGetFlag(cmd, "instance-id"),
				"nowActionerId":     mustGetFlag(cmd, "to-user"),
			}
			if v, _ := cmd.Flags().GetString("form"); v != "" {
				argsMap["formUuid"] = v
			}
			if v, _ := cmd.Flags().GetString("remark"); v != "" {
				argsMap["remark"] = v
			}
			if v, _ := cmd.Flags().GetBool("by-manager"); v {
				argsMap["byManager"] = true
			}
			return callMCPTool("redirect_task", argsMap)
		},
	}

	processStartCmd := &cobra.Command{
		Use:   "start",
		Short: "发起流程表单实例",
		Long: `在宜搭中发起一个新的流程表单实例，支持传入表单数据和流程配置。
**CAUTION:** 发起流程后将触发审批流转 — 执行前必须向用户确认。`,
		Example: `  dws yida process start --app <appType> --form <formUuid> --form-data '{"textField_abc":"hello"}'
  dws yida process start --app <appType> --form <formUuid> --form-data '{"textField_abc":"hello"}' --dept-id <deptId>
  dws yida process start --app <appType> --form <formUuid> --form-data '{"textField_abc":"hello"}' --process-code <processCode> --use-alias`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "app", "form", "form-data"); err != nil {
				return err
			}
			argsMap := map[string]any{
				"appType":      mustGetFlag(cmd, "app"),
				"formUuid":     mustGetFlag(cmd, "form"),
				"formDataJson": mustGetFlag(cmd, "form-data"),
			}
			if v, _ := cmd.Flags().GetString("dept-id"); v != "" {
				argsMap["deptId"] = v
			}
			if v, _ := cmd.Flags().GetString("process-code"); v != "" {
				argsMap["processCode"] = v
			}
			if v, _ := cmd.Flags().GetString("process-data"); v != "" {
				argsMap["processData"] = v
			}
			if v, _ := cmd.Flags().GetString("business-id"); v != "" {
				argsMap["businessId"] = v
			}
			if v, _ := cmd.Flags().GetString("instance-id"); v != "" {
				argsMap["processInstanceId"] = v
			}
			if v, _ := cmd.Flags().GetBool("use-alias"); v {
				argsMap["useAlias"] = true
			}
			if v, _ := cmd.Flags().GetString("language"); v != "" {
				argsMap["language"] = v
			}
			return callMCPTool("start_process_instance", argsMap)
		},
	}

	// ── automation 子命令（自动化流程管理）────────────────────────
	automationCmd := &cobra.Command{Use: "automation", Short: "自动化流程管理", RunE: groupRunE}

	automationCreateCmd := &cobra.Command{
		Use:   "create",
		Short: "创建自动化流程",
		Long:  `创建一条空白自动化流程，返回 processCode 供后续更新节点使用。`,
		Example: `  dws yida automation create --app <appType> --form <formUuid> --name "我的自动化" --type 1
  dws yida automation create --app <appType> --form <formUuid> --name "定时任务" --type 2 --desc "每日定时触发"`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "app", "form", "name", "type"); err != nil {
				return err
			}
			typeVal, err := strconv.ParseFloat(mustGetFlag(cmd, "type"), 64)
			if err != nil {
				return fmt.Errorf("--type must be a number (1=表单事件 / 2=定时 / 3=应用级)")
			}
			argsMap := map[string]any{
				"appType":  mustGetFlag(cmd, "app"),
				"formUuid": mustGetFlag(cmd, "form"),
				"name":     mustGetFlag(cmd, "name"),
				"type":     typeVal,
			}
			if v, _ := cmd.Flags().GetString("desc"); v != "" {
				argsMap["flowDesc"] = v
			}
			if v, _ := cmd.Flags().GetString("language"); v != "" {
				argsMap["language"] = v
			}
			return callMCPTool("create_manage_automation", argsMap)
		},
	}

	automationUpdateCmd := &cobra.Command{
		Use:   "update",
		Short: "更新自动化流程节点",
		Long: `更新指定自动化流程的节点 schema 配置。
通常与 automation create 配合使用：先创建流程获取 processCode，再构造节点 JSON 后更新。`,
		Example: `  dws yida automation update --app <appType> --process-code <processCode> --json '<节点设置JSON>'
  dws yida automation update --app <appType> --process-code <processCode> --json '<节点设置JSON>' --view-json '<预览JSON>'`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "app", "process-code", "json"); err != nil {
				return err
			}
			argsMap := map[string]any{
				"appType":     mustGetFlag(cmd, "app"),
				"processCode": mustGetFlag(cmd, "process-code"),
				"json":        mustGetFlag(cmd, "json"),
			}
			if v, _ := cmd.Flags().GetString("view-json"); v != "" {
				argsMap["viewJson"] = v
			}
			if v, _ := cmd.Flags().GetString("language"); v != "" {
				argsMap["language"] = v
			}
			return callMCPTool("update_manage_automation", argsMap)
		},
	}

	appUpdatePermCmd := &cobra.Command{
		Use:   "update-permission",
		Short: "更新应用管理员权限",
		Long: `更新宜搭应用的管理员权限配置。
**CAUTION:** 权限变更会立即生效，请确保已收到用户的明确指令再调用。`,
		Example: `  dws yida app update-permission --app <appType> --admin-type <adminType> --managers <userId>
  dws yida app update-permission --app <appType> --admin-type <adminType> --managers <userId> --language zh_CN`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "app", "admin-type", "managers"); err != nil {
				return err
			}
			argsMap := map[string]any{
				"appType":   mustGetFlag(cmd, "app"),
				"adminType": mustGetFlag(cmd, "admin-type"),
				"managers":  mustGetFlag(cmd, "managers"),
			}
			if v, _ := cmd.Flags().GetString("language"); v != "" {
				argsMap["language"] = v
			}
			return callMCPTool("update_app_permission", argsMap)
		},
	}

	// ── app flags ────────────────────────────────────────────
	appListCmd.Flags().String("keyword", "", "按应用名搜索 (可选)")
	appListCmd.Flags().String("filter", "", "过滤条件 (可选)：all=全部 / createdByMe=我创建的 / managedByMe=我管理的")
	appListCmd.Flags().String("page", "", "分页页码，默认 1 (可选)")
	appListCmd.Flags().String("size", "", "每页条数，默认 20，最大 100 (可选)")
	appListCmd.Flags().String("language", "", "语言：zh_CN / en_US (可选)")

	appUpdatePermCmd.Flags().String("app", "", "应用编码 (必填)")
	appUpdatePermCmd.Flags().String("admin-type", "", "管理员类型 (必填)")
	appUpdatePermCmd.Flags().String("managers", "", "管理员UserId (必填)")
	appUpdatePermCmd.Flags().String("language", "", "语言：zh_CN / en_US (可选)")

	appListFormsCmd.Flags().String("app", "", "应用编码 (必填)")
	appListFormsCmd.Flags().String("form-types", "", "表单类型：receipt（单据）/ process（流程），不传默认全选 (可选)")
	appListFormsCmd.Flags().String("page", "", "分页页码，默认 1 (可选)")
	appListFormsCmd.Flags().String("size", "", "每页条数，默认 20，最大 100 (可选)")
	appListFormsCmd.Flags().String("language", "", "语言：zh_CN / en_US (可选)")

	// ── form flags ───────────────────────────────────────────
	formComponentsCmd.Flags().String("app", "", "应用编码 (必填)")
	formComponentsCmd.Flags().String("form", "", "表单 UUID (必填)")
	formComponentsCmd.Flags().String("language", "", "语言 (可选)")
	formComponentsCmd.Flags().String("version", "", "表单版本，默认最新版本 (可选)")

	// ── data flags ──────────────────────────────────────────
	dataSearchCmd.Flags().String("app", "", "应用编码 (必填)")
	dataSearchCmd.Flags().String("form", "", "表单 UUID (必填)")
	dataSearchCmd.Flags().String("page", "", "分页页码，默认 1 (可选)")
	dataSearchCmd.Flags().String("size", "", "每页记录数，默认 20，最大 100 (可选)")
	dataSearchCmd.Flags().String("search-field", "", "按组件值过滤查询（JSON 字符串）(可选)")
	dataSearchCmd.Flags().Bool("use-alias", false, "开启后 searchFieldJson 中支持以别名形式传入组件 ID (可选)")
	dataSearchCmd.Flags().String("originator-id", "", "按流程发起人工号过滤 (可选)")
	dataSearchCmd.Flags().String("create-from", "", "创建时间起始 ISO-8601 (可选)")
	dataSearchCmd.Flags().String("create-to", "", "创建时间截止 ISO-8601 (可选)")
	dataSearchCmd.Flags().String("modified-from", "", "修改时间起始 ISO-8601 (可选)")
	dataSearchCmd.Flags().String("modified-to", "", "修改时间截止 ISO-8601 (可选)")
	dataSearchCmd.Flags().String("language", "", "语言：zh_CN / en_US (可选)")

	dataDetailCmd.Flags().String("app", "", "应用标识 (必填)")
	dataDetailCmd.Flags().String("instance-id", "", "实例 ID (必填)")
	dataDetailCmd.Flags().String("form", "", "表单 UUID (可选)")
	dataDetailCmd.Flags().String("need-inst-value", "", "是否返回 instValue，传 n 不返回 (可选)")

	dataCreateCmd.Flags().String("app", "", "应用编码 (必填)")
	dataCreateCmd.Flags().String("form", "", "表单 UUID (必填)")
	dataCreateCmd.Flags().String("form-data", "", "表单数据 JSON (必填)")
	dataCreateCmd.Flags().String("instance-id", "", "指定新实例 ID；一般不传，默认由后端生成 (可选)")
	dataCreateCmd.Flags().Bool("use-alias", false, "是否使用组件别名 (可选)")
	dataCreateCmd.Flags().String("language", "", "语言：zh_CN / en_US (可选)")

	dataUpdateCmd.Flags().String("app", "", "应用编码 (必填)")
	dataUpdateCmd.Flags().String("form", "", "表单 UUID (必填)")
	dataUpdateCmd.Flags().String("instance-id", "", "表单实例 ID (必填)")
	dataUpdateCmd.Flags().String("form-data", "", "要修改的表单数据 JSON (必填)")
	dataUpdateCmd.Flags().Bool("use-alias", false, "是否使用组件别名 (可选)")
	dataUpdateCmd.Flags().String("language", "", "语言：zh_CN / en_US (可选)")

	// ── task flags ───────────────────────────────────────────
	taskListCmd.Flags().String("apps", "", "指定应用的待办，多个用逗号分隔，如 APP_AAA,APP_BBB (可选)")
	taskListCmd.Flags().String("process-codes", "", "指定流程 code 的待办，多个用逗号分隔 (可选)")
	taskListCmd.Flags().String("keyword", "", "待办关键字（如标题）(可选)")
	taskListCmd.Flags().String("status", "", "任务状态 (可选)")
	taskListCmd.Flags().String("page", "", "分页页码，默认 1 (可选)")
	taskListCmd.Flags().String("size", "", "每页条数，默认 20，最大 100 (可选)")
	taskListCmd.Flags().String("create-from", "", "任务创建时间起始 ISO-8601 (可选)")
	taskListCmd.Flags().String("create-to", "", "任务创建时间截止 ISO-8601 (可选)")
	taskListCmd.Flags().String("instance-create-from", "", "实例创建时间起始 ISO-8601 (可选)")
	taskListCmd.Flags().String("instance-create-to", "", "实例创建时间截止 ISO-8601 (可选)")
	taskListCmd.Flags().String("task-finish-from", "", "任务完成时间起始 ISO-8601 (可选)")
	taskListCmd.Flags().String("task-finish-to", "", "任务完成时间截止 ISO-8601 (可选)")

	processRunningTasksCmd.Flags().String("app", "", "应用编码 (必填)")
	processRunningTasksCmd.Flags().String("instance-id", "", "流程实例 ID (必填)")
	processRunningTasksCmd.Flags().String("language", "", "语言 (可选)")

	processRecordsCmd.Flags().String("app", "", "应用编码 (必填)")
	processRecordsCmd.Flags().String("instance-id", "", "流程实例 ID (必填)")
	processRecordsCmd.Flags().String("language", "", "语言 (可选)")

	processExecuteCmd.Flags().String("app", "", "应用编码 (必填)")
	processExecuteCmd.Flags().String("form", "", "表单 UUID (必填)")
	processExecuteCmd.Flags().String("task", "", "任务 ID (必填)")
	processExecuteCmd.Flags().String("result", "", "审批结果：同意 / 拒绝 (必填)")
	processExecuteCmd.Flags().String("remark", "", "审批意见 (可选)")
	processExecuteCmd.Flags().String("instance-id", "", "流程实例 ID (可选)")
	processExecuteCmd.Flags().String("form-data", "", "审批时修改的表单数据 JSON (可选)")
	processExecuteCmd.Flags().String("digital-sign-url", "", "电子签名 URL (可选)")
	processExecuteCmd.Flags().Bool("no-execute-expressions", false, "是否跳过表达式执行 (可选)")

	processRedirectCmd.Flags().String("app", "", "应用编码 (必填)")
	processRedirectCmd.Flags().String("task", "", "任务 ID (必填)")
	processRedirectCmd.Flags().String("instance-id", "", "流程实例 ID (必填)")
	processRedirectCmd.Flags().String("to-user", "", "转交后的新执行人 userId (必填)")
	processRedirectCmd.Flags().String("form", "", "表单 UUID (可选)")
	processRedirectCmd.Flags().String("remark", "", "转交备注 (可选)")
	processRedirectCmd.Flags().Bool("by-manager", false, "是否由管理员转交 (可选)")

	processStartCmd.Flags().String("app", "", "应用编码 (必填)")
	processStartCmd.Flags().String("form", "", "表单 UUID (必填)")
	processStartCmd.Flags().String("form-data", "", "表单数据 JSON (必填)")
	processStartCmd.Flags().String("dept-id", "", "部门 ID (可选)")
	processStartCmd.Flags().String("process-code", "", "流程编码 (可选)")
	processStartCmd.Flags().String("process-data", "", "流程数据 (可选)")
	processStartCmd.Flags().String("business-id", "", "业务自定义 ID (可选)")
	processStartCmd.Flags().String("instance-id", "", "流程实例 ID (可选)")
	processStartCmd.Flags().Bool("use-alias", false, "是否使用组件别名 (可选)")
	processStartCmd.Flags().String("language", "", "语言：zh_CN / en_US (可选)")

	// ── automation flags ─────────────────────────────────────
	automationCreateCmd.Flags().String("app", "", "应用编码 (必填)")
	automationCreateCmd.Flags().String("form", "", "表单 UUID (必填)")
	automationCreateCmd.Flags().String("name", "", "流程名称 (必填)")
	automationCreateCmd.Flags().String("type", "", "流程类型 (必填): 1=表单事件 / 2=定时 / 3=应用级")
	automationCreateCmd.Flags().String("desc", "", "流程描述 (可选)")
	automationCreateCmd.Flags().String("language", "", "语言：zh_CN / en_US (可选)")

	automationUpdateCmd.Flags().String("app", "", "应用编码 (必填)")
	automationUpdateCmd.Flags().String("process-code", "", "流程 ID，由 automation create 返回 (必填)")
	automationUpdateCmd.Flags().String("json", "", "节点设置信息 JSON (必填)")
	automationUpdateCmd.Flags().String("view-json", "", "节点预览信息 JSON (可选)")
	automationUpdateCmd.Flags().String("language", "", "语言：zh_CN / en_US (可选)")

	dataExportStartCmd.Flags().String("app", "", "应用编码 (必填)")
	dataExportStartCmd.Flags().String("form", "", "表单 UUID (必填)")
	dataExportStartCmd.Flags().String("fields", "", "导出的表头字段 JSON 数组 (必填)")
	dataExportStartCmd.Flags().String("view", "", "数据视图 ID (可选)")
	dataExportStartCmd.Flags().String("filter", "", "过滤条件 JSON (可选)")
	dataExportStartCmd.Flags().String("sort", "", "排序规则 (可选)")
	dataExportStartCmd.Flags().String("language", "", "语言：zh_CN / en_US (可选)")

	dataExportQueryCmd.Flags().String("app", "", "应用编码 (必填)")
	dataExportQueryCmd.Flags().String("sequence", "", "导出任务 ID，由 export-start 返回 (必填)")
	dataExportQueryCmd.Flags().String("language", "", "语言：zh_CN / en_US (可选)")

	appCreateCmd := &cobra.Command{
		Use:   "create",
		Short: "创建宜搭应用",
		Long:  `创建一个宜搭应用，返回 appKey 用作后续命令的 --app 入参。`,
		Example: `  dws yida app create
  dws yida app create --name "销售管理" --colour blue`,
		RunE: func(cmd *cobra.Command, args []string) error {
			argsMap := map[string]any{}
			if v, _ := cmd.Flags().GetString("name"); v != "" {
				argsMap["appName"] = v
			}
			if v, _ := cmd.Flags().GetString("description"); v != "" {
				argsMap["description"] = v
			}
			if v, _ := cmd.Flags().GetString("icon"); v != "" {
				argsMap["icon"] = v
			}
			if v, _ := cmd.Flags().GetString("icon-url"); v != "" {
				argsMap["iconUrl"] = v
			}
			if v, _ := cmd.Flags().GetString("colour"); v != "" {
				if err := validateYidaEnum("colour", v, "blue", "green", "orange"); err != nil {
					return err
				}
				argsMap["colour"] = v
			}
			if v, _ := cmd.Flags().GetString("default-language"); v != "" {
				if err := validateYidaEnum("default-language", v, "zh_CN", "en_US", "ja_JP"); err != nil {
					return err
				}
				argsMap["defaultLanguage"] = v
			}
			for cliName, mcpKey := range map[string]string{
				"open-exclusive":      "openExclusive",
				"open-physic-column":  "openPhysicColumn",
				"open-exclusive-unit": "openExclusiveUnit",
			} {
				if v, _ := cmd.Flags().GetString(cliName); v != "" {
					if err := validateYidaEnum(cliName, v, "Y", "N"); err != nil {
						return err
					}
					argsMap[mcpKey] = v
				}
			}
			return callMCPTool("create_app", argsMap)
		},
	}

	designCmd := &cobra.Command{
		Use:   "design",
		Short: "宜搭设计相关命令",
		RunE:  groupRunE,
	}
	designFormCmd := &cobra.Command{Use: "form", Short: "表单设计", RunE: groupRunE}
	designProcessCmd := &cobra.Command{Use: "process", Short: "流程设计", RunE: groupRunE}
	designProcessVersionsCmd := &cobra.Command{Use: "versions", Short: "流程版本", RunE: groupRunE}

	designFormCreateCmd := &cobra.Command{
		Use:   "create",
		Short: "新建表单/报表/自定义页面",
		Long: `按 form-type 新建宜搭表单（receipt/process/report/display），返回 formUuid。
process 类型同时返回 processCode，可直接用于后续流程命令。`,
		Example: `  dws yida design form create --app <appType> --form-type receipt
  dws yida design form create --app <appType> --form-type process --title "请假申请"`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "app", "form-type"); err != nil {
				return err
			}
			ft := mustGetFlag(cmd, "form-type")
			if err := validateYidaEnum("form-type", ft, "receipt", "process", "report", "display"); err != nil {
				return err
			}
			argsMap := map[string]any{
				"appType":  mustGetFlag(cmd, "app"),
				"formType": ft,
			}
			if v, _ := cmd.Flags().GetString("title"); v != "" {
				argsMap["title"] = v
			}
			if v, _ := cmd.Flags().GetString("description"); v != "" {
				argsMap["description"] = v
			}
			// content / parentNavUuid / titleEn / descriptionEn 暂不暴露 CLI flag
			return callMCPTool("create_form", argsMap)
		},
	}

	designFormGetInfoCmd := &cobra.Command{
		Use:   "get-info",
		Short: "读取表单元数据",
		Long: `读取表单元数据：formType / formStatus / 标题 / processCode（仅流程表单）/ icon 等。
要修改流程表单时，可通过本命令拿到 processCode。`,
		Example: `  dws yida design form get-info --app <appType> --form FORM-XXX`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "app", "form"); err != nil {
				return err
			}
			argsMap := map[string]any{
				"appType":  mustGetFlag(cmd, "app"),
				"formUuid": mustGetFlag(cmd, "form"),
			}
			return callMCPTool("get_form_info", argsMap)
		},
	}

	// ── design form get-schema ──
	designFormGetSchemaCmd := &cobra.Command{
		Use:     "get-schema",
		Short:   "读取表单完整 schema",
		Long:    `读取表单完整 schema（含 fields/config/style 等结构）。`,
		Example: `  dws yida design form get-schema --app <appType> --form FORM-XXX`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "app", "form"); err != nil {
				return err
			}
			argsMap := map[string]any{
				"appType":  mustGetFlag(cmd, "app"),
				"formUuid": mustGetFlag(cmd, "form"),
			}
			if v, _ := cmd.Flags().GetString("language"); v != "" {
				argsMap["language"] = v
			}
			return callMCPTool("get_form_schema", argsMap)
		},
	}

	// ── design form update-schema (--yes 兜底)
	// 注意 CLI 命令名是 update-schema（符合 cr-rules 推荐动词 update），但 MCP 工具名仍是 save_form_schema。
	designFormUpdateSchemaCmd := &cobra.Command{
		Use:   "update-schema",
		Short: "覆盖式更新表单 schema",
		Long: `覆盖式保存表单 schema，整体替换字段/布局/样式。
**CAUTION:** 覆盖式写入不可逆 — 执行前必须向用户确认。`,
		Example: `  dws yida design form update-schema --app <appType> --form FORM-XXX --content '<schema-json>'`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "app", "form", "content"); err != nil {
				return err
			}
			if !confirmYidaAction(cmd, "update form schema", mustGetFlag(cmd, "form")) {
				return nil
			}
			argsMap := map[string]any{
				"appType":  mustGetFlag(cmd, "app"),
				"formUuid": mustGetFlag(cmd, "form"),
				"content":  mustGetFlag(cmd, "content"),
			}
			if v, _ := cmd.Flags().GetString("form-type"); v != "" {
				if err := validateYidaEnum("form-type", v, "receipt", "process", "report", "display"); err != nil {
					return err
				}
				argsMap["formType"] = v
			}
			if v, _ := cmd.Flags().GetString("gmt-modified"); v != "" {
				argsMap["gmtModified"] = v
			}
			if v, _ := cmd.Flags().GetString("system-type"); v != "" {
				argsMap["systemType"] = v
			}
			return callMCPTool("save_form_schema", argsMap)
		},
	}

	// ── design form update-title ──
	designFormUpdateTitleCmd := &cobra.Command{
		Use:     "update-title",
		Short:   "修改表单标题",
		Long:    `仅修改表单标题，不影响 schema。流程类型表单同时同步流程名。`,
		Example: `  dws yida design form update-title --app <appType> --form FORM-XXX --title "请假申请"`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "app", "form"); err != nil {
				return err
			}
			title, _ := cmd.Flags().GetString("title")
			titleEn, _ := cmd.Flags().GetString("title-en")
			if title == "" && titleEn == "" {
				return fmt.Errorf("--title or --title-en is required (at least one)")
			}
			argsMap := map[string]any{
				"appType":  mustGetFlag(cmd, "app"),
				"formUuid": mustGetFlag(cmd, "form"),
			}
			if title != "" {
				argsMap["title"] = title
			}
			if titleEn != "" {
				argsMap["titleEn"] = titleEn
			}
			if v, _ := cmd.Flags().GetString("language"); v != "" {
				argsMap["language"] = v
			}
			return callMCPTool("update_form_title", argsMap)
		},
	}

	// ── design form preview ──
	designFormPreviewCmd := &cobra.Command{
		Use:     "preview",
		Short:   "生成表单预览链接",
		Long:    `为表单生成预览链接，用于设计阶段查看效果。`,
		Example: `  dws yida design form preview --app <appType> --form FORM-XXX`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "app", "form"); err != nil {
				return err
			}
			argsMap := map[string]any{
				"appType":  mustGetFlag(cmd, "app"),
				"formUuid": mustGetFlag(cmd, "form"),
			}
			if v, _ := cmd.Flags().GetString("content"); v != "" {
				argsMap["content"] = v
			}
			if v, _ := cmd.Flags().GetString("form-type"); v != "" {
				if err := validateYidaEnum("form-type", v, "receipt", "process", "report", "display"); err != nil {
					return err
				}
				argsMap["formType"] = v
			}
			if v, _ := cmd.Flags().GetString("gmt-modified"); v != "" {
				argsMap["gmtModified"] = v
			}
			if v, _ := cmd.Flags().GetString("system-type"); v != "" {
				argsMap["systemType"] = v
			}
			return callMCPTool("preview_form", argsMap)
		},
	}

	// ── design process create-draft ──
	designProcessCreateDraftCmd := &cobra.Command{
		Use:     "create-draft",
		Short:   "基于源流程开新草稿",
		Long:    `从指定源流程版本复制生成新的草稿流程，返回新 processId 用于后续保存与发布。`,
		Example: `  dws yida design process create-draft --app <appType> --form FORM-XXX --process-id 88440795480`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "app", "form", "process-id"); err != nil {
				return err
			}
			pid := mustGetFlag(cmd, "process-id")
			if !isAllDigits(pid) {
				return fmt.Errorf("--process-id must be numeric, got: %s", pid)
			}
			argsMap := map[string]any{
				"appType":   mustGetFlag(cmd, "app"),
				"formUuid":  mustGetFlag(cmd, "form"),
				"processId": pid,
			}
			return callMCPTool("create_process_flow", argsMap)
		},
	}

	// ── design process update (--yes 兜底) ──（CLI: update，MCP: save_process）
	designProcessUpdateCmd := &cobra.Command{
		Use:   "update",
		Short: "保存流程定义",
		Long: `覆盖式保存草稿流程的定义。
**CAUTION:** 写入不可逆；若选择直接发布则等同上线 — 执行前必须向用户确认。`,
		Example: `  dws yida design process update --app <appType> --process-code TPROC--XX --process-id 88440795481 --content '<json>'`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "app", "process-code", "process-id", "content"); err != nil {
				return err
			}
			pid := mustGetFlag(cmd, "process-id")
			if !isAllDigits(pid) {
				return fmt.Errorf("--process-id must be numeric, got: %s", pid)
			}
			if !confirmYidaAction(cmd, "update process", pid) {
				return nil
			}
			argsMap := map[string]any{
				"appType":     mustGetFlag(cmd, "app"),
				"processCode": mustGetFlag(cmd, "process-code"),
				"processId":   pid,
				"json":        mustGetFlag(cmd, "content"),
			}
			if v, _ := cmd.Flags().GetString("view-content"); v != "" {
				argsMap["viewJson"] = v
			}
			if v, _ := cmd.Flags().GetBool("online"); v {
				argsMap["online"] = true
			}
			if v, _ := cmd.Flags().GetBool("logic"); v {
				argsMap["logic"] = true
			}
			return callMCPTool("save_process", argsMap)
		},
	}

	// ── design process publish (--yes 兜底) ──
	designProcessPublishCmd := &cobra.Command{
		Use:   "publish",
		Short: "发布流程",
		Long: `将草稿流程发布为生效版本。
**CAUTION:** 发布后对终端用户立即生效，不可回滚 — 执行前必须向用户确认。`,
		Example: `  dws yida design process publish --app <appType> --process-code TPROC--XX --process-id 88440795481`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "app", "process-code", "process-id"); err != nil {
				return err
			}
			pid := mustGetFlag(cmd, "process-id")
			if !isAllDigits(pid) {
				return fmt.Errorf("--process-id must be numeric, got: %s", pid)
			}
			if !confirmYidaAction(cmd, "publish process", pid) {
				return nil
			}
			argsMap := map[string]any{
				"appType":     mustGetFlag(cmd, "app"),
				"processCode": mustGetFlag(cmd, "process-code"),
				"processId":   pid,
			}
			return callMCPTool("publish_process", argsMap)
		},
	}

	// ── design process get ──
	designProcessGetCmd := &cobra.Command{
		Use:     "get",
		Short:   "读取流程版本详情",
		Long:    `读取指定流程版本的完整内容，含后端定义、前端视图、状态和版本号。`,
		Example: `  dws yida design process get --app <appType> --process-code TPROC--XX --process-id 88440795481`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "app", "process-code", "process-id"); err != nil {
				return err
			}
			pid := mustGetFlag(cmd, "process-id")
			if !isAllDigits(pid) {
				return fmt.Errorf("--process-id must be numeric, got: %s", pid)
			}
			argsMap := map[string]any{
				"appType":     mustGetFlag(cmd, "app"),
				"processCode": mustGetFlag(cmd, "process-code"),
				"processId":   pid,
			}
			return callMCPTool("get_process", argsMap)
		},
	}

	// ── design process versions list ──
	designProcessVersionsListCmd := &cobra.Command{
		Use:   "list",
		Short: "列出流程版本",
		Long:  `分页列出某条流程的全部历史版本（含状态过滤、排序、别名匹配）。`,
		Example: `  dws yida design process versions list --app <appType> --process-code TPROC--XX --status PUBLISHED --size 1
  dws yida design process versions list --app <appType> --process-code TPROC--XX`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "app", "process-code"); err != nil {
				return err
			}
			argsMap := map[string]any{
				"appType":     mustGetFlag(cmd, "app"),
				"processCode": mustGetFlag(cmd, "process-code"),
			}
			if v, _ := cmd.Flags().GetString("process-id"); v != "" {
				if !isAllDigits(v) {
					return fmt.Errorf("--process-id must be numeric, got: %s", v)
				}
				argsMap["processId"] = v
			}
			if v, _ := cmd.Flags().GetString("status"); v != "" {
				if err := validateYidaEnum("status", v, "DRAFT", "PUBLISHED", "DISABLED", "INVALID"); err != nil {
					return err
				}
				argsMap["status"] = v
			}
			oc, _ := cmd.Flags().GetString("order-by-create-time")
			om, _ := cmd.Flags().GetString("order-by-modify-time")
			if oc != "" && om != "" {
				return fmt.Errorf("--order-by-create-time and --order-by-modify-time are mutually exclusive")
			}
			if oc != "" {
				if err := validateYidaEnum("order-by-create-time", oc, "ASC", "DESC"); err != nil {
					return err
				}
				argsMap["orderByCreateTime"] = oc
			}
			if om != "" {
				if err := validateYidaEnum("order-by-modify-time", om, "ASC", "DESC"); err != nil {
					return err
				}
				argsMap["orderByModifyTime"] = om
			}
			if v, _ := cmd.Flags().GetString("alias-keyword"); v != "" {
				argsMap["aliasKeyword"] = v
			}
			page, size, err := yidaParsePageSize(cmd, "page", "size")
			if err != nil {
				return err
			}
			argsMap["pageIndex"] = page
			argsMap["pageSize"] = size
			return callMCPTool("list_process_versions", argsMap)
		},
	}

	// app create —— --yes / -y 走全局 flag，本地不再重复声明
	appCreateCmd.Flags().String("name", "", "应用名称，最大 50 字符 (可选)")
	appCreateCmd.Flags().String("description", "", "应用描述 (可选)")
	appCreateCmd.Flags().String("icon", "", "应用图标，如 appdiqiu%%#0089FF (可选)")
	appCreateCmd.Flags().String("icon-url", "", "应用图标 URL (可选)")
	appCreateCmd.Flags().String("colour", "", "主题色：blue / green / orange (可选)")
	appCreateCmd.Flags().String("default-language", "", "默认语言：zh_CN / en_US / ja_JP (可选)")
	appCreateCmd.Flags().String("open-exclusive", "", "是否开启专属存储 Y/N，仅专属版组织 (可选)")
	appCreateCmd.Flags().String("open-physic-column", "", "是否开启物理列 Y/N，仅专属版组织 (可选)")
	appCreateCmd.Flags().String("open-exclusive-unit", "", "是否开启专属环境 Y/N，仅专属版组织 (可选)")

	// design form create
	designFormCreateCmd.Flags().String("app", "", "应用编码 (必填)")
	designFormCreateCmd.Flags().String("form-type", "", "页面类型 receipt/process/report/display (必填)")
	designFormCreateCmd.Flags().String("title", "", "中文标题 (可选；英文由服务端用中文兜底)")
	designFormCreateCmd.Flags().String("description", "", "中文描述 (可选；英文由服务端用中文兜底)")
	// --content / --parent-nav / --title-en / --description-en 暂不暴露：写 schema 走 update-schema --yes

	// design form get-info
	designFormGetInfoCmd.Flags().String("app", "", "应用编码 (必填)")
	designFormGetInfoCmd.Flags().String("form", "", "表单 UUID (必填)")

	// design form get-schema
	designFormGetSchemaCmd.Flags().String("app", "", "应用编码 (必填)")
	designFormGetSchemaCmd.Flags().String("form", "", "表单 UUID (必填)")
	designFormGetSchemaCmd.Flags().String("language", "", "语言 (可选)")

	// design form update-schema [危险] —— --yes / -y 走全局 flag
	designFormUpdateSchemaCmd.Flags().String("app", "", "应用编码 (必填)")
	designFormUpdateSchemaCmd.Flags().String("form", "", "表单 UUID (必填)")
	designFormUpdateSchemaCmd.Flags().String("content", "", "完整 schema JSON (必填)")
	designFormUpdateSchemaCmd.Flags().String("form-type", "", "页面类型，传了省一次反查 (可选)")
	designFormUpdateSchemaCmd.Flags().String("gmt-modified", "", "乐观并发校验，毫秒时间戳 (可选)")
	designFormUpdateSchemaCmd.Flags().String("system-type", "", "系统类型 (可选)")

	// design form update-title
	designFormUpdateTitleCmd.Flags().String("app", "", "应用编码 (必填)")
	designFormUpdateTitleCmd.Flags().String("form", "", "表单 UUID (必填)")
	designFormUpdateTitleCmd.Flags().String("title", "", "中文标题 (与 --title-en 至少一)")
	designFormUpdateTitleCmd.Flags().String("title-en", "", "英文标题 (与 --title 至少一)")
	designFormUpdateTitleCmd.Flags().String("language", "", "语言 (可选)")

	// design form preview
	designFormPreviewCmd.Flags().String("app", "", "应用编码 (必填)")
	designFormPreviewCmd.Flags().String("form", "", "表单 UUID (必填)")
	designFormPreviewCmd.Flags().String("content", "", "传了内部先 save 再返回预览链接 (可选)")
	designFormPreviewCmd.Flags().String("form-type", "", "页面类型，仅 --content 非空时有意义 (可选)")
	designFormPreviewCmd.Flags().String("gmt-modified", "", "毫秒时间戳，仅 --content 非空时有意义 (可选)")
	designFormPreviewCmd.Flags().String("system-type", "", "系统类型 (可选)")

	// design process create-draft
	designProcessCreateDraftCmd.Flags().String("app", "", "应用编码 (必填)")
	designProcessCreateDraftCmd.Flags().String("form", "", "表单 UUID (必填)")
	designProcessCreateDraftCmd.Flags().String("process-id", "", "源流程版本 id，纯数字 (必填)")

	// design process update [危险] —— --yes / -y 走全局 flag
	designProcessUpdateCmd.Flags().String("app", "", "应用编码 (必填)")
	designProcessUpdateCmd.Flags().String("process-code", "", "流程 code，形如 TPROC--XXX (必填)")
	designProcessUpdateCmd.Flags().String("process-id", "", "目标草稿 processId，纯数字 (必填)")
	designProcessUpdateCmd.Flags().String("content", "", "后端流程定义 JSON (必填)")
	designProcessUpdateCmd.Flags().String("view-content", "", "前端视图 JSON (可选)")
	designProcessUpdateCmd.Flags().Bool("online", false, "保存事务里直接发布 (可选)")
	designProcessUpdateCmd.Flags().Bool("logic", false, "是否为逻辑流/连接器 (可选；普通审批流不传)")

	// design process publish [危险] —— --yes / -y 走全局 flag
	designProcessPublishCmd.Flags().String("app", "", "应用编码 (必填)")
	designProcessPublishCmd.Flags().String("process-code", "", "流程 code (必填)")
	designProcessPublishCmd.Flags().String("process-id", "", "草稿 processId (必填)")

	// design process get
	designProcessGetCmd.Flags().String("app", "", "应用编码 (必填)")
	designProcessGetCmd.Flags().String("process-code", "", "流程 code (必填)")
	designProcessGetCmd.Flags().String("process-id", "", "流程版本 id (必填)")

	// design process versions list（分页 flag 走规范 --page / --size）
	designProcessVersionsListCmd.Flags().String("app", "", "应用编码 (必填)")
	designProcessVersionsListCmd.Flags().String("process-code", "", "流程 code (必填)")
	designProcessVersionsListCmd.Flags().String("process-id", "", "只查这一版本 (可选)")
	designProcessVersionsListCmd.Flags().String("status", "", "状态过滤：DRAFT/PUBLISHED/DISABLED/INVALID (可选)")
	designProcessVersionsListCmd.Flags().String("order-by-create-time", "", "创建时间排序 ASC/DESC (可选；与 modify-time 互斥)")
	designProcessVersionsListCmd.Flags().String("order-by-modify-time", "", "修改时间排序 ASC/DESC (可选；与 create-time 互斥)")
	designProcessVersionsListCmd.Flags().String("alias-keyword", "", "版本别名模糊匹配 (可选)")
	designProcessVersionsListCmd.Flags().String("page", "", "分页页码，默认 1 (可选)")
	designProcessVersionsListCmd.Flags().String("size", "", "每页条数，默认 20，最大 100 (可选)")

	// ── 注册子命令 ───────────────────────────────────────────
	appCmd.AddCommand(appListCmd, appListFormsCmd, appUpdatePermCmd, appCreateCmd)
	formCmd.AddCommand(formComponentsCmd)
	dataCmd.AddCommand(dataSearchCmd, dataDetailCmd, dataCreateCmd, dataUpdateCmd, dataExportStartCmd, dataExportQueryCmd)
	taskCmd.AddCommand(taskListCmd)
	processCmd.AddCommand(processRunningTasksCmd, processRecordsCmd, processExecuteCmd, processRedirectCmd, processStartCmd)
	automationCmd.AddCommand(automationCreateCmd, automationUpdateCmd)
	root.AddCommand(appCmd, formCmd, dataCmd, taskCmd, processCmd, automationCmd)

	// 设计态子树
	designFormCmd.AddCommand(designFormCreateCmd, designFormGetInfoCmd, designFormGetSchemaCmd,
		designFormUpdateSchemaCmd, designFormUpdateTitleCmd, designFormPreviewCmd)
	designProcessVersionsCmd.AddCommand(designProcessVersionsListCmd)
	designProcessCmd.AddCommand(designProcessCreateDraftCmd, designProcessUpdateCmd,
		designProcessPublishCmd, designProcessGetCmd, designProcessVersionsCmd)
	designCmd.AddCommand(designFormCmd, designProcessCmd)

	root.AddCommand(appCmd, formCmd, dataCmd, taskCmd, processCmd, designCmd)

	return root
}
