package helpers

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func driveLatestTestCmd() *cobra.Command {
	cmd := driveDepthTestCmd()
	cmd.Flags().String("order-by", "", "")
	cmd.Flags().String("order", "", "")
	return cmd
}

func useDriveLatestArgs(t *testing.T) {
	t.Helper()
	old := os.Args
	os.Args = []string{"dws", "drive", "list", "--latest", "5"}
	t.Cleanup(func() { os.Args = old })
}

// captureDriveLatestStderr 抓取直写 os.Stderr 的进度/提示行（不经 deps.Out.errW）。
func captureDriveLatestStderr(t *testing.T, run func()) string {
	t.Helper()
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatalf("create stderr pipe: %v", err)
	}
	previous := os.Stderr
	os.Stderr = writer
	defer func() {
		os.Stderr = previous
		_ = reader.Close()
	}()

	run()
	os.Stderr = previous
	if err := writer.Close(); err != nil {
		t.Fatalf("close stderr writer: %v", err)
	}
	output, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("read stderr: %v", err)
	}
	return string(output)
}

// drivePanPage 造一页钉盘响应：首条为文件夹（供 BFS 下钻），其余为文件。
func drivePanPage(prefix string, files int, nextToken string) string {
	entries := []string{fmt.Sprintf(`{"fileId":%q,"name":"%s-dir","type":"FOLDER"}`, prefix+"-dir", prefix)}
	for i := 0; i < files; i++ {
		entries = append(entries, fmt.Sprintf(
			`{"fileId":"%s-f%d","name":"%s-f%d.txt","type":"FILE","modifyTime":%d}`,
			prefix, i, prefix, i, 1700000000000+i))
	}
	body := fmt.Sprintf(`{"items":[%s]`, strings.Join(entries, ","))
	if nextToken != "" {
		body += fmt.Sprintf(`,"nextToken":%q`, nextToken)
	}
	return body + "}"
}

func docPage(prefix string, nodes int, nextToken string) string {
	entries := make([]string, 0, nodes)
	for i := 0; i < nodes; i++ {
		entries = append(entries, fmt.Sprintf(
			`{"nodeId":"%s-n%d","name":"%s-n%d","nodeType":"doc","updateTime":%d}`,
			prefix, i, prefix, i, 1700000000000+i))
	}
	body := fmt.Sprintf(`{"nodes":[%s],"hasMore":true`, strings.Join(entries, ","))
	if nextToken != "" {
		body += fmt.Sprintf(`,"nextPageToken":%q`, nextToken)
	}
	return body + "}"
}

func TestCrossPlatformCoverageValidateDriveListLatest(t *testing.T) {
	if err := validateDriveListLatest(driveLatestTestCmd(), 0); err == nil {
		t.Fatal("latest 0 returned nil")
	}
	if err := validateDriveListLatest(driveLatestTestCmd(), driveLatestMax+1); err == nil {
		t.Fatal("latest above max returned nil")
	}
	if err := validateDriveListLatest(driveLatestTestCmd(), 1); err != nil {
		t.Fatalf("latest 1 returned %v", err)
	}
	if err := validateDriveListLatest(driveLatestTestCmd(), driveLatestMax); err != nil {
		t.Fatalf("latest max returned %v", err)
	}

	for _, tc := range []struct{ flag, value string }{
		{"order-by", "name"},
		{"order", "asc"},
		{"limit", "5"},
		{"max", "5"},
		{"cursor", "c1"},
		{"next-token", "t1"},
	} {
		cmd := driveLatestTestCmd()
		if err := cmd.Flags().Set(tc.flag, tc.value); err != nil {
			t.Fatalf("set --%s: %v", tc.flag, err)
		}
		err := validateDriveListLatest(cmd, 3)
		if err == nil {
			t.Fatalf("--latest with --%s returned nil", tc.flag)
		}
		cliErr, ok := err.(*CLIError)
		if !ok || cliErr.Code != CodeInvalidParam {
			t.Fatalf("--%s error = %#v", tc.flag, err)
		}
		// 互斥报错必须给出等效改写，否则 Agent 只能盲试。
		if !strings.Contains(cliErr.Message, "--order-by modifyTime --order desc --limit 3") {
			t.Fatalf("--%s message lacks rewrite hint: %s", tc.flag, cliErr.Message)
		}
	}
}

