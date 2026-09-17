package helpers

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
)

// 本文件补齐几条在默认策略改为 skip 后失去覆盖的边界分支。

// pull 自动创建目标根失败（--local-folder 指向一个常规文件）→ 明确报错，不落盘。
func TestCrossPlatformCoverageDrivePull_localRootIsFileFails(t *testing.T) {
	dir := t.TempDir()
	asFile := filepath.Join(dir, "not-a-dir")
	mustWrite(t, asFile, "x")

	caller := pullListingCaller(`{"name":"a.txt","type":"file","fileId":"A","modifyTime":1}`)
	err := runDriveCmd(t, caller, "pull", "--local-folder", asFile, "--remote-folder", "ROOT")
	if err == nil || !strings.Contains(err.Error(), "创建本地目录失败") {
		t.Fatalf("expected local root creation failure, got %v", err)
	}
}

// push 遍历远端时 list_files 在**子目录**层失败 → 错误如实上抛（walkRemoteForPush 的递归错误分支）。
func TestCrossPlatformCoverageDrivePush_remoteSubdirListFailurePropagates(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "a.txt"), "x")
	withNoopPut(t)

	caller := &driveScriptCaller{reply: func(tool string, args map[string]any, _ int) (string, error) {
		if tool != "list_files" {
			return `{"result":{},"success":true}`, nil
		}
		switch args["parentId"] {
		case "ROOT":
			return `{"result":{"items":[{"name":"sub","type":"folder","fileId":"SUB"}],"nextToken":""}}`, nil
		default:
			return "", errTestList // 递归进入子目录后失败
		}
	}}
	if err := runDriveCmd(t, caller, "push", "--local-folder", dir, "--remote-folder", "ROOT"); err == nil {
		t.Fatal("expected the nested list_files failure to propagate")
	}
}

// push 的远端索引跳过非 file 类型（在线文档/快捷方式），不把它们当成可覆盖的既有文件。
func TestCrossPlatformCoverageDrivePush_remoteNonFileEntriesAreSkipped(t *testing.T) {
	caller := &driveScriptCaller{reply: func(tool string, args map[string]any, _ int) (string, error) {
		if tool != "list_files" {
			return `{"result":{},"success":true}`, nil
		}
		if args["parentId"] == "ROOT" {
			return `{"result":{"items":[` +
				`{"name":"online.adoc","type":"document","fileId":"D1"},` +
				`{"name":"link","type":"shortcut","fileId":"S1"},` +
				`{"name":"real.txt","type":"file","fileId":"F1"}` +
				`],"nextToken":""}}`, nil
		}
		return `{"result":{"items":[],"nextToken":""}}`, nil
	}}

	testseam.Swap(t, &deps, &Deps{Caller: caller, Out: &Formatter{w: io.Discard}})
	testseam.Swap(t, &os.Args, []string{"dws", "drive"})

	files, _, err := fetchRemoteTreeForPush(context.Background(), "", "ROOT")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := files["real.txt"]; !ok {
		t.Error("regular file must be indexed")
	}
	for _, skipped := range []string{"online.adoc", "link"} {
		if _, ok := files[skipped]; ok {
			t.Errorf("non-file entry %q must not be indexed", skipped)
		}
	}
}

// list_files 是远端权威树的数据来源；缺失 result 不能伪装成空目录。
func TestCrossPlatformCoverageParseDriveList_missingResultFailsClosed(t *testing.T) {
	if _, _, err := parseDriveList(`{"success":true}`); err == nil || !strings.Contains(err.Error(), "缺少 result") {
		t.Fatalf("parseDriveList error = %v, want missing-result failure", err)
	}
}

// push 遍历远端时返回体无法解析 → 解析错误如实上抛（与调用失败是不同分支）。
func TestCrossPlatformCoverageDrivePush_remoteListParseFailurePropagates(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "a.txt"), "x")
	withNoopPut(t)

	caller := &driveScriptCaller{reply: func(tool string, _ map[string]any, _ int) (string, error) {
		if tool != "list_files" {
			return `{"result":{},"success":true}`, nil
		}
		return `{"result":"not-a-list"}`, nil // 既不是 {items:[]} 也不是数组
	}}
	err := runDriveCmd(t, caller, "push", "--local-folder", dir, "--remote-folder", "ROOT")
	if err == nil || !strings.Contains(err.Error(), "list_files") {
		t.Fatalf("expected a list_files parse error, got %v", err)
	}
}
