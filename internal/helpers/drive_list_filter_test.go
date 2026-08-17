package helpers

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
)

// drive list --type/--start/--end 客户端过滤测试矩阵（设计稿 §9.3）。
// 命令级用例统一经 executeDriveListCapture 捕获 stdout JSON；
// 零值 filter 的存量分支行为由 drive_depth_test.go / drive_list_modes_test.go 既有用例兜底。

// 钉盘路由（serverID 空）经 resolveProductID 扫 os.Args 推断产品，命令级测试需伪装命令行。
func useDriveListArgs(t *testing.T) {
	t.Helper()
	old := os.Args
	os.Args = []string{"dws", "drive", "list"}
	t.Cleanup(func() { os.Args = old })
}

func executeDriveListCapture(t *testing.T, caller edition.ToolCaller, args ...string) (*bytes.Buffer, error) {
	t.Helper()
	previousDeps := deps
	t.Cleanup(func() { deps = previousDeps })
	InitDeps(caller)
	buf := &bytes.Buffer{}
	deps.Out.w = buf
	deps.Out.errW = io.Discard
	root := newDriveCommand()
	root.SilenceErrors = true
	root.SilenceUsage = true
	root.SetArgs(args)
	err := root.Execute()
	return buf, err
}

// 1. 正向：--workspace --type file → list_nodes 全量拉取后仅 file，单层剥装饰字段。
func TestCrossPlatformCoverageDriveListFilterWorkspaceTypeFile(t *testing.T) {
	caller := &scriptedToolCaller{steps: []scriptedToolStep{
		{text: `{"nodes":[
			{"nodeId":"n1","name":"doc1","nodeType":"doc","updateTime":1754000000000},
			{"nodeId":"n2","name":"sub","nodeType":"folder","updateTime":1754000000000}
		],"hasMore":false}`},
	}}
	buf, err := executeDriveListCapture(t, caller, "list", "--workspace", "ws-1", "--type", "file")
	if err != nil {
		t.Fatal(err)
	}
	if caller.calls != 1 {
		t.Fatalf("calls = %d, want 1（depth=1 退化态不下钻 folder）", caller.calls)
	}
	result := decodeDepthResult(t, buf)
	items := result["items"].([]any)
	if len(items) != 1 {
		t.Fatalf("filtered items = %#v", items)
	}
	item := items[0].(map[string]any)
	if item["nodeId"] != "n1" {
		t.Fatalf("item = %#v", item)
	}
	for _, key := range []string{"depth", "parentId", "rel_path", "sortTime"} {
		if _, ok := item[key]; ok {
			t.Fatalf("decoration %q not stripped: %#v", key, item)
		}
	}
	if result["truncated"] != false || result["maxDepth"] != float64(1) {
		t.Fatalf("result = %#v", result)
	}
}

// 2. 正向：--workspace --start 7d --end <RFC3339> → sortTime 区间断言。
func TestCrossPlatformCoverageDriveListFilterWorkspaceTimeRange(t *testing.T) {
	now := time.Now()
	within := now.Add(-24 * time.Hour).UnixMilli()
	before := now.Add(-30 * 24 * time.Hour).UnixMilli()
	after := now.Add(24 * time.Hour).UnixMilli()
	caller := &scriptedToolCaller{steps: []scriptedToolStep{
		{text: fmt.Sprintf(`{"nodes":[
			{"nodeId":"in","name":"within","nodeType":"doc","updateTime":%d},
			{"nodeId":"old","name":"before","nodeType":"doc","updateTime":%d},
			{"nodeId":"new","name":"after","nodeType":"doc","updateTime":%d}
		],"hasMore":false}`, within, before, after)},
	}}
	buf, err := executeDriveListCapture(t, caller,
		"list", "--workspace", "ws-1", "--start", "7d", "--end", now.Format(time.RFC3339))
	if err != nil {
		t.Fatal(err)
	}
	items := decodeDepthResult(t, buf)["items"].([]any)
	if len(items) != 1 || items[0].(map[string]any)["nodeId"] != "in" {
		t.Fatalf("time-range items = %#v", items)
	}
}