func TestCrossPlatformCoverageApplyDriveListLatest(t *testing.T) {
	items := []map[string]any{
		{"fileId": "dir", "name": "dir", "type": "FOLDER", "sortTime": int64(9999)},
		{"nodeId": "docdir", "name": "docdir", "nodeType": "folder", "sortTime": int64(9999)},
		{"fileId": "old", "rel_path": "a/old.txt", "sortTime": int64(100)},
		{"fileId": "new", "rel_path": "a/new.txt", "sortTime": int64(300)},
		{"fileId": "mid", "rel_path": "a/mid.txt", "sortTime": int64(200)},
		{"fileId": "notime", "rel_path": "a/notime.txt", "sortTime": int64(0)},
	}
	got := applyDriveListLatest(items, 2)
	if len(got) != 2 {
		t.Fatalf("truncate to 2 = %#v", got)
	}
	if got[0]["fileId"] != "new" || got[1]["fileId"] != "mid" {
		t.Fatalf("sortTime desc order = %#v", got)
	}

	all := applyDriveListLatest(items, 10)
	if len(all) != 4 {
		t.Fatalf("folders should be dropped: %#v", all)
	}
	if all[3]["fileId"] != "notime" {
		t.Fatalf("missing timestamp should sort last: %#v", all)
	}
}

func TestCrossPlatformCoverageApplyDriveListLatestTieBreak(t *testing.T) {
	same := []map[string]any{
		{"fileId": "z9", "rel_path": "b/dup.txt", "sortTime": int64(500)},
		{"fileId": "a1", "rel_path": "b/dup.txt", "sortTime": int64(500)},
		{"fileId": "m5", "rel_path": "a/dup.txt", "sortTime": int64(500)},
	}
	got := applyDriveListLatest(same, 3)
	// 同 sortTime → rel_path 升序 → id 升序，输出完全确定。
	if got[0]["fileId"] != "m5" || got[1]["fileId"] != "a1" || got[2]["fileId"] != "z9" {
		t.Fatalf("tie-break order = %#v", got)
	}
}

func TestCrossPlatformCoverageStripDriveDepthDecorations(t *testing.T) {
	items := []map[string]any{
		{"fileId": "f1", "depth": 1, "parentId": "p", "rel_path": "a", "sortTime": int64(1), "name": "keep"},
	}
	stripDriveDepthDecorations(items)
	for _, key := range []string{"depth", "parentId", "rel_path", "sortTime"} {
		if _, ok := items[0][key]; ok {
			t.Fatalf("%s not stripped: %#v", key, items[0])
		}
	}
	if items[0]["name"] != "keep" || items[0]["fileId"] != "f1" {
		t.Fatalf("business fields lost: %#v", items[0])
	}
}

func TestCrossPlatformCoverageDriveLatestTruncatedRefusesTopN(t *testing.T) {
	useDriveLatestArgs(t)
	// 每页 50 条（1 文件夹 + 49 文件）且始终有 nextToken：40 页打满全局 2000 上限。
	caller := &scriptedToolCaller{steps: []scriptedToolStep{
		{text: drivePanPage("p", driveDepthPageSize-1, "next")},
	}}
	out, err := runDepthBFSRaw(t, caller, newDrivePanDepthRoute(), "", 3, "", 5)
	if err == nil {
		t.Fatal("truncated latest returned nil error")
	}
	cliErr, ok := err.(*CLIError)
	if !ok || cliErr.Code != CodeContentTruncated {
		t.Fatalf("error = %#v", err)
	}
	if !strings.Contains(cliErr.Message, "LATEST_SCAN_TRUNCATED") {
		t.Fatalf("message = %q", cliErr.Message)
	}
	if cliErr.Suggestion == "" {
		t.Fatal("truncation error must carry a suggestion")
	}
	// 拒绝即不产出：截断集上的 Top-N 不是全局最新，stdout 必须为空。
	if out.Len() != 0 {
		t.Fatalf("stdout should be empty, got %q", out.String())
	}
	if caller.calls != driveDepthMaxItems/driveDepthPageSize {
		t.Fatalf("calls = %d, want %d", caller.calls, driveDepthMaxItems/driveDepthPageSize)
	}
}

