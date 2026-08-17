package helpers

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

// 本文件锁定 drive list --latest 的两个 P1 行为，独立于 pr868_*_test.go / drive_depth_test.go：
//
//	P1-a：sortTime 是内部排序字段，任何输出路径都不得泄露进契约；
//	P1-b：递归途中目录读取失败时，Top-N 建立在不完整集合上，必须拒绝产出而非吐 partial。
//
// 改代码前这些断言对 origin/main 应为红：main 采集端无条件写 sortTime、emit 仅在单层
// latest/filter 才剥；main 尾部拒绝 guard 只拦截断、不拦目录失败。
//
// 另锁定评审反馈的三个边界：
//
//	截断与目录失败同真时，失败详情（permission_denied 等）不得被截断提示吞掉；
//	拒绝产出时给的恢复命令必须保留原查询域（--workspace / --space-id），照抄不会切换查询域；
//	恢复命令里的用户值必须过 argv 引用，URL 查询串与 shell 元字符都不能改变命令解析。
//
// 关于 TestCrossPlatformCoverage 前缀：这是门禁约定，不是命名风格。平台覆盖率门禁
// scripts/policy/run-platform-coverage-gate.sh 只跑
// -run '^(TestAllShortcuts|TestCrossPlatformCoverage)'，本包新增的生产代码若没有带该前缀的
// 测试覆盖，Coverage (macOS) / (Windows) 会以「changed code coverage 低于 100%」失败
// （本 PR 改名前实测 69.0476%，84 条可执行语句）。改名务必保留前缀。

// assertNoSortTime 断言 stdout 每个 item 都不含内部排序字段 sortTime。
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

// TestCrossPlatformCoverageDriveLatestNoSortTimeLeak 覆盖 main 的覆盖漏洞：main 的
// TestCrossPlatformCoverageDriveDepthLatestTruncatedAndSortTime 名字带 SortTime，
// 却只断言 TRUNCATED、从不检查输出无 sortTime。这里把三条泄露路径都钉住。
func TestCrossPlatformCoverageDriveLatestNoSortTimeLeak(t *testing.T) {
	twoFiles := `{"items":[{"fileId":"f1","name":"a.txt","type":"FILE","modifiedTime":1000},{"fileId":"f2","name":"b.txt","type":"FILE","modifiedTime":2000}]}`

	// 场景 A：depth>1 --latest —— 走 applyDriveListLatest（只读 sortTime、不剥），reqDepth>1 不触发 strip。
	t.Run("depth_latest", func(t *testing.T) {
		useDriveDepthArgs(t)
		caller := &scriptedToolCaller{steps: []scriptedToolStep{{text: twoFiles}}}
		out := installDepthCaller(t, caller)
		cmd := &cobra.Command{Use: "list"}
		if err := runDriveListDepth(cmd, newDrivePanDepthRoute(), map[string]any{}, "", 3, "", true, 5, driveListFilter{}); err != nil {
			t.Fatalf("runDriveListDepth: %v", err)
		}
		assertNoSortTime(t, decodeDepthResult(t, out))
	})

	// 场景 B：--depth 2 无 latest 无 filter —— 走 else 分支树序排序，同样不触发 strip。
	t.Run("depth_no_latest", func(t *testing.T) {
		useDriveDepthArgs(t)
		caller := &scriptedToolCaller{steps: []scriptedToolStep{{text: twoFiles}}}
		out := installDepthCaller(t, caller)
		cmd := &cobra.Command{Use: "list"}
		if err := runDriveListDepth(cmd, newDrivePanDepthRoute(), map[string]any{}, "", 2, "", true, 0, driveListFilter{}); err != nil {
			t.Fatalf("runDriveListDepth: %v", err)
		}
		assertNoSortTime(t, decodeDepthResult(t, out))
	})

	// 场景 C：--depth 2 --type file —— #971 引入的 filter 也读 sortTime（applyDriveListFilter），
	// strip 条件 (latest>0||filter.active()) && reqDepth==1 在多层下依旧不成立，泄露面因此变大。
	t.Run("depth_filter_no_latest", func(t *testing.T) {
		useDriveDepthArgs(t)
		caller := &scriptedToolCaller{steps: []scriptedToolStep{{text: twoFiles}}}
		out := installDepthCaller(t, caller)
		cmd := &cobra.Command{Use: "list"}
		if err := runDriveListDepth(cmd, newDrivePanDepthRoute(), map[string]any{}, "", 2, "", true, 0, driveListFilter{nodeType: "file"}); err != nil {
			t.Fatalf("runDriveListDepth: %v", err)
		}
		result := decodeDepthResult(t, out)
		if len(result["items"].([]any)) != 2 {
			t.Fatalf("filter 应保留两个 FILE: %#v", result["items"])
		}
		assertNoSortTime(t, result)
	})
}

