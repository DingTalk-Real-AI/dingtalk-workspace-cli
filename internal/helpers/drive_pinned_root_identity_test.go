package helpers

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
)

// Windows 会用已打开的目录句柄锁住目录，把「固定后目录被移走／换成软链」这类
// TOCTOU 在该平台变成物理不可达：os.Rename 直接 ERROR_SHARING_VIOLATION。
// 但被测的 fail-closed 分支必须在所有平台都覆盖，所以这里提供与真实替换等价的
// 身份注入：pinnedPullRoot.verify() 与 verifyParent() 都只通过 pullPathStat /
// pullRootLstat 读取当前身份，让这两个 seam 指向另一个目录即可命中同一分支。

// forcePinnedFallbackForTest 打开后，「移走固定目录」的复现手法一律走身份注入降级。
// 默认 false：Unix 上用真实移动，验证更强。只有 Windows 才会真的走降级分支，所以
// 下面的注入回归测试会打开它，让那条路径在任何平台都能被验证。
var forcePinnedFallbackForTest = false

// swapPinnedRootIdentity 让 absDir 的根身份读数指向另一个目录，使随后的
// pinnedPullRoot.verify() 判定「本地根目录在同步期间被替换」。
func swapPinnedRootIdentity(t *testing.T, absDir string) {
	t.Helper()
	decoy, err := os.Stat(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Clean(absDir)
	testseam.Swap(t, &pullPathStat, func(path string) (os.FileInfo, error) {
		if filepath.Clean(path) == want {
			return decoy, nil
		}
		return os.Stat(path)
	})
}

// 降级注入只有 Windows 会真的走到（Unix 的 rename 成功）。这里强制打开开关，
// 确保注入手段本身在任何平台都可回归：不移动目录，但固定根的校验必须随之失败，
// 且原有目录树不得被动过。
func TestCrossPlatformCoveragePinnedRootFallbackInjectionFailsVerification(t *testing.T) {
	testseam.Swap(t, &forcePinnedFallbackForTest, true)
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "a.txt"), "pinned-original")
	root := newPinnedPullRootForCoverage(t, dir)
	if err := root.verify(); err != nil {
		t.Fatalf("baseline verify: %v", err)
	}

	if moved := replacePinnedMirrorRoot(t, dir, filepath.Join(t.TempDir(), "moved")); moved {
		t.Fatal("降级路径不应真的移动目录")
	}
	if err := root.verify(); err == nil || !strings.Contains(err.Error(), "本地根目录在同步期间被替换") {
		t.Fatalf("身份注入后校验必须失败，got %v", err)
	}
	if b, err := os.ReadFile(filepath.Join(dir, "a.txt")); err != nil || string(b) != "pinned-original" {
		t.Fatalf("降级必须保持原目录树不变: %q err=%v", string(b), err)
	}
}

// swapPinnedParentIdentity 让固定根下名为 rel 的父目录身份读数指向另一个目录，
// 使随后的 verifyParent() 判定「本地目标目录在下载期间被替换」。
func swapPinnedParentIdentity(t *testing.T, rel string) {
	t.Helper()
	decoy, err := os.Lstat(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	testseam.Swap(t, &pullRootLstat, func(parent *os.Root, name string) (os.FileInfo, error) {
		if filepath.ToSlash(filepath.Clean(name)) == rel {
			return decoy, nil
		}
		return parent.Lstat(name)
	})
}