func TestCrossPlatformCoverageDocLatestTruncatedRefusesTopN(t *testing.T) {
	caller := &scriptedToolCaller{steps: []scriptedToolStep{
		{text: docPage("d", docDepthPageSize, "next")},
	}}
	out, err := runDepthBFSRaw(t, caller, newDocDepthRoute(), "root1", 1, "", 5)
	cliErr, ok := err.(*CLIError)
	if !ok || cliErr.Code != CodeContentTruncated {
		t.Fatalf("error = %#v", err)
	}
	if out.Len() != 0 {
		t.Fatalf("stdout should be empty, got %q", out.String())
	}
}

func TestCrossPlatformCoverageDriveDepthTruncatedWithoutLatestKeepsPartial(t *testing.T) {
	useDriveDepthArgs(t)
	caller := &scriptedToolCaller{steps: []scriptedToolStep{
		{text: drivePanPage("p", driveDepthPageSize-1, "next")},
	}}
	// 回归护栏：不带 latest 时截断仍以 truncated:true 输出满额部分结果。
	result, err := runDepthBFS(t, caller, newDrivePanDepthRoute(), "", 3, "")
	if err != nil {
		t.Fatal(err)
	}
	if result["truncated"] != true {
		t.Fatalf("truncated = %#v", result["truncated"])
	}
	if items := result["items"].([]any); len(items) != driveDepthMaxItems {
		t.Fatalf("items = %d, want %d", len(items), driveDepthMaxItems)
	}
}

func TestCrossPlatformCoverageDocLatestSingleLayerStripsDecorations(t *testing.T) {
	caller := &scriptedToolCaller{steps: []scriptedToolStep{
		{text: `{"nodes":[
			{"nodeId":"nDir","name":"folder","nodeType":"folder","updateTime":1700000009000},
			{"nodeId":"nOld","name":"old.doc","nodeType":"doc","updateTime":1700000001000},
			{"nodeId":"nNew","name":"new.doc","nodeType":"doc","updateTime":1700000005000}],"hasMore":false}`},
	}}
	result, err := runDepthBFSLatest(t, caller, newDocDepthRoute(), "root1", 1, "", 2)
	if err != nil {
		t.Fatal(err)
	}
	items := result["items"].([]any)
	if len(items) != 2 {
		t.Fatalf("folders should be excluded from Top-N: %#v", items)
	}
	first := items[0].(map[string]any)
	if first["nodeId"] != "nNew" {
		t.Fatalf("newest first = %#v", first)
	}
	// 单层输出与普通 list 对齐：不带 BFS 装饰字段。
	for _, key := range []string{"depth", "parentId", "rel_path", "sortTime"} {
		if _, ok := first[key]; ok {
			t.Fatalf("%s leaked into single-layer output: %#v", key, first)
		}
	}
}

func TestCrossPlatformCoverageDocLatestMultiLayerKeepsDecorations(t *testing.T) {
	caller := &scriptedToolCaller{steps: []scriptedToolStep{
		{text: `{"nodes":[{"nodeId":"nDir","name":"folder","nodeType":"folder","updateTime":1700000009000}],"hasMore":false}`},
		{text: `{"nodes":[{"nodeId":"nInner","name":"inner.doc","nodeType":"doc","updateTime":1700000005000}],"hasMore":false}`},
	}}
	result, err := runDepthBFSLatest(t, caller, newDocDepthRoute(), "root1", 2, "", 3)
	if err != nil {
		t.Fatal(err)
	}
	items := result["items"].([]any)
	if len(items) != 1 {
		t.Fatalf("items = %#v", items)
	}
	// depth>1 时保留 rel_path，用户需要知道命中文件在哪一层。
	if items[0].(map[string]any)["rel_path"] != "folder/inner.doc" {
		t.Fatalf("rel_path = %#v", items[0])
	}
}