// TestCrossPlatformCoverageDriveLatestSigintStillEmitsPartialWithoutSortTime 同时承担两件事：
//  1. P1-a 的第四条路径 —— SIGINT 取消也走 emitDriveDepthResult，同样不得泄露 sortTime；
//  2. SIGINT 契约锁 —— 本次 P1-b 只在 BFS 尾部 guard 与 unrecoverable 分支生效，**不改**
//     取消路径：SIGINT 是用户主动中断、退出码 130 已明确告知不完整，partial 是明确预期。
//     这条测试防止后续误把 fail-closed 扩到取消路径，也为外部评测用例
//     test_sigint_exits_130_with_partial 提供本地对照。
func TestCrossPlatformCoverageDriveLatestSigintStillEmitsPartialWithoutSortTime(t *testing.T) {
	caller := &scriptedToolCaller{}
	out := installDepthCaller(t, caller)
	items := []map[string]any{
		{"fileId": "f1", "name": "a.txt", "type": "FILE", "sortTime": int64(2000), "rel_path": "a.txt", "depth": 2},
	}
	errs := []driveDepthError{
		{Depth: 1, FolderID: "fid-1", FolderName: "半路目录", Reason: "permission_denied", Message: "denied"},
	}
	// latest>0 且 reqDepth>1：main 在此不 strip，sortTime 泄露。
	err := emitDriveDepthCancelled(items, errs, "", 5, 3, newDrivePanDepthRoute(), driveListFilter{})
	var cancelErr *driveDepthCancelledError
	if !errors.As(err, &cancelErr) {
		t.Fatalf("err = %T %v, want driveDepthCancelledError", err, err)
	}
	if cancelErr.ExitCode() != 130 {
		t.Fatalf("exit code = %d, want 130", cancelErr.ExitCode())
	}
	result := decodeDepthResult(t, out)
	// partial 契约不变：items 与 errors 都照吐，truncated 标记为真。
	if len(result["items"].([]any)) != 1 {
		t.Fatalf("取消路径应保留 partial items: %#v", result["items"])
	}
	if len(result["errors"].([]any)) != 1 {
		t.Fatalf("取消路径应保留 errors[]: %#v", result["errors"])
	}
	if result["truncated"] != true {
		t.Fatalf("取消结果 truncated = %#v, want true", result["truncated"])
	}
	assertNoSortTime(t, result)
}

// TestCrossPlatformCoverageDriveLatestRefusesOnFolderFailure 覆盖 P1-b：递归途中一个可恢复目录失败（403/business，
// 非 auth 非限流 → 记 errs[] 跳过），Top-N 落在不完整集合上，必须拒绝产出。
// 构造：根目录成功产出 FOLDER+FILE（collected>0 且 dirA 入队），子目录返回 forbidden.* →
// recoverable → errs=[1]。旧代码尾部 `if truncated && latest>0` 不触发 → emit 吐 partial（err=nil）；
// 新代码 `len(errs)>0` → LATEST_SCAN_INCOMPLETE 且 stdout 无 items。
func TestCrossPlatformCoverageDriveLatestRefusesOnFolderFailure(t *testing.T) {
	useDriveDepthArgs(t)
	caller := &scriptedToolCaller{steps: []scriptedToolStep{
		{text: `{"items":[{"fileId":"dirA","name":"dirA","type":"FOLDER"},{"fileId":"fX","name":"x.txt","type":"FILE","modifiedTime":1000}]}`},
		{text: `{"errorCode":"forbidden.noPermission","errorMsg":"denied"}`},
	}}
	out := installDepthCaller(t, caller)
	cmd := &cobra.Command{Use: "list"}
	err := runDriveListDepth(cmd, newDrivePanDepthRoute(), map[string]any{}, "", 3, "", true, 5, driveListFilter{})
	if err == nil || !strings.Contains(err.Error(), "LATEST_SCAN_INCOMPLETE") {
		t.Fatalf("err = %v, want LATEST_SCAN_INCOMPLETE", err)
	}
	// 拒绝产出：stdout 必须没有 items（不是 partial）。旧代码此处会吐 partial，断言随之失败。
	if out.Len() != 0 {
		t.Fatalf("expected no stdout on refusal, got: %s", out.String())
	}
}

