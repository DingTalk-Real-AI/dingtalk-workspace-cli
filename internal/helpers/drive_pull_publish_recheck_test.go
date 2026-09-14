package helpers

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
)

// smart 策略在入口按固定的 initialInfo 判定「本地落后、需要下载」，但远端读与落盘
// 期间同一 inode 的 mtime 仍可能被并发写者推到 ≥ 远端时间。此时发布会用旧的远端版本
// 覆盖更新的本地内容，因此必须在发布前按当前状态二次判定并跳过。
func TestCrossPlatformCoverageDrivePullSmartSkipsWhenTargetCatchesUpDuringDownload(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "a.txt")
	const remoteMillis int64 = 2_000_000
	if err := os.WriteFile(target, []byte("local"), 0o600); err != nil {
		t.Fatal(err)
	}
	// 入口处的 mtime 必须严格早于远端，否则在发出远端读之前就已跳过。
	stale := time.UnixMilli(remoteMillis - 60_000)
	if err := os.Chtimes(target, stale, stale); err != nil {
		t.Fatal(err)
	}
	installPinnedPullSuccess(t)
	testseam.Swap(t, &pullDownloadFile, func(_ context.Context, _ string, _ map[string]string, destination *os.File) error {
		if _, err := destination.WriteString("remote"); err != nil {
			return err
		}
		// 就地改时间：inode 不变，所以身份比较仍然成立，只有时间判定能拦下发布。
		fresh := time.UnixMilli(remoteMillis + 60_000)
		return os.Chtimes(target, fresh, fresh)
	})

	action, err := pullOneFileAtRoot(context.Background(), "", &remoteFile{
		FileID:            "F",
		ModifiedTime:      remoteMillis,
		ModifiedTimeValid: true,
	}, root, "a.txt", ifExistsSmart)
	if err != nil {
		t.Fatalf("pull = %v", err)
	}
	if action != pullActionSkipped {
		t.Fatalf("action = %q, want %q", action, pullActionSkipped)
	}
	if got, err := os.ReadFile(target); err != nil || string(got) != "local" {
		t.Fatalf("target = %q, err=%v; 追平后的本地内容必须保留", got, err)
	}
}

// 发布成功后固定目录仍可能被移走。此时结果已经落盘，门禁只能报失败而不能回删——
// 同名 terminal entry 可能已被并发替换，二次 Lstat→Remove 会误删他人的文件。
func TestCrossPlatformCoverageDrivePullFailsWhenPinnedRootMovesAfterPublish(t *testing.T) {
	root := t.TempDir()
	installPinnedPullSuccess(t)
	decoy := t.TempDir()
	decoyInfo, err := os.Stat(decoy)
	if err != nil {
		t.Fatal(err)
	}
	published := false
	testseam.Swap(t, &pullRename, func(parent *os.Root, oldName, newName string) error {
		if err := parent.Rename(oldName, newName); err != nil {
			return err
		}
		published = true
		return nil
	})
	// 只在发布之后让根目录身份不匹配：下载前与发布前的两次核对都必须先通过。
	testseam.Swap(t, &pullPathStat, func(path string) (os.FileInfo, error) {
		if published {
			return decoyInfo, nil
		}
		return os.Stat(path)
	})

	action, err := pullOneFileAtRoot(context.Background(), "", &remoteFile{FileID: "F"}, root, "a.txt", ifExistsOverwrite)
	if err == nil || !strings.Contains(err.Error(), "本地根目录在同步期间被替换") {
		t.Fatalf("error = %v", err)
	}
	if action != pullActionFailed {
		t.Fatalf("action = %q, want %q", action, pullActionFailed)
	}
	if got, err := os.ReadFile(filepath.Join(root, "a.txt")); err != nil || string(got) != "remote" {
		t.Fatalf("published target = %q, err=%v; 已发布的结果不得被回删", got, err)
	}
}