func TestCrossPlatformCoverageRunDriveListLatestDryRun(t *testing.T) {
	out := installDepthCaller(t, &scriptedToolCaller{format: "json", dry: true})
	cmd := driveLatestTestCmd()
	if err := runDriveListLatest(cmd, map[string]any{"spaceId": "s1"}, "folder1", 3, "", true); err != nil {
		t.Fatal(err)
	}
	var payload map[string]any
	if err := json.Unmarshal(out.Bytes(), &payload); err != nil {
		t.Fatalf("dry-run output: %v\n%s", err, out.String())
	}
	if payload["tool"] != "list_files" || payload["latest"] != float64(3) {
		t.Fatalf("payload = %#v", payload)
	}
	args := payload["args"].(map[string]any)
	if args["orderBy"] != "modifyTime" || args["order"] != "desc" ||
		args["spaceId"] != "s1" || args["parentId"] != "folder1" {
		t.Fatalf("dry-run args = %#v", args)
	}
}

func TestCrossPlatformCoverageRunDriveListLatestPanSingleLayer(t *testing.T) {
	useDriveLatestArgs(t)
	caller := &scriptedToolCaller{steps: []scriptedToolStep{
		{text: `{"items":[
			{"fileId":"fDir","name":"日报归档","type":"FOLDER","modifyTime":1700000009000},
			{"fileId":"f1","name":"日报-0801.docx","type":"FILE","modifyTime":1700000005000},
			{"fileId":"f2","name":"周报-0801.docx","type":"FILE","modifyTime":1700000004000},
			{"fileId":"f3","name":"日报-0731.docx","type":"FILE","modifyTime":1700000003000}],"nextToken":"more"}`},
		{text: `{"items":[{"fileId":"f4","name":"日报-0730.docx","type":"FILE","modifyTime":1700000002000}]}`},
	}}
	out := installDepthCaller(t, caller)
	cmd := driveLatestTestCmd()
	if err := runDriveListLatest(cmd, map[string]any{}, "", 2, "*日报*", true); err != nil {
		t.Fatal(err)
	}
	// 服务端已按 modifyTime desc 返回，首页凑够 2 条即停，不该翻第二页。
	if caller.calls != 1 {
		t.Fatalf("calls = %d, want 1", caller.calls)
	}
	items := decodeDepthResult(t, out)["items"].([]any)
	if len(items) != 2 {
		t.Fatalf("items = %#v", items)
	}
	if items[0].(map[string]any)["fileId"] != "f1" || items[1].(map[string]any)["fileId"] != "f3" {
		t.Fatalf("folder/pattern filtering failed: %#v", items)
	}
}

// 上游对文件名字段有 name / fileName 两种形态，pattern 匹配必须两种都认。
func TestCrossPlatformCoverageRunDriveListLatestFallsBackToFileName(t *testing.T) {
	useDriveLatestArgs(t)
	caller := &scriptedToolCaller{steps: []scriptedToolStep{
		{text: `{"items":[
			{"fileId":"f1","fileName":"日报-0801.docx","type":"FILE","modifyTime":1700000005000},
			{"fileId":"f2","fileName":"周报-0801.docx","type":"FILE","modifyTime":1700000004000}]}`},
	}}
	out := installDepthCaller(t, caller)
	cmd := driveLatestTestCmd()
	if err := runDriveListLatest(cmd, map[string]any{}, "", 5, "*日报*", true); err != nil {
		t.Fatal(err)
	}
	items := decodeDepthResult(t, out)["items"].([]any)
	// 只命中 f1 才说明 pattern 匹配用的是 fileName 回落值，而非空串。
	if len(items) != 1 || items[0].(map[string]any)["fileId"] != "f1" {
		t.Fatalf("fileName fallback not used for pattern match: %#v", items)
	}
}

