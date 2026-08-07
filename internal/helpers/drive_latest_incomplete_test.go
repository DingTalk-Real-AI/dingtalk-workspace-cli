package helpers

import (
	"errors"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

// 本文件锁定 drive list --latest 的两个 P1 行为，独立于 pr868_*_test.go / drive_depth_test.go：
//   P1-a：sortTime 是内部排序字段，任何输出路径都不得泄露进契约；
//   P1-b：递归途中目录读取失败时，Top-N 建立在不完整集合上，必须拒绝产出而非吐 partial。
// 改代码前这些断言对 origin/main 应为红：main 采集端无条件写 sortTime、emit 仅在单层 latest 才剥；
// main 尾部拒绝 guard 只拦截断、不拦目录失败。

// depthItemsWithSortTime 断言 stdout 每个 item 都不含内部排序字段 sortTime。
func assertNoSortTime(t *testing.T, result map[string]any) {
	t.Helper()
	items, ok := result["items"].([]any)
	if !ok {
		t.Fatalf("items missing or wrong type: %#v", result["items"])
	}
	for i, raw := range items {
		item, ok := raw.(map[string]any)
		if !ok {
			t.Fatalf("item[%d] not an object: %#v", i, raw)
		}
		if _, leaked := item["sortTime"]; leaked {
			t.Fatalf("item[%d] leaked internal sortTime into output contract: %#v", i, item)
		}
	}
}

// TestDriveLatestNoSortTimeLeak 覆盖 main 的覆盖漏洞：main 的
// TestCrossPlatformCoverageDriveDepthLatestTruncatedAndSortTime 名字带 SortTime，
// 却只断言 TRUNCATED、从不检查输出无 sortTime（`_ = out`）。这里把两条泄露路径都钉住。
func TestDriveLatestNoSortTimeLeak(t *testing.T) {
	// 场景 A：depth>1 --latest —— 走 applyDriveListLatest（只读 sortTime、不剥），reqDepth>1 不触发 strip。
	t.Run("depth_latest", func(t *testing.T) {
		useDriveDepthArgs(t)
		caller := &scriptedToolCaller{steps: []scriptedToolStep{
			{text: `{"items":[{"fileId":"f1","name":"a.txt","type":"FILE","modifiedTime":1000},{"fileId":"f2","name":"b.txt","type":"FILE","modifiedTime":2000}]}`},
		}}
		out := installDepthCaller(t, caller)
		cmd := &cobra.Command{Use: "list"}
		if err := runDriveListDepth(cmd, newDrivePanDepthRoute(), map[string]any{}, "", 3, "", true, 5); err != nil {
			t.Fatalf("runDriveListDepth: %v", err)
		}
		assertNoSortTime(t, decodeDepthResult(t, out))
	})

	// 场景 B：--depth 2 无 latest —— 走 else 分支树序排序，同样不触发 strip。
	t.Run("depth_no_latest", func(t *testing.T) {
		useDriveDepthArgs(t)
		caller := &scriptedToolCaller{steps: []scriptedToolStep{
			{text: `{"items":[{"fileId":"f1","name":"a.txt","type":"FILE","modifiedTime":1000},{"fileId":"f2","name":"c.txt","type":"FILE","modifiedTime":3000}]}`},
		}}
		out := installDepthCaller(t, caller)
		cmd := &cobra.Command{Use: "list"}
		if err := runDriveListDepth(cmd, newDrivePanDepthRoute(), map[string]any{}, "", 2, "", true, 0); err != nil {
			t.Fatalf("runDriveListDepth: %v", err)
		}
		assertNoSortTime(t, decodeDepthResult(t, out))
	})
}

// TestDriveLatestRefusesOnFolderFailure 覆盖 P1-b：递归途中一个可恢复目录失败（403/business，
// 非 auth 非限流 → 记 errs[] 跳过），Top-N 落在不完整集合上，必须拒绝产出。
// 构造：根目录成功产出 FOLDER+FILE（collected>0 且 dirA 入队），子目录返回 forbidden.* →
// recoverable → errs=[1]。旧代码尾部 `if truncated && latest>0` 不触发 → emit 吐 partial（err=nil）；
// 新代码 `len(errs)>0` → LATEST_SCAN_INCOMPLETE 且 stdout 无 items。
func TestDriveLatestRefusesOnFolderFailure(t *testing.T) {
	useDriveDepthArgs(t)
	caller := &scriptedToolCaller{steps: []scriptedToolStep{
		{text: `{"items":[{"fileId":"dirA","name":"dirA","type":"FOLDER"},{"fileId":"fX","name":"x.txt","type":"FILE","modifiedTime":1000}]}`},
		{text: `{"errorCode":"forbidden.noPermission","errorMsg":"denied"}`},
	}}
	out := installDepthCaller(t, caller)
	cmd := &cobra.Command{Use: "list"}
	err := runDriveListDepth(cmd, newDrivePanDepthRoute(), map[string]any{}, "", 3, "", true, 5)
	if err == nil || !strings.Contains(err.Error(), "LATEST_SCAN_INCOMPLETE") {
		t.Fatalf("err = %v, want LATEST_SCAN_INCOMPLETE", err)
	}
	// 拒绝产出：stdout 必须没有 items（不是 partial）。旧代码此处会吐 partial，断言随之失败。
	if out.Len() != 0 {
		t.Fatalf("expected no stdout on refusal, got: %s", out.String())
	}
}

// TestDriveLatestRefusesOnUnrecoverableFailure 覆盖 P1-b 的另一半：递归途中遇不可恢复错误
// （auth 过期 / 网络不可达）且 latest>0 时，不吐 partial，直接回根因错误。
// 与 latest=0 的既有行为（TestCrossPlatformCoverageRunDriveListDepthUnrecoverable：partial
// + errors[] 进 stdout 后非零退出）对照——latest 下 partial 的 Top-N 会被误读为全局最新，
// 故必须拒绝产出；回根因错误而非 INCOMPLETE token，因为 auth/网络比通用截断提示更可操作。
func TestDriveLatestRefusesOnUnrecoverableFailure(t *testing.T) {
	useDriveDepthArgs(t)
	caller := &scriptedToolCaller{steps: []scriptedToolStep{
		{text: `{"items":[{"fileId":"dirA","name":"dirA","type":"FOLDER"},{"fileId":"fX","name":"x.txt","type":"FILE","modifiedTime":1000}]}`},
		{text: `{"errorCode":"DWS_SERVICE_UNAUTHORIZED"}`},
	}}
	out := installDepthCaller(t, caller)
	cmd := &cobra.Command{Use: "list"}
	err := runDriveListDepth(cmd, newDrivePanDepthRoute(), map[string]any{}, "", 3, "", true, 5)
	// 回根因错误：Code 仍是 auth 过期，不被包装成 LATEST_SCAN_INCOMPLETE。
	var cliErr *CLIError
	if !errors.As(err, &cliErr) || cliErr.Code != CodeAuthTokenExpired {
		t.Fatalf("err = %T %v, want CodeAuthTokenExpired", err, err)
	}
	if strings.Contains(cliErr.Message, "LATEST_SCAN_INCOMPLETE") {
		t.Fatalf("unrecoverable 应回根因错误而非 INCOMPLETE 包装: %q", cliErr.Message)
	}
	// 拒绝产出：不吐 partial（对照 latest=0 时会输出 2 条 items + 1 条 errors）。
	if out.Len() != 0 {
		t.Fatalf("expected no stdout on refusal, got: %s", out.String())
	}
}

// TestDriveLatestIncompleteErrorBranches 直接单测构造器的两条分支。
func TestDriveLatestIncompleteErrorBranches(t *testing.T) {
	// 截断分支：truncated=true → TRUNCATED，与 errs 是否为空无关。
	t.Run("truncated", func(t *testing.T) {
		err := driveLatestIncompleteError(5, true, nil)
		var cliErr *CLIError
		if !errors.As(err, &cliErr) || cliErr.Code != CodeContentTruncated {
			t.Fatalf("err = %T %v, want CodeContentTruncated", err, err)
		}
		if !strings.Contains(cliErr.Message, "LATEST_SCAN_TRUNCATED") {
			t.Fatalf("message = %q", cliErr.Message)
		}
		assertDriveLatestSuggestion(t, cliErr.Suggestion)
	})

	// 目录失败分支：errs 非空且未截断 → INCOMPLETE，Message 含首个失败的 folder/depth/reason。
	t.Run("incomplete", func(t *testing.T) {
		errs := []driveDepthError{
			{Depth: 2, FolderID: "fid-9", FolderName: "报表", Reason: "permission_denied", Message: "denied"},
			{Depth: 1, FolderID: "fid-3", FolderName: "归档", Reason: "api_error", Message: "boom"},
		}
		err := driveLatestIncompleteError(3, false, errs)
		var cliErr *CLIError
		if !errors.As(err, &cliErr) || cliErr.Code != CodeContentTruncated {
			t.Fatalf("err = %T %v, want CodeContentTruncated", err, err)
		}
		msg := cliErr.Message
		if !strings.Contains(msg, "LATEST_SCAN_INCOMPLETE") ||
			!strings.Contains(msg, "报表") ||
			!strings.Contains(msg, "depth=2") ||
			!strings.Contains(msg, "permission_denied") ||
			!strings.Contains(msg, "2 个目录未读全") {
			t.Fatalf("message = %q", msg)
		}
		assertDriveLatestSuggestion(t, cliErr.Suggestion)
	})

	// folderName 为空回落 folderID；folderID 也空回落 <root>。
	t.Run("folder_fallback", func(t *testing.T) {
		byID := driveLatestIncompleteError(1, false, []driveDepthError{{FolderID: "fid-x"}})
		if !strings.Contains(byID.Error(), "folder=fid-x") {
			t.Fatalf("fallback to folderID: %v", byID)
		}
		byRoot := driveLatestIncompleteError(1, false, []driveDepthError{{}})
		if !strings.Contains(byRoot.Error(), "folder=<root>") {
			t.Fatalf("fallback to <root>: %v", byRoot)
		}
	})
}

// assertDriveLatestSuggestion 钉住 Suggestion 每子句自洽：含 --latest 的引导子句存在，
// 且「去掉 --latest」子句给出的示例命令本身不带 --latest（否则照抄会复现同一错误）。
func assertDriveLatestSuggestion(t *testing.T, suggestion string) {
	t.Helper()
	clauses := strings.Split(suggestion, "；")
	sawLatestGuide := false
	for _, clause := range clauses {
		cmd := extractTrailingDwsCommand(clause)
		if cmd == "" {
			continue
		}
		if strings.Contains(clause, "去掉 --latest") {
			if strings.Contains(cmd, "--latest") {
				t.Fatalf("「去掉 --latest」子句的示例命令仍含 --latest: %q", cmd)
			}
			continue
		}
		if strings.Contains(cmd, "--latest") {
			sawLatestGuide = true
		}
	}
	if !sawLatestGuide {
		t.Fatalf("no --latest-bearing guidance clause in suggestion: %q", suggestion)
	}
}

// extractTrailingDwsCommand 抽子句里以 "dws " 开头的尾部命令片段（到子句末），无则空串。
func extractTrailingDwsCommand(clause string) string {
	idx := strings.LastIndex(clause, "dws ")
	if idx < 0 {
		return ""
	}
	return clause[idx:]
}