// TestCrossPlatformCoverageDriveLatestRefusesOnTruncationWithFolderFailure 端到端证明「截断 + 目录失败」组合确实可达：
// 根目录出 dirA/dirB 两个子目录 → dirA 权限失败记 errs[] → dirB 返回 2000 条触发全局截断，
// 尾部 guard 拿到 truncated=true 且 len(errs)=1。旧实现在此让 truncated 短路，permission_denied
// 详情整块丢失（评审反馈的阻塞点）；现在两个 token 与失败详情都必须在。
func TestCrossPlatformCoverageDriveLatestRefusesOnTruncationWithFolderFailure(t *testing.T) {
	useDriveDepthArgs(t)
	var bulk strings.Builder
	bulk.WriteString(`{"items":[`)
	for i := 0; i < driveDepthMaxItems; i++ {
		if i > 0 {
			bulk.WriteString(",")
		}
		fmt.Fprintf(&bulk, `{"fileId":"f%d","name":"file-%d.txt","type":"FILE","modifiedTime":%d}`, i, i, 1000+i)
	}
	bulk.WriteString(`]}`)
	caller := &scriptedToolCaller{steps: []scriptedToolStep{
		{text: `{"items":[{"fileId":"dirA","name":"报表","type":"FOLDER"},{"fileId":"dirB","name":"dirB","type":"FOLDER"}]}`},
		{text: `{"errorCode":"forbidden.noPermission","errorMsg":"denied"}`}, // dirA：可恢复 → 记 errs[]
		{text: bulk.String()}, // dirB：撞 2000 上限 → truncated
	}}
	out := installDepthCaller(t, caller)
	cmd := &cobra.Command{Use: "list"}
	err := runDriveListDepth(cmd, newDrivePanDepthRoute(), map[string]any{}, "", 3, "", true, 5, driveListFilter{})
	var cliErr *CLIError
	if !errors.As(err, &cliErr) || cliErr.Code != CodeContentTruncated {
		t.Fatalf("err = %T %v, want CodeContentTruncated", err, err)
	}
	msg := cliErr.Message
	if !strings.Contains(msg, "LATEST_SCAN_TRUNCATED") {
		t.Fatalf("组合场景缺 TRUNCATED token: %q", msg)
	}
	// 旧实现在这里丢掉整段目录失败详情。
	if !strings.Contains(msg, "LATEST_SCAN_INCOMPLETE") ||
		!strings.Contains(msg, "folder=报表") ||
		!strings.Contains(msg, "permission_denied") {
		t.Fatalf("组合场景丢失目录失败详情: %q", msg)
	}
	if out.Len() != 0 {
		t.Fatalf("expected no stdout on refusal, got: %s", out.String())
	}
}

// TestCrossPlatformCoverageDriveLatestScopeWiredFromCommand 端到端验证查询域从原命令一路带到恢复命令：单测构造器
// 拿不到的是 runDriveListDepth 里的接线（driveLatestScopeFromCmd(cmd, maxDepth, rootFolderID)），这里补上。
func TestCrossPlatformCoverageDriveLatestScopeWiredFromCommand(t *testing.T) {
	useDriveDepthArgs(t)
	caller := &scriptedToolCaller{steps: []scriptedToolStep{
		{text: `{"items":[{"fileId":"dirA","name":"dirA","type":"FOLDER"},{"fileId":"fX","name":"x.txt","type":"FILE","modifiedTime":1000}]}`},
		{text: `{"errorCode":"forbidden.noPermission","errorMsg":"denied"}`},
	}}
	installDepthCaller(t, caller)
	cmd := newDriveListScopeCmd(t, map[string]string{"space-id": "sp-7"})
	err := runDriveListDepth(cmd, newDrivePanDepthRoute(), map[string]any{"spaceId": "sp-7"}, "", 4, "", true, 5, driveListFilter{})
	var cliErr *CLIError
	if !errors.As(err, &cliErr) {
		t.Fatalf("err = %T %v", err, err)
	}
	// 恢复命令必须带原 --space-id，且给出原 --depth 层数。
	assertDriveLatestSuggestion(t, cliErr.Suggestion, "--space-id sp-7")
	if !strings.Contains(cliErr.Suggestion, "--depth 4") {
		t.Fatalf("恢复命令应保留原层数: %q", cliErr.Suggestion)
	}
}