func TestCrossPlatformCoverageRunDriveListLatestPagesUntilFilled(t *testing.T) {
	useDriveLatestArgs(t)
	caller := &scriptedToolCaller{steps: []scriptedToolStep{
		{text: `{"items":[{"fileId":"fDir","name":"dir","type":"FOLDER"}],"nextToken":"p2"}`},
		{text: `{"items":[{"fileId":"f1","name":"a.txt","type":"FILE","modifyTime":1700000001000}]}`},
	}}
	out := installDepthCaller(t, caller)
	cmd := driveLatestTestCmd()
	// quiet=false 覆盖翻页进度输出分支。
	stderr := captureDriveLatestStderr(t, func() {
		if err := runDriveListLatest(cmd, map[string]any{}, "", 1, "", false); err != nil {
			t.Fatal(err)
		}
	})
	if caller.calls != 2 {
		t.Fatalf("calls = %d, want 2", caller.calls)
	}
	if !strings.Contains(stderr, "latest 扫描中") {
		t.Fatalf("progress line missing: %q", stderr)
	}
	if items := decodeDepthResult(t, out)["items"].([]any); len(items) != 1 {
		t.Fatalf("items = %#v", items)
	}
}

func TestCrossPlatformCoverageRunDriveListLatestShortfall(t *testing.T) {
	useDriveLatestArgs(t)
	caller := &scriptedToolCaller{steps: []scriptedToolStep{
		{text: `{"items":[{"fileId":"f1","name":"a.txt","type":"FILE","modifyTime":1700000001000}]}`},
	}}
	out := installDepthCaller(t, caller)
	cmd := driveLatestTestCmd()
	var runErr error
	stderr := captureDriveLatestStderr(t, func() {
		runErr = runDriveListLatest(cmd, map[string]any{}, "", 5, "*.txt", true)
	})
	// 凑不满 N 不是失败：照常输出已找到的部分，提示走 stderr。
	if runErr != nil {
		t.Fatalf("shortfall returned error: %v", runErr)
	}
	if !strings.Contains(stderr, "找到 1/5 条") || !strings.Contains(stderr, `--pattern "*.txt"`) {
		t.Fatalf("shortfall hint = %q", stderr)
	}
	if items := decodeDepthResult(t, out)["items"].([]any); len(items) != 1 {
		t.Fatalf("items = %#v", items)
	}
}

func TestCrossPlatformCoverageRunDriveListLatestStopsAtScanCap(t *testing.T) {
	useDriveLatestArgs(t)
	// 每页 50 条全是文件夹：永远凑不够，靠 driveLatestScanMax 兜底停机。
	folders := make([]string, 0, driveDepthPageSize)
	for i := 0; i < driveDepthPageSize; i++ {
		folders = append(folders, fmt.Sprintf(`{"fileId":"d%d","name":"d%d","type":"FOLDER"}`, i, i))
	}
	caller := &scriptedToolCaller{steps: []scriptedToolStep{
		{text: fmt.Sprintf(`{"items":[%s],"nextToken":"loop"}`, strings.Join(folders, ","))},
	}}
	out := installDepthCaller(t, caller)
	cmd := driveLatestTestCmd()
	stderr := captureDriveLatestStderr(t, func() {
		if err := runDriveListLatest(cmd, map[string]any{}, "", 3, "", true); err != nil {
			t.Fatal(err)
		}
	})
	if caller.calls != driveLatestScanMax/driveDepthPageSize {
		t.Fatalf("calls = %d, want %d", caller.calls, driveLatestScanMax/driveDepthPageSize)
	}
	if !strings.Contains(stderr, fmt.Sprintf("已扫描 %d 条", driveLatestScanMax)) {
		t.Fatalf("scan cap hint = %q", stderr)
	}
	if items := decodeDepthResult(t, out)["items"].([]any); len(items) != 0 {
		t.Fatalf("items = %#v", items)
	}
}

func TestCrossPlatformCoverageRunDriveListLatestFetchError(t *testing.T) {
	useDriveLatestArgs(t)
	caller := &scriptedToolCaller{steps: []scriptedToolStep{
		{text: `{"errorCode":"forbidden.noPermission","errorMsg":"denied"}`},
	}}
	installDepthCaller(t, caller)
	cmd := driveLatestTestCmd()
	err := runDriveListLatest(cmd, map[string]any{}, "", 3, "", true)
	if err == nil {
		t.Fatal("fetch error returned nil")
	}
	if !strings.Contains(err.Error(), "latest 扫描第 1 页失败") {
		t.Fatalf("error = %v", err)
	}
}
