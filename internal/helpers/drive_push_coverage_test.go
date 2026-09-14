package helpers

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
)

var errTestUpload = errors.New("simulated upload failure")

// pushOKCaller 是 push 的“一切正常”mock：list_files 按 parentId 返回远端现状，
// create_folder 返回新 fileId，get_upload_info 返回可解析凭证，ln 提交成功。
func pushOKCaller(listing map[string]string) *driveScriptCaller {
	return &driveScriptCaller{reply: func(tool string, args map[string]any, _ int) (string, error) {
		switch tool {
		case "list_files":
			parent, _ := args["parentId"].(string)
			if body, ok := listing[parent]; ok {
				return body, nil
			}
			return `{"result":{"items":[],"nextToken":""}}`, nil
		case "create_folder":
			return `{"result":{"fileId":"NEWDIR"},"success":true}`, nil
		case "get_upload_info":
			return `{"result":{"resourceUrls":[{"url":"https://oss.example.com/put","headers":{}}],"uploadId":"U1"}}`, nil
		}
		return `{"result":{"fileId":"NEWFILE"},"success":true}`, nil
	}}
}

func withNoopPut(t *testing.T) {
	t.Helper()
	testseam.Swap(t, &pushPutOpenedFile, func(context.Context, string, map[string]string, *os.File, int64) error { return nil })
}

// ──────────────────────────────────────────────────────────
// runDrivePush — 端到端
// ──────────────────────────────────────────────────────────

// 本地子目录在远端缺失 → 按需 create_folder 并留 folder_created 痕迹，再上传其中文件。
func TestCrossPlatformCoverageDrivePush_createsMissingRemoteFolders(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, filepath.Join(dir, "sub", "deep.txt"), "d")
	withNoopPut(t)

	caller := pushOKCaller(map[string]string{"ROOT": `{"result":{"items":[],"nextToken":""}}`})
	if err := runDriveCmd(t, caller, "push", "--local-folder", dir, "--remote-folder", "ROOT"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	cf := caller.callsFor("create_folder")
	if len(cf) != 1 || cf[0].args["name"] != "sub" {
		t.Fatalf("expected create_folder(sub), got %v", cf)
	}
	if cf[0].args["parentId"] != "ROOT" {
		t.Errorf("create_folder parentId = %v, want ROOT", cf[0].args["parentId"])
	}
	// 新建目录的 fileId 必须成为其中文件的 parentId。
	up := caller.callsFor("get_upload_info")
	if len(up) != 1 || up[0].args["parentId"] != "NEWDIR" {
		t.Errorf("upload parentId = %v, want NEWDIR", up[0].args["parentId"])
	}
}

// 远端已存在同名目录 → 复用其 fileId，不重建、不出现在 items[]。
func TestCrossPlatformCoverageDrivePush_reusesExistingRemoteFolder(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, filepath.Join(dir, "sub", "deep.txt"), "d")
	withNoopPut(t)

	caller := pushOKCaller(map[string]string{
		"ROOT": `{"result":{"items":[{"name":"sub","type":"folder","fileId":"EXISTING"}],"nextToken":""}}`,
	})
	if err := runDriveCmd(t, caller, "push", "--local-folder", dir, "--remote-folder", "ROOT"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := caller.callsFor("create_folder"); len(got) != 0 {
		t.Errorf("existing remote folder must not be recreated, got %v", got)
	}
	if up := caller.callsFor("get_upload_info"); up[0].args["parentId"] != "EXISTING" {
		t.Errorf("upload parentId = %v, want EXISTING", up[0].args["parentId"])
	}
}

// create_folder 失败 / 未返回 fileId → 该目录记 failed，其中文件因父目录缺失也 failed。
func TestCrossPlatformCoverageDrivePush_folderCreationFailureCascades(t *testing.T) {
	for _, tc := range []struct {
		name  string
		reply func(string, map[string]any, int) (string, error)
	}{
		{"tool error", func(tool string, _ map[string]any, _ int) (string, error) {
			if tool == "create_folder" {
				return "", errors.New("create_folder boom")
			}
			return `{"result":{"items":[],"nextToken":""}}`, nil
		}},
		{"no fileId", func(tool string, _ map[string]any, _ int) (string, error) {
			if tool == "create_folder" {
				return `{"result":{},"success":true}`, nil
			}
			return `{"result":{"items":[],"nextToken":""}}`, nil
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			if err := os.MkdirAll(filepath.Join(dir, "sub"), 0o755); err != nil {
				t.Fatal(err)
			}
			mustWrite(t, filepath.Join(dir, "sub", "deep.txt"), "d")
			withNoopPut(t)

			caller := &driveScriptCaller{reply: tc.reply}
			err := runDriveCmd(t, caller, "push", "--local-folder", dir, "--remote-folder", "ROOT")
			var pf *drivePushFailure
			if !errors.As(err, &pf) {
				t.Fatalf("expected drivePushFailure, got %T %v", err, err)
			}
			// 目录 + 其中文件各记一次失败。
			if pf.failed != 2 {
				t.Errorf("failed = %d, want 2 (folder + orphaned file)", pf.failed)
			}
			if got := caller.callsFor("get_upload_info"); len(got) != 0 {
				t.Errorf("no upload may happen without a parent folder, got %v", got)
			}
		})
	}
}