// 3. 组合：--workspace --depth 2 --type file → BFS 路径 filter 生效，装饰字段保留。
func TestCrossPlatformCoverageDriveListFilterWithDepthKeepsDecorations(t *testing.T) {
	caller := &scriptedToolCaller{steps: []scriptedToolStep{
		{text: `{"nodes":[
			{"nodeId":"fA","name":"dirA","nodeType":"folder"},
			{"nodeId":"n1","name":"doc1","nodeType":"doc"}
		],"hasMore":false}`},
		{text: `{"nodes":[{"nodeId":"n2","name":"doc2","nodeType":"doc"}],"hasMore":false}`},
	}}
	buf, err := executeDriveListCapture(t, caller,
		"list", "--workspace", "ws-1", "--depth", "2", "--type", "file")
	if err != nil {
		t.Fatal(err)
	}
	if caller.calls != 2 {
		t.Fatalf("calls = %d, want 2（depth=2 下钻 dirA）", caller.calls)
	}
	items := decodeDepthResult(t, buf)["items"].([]any)
	if len(items) != 2 {
		t.Fatalf("filtered items = %#v（folder 应被滤除）", items)
	}
	for _, raw := range items {
		item := raw.(map[string]any)
		if _, ok := item["depth"]; !ok {
			t.Fatalf("depth>1 装饰字段应保留: %#v", item)
		}
	}
}

// 4. 组合：--workspace --type file --latest 3 → filter 先筛后 Top-N；
// 触顶时 LATEST_SCAN_TRUNCATED 拒绝优先于 filter 的 partial 语义。
func TestCrossPlatformCoverageDriveListFilterWithLatestTopN(t *testing.T) {
	caller := &scriptedToolCaller{steps: []scriptedToolStep{
		{text: `{"nodes":[
			{"nodeId":"d1","name":"dir","nodeType":"folder","updateTime":400},
			{"nodeId":"f1","name":"a","nodeType":"doc","updateTime":100},
			{"nodeId":"f2","name":"b","nodeType":"doc","updateTime":300},
			{"nodeId":"f3","name":"c","nodeType":"doc","updateTime":200},
			{"nodeId":"f4","name":"d","nodeType":"doc","updateTime":50}
		],"hasMore":false}`},
	}}
	buf, err := executeDriveListCapture(t, caller,
		"list", "--workspace", "ws-1", "--type", "file", "--latest", "2")
	if err != nil {
		t.Fatal(err)
	}
	items := decodeDepthResult(t, buf)["items"].([]any)
	if len(items) != 2 {
		t.Fatalf("top-n items = %#v", items)
	}
	// folder（updateTime 最大）不得进入 Top-N 排序基；筛选后按 sortTime 倒序。
	if items[0].(map[string]any)["nodeId"] != "f2" || items[1].(map[string]any)["nodeId"] != "f3" {
		t.Fatalf("top-n order = %#v", items)
	}
}

func TestCrossPlatformCoverageDriveListFilterLatestTruncatedRejected(t *testing.T) {
	useDriveListArgs(t)
	var sb strings.Builder
	sb.WriteString(`{"items":[`)
	for i := 0; i < driveDepthMaxItems; i++ {
		if i > 0 {
			sb.WriteString(",")
		}
		fmt.Fprintf(&sb, `{"fileId":"f%d","name":"file-%d.txt","type":"FILE","modifyTime":%d}`, i, i, 1000+i)
	}
	sb.WriteString(`]}`)
	caller := &scriptedToolCaller{steps: []scriptedToolStep{{text: sb.String()}}}
	_, err := executeDriveListCapture(t, caller, "list", "--type", "file", "--latest", "2")
	if err == nil || !strings.Contains(err.Error(), "LATEST_SCAN_TRUNCATED") {
		t.Fatalf("err = %v, want LATEST_SCAN_TRUNCATED", err)
	}
}

