package helpers

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestCrossPlatformCoverageBuildConnectVideoStoryboardEdges(t *testing.T) {
	originalLookPath := connectStoryboardLookPath
	originalCommand := connectStoryboardCommand
	originalMkdir := connectStoryboardMkdirAll
	originalStat := connectStoryboardStat
	originalRemove := connectStoryboardRemove
	t.Cleanup(func() {
		connectStoryboardLookPath = originalLookPath
		connectStoryboardCommand = originalCommand
		connectStoryboardMkdirAll = originalMkdir
		connectStoryboardStat = originalStat
		connectStoryboardRemove = originalRemove
	})

	connectStoryboardLookPath = func(name string) (string, error) {
		return "", errors.New(name)
	}
	if _, err := buildConnectVideoStoryboard(context.Background(), "video"); err == nil || !strings.Contains(err.Error(), "ffmpeg") {
		t.Fatalf("missing ffmpeg error = %v", err)
	}
	connectStoryboardLookPath = func(name string) (string, error) {
		if name == "ffprobe" {
			return "", errors.New(name)
		}
		return name, nil
	}
	if _, err := buildConnectVideoStoryboard(context.Background(), "video"); err == nil || !strings.Contains(err.Error(), "ffprobe") {
		t.Fatalf("missing ffprobe error = %v", err)
	}
	connectStoryboardLookPath = func(name string) (string, error) { return name, nil }

	helper := func(script string) *exec.Cmd { return exec.Command("sh", "-c", script) }
	connectStoryboardCommand = func(context.Context, string, ...string) *exec.Cmd { return helper("exit 1") }
	if _, err := buildConnectVideoStoryboard(context.Background(), "video"); err == nil || !strings.Contains(err.Error(), "读取视频时长失败") {
		t.Fatalf("probe failure error = %v", err)
	}
	connectStoryboardCommand = func(context.Context, string, ...string) *exec.Cmd { return helper("printf invalid") }
	if _, err := buildConnectVideoStoryboard(context.Background(), "video"); err == nil || !strings.Contains(err.Error(), "无效视频时长") {
		t.Fatalf("invalid duration error = %v", err)
	}
	connectStoryboardCommand = func(context.Context, string, ...string) *exec.Cmd { return helper("printf 0") }
	if _, err := buildConnectVideoStoryboard(context.Background(), "video"); err == nil || !strings.Contains(err.Error(), "无效视频时长") {
		t.Fatalf("zero duration error = %v", err)
	}
	connectStoryboardMkdirAll = func(string, os.FileMode) error { return errors.New("mkdir") }
	connectStoryboardCommand = func(context.Context, string, ...string) *exec.Cmd { return helper("printf 1") }
	if _, err := buildConnectVideoStoryboard(context.Background(), "video"); err == nil || err.Error() != "mkdir" {
		t.Fatalf("mkdir error = %v", err)
	}
	connectStoryboardMkdirAll = originalMkdir

	commandCalls := 0
	connectStoryboardCommand = func(context.Context, string, ...string) *exec.Cmd {
		commandCalls++
		if commandCalls%2 == 1 {
			return helper("printf 12")
		}
		return helper("printf boom; exit 1")
	}
	if _, err := buildConnectVideoStoryboard(context.Background(), "video"); err == nil || !strings.Contains(err.Error(), "生成视频故事板失败") {
		t.Fatalf("ffmpeg error = %v", err)
	}

	commandCalls = 0
	connectStoryboardCommand = func(context.Context, string, ...string) *exec.Cmd {
		commandCalls++
		if commandCalls%2 == 1 {
			return helper("printf 1")
		}
		return helper("exit 0")
	}
	connectStoryboardStat = func(string) (os.FileInfo, error) { return nil, errors.New("stat") }
	if _, err := buildConnectVideoStoryboard(context.Background(), "video"); err == nil || !strings.Contains(err.Error(), "未生成") {
		t.Fatalf("stat error = %v", err)
	}

	empty := filepath.Join(t.TempDir(), "empty")
	if err := os.WriteFile(empty, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	connectStoryboardStat = func(string) (os.FileInfo, error) { return os.Stat(empty) }
	if _, err := buildConnectVideoStoryboard(context.Background(), "video"); err == nil || !strings.Contains(err.Error(), "大小异常") {
		t.Fatalf("size error = %v", err)
	}

	nonempty := filepath.Join(t.TempDir(), "storyboard")
	if err := os.WriteFile(nonempty, []byte("image"), 0o600); err != nil {
		t.Fatal(err)
	}
	connectStoryboardStat = func(string) (os.FileInfo, error) { return os.Stat(nonempty) }
	if path, err := buildConnectVideoStoryboard(context.Background(), "video"); err != nil || !strings.HasSuffix(path, ".storyboard.jpg") {
		t.Fatalf("successful storyboard = %q, %v", path, err)
	}
}
