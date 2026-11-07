// Copyright 2026 Alibaba Group
// SPDX-License-Identifier: Apache-2.0

package helpers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"time"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/spf13/cobra"
)

const aitableCreateWaitHelp = "创建只执行一次；--wait 在写回执后只读轮询字段，连续两次读到声明的结构才完成。默认等待 30 秒（--wait-timeout，1-120 秒），不保证之后的强一致性。超时或回执不完整会保留原始回执和 ID；禁止自动重放创建。"

func declareAitableCreateWaitFlags(cmd *cobra.Command) {
	cmd.Flags().Bool("wait", false, "创建后等待连续两次只读核对字段结构通过；不重试写入")
	cmd.Flags().Int("wait-timeout", 30, "--wait 的只读等待上限，1-120 秒，从收到创建回执后开始计时")
}

// A timer seam keeps the bounded polling tests deterministic without changing
// the production deadline or retrying the non-idempotent creation request.
var aitableCreateReadbackWait = func(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func callAitableCreateWithWait(cmd *cobra.Command, tool string, args map[string]any, fields []any) error {
	wait, _ := cmd.Flags().GetBool("wait")
	seconds, _ := cmd.Flags().GetInt("wait-timeout")
	if seconds < 1 || seconds > 120 {
		return apperrors.NewValidation("--wait-timeout must be between 1 and 120 seconds")
	}
	if cmd.Flags().Changed("wait-timeout") && !wait {
		return apperrors.NewValidation("--wait-timeout requires --wait")
	}
	if !wait || deps.Caller.DryRun() {
		return callMCPToolContext(cmd.Context(), tool, args)
	}
	if err := cmd.Context().Err(); err != nil {
		return apperrors.NewAPI("创建尚未发送，请检查调用方取消或超时设置",
			apperrors.WithExecutionStarted(false), apperrors.WithCause(err))
	}
	receipt, err := CallMCPToolDataOnServer(cmd.Context(), "aitable", tool, args)
	if err != nil {
		return aitableCreateCallError(tool, args, err)
	}
	data := aitableReceiptData(receipt)
	baseID, _ := args["baseId"].(string)
	tableID, _ := args["tableId"].(string)
	if tool == "create_table" {
		tableID, _ = data["tableId"].(string)
	}
	ids := []string{}
	var receiptErr error
	if strings.TrimSpace(tableID) == "" {
		receiptErr = fmt.Errorf("creation receipt has no tableId")
	} else if tool == "create_fields" {
		ids, receiptErr = aitableCreatedFieldIDs(data, len(fields))
	}
	if receiptErr == nil {
		ctx, cancel := context.WithTimeout(cmd.Context(), time.Duration(seconds)*time.Second)
		defer cancel()
		receiptErr = waitAitableCreatedFields(ctx, baseID, tableID, ids, fields)
	}
	if receiptErr != nil {
		return apperrors.NewAPI("创建已发起，但字段结构尚未确认可读；不要重复创建",
			apperrors.WithReason("create_readback_unconfirmed"),
			apperrors.WithExecutionStarted(true), apperrors.WithRetryable(false),
			apperrors.WithHint("保留 receipt；使用 field get 核对已有 tableId/fieldId；缺失 ID 时先用 table get 查目录，禁止自动重放创建"),
			apperrors.WithDetails(map[string]any{
				"receipt": receipt, "baseId": baseID, "tableId": tableID,
				"fieldIds": ids, "readiness": "unconfirmed", "readbackError": receiptErr.Error(),
			}))
	}
	// Keep the legacy MCP envelope and its verificationStatus intact. Readiness
	// is an additive client observation, not a replacement for server truth.
	data["readiness"] = map[string]any{"status": "observed", "consecutiveReads": 2}
	return renderAitableReceipt(tool, receipt)
}

// Preserve pre-dispatch failures (including confirmation/auth/discovery), but
// never let generic network recovery hints authorize replay of a creation.
func aitableCreateCallError(tool string, args map[string]any, err error) error {
	var typed *apperrors.Error
	if errors.As(err, &typed) && typed.ExecutionStarted != nil {
		if !*typed.ExecutionStarted {
			return err
		}
	} else {
		switch apperrors.ExitCode(err) {
		case ExitAuth, ExitValidation, ExitPermission, apperrors.ExitCodeDiscovery:
			return err
		}
	}
	return apperrors.NewAPI("创建请求的执行结果不明；可能已创建成功，禁止重复创建",
		apperrors.WithReason("create_outcome_unknown"),
		apperrors.WithOperation("aitable."+tool),
		apperrors.WithExecutionStarted(true), apperrors.WithRetryable(false),
		apperrors.WithHint("保留原始错误和目标；使用 table get / field get 只读核对原 Base/Table，不能因超时或空回复重放创建"),
		apperrors.WithDetails(map[string]any{"baseId": args["baseId"], "tableId": args["tableId"]}),
		apperrors.WithCause(err))
}

func aitableReceiptData(receipt any) map[string]any {
	root, _ := receipt.(map[string]any)
	if data, ok := root["data"].(map[string]any); ok {
		return data
	}
	return root
}

func aitableCreatedFieldIDs(data map[string]any, count int) ([]string, error) {
	results, ok := data["results"].([]any)
	if !ok || len(results) != count || count == 0 {
		return nil, fmt.Errorf("create_fields receipt must contain one result for each requested field")
	}
	ids := make([]string, 0, count)
	seen := map[string]bool{}
	for index, raw := range results {
		result, _ := raw.(map[string]any)
		id, _ := result["fieldId"].(string)
		success, _ := result["success"].(bool)
		if !success || strings.TrimSpace(id) == "" || seen[id] {
			return ids, fmt.Errorf("create_fields results[%d] lacks a unique acknowledged fieldId; inspect the original receipt", index)
		}
		seen[id] = true
		ids = append(ids, id)
	}
	return ids, nil
}

func waitAitableCreatedFields(ctx context.Context, baseID, tableID string, ids []string, expected []any) error {
	consecutive := 0
	var lastErr error
	// Reserve room for both observations even for the minimum one-second budget.
	// Compute once: shrinking the interval every loop would create a busy poll
	// near the deadline. The context remains the hard limit for calls and waits.
	interval := time.Second
	if deadline, ok := ctx.Deadline(); ok {
		interval = min(interval, time.Until(deadline)/4)
	}
	for {
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("%w; last observation: %v", err, lastErr)
		}
		actual, err := readAitableCreatedFields(ctx, baseID, tableID, ids)
		if err == nil {
			err = matchAitableCreatedFields(actual, ids, expected)
		}
		if err == nil {
			consecutive++
			if consecutive == 2 {
				return nil
			}
		} else {
			consecutive = 0
		}
		lastErr = err
		if err := aitableCreateReadbackWait(ctx, interval); err != nil {
			return fmt.Errorf("%w; last observation: %v", err, lastErr)
		}
	}
}

