package helpers

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
	"github.com/spf13/cobra"
)

const (
	aitablePsqlAgentSummary     = "使用 PostgreSQL 在服务端完成 AI 表格复杂分析。"
	aitablePsqlUseWhen          = "同 Base JOIN、字段间算术、CASE、聚合后派生、汇总结果 Top N 或排名、窗口计算等原生记录接口无法直接表达的分析。"
	aitablePsqlAvoidRecordQuery = "需要 recordId、cells 或 cursor，或仅对单表原始记录筛选、排序、取 Top N、逐条后续操作时，使用 record query；psql 执行失败后仅当原始意图完全属于这些场景时，才可重新发起 record query。"
	aitablePsqlAvoidStats       = "即使明确要求 SQL，单表直接标量、分组或去重统计仍使用 record stats 或 record group-stats；psql 执行失败后仅当原始意图完全属于这些场景时，才可重新发起对应统计。统计后还需派生、聚合结果 Top N 或排名、窗口计算时仍使用 psql。"
	aitablePsqlAvoidExport      = "交付完整原始数据文件时，使用 export data。"
	aitablePsqlAvoidWriteOrDDL  = "新增、更新、删除记录或执行 DDL 时不可使用。"
)

var aitablePsqlBusinessResultSchema = json.RawMessage(`{
  "description":"PSQL 表清单、表结构或只读查询的业务结果；具体形态由 -l、-t 或 -c 模式决定",
  "oneOf":[
    {
      "type":"array",
      "items":{"type":"object","properties":{
        "tableId":{"type":"string","description":"逻辑表对应的数据表 ID"},
        "tableName":{"type":"string","description":"可在 SQL 中引用的逻辑表名称"},
        "description":{"type":["string","null"],"description":"逻辑表说明"}
      },"required":["tableId","tableName"],"additionalProperties":true}
    },
    {
      "type":"object",
      "properties":{
        "tableId":{"type":"string","description":"数据表 ID"},
        "tableName":{"type":"string","description":"逻辑表名称"},
        "description":{"type":["string","null"],"description":"逻辑表说明"},
        "columns":{"type":"array","description":"字段及其 PostgreSQL 投影列","items":{"type":"object","additionalProperties":true}}
      },
      "required":["tableId","tableName","columns"],
      "additionalProperties":true
    },
    {
      "type":"object",
      "properties":{
        "columns":{"type":"array","description":"查询结果列定义","items":{"type":"object","additionalProperties":true}},
        "rows":{"type":"array","description":"与 columns 顺序对应的查询结果行","items":{"type":"array"}},
        "rowCount":{"type":"integer","description":"本次返回的行数"},
        "truncated":{"type":"boolean","description":"结果是否因 limit 被截断"}
      },
      "required":["columns","rows","rowCount","truncated"],
      "additionalProperties":true
    }
  ]
}`)

var aitablePsqlResultSchema = aitableResultSchemaWithDryRun(
	"PSQL 表清单、表结构、只读查询结果或 dry-run 请求预览",
	aitablePsqlBusinessResultSchema,
)

func newAitablePsqlCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "psql",
		Short: "以 PostgreSQL 逻辑表方式查询 AI 表格",
		Long:  "以 PostgreSQL 逻辑表方式查询 AI 表格。默认保留 psql 风格表格展示；显式传 --format json 可获得统一结果信封，--jq 对该信封求值。-x 只影响默认表格展示。",
		Example: strings.Join([]string{
			"  dws aitable psql -d BASE_ID -l",
			"  dws aitable psql -d BASE_ID -t TABLE_ID --all-properties",
			`  dws aitable psql -d BASE_ID -c 'SELECT * FROM "项目表" LIMIT 20'`,
			`  dws aitable psql -d BASE_ID -x -c 'SELECT "人员字段1_id" FROM "项目表"'`,
		}, "\n"),
		Args: cobra.NoArgs,
		RunE: runAitablePsql,
	}
	cmd.Flags().StringP("database", "d", "", "AI 表格 Base ID（必填）")
	cmd.Flags().BoolP("list", "l", false, "列出 Base 内可查询的逻辑表")
	cmd.Flags().StringP("table", "t", "", "查看表结构时使用的 tableId")
	cmd.Flags().Bool("all-properties", false, "查看表结构时展开字段的全部属性列")
	cmd.Flags().BoolP("expanded", "x", false, "按 psql expanded display 逐行纵向展示查询结果")
	cmd.Flags().StringP("command", "c", "", "执行一条标准 PostgreSQL SELECT")
	cmd.Flags().Int("limit", 200, "最大返回行数，范围 1-1000")
	cmd.Flags().Int("timeout", 30, "查询超时秒数，范围 1-60")

	DeclareLeafMetadata(cmd, LeafSpec{
		Safety:        aitableSafetyRead(),
		OutputRollout: output.RolloutUnifiedActive,
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "aitable",
				Name:           "psql",
				CanonicalPath:  "aitable.psql",
				CLIPath:        "aitable psql",
				PrimaryCLIPath: "aitable psql",
			},
			Description: "使用 PostgreSQL 语法查看 AI 表格逻辑表结构并执行只读 SELECT。",
			DryRun:      &contract.DryRunSpec{PreviewKind: contract.DryRunPreviewRequest, RemoteReads: false},
			Result: &contract.ResultSpec{
				Outcomes:   []contract.ResultOutcome{contract.ResultOutcomeSuccess, contract.ResultOutcomeFailure},
				DataSchema: aitablePsqlResultSchema,
			},
			Interface: &contract.InterfaceSpec{
				Mode:         "composite",
				Availability: "available",
				Reason:       "命令根据 -l、-t 和 -c 路由到表清单、表结构或统一 SQL 执行 MCP Tool。",
			},
			Selection: contract.SelectionSpec{
				AgentSummary: aitablePsqlAgentSummary,
				UseWhen:      []string{aitablePsqlUseWhen},
				AvoidWhen: []string{
					aitablePsqlAvoidRecordQuery,
					aitablePsqlAvoidStats,
					aitablePsqlAvoidExport,
					aitablePsqlAvoidWriteOrDDL,
				},
				Examples: []string{"dws aitable psql -d <BASE_ID> -l", "dws aitable psql -d <BASE_ID> -c 'SELECT * FROM 表名'"},
			},
		},
	})
	return cmd
}

func runAitablePsql(cmd *cobra.Command, _ []string) error {
	baseID, _ := cmd.Flags().GetString("database")
	list, _ := cmd.Flags().GetBool("list")
	tableID, _ := cmd.Flags().GetString("table")
	allProperties, _ := cmd.Flags().GetBool("all-properties")
	expanded, _ := cmd.Flags().GetBool("expanded")
	sql, _ := cmd.Flags().GetString("command")
	limit, _ := cmd.Flags().GetInt("limit")
	timeout, _ := cmd.Flags().GetInt("timeout")

	baseID = strings.TrimSpace(baseID)
	tableID = strings.TrimSpace(tableID)
	sql = strings.TrimSpace(sql)
	if baseID == "" {
		return apperrors.NewValidation("missing required flag: --database")
	}
	modes := 0
	if list {
		modes++
	}
	if tableID != "" && sql == "" {
		modes++
	}
	if sql != "" {
		modes++
	}
	if modes != 1 {
		return apperrors.NewValidation("exactly one mode is required: -l, -t TABLE_ID, or -c SQL")
	}
	if limit < 1 || limit > 1000 {
		return apperrors.NewValidation("--limit must be between 1 and 1000")
	}
	if timeout < 1 || timeout > 60 {
		return apperrors.NewValidation("--timeout must be between 1 and 60")
	}

	var (
		toolName string
		toolArgs map[string]any
		render   func(io.Writer, any) error
	)
	switch {
	case list:
		toolName = "otable_pg_list_tables"
		toolArgs = map[string]any{"baseId": baseID}
		render = renderPgTables
	case sql == "":
		toolName = "otable_pg_describe_table"
		toolArgs = map[string]any{
			"baseId": baseID, "tableId": tableID, "allProperties": allProperties,
		}
		render = renderPgSchema
	default:
		toolName = "otable_pg_execute"
		toolArgs = map[string]any{
			"baseId": baseID, "sql": sql,
			"limit": limit, "timeoutSeconds": timeout,
		}
		render = func(out io.Writer, data any) error { return renderPgQuery(out, data, expanded) }
	}
	if result, ok := aitableUnifiedDryRunResult(toolName, toolArgs); ok {
		return output.StoreResult(cmd.Context(), result)
	}
	data, err := callAitablePsqlTool(cmd.Context(), toolName, toolArgs)
	if err != nil {
		return err
	}
	if err := render(io.Discard, data); err != nil {
		return apperrors.NewInternal(fmt.Sprintf("%s returned invalid data: %v", toolName, err), apperrors.WithCause(err))
	}
	return output.StoreResult(cmd.Context(), output.Success(data, output.WithTablePresentation(render)))
}

