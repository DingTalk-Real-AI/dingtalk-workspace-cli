package helpers

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
	"github.com/spf13/cobra"
)

const (
	aitablePsqlAgentSummary      = "使用 PostgreSQL 语法进行多表关联和分析查询。"
	aitablePsqlUseWhen           = "多表关联、跨表分析、SQL 聚合、分组或窗口计算时使用。"
	aitablePsqlAvoidRecordQuery  = "单表按 recordId、关键词或字段条件读取记录时使用 record query。"
	aitablePsqlAvoidWriteOrDDL   = "新增、更新、删除记录或执行 DDL 时不可使用。"
	aitablePsqlAvoidMixedResults = "禁止静默降级为 record query 模拟 JOIN 或 SQL 聚合，也不得混用两者的结果模型。"
)

func newAitablePsqlCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "psql",
		Short: "以 PostgreSQL 逻辑表方式查询 AI 表格",
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
		Safety: aitableSafetyRead(),
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "aitable",
				Name:           "psql",
				CanonicalPath:  "aitable.psql",
				CLIPath:        "aitable psql",
				PrimaryCLIPath: "aitable psql",
			},
			Description: "使用 PostgreSQL 语法查看 AI 表格逻辑表结构并执行只读 SELECT。",
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
					aitablePsqlAvoidWriteOrDDL,
					aitablePsqlAvoidMixedResults,
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
		return fmt.Errorf("missing required flag: --database")
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
		return fmt.Errorf("exactly one mode is required: -l, -t TABLE_ID, or -c SQL")
	}
	if limit < 1 || limit > 1000 {
		return fmt.Errorf("--limit must be between 1 and 1000")
	}
	if timeout < 1 || timeout > 60 {
		return fmt.Errorf("--timeout must be between 1 and 60")
	}

	switch {
	case list:
		data, err := callAitablePsqlTool("otable_pg_list_tables", map[string]any{"baseId": baseID})
		if err != nil {
			return err
		}
		return renderPgTables(cmd.OutOrStdout(), data)
	case sql == "":
		data, err := callAitablePsqlTool("otable_pg_describe_table", map[string]any{
			"baseId": baseID, "tableId": tableID, "allProperties": allProperties,
		})
		if err != nil {
			return err
		}
		return renderPgSchema(cmd.OutOrStdout(), data)
	default:
		data, err := callAitablePsqlTool("otable_pg_execute", map[string]any{
			"baseId": baseID, "sql": sql,
			"limit": limit, "timeoutSeconds": timeout,
		})
		if err != nil {
			return err
		}
		return renderPgQuery(cmd.OutOrStdout(), data, expanded)
	}
}

func callAitablePsqlTool(toolName string, args map[string]any) (any, error) {
	result, err := deps.Caller.CallTool(context.Background(), "aitable", toolName, args)
	if err != nil {
		return nil, err
	}
	if result == nil {
		return nil, fmt.Errorf("%s returned a nil result", toolName)
	}
	for _, content := range result.Content {
		if content.Type != "text" || strings.TrimSpace(content.Text) == "" {
			continue
		}
		var envelope map[string]any
		decoder := json.NewDecoder(strings.NewReader(content.Text))
		decoder.UseNumber()
		if err := decoder.Decode(&envelope); err != nil {
			return nil, fmt.Errorf("%s returned invalid JSON: %w", toolName, err)
		}
		if status, _ := envelope["status"].(string); strings.EqualFold(status, "error") {
			message := "MCP tool returned an error"
			if detail, ok := envelope["error"].(map[string]any); ok {
				if value, ok := detail["message"].(string); ok && value != "" {
					message = value
				}
			}
			return nil, fmt.Errorf("%s: %s", toolName, message)
		}
		return envelope["data"], nil
	}
	return nil, fmt.Errorf("%s returned no text content", toolName)
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
		rows = append(rows, []string{
			stringValue(table["tableName"]), stringValue(table["tableId"]),
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
		headers = append(headers, stringValue(column["columnName"]))
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
		row := make([]string, len(values))
		for i, value := range values {
			row[i] = pgValue(value)
		}
		rows = append(rows, row)
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
