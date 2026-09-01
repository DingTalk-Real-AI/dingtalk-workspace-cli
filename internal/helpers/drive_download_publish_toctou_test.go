package helpers

// 发布点 TOCTOU 回归测试：checkDownloadConflict 只能在下载开始前检查，
// 长下载期间目标可能被并发创建。这些测试模拟"检查后注入目标文件"，锁定
// 发布阶段的原子 no-replace 兜底（drivePublishFile）不被回归破坏。

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// drivePublishFile 单元：no-replace 语义三态 + overwrite 替换。
func TestCrossPlatformCoverageDrivePublishFile(t *testing.T) {
	t.Run("no-replace publishes and removes source", func(t *testing.T) {
		dir := t.TempDir()
		source := filepath.Join(dir, "a.dwspart")
		target := filepath.Join(dir, "a.bin")
		if err := os.WriteFile(source, []byte("payload"), 0o644); err != nil {
			t.Fatal(err)
		}

		if err := drivePublishFile(source, target, false); err != nil {
			t.Fatal(err)
		}

		got, err := os.ReadFile(target)
		if err != nil {
			t.Fatal(err)
		}
		if string(got) != "payload" {
			t.Fatalf("target = %q, want payload", got)
		}
		if _, statErr := os.Stat(source); !os.IsNotExist(statErr) {
			t.Fatal("no-replace 发布成功后应移除临时文件")
		}
	})

	t.Run("no-replace rejects existing target and keeps source", func(t *testing.T) {
		dir := t.TempDir()
		source := filepath.Join(dir, "b.dwspart")
		target := filepath.Join(dir, "b.bin")
		if err := os.WriteFile(source, []byte("payload"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(target, []byte("victim"), 0o644); err != nil {
			t.Fatal(err)
		}

		err := drivePublishFile(source, target, false)
		if err == nil {
			t.Fatal("no-replace 发布到已存在目标应失败")
		}
		if !errors.Is(err, errDriveDownloadTargetExists) {
			t.Fatalf("want errDriveDownloadTargetExists, got %v", err)
		}

		got, readErr := os.ReadFile(target)
		if readErr != nil {
			t.Fatal(readErr)
		}
		if string(got) != "victim" {
			t.Fatalf("已存在目标必须原样保留, got %q", got)
		}
		if _, statErr := os.Stat(source); statErr != nil {
			t.Fatalf("拒绝发布后应保留临时文件（--overwrite 重跑可复用）: %v", statErr)
		}
	})

	t.Run("overwrite replaces existing target", func(t *testing.T) {
		dir := t.TempDir()
		source := filepath.Join(dir, "c.dwspart")
		target := filepath.Join(dir, "c.bin")
		if err := os.WriteFile(source, []byte("payload"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(target, []byte("victim"), 0o644); err != nil {
			t.Fatal(err)
		}

		if err := drivePublishFile(source, target, true); err != nil {
			t.Fatal(err)
		}

		got, err := os.ReadFile(target)
		if err != nil {
			t.Fatal(err)
		}
		if string(got) != "payload" {
			t.Fatalf("overwrite 应替换目标, got %q", got)
		}
	})

	t.Run("no-replace link failure other than EEXIST propagates", func(t *testing.T) {
		dir := t.TempDir()
		source := filepath.Join(dir, "d.dwspart")
		origLink := driveOsLink
		t.Cleanup(func() { driveOsLink = origLink })
		driveOsLink = func(string, string) error { return errors.New("injected link failure") }

		err := drivePublishFile(source, filepath.Join(dir, "d.bin"), false)
		if err == nil {
			t.Fatal("link 失败应返回错误")
		}
		if errors.Is(err, errDriveDownloadTargetExists) {
			t.Fatalf("非 EEXIST 失败不应归类为目标已存在: %v", err)
		}
		if !strings.Contains(err.Error(), "injected link failure") {
			t.Fatalf("原始 link 错误应传播: %v", err)
		}
	})
}

// 整流路径 TOCTOU：检查通过后、发布前目标被并发创建 → 原子拒绝，victim
// 原样保留，临时文件清理。httpGetFile stub 写完临时文件后注入目标，精确
// 复现评审指出的竞态窗口。
func TestCrossPlatformCoverageDownloadSingleStreamTOCTOU(t *testing.T) {
	SetHTTPGetFile(func(ctx context.Context, url string, headers map[string]string, destPath string) error {
		if err := os.WriteFile(destPath, []byte("downloaded"), 0o644); err != nil {
			return err
		}
		// 模拟竞态：检查点已过，发布前目标被另一个进程创建。
		return os.WriteFile(streamTempDest(destPath), []byte("victim"), 0o644)
	})
	defer SetHTTPGetFile(nil)

	dest := filepath.Join(t.TempDir(), "toctou.bin")
	err := downloadSingleWithAuthRetry(context.Background(), nil, "https://old.example.com/f", nil, dest, false)
	if err == nil {
		t.Fatal("竞态注入目标后发布应失败")
	}
	if !errors.Is(err, errDriveDownloadTargetExists) {
		t.Fatalf("want errDriveDownloadTargetExists, got %v", err)
	}

	got, readErr := os.ReadFile(dest)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if string(got) != "victim" {
		t.Fatalf("并发出现的目标必须未被覆盖, got %q", got)
	}
	assertNoStreamTempLeftovers(t, dest)
}

// 整流路径 TOCTOU + --overwrite：发布时目标已存在但显式允许覆盖 → 成功替换。
func TestCrossPlatformCoverageDownloadSingleStreamTOCTOUOverwrite(t *testing.T) {
	SetHTTPGetFile(func(ctx context.Context, url string, headers map[string]string, destPath string) error {
		if err := os.WriteFile(destPath, []byte("downloaded"), 0o644); err != nil {
			return err
		}
		return os.WriteFile(streamTempDest(destPath), []byte("victim"), 0o644)
	})
	defer SetHTTPGetFile(nil)

	dest := filepath.Join(t.TempDir(), "toctou-overwrite.bin")
	if err := downloadSingleWithAuthRetry(context.Background(), nil, "https://old.example.com/f", nil, dest, true); err != nil {
		t.Fatalf("--overwrite 应容忍竞态注入: %v", err)
	}

	got, err := os.ReadFile(dest)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "downloaded" {
		t.Fatalf("--overwrite 应替换竞态窗口内出现的目标, got %q", got)
	}
	assertNoStreamTempLeftovers(t, dest)
}

// 分片路径 TOCTOU：logf 在分片开始前被调用（检查点已过），此时注入目标；
// 分片全部完成后发布必须原子拒绝，victim 原样保留，.dwspart 保留供
// --overwrite 重跑复用。
func TestCrossPlatformCoverageRangedPartsTOCTOU(t *testing.T) {
	content := makeTestContent(1000)
	srv := rangeTestServer(t, content, "", nil)
	defer srv.Close()

	dest := filepath.Join(t.TempDir(), "toctou.bin")
	opts := smallPartOpts(300, 3)
	opts.knownSize = 1000
	opts.logf = func(format string, args ...any) {
		// 检查后注入：第一次日志（分片开始）即创建受害目标。
		if _, statErr := os.Stat(dest); os.IsNotExist(statErr) {
			if err := os.WriteFile(dest, []byte("victim"), 0o644); err != nil {
				t.Errorf("注入目标文件失败: %v", err)
			}
		}
	}

	err := driveTransferDownload(context.Background(), nil, srv.URL, nil, dest, opts)
	if err == nil {
		t.Fatal("竞态注入目标后发布应失败")
	}
	if !errors.Is(err, errDriveDownloadTargetExists) {
		t.Fatalf("want errDriveDownloadTargetExists, got %v", err)
	}

	got, readErr := os.ReadFile(dest)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if string(got) != "victim" {
		t.Fatalf("并发出现的目标必须未被覆盖, got %q", got)
	}
	if _, statErr := os.Stat(dest + drivePartFileSuffix); statErr != nil {
		t.Fatal("分片发布失败后应保留 .dwspart（--overwrite 重跑可复用已完成分片）")
	}
}

// 分片路径 TOCTOU + --overwrite：显式允许覆盖时，竞态窗口内出现的目标被替换。
func TestCrossPlatformCoverageRangedPartsTOCTOUOverwrite(t *testing.T) {
	content := makeTestContent(1000)
	srv := rangeTestServer(t, content, "", nil)
	defer srv.Close()

	dest := filepath.Join(t.TempDir(), "toctou-overwrite.bin")
	opts := smallPartOpts(300, 3)
	opts.knownSize = 1000
	opts.overwrite = true
	opts.logf = func(format string, args ...any) {
		if _, statErr := os.Stat(dest); os.IsNotExist(statErr) {
			if err := os.WriteFile(dest, []byte("victim"), 0o644); err != nil {
				t.Errorf("注入目标文件失败: %v", err)
			}
		}
	}

	if err := driveTransferDownload(context.Background(), nil, srv.URL, nil, dest, opts); err != nil {
		t.Fatalf("--overwrite 应容忍竞态注入: %v", err)
	}
	verifyFile(t, dest, content)
	if _, statErr := os.Stat(dest + drivePartFileSuffix); !os.IsNotExist(statErr) {
		t.Fatal("overwrite 发布成功后不应残留 .dwspart 临时文件")
	}
}

// RunE 层 TOCTOU：检查点通过后、发布前目标被并发创建 → 整流引擎在发布点
// 原子拒绝，RunE 必须把 sentinel 转成与检查点相同的 INPUT_FILE_ALREADY_EXISTS
// 结构化错误（两个命令路径各验一次）。
func TestCrossPlatformCoverageDriveDownloadRunEpublishConflict(t *testing.T) {
	// stub 写完整流临时文件后注入最终目标，复现检查后竞态窗口。
	oldGet := httpGetFile
	httpGetFile = func(_ context.Context, _ string, _ map[string]string, destPath string) error {
		if err := os.WriteFile(destPath, []byte("downloaded"), 0o644); err != nil {
			return err
		}
		return os.WriteFile(streamTempDest(destPath), []byte("victim"), 0o644)
	}
	t.Cleanup(func() { httpGetFile = oldGet })

	t.Run("latest via download", func(t *testing.T) {
		target := filepath.Join(t.TempDir(), "latest.txt")
		caller := &scriptedToolCaller{steps: []scriptedToolStep{{text: `{"downloadUrl":"https://example.test/latest.txt","fileName":"latest.txt"}`}}}
		err := executeDriveEdge(t, caller, "download", "--node", "node-latest", "--output", target)
		if err == nil {
			t.Fatal("发布阶段冲突应报错")
		}
		var cliErr *CLIError
		if !errors.As(err, &cliErr) || cliErr.Code != CodeFileAlreadyExists || cliErr.Operation != "drive download" {
			t.Fatalf("want INPUT_FILE_ALREADY_EXISTS CLIError for drive download, got %v", err)
		}
	})

	t.Run("versioned via download-version", func(t *testing.T) {
		target := filepath.Join(t.TempDir(), "versioned.txt")
		caller := &scriptedToolCaller{steps: []scriptedToolStep{{text: `{"downloadUrl":"https://example.test/versioned.txt","fileName":"versioned.txt"}`}}}
		err := executeDriveEdge(t, caller, "download-version", "--node", "node-v", "--version", "4", "--output", target)
		if err == nil {
			t.Fatal("发布阶段冲突应报错")
		}
		var cliErr *CLIError
		if !errors.As(err, &cliErr) || cliErr.Code != CodeFileAlreadyExists || cliErr.Operation != "drive download-version" {
			t.Fatalf("want INPUT_FILE_ALREADY_EXISTS CLIError for drive download-version, got %v", err)
		}
	})
}
