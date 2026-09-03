// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package smart

import (
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut/responsecheck"
)

func attendanceRecordsResult() *contract.ResultSpec {
	return &contract.ResultSpec{
		Outcomes:   []contract.ResultOutcome{contract.ResultOutcomeSuccess, contract.ResultOutcomeFailure},
		DataSchema: json.RawMessage(`{"type":"object","description":"严格校验的个人考勤打卡流水","properties":{"count":{"type":"integer","description":"有效打卡流水数量"},"records":{"type":"array","description":"个人打卡流水","items":{"type":"object","description":"打卡流水记录","additionalProperties":true}},"complete":{"type":"boolean","description":"当前时间窗内结果是否完整"}},"required":["count","records","complete"],"additionalProperties":false}`),
	}
}

func outputStrictAttendanceRecords(rt *shortcut.RuntimeContext, data map[string]any) error {
	records, err := responsecheck.RequireObjectCollection(data, "attendance-wukong/query_check_record", "result")
	if err != nil {
		return err
	}
	seen := make(map[int64]struct{}, len(records))
	for index, record := range records {
		identity, ok := smartAttendancePositiveInteger(record["id"])
		if !ok {
			return responsecheck.Error("attendance-wukong/query_check_record", "invalid_item_identity", fmt.Sprintf("第 %d 项缺少大于 0 的稳定打卡 ID", index))
		}
		if _, duplicate := seen[identity]; duplicate {
			return responsecheck.Error("attendance-wukong/query_check_record", "duplicate_item_identity", fmt.Sprintf("第 %d 项稳定打卡 ID 重复", index))
		}
		seen[identity] = struct{}{}
	}
	return rt.Output(map[string]any{
		"count":    len(records),
		"records":  records,
		"complete": true,
	})
}

func smartAttendancePositiveInteger(value any) (int64, bool) {
	switch number := value.(type) {
	case int:
		return int64(number), number > 0
	case int32:
		return int64(number), number > 0
	case int64:
		return number, number > 0
	case float64:
		return int64(number), number > 0 && number == float64(int64(number))
	case json.Number:
		parsed, err := strconv.ParseInt(string(number), 10, 64)
		return parsed, err == nil && parsed > 0
	default:
		return 0, false
	}
}