// --if-exists skip（默认）：远端已存在 → 跳过，不上传。
func TestCrossPlatformCoverageDrivePush_ifExistsSkipIsDefault(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "a.txt"), "local")
	withNoopPut(t)

	caller := pushOKCaller(map[string]string{
		"ROOT": `{"result":{"items":[{"name":"a.txt","type":"file","fileId":"A","modifyTime":1}],"nextToken":""}}`,
	})
	if err := runDriveCmd(t, caller, "push", "--local-folder", dir, "--remote-folder", "ROOT"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := caller.callsFor("get_upload_info"); len(got) != 0 {
		t.Errorf("default skip must not upload, got %v", got)
	}
}

// --if-exists smart：远端时间已 ≥ 本地 → 跳过；远端更旧 → 覆盖上传。
func TestCrossPlatformCoverageDrivePush_ifExistsSmart(t *testing.T) {
	t.Run("remote newer skips", func(t *testing.T) {
		dir := t.TempDir()
		p := filepath.Join(dir, "a.txt")
		mustWrite(t, p, "local")
		local := readFileMTimeMillis(t, p)
		withNoopPut(t)

		caller := pushOKCaller(map[string]string{
			"ROOT": `{"result":{"items":[{"name":"a.txt","type":"file","fileId":"A","modifyTime":` +
				strconv.FormatInt(local+60000, 10) + `}],"nextToken":""}}`,
		})
		if err := runDriveCmd(t, caller, "push", "--local-folder", dir, "--remote-folder", "ROOT", "--if-exists", "smart"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got := caller.callsFor("get_upload_info"); len(got) != 0 {
			t.Errorf("smart must skip when remote is newer, got %v", got)
		}
	})

	t.Run("remote older overwrites in place", func(t *testing.T) {
		dir := t.TempDir()
		p := filepath.Join(dir, "a.txt")
		mustWrite(t, p, "local")
		local := readFileMTimeMillis(t, p)
		withNoopPut(t)

		caller := pushOKCaller(map[string]string{
			"ROOT": `{"result":{"items":[{"name":"a.txt","type":"file","fileId":"A","modifyTime":` +
				strconv.FormatInt(local-60000, 10) + `}],"nextToken":""}}`,
		})
		if err := runDriveCmd(t, caller, "push", "--local-folder", dir, "--remote-folder", "ROOT", "--if-exists", "smart"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		up := caller.callsFor("get_upload_info")
		if len(up) != 1 || up[0].args["overwriteFileId"] != "A" {
			t.Fatalf("smart overwrite must pass overwriteFileId=A, got %v", up)
		}
		if _, has := up[0].args["parentId"]; has {
			t.Error("overwrite upload must not carry parentId")
		}
	})

	t.Run("remote time invalid overwrites", func(t *testing.T) {
		dir := t.TempDir()
		mustWrite(t, filepath.Join(dir, "a.txt"), "local")
		withNoopPut(t)

		// 远端无 modifyTime → 时间不可信，smart 不敢跳过，走覆盖。
		caller := pushOKCaller(map[string]string{
			"ROOT": `{"result":{"items":[{"name":"a.txt","type":"file","fileId":"A"}],"nextToken":""}}`,
		})
		if err := runDriveCmd(t, caller, "push", "--local-folder", dir, "--remote-folder", "ROOT", "--if-exists", "smart"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if up := caller.callsFor("get_upload_info"); len(up) != 1 {
			t.Fatalf("expected overwrite upload, got %v", up)
		}
	})
}

// 上传失败 → 记 failed 并以非零退出码退出，结构化结果仍打印。
func TestCrossPlatformCoverageDrivePush_uploadFailureExitsNonZero(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "a.txt"), "local")
	testseam.Swap(t, &pushPutOpenedFile, func(context.Context, string, map[string]string, *os.File, int64) error { return errTestUpload })

	caller := pushOKCaller(nil)
	err := runDriveCmd(t, caller, "push", "--local-folder", dir, "--remote-folder", "ROOT")
	var pf *drivePushFailure
	if !errors.As(err, &pf) || pf.failed != 1 {
		t.Fatalf("expected 1 push failure, got %T %v", err, err)
	}
}

// --if-exists 非法值直接拒绝。
func TestCrossPlatformCoverageDrivePush_invalidIfExistsRejected(t *testing.T) {
	err := runDriveCmd(t, pushOKCaller(nil), "push", "--local-folder", t.TempDir(), "--remote-folder", "ROOT", "--if-exists", "bogus")
	if err == nil || !strings.Contains(err.Error(), "--if-exists") {
		t.Fatalf("expected --if-exists rejection, got %v", err)
	}
}

// space-id 必须透传到 create_folder / get_upload_info / ln 三处。
func TestCrossPlatformCoverageDrivePush_passesSpaceIDEverywhere(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, filepath.Join(dir, "sub", "a.txt"), "x")
	withNoopPut(t)

	caller := pushOKCaller(nil)
	if err := runDriveCmd(t, caller, "push", "--local-folder", dir, "--remote-folder", "ROOT", "--space-id", "SP7"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, tool := range []string{"list_files", "create_folder", "get_upload_info"} {
		calls := caller.callsFor(tool)
		if len(calls) == 0 {
			t.Fatalf("%s was never called", tool)
		}
		if calls[0].args["spaceId"] != "SP7" {
			t.Errorf("%s spaceId = %v, want SP7", tool, calls[0].args["spaceId"])
		}
	}
}

// 远端列举失败 → push 直接报错，不做任何上传。
func TestCrossPlatformCoverageDrivePush_remoteListFailurePropagates(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "a.txt"), "x")
	caller := &driveScriptCaller{reply: func(string, map[string]any, int) (string, error) { return "", errTestList }}
	if err := runDriveCmd(t, caller, "push", "--local-folder", dir, "--remote-folder", "ROOT"); err == nil {
		t.Fatal("expected remote listing failure to propagate")
	}
}