// TestCrossPlatformCoverageDriveLatestRefusesOnUnrecoverableFailure 覆盖 P1-b 的另一半：递归途中遇不可恢复错误
// （auth 过期 / 网络不可达）且 latest>0 时，不吐 partial，直接回根因错误。
// 与 latest=0 的既有行为（TestCrossPlatformCoverageRunDriveListDepthUnrecoverable：partial
// + errors[] 进 stdout 后非零退出）对照——latest 下 partial 的 Top-N 会被误读为全局最新，
// 故必须拒绝产出；回根因错误而非 INCOMPLETE token，因为 auth/网络比通用截断提示更可操作。
func TestCrossPlatformCoverageDriveLatestRefusesOnUnrecoverableFailure(t *testing.T) {
	useDriveDepthArgs(t)
	caller := &scriptedToolCaller{steps: []scriptedToolStep{
		{text: `{"items":[{"fileId":"dirA","name":"dirA","type":"FOLDER"},{"fileId":"fX","name":"x.txt","type":"FILE","modifiedTime":1000}]}`},
		{text: `{"errorCode":"DWS_SERVICE_UNAUTHORIZED"}`},
	}}
	out := installDepthCaller(t, caller)
	cmd := &cobra.Command{Use: "list"}
	err := runDriveListDepth(cmd, newDrivePanDepthRoute(), map[string]any{}, "", 3, "", true, 5, driveListFilter{})
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

// TestCrossPlatformCoverageDriveLatestIncompleteErrorBranches 直接单测构造器的各条分支。
func TestCrossPlatformCoverageDriveLatestIncompleteErrorBranches(t *testing.T) {
	// 纯截断分支：errs 为空 → 只有 TRUNCATED。
	t.Run("truncated_only", func(t *testing.T) {
		err := driveLatestIncompleteError(5, true, nil, driveLatestScope{depth: 3})
		var cliErr *CLIError
		if !errors.As(err, &cliErr) || cliErr.Code != CodeContentTruncated {
			t.Fatalf("err = %T %v, want CodeContentTruncated", err, err)
		}
		if !strings.Contains(cliErr.Message, "LATEST_SCAN_TRUNCATED") {
			t.Fatalf("message = %q", cliErr.Message)
		}
		if strings.Contains(cliErr.Message, "LATEST_SCAN_INCOMPLETE") {
			t.Fatalf("无目录失败时不应出现 INCOMPLETE: %q", cliErr.Message)
		}
		assertDriveLatestSuggestion(t, cliErr.Suggestion, "")
	})

	// 纯目录失败分支：未截断 → 只有 INCOMPLETE，Message 含首个失败的 folder/depth/reason。
	t.Run("folder_failure_only", func(t *testing.T) {
		err := driveLatestIncompleteError(3, false, twoDriveDepthErrors(), driveLatestScope{depth: 3})
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
		if strings.Contains(msg, "LATEST_SCAN_TRUNCATED") {
			t.Fatalf("未截断时不应出现 TRUNCATED: %q", msg)
		}
		assertDriveLatestSuggestion(t, cliErr.Suggestion, "")
	})

	// 组合分支（评审反馈的阻塞点）：截断与目录失败同真时，旧实现让 truncated 短路、
	// permission_denied 这类目录失败详情整块丢失。现在两个 token 都必须在，且失败详情不得被吞。
	t.Run("truncated_with_folder_failures", func(t *testing.T) {
		err := driveLatestIncompleteError(4, true, twoDriveDepthErrors(), driveLatestScope{depth: 3})
		var cliErr *CLIError
		if !errors.As(err, &cliErr) || cliErr.Code != CodeContentTruncated {
			t.Fatalf("err = %T %v, want CodeContentTruncated", err, err)
		}
		msg := cliErr.Message
		// 两个成因都要可被消费方 token 匹配到。
		if !strings.Contains(msg, "LATEST_SCAN_INCOMPLETE") || !strings.Contains(msg, "LATEST_SCAN_TRUNCATED") {
			t.Fatalf("组合场景需同时带两个 token: %q", msg)
		}
		// 目录失败详情必须完整保留——这是用户唯一能动手修的线索。
		if !strings.Contains(msg, "报表") ||
			!strings.Contains(msg, "depth=2") ||
			!strings.Contains(msg, "permission_denied") ||
			!strings.Contains(msg, "2 个目录未读全") {
			t.Fatalf("组合场景丢失目录失败详情: %q", msg)
		}
		if !strings.Contains(msg, "拒绝输出不完整的 Top-4") {
			t.Fatalf("message 缺少拒绝产出结论: %q", msg)
		}
		assertDriveLatestSuggestion(t, cliErr.Suggestion, "")
		// 组合场景的指引要同时覆盖两条补救：换可读目录 + 降层数。
		if !strings.Contains(cliErr.Suggestion, "确认目录权限") || !strings.Contains(cliErr.Suggestion, "--depth") {
			t.Fatalf("组合场景 suggestion 需同时给出权限与降层数补救: %q", cliErr.Suggestion)
		}
	})

	// folderName 为空回落 folderID；folderID 也空回落 <root>。
	t.Run("folder_fallback", func(t *testing.T) {
		byID := driveLatestIncompleteError(1, false, []driveDepthError{{FolderID: "fid-x"}}, driveLatestScope{depth: 2})
		if !strings.Contains(byID.Error(), "folder=fid-x") {
			t.Fatalf("fallback to folderID: %v", byID)
		}
		byRoot := driveLatestIncompleteError(1, false, []driveDepthError{{}}, driveLatestScope{depth: 2})
		if !strings.Contains(byRoot.Error(), "folder=<root>") {
			t.Fatalf("fallback to <root>: %v", byRoot)
		}
	})
}

// twoDriveDepthErrors 是两条目录失败样本，首条用于断言「首个失败」详情。
func twoDriveDepthErrors() []driveDepthError {
	return []driveDepthError{
		{Depth: 2, FolderID: "fid-9", FolderName: "报表", Reason: "permission_denied", Message: "denied"},
		{Depth: 1, FolderID: "fid-3", FolderName: "归档", Reason: "api_error", Message: "boom"},
	}
}

// TestCrossPlatformCoverageDriveLatestScopePreservedInSuggestion 覆盖评审反馈的第二个阻塞点：恢复命令此前固定
// 生成 `dws drive list --folder ...`，把原调用的 --workspace / --space-id 丢掉。用户照抄后
// 会从知识库切到普通钉盘（或从指定钉盘空间切到「我的文件」），在另一个查询域里拿到一份
// 「看起来对」的 Top-N —— 比直接报错更难发现。
func TestCrossPlatformCoverageDriveLatestScopePreservedInSuggestion(t *testing.T) {
	cases := []struct {
		name  string
		flags map[string]string
		want  string
	}{
		// 知识库路由：--workspace 决定路由，丢了就切到普通钉盘。
		{name: "workspace", flags: map[string]string{"workspace": "ws-1"}, want: "--workspace ws-1"},
		// 别名路径：--workspace-id 与 --workspace 同源（flagOrFallback）。
		{name: "workspace_id_alias", flags: map[string]string{"workspace-id": "ws-alias"}, want: "--workspace ws-alias"},
		// 钉盘路由：--space-id 丢了就退回「我的文件」。
		{name: "space_id", flags: map[string]string{"space-id": "sp-7"}, want: "--space-id sp-7"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			scope := driveLatestScopeFromCmd(newDriveListScopeCmd(t, tc.flags), 3, "")
			// 三条成因组合下的恢复命令都必须带原查询域。
			for _, variant := range []struct {
				label     string
				truncated bool
				errs      []driveDepthError
			}{
				{"truncated_only", true, nil},
				{"folder_failure_only", false, twoDriveDepthErrors()},
				{"both", true, twoDriveDepthErrors()},
			} {
				err := driveLatestIncompleteError(5, variant.truncated, variant.errs, scope)
				var cliErr *CLIError
				if !errors.As(err, &cliErr) {
					t.Fatalf("%s: err = %T %v", variant.label, err, err)
				}
				assertDriveLatestSuggestion(t, cliErr.Suggestion, tc.want)
			}
		})
	}

	// --workspace 优先于 --space-id：与 drive list 的路由判定同序（先看 workspace 再看 space-id），
	// 否则恢复命令会把知识库查询写成钉盘查询。
	t.Run("workspace_wins_over_space_id", func(t *testing.T) {
		scope := driveLatestScopeFromCmd(newDriveListScopeCmd(t, map[string]string{
			"workspace": "ws-1", "space-id": "sp-7",
		}), 3, "")
		err := driveLatestIncompleteError(5, false, twoDriveDepthErrors(), scope)
		suggestion := err.(*CLIError).Suggestion
		if !strings.Contains(suggestion, "--workspace ws-1") || strings.Contains(suggestion, "--space-id") {
			t.Fatalf("workspace 应优先且不混入 space-id: %q", suggestion)
		}
	})

	// 无查询域时不得凭空造 flag（原调用就是「我的文件」根，硬塞 scope 同样是改查询域）。
	t.Run("no_scope_adds_nothing", func(t *testing.T) {
		scope := driveLatestScopeFromCmd(newDriveListScopeCmd(t, nil), 3, "")
		err := driveLatestIncompleteError(5, false, twoDriveDepthErrors(), scope)
		suggestion := err.(*CLIError).Suggestion
		if strings.Contains(suggestion, "--workspace") || strings.Contains(suggestion, "--space-id") {
			t.Fatalf("无 scope 时不应凭空造查询域 flag: %q", suggestion)
		}
		assertDriveLatestSuggestion(t, suggestion, "")
	})

	// depth==1（知识库 --latest 单层）时不给 --depth 1：partial+errors[] 契约只在多层成立，
	// 硬塞 --depth 1 会让「去掉 --latest 看明细」的子句自相矛盾。
	t.Run("single_depth_omits_depth_flag", func(t *testing.T) {
		err := driveLatestIncompleteError(5, false, twoDriveDepthErrors(), driveLatestScope{domain: "--workspace ws-1", depth: 1})
		suggestion := err.(*CLIError).Suggestion
		if strings.Contains(suggestion, "--depth") {
			t.Fatalf("单层不应出现 --depth: %q", suggestion)
		}
	})

	// 多层时给出确切层数，用户无需把 <原层数> 换成数字。
	t.Run("multi_depth_emits_actual_depth", func(t *testing.T) {
		err := driveLatestIncompleteError(5, false, twoDriveDepthErrors(), driveLatestScope{domain: "--space-id sp-7", depth: 4})
		suggestion := err.(*CLIError).Suggestion
		if !strings.Contains(suggestion, "--depth 4") {
			t.Fatalf("应给出原层数 --depth 4: %q", suggestion)
		}
		if strings.Contains(suggestion, "<原层数>") {
			t.Fatalf("不应残留占位符: %q", suggestion)
		}
	})
}

