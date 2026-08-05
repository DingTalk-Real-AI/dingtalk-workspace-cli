package helpers

import (
	"strings"
	"testing"
)

// 命令层路由：--latest 的校验时机、三条落点（钉盘单层 / 钉盘 BFS / 知识库 BFS）。
// 后置处理器与扫描循环的行为断言在 drive_latest_test.go。

func TestCrossPlatformCoverageDriveListLatestRejectedWithVersions(t *testing.T) {
	caller := &guardedMutationCaller{}
	err := executeGuardedMutationCommand(t, caller, newDriveCommand,
		"list", "--versions", "--node", "node-1", "--latest", "3")
	if err == nil || !strings.Contains(err.Error(), "--versions 与 --latest 不能同时使用") {
		t.Fatalf("err = %v", err)
	}
	if len(caller.calls) != 0 {
		t.Fatalf("calls = %#v", caller.calls)
	}
}

// versions 合法使用 --limit：--latest 的互斥校验不得在此路径上误伤。
func TestCrossPlatformCoverageDriveListVersionsWithLimitStillWorks(t *testing.T) {
	caller := &guardedMutationCaller{}
	err := executeGuardedMutationCommand(t, caller, newDriveCommand,
		"list", "--versions", "--node", "node-1", "--limit", "5")
	if err != nil {
		t.Fatal(err)
	}
	if len(caller.calls) != 1 || caller.calls[0].toolName != "list_file_versions" {
		t.Fatalf("calls = %#v", caller.calls)
	}
}

func TestCrossPlatformCoverageDriveListLatestRejectsOutOfRange(t *testing.T) {
	for _, value := range []string{"0", "51"} {
		caller := &guardedMutationCaller{}
		err := executeGuardedMutationCommand(t, caller, newDriveCommand, "list", "--latest", value)
		if err == nil || !strings.Contains(err.Error(), "--latest 必须为 1~50 的整数") {
			t.Fatalf("--latest %s err = %v", value, err)
		}
		if len(caller.calls) != 0 {
			t.Fatalf("--latest %s calls = %#v", value, caller.calls)
		}
	}
}

func TestCrossPlatformCoverageDriveListLatestRejectsExclusiveFlags(t *testing.T) {
	for _, extra := range [][]string{
		{"--limit", "5"},
		{"--order-by", "name"},
		{"--order", "asc"},
		{"--cursor", "c1"},
	} {
		caller := &guardedMutationCaller{}
		args := append([]string{"list", "--latest", "3"}, extra...)
		err := executeGuardedMutationCommand(t, caller, newDriveCommand, args...)
		if err == nil || !strings.Contains(err.Error(), "--latest 不能与") {
			t.Fatalf("%v err = %v", extra, err)
		}
		if len(caller.calls) != 0 {
			t.Fatalf("%v calls = %#v", extra, caller.calls)
		}
	}
}

func TestCrossPlatformCoverageDriveListLatestPanRoute(t *testing.T) {
	useDriveLatestArgs(t)
	caller := &depthArgsRecordingCaller{steps: []scriptedToolStep{
		{text: `{"items":[
			{"fileId":"f1","name":"a.txt","type":"FILE","modifyTime":1700000002000},
			{"fileId":"f2","name":"b.txt","type":"FILE","modifyTime":1700000001000}]}`},
	}}
	err := executeDriveCommand(t, caller,
		"list", "--latest", "2", "--space-id", "sp-1", "--thumbnail", "--folder", "root-folder")
	if err != nil {
		t.Fatal(err)
	}
	if len(caller.calls) != 1 {
		t.Fatalf("calls = %#v, want 1", caller.calls)
	}
	args := caller.calls[0]
	// 钉盘单层 latest 借服务端排序：orderBy/order 由 CLI 强制注入。
	if args["orderBy"] != "modifyTime" || args["order"] != "desc" {
		t.Fatalf("server-side ordering missing: %#v", args)
	}
	if args["spaceId"] != "sp-1" || args["withThumbnail"] != true ||
		args["parentId"] != "root-folder" || args["maxResults"] != float64(driveDepthPageSize) {
		t.Fatalf("pan latest args = %#v", args)
	}
}

func TestCrossPlatformCoverageDriveListLatestPanRejectsNumericFolder(t *testing.T) {
	caller := &guardedMutationCaller{}
	err := executeGuardedMutationCommand(t, caller, newDriveCommand,
		"list", "--latest", "3", "--folder", "12345")
	if err == nil {
		t.Fatal("numeric drive folder returned nil")
	}
	if len(caller.calls) != 0 {
		t.Fatalf("calls = %#v", caller.calls)
	}
}

func TestCrossPlatformCoverageDriveListLatestPanDepthRoutesBFS(t *testing.T) {
	useDriveLatestArgs(t)
	caller := &depthArgsRecordingCaller{steps: []scriptedToolStep{
		{text: `{"items":[{"fileId":"f1","name":"a.txt","type":"FILE","modifyTime":1700000002000}]}`},
	}}
	err := executeDriveCommand(t, caller, "list", "--latest", "2", "--depth", "2")
	if err != nil {
		t.Fatal(err)
	}
	if len(caller.calls) != 1 {
		t.Fatalf("calls = %#v, want 1", caller.calls)
	}
	// depth>1 走 BFS，不注入服务端排序（多目录合并后由后置处理器统一排序）。
	if _, ok := caller.calls[0]["orderBy"]; ok {
		t.Fatalf("BFS route should not force orderBy: %#v", caller.calls[0])
	}
}

func TestCrossPlatformCoverageDriveListLatestWorkspaceRoutesDocBFS(t *testing.T) {
	caller := &depthArgsRecordingCaller{steps: []scriptedToolStep{
		{text: `{"nodes":[
			{"nodeId":"n1","name":"old.doc","nodeType":"doc","updateTime":1700000001000},
			{"nodeId":"n2","name":"new.doc","nodeType":"doc","updateTime":1700000005000}],"hasMore":false}`},
	}}
	err := executeDriveCommand(t, caller, "list", "--workspace", "ws-1", "--latest", "2")
	if err != nil {
		t.Fatal(err)
	}
	if len(caller.calls) != 1 {
		t.Fatalf("calls = %#v, want 1", caller.calls)
	}
	args := caller.calls[0]
	if args["workspaceId"] != "ws-1" || args["pageSize"] != float64(docDepthPageSize) {
		t.Fatalf("doc latest args = %#v", args)
	}
}

// --pattern 在知识库单层原本被拒；latest 打开 BFS 路径后应放行（先递归后过滤）。
func TestCrossPlatformCoverageDriveListLatestWorkspaceAllowsPattern(t *testing.T) {
	caller := &depthArgsRecordingCaller{steps: []scriptedToolStep{
		{text: `{"nodes":[
			{"nodeId":"n1","name":"日报-0801","nodeType":"doc","updateTime":1700000001000},
			{"nodeId":"n2","name":"周报-0801","nodeType":"doc","updateTime":1700000005000}],"hasMore":false}`},
	}}
	if err := executeDriveCommand(t, caller,
		"list", "--workspace", "ws-1", "--latest", "2", "--pattern", "*日报*"); err != nil {
		t.Fatal(err)
	}

	// 不带 latest 的知识库单层仍然拒绝 --pattern。
	rejected := &guardedMutationCaller{}
	err := executeGuardedMutationCommand(t, rejected, newDriveCommand,
		"list", "--workspace", "ws-1", "--pattern", "*日报*")
	if err == nil || !strings.Contains(err.Error(), "--pattern 仅适用于钉盘文件列表") {
		t.Fatalf("err = %v", err)
	}
}