func callAitablePsqlTool(ctx context.Context, toolName string, args map[string]any) (any, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	result, err := deps.Caller.CallTool(ctx, "aitable", toolName, args)
	if err != nil {
		return nil, err
	}
	if result == nil {
		return nil, apperrors.NewInternal(fmt.Sprintf("%s returned a nil result", toolName))
	}
	for _, content := range result.Content {
		if content.Type != "text" || strings.TrimSpace(content.Text) == "" {
			continue
		}
		var envelope map[string]any
		decoder := json.NewDecoder(strings.NewReader(content.Text))
		decoder.UseNumber()
		if err := decoder.Decode(&envelope); err != nil {
			return nil, apperrors.NewInternal(fmt.Sprintf("%s returned invalid JSON: %v", toolName, err), apperrors.WithCause(err))
		}
		if err := decoder.Decode(&struct{}{}); err != io.EOF {
			if err == nil {
				err = fmt.Errorf("multiple JSON values")
			}
			return nil, apperrors.NewInternal(fmt.Sprintf("%s returned invalid JSON: %v", toolName, err), apperrors.WithCause(err))
		}
		if status, _ := envelope["status"].(string); strings.EqualFold(status, "error") {
			message := "MCP tool returned an error"
			if detail, ok := envelope["error"].(map[string]any); ok {
				if value, ok := detail["message"].(string); ok && value != "" {
					message = value
				}
			}
			return nil, apperrors.NewAPI(fmt.Sprintf("%s: %s", toolName, message))
		}
		data, ok := envelope["data"]
		if !ok || data == nil {
			return nil, apperrors.NewInternal(toolName + " returned no data")
		}
		return data, nil
	}
	return nil, apperrors.NewInternal(fmt.Sprintf("%s returned no text content", toolName))
}

func renderPgTables(out io.Writer, data any) error {
	tables, ok := data.([]any)
	if !ok {
		return fmt.Errorf("list tables response must be an array, got %T", data)
	}
	rows := make([][]string, 0, len(tables))
	for index, item := range tables {
		table, ok := item.(map[string]any)
		if !ok {
			return fmt.Errorf("table %d must be an object", index)
		}
		tableName, tableID := stringValue(table["tableName"]), stringValue(table["tableId"])
		if strings.TrimSpace(tableName) == "" || strings.TrimSpace(tableID) == "" {
			return fmt.Errorf("table %d must contain non-empty tableName and tableId", index)
		}
		rows = append(rows, []string{
			tableName, tableID,
			stringValue(table["description"]),
		})
	}
	return printPgTable(out, []string{"Name", "Table ID", "Description"}, rows)
}

func renderPgSchema(out io.Writer, data any) error {
	schema, ok := data.(map[string]any)
	if !ok {
		return fmt.Errorf("describe table response must be an object, got %T", data)
	}
	if strings.TrimSpace(stringValue(schema["tableId"])) == "" || strings.TrimSpace(stringValue(schema["tableName"])) == "" {
		return fmt.Errorf("describe table response is missing tableId or tableName")
	}
	columns, ok := schema["columns"].([]any)
	if !ok {
		return fmt.Errorf("describe table response is missing columns")
	}
	rows := make([][]string, 0, len(columns))
	for index, item := range columns {
		column, ok := item.(map[string]any)
		if !ok {
			return fmt.Errorf("column %d must be an object", index)
		}
		if strings.TrimSpace(stringValue(column["columnName"])) == "" || strings.TrimSpace(stringValue(column["pgType"])) == "" {
			return fmt.Errorf("column %d must contain non-empty columnName and pgType", index)
		}
		rows = append(rows, []string{
			stringValue(column["columnName"]), stringValue(column["pgType"]),
			stringValue(column["fieldId"]), stringValue(column["attribute"]),
			stringValue(column["description"]),
		})
	}
	fmt.Fprintf(out, "Table %s (%s)\n", stringValue(schema["tableName"]), stringValue(schema["tableId"]))
	fmt.Fprintf(out, "Description: %s\n", stringValue(schema["description"]))
	return printPgTable(out, []string{"Column", "Type", "Field ID", "Attribute", "Description"}, rows)
}