// 5. 钉盘路由：--folder + --type file 单层退化态全量翻页后仅 file；
// --start 7d --latest 3 走 BFS(depth=1)；filter 与 cursor/order-by/order/limit/versions 互斥退出码 3。
func TestCrossPlatformCoverageDriveListFilterPanFolderTypeFile(t *testing.T) {
	useDriveListArgs(t)
	caller := &depthArgsRecordingCaller{steps: []scriptedToolStep{
		{text: `{"items":[
			{"fileId":"d1","name":"sub","type":"FOLDER"},
			{"fileId":"f1","name":"a.txt","type":"FILE"}
		],"nextToken":"p2"}`},
		{text: `{"items":[{"fileId":"f2","name":"b.txt","type":"FILE"}]}`},
	}}
	buf, err := executeDriveListCapture(t, caller, "list", "--folder", "root-1", "--type", "file")
	if err != nil {
		t.Fatal(err)
	}
	if len(caller.calls) != 2 {
		t.Fatalf("calls = %d, want 2（翻完当前目录两页，sub 不下钻）", len(caller.calls))
	}
	if caller.calls[0]["parentId"] != "root-1" || caller.calls[0]["maxResults"] != float64(driveDepthPageSize) {
		t.Fatalf("page args = %#v", caller.calls[0])
	}
	if caller.calls[1]["nextToken"] != "p2" {
		t.Fatalf("page2 args = %#v", caller.calls[1])
	}
	items := decodeDepthResult(t, buf)["items"].([]any)
	if len(items) != 2 {
		t.Fatalf("filtered items = %#v", items)
	}
	for _, raw := range items {
		item := raw.(map[string]any)
		if item["type"] != "FILE" {
			t.Fatalf("folder leaked: %#v", item)
		}
		if _, ok := item["depth"]; ok {
			t.Fatalf("single-layer decorations not stripped: %#v", item)
		}
	}
}

func TestCrossPlatformCoverageDriveListFilterPanStartWithLatest(t *testing.T) {
	useDriveListArgs(t)
	now := time.Now()
	within := now.Add(-24 * time.Hour).UnixMilli()
	stale := now.Add(-30 * 24 * time.Hour).UnixMilli()
	caller := &scriptedToolCaller{steps: []scriptedToolStep{
		{text: fmt.Sprintf(`{"items":[
			{"fileId":"f1","name":"a.txt","type":"FILE","modifyTime":%d},
			{"fileId":"f2","name":"b.txt","type":"FILE","modifyTime":%d},
			{"fileId":"f3","name":"c.txt","type":"FILE","modifyTime":%d}
		]}`, within-1000, within, stale)},
	}}
	buf, err := executeDriveListCapture(t, caller, "list", "--start", "7d", "--latest", "1")
	if err != nil {
		t.Fatal(err)
	}
	result := decodeDepthResult(t, buf)
	if _, ok := result["truncated"]; !ok {
		t.Fatalf("filter+latest 应走 BFS 聚合形态: %#v", result)
	}
	items := result["items"].([]any)
	// --start 7d 先筛掉 stale，Top-1 取 sortTime 最大的 f2。
	if len(items) != 1 || items[0].(map[string]any)["fileId"] != "f2" {
		t.Fatalf("items = %#v", items)
	}
}

func TestCrossPlatformCoverageDriveListFilterExclusiveFlags(t *testing.T) {
	cases := [][]string{
		{"list", "--type", "file", "--cursor", "c1"},
		{"list", "--type", "file", "--cursor", ""},
		{"list", "--type", "file", "--order-by", "name"},
		{"list", "--start", "7d", "--order", "asc"},
		{"list", "--type", "file", "--limit", "5"},
		{"list", "--end", "2026-08-01", "--max", "5"},
		{"list", "--versions", "--node", "n1", "--type", "file"},
	}
	for _, args := range cases {
		caller := &scriptedToolCaller{}
		_, err := executeDriveListCapture(t, caller, args...)
		var cliErr *CLIError
		if !errors.As(err, &cliErr) || cliErr.ExitCode() != ExitValidation {
			t.Fatalf("args %v: err = %v, want CLIError exit 3", args, err)
		}
		if !strings.Contains(err.Error(), "不能与") || !strings.Contains(err.Error(), "同时使用") {
			t.Fatalf("args %v: err = %v, want exclusivity message", args, err)
		}
		if caller.calls != 0 {
			t.Fatalf("args %v: calls = %d, want 0", args, caller.calls)
		}
	}
}