// TestCrossPlatformCoverageDriveLatestScopeQuotesHostileValues 覆盖评审的 P1 阻断项：恢复命令是给用户直接复制到
// shell 里执行的，查询域的值来自用户输入（workspace 常见形态就是带查询串的 URL）。未引用时
// 一个 `&` 就把命令拆成后台任务，`;` / `$()` 更能执行额外内容。
// 断言落在「值被完整包在单引号里」而不只是「出现过」——后者对裸拼接也成立，抓不住缺陷。
func TestCrossPlatformCoverageDriveLatestScopeQuotesHostileValues(t *testing.T) {
	cases := []struct {
		name  string
		flag  string
		value string
		want  string
	}{
		// 合法 workspace URL：& 在裸拼接下直接改变 shell 解析（前半段被丢进后台）。
		{name: "workspace_url", flag: "workspace", value: "https://alidocs.dingtalk.com/i/nodes/abc?spaceId=1&type=doc",
			want: `--workspace 'https://alidocs.dingtalk.com/i/nodes/abc?spaceId=1&type=doc'`},
		// 命令替换：裸拼接会在用户复制执行时真的跑起来。
		{name: "command_substitution", flag: "workspace", value: "$(id)", want: `--workspace '$(id)'`},
		// 分号拆语句。
		{name: "semicolon", flag: "space-id", value: "sp-7;id", want: `--space-id 'sp-7;id'`},
		// 空格拆参：裸拼接会让 --folder 收到错误的值。
		{name: "space", flag: "space-id", value: "sp 7", want: `--space-id 'sp 7'`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// 显式注入 POSIX 策略：本测试断言的是「危险值被单引号正确包住」这一 POSIX 形态。
			// Windows 策略下这些值根本不内联（见 ...SuggestionNeverInlinesHostileValue），
			// 若走平台绑定，这里在 Windows runner 上会因形态不同而误报。
			scope := driveLatestScopeFrom(newDriveListScopeCmd(t, map[string]string{tc.flag: tc.value}), 3, "", driveLatestPosixScopeValue)
			err := driveLatestIncompleteError(5, true, twoDriveDepthErrors(), scope)
			suggestion := err.(*CLIError).Suggestion
			if !strings.Contains(suggestion, tc.want) {
				t.Fatalf("查询域未安全引用\n want substring: %s\n got suggestion: %s", tc.want, suggestion)
			}
		})
	}
}

