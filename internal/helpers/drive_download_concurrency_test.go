package helpers

// 并发下载回归测试：整流路径使用目标同目录独占创建的随机唯一临时文件、
// 分片路径使用跨进程 O_EXCL 锁，保证两个并发写者只能发布一个完整、未混写
// 的产物（评审 P1：固定 <dest>.dwspart 会被并发 os.Create 互相截断）。

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// streamTempDest 从整流唯一临时文件路径推导发布目标（镜像
// createDriveStreamTempFile 的 ".<base>.dwspart.<rand>" 命名约定）。
func streamTempDest(tmpPath string) string {
	dir, name := filepath.Split(tmpPath)
	name = strings.TrimPrefix(name, ".")
	if idx := strings.Index(name, drivePartFileSuffix+"."); idx >= 0 {
		name = name[:idx]
	}
	return filepath.Join(dir, name)
}

// assertNoStreamTempLeftovers 断言目标所在目录无整流临时文件残留。
func assertNoStreamTempLeftovers(t *testing.T, dest string) {
	t.Helper()
	leftovers, _ := filepath.Glob(filepath.Join(filepath.Dir(dest), ".*"+drivePartFileSuffix+"*"))
	if len(leftovers) != 0 {
		t.Fatalf("整流临时文件残留: %v", leftovers)
	}
}

// 整流并发写者：两个下载同时写同一目标，各自写完整内容（中途留交错窗口，
// 固定临时名若有回归必然互相截断混写）。发布点 no-replace 保证恰好一个
// 成功，产物必须是赢家的完整内容。
func TestCrossPlatformCoverageConcurrentStreamWritersPublishIntactFile(t *testing.T) {
	const size = 8192
	dir := t.TempDir()
	dest := filepath.Join(dir, "same-name.bin")

	oldGet := httpGetFile
	httpGetFile = func(_ context.Context, url string, _ map[string]string, tmpPath string) error {
		content := bytes.Repeat([]byte("A"), size)
		if strings.Contains(url, "writer-b") {
			content = bytes.Repeat([]byte("B"), size)
		}
		// 分两段写并留出交错窗口：若临时文件名回归为固定共享名，
		// 两个写者的内容必然互相截断、混写。
		if err := os.WriteFile(tmpPath, content[:size/2], 0o644); err != nil {
			return err
		}
		time.Sleep(60 * time.Millisecond)
		f, err := os.OpenFile(tmpPath, os.O_WRONLY|os.O_APPEND, 0o644)
		if err != nil {
			return err
		}
		_, werr := f.Write(content[size/2:])
		f.Close()
		return werr
	}
	t.Cleanup(func() { httpGetFile = oldGet })

	const writers = 2
	start := make(chan struct{})
	errs := make([]error, writers)
	var wg sync.WaitGroup
	for i := 0; i < writers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			url := fmt.Sprintf("https://writer-%c.example.com/f", 'a'+i)
			errs[i] = downloadSingleWithAuthRetry(context.Background(), nil, url, nil, dest, false)
		}(i)
	}
	close(start)
	wg.Wait()

	var ok int
	for _, err := range errs {
		if err == nil {
			ok++
			continue
		}
		if !errors.Is(err, errDriveDownloadTargetExists) {
			t.Fatalf("并发失败方应为发布点原子拒绝, got %v", err)
		}
	}
	if ok != 1 {
		t.Fatalf("两个并发整流写者只能有一个成功, got %d (errs=%v)", ok, errs)
	}

	got, err := os.ReadFile(dest)
	if err != nil {
		t.Fatal(err)
	}
	wantA := bytes.Repeat([]byte("A"), size)
	wantB := bytes.Repeat([]byte("B"), size)
	if !bytes.Equal(got, wantA) && !bytes.Equal(got, wantB) {
		t.Fatalf("发布产物必须是某一写者的完整内容, got %d bytes head=%q", len(got), got[:8])
	}
	assertNoStreamTempLeftovers(t, dest)
}

// 分片锁独占（确定性）：持锁期间第二个写者被立即拒绝、不触碰任何断点
// 产物；锁释放后同一命令重跑成功且锁文件不残留。
func TestCrossPlatformCoverageRangedDownloadLockExcludesSecondProcess(t *testing.T) {
	content := makeTestContent(1000)
	srv := rangeTestServer(t, content, "", nil)
	defer srv.Close()

	dir := t.TempDir()
	dest := filepath.Join(dir, "locked.bin")

	// 预持锁，模拟另一进程正在写同一目标
	release, err := acquireDriveDownloadLock(dest)
	if err != nil {
		t.Fatalf("预取锁失败: %v", err)
	}

	opts := smallPartOpts(300, 3)
	opts.knownSize = 1000
	err = driveTransferDownload(context.Background(), nil, srv.URL, nil, dest, opts)
	if err == nil || !strings.Contains(err.Error(), "另一个下载进程") {
		t.Fatalf("持锁期间第二个写者应被拒绝: %v", err)
	}
	if _, statErr := os.Stat(dest); !os.IsNotExist(statErr) {
		t.Fatal("被拒绝的写者不得产出目标文件")
	}
	if _, statErr := os.Stat(dest + drivePartFileSuffix); !os.IsNotExist(statErr) {
		t.Fatal("被拒绝的写者不得触碰 .dwspart")
	}

	release()

	if err := driveTransferDownload(context.Background(), nil, srv.URL, nil, dest, opts); err != nil {
		t.Fatalf("锁释放后重跑应成功: %v", err)
	}
	verifyFile(t, dest, content)
	if _, statErr := os.Stat(dest + drivePartLockSuffix); !os.IsNotExist(statErr) {
		t.Fatal("下载完成后锁文件应被清理")
	}
}

// 分片并发写者：两个下载同时进入分片引擎，跨进程锁保证同一时刻只有一个
// 写者写 .dwspart；无论输家是撞锁还是稍后在发布点被原子拒绝，最终目标都
// 只能是完整的未混写内容。
func TestCrossPlatformCoverageConcurrentRangedWritersSingleIntactPublish(t *testing.T) {
	content := makeTestContent(1200)
	srv := rangeTestServer(t, content, "", nil)
	defer srv.Close()

	dir := t.TempDir()
	dest := filepath.Join(dir, "race.bin")
	opts := smallPartOpts(300, 2)
	opts.knownSize = 1200

	const writers = 2
	start := make(chan struct{})
	errs := make([]error, writers)
	var wg sync.WaitGroup
	for i := 0; i < writers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			errs[i] = driveTransferDownload(context.Background(), nil, srv.URL, nil, dest, opts)
		}(i)
	}
	close(start)
	wg.Wait()

	var ok int
	for _, err := range errs {
		if err == nil {
			ok++
		}
	}
	if ok != 1 {
		t.Fatalf("两个并发分片写者只能有一个成功, got %d (errs=%v)", ok, errs)
	}
	verifyFile(t, dest, content)
	// 锁必须恒被释放（赢家与输家的 defer 都会清理）。
	if _, statErr := os.Stat(dest + drivePartLockSuffix); !os.IsNotExist(statErr) {
		t.Fatal("并发结束后锁文件不应残留")
	}
	// 输家若是在发布点被原子拒绝（串行完成但目标已存在），保留的 .dwspart
	// 供 --overwrite 重跑复用——它必须是同一请求的完整内容，绝不允许混写。
	if _, statErr := os.Stat(dest + drivePartFileSuffix); statErr == nil {
		verifyFile(t, dest+drivePartFileSuffix, content)
	}
}