// 6. 边界：--type 非法值列合法值；start>end 退出码 3；相对时间解析与 m/毫秒拒绝。
func TestCrossPlatformCoverageDriveListFilterInvalidValues(t *testing.T) {
	caller := &scriptedToolCaller{}
	_, err := executeDriveListCapture(t, caller, "list", "--type", "dir")
	var cliErr *CLIError
	if !errors.As(err, &cliErr) || cliErr.ExitCode() != ExitValidation {
		t.Fatalf("--type dir err = %v, want exit 3", err)
	}
	if !strings.Contains(err.Error(), "file|folder") {
		t.Fatalf("--type dir err = %v, want valid values listed", err)
	}

	_, err = executeDriveListCapture(t, caller, "list", "--start", "2026-08-10", "--end", "2026-08-01")
	if !errors.As(err, &cliErr) || cliErr.ExitCode() != ExitValidation {
		t.Fatalf("start>end err = %v, want exit 3", err)
	}
	if !strings.Contains(err.Error(), "晚于") {
		t.Fatalf("start>end err = %v", err)
	}
	if caller.calls != 0 {
		t.Fatalf("calls = %d, want 0", caller.calls)
	}
}

func TestCrossPlatformCoverageDriveListFilterTimeParsing(t *testing.T) {
	// 相对时间按本机时钟换算为绝对毫秒。
	for _, tc := range []struct {
		raw  string
		unit time.Duration
	}{
		{"24h", time.Hour},
		{"7d", 24 * time.Hour},
		{"2w", 7 * 24 * time.Hour},
	} {
		before := time.Now()
		ms, err := parseDriveListTime(tc.raw)
		after := time.Now()
		if err != nil {
			t.Fatalf("%s: %v", tc.raw, err)
		}
		expected := time.Duration(mustAtoi(t, tc.raw[:len(tc.raw)-1])) * tc.unit
		lo := before.Add(-expected).UnixMilli()
		hi := after.Add(-expected).UnixMilli()
		if ms < lo || ms > hi {
			t.Fatalf("%s: ms = %d, want in [%d, %d]", tc.raw, ms, lo, hi)
		}
	}

	// RFC3339 / 无时区 ISO8601（默认 Asia/Shanghai）/ 仅日期。
	shanghai := time.FixedZone("CST", 8*3600)
	for _, tc := range []struct {
		raw  string
		want int64
	}{
		{"2026-08-01T00:00:00+08:00", time.Date(2026, 8, 1, 0, 0, 0, 0, shanghai).UnixMilli()},
		{"2026-08-01 08:00:00", time.Date(2026, 8, 1, 8, 0, 0, 0, shanghai).UnixMilli()},
		{"2026-08-01", time.Date(2026, 8, 1, 0, 0, 0, 0, shanghai).UnixMilli()},
	} {
		ms, err := parseDriveListTime(tc.raw)
		if err != nil || ms != tc.want {
			t.Fatalf("%s: ms = %d err = %v, want %d", tc.raw, ms, err, tc.want)
		}
	}

	// m 单位 / 毫秒时间戳 / 零与负值一律拒绝。
	for _, raw := range []string{"7m", "30m", "1754000000000", "0d", "-3d", "abc"} {
		if _, err := parseDriveListTime(raw); err == nil {
			t.Fatalf("%q accepted, want rejection", raw)
		}
	}
}

func mustAtoi(t *testing.T, s string) int {
	t.Helper()
	n := 0
	if _, err := fmt.Sscanf(s, "%d", &n); err != nil {
		t.Fatalf("atoi %q: %v", s, err)
	}
	return n
}

