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
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"
)

func newSheetFormulaVerifyCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "formula-verify",
		Short: "校验表格公式错误",
		Long: `扫描钉钉电子表格中已经落表的公式单元格，按错误类型聚合返回数量、位置和样本。

不指定 --sheet-id、--range 或 --targets 时扫描整本表格。
--sheet-id/--range 用于单目标；多目标使用 --targets JSON 数组。`,
		Example: `  dws sheet formula-verify --node NODE_ID
  dws sheet formula-verify --node NODE_ID --sheet-id Sheet1 --range A1:D100
  dws sheet formula-verify --node NODE_ID --targets '[{"sheetId":"Sheet1","range":"A1:D100"}]'
  dws sheet formula-verify --node NODE_ID --exit-on-error`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := validateRequiredFlags(cmd, "node"); err != nil {
				return err
			}
			toolArgs := map[string]any{"nodeId": mustGetFlag(cmd, "node")}
			targets, err := sheetFormulaVerifyTargetsFromFlags(cmd)
			if err != nil {
				return err
			}
			if len(targets) > 0 {
				toolArgs["targets"] = targets
			}
			if cmd.Flags().Changed("max-locations-per-error") {
				value, _ := cmd.Flags().GetInt("max-locations-per-error")
				if value <= 0 {
					return fmt.Errorf("--max-locations-per-error must be a positive integer")
				}
				toolArgs["maxLocationsPerError"] = value
			}
			if cmd.Flags().Changed("max-cells") {
				value, _ := cmd.Flags().GetInt("max-cells")
				if value <= 0 {
					return fmt.Errorf("--max-cells must be a positive integer")
				}
				toolArgs["maxCells"] = value
			}
			exitOnError, _ := cmd.Flags().GetBool("exit-on-error")
			return callMCPToolSheetFormulaVerify(toolArgs, exitOnError)
		},
	}
	cmd.Flags().String("node", "", "表格文档 ID 或 URL (必填)")
	cmd.Flags().String("sheet-id", "", "工作表 ID 或名称；与 --range 组成单个扫描目标")
	cmd.Flags().String("range", "", "A1 范围；必须与 --sheet-id 配合，不支持 Sheet1!A1:D10 前缀")
	cmd.Flags().String("targets", "", `扫描目标 JSON 数组、@文件路径或 - 表示 stdin；每项包含 sheetId 和可选 range`)
	cmd.Flags().Int("max-locations-per-error", 0, "每种错误类型最多返回的位置和样本数量")
	cmd.Flags().Int("max-cells", 0, "本次最多扫描的单元格数")
	cmd.Flags().Bool("exit-on-error", false, "发现公式错误时返回非 0 退出码")
	return cmd
}

func sheetFormulaVerifyTargetsFromFlags(cmd *cobra.Command) ([]map[string]any, error) {
	targetsRaw, _ := cmd.Flags().GetString("targets")
	sheetID, _ := cmd.Flags().GetString("sheet-id")
	rangeAddress, _ := cmd.Flags().GetString("range")

	if strings.TrimSpace(targetsRaw) != "" {
		if strings.TrimSpace(sheetID) != "" || strings.TrimSpace(rangeAddress) != "" {
			return nil, fmt.Errorf("--targets cannot be combined with --sheet-id or --range")
		}
		return parseSheetFormulaVerifyTargets(cmd, targetsRaw)
	}

	sheetID = strings.TrimSpace(sheetID)
	rangeAddress = strings.TrimSpace(rangeAddress)
	if sheetID == "" {
		if rangeAddress != "" {
			return nil, fmt.Errorf("--range requires --sheet-id; use --targets for multiple targets")
		}
		return nil, nil
	}
	target := map[string]any{"sheetId": sheetID}
	if rangeAddress != "" {
		if strings.Contains(rangeAddress, "!") {
			return nil, fmt.Errorf("--range must not include a sheet prefix; use --sheet-id separately")
		}
		target["range"] = rangeAddress
	}
	return []map[string]any{target}, nil
}

func parseSheetFormulaVerifyTargets(cmd *cobra.Command, value string) ([]map[string]any, error) {
	raw, err := resolveSheetFormulaVerifyTargetsInput(cmd, value)
	if err != nil {
		return nil, err
	}
	var targets []map[string]any
	if err := json.Unmarshal([]byte(raw), &targets); err != nil {
		return nil, fmt.Errorf("failed to parse --targets JSON: %w", err)
	}
	if len(targets) == 0 {
		return nil, fmt.Errorf("--targets must not be an empty array")
	}
	for index, target := range targets {
		if err := validateSheetFormulaVerifyTarget(index, target); err != nil {
			return nil, err
		}
	}
	return targets, nil
}

func resolveSheetFormulaVerifyTargetsInput(cmd *cobra.Command, value string) (string, error) {
	value = strings.TrimSpace(value)
	switch {
	case value == "-":
		data, err := io.ReadAll(cmd.InOrStdin())
		if err != nil {
			return "", fmt.Errorf("failed to read --targets from stdin: %w", err)
		}
		return string(data), nil
	case strings.HasPrefix(value, "@"):
		data, err := os.ReadFile(strings.TrimPrefix(value, "@"))
		if err != nil {
			return "", fmt.Errorf("failed to read --targets file: %w", err)
		}
		return string(data), nil
	default:
		return value, nil
	}
}

func validateSheetFormulaVerifyTarget(index int, target map[string]any) error {
	for key := range target {
		if key != "sheetId" && key != "range" {
			return fmt.Errorf("--targets[%d]: unsupported field %q; use sheetId/range only", index, key)
		}
	}
	sheetID, ok := target["sheetId"].(string)
	if !ok || strings.TrimSpace(sheetID) == "" {
		return fmt.Errorf("--targets[%d].sheetId must be a non-empty string", index)
	}
	target["sheetId"] = strings.TrimSpace(sheetID)
	if rawRange, exists := target["range"]; exists {
		rangeAddress, ok := rawRange.(string)
		if !ok {
			return fmt.Errorf("--targets[%d].range must be a string", index)
		}
		rangeAddress = strings.TrimSpace(rangeAddress)
		if rangeAddress == "" {
			delete(target, "range")
			return nil
		}
		if strings.Contains(rangeAddress, "!") {
			return fmt.Errorf("--targets[%d].range must not include a sheet prefix", index)
		}
		target["range"] = rangeAddress
	}
	return nil
}

func callMCPToolSheetFormulaVerify(toolArgs map[string]any, exitOnError bool) error {
	if deps.Caller.DryRun() {
		return callMCPToolOnServer("sheet", "verify_formula", toolArgs)
	}
	text, err := callMCPToolReturnTextOnServer(context.Background(), "sheet", "verify_formula", toolArgs)
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
		return fmt.Errorf("verify_formula returned empty result")
	}
	if err := deps.Out.PrintJSON(parsed); err != nil {
		return err
	}
	if exitOnError && sheetFormulaVerifyHasErrors(parsed) {
		return fmt.Errorf("formula errors found")
	}
	return nil
}

func sheetFormulaVerifyHasErrors(parsed map[string]any) bool {
	result := parsed
	if nested, ok := parsed["result"].(map[string]any); ok {
		result = nested
	}
	if strings.EqualFold(strings.TrimSpace(fmt.Sprint(result["status"])), "errors_found") {
		return true
	}
	totalErrors, ok := nonNegativeJSONInt(result["totalErrors"])
	return ok && totalErrors > 0
}