func readAitableCreatedFields(ctx context.Context, baseID, tableID string, ids []string) ([]any, error) {
	var fields []any
	for start := 0; ; start += 10 {
		args := map[string]any{"baseId": baseID, "tableId": tableID}
		end := min(start+10, len(ids))
		if len(ids) > 0 {
			args["fieldIds"] = ids[start:end]
		}
		raw, err := CallMCPToolDataOnServer(ctx, "aitable", "get_fields", args)
		if err != nil {
			return nil, err
		}
		page, ok := aitableReceiptData(raw)["fields"].([]any)
		if !ok {
			return nil, fmt.Errorf("get_fields response has no fields array")
		}
		fields = append(fields, page...)
		if end == len(ids) {
			return fields, nil
		}
	}
}

func matchAitableCreatedFields(actual []any, ids []string, expected []any) error {
	if len(actual) == 0 {
		return fmt.Errorf("created table has no readable fields yet")
	}
	seen := map[string]bool{}
	for _, raw := range actual {
		field, _ := raw.(map[string]any)
		id, _ := field["fieldId"].(string)
		if strings.TrimSpace(id) == "" || seen[id] {
			return fmt.Errorf("get_fields returned a missing or duplicate fieldId")
		}
		seen[id] = true
	}
	for index, raw := range expected {
		want, _ := raw.(map[string]any)
		var matches []map[string]any
		for _, item := range actual {
			field, _ := item.(map[string]any)
			if (len(ids) > 0 && field["fieldId"] == ids[index]) || (len(ids) == 0 && field["fieldName"] == want["fieldName"]) {
				matches = append(matches, field)
			}
		}
		if len(matches) != 1 || !aitableDeclaredSubset(matches[0], aitableCreatedFieldExpectation(want)) {
			return fmt.Errorf("fields[%d] (%v) is missing, ambiguous or differs from the requested structure", index, want["fieldName"])
		}
	}
	return nil
}

// MCP projects an empty physical config to null/absent, and creation ignores
// an empty description. Normalize only these optional top-level members; do
// not discard nested false/zero values, option order, or nonempty config.
func aitableCreatedFieldExpectation(field map[string]any) map[string]any {
	expected := make(map[string]any, len(field))
	for key, value := range field {
		if key == "description" && (value == nil || value == "") {
			continue
		}
		if key == "config" {
			config, ok := value.(map[string]any)
			if value == nil || (ok && len(config) == 0) {
				continue
			}
		}
		expected[key] = value
	}
	return expected
}

func aitableDeclaredSubset(actual, expected any) bool {
	switch want := expected.(type) {
	case map[string]any:
		got, ok := actual.(map[string]any)
		if !ok {
			return false
		}
		for key, child := range want {
			value, exists := got[key]
			if !exists || !aitableDeclaredSubset(value, child) {
				return false
			}
		}
		return true
	case []any:
		got, ok := actual.([]any)
		if !ok || len(got) != len(want) {
			return false
		}
		for i := range want {
			if !aitableDeclaredSubset(got[i], want[i]) {
				return false
			}
		}
		return true
	default:
		return reflect.DeepEqual(actual, expected)
	}
}

func renderAitableReceipt(tool string, receipt any) error {
	encoded, err := json.Marshal(receipt)
	if err != nil {
		return err
	}
	return renderLegacyMCPText(tool, string(encoded), false)
}