// 7. dry-run：--workspace --type file --dry-run → printDriveDepthDryRun 路径，不发起调用。
func TestCrossPlatformCoverageDriveListFilterDryRun(t *testing.T) {
	caller := &scriptedToolCaller{format: "json", dry: true}
	buf, err := executeDriveListCapture(t, caller, "list", "--workspace", "ws-1", "--type", "file")
	if err != nil {
		t.Fatal(err)
	}
	if caller.calls != 0 {
		t.Fatalf("dry-run calls = %d", caller.calls)
	}
	result := decodeDepthResult(t, buf)
	if result["dry_run"] != true || result["tool"] != "list_nodes" || result["maxDepth"] != float64(1) {
		t.Fatalf("dry-run payload = %#v", result)
	}
}

// 8. 触顶：2000 条上限 → partial + truncated=true + 退出码 0（filter 触顶结果不全但不会错）。
func TestCrossPlatformCoverageDriveListFilterTruncatedPartial(t *testing.T) {
	useDriveListArgs(t)
	var sb strings.Builder
	sb.WriteString(`{"items":[`)
	for i := 0; i < driveDepthMaxItems; i++ {
		if i > 0 {
			sb.WriteString(",")
		}
		fmt.Fprintf(&sb, `{"fileId":"f%d","name":"file-%d.txt","type":"FILE","modifyTime":%d}`, i, i, 1000+i)
	}
	sb.WriteString(`]}`)
	caller := &scriptedToolCaller{steps: []scriptedToolStep{{text: sb.String()}}}
	buf, err := executeDriveListCapture(t, caller, "list", "--type", "file")
	if err != nil {
		t.Fatalf("filter 触顶应放行（退出码 0）: %v", err)
	}
	result := decodeDepthResult(t, buf)
	if result["truncated"] != true {
		t.Fatalf("truncated = %#v", result["truncated"])
	}
	if got := len(result["items"].([]any)); got != driveDepthMaxItems {
		t.Fatalf("items len = %d, want %d（全 FILE 类型筛全保留）", got, driveDepthMaxItems)
	}
}

// 9. 回归：不带 filter 的存量分支行为不变；--pattern 单层钉盘页内过滤为本次有意修复。
func TestCrossPlatformCoverageDriveListFilterRegressionUnaffectedPaths(t *testing.T) {
	useDriveListArgs(t)
	// 单层纯透传（无 filter/pattern）：body 原样输出，args 语义不变。
	caller := &scriptedToolCaller{steps: []scriptedToolStep{
		{text: `{"items":[{"fileId":"f1","name":"a.txt"}],"nextToken":"nt"}`},
	}}
	buf, err := executeDriveListCapture(t, caller, "list")
	if err != nil {
		t.Fatal(err)
	}
	result := decodeDepthResult(t, buf)
	if result["nextToken"] != "nt" || len(result["items"].([]any)) != 1 {
		t.Fatalf("透传 body 被改变: %#v", result)
	}
	if _, ok := result["truncated"]; ok {
		t.Fatalf("透传形态不应含 BFS 聚合字段: %#v", result)
	}
	if caller.args["maxResults"] != float64(20) {
		t.Fatalf("args = %#v", caller.args)
	}

	// workspace 单层透传：无 filter/depth/latest 时仍走单页 list_nodes。
	caller2 := &scriptedToolCaller{steps: []scriptedToolStep{
		{text: `{"nodes":[{"nodeId":"n1","name":"doc1","nodeType":"doc"}],"hasMore":false}`},
	}}
	buf2, err := executeDriveListCapture(t, caller2, "list", "--workspace", "ws-1")
	if err != nil {
		t.Fatal(err)
	}
	result2 := decodeDepthResult(t, buf2)
	if _, ok := result2["nodes"]; !ok {
		t.Fatalf("workspace 单层透传形态被改变: %#v", result2)
	}
	if caller2.args["pageSize"] != 20 || caller2.args["workspaceId"] != "ws-1" {
		t.Fatalf("args = %#v", caller2.args)
	}

	// 钉盘单层 --latest（无 filter）：仍走 runDriveListLatest（非 BFS 聚合形态）。
	now := time.Now().UnixMilli()
	caller3 := &depthArgsRecordingCaller{steps: []scriptedToolStep{
		{text: fmt.Sprintf(`{"items":[{"fileId":"f1","name":"a.txt","type":"FILE","modifyTime":%d}],"nextToken":""}`, now)},
	}}
	buf3, err := executeDriveListCapture(t, caller3, "list", "--latest", "1")
	if err != nil {
		t.Fatal(err)
	}
	result3 := decodeDepthResult(t, buf3)
	if _, ok := result3["truncated"]; ok {
		t.Fatalf("单层 latest 不应切到 BFS 聚合形态: %#v", result3)
	}
	if len(result3["items"].([]any)) != 1 {
		t.Fatalf("items = %#v", result3["items"])
	}
	if caller3.calls[0]["orderBy"] != "modifyTime" || caller3.calls[0]["order"] != "desc" {
		t.Fatalf("latest 扫描参数被改变: %#v", caller3.calls[0])
	}
}