// TestCrossPlatformCoverageDriveLatestScopePreservesFiltersAndFolder 覆盖自动 CR 的 P2：
// --pattern/--type/--start/--end 与 --folder 同样决定 --latest 的候选集合。恢复命令丢掉任一项，
// 用户照抄后就是在**另一个集合**上取 Top-N —— 原调用带 --pattern 时，缺了它的重试命令会对全部
// 条目排序，结果「看起来成功」却答非所问，比直接报错更难发现。
func TestCrossPlatformCoverageDriveLatestScopePreservesFiltersAndFolder(t *testing.T) {
	cmd := newDriveListScopeCmd(t, map[string]string{
		"workspace": "ws-1",
		"pattern":   "*日报*",
		"type":      "file",
		"start":     "7d",
		"end":       "2026-08-01",
	})
	// 注入 POSIX 策略：`--pattern '*日报*'` 这种引用形态是 POSIX 专属，Windows 下该值会降级为
	// 占位符 + 展示行（见 ...SuggestionNeverInlinesHostileValue）。走平台绑定会在 Windows 误报。
	scope := driveLatestScopeFrom(cmd, 4, "folder-root", driveLatestPosixScopeValue)
	err := driveLatestIncompleteError(5, false, twoDriveDepthErrors(), scope)
	suggestion := err.(*CLIError).Suggestion

	// 每条示例命令都要带齐查询域与全部过滤条件；pattern 含 * 与中文，必须是引用后的形态。
	for _, want := range []string{
		"--workspace ws-1",
		"--pattern '*日报*'",
		"--type file",
		"--start 7d",
		"--end 2026-08-01",
	} {
		for _, clause := range strings.Split(suggestion, "；") {
			cmdText := extractTrailingDwsCommand(clause)
			if cmdText == "" {
				continue
			}
			if !strings.Contains(cmdText, want) {
				t.Fatalf("恢复命令丢失候选集条件 %q:\n  clause: %s", want, cmdText)
			}
		}
	}

	// 「去掉 --latest 按原范围重跑」是唯一的原范围命令，必须原样带回原 --folder 而非占位符，
	// 否则它就不是「原范围」。
	var origin string
	for _, clause := range strings.Split(suggestion, "；") {
		if strings.Contains(clause, "去掉 --latest") {
			origin = extractTrailingDwsCommand(clause)
		}
	}
	if origin == "" {
		t.Fatalf("多层场景应给出「去掉 --latest 按原范围重跑」子句: %q", suggestion)
	}
	if !strings.Contains(origin, "--folder folder-root") {
		t.Fatalf("原范围命令应保留原 --folder: %q", origin)
	}
	if strings.Contains(origin, "<可读子目录ID>") {
		t.Fatalf("原范围命令不应把原 --folder 换成占位符: %q", origin)
	}
}

