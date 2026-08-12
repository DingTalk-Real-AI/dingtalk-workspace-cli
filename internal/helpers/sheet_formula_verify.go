package helpers

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
	"github.com/spf13/cobra"
)

const sheetFormulaVerifyRemoteTool = "verify_formula"

func newSheetFormulaVerifyCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "formula-verify",
		Short: "校验表格公式错误",
		Long: `扫描钉钉电子表格中已落表的公式单元格，按计算结果错误类型聚合返回错误数量、位置和样本。

不指定 --sheet-id / --range / --targets 时默认扫描整本表格的全部工作表。`,
		Example: `  dws sheet formula-verify --node NODE_ID
  dws sheet formula-verify --node NODE_ID --sheet-id Sheet1 --range A1:D100
  dws sheet formula-verify --node NODE_ID --targets '[{"sheetId":"Sheet1","range":"A1:D100"}]'`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "node"); err != nil {
				return err
			}
			toolArgs := map[string]any{
				"nodeId": mustGetFlag(cmd, "node"),
			}
			targets, err := formulaVerifyTargetsFromFlags(cmd)
			if err != nil {
				return err
			}
			if len(targets) > 0 {
				toolArgs["targets"] = targets
			}
			if cmd.Flags().Changed("max-locations-per-error") {
				v, _ := cmd.Flags().GetInt("max-locations-per-error")
				if v <= 0 {
					return fmt.Errorf("--max-locations-per-error 必须是正整数")
				}
				toolArgs["maxLocationsPerError"] = v
			}
			if cmd.Flags().Changed("max-cells") {
				v, _ := cmd.Flags().GetInt("max-cells")
				if v <= 0 {
					return fmt.Errorf("--max-cells 必须是正整数")
				}
				toolArgs["maxCells"] = v
			}
			exitOnError, _ := cmd.Flags().GetBool("exit-on-error")
			return callMCPToolFormulaVerify(toolArgs, exitOnError)
		},
	}
	cmd.Flags().String("node", "", "表格文档 ID 或 URL (必填)")
	cmd.Flags().String("sheet-id", "", "工作表 ID 或名称；与 --range 组成单个扫描目标")
	cmd.Flags().String("range", "", "A1 范围；需与 --sheet-id 配合使用")
	cmd.Flags().String("targets", "", `扫描目标 JSON 数组、@文件路径 或 - 表示 stdin；每项 {"sheetId":"Sheet1","range":"A1:D100"}`)
	cmd.Flags().Int("max-locations-per-error", 0, "每种错误类型最多返回的位置数")
	cmd.Flags().Int("max-cells", 0, "最多扫描的单元格数")
	cmd.Flags().Bool("exit-on-error", false, "发现公式错误时返回非 0 退出码，便于 CI/自动化使用")
	DeclareLeafMetadata(cmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "read", Risk: "low",
			Confirmation: "not_required", Idempotency: "idempotent",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "sheet",
				Name:           "formula_verify",
				CanonicalPath:  "sheet.formula_verify",
				CLIPath:        "sheet formula-verify",
				PrimaryCLIPath: "sheet formula-verify",
			},
			Description: "扫描表格公式单元格并按错误类型聚合返回错误数量与位置",
			Interface: &contract.InterfaceSpec{
				Mode:         "composite",
				Availability: "available",
				Reason:       "Reviewed unpinned remote adapter: this executable CLI wrapper calls a remote helper that is absent from the pinned MCP metadata snapshot; no single pinned semantically equivalent interface_ref can represent the command.",
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "扫描表格公式单元格并按错误类型聚合返回错误数量与位置",
				UseWhen:      []string{"用户说 校验公式/检查公式错误/公式错误扫描"},
				AvoidWhen:    []string{"读取公式文本用 range read --value-render-option formula"},
				Examples:     []string{"dws sheet formula-verify --node <NODE_ID> --format json"},
			},
		},
	})
	return cmd
}

// callMCPToolFormulaVerify 与常规打印路径的差异：--exit-on-error 需要读取
// 返回 payload 判定是否存在公式错误，故走 ReturnText 再自行 PrintJSON。
func callMCPToolFormulaVerify(toolArgs map[string]any, exitOnError bool) error {
	if deps.Caller.DryRun() {
		return callMCPToolOnServer("sheet", sheetFormulaVerifyRemoteTool, toolArgs)
	}
	text, err := callMCPToolReturnTextOnServer(context.Background(), "sheet", sheetFormulaVerifyRemoteTool, toolArgs)
	if err != nil {
		return err
	}
	if text == "" {
		return nil
	}
	var parsed map[string]any
	if err := json.Unmarshal([]byte(text), &parsed); err != nil {
		deps.Out.PrintRaw(text)
		return nil
	}
	if parsed == nil {
		return fmt.Errorf("%s returned empty result", sheetFormulaVerifyRemoteTool)
	}
	if err := deps.Out.PrintJSON(parsed); err != nil {
		return err
	}
	if exitOnError && formulaVerifyHasErrors(parsed) {
		return fmt.Errorf("formula errors found")
	}
	return nil
}

func formulaVerifyHasErrors(parsed map[string]any) bool {
	result := formulaVerifyResultObject(parsed)
	status := strings.ToLower(strings.TrimSpace(fmt.Sprint(result["status"])))
	if status == "errors_found" {
		return true
	}
	totalErrors, ok := nonNegativeJSONInt(result["totalErrors"])
	return ok && totalErrors > 0
}

func formulaVerifyResultObject(parsed map[string]any) map[string]any {
	if result, ok := parsed["result"].(map[string]any); ok {
		return result
	}
	return parsed
}

func formulaVerifyTargetsFromFlags(cmd *cobra.Command) ([]map[string]any, error) {
	sheetID, _ := cmd.Flags().GetString("sheet-id")
	rangeStr, _ := cmd.Flags().GetString("range")
	if v, _ := cmd.Flags().GetString("targets"); v != "" {
		if strings.TrimSpace(sheetID) != "" || strings.TrimSpace(rangeStr) != "" {
			return nil, fmt.Errorf("--targets 不能与 --sheet-id 或 --range 同时使用")
		}
		return parseFormulaVerifyTargets(cmd, v)
	}
	if sheetID == "" && rangeStr != "" {
		return nil, fmt.Errorf("--range 必须与 --sheet-id 配合使用")
	}
	if sheetID != "" {
		t := map[string]any{"sheetId": sheetID}
		if rangeStr != "" {
			t["range"] = rangeStr
		}
		return []map[string]any{t}, nil
	}
	return nil, nil
}

func parseFormulaVerifyTargets(cmd *cobra.Command, raw string) ([]map[string]any, error) {
	data := raw
	if strings.HasPrefix(raw, "@") {
		filePath := strings.TrimPrefix(raw, "@")
		content, err := os.ReadFile(filePath)
		if err != nil {
			return nil, fmt.Errorf("读取 --targets 文件失败: %w", err)
		}
		data = string(content)
	} else if raw == "-" {
		content, err := io.ReadAll(cmd.InOrStdin())
		if err != nil {
			return nil, fmt.Errorf("读取 stdin 失败: %w", err)
		}
		data = string(content)
	}
	var targets []map[string]any
	if err := json.Unmarshal([]byte(data), &targets); err != nil {
		return nil, fmt.Errorf("--targets JSON 解析失败: %w", err)
	}
	return targets, nil
}