func TestCrossPlatformCoverageDriveListPatternSingleLayerFixed(t *testing.T) {
	useDriveListArgs(t)
	caller := &scriptedToolCaller{steps: []scriptedToolStep{
		{text: `{"items":[
			{"fileId":"f1","name":"a.txt"},
			{"fileId":"f2","name":"b.md"}
		],"nextToken":"nt"}`},
	}}
	buf, err := executeDriveListCapture(t, caller, "list", "--pattern", "*.txt")
	if err != nil {
		t.Fatal(err)
	}
	result := decodeDepthResult(t, buf)
	items := result["items"].([]any)
	if len(items) != 1 || items[0].(map[string]any)["fileId"] != "f1" {
		t.Fatalf("pattern 页内过滤未生效: %#v", items)
	}
	if result["nextToken"] != "nt" {
		t.Fatalf("分页字段应保留: %#v", result)
	}
}

// 10. 钉盘路由：--type folder 反向筛（FILE 被滤除，仅 FOLDER 保留）。
func TestCrossPlatformCoverageDriveListFilterPanFolderTypeFolder(t *testing.T) {
	useDriveListArgs(t)
	caller := &scriptedToolCaller{steps: []scriptedToolStep{
		{text: `{"items":[
			{"fileId":"d1","name":"sub","type":"FOLDER"},
			{"fileId":"f1","name":"a.txt","type":"FILE"}
		]}`},
	}}
	buf, err := executeDriveListCapture(t, caller, "list", "--folder", "root-1", "--type", "folder")
	if err != nil {
		t.Fatal(err)
	}
	if caller.calls != 1 {
		t.Fatalf("calls = %d, want 1（depth=1 退化态不下钻 folder）", caller.calls)
	}
	items := decodeDepthResult(t, buf)["items"].([]any)
	if len(items) != 1 || items[0].(map[string]any)["fileId"] != "d1" {
		t.Fatalf("folder filter items = %#v", items)
	}
}

// 11. 钉盘路由：--start 下无时间信息的条目（sortTime<=0）不可判定，保守滤除。
func TestCrossPlatformCoverageDriveListFilterPanMissingTimeDropped(t *testing.T) {
	useDriveListArgs(t)
	now := time.Now().UnixMilli()
	caller := &scriptedToolCaller{steps: []scriptedToolStep{
		{text: fmt.Sprintf(`{"items":[
			{"fileId":"f1","name":"a.txt","type":"FILE","modifyTime":%d},
			{"fileId":"f2","name":"no-time.txt","type":"FILE"}
		]}`, now)},
	}}
	buf, err := executeDriveListCapture(t, caller, "list", "--start", "7d")
	if err != nil {
		t.Fatal(err)
	}
	items := decodeDepthResult(t, buf)["items"].([]any)
	if len(items) != 1 || items[0].(map[string]any)["fileId"] != "f1" {
		t.Fatalf("missing-time 条目应被滤除: %#v", items)
	}
}