func renderPgQuery(out io.Writer, data any, expanded bool) error {
	result, ok := data.(map[string]any)
	if !ok {
		return fmt.Errorf("query response must be an object, got %T", data)
	}
	columnsRaw, ok := result["columns"].([]any)
	if !ok {
		return fmt.Errorf("query response is missing columns")
	}
	headers := make([]string, 0, len(columnsRaw))
	for index, item := range columnsRaw {
		column, ok := item.(map[string]any)
		if !ok {
			return fmt.Errorf("query column %d must be an object", index)
		}
		columnName := stringValue(column["columnName"])
		if strings.TrimSpace(columnName) == "" {
			return fmt.Errorf("query column %d must contain a non-empty columnName", index)
		}
		headers = append(headers, columnName)
	}
	rowsRaw, ok := result["rows"].([]any)
	if !ok {
		return fmt.Errorf("query response is missing rows")
	}
	rows := make([][]string, 0, len(rowsRaw))
	for index, item := range rowsRaw {
		values, ok := item.([]any)
		if !ok {
			return fmt.Errorf("query row %d must be an array", index)
		}
		if len(values) != len(headers) {
			return fmt.Errorf("query row %d has %d values for %d columns", index, len(values), len(headers))
		}
		row := make([]string, len(values))
		for i, value := range values {
			row[i] = pgValue(value)
		}
		rows = append(rows, row)
	}
	rowCount, exists := result["rowCount"]
	if !exists || !aitableJSONInteger(rowCount) {
		return fmt.Errorf("query response rowCount must be an integer")
	}
	if _, ok := result["truncated"].(bool); !ok {
		return fmt.Errorf("query response truncated must be a boolean")
	}
	if expanded {
		for index, row := range rows {
			fmt.Fprintf(out, "-[ RECORD %d ]-\n", index+1)
			for i, header := range headers {
				value := ""
				if i < len(row) {
					value = row[i]
				}
				fmt.Fprintf(out, "%s | %s\n", header, value)
			}
		}
	} else if err := printPgTable(out, headers, rows); err != nil {
		return err
	}
	fmt.Fprintf(out, "(%s rows)\n", stringValue(result["rowCount"]))
	if truncated, _ := result["truncated"].(bool); truncated {
		fmt.Fprintln(out, "Warning: result truncated; add stricter filters or aggregation, or increase --limit within 1-1000 before treating it as complete.")
	}
	return nil
}

func printPgTable(out io.Writer, headers []string, rows [][]string) error {
	var rendered strings.Builder
	writer := tabwriter.NewWriter(&rendered, 0, 4, 2, ' ', 0)
	_, _ = fmt.Fprintln(writer, strings.Join(headers, "\t| "))
	separator := make([]string, len(headers))
	for i, header := range headers {
		separator[i] = strings.Repeat("-", max(3, len([]rune(header))))
	}
	_, _ = fmt.Fprintln(writer, strings.Join(separator, "\t+-"))
	for _, row := range rows {
		_, _ = fmt.Fprintln(writer, strings.Join(row, "\t| "))
	}
	_ = writer.Flush()
	_, err := io.WriteString(out, rendered.String())
	return err
}

func pgValue(value any) string {
	if value == nil {
		return ""
	}
	switch typed := value.(type) {
	case string:
		return typed
	case []any, map[string]any:
		encoded, _ := json.Marshal(typed)
		return string(encoded)
	default:
		return fmt.Sprint(typed)
	}
}