// 本地根目录不存在 → 扫描失败。
func TestCrossPlatformCoverageDrivePush_missingLocalRootFails(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "absent")
	err := runDriveCmd(t, pushOKCaller(nil), "push", "--local-folder", missing, "--remote-folder", "ROOT")
	if err == nil || !strings.Contains(err.Error(), "扫描本地目录失败") {
		t.Fatalf("expected local scan failure, got %v", err)
	}
}

// ──────────────────────────────────────────────────────────
// walkRemoteForPush — 分页、递归、深度上限、非镜像类型跳过
// ──────────────────────────────────────────────────────────

func TestCrossPlatformCoverageWalkRemoteForPush_paginationRecursionAndSkips(t *testing.T) {
	// 递归会插在 ROOT 的两页之间，所以按 (parentId, nextToken) 判定而非调用序号。
	caller := &driveScriptCaller{reply: func(_ string, args map[string]any, _ int) (string, error) {
		parent, _ := args["parentId"].(string)
		token, _ := args["nextToken"].(string)
		if parent == "SUB" {
			return `{"result":{"items":[{"name":"deep.txt","type":"file","fileId":"D"}],"nextToken":""}}`, nil
		}
		if parent == "ROOT" && token == "" {
			return `{"result":{"items":[{"name":"sub","type":"folder","fileId":"SUB"}],"nextToken":"P2"}}`, nil
		}
		if parent == "ROOT" && token == "P2" {
			return `{"result":{"items":[` +
				`{"name":"online.adoc","type":"document","fileId":"DOC"},` +
				`{"name":"ok.txt","type":"file","fileId":"OK"}` +
				`],"nextToken":""}}`, nil
		}
		return `{"result":{"items":[],"nextToken":""}}`, nil
	}}

	testseam.Swap(t, &deps, &Deps{Caller: caller, Out: &Formatter{w: io.Discard}})
	testseam.Swap(t, &os.Args, []string{"dws", "drive"})

	files, folders, err := fetchRemoteTreeForPush(context.Background(), "", "ROOT")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := folders["sub"]; !ok {
		t.Error("sub folder must be indexed")
	}
	if _, ok := files["sub/deep.txt"]; !ok {
		t.Error("nested file must be indexed with joined rel_path")
	}
	if _, ok := files["ok.txt"]; !ok {
		t.Error("second page file must be indexed")
	}
	if _, ok := files["online.adoc"]; ok {
		t.Error("non-file document must remain outside the folder mirror index")
	}
}

func TestCrossPlatformCoverageWalkRemoteForPush_depthLimitAborts(t *testing.T) {
	caller := &driveScriptCaller{reply: func(string, map[string]any, int) (string, error) {
		return `{"result":{"items":[{"name":"loop","type":"folder","fileId":"LOOP"}],"nextToken":""}}`, nil
	}}
	testseam.Swap(t, &deps, &Deps{Caller: caller, Out: &Formatter{w: io.Discard}})
	testseam.Swap(t, &os.Args, []string{"dws", "drive"})

	if _, _, err := fetchRemoteTreeForPush(context.Background(), "", "ROOT"); err == nil ||
		!strings.Contains(err.Error(), "循环引用") {
		t.Fatalf("expected depth-limit abort, got %v", err)
	}
}