// 12. 单层钉盘 --pattern 页内过滤边界：dry-run 预览 / MCP 错误透传 / 非 JSON 原样输出 /
// 非 map 条目保留 + name 缺失时 fileName 兜底匹配。
func TestCrossPlatformCoverageDriveListPatternPassthroughEdges(t *testing.T) {
	useDriveListArgs(t)

	// dry-run：打印预览，不发起调用。
	dryCaller := &scriptedToolCaller{dry: true}
	buf, err := executeDriveListCapture(t, dryCaller, "list", "--pattern", "*.txt")
	if err != nil {
		t.Fatal(err)
	}
	if dryCaller.calls != 0 {
		t.Fatalf("dry-run calls = %d", dryCaller.calls)
	}
	preview := decodeDepthResult(t, buf)
	if preview["dry_run"] != true || preview["tool"] != "list_files" || preview["pattern"] != "*.txt" {
		t.Fatalf("dry-run payload = %#v", preview)
	}

	// MCP 错误：原样返回。
	errCaller := &scriptedToolCaller{steps: []scriptedToolStep{{err: errors.New("boom")}}}
	if _, err := executeDriveListCapture(t, errCaller, "list", "--pattern", "*.txt"); err == nil || !strings.Contains(err.Error(), "boom") {
		t.Fatalf("err = %v, want boom", err)
	}

	// 非 JSON 返回：无法筛，原样输出（与透传分支兜底形态一致）。
	rawCaller := &scriptedToolCaller{steps: []scriptedToolStep{{text: "plain-text-result"}}}
	rawBuf, err := executeDriveListCapture(t, rawCaller, "list", "--pattern", "*.txt")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(rawBuf.String(), "plain-text-result") {
		t.Fatalf("raw output = %q", rawBuf.String())
	}

	// 非 map 条目原样保留；name 缺失时 fileName 兜底参与匹配。
	mixedCaller := &scriptedToolCaller{steps: []scriptedToolStep{
		{text: `{"items":[
			"raw-entry",
			{"fileId":"f1","fileName":"日报模板.doc"},
			{"fileId":"f2","name":"其他.txt"}
		],"nextToken":"nt"}`},
	}}
	mixedBuf, err := executeDriveListCapture(t, mixedCaller, "list", "--pattern", "*模板*")
	if err != nil {
		t.Fatal(err)
	}
	mixed := decodeDepthResult(t, mixedBuf)["items"].([]any)
	if len(mixed) != 2 || mixed[0] != "raw-entry" || mixed[1].(map[string]any)["fileId"] != "f1" {
		t.Fatalf("items = %#v", mixed)
	}
}

// 13. --type folder --latest 回归（P1 CR）：latest 对过滤后的目录按时间取 Top-N，
// 不再「先留目录再剔目录」返回空列表。
func TestCrossPlatformCoverageDriveListFilterFolderLatest(t *testing.T) {
	useDriveListArgs(t)
	now := time.Now().UnixMilli()
	caller := &scriptedToolCaller{steps: []scriptedToolStep{
		{text: fmt.Sprintf(`{"items":[
			{"fileId":"d1","name":"old-dir","type":"FOLDER","modifyTime":%d},
			{"fileId":"d2","name":"new-dir","type":"FOLDER","modifyTime":%d},
			{"fileId":"f1","name":"a.txt","type":"FILE","modifyTime":%d}
		]}`, now-5000, now, now)},
	}}
	buf, err := executeDriveListCapture(t, caller, "list", "--type", "folder", "--latest", "1")
	if err != nil {
		t.Fatal(err)
	}
	items := decodeDepthResult(t, buf)["items"].([]any)
	if len(items) != 1 || items[0].(map[string]any)["fileId"] != "d2" {
		t.Fatalf("folder latest items = %#v", items)
	}
}