// TestCrossPlatformCoverageDriveLatestScopeOmitsFolderAtSpaceRoot 原调用就在空间根时不得凭空
// 造 --folder：硬塞一个目录同样是改候选集。
func TestCrossPlatformCoverageDriveLatestScopeOmitsFolderAtSpaceRoot(t *testing.T) {
	scope := driveLatestScopeFromCmd(newDriveListScopeCmd(t, map[string]string{"space-id": "sp-7"}), 3, "")
	err := driveLatestIncompleteError(2, false, twoDriveDepthErrors(), scope)
	suggestion := err.(*CLIError).Suggestion
	for _, clause := range strings.Split(suggestion, "；") {
		if !strings.Contains(clause, "去掉 --latest") {
			continue
		}
		if strings.Contains(extractTrailingDwsCommand(clause), "--folder") {
			t.Fatalf("空间根扫描的原范围命令不应带 --folder: %q", clause)
		}
	}
}

// TestCrossPlatformCoverageDriveLatestErrorStripsRemoteControlChars 覆盖自动 CR 的 P2 安全项：
// 拒绝产出后，目录名与服务端错误文本从 JSON（编码时会被转义）挪进了纯文本 stderr。若原样透传，
// 其中的 ANSI/OSC 序列会被终端直接执行 —— 可清屏、伪造彩色「成功」、隐藏后续输出、改窗口标题，
// 在 AI Agent 场景还会污染上下文窗口。目录名对共享目录而言是他人可控输入。
func TestCrossPlatformCoverageDriveLatestErrorStripsRemoteControlChars(t *testing.T) {
	assertNoControlChars := func(t *testing.T, msg string) {
		t.Helper()
		for name, r := range map[string]rune{"ESC": 0x1b, "BEL": 0x07, "CR": '\r', "LF": '\n', "TAB": '\t'} {
			if strings.ContainsRune(msg, r) {
				t.Fatalf("错误消息残留 %s 控制字符: %q", name, msg)
			}
		}
	}

	// folderName 路径：CSI 清屏 + 变色，外加 OSC 改标题。
	t.Run("folder_name", func(t *testing.T) {
		err := driveLatestIncompleteError(3, false, []driveDepthError{{
			Depth:      2,
			FolderName: "报表\x1b[2J\x1b[31m看起来成功\x1b[0m",
			Reason:     "permission_denied",
			Message:    "denied\r\n\x1b]0;pwned\x07次行",
		}}, driveLatestScope{depth: 2})
		msg := err.(*CLIError).Message
		assertNoControlChars(t, msg)
		// 清理不能把诊断信息一起抹掉：可读部分必须留下。
		if !strings.Contains(msg, "报表") || !strings.Contains(msg, "denied") || !strings.Contains(msg, "次行") {
			t.Fatalf("清理后应保留可读文本: %q", msg)
		}
	})

	// folderName 为空时回落到 folderID，该字段同样来自服务端。
	t.Run("folder_id_fallback", func(t *testing.T) {
		err := driveLatestIncompleteError(1, true, []driveDepthError{{
			FolderID: "fid\x1b[1m-x",
			Reason:   "api_error",
			Message:  "boom",
		}}, driveLatestScope{depth: 1})
		msg := err.(*CLIError).Message
		assertNoControlChars(t, msg)
		if !strings.Contains(msg, "fid-x") {
			t.Fatalf("剥离控制序列后 folderID 应连成 fid-x: %q", msg)
		}
	})
}

