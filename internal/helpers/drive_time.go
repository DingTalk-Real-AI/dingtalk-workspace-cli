package helpers

import (
	"encoding/json"
	"strconv"
	"strings"
	"time"
)

// ──────────────────────────────────────────────────────────
// 钉盘 / 知识库条目的修改时间归一化
//
// 两条路由的时间字段名与类型都不统一：钉盘返回 modifyTime，知识库返回 updateTime，
// 值可能是毫秒数字、数字字符串或 RFC3339 字符串。--latest 需要跨路由排序，
// 因此在此处收敛为统一的 Unix 毫秒。纯函数，不修改入参。
// ──────────────────────────────────────────────────────────

// 探测顺序即优先级：先精确字段名，再回落到各服务端的历史别名。
var driveModifiedTimeKeys = []string{
	"modifiedTime",
	"modifyTime",
	"modified_time",
	"gmtModified",
	"lastModifiedTime",
	"updateTime",
}

// driveModifiedMillis 按 driveModifiedTimeKeys 顺序探测条目的修改时间。
// 首个可解析出正毫秒值的字段胜出；全部缺失或不可解析时返回 (0, false)。
func driveModifiedMillis(item map[string]any) (int64, bool) {
	for _, key := range driveModifiedTimeKeys {
		v, ok := item[key]
		if !ok {
			continue
		}
		if ms, ok := driveToMillis(v); ok {
			return ms, true
		}
	}
	return 0, false
}

// driveToMillis 把时间字段归一化为 Unix 毫秒。支持毫秒时间戳（数字或数字字符串）
// 与 RFC3339 字符串；非正值与无法识别的形态一律返回 (0, false)，由调用方决定降级策略。
func driveToMillis(v any) (int64, bool) {
	switch t := v.(type) {
	case float64:
		if t <= 0 {
			return 0, false
		}
		return int64(t), true
	case json.Number:
		if n, err := t.Int64(); err == nil && n > 0 {
			return n, true
		}
	case string:
		s := strings.TrimSpace(t)
		if s == "" {
			return 0, false
		}
		if n, err := strconv.ParseInt(s, 10, 64); err == nil && n > 0 {
			return n, true
		}
		if tm, err := time.Parse(time.RFC3339, s); err == nil {
			return tm.UnixMilli(), true
		}
	}
	return 0, false
}