// ──────────────────────────────────────────────────────────
// walkLocalForPush — 非常规文件与错误分支
// ──────────────────────────────────────────────────────────

func TestCrossPlatformCoverageWalkLocalForPush_skipsIrregularAndMissingRoot(t *testing.T) {
	if _, _, err := walkLocalForPush(filepath.Join(t.TempDir(), "absent")); err == nil {
		t.Fatal("expected error for missing root")
	}

	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "real.txt"), "x")
	if err := os.Symlink(filepath.Join(dir, "absent"), filepath.Join(dir, "dangling")); err != nil {
		t.Skipf("symlink unsupported: %v", err)
	}
	_, files, err := walkLocalForPush(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, f := range files {
		if f.RelPath == "dangling" {
			t.Error("dangling symlink must not be pushed")
		}
	}
}

// ──────────────────────────────────────────────────────────
// pushUploadFile — 凭证解析与提交失败分支
// ──────────────────────────────────────────────────────────

func TestCrossPlatformCoveragePushUploadFile_failureBranches(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "a.txt")
	mustWrite(t, p, "payload")

	cases := []struct {
		name  string
		reply func(string, map[string]any, int) (string, error)
		put   func(context.Context, string, map[string]string, string, int64) error
	}{
		{"get_upload_info tool error",
			func(tool string, _ map[string]any, _ int) (string, error) {
				if tool == "get_upload_info" {
					return "", errTestUpload
				}
				return `{"result":{},"success":true}`, nil
			}, nil},
		{"unparsable upload credential",
			func(tool string, _ map[string]any, _ int) (string, error) {
				if tool == "get_upload_info" {
					return `{"result":{}}`, nil // 无 resourceUrls
				}
				return `{"result":{},"success":true}`, nil
			}, nil},
		{"oss put fails",
			func(string, map[string]any, int) (string, error) {
				return `{"result":{"resourceUrls":[{"url":"https://oss.example.com/put","headers":{}}],"uploadId":"U1"}}`, nil
			},
			func(context.Context, string, map[string]string, string, int64) error { return errTestUpload }},
		{"commit fails",
			func(tool string, _ map[string]any, _ int) (string, error) {
				if tool == "get_upload_info" {
					return `{"result":{"resourceUrls":[{"url":"https://oss.example.com/put","headers":{}}],"uploadId":"U1"}}`, nil
				}
				return "", errTestUpload
			}, nil},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			testseam.Swap(t, &deps, &Deps{Caller: &driveScriptCaller{reply: tc.reply}, Out: &Formatter{w: io.Discard}})
			testseam.Swap(t, &os.Args, []string{"dws", "drive"})
			put := tc.put
			if put == nil {
				put = func(context.Context, string, map[string]string, string, int64) error { return nil }
			}
			testseam.Swap(t, &httpPutFile, put)

			if err := pushUploadFile(context.Background(), "SP", "PARENT", "", "a.txt", p, 7); err == nil {
				t.Fatal("expected upload failure")
			}
		})
	}
}

// pushCreateFolder：MCP 失败上抛，成功时从返回体提取 fileId。
func TestCrossPlatformCoveragePushCreateFolder_errorAndSuccess(t *testing.T) {
	testseam.Swap(t, &os.Args, []string{"dws", "drive"})

	testseam.Swap(t, &deps, &Deps{Caller: &driveScriptCaller{reply: func(string, map[string]any, int) (string, error) {
		return "", errTestUpload
	}}, Out: &Formatter{w: io.Discard}})
	if _, err := pushCreateFolder(context.Background(), "", "P", "n"); err == nil {
		t.Fatal("expected create_folder error to propagate")
	}

	ok := &driveScriptCaller{reply: func(string, map[string]any, int) (string, error) {
		return `{"result":{"fileId":"FID42"},"success":true}`, nil
	}}
	testseam.Swap(t, &deps, &Deps{Caller: ok, Out: &Formatter{w: io.Discard}})
	got, err := pushCreateFolder(context.Background(), "SP", "P", "n")
	if err != nil || got != "FID42" {
		t.Fatalf("pushCreateFolder = (%q, %v), want (FID42, nil)", got, err)
	}
	if ok.calls[0].args["spaceId"] != "SP" || ok.calls[0].args["parentId"] != "P" {
		t.Errorf("create_folder args = %v", ok.calls[0].args)
	}
}