// TestCrossPlatformCoverageDriveLatestSafeRemoteText 直接钉住清理函数的各条分支：换行/制表符
// 折成空格（SanitizeForTerminal 按设计保留这两者，但单行错误消息里它们会拆出伪造行），
// 首尾空白收掉，可打印内容与中文原样保留。
func TestCrossPlatformCoverageDriveLatestSafeRemoteText(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{name: "plain", in: "denied", want: "denied"},
		{name: "cjk", in: "报表目录", want: "报表目录"},
		{name: "csi", in: "a\x1b[31mb", want: "ab"},
		{name: "osc", in: "a\x1b]0;t\x07b", want: "ab"},
		{name: "newline_to_space", in: "a\nb", want: "a b"},
		{name: "tab_to_space", in: "a\tb", want: "a b"},
		{name: "carriage_return_dropped", in: "a\rb", want: "ab"},
		{name: "trim_outer", in: "\n denied \t", want: "denied"},
		{name: "empty", in: "", want: ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := driveLatestSafeRemoteText(tc.in); got != tc.want {
				t.Fatalf("driveLatestSafeRemoteText(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

// TestCrossPlatformCoverageDriveLatestSuggestionNeverInlinesHostileValue 端到端锁定评审提出的
// Windows 注入阻断项：`--space-id sp-7&whoami` 这种值，在 POSIX 下靠单引号关停，但 cmd.exe
// 不把单引号当引号，粘贴后 `&whoami` 仍会执行。因此不变量必须是「恢复命令里不存在能被目标
// shell 解释的裸元字符」，两个平台分别用各自的手段满足：POSIX 引用内联，Windows 不内联。
//
// 两种形态都通过注入渲染策略在本机验证，不依赖当前 GOOS —— 否则「Windows 上降级为占位符」
// 这条路在 POSIX 机器上永不可达，就成了只有 Windows runner 才跑到的盲区（平台覆盖率门禁也会
// 因此报未覆盖）。平台绑定本身由 TestCrossPlatformCoverageDriveLatestScopeValueBinding 覆盖。
func TestCrossPlatformCoverageDriveLatestSuggestionNeverInlinesHostileValue(t *testing.T) {
	const hostile = "sp-7&whoami"
	newSuggestion := func(t *testing.T, render driveLatestValueRenderer) string {
		t.Helper()
		scope := driveLatestScopeFrom(newDriveListScopeCmd(t, map[string]string{"space-id": hostile}), 3, "", render)
		return driveLatestIncompleteError(5, true, twoDriveDepthErrors(), scope).(*CLIError).Suggestion
	}

	t.Run("windows_never_inlines", func(t *testing.T) {
		suggestion := newSuggestion(t, driveLatestWindowsScopeValue)
		for _, clause := range strings.Split(suggestion, "；") {
			cmdText := extractTrailingDwsCommand(clause)
			if cmdText == "" {
				continue
			}
			// cmd.exe 里 & 分隔命令，且单引号不是引号，故它压根不能进命令。
			if strings.Contains(cmdText, "&") {
				t.Fatalf("windows 形态的命令不得含 &: %s", cmdText)
			}
			if strings.Contains(cmdText, hostile) {
				t.Fatalf("windows 形态的命令不得内联原值: %s", cmdText)
			}
		}
		if !strings.Contains(suggestion, driveLatestUnsafeValuePlaceholder) {
			t.Fatalf("应降级为占位符: %s", suggestion)
		}
		if !strings.Contains(suggestion, strconv.Quote(hostile)) {
			t.Fatalf("应在展示行给出原值: %s", suggestion)
		}
		if !strings.Contains(suggestion, "不是可执行命令") {
			t.Fatalf("展示行须显式声明非可执行: %s", suggestion)
		}
	})

	t.Run("posix_quotes_inline", func(t *testing.T) {
		suggestion := newSuggestion(t, driveLatestPosixScopeValue)
		if !strings.Contains(suggestion, "'"+hostile+"'") {
			t.Fatalf("posix 形态应单引号内联原值: %s", suggestion)
		}
		// POSIX 下不该无谓降级 —— 那会白白损失可复制体验。
		if strings.Contains(suggestion, driveLatestUnsafeValuePlaceholder) {
			t.Fatalf("posix 形态不应降级为占位符: %s", suggestion)
		}
		for _, clause := range strings.Split(suggestion, "；") {
			cmdText := extractTrailingDwsCommand(clause)
			if cmdText == "" {
				continue
			}
			if strings.Contains(cmdText, "&") && !strings.Contains(cmdText, "'"+hostile+"'") {
				t.Fatalf("posix 形态出现未引用的元字符: %s", cmdText)
			}
		}
	})
}

// newDriveListScopeCmd 造一个带 drive list 查询域与过滤 flag 的命令。flag 名与 newDriveCommand
// 里 driveListCmd 的注册保持一致；workspace-id 是 cross-product 别名，此处显式注册以覆盖别名路径。
func newDriveListScopeCmd(t *testing.T, flags map[string]string) *cobra.Command {
	t.Helper()
	cmd := &cobra.Command{Use: "list"}
	for _, name := range []string{
		"workspace", "workspace-id", "space-id", // 查询域
		"folder",                          // 扫描根（scope 从 rootFolder 参数取，注册仅为对齐真实命令）
		"pattern", "type", "start", "end", // 决定候选集的过滤条件
	} {
		cmd.Flags().String(name, "", "")
	}
	for name, value := range flags {
		if err := cmd.Flags().Set(name, value); err != nil {
			t.Fatalf("set --%s=%s: %v", name, value, err)
		}
	}
	return cmd
}

// assertDriveLatestSuggestion 钉住 Suggestion 的三条约束：
//  1. 每条示例命令都带原查询域 wantScope（空串表示原调用无查询域，此时只跳过该项检查）；
//  2. 含 --latest 的引导子句存在；
//  3. 「去掉 --latest」子句给出的示例命令本身不带 --latest（否则照抄复现同一错误）。
func assertDriveLatestSuggestion(t *testing.T, suggestion, wantScope string) {
	t.Helper()
	clauses := strings.Split(suggestion, "；")
	sawLatestGuide := false
	for _, clause := range clauses {
		cmd := extractTrailingDwsCommand(clause)
		if cmd == "" {
			continue
		}
		if wantScope != "" && !strings.Contains(cmd, wantScope) {
			t.Fatalf("示例命令丢失原查询域 %q（照抄会切换查询域）: %q", wantScope, cmd)
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
