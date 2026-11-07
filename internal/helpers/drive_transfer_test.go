package helpers

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
)

// ──────────────────────────────────────────────────────────
// parsePartSize / formatByteSize
// ──────────────────────────────────────────────────────────

func TestCrossPlatformCoverageParsePartSize(t *testing.T) {
	cases := []struct {
		in      string
		want    int64
		wantErr bool
	}{
		{"16MB", 16 << 20, false},
		{"16M", 16 << 20, false},
		{"1GB", 1 << 30, false},
		{"1g", 1 << 30, false},
		{"1024KB", 1 << 20, false},
		{"33554432", 32 << 20, false}, // 纯数字按字节
		{" 8mb ", 8 << 20, false},
		{"", 0, true},
		{"abc", 0, true},
		{"-4MB", 0, true},
		{"0", 0, true},
		{"512KB", 0, true},  // 低于 1MB 下限
		{"2048MB", 0, true}, // 高于 1GB 上限
		{"1.5MB", 0, true},  // 不支持小数
	}
	for _, c := range cases {
		got, err := parsePartSize(c.in)
		if c.wantErr {
			if err == nil {
				t.Errorf("parsePartSize(%q) 应报错, got %d", c.in, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("parsePartSize(%q) unexpected error: %v", c.in, err)
			continue
		}
		if got != c.want {
			t.Errorf("parsePartSize(%q) = %d, want %d", c.in, got, c.want)
		}
	}
}

func TestCrossPlatformCoverageFormatByteSize(t *testing.T) {
	cases := map[int64]string{
		16 << 20: "16MB",
		1 << 30:  "1GB",
		4 << 10:  "4KB",
		123:      "123B",
	}
	for in, want := range cases {
		if got := formatByteSize(in); got != want {
			t.Errorf("formatByteSize(%d) = %q, want %q", in, got, want)
		}
	}
}

// ──────────────────────────────────────────────────────────
// splitDownloadParts
// ──────────────────────────────────────────────────────────

func TestCrossPlatformCoverageSplitDownloadParts(t *testing.T) {
	// 整除
	parts := splitDownloadParts(32, 16)
	if len(parts) != 2 || parts[0].offset != 0 || parts[0].length != 16 || parts[1].offset != 16 || parts[1].length != 16 {
		t.Errorf("整除切分错误: %+v", parts)
	}
	// 有余量：末片较短
	parts = splitDownloadParts(33, 16)
	if len(parts) != 3 || parts[2].offset != 32 || parts[2].length != 1 {
		t.Errorf("余量切分错误: %+v", parts)
	}
	// 单片
	parts = splitDownloadParts(10, 16)
	if len(parts) != 1 || parts[0].length != 10 {
		t.Errorf("单片切分错误: %+v", parts)
	}
	// 非法输入
	if splitDownloadParts(0, 16) != nil || splitDownloadParts(16, 0) != nil {
		t.Error("非法输入应返回 nil")
	}
	// 覆盖完整性
	parts = splitDownloadParts(100, 7)
	var sum int64
	for i, p := range parts {
		if p.index != i {
			t.Errorf("index 不连续: %+v", p)
		}
		sum += p.length
	}
	if sum != 100 {
		t.Errorf("分片总长 %d != 100", sum)
	}
}

// ──────────────────────────────────────────────────────────
// checkpoint 读写与恢复
// ──────────────────────────────────────────────────────────

func TestCrossPlatformCoverageCheckpointSaveLoadRoundtrip(t *testing.T) {
	dir := t.TempDir()
	metaPath := filepath.Join(dir, "f.bin.dwspart.meta")
	fp := driveDownloadFingerprint("test-node-1", 0, 100, "")
	cp := &driveDownloadCheckpoint{
		Version:     driveCheckpointVersion,
		Fingerprint: fp,
		TotalSize:   100,
		PartSize:    30,
		Completed:   []bool{true, false, true, false},
	}
	if err := cp.save(metaPath); err != nil {
		t.Fatalf("save: %v", err)
	}
	got := loadDriveDownloadCheckpoint(metaPath, fp, 100, 30, 4)
	if got == nil {
		t.Fatal("roundtrip 应成功加载")
	}
	if !got.Completed[0] || got.Completed[1] || !got.Completed[2] {
		t.Errorf("Completed 位图不符: %v", got.Completed)
	}
}

func TestCrossPlatformCoverageCheckpointLoadRejectsMismatch(t *testing.T) {
	dir := t.TempDir()
	metaPath := filepath.Join(dir, "f.meta")
	fp := driveDownloadFingerprint("test-node-2", 0, 100, "")
	cp := &driveDownloadCheckpoint{Version: driveCheckpointVersion, Fingerprint: fp, TotalSize: 100, PartSize: 30, Completed: make([]bool, 4)}
	if err := cp.save(metaPath); err != nil {
		t.Fatalf("save: %v", err)
	}
	if loadDriveDownloadCheckpoint(metaPath, "other-fp", 100, 30, 4) != nil {
		t.Error("指纹不符应作废")
	}
	if loadDriveDownloadCheckpoint(metaPath, fp, 200, 30, 4) != nil {
		t.Error("总长不符应作废")
	}
	if loadDriveDownloadCheckpoint(metaPath, fp, 100, 40, 4) != nil {
		t.Error("分片大小不符应作废")
	}
	if loadDriveDownloadCheckpoint(metaPath, fp, 100, 30, 5) != nil {
		t.Error("分片数不符应作废")
	}
	if loadDriveDownloadCheckpoint(filepath.Join(dir, "missing"), fp, 100, 30, 4) != nil {
		t.Error("文件缺失应返回 nil")
	}
	// 损坏 JSON
	if err := os.WriteFile(metaPath, []byte("{bad"), 0o644); err != nil {
		t.Fatal(err)
	}
	if loadDriveDownloadCheckpoint(metaPath, fp, 100, 30, 4) != nil {
		t.Error("损坏 JSON 应返回 nil")
	}
}

// 指纹基于 nodeID + version + totalSize，不同 nodeID / version / size 产生不同指纹。
func TestCrossPlatformCoverageFingerprintNodeIDBased(t *testing.T) {
	a := driveDownloadFingerprint("node-aaa", 0, 500, "")
	b := driveDownloadFingerprint("node-aaa", 0, 500, "")
	if a != b {
		t.Error("相同参数应产生相同指纹")
	}
	c := driveDownloadFingerprint("node-bbb", 0, 500, "")
	if a == c {
		t.Error("不同 nodeID 应产生不同指纹")
	}
	d := driveDownloadFingerprint("node-aaa", 0, 501, "")
	if a == d {
		t.Error("不同 totalSize 应产生不同指纹")
	}
	e := driveDownloadFingerprint("node-aaa", 2, 500, "")
	if a == e {
		t.Error("不同 version 应产生不同指纹")
	}
}

// checkpoint 指纹碰撞防护：相同输出路径、相同大小、不同 nodeID 不复用 checkpoint。
func TestCrossPlatformCoverageCheckpointNotReusedDifferentNodeID(t *testing.T) {
	dir := t.TempDir()
	metaPath := filepath.Join(dir, "f.meta")
	const totalSize int64 = 1000
	const partSize int64 = 300
	partCount := int((totalSize + partSize - 1) / partSize)

	// 用 nodeA 写入一个 checkpoint
	fpA := driveDownloadFingerprint("node-A", 0, totalSize, "")
	cp := &driveDownloadCheckpoint{
		Version:     driveCheckpointVersion,
		Fingerprint: fpA,
		TotalSize:   totalSize,
		PartSize:    partSize,
		Completed:   make([]bool, partCount),
	}
	cp.Completed[0] = true
	if err := cp.save(metaPath); err != nil {
		t.Fatal(err)
	}

	// 用 nodeB 尝试加载，应返回 nil（不复用）
	fpB := driveDownloadFingerprint("node-B", 0, totalSize, "")
	if loadDriveDownloadCheckpoint(metaPath, fpB, totalSize, partSize, partCount) != nil {
		t.Error("不同 nodeID 同大小不应复用 checkpoint")
	}
}

// checkpoint 指纹碰撞防护：相同 nodeID、不同大小不复用 checkpoint。
func TestCrossPlatformCoverageCheckpointNotReusedDifferentSize(t *testing.T) {
	dir := t.TempDir()
	metaPath := filepath.Join(dir, "f.meta")
	const partSize int64 = 300

	// 用 size=1000 写入 checkpoint
	fp1000 := driveDownloadFingerprint("same-node", 0, 1000, "")
	pc := int((int64(1000) + partSize - 1) / partSize)
	cp := &driveDownloadCheckpoint{
		Version:     driveCheckpointVersion,
		Fingerprint: fp1000,
		TotalSize:   1000,
		PartSize:    partSize,
		Completed:   make([]bool, pc),
	}
	if err := cp.save(metaPath); err != nil {
		t.Fatal(err)
	}

	// 用 size=2000 尝试加载，应返回 nil
	fp2000 := driveDownloadFingerprint("same-node", 0, 2000, "")
	pc2 := int((int64(2000) + partSize - 1) / partSize)
	if loadDriveDownloadCheckpoint(metaPath, fp2000, 2000, partSize, pc2) != nil {
		t.Error("相同 nodeID 不同大小不应复用 checkpoint")
	}
}

// checkpoint 正常续传：相同 nodeID + 相同大小复用 checkpoint。
func TestCrossPlatformCoverageCheckpointReusedSameNodeIDAndSize(t *testing.T) {
	dir := t.TempDir()
	metaPath := filepath.Join(dir, "f.meta")
	const totalSize int64 = 1000
	const partSize int64 = 300
	partCount := int((totalSize + partSize - 1) / partSize)

	fp := driveDownloadFingerprint("resume-node", 0, totalSize, "")
	cp := &driveDownloadCheckpoint{
		Version:     driveCheckpointVersion,
		Fingerprint: fp,
		TotalSize:   totalSize,
		PartSize:    partSize,
		Completed:   make([]bool, partCount),
	}
	cp.Completed[0] = true
	cp.Completed[2] = true
	if err := cp.save(metaPath); err != nil {
		t.Fatal(err)
	}

	// 用相同 nodeID + 相同大小加载，应成功复用
	got := loadDriveDownloadCheckpoint(metaPath, fp, totalSize, partSize, partCount)
	if got == nil {
		t.Fatal("相同 nodeID + 相同大小应复用 checkpoint")
	}
	if !got.Completed[0] || got.Completed[1] || !got.Completed[2] || got.Completed[3] {
		t.Errorf("Completed 位图不符: %v", got.Completed)
	}
}

// checkpoint 指纹碰撞防护：同 nodeID、同大小、version=0 但不同 resourceURL 不复用 checkpoint。
// 模拟最新版下载场景：文件被相同大小内容覆盖后 URL path 变化，旧 checkpoint 应作废。
func TestCrossPlatformCoverageCheckpointNotReusedDifferentResourceURL(t *testing.T) {
	dir := t.TempDir()
	metaPath := filepath.Join(dir, "f.meta")
	const totalSize int64 = 1000
	const partSize int64 = 300
	partCount := int((totalSize + partSize - 1) / partSize)

	// 模拟首次下载时的资源 URL（旧存储位置）
	oldURL := "https://storage.example.com/v1/files/abc123/content?token=xxx"
	fpOld := driveDownloadFingerprint("same-node", 0, totalSize, oldURL)
	cp := &driveDownloadCheckpoint{
		Version:     driveCheckpointVersion,
		Fingerprint: fpOld,
		TotalSize:   totalSize,
		PartSize:    partSize,
		Completed:   make([]bool, partCount),
	}
	cp.Completed[0] = true
	if err := cp.save(metaPath); err != nil {
		t.Fatal(err)
	}

	// 文件被覆盖后获得新的资源 URL（新存储位置，path 不同）
	newURL := "https://storage.example.com/v1/files/def456/content?token=yyy"
	fpNew := driveDownloadFingerprint("same-node", 0, totalSize, newURL)

	// 新旧指纹应不同
	if fpOld == fpNew {
		t.Fatal("不同 resourceURL 应产生不同指纹")
	}

	// 用新指纹加载旧 checkpoint 应返回 nil（不复用）
	if loadDriveDownloadCheckpoint(metaPath, fpNew, totalSize, partSize, partCount) != nil {
		t.Error("同 nodeID、同大小、version=0 但不同 resourceURL 不应复用 checkpoint")
	}
}

// 验证 resourceURL 为空时的安全降级：不影响其他字段的指纹计算。
func TestCrossPlatformCoverageFingerprintEmptyResourceURLFallback(t *testing.T) {
	// 空 URL 应产生确定性指纹
	a := driveDownloadFingerprint("node-x", 0, 500, "")
	b := driveDownloadFingerprint("node-x", 0, 500, "")
	if a != b {
		t.Error("空 resourceURL 时相同参数应产生相同指纹")
	}

	// 空 URL 与有 URL 的指纹应不同
	c := driveDownloadFingerprint("node-x", 0, 500, "https://example.com/path/to/file")
	if a == c {
		t.Error("空 URL 与有 URL 应产生不同指纹")
	}

	// version=0 时，仅 query 不同的 URL 应产生不同指纹（中心协议区分实际版本）
	d := driveDownloadFingerprint("node-x", 0, 500, "https://example.com/path/to/file?token=aaa")
	e := driveDownloadFingerprint("node-x", 0, 500, "https://example.com/path/to/file?token=bbb")
	if d == e {
		t.Error("version=0 时仅 query 不同应产生不同指纹")
	}

	// version>0 时，仅 query 不同的 URL 应产生相同指纹（只取 path）
	f := driveDownloadFingerprint("node-x", 3, 500, "https://example.com/path/to/file?token=aaa")
	g := driveDownloadFingerprint("node-x", 3, 500, "https://example.com/path/to/file?token=bbb")
	if f != g {
		t.Error("version>0 时仅 query 不同应产生相同指纹（只取 path）")
	}
}

// ──────────────────────────────────────────────────────────
// parseContentRangeTotal / parseDownloadFileSize / parseDriveUploadType
// ──────────────────────────────────────────────────────────

func TestCrossPlatformCoverageParseContentRangeTotal(t *testing.T) {
	if n, err := parseContentRangeTotal("bytes 0-0/12345"); err != nil || n != 12345 {
		t.Errorf("got %d, %v", n, err)
	}
	for _, bad := range []string{"", "bytes 0-0/*", "bytes 0-0/", "bytes 0-0/abc", "12345"} {
		if _, err := parseContentRangeTotal(bad); err == nil {
			t.Errorf("%q 应报错", bad)
		}
	}
}

func TestCrossPlatformCoverageParseContentRange(t *testing.T) {
	cases := []struct {
		in                            string
		wantStart, wantEnd, wantTotal int64
		wantErr                       bool
	}{
		{"bytes 0-1048575/104857600", 0, 1048575, 104857600, false},
		{"bytes 500-999/2000", 500, 999, 2000, false},
		{"bytes 0-0/1", 0, 0, 1, false},
		{"bytes 100-199/*", 100, 199, -1, false}, // total 未知
		{"", 0, 0, 0, true},                      // 空串
		{"0-100/200", 0, 0, 0, true},             // 无 "bytes " 前缀
		{"bytes 0-100", 0, 0, 0, true},           // 缺少 /total
		{"bytes abc-100/200", 0, 0, 0, true},     // start 非数字
		{"bytes 0-abc/200", 0, 0, 0, true},       // end 非数字
		{"bytes 0-100/abc", 0, 0, 0, true},       // total 非数字
		{"bytes 100-50/200", 0, 0, 0, true},      // end < start
	}
	for _, c := range cases {
		start, end, total, err := parseContentRange(c.in)
		if c.wantErr {
			if err == nil {
				t.Errorf("parseContentRange(%q) 应报错", c.in)
			}
			continue
		}
		if err != nil {
			t.Errorf("parseContentRange(%q) unexpected error: %v", c.in, err)
			continue
		}
		if start != c.wantStart || end != c.wantEnd || total != c.wantTotal {
			t.Errorf("parseContentRange(%q) = (%d,%d,%d), want (%d,%d,%d)",
				c.in, start, end, total, c.wantStart, c.wantEnd, c.wantTotal)
		}
	}
}

func TestCrossPlatformCoverageParseDownloadFileSize(t *testing.T) {
	if n := parseDownloadFileSize(`{"result":{"fileSize":1048576,"downloadUrl":"u"}}`); n != 1048576 {
		t.Errorf("number: got %d", n)
	}
	if n := parseDownloadFileSize(`{"fileSize":"2048"}`); n != 2048 {
		t.Errorf("string: got %d", n)
	}
	if n := parseDownloadFileSize(`{"result":{}}`); n != 0 {
		t.Errorf("missing: got %d", n)
	}
	if n := parseDownloadFileSize("not json"); n != 0 {
		t.Errorf("invalid: got %d", n)
	}
}

func TestCrossPlatformCoverageParseDownloadFileVersion(t *testing.T) {
	// 数值类型
	if n := parseDownloadFileVersion(`{"result":{"version":3,"downloadUrl":"u"}}`); n != 3 {
		t.Errorf("number: got %d, want 3", n)
	}
	// 字符串类型
	if n := parseDownloadFileVersion(`{"version":"7"}`); n != 7 {
		t.Errorf("string: got %d, want 7", n)
	}
	// 缺失字段
	if n := parseDownloadFileVersion(`{"result":{}}`); n != 0 {
		t.Errorf("missing: got %d, want 0", n)
	}
	// 非法 JSON
	if n := parseDownloadFileVersion("not json"); n != 0 {
		t.Errorf("invalid json: got %d, want 0", n)
	}
	// 无 result 包裹的数值
	if n := parseDownloadFileVersion(`{"version":12}`); n != 12 {
		t.Errorf("top-level number: got %d, want 12", n)
	}
}

func TestCrossPlatformCoverageParseDriveUploadType(t *testing.T) {
	if got := parseDriveUploadType(`{"result":{"uploadType":"httpToCenterWithToken","resourceUrl":"u"}}`); got != uploadTypeCenterToken {
		t.Errorf("got %q", got)
	}
	if got := parseDriveUploadType(`{"resourceUrl":"u"}`); got != "" {
		t.Errorf("存量返回无 uploadType 应为空串, got %q", got)
	}
	if got := parseDriveUploadType("not json"); got != "" {
		t.Errorf("invalid json 应为空串, got %q", got)
	}
}

// ──────────────────────────────────────────────────────────
// isAuthStatusError / decorateUploadSizeError
// ──────────────────────────────────────────────────────────

func TestCrossPlatformCoverageIsAuthStatusError(t *testing.T) {
	if !isAuthStatusError(&httpStatusError{StatusCode: 401}) || !isAuthStatusError(&httpStatusError{StatusCode: 403}) {
		t.Error("typed 401/403 应命中")
	}
	if isAuthStatusError(&httpStatusError{StatusCode: 500}) {
		t.Error("typed 500 不应命中")
	}
	if !isAuthStatusError(fmt.Errorf("OSS upload failed: %w", &httpStatusError{StatusCode: 403, Body: "x"})) {
		t.Error("包装后的 typed 错误应命中")
	}
	if !isAuthStatusError(fmt.Errorf("HTTP 401: expired")) {
		t.Error("字符串形态应命中（测试注入兼容）")
	}
	if isAuthStatusError(nil) || isAuthStatusError(fmt.Errorf("HTTP 404: not found")) {
		t.Error("nil/404 不应命中")
	}
}

func TestCrossPlatformCoverageDecorateUploadSizeError(t *testing.T) {
	// 413 → 补充可读提示
	err := decorateUploadSizeError(&httpStatusError{StatusCode: 413, Body: "too large"}, "")
	if !strings.Contains(err.Error(), "提示") {
		t.Errorf("413 应补充提示: %v", err)
	}
	// 中心协议 + 超限语义 body
	err = decorateUploadSizeError(&httpStatusError{StatusCode: 400, Body: "file size exceed limit"}, uploadTypeCenterToken)
	if !strings.Contains(err.Error(), "提示") {
		t.Errorf("中心协议超限应补充提示: %v", err)
	}
	// 普通错误原样返回
	orig := &httpStatusError{StatusCode: 500, Body: "internal"}
	if got := decorateUploadSizeError(orig, uploadTypeCenterToken); got.Error() != orig.Error() {
		t.Errorf("普通错误不应装饰: %v", got)
	}
	nonHTTP := fmt.Errorf("dial timeout")
	if got := decorateUploadSizeError(nonHTTP, ""); got != nonHTTP {
		t.Errorf("非 HTTP 错误不应装饰: %v", got)
	}
}

// ──────────────────────────────────────────────────────────
// driveTransferDownload：整流 / 分片 / 回退 / 断点续传
// ──────────────────────────────────────────────────────────

// rangeTestServer 支持 Range 的测试服务端。
func rangeTestServer(t *testing.T, content []byte, requireToken string, tokenGen *atomic.Int32) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if requireToken != "" {
			want := requireToken
			if tokenGen != nil {
				want = fmt.Sprintf("%s-%d", requireToken, tokenGen.Load())
			}
			if r.Header.Get("dentry-token") != want {
				w.WriteHeader(http.StatusUnauthorized)
				fmt.Fprint(w, "token expired")
				return
			}
		}
		rng := r.Header.Get("Range")
		if rng == "" {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(content)
			return
		}
		var start, end int64
		if _, err := fmt.Sscanf(rng, "bytes=%d-%d", &start, &end); err != nil || start > end || start >= int64(len(content)) {
			w.WriteHeader(http.StatusRequestedRangeNotSatisfiable)
			return
		}
		if end >= int64(len(content)) {
			end = int64(len(content)) - 1
		}
		w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, end, len(content)))
		w.WriteHeader(http.StatusPartialContent)
		_, _ = w.Write(content[start : end+1])
	}))
}

func makeTestContent(n int) []byte {
	buf := make([]byte, n)
	for i := range buf {
		buf[i] = byte(i % 251)
	}
	return buf
}

func verifyFile(t *testing.T, path string, want []byte) {
	t.Helper()
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("读取产物失败: %v", err)
	}
	if len(got) != len(want) {
		t.Fatalf("产物长度 %d != %d", len(got), len(want))
	}
	for i := range got {
		if got[i] != want[i] {
			t.Fatalf("产物内容第 %d 字节不符", i)
		}
	}
}

// 小分片选项：partSize 用引擎内部值绕过 flag 校验（单测直接构造 options）。
func smallPartOpts(partSize int64, parallel int) driveDownloadOptions {
	return driveDownloadOptions{partSize: partSize, parallel: parallel, resume: true}
}

func TestCrossPlatformCoverageDriveTransferDownload_RangedParts(t *testing.T) {
	content := makeTestContent(1000)
	srv := rangeTestServer(t, content, "", nil)
	defer srv.Close()
	dest := filepath.Join(t.TempDir(), "out.bin")

	// knownSize=1000 ≥ 2×300 → 分片
	opts := smallPartOpts(300, 3)
	opts.knownSize = 1000
	if err := driveTransferDownload(context.Background(), nil, srv.URL, nil, dest, opts); err != nil {
		t.Fatalf("分片下载失败: %v", err)
	}
	verifyFile(t, dest, content)
	if _, err := os.Stat(dest + drivePartFileSuffix); !os.IsNotExist(err) {
		t.Error("完成后应清理 .dwspart")
	}
	if _, err := os.Stat(dest + drivePartMetaSuffix); !os.IsNotExist(err) {
		t.Error("完成后应清理 checkpoint")
	}
}

// knownSize 小于阈值 → 直接整流（走可注入 httpGetFile）。
func TestCrossPlatformCoverageDriveTransferDownload_SmallFileSingleStream(t *testing.T) {
	var calls atomic.Int32
	SetHTTPGetFile(func(ctx context.Context, url string, headers map[string]string, destPath string) error {
		calls.Add(1)
		return os.WriteFile(destPath, []byte("small"), 0o644)
	})
	defer SetHTTPGetFile(nil)

	dest := filepath.Join(t.TempDir(), "small.bin")
	opts := smallPartOpts(1<<20, 4)
	opts.knownSize = 100 // < 2MB 阈值
	if err := driveTransferDownload(context.Background(), nil, "https://x.example.com/f", nil, dest, opts); err != nil {
		t.Fatalf("整流下载失败: %v", err)
	}
	if calls.Load() != 1 {
		t.Errorf("应恰好调用一次 httpGetFile, got %d", calls.Load())
	}
	verifyFile(t, dest, []byte("small"))
}

// 整流 401 → 重取凭证重试一次。
func TestCrossPlatformCoverageDownloadSingleWithAuthRetry(t *testing.T) {
	var calls atomic.Int32
	SetHTTPGetFile(func(ctx context.Context, url string, headers map[string]string, destPath string) error {
		if calls.Add(1) == 1 {
			return &httpStatusError{StatusCode: 401, Body: "expired"}
		}
		if url != "https://new.example.com/f" || headers["dentry-token"] != "new-token" {
			return fmt.Errorf("重试应使用新凭证, got url=%s headers=%v", url, headers)
		}
		return os.WriteFile(destPath, []byte("ok"), 0o644)
	})
	defer SetHTTPGetFile(nil)

	fetched := false
	fetch := func(ctx context.Context) (string, map[string]string, int, error) {
		fetched = true
		return "https://new.example.com/f", map[string]string{"dentry-token": "new-token"}, 0, nil
	}
	dest := filepath.Join(t.TempDir(), "auth.bin")
	if err := downloadSingleWithAuthRetry(context.Background(), fetch, "https://old.example.com/f", nil, dest, false); err != nil {
		t.Fatalf("401 重试后应成功: %v", err)
	}
	if !fetched || calls.Load() != 2 {
		t.Errorf("fetched=%v calls=%d", fetched, calls.Load())
	}
	verifyFile(t, dest, []byte("ok"))
}

// 整流重取凭证后仍失败 → 走既有错误路径（返回错误）。
func TestCrossPlatformCoverageDownloadSingleWithAuthRetry_StillFails(t *testing.T) {
	SetHTTPGetFile(func(ctx context.Context, url string, headers map[string]string, destPath string) error {
		return &httpStatusError{StatusCode: 403, Body: "denied"}
	})
	defer SetHTTPGetFile(nil)
	fetch := func(ctx context.Context) (string, map[string]string, int, error) {
		return "https://new.example.com/f", nil, 0, nil
	}
	err := downloadSingleWithAuthRetry(context.Background(), fetch, "https://old.example.com/f", nil, filepath.Join(t.TempDir(), "x"), false)
	if err == nil || !isAuthStatusError(err) {
		t.Fatalf("应返回原始鉴权错误: %v", err)
	}
}

// 服务端不支持 Range（探测返回 200）→ 自动回退整流。
func TestCrossPlatformCoverageDriveTransferDownload_FallbackWhenNoRangeSupport(t *testing.T) {
	content := makeTestContent(900)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 忽略 Range 头，始终 200 全量
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(content)
	}))
	defer srv.Close()

	dest := filepath.Join(t.TempDir(), "fallback.bin")
	opts := smallPartOpts(300, 4)
	opts.knownSize = 900 // ≥ 阈值，进入探测
	if err := driveTransferDownload(context.Background(), nil, srv.URL, nil, dest, opts); err != nil {
		t.Fatalf("回退整流失败: %v", err)
	}
	verifyFile(t, dest, content)
}

// Content-Range 校验：正常匹配、区间错位、header 缺失
func TestCrossPlatformCoverageFetchRangeInto_ContentRangeValidation(t *testing.T) {
	content := makeTestContent(1000)

	// 正常情况：Content-Range 区间匹配
	t.Run("match", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var start, end int64
			fmt.Sscanf(r.Header.Get("Range"), "bytes=%d-%d", &start, &end)
			w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, end, len(content)))
			w.WriteHeader(http.StatusPartialContent)
			_, _ = w.Write(content[start : end+1])
		}))
		defer srv.Close()

		f, err := os.CreateTemp(t.TempDir(), "cr-match-*")
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close()
		if err := f.Truncate(1000); err != nil {
			t.Fatal(err)
		}
		part := driveDownloadPart{index: 1, offset: 300, length: 300}
		if err := fetchRangeInto(context.Background(), srv.URL, nil, f, part, 0); err != nil {
			t.Fatalf("正常匹配不应报错: %v", err)
		}
	})

	// Content-Range 区间错位（start 不匹配）→ 返回错误
	t.Run("mismatch_start", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// 请求 300-599，但返回假装的 0-299
			w.Header().Set("Content-Range", fmt.Sprintf("bytes 0-299/%d", len(content)))
			w.WriteHeader(http.StatusPartialContent)
			_, _ = w.Write(content[0:300])
		}))
		defer srv.Close()

		f, err := os.CreateTemp(t.TempDir(), "cr-mismatch-*")
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close()
		if err := f.Truncate(1000); err != nil {
			t.Fatal(err)
		}
		part := driveDownloadPart{index: 1, offset: 300, length: 300}
		err = fetchRangeInto(context.Background(), srv.URL, nil, f, part, 0)
		if err == nil {
			t.Fatal("Content-Range 错位应返回错误")
		}
		if !strings.Contains(err.Error(), "不匹配") {
			t.Errorf("错误信息应包含'不匹配': %v", err)
		}
	})

	// Content-Range header 缺失 → 强制报错，拒绝无法验证偏移一致性的分片
	t.Run("missing_header", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var start, end int64
			fmt.Sscanf(r.Header.Get("Range"), "bytes=%d-%d", &start, &end)
			// 不设置 Content-Range header
			w.WriteHeader(http.StatusPartialContent)
			_, _ = w.Write(content[start : end+1])
		}))
		defer srv.Close()

		f, err := os.CreateTemp(t.TempDir(), "cr-missing-*")
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close()
		if err := f.Truncate(1000); err != nil {
			t.Fatal(err)
		}
		part := driveDownloadPart{index: 0, offset: 0, length: 300}
		err = fetchRangeInto(context.Background(), srv.URL, nil, f, part, 0)
		if err == nil {
			t.Fatal("Content-Range 缺失应返回错误")
		}
		if !strings.Contains(err.Error(), "缺少 Content-Range") {
			t.Errorf("错误信息应包含'缺少 Content-Range': %v", err)
		}
	})

	// Content-Range 格式异常 → 返回错误
	t.Run("malformed_header", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Range", "invalid-format")
			w.WriteHeader(http.StatusPartialContent)
			_, _ = w.Write(content[0:300])
		}))
		defer srv.Close()

		f, err := os.CreateTemp(t.TempDir(), "cr-malformed-*")
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close()
		if err := f.Truncate(1000); err != nil {
			t.Fatal(err)
		}
		part := driveDownloadPart{index: 0, offset: 0, length: 300}
		err = fetchRangeInto(context.Background(), srv.URL, nil, f, part, 0)
		if err == nil {
			t.Fatal("Content-Range 格式异常应返回错误")
		}
		if !strings.Contains(err.Error(), "解析失败") {
			t.Errorf("错误信息应包含'解析失败': %v", err)
		}
	})
}

// 分片过程 401 → single-flight 刷新凭证后续传（不重下已完成分片）。
func TestCrossPlatformCoverageDriveTransferDownload_AuthRefreshDuringParts(t *testing.T) {
	content := makeTestContent(1200)
	var tokenGen atomic.Int32 // 服务端当前有效 token 代数
	srv := rangeTestServer(t, content, "tok", &tokenGen)
	defer srv.Close()

	var fetchCalls atomic.Int32
	fetch := func(ctx context.Context) (string, map[string]string, int, error) {
		fetchCalls.Add(1)
		return srv.URL, map[string]string{"dentry-token": fmt.Sprintf("tok-%d", tokenGen.Load())}, 5, nil
	}
	dest := filepath.Join(t.TempDir(), "auth-parts.bin")
	opts := smallPartOpts(300, 2)
	opts.knownSize = 1200
	opts.version = 5

	// 初始凭证是第 0 代；探测后服务端轮换到第 1 代，分片请求将收到 401
	initial := map[string]string{"dentry-token": "tok-0"}
	go func() {
		// 探测完成前不轮换：用探测本身消耗第 0 代，随后轮换
		tokenGen.Store(1)
	}()
	if err := driveTransferDownload(context.Background(), fetch, srv.URL, initial, dest, opts); err != nil {
		t.Fatalf("凭证刷新续传失败: %v", err)
	}
	verifyFile(t, dest, content)
}

// 断点续传：模拟部分分片完成后中断，重跑跳过已完成分片。
func TestCrossPlatformCoverageDriveTransferDownload_ResumeSkipsCompletedParts(t *testing.T) {
	content := makeTestContent(1000)
	partSize := int64(300)
	dest := filepath.Join(t.TempDir(), "resume.bin")
	partPath := dest + drivePartFileSuffix
	metaPath := dest + drivePartMetaSuffix

	// 预置：分片 0、2 已完成（写入正确数据），1、3 未完成
	pre := make([]byte, 1000)
	copy(pre[0:300], content[0:300])
	copy(pre[600:900], content[600:900])
	if err := os.WriteFile(partPath, pre, 0o644); err != nil {
		t.Fatal(err)
	}
	const resumeNodeID = "resume-test-node"

	// 服务端记录收到的 Range 区间
	var mu struct {
		ranges []string
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rng := r.Header.Get("Range")
		mu.ranges = append(mu.ranges, rng)
		var start, end int64
		fmt.Sscanf(rng, "bytes=%d-%d", &start, &end)
		if end >= int64(len(content)) {
			end = int64(len(content)) - 1
		}
		w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, end, len(content)))
		w.WriteHeader(http.StatusPartialContent)
		_, _ = w.Write(content[start : end+1])
	}))
	defer srv.Close()

	cp := &driveDownloadCheckpoint{
		Version:     driveCheckpointVersion,
		Fingerprint: driveDownloadFingerprint(resumeNodeID, 0, 1000, srv.URL),
		TotalSize:   1000,
		PartSize:    partSize,
		Completed:   []bool{true, false, true, false},
	}
	if err := cp.save(metaPath); err != nil {
		t.Fatal(err)
	}

	opts := smallPartOpts(partSize, 1) // 串行便于断言
	opts.knownSize = 1000
	opts.nodeID = resumeNodeID
	if err := driveTransferDownload(context.Background(), nil, srv.URL, nil, dest, opts); err != nil {
		t.Fatalf("续传失败: %v", err)
	}
	verifyFile(t, dest, content)

	// 探测请求(bytes=0-0) + 分片1(300-599) + 分片3(900-999)，不应重下分片0/2
	for _, rng := range mu.ranges {
		if rng == fmt.Sprintf("bytes=%d-%d", 0, partSize-1) || rng == fmt.Sprintf("bytes=%d-%d", 2*partSize, 3*partSize-1) {
			t.Errorf("已完成分片被重下: %s", rng)
		}
	}
}

// --no-resume：清理历史断点从头下载，且过程中不写 checkpoint。
func TestCrossPlatformCoverageDriveTransferDownload_NoResume(t *testing.T) {
	content := makeTestContent(1000)
	srv := rangeTestServer(t, content, "", nil)
	defer srv.Close()

	dest := filepath.Join(t.TempDir(), "noresume.bin")
	// 预置一个陈旧 checkpoint（若未清理会因 Completed 全 true 跳过下载产出错误内容）
	stale := &driveDownloadCheckpoint{
		Version:     driveCheckpointVersion,
		Fingerprint: driveDownloadFingerprint("stale-node", 0, 1000, ""),
		TotalSize:   1000,
		PartSize:    300,
		Completed:   []bool{true, true, true, true},
	}
	if err := os.WriteFile(dest+drivePartFileSuffix, make([]byte, 1000), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := stale.save(dest + drivePartMetaSuffix); err != nil {
		t.Fatal(err)
	}

	opts := driveDownloadOptions{partSize: 300, parallel: 2, resume: false, knownSize: 1000}
	if err := driveTransferDownload(context.Background(), nil, srv.URL, nil, dest, opts); err != nil {
		t.Fatalf("no-resume 下载失败: %v", err)
	}
	verifyFile(t, dest, content) // 内容正确说明确实重下而非采信陈旧 checkpoint
	if _, err := os.Stat(dest + drivePartMetaSuffix); !os.IsNotExist(err) {
		t.Error("no-resume 不应遗留 checkpoint")
	}
}

// 分片失败重试后成功（指数退避路径）。
func TestCrossPlatformCoverageDownloadOnePart_RetryOnTransientError(t *testing.T) {
	content := makeTestContent(600)
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if hits.Add(1) <= 2 { // 前两次 500
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		rng := r.Header.Get("Range")
		var start, end int64
		fmt.Sscanf(rng, "bytes=%d-%d", &start, &end)
		w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, end, len(content)))
		w.WriteHeader(http.StatusPartialContent)
		_, _ = w.Write(content[start : end+1])
	}))
	defer srv.Close()

	dir := t.TempDir()
	f, err := os.Create(filepath.Join(dir, "part.tmp"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := f.Truncate(600); err != nil {
		t.Fatal(err)
	}
	creds := &driveCredentialState{url: srv.URL}
	if err := downloadOnePart(context.Background(), creds, f, driveDownloadPart{index: 0, offset: 0, length: 600}, 0); err != nil {
		t.Fatalf("瞬时错误重试后应成功: %v", err)
	}
	if hits.Load() != 3 {
		t.Errorf("应请求 3 次, got %d", hits.Load())
	}
}

// 分片持续失败超过重试上限 → 返回错误。
func TestCrossPlatformCoverageDownloadOnePart_ExhaustsRetries(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	f, err := os.Create(filepath.Join(t.TempDir(), "part.tmp"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	creds := &driveCredentialState{url: srv.URL}
	err = downloadOnePart(context.Background(), creds, f, driveDownloadPart{index: 0, offset: 0, length: 10}, 0)
	if err == nil {
		t.Fatal("超过重试上限应失败")
	}
	if hits.Load() != int32(driveDownloadPartRetries)+1 {
		t.Errorf("应请求 %d 次, got %d", driveDownloadPartRetries+1, hits.Load())
	}
}

// ──────────────────────────────────────────────────────────
// driveUploadPut：中心协议 PUT + 401 重取凭证重试
// ──────────────────────────────────────────────────────────

func TestCrossPlatformCoverageDriveUploadPut_Success(t *testing.T) {
	var gotURL string
	var gotHeaders map[string]string
	SetHTTPPutFile(func(ctx context.Context, url string, headers map[string]string, filePath string, fileSize int64) error {
		gotURL, gotHeaders = url, headers
		return nil
	})
	defer SetHTTPPutFile(nil)

	cred := `{"result":{"uploadType":"httpToCenterWithToken","resourceUrl":"https://c.example.com/attachment/token/upload/single?file_size=5&spaceId=s1&upload_key=k1","uploadId":"k1","headers":{"dentry-token":"tk"}}}`
	uploadID, err := driveUploadPut(context.Background(), cred, nil, "/tmp/f.bin", 5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if uploadID != "k1" {
		t.Errorf("uploadID = %q", uploadID)
	}
	if !strings.Contains(gotURL, "upload_key=k1") {
		t.Errorf("应使用服务端拼好的完整 URL: %q", gotURL)
	}
	if gotHeaders["dentry-token"] != "tk" {
		t.Errorf("headers 应透传 dentry-token: %v", gotHeaders)
	}
}

func TestCrossPlatformCoverageDriveUploadPut_AuthRetryWithNewCredential(t *testing.T) {
	var calls atomic.Int32
	SetHTTPPutFile(func(ctx context.Context, url string, headers map[string]string, filePath string, fileSize int64) error {
		if calls.Add(1) == 1 {
			return &httpStatusError{StatusCode: 401, Body: "token expired"}
		}
		if headers["dentry-token"] != "tk2" {
			return fmt.Errorf("重试应使用新 token, got %v", headers)
		}
		return nil
	})
	defer SetHTTPPutFile(nil)

	cred1 := `{"result":{"uploadType":"httpToCenterWithToken","resourceUrl":"https://c.example.com/u?upload_key=k1","uploadId":"k1","headers":{"dentry-token":"tk1"}}}`
	cred2 := `{"result":{"uploadType":"httpToCenterWithToken","resourceUrl":"https://c.example.com/u?upload_key=k2","uploadId":"k2","headers":{"dentry-token":"tk2"}}}`
	refetch := func(ctx context.Context) (string, error) { return cred2, nil }

	uploadID, err := driveUploadPut(context.Background(), cred1, refetch, "/tmp/f.bin", 5)
	if err != nil {
		t.Fatalf("401 重试后应成功: %v", err)
	}
	if uploadID != "k2" {
		t.Errorf("应返回新凭证的 uploadId, got %q", uploadID)
	}
	if calls.Load() != 2 {
		t.Errorf("PUT 应调用 2 次, got %d", calls.Load())
	}
}

func TestCrossPlatformCoverageDriveUploadPut_AuthRetryStillFails(t *testing.T) {
	SetHTTPPutFile(func(ctx context.Context, url string, headers map[string]string, filePath string, fileSize int64) error {
		return &httpStatusError{StatusCode: 403, Body: "denied"}
	})
	defer SetHTTPPutFile(nil)

	cred := `{"result":{"resourceUrl":"https://c.example.com/u","uploadId":"k1"}}`
	refetch := func(ctx context.Context) (string, error) { return cred, nil }
	if _, err := driveUploadPut(context.Background(), cred, refetch, "/tmp/f.bin", 5); err == nil {
		t.Fatal("重试仍失败应返回错误")
	}
}

func TestCrossPlatformCoverageDriveUploadPut_SizeLimitHint(t *testing.T) {
	SetHTTPPutFile(func(ctx context.Context, url string, headers map[string]string, filePath string, fileSize int64) error {
		return &httpStatusError{StatusCode: 413, Body: "request entity too large"}
	})
	defer SetHTTPPutFile(nil)

	cred := `{"result":{"uploadType":"httpToCenterWithToken","resourceUrl":"https://c.example.com/u","uploadId":"k1","headers":{"dentry-token":"tk"}}}`
	_, err := driveUploadPut(context.Background(), cred, nil, "/tmp/f.bin", 5)
	if err == nil || !strings.Contains(err.Error(), "提示") {
		t.Fatalf("超限错误应含可读提示: %v", err)
	}
}

// ──────────────────────────────────────────────────────────
// parseDriveDownloadInfo：中心协议 headers 透传（新增行为）
// ──────────────────────────────────────────────────────────

func TestCrossPlatformCoverageParseDriveDownloadInfo_CenterProtocolHeaders(t *testing.T) {
	text := `{"result":{"downloadType":"httpToCenterWithToken","downloadUrl":"https://c.example.com/attachment/token/mdown?k=v","headers":{"dentry-token":"tk"},"fileName":"a.bin"},"success":true}`
	url, headers, err := parseDriveDownloadInfo(text)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if url != "https://c.example.com/attachment/token/mdown?k=v" {
		t.Errorf("url = %q", url)
	}
	if headers["dentry-token"] != "tk" {
		t.Errorf("中心协议应透传 dentry-token, got %v", headers)
	}
}

// ──────────────────────────────────────────────────────────
// driveCredentialState：single-flight 刷新
// ──────────────────────────────────────────────────────────

func TestCrossPlatformCoverageDriveCredentialStateSingleFlightRefresh(t *testing.T) {
	var fetches atomic.Int32
	cs := &driveCredentialState{
		url:            "u0",
		initialVersion: 5,
		fetch: func(ctx context.Context) (string, map[string]string, int, error) {
			n := fetches.Add(1)
			return fmt.Sprintf("u%d", n), nil, 5, nil
		},
	}
	_, _, gen0 := cs.current()
	// 两个并发分片都持第 0 代请求刷新：只应真正 fetch 一次
	if err := cs.refresh(context.Background(), gen0); err != nil {
		t.Fatal(err)
	}
	if err := cs.refresh(context.Background(), gen0); err != nil {
		t.Fatal(err)
	}
	if fetches.Load() != 1 {
		t.Errorf("同代刷新应 single-flight, fetch 次数 = %d", fetches.Load())
	}
	url, _, gen1 := cs.current()
	if url != "u1" || gen1 != gen0+1 {
		t.Errorf("刷新后 url=%q gen=%d", url, gen1)
	}
	// 持新代再刷新 → 再 fetch 一次
	if err := cs.refresh(context.Background(), gen1); err != nil {
		t.Fatal(err)
	}
	if fetches.Load() != 2 {
		t.Errorf("新代刷新应真正执行, fetch 次数 = %d", fetches.Load())
	}
}

// refresh 无 fetcher → 返回错误
func TestCrossPlatformCoverageDriveCredentialStateRefreshNilFetch(t *testing.T) {
	cs := &driveCredentialState{url: "u0", fetch: nil}
	_, _, gen := cs.current()
	err := cs.refresh(context.Background(), gen)
	if err == nil || !strings.Contains(err.Error(), "无法自动刷新") {
		t.Errorf("fetch==nil 应报错, got %v", err)
	}
}

// refresh fetcher 返回 error → 传播给调用方
func TestCrossPlatformCoverageDriveCredentialStateRefreshFetchError(t *testing.T) {
	cs := &driveCredentialState{
		url: "u0",
		fetch: func(ctx context.Context) (string, map[string]string, int, error) {
			return "", nil, 0, fmt.Errorf("network")
		},
	}
	_, _, gen := cs.current()
	err := cs.refresh(context.Background(), gen)
	if err == nil || !strings.Contains(err.Error(), "network") {
		t.Errorf("fetch 报错应传播, got %v", err)
	}
}

// ──────────────────────────────────────────────────────────
// parsePartSize 补充分支
// ──────────────────────────────────────────────────────────

func TestCrossPlatformCoverageParsePartSize_ExtraSuffixes(t *testing.T) {
	// "K" 后缀
	if got, err := parsePartSize("1024K"); err != nil || got != 1<<20 {
		t.Errorf("1024K: got %d, err=%v", got, err)
	}
	// 裸 "B" 后缀（去 B 后当纯数字）
	if got, err := parsePartSize("16777216B"); err != nil || got != 16<<20 {
		t.Errorf("16777216B: got %d, err=%v", got, err)
	}
}

// ──────────────────────────────────────────────────────────
// parseContentRange 补充：负 start
// ──────────────────────────────────────────────────────────

func TestCrossPlatformCoverageParseContentRange_NegativeStart(t *testing.T) {
	_, _, _, err := parseContentRange("bytes -1-100/200")
	if err == nil {
		t.Error("负 start 应报错")
	}
}

// ──────────────────────────────────────────────────────────
// likelySizeLimitBody 补充中文关键词和 "size"+"over"/"size"+"limit"
// ──────────────────────────────────────────────────────────

func TestCrossPlatformCoverageLikelySizeLimitBody(t *testing.T) {
	cases := []struct {
		in   string
		want bool
	}{
		{"文件超限", true},
		{"超出配额", true},
		{"容量不足", true},
		{"file size over limit", true},
		{"Size Limit Exceeded", true},
		{"no error", false},
		{"", false},
	}
	for _, c := range cases {
		if got := likelySizeLimitBody(c.in); got != c.want {
			t.Errorf("likelySizeLimitBody(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}

// ──────────────────────────────────────────────────────────
// driveTransferDownload 默认 opts 和 probe 错误路径
// ──────────────────────────────────────────────────────────

// opts.partSize/parallel 为 0 时使用默认值
func TestCrossPlatformCoverageDriveTransferDownload_DefaultOpts(t *testing.T) {
	SetHTTPGetFile(func(ctx context.Context, url string, headers map[string]string, destPath string) error {
		return os.WriteFile(destPath, []byte("ok"), 0o644)
	})
	defer SetHTTPGetFile(nil)

	dest := filepath.Join(t.TempDir(), "default.bin")
	// knownSize < 2*defaultPartSize → 整流; partSize=0, parallel=0 用默认值
	opts := driveDownloadOptions{knownSize: 100}
	if err := driveTransferDownload(context.Background(), nil, "https://x.example.com/f", nil, dest, opts); err != nil {
		t.Fatalf("默认 opts 应正常: %v", err)
	}
}

// probeRangeSupport 返回 error → driveTransferDownload 传播
func TestCrossPlatformCoverageDriveTransferDownload_ProbeError(t *testing.T) {
	// 使用无效 URL 触发 probeRangeSupport 错误
	opts := driveDownloadOptions{partSize: 10, parallel: 2, knownSize: 100}
	err := driveTransferDownload(context.Background(), nil, "://invalid-url", nil, filepath.Join(t.TempDir(), "x"), opts)
	if err == nil {
		t.Fatal("无效 URL 的 probe 应失败")
	}
}

// probeRangeSupport 返回 totalSize 小于阈值 → 回退整流
func TestCrossPlatformCoverageDriveTransferDownload_ProbeTotalBelowThreshold(t *testing.T) {
	// 服务端返回 206 但 Content-Range 中的 total 很小
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Range", "bytes 0-0/5") // total=5 < 2*partSize=20
		w.WriteHeader(http.StatusPartialContent)
		_, _ = w.Write([]byte{0})
	}))
	defer srv.Close()

	var called atomic.Int32
	SetHTTPGetFile(func(ctx context.Context, url string, headers map[string]string, destPath string) error {
		called.Add(1)
		return os.WriteFile(destPath, []byte("small"), 0o644)
	})
	defer SetHTTPGetFile(nil)

	dest := filepath.Join(t.TempDir(), "small-probe.bin")
	opts := driveDownloadOptions{partSize: 10, parallel: 2, knownSize: 100}
	if err := driveTransferDownload(context.Background(), nil, srv.URL, nil, dest, opts); err != nil {
		t.Fatalf("totalSize < threshold 应回退整流: %v", err)
	}
	if called.Load() == 0 {
		t.Error("应走整流路径（httpGetFile）")
	}
}

// ──────────────────────────────────────────────────────────
// downloadSingleWithAuthRetry: fetch==nil、fetch 报错
// ──────────────────────────────────────────────────────────

func TestCrossPlatformCoverageDownloadSingleWithAuthRetry_NilFetch(t *testing.T) {
	SetHTTPGetFile(func(ctx context.Context, url string, headers map[string]string, destPath string) error {
		return &httpStatusError{StatusCode: 401, Body: "expired"}
	})
	defer SetHTTPGetFile(nil)
	// fetch==nil → 不重试，直接返回原始错误
	err := downloadSingleWithAuthRetry(context.Background(), nil, "https://old.example.com/f", nil, filepath.Join(t.TempDir(), "x"), false)
	if err == nil || !isAuthStatusError(err) {
		t.Fatalf("fetch==nil 应直接返回鉴权错误: %v", err)
	}
}

func TestCrossPlatformCoverageDownloadSingleWithAuthRetry_FetchError(t *testing.T) {
	var calls atomic.Int32
	SetHTTPGetFile(func(ctx context.Context, url string, headers map[string]string, destPath string) error {
		calls.Add(1)
		return &httpStatusError{StatusCode: 401, Body: "expired"}
	})
	defer SetHTTPGetFile(nil)
	// fetch 返回 error → 返回原始 httpGetFile 错误（非 fetch 错误）
	fetch := func(ctx context.Context) (string, map[string]string, int, error) {
		return "", nil, 0, fmt.Errorf("fetch failed")
	}
	err := downloadSingleWithAuthRetry(context.Background(), fetch, "https://old.example.com/f", nil, filepath.Join(t.TempDir(), "x"), false)
	if err == nil || !isAuthStatusError(err) {
		t.Fatalf("fetch 报错应返回原始鉴权错误: %v", err)
	}
	if calls.Load() != 1 {
		t.Errorf("httpGetFile 应只调用一次, got %d", calls.Load())
	}
}

// ──────────────────────────────────────────────────────────
// probeRangeSupport: 401 刷新失败、default 非 2xx、循环耗尽
// ──────────────────────────────────────────────────────────

func TestCrossPlatformCoverageProbeRangeSupport_AuthRefreshFail(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		fmt.Fprint(w, "expired")
	}))
	defer srv.Close()

	creds := &driveCredentialState{
		url: srv.URL,
		fetch: func(ctx context.Context) (string, map[string]string, int, error) {
			return "", nil, 0, fmt.Errorf("refresh-fail")
		},
	}
	_, _, err := probeRangeSupport(context.Background(), creds)
	if err == nil || !strings.Contains(err.Error(), "重新获取下载凭证失败") {
		t.Fatalf("401+refresh 失败应报凭证错误: %v", err)
	}
}

func TestCrossPlatformCoverageProbeRangeSupport_NonRetryableError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprint(w, "not found")
	}))
	defer srv.Close()

	creds := &driveCredentialState{url: srv.URL}
	_, _, err := probeRangeSupport(context.Background(), creds)
	if err == nil {
		t.Fatal("404 应返回错误")
	}
	var se *httpStatusError
	if !errors.As(err, &se) || se.StatusCode != 404 {
		t.Errorf("应返回 httpStatusError(404): %v", err)
	}
}

func TestCrossPlatformCoverageProbeRangeSupport_AuthExhausted(t *testing.T) {
	// 第一次 403 → refresh 成功 → 第二次仍 403 → attempt>0 返回 "下载凭证刷新后仍鉴权失败"
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusForbidden)
		fmt.Fprint(w, "denied")
	}))
	defer srv.Close()

	creds := &driveCredentialState{
		url:   srv.URL,
		fetch: func(ctx context.Context) (string, map[string]string, int, error) { return srv.URL, nil, 0, nil },
	}
	_, _, err := probeRangeSupport(context.Background(), creds)
	if err == nil {
		t.Fatal("持续 403 应报错")
	}
	if !strings.Contains(err.Error(), "下载凭证刷新后仍鉴权失败") {
		t.Fatalf("应返回凭证刷新失败错误: %v", err)
	}
	if hits.Load() != 2 {
		t.Errorf("应请求 2 次, got %d", hits.Load())
	}
}

func TestCrossPlatformCoverageProbeRangeSupport_InvalidURL(t *testing.T) {
	creds := &driveCredentialState{url: "://bad"}
	_, _, err := probeRangeSupport(context.Background(), creds)
	if err == nil {
		t.Fatal("无效 URL 应报错")
	}
}

func TestCrossPlatformCoverageProbeRangeSupport_NetworkError(t *testing.T) {
	// 使用已关闭的服务端模拟网络错误
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	srv.Close()

	creds := &driveCredentialState{url: srv.URL}
	_, _, err := probeRangeSupport(context.Background(), creds)
	if err == nil {
		t.Fatal("网络错误应传播")
	}
}

func TestCrossPlatformCoverageProbeRangeSupport_ContentRangeParseError(t *testing.T) {
	// 返回 206 但 Content-Range 无法解析 total → 返回 0, nil, nil
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Range", "bytes 0-0/*") // total 未知
		w.WriteHeader(http.StatusPartialContent)
		_, _ = w.Write([]byte{0})
	}))
	defer srv.Close()

	creds := &driveCredentialState{url: srv.URL}
	total, resp, err := probeRangeSupport(context.Background(), creds)
	if err != nil {
		t.Fatalf("Content-Range 异常不应报 error: %v", err)
	}
	if resp != nil {
		t.Error("不应返回 resp")
	}
	if total != 0 {
		t.Errorf("total 应为 0, got %d", total)
	}
}

// ──────────────────────────────────────────────────────────
// writeStreamToFile 错误路径
// ──────────────────────────────────────────────────────────

func TestCrossPlatformCoverageWriteStreamToFile_CreateError(t *testing.T) {
	// 写入不存在目录下的文件 → os.Create 失败
	err := writeStreamToFile(strings.NewReader("data"), "/no-such-dir/sub/file.bin", false)
	if err == nil {
		t.Fatal("不存在路径应失败")
	}
}

func TestCrossPlatformCoverageWriteStreamToFile_CopyError(t *testing.T) {
	dest := filepath.Join(t.TempDir(), "copy-err.bin")
	errReader := &errReaderHelper{err: fmt.Errorf("read broken")}
	err := writeStreamToFile(errReader, dest, false)
	if err == nil || !strings.Contains(err.Error(), "read broken") {
		t.Fatalf("io.Copy 错误应传播: %v", err)
	}
}

func TestCrossPlatformCoverageWriteStreamToFile_StreamCreateError(t *testing.T) {
	// 预创建临时文件后重开失败（被外部移除等）：错误必须传播并清理。
	testseam.Swap(t, &driveOsCreate, func(string) (*os.File, error) {
		return nil, fmt.Errorf("stream create failed")
	})
	dest := filepath.Join(t.TempDir(), "stream-create-err.bin")
	err := writeStreamToFile(strings.NewReader("data"), dest, false)
	if err == nil || !strings.Contains(err.Error(), "stream create failed") {
		t.Fatalf("os.Create 错误应传播: %v", err)
	}
	assertNoStreamTempLeftovers(t, dest)
}

type errReaderHelper struct{ err error }

func (r *errReaderHelper) Read(p []byte) (int, error) { return 0, r.err }

// ──────────────────────────────────────────────────────────
// checkpoint save 错误路径
// ──────────────────────────────────────────────────────────

func TestCrossPlatformCoverageCheckpointSave_WriteError(t *testing.T) {
	cp := &driveDownloadCheckpoint{
		Version: driveCheckpointVersion, Fingerprint: "fp", TotalSize: 100, PartSize: 30, Completed: []bool{true},
	}
	// 写入不存在的目录 → 失败
	err := cp.save("/no-such-dir/sub/meta.json")
	if err == nil {
		t.Fatal("不存在路径应失败")
	}
}

// ──────────────────────────────────────────────────────────
// downloadRangedParts: 文件操作失败、上下文取消、logf 分支
// ──────────────────────────────────────────────────────────

func TestCrossPlatformCoverageDownloadRangedParts_LockCreateError(t *testing.T) {
	creds := &driveCredentialState{url: "http://x.example.com"}
	// destPath 指向不存在的目录 → 锁文件创建失败
	dest := "/no-such-dir/sub/out.bin"
	opts := driveDownloadOptions{partSize: 10, parallel: 1, resume: false}
	err := downloadRangedParts(context.Background(), creds, dest, 100, opts)
	if err == nil || !strings.Contains(err.Error(), "锁文件") {
		t.Fatalf("锁创建失败应报错: %v", err)
	}
}

func TestCrossPlatformCoverageDownloadRangedParts_OpenFileError(t *testing.T) {
	creds := &driveCredentialState{url: "http://x.example.com"}
	// 锁可正常创建；partPath 被只读文件占用 → O_RDWR 打开失败（EACCES）
	dest := filepath.Join(t.TempDir(), "out.bin")
	partPath := dest + drivePartFileSuffix
	if err := os.WriteFile(partPath, nil, 0o444); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(partPath, 0o644) })
	// resume=true：不触发 --no-resume 清理分支，直接在 OpenFile 撞只读占用
	opts := driveDownloadOptions{partSize: 10, parallel: 1, resume: true}
	err := downloadRangedParts(context.Background(), creds, dest, 100, opts)
	if err == nil || !strings.Contains(err.Error(), "分片临时文件失败") {
		t.Fatalf("OpenFile 失败应报错: %v", err)
	}
	// 失败路径同样必须释放锁
	if _, statErr := os.Stat(dest + drivePartLockSuffix); !os.IsNotExist(statErr) {
		t.Fatal("失败后锁文件应被释放")
	}
}

func TestCrossPlatformCoverageDownloadRangedParts_ContextCancelDuringDispatch(t *testing.T) {
	started := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case started <- struct{}{}:
		default:
		}
		// 阻塞直到请求上下文被取消
		<-r.Context().Done()
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	creds := &driveCredentialState{url: srv.URL}
	dest := filepath.Join(t.TempDir(), "cancel-dispatch.bin")
	// 多分片、parallel=1 → worker 卡在第一个请求，dispatch 第二个时 select 触发 ctx.Done
	opts := driveDownloadOptions{partSize: 10, parallel: 1, resume: false, knownSize: 100}

	go func() {
		<-started // 确保第一个 HTTP 请求已发出
		cancel()
	}()

	err := downloadRangedParts(ctx, creds, dest, 100, opts)
	if err == nil {
		t.Fatal("context cancel 应返回错误")
	}
}

func TestCrossPlatformCoverageDownloadRangedParts_LogfBranches(t *testing.T) {
	content := makeTestContent(100)
	srv := rangeTestServer(t, content, "", nil)
	defer srv.Close()

	var logs []string
	dest := filepath.Join(t.TempDir(), "logf.bin")
	creds := &driveCredentialState{url: srv.URL}
	opts := driveDownloadOptions{
		partSize: 30, parallel: 2, resume: true, knownSize: 100,
		logf: func(format string, args ...any) {
			logs = append(logs, fmt.Sprintf(format, args...))
		},
	}
	if err := downloadRangedParts(context.Background(), creds, dest, 100, opts); err != nil {
		t.Fatalf("logf 测试下载失败: %v", err)
	}
	if len(logs) == 0 {
		t.Error("应有日志输出")
	}
	// 应包含"分片下载"字样（全新下载）
	found := false
	for _, l := range logs {
		if strings.Contains(l, "分片下载") {
			found = true
		}
	}
	if !found {
		t.Errorf("全新下载应有'分片下载'日志, got %v", logs)
	}
}

func TestCrossPlatformCoverageDownloadRangedParts_ResumeLogf(t *testing.T) {
	// 模拟断点续传场景的 logf 输出
	content := makeTestContent(100)
	srv := rangeTestServer(t, content, "", nil)
	defer srv.Close()

	dest := filepath.Join(t.TempDir(), "resume-logf.bin")
	partPath := dest + drivePartFileSuffix
	metaPath := dest + drivePartMetaSuffix

	// 预置分片数据文件和 checkpoint（分片 0 已完成）
	pre := make([]byte, 100)
	copy(pre[0:30], content[0:30])
	if err := os.WriteFile(partPath, pre, 0o644); err != nil {
		t.Fatal(err)
	}
	fp := driveDownloadFingerprint("", 0, 100, srv.URL)
	cp := &driveDownloadCheckpoint{
		Version: driveCheckpointVersion, Fingerprint: fp,
		TotalSize: 100, PartSize: 30, Completed: []bool{true, false, false, false},
	}
	if err := cp.save(metaPath); err != nil {
		t.Fatal(err)
	}

	var logs []string
	creds := &driveCredentialState{url: srv.URL}
	opts := driveDownloadOptions{
		partSize: 30, parallel: 2, resume: true, knownSize: 100,
		logf: func(format string, args ...any) {
			logs = append(logs, fmt.Sprintf(format, args...))
		},
	}
	if err := downloadRangedParts(context.Background(), creds, dest, 100, opts); err != nil {
		t.Fatalf("续传 logf 失败: %v", err)
	}
	// 应包含"断点续传"字样
	found := false
	for _, l := range logs {
		if strings.Contains(l, "断点续传") {
			found = true
		}
	}
	if !found {
		t.Errorf("续传应有'断点续传'日志, got %v", logs)
	}
}

// checkpoint 存在但分片文件缺失 → 重新从头下载
func TestCrossPlatformCoverageDownloadRangedParts_CheckpointPartFileMissing(t *testing.T) {
	content := makeTestContent(100)
	srv := rangeTestServer(t, content, "", nil)
	defer srv.Close()

	dest := filepath.Join(t.TempDir(), "cp-missing.bin")
	metaPath := dest + drivePartMetaSuffix

	// 只有 checkpoint 没有 partPath → cp 作废
	fp := driveDownloadFingerprint("", 0, 100, srv.URL)
	cp := &driveDownloadCheckpoint{
		Version: driveCheckpointVersion, Fingerprint: fp,
		TotalSize: 100, PartSize: 30, Completed: []bool{true, true, true, true},
	}
	if err := cp.save(metaPath); err != nil {
		t.Fatal(err)
	}

	creds := &driveCredentialState{url: srv.URL}
	opts := driveDownloadOptions{partSize: 30, parallel: 2, resume: true, knownSize: 100}
	if err := downloadRangedParts(context.Background(), creds, dest, 100, opts); err != nil {
		t.Fatalf("checkpoint 无分片文件应从头下载: %v", err)
	}
	verifyFile(t, dest, content)
}

// checkpoint 存在但分片文件大小不符 → 重新从头下载
func TestCrossPlatformCoverageDownloadRangedParts_CheckpointPartFileSizeMismatch(t *testing.T) {
	content := makeTestContent(100)
	srv := rangeTestServer(t, content, "", nil)
	defer srv.Close()

	dest := filepath.Join(t.TempDir(), "cp-size.bin")
	partPath := dest + drivePartFileSuffix
	metaPath := dest + drivePartMetaSuffix

	// partPath 大小错误（50 != 100）
	if err := os.WriteFile(partPath, make([]byte, 50), 0o644); err != nil {
		t.Fatal(err)
	}
	fp := driveDownloadFingerprint("", 0, 100, srv.URL)
	cp := &driveDownloadCheckpoint{
		Version: driveCheckpointVersion, Fingerprint: fp,
		TotalSize: 100, PartSize: 30, Completed: []bool{true, false, false, false},
	}
	if err := cp.save(metaPath); err != nil {
		t.Fatal(err)
	}

	creds := &driveCredentialState{url: srv.URL}
	opts := driveDownloadOptions{partSize: 30, parallel: 2, resume: true, knownSize: 100}
	if err := downloadRangedParts(context.Background(), creds, dest, 100, opts); err != nil {
		t.Fatalf("分片大小不符应从头下载: %v", err)
	}
	verifyFile(t, dest, content)
}

// ──────────────────────────────────────────────────────────
// downloadOnePart: context 取消、auth retry 失败
// ──────────────────────────────────────────────────────────

func TestCrossPlatformCoverageDownloadOnePart_ContextCanceled(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	f, _ := os.Create(filepath.Join(t.TempDir(), "ctx-cancel.tmp"))
	defer f.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // 立即取消

	creds := &driveCredentialState{url: srv.URL}
	err := downloadOnePart(ctx, creds, f, driveDownloadPart{index: 0, offset: 0, length: 10}, 0)
	if err == nil {
		t.Fatal("context 已取消应返回错误")
	}
}

func TestCrossPlatformCoverageDownloadOnePart_AuthRefreshFail(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusUnauthorized)
		fmt.Fprint(w, "expired")
	}))
	defer srv.Close()

	f, _ := os.Create(filepath.Join(t.TempDir(), "auth-fail.tmp"))
	defer f.Close()

	creds := &driveCredentialState{
		url: srv.URL,
		fetch: func(ctx context.Context) (string, map[string]string, int, error) {
			return "", nil, 0, fmt.Errorf("refresh-fail")
		},
	}
	err := downloadOnePart(context.Background(), creds, f, driveDownloadPart{index: 0, offset: 0, length: 10}, 0)
	if err == nil || !strings.Contains(err.Error(), "重新获取下载凭证失败") {
		t.Fatalf("auth refresh 失败应报凭证错误: %v", err)
	}
}

// ──────────────────────────────────────────────────────────
// fetchRangeInto: 网络错误、io.Copy 错误、短读
// ──────────────────────────────────────────────────────────

func TestCrossPlatformCoverageFetchRangeInto_InvalidURL(t *testing.T) {
	f, _ := os.Create(filepath.Join(t.TempDir(), "inv.tmp"))
	defer f.Close()
	err := fetchRangeInto(context.Background(), "://bad", nil, f, driveDownloadPart{offset: 0, length: 10}, 0)
	if err == nil {
		t.Fatal("无效 URL 应报错")
	}
}

func TestCrossPlatformCoverageFetchRangeInto_NetworkError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	srv.Close() // 关闭模拟网络错误

	f, _ := os.Create(filepath.Join(t.TempDir(), "net.tmp"))
	defer f.Close()
	err := fetchRangeInto(context.Background(), srv.URL, nil, f, driveDownloadPart{offset: 0, length: 10}, 0)
	if err == nil {
		t.Fatal("网络错误应传播")
	}
}

func TestCrossPlatformCoverageFetchRangeInto_ShortRead(t *testing.T) {
	// 服务端只返回 5 字节但分片期望 10 字节 → 短读错误
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Range", "bytes 0-9/100")
		w.WriteHeader(http.StatusPartialContent)
		_, _ = w.Write([]byte("short")) // 只有 5 字节
	}))
	defer srv.Close()

	f, _ := os.CreateTemp(t.TempDir(), "short-*")
	defer f.Close()
	if err := f.Truncate(100); err != nil {
		t.Fatal(err)
	}
	err := fetchRangeInto(context.Background(), srv.URL, nil, f, driveDownloadPart{offset: 0, length: 10}, 0)
	if err == nil || !strings.Contains(err.Error(), "分片长度不符") {
		t.Fatalf("短读应报错: %v", err)
	}
}

func TestCrossPlatformCoverageFetchRangeInto_CopyError(t *testing.T) {
	// 服务端在部分传输后关闭连接 → io.Copy 报错
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Range", "bytes 0-99/100")
		w.Header().Set("Content-Length", "100")
		w.WriteHeader(http.StatusPartialContent)
		// 只写入一部分然后 panic 来中断连接
		_, _ = w.Write([]byte("partial"))
		panic(http.ErrAbortHandler)
	}))
	defer srv.Close()

	f, _ := os.CreateTemp(t.TempDir(), "copy-err-*")
	defer f.Close()
	if err := f.Truncate(100); err != nil {
		t.Fatal(err)
	}
	err := fetchRangeInto(context.Background(), srv.URL, nil, f, driveDownloadPart{offset: 0, length: 100}, 0)
	// 应有 error（io.Copy 失败或短读）
	if err == nil {
		t.Fatal("连接中断应报错")
	}
}

// ──────────────────────────────────────────────────────────
// parseDriveDownloadInfo: 各种 fallback 路径
// ──────────────────────────────────────────────────────────

func TestCrossPlatformCoverageParseDriveDownloadInfo_InvalidJSON(t *testing.T) {
	_, _, err := parseDriveDownloadInfo("not json")
	if err == nil || !strings.Contains(err.Error(), "解析") {
		t.Fatalf("非法 JSON 应报错: %v", err)
	}
}

func TestCrossPlatformCoverageParseDriveDownloadInfo_ResourceUrlFallback(t *testing.T) {
	// downloadUrl 缺失，走 resourceUrl fallback
	text := `{"result":{"resourceUrl":"https://oss.example.com/signed"}}`
	url, headers, err := parseDriveDownloadInfo(text)
	if err != nil {
		t.Fatalf("resourceUrl fallback 应成功: %v", err)
	}
	if url != "https://oss.example.com/signed" {
		t.Errorf("url = %q", url)
	}
	if len(headers) != 0 {
		t.Errorf("无 headers 应为空 map: %v", headers)
	}
}

func TestCrossPlatformCoverageParseDriveDownloadInfo_ResourceUrlsFallback(t *testing.T) {
	// downloadUrl 和 resourceUrl 都缺失，走 resourceUrls 数组 fallback
	text := `{"result":{"resourceUrls":[{"url":"https://cdn.example.com/file","headers":{"x-custom":"val"}}]}}`
	url, headers, err := parseDriveDownloadInfo(text)
	if err != nil {
		t.Fatalf("resourceUrls fallback 应成功: %v", err)
	}
	if url != "https://cdn.example.com/file" {
		t.Errorf("url = %q", url)
	}
	if headers["x-custom"] != "val" {
		t.Errorf("headers 应包含 x-custom: %v", headers)
	}
}

func TestCrossPlatformCoverageParseDriveDownloadInfo_EmptyURL(t *testing.T) {
	// 所有 URL 字段都为空
	text := `{"result":{"fileName":"a.bin"}}`
	_, _, err := parseDriveDownloadInfo(text)
	if err == nil || !strings.Contains(err.Error(), "downloadUrl 为空") {
		t.Fatalf("所有 URL 缺失应报错: %v", err)
	}
}

// ──────────────────────────────────────────────────────────
// P1-1/P1-2 补充测试：中心协议解析与指纹唯一性
// ──────────────────────────────────────────────────────────

// TestCrossPlatformCoverageParseDriveDownloadInfo_ResourceUrlsPerURLHeaders
// 验证 resourceUrls[].headers 中的 dentry-token 被正确透传到最终 headers。
func TestCrossPlatformCoverageParseDriveDownloadInfo_ResourceUrlsPerURLHeaders(t *testing.T) {
	text := `{"result":{"resourceUrls":[{"url":"https://center.example.com/download/path","headers":{"dentry-token":"dt-abc123","x-oss-security":"sig"}}]}}`
	url, headers, err := parseDriveDownloadInfo(text)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if url != "https://center.example.com/download/path" {
		t.Errorf("url = %q, want center URL", url)
	}
	if headers["dentry-token"] != "dt-abc123" {
		t.Errorf("dentry-token 未正确透传: got %v", headers)
	}
	if headers["x-oss-security"] != "sig" {
		t.Errorf("x-oss-security 未正确透传: got %v", headers)
	}
}

// TestCrossPlatformCoverageParseDriveDownloadInfo_TopLevelAndPerURLHeadersMerge
// 验证顶层 headers 与 resourceUrls[].headers 合并（per-URL 覆盖顶层同名 key）。
func TestCrossPlatformCoverageParseDriveDownloadInfo_TopLevelAndPerURLHeadersMerge(t *testing.T) {
	text := `{"result":{"headers":{"x-top":"top-val","dentry-token":"old"},"resourceUrls":[{"url":"https://center.example.com/dl","headers":{"dentry-token":"new"}}]}}`
	_, headers, err := parseDriveDownloadInfo(text)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// 顶层 headers 应被读入
	if headers["x-top"] != "top-val" {
		t.Errorf("顶层 x-top 应保留: got %v", headers)
	}
	// per-URL headers 覆盖同名 key
	if headers["dentry-token"] != "new" {
		t.Errorf("per-URL dentry-token 应覆盖顶层: got %q", headers["dentry-token"])
	}
}

// TestCrossPlatformCoverageFingerprintVersion0DifferentURLs
// 验证 version=0 时不同完整 URL（含 query）产生不同指纹。
func TestCrossPlatformCoverageFingerprintVersion0DifferentURLs(t *testing.T) {
	urlA := "https://center.example.com/download/file?sign=aaa&expire=100"
	urlB := "https://center.example.com/download/file?sign=bbb&expire=200"
	urlC := "https://center.example.com/download/other-file?sign=aaa&expire=100"

	fpA := driveDownloadFingerprint("node-1", 0, 1024, urlA)
	fpB := driveDownloadFingerprint("node-1", 0, 1024, urlB)
	fpC := driveDownloadFingerprint("node-1", 0, 1024, urlC)

	// 同 path 不同 query → 不同指纹
	if fpA == fpB {
		t.Error("version=0: 同 path 不同 query 应产生不同指纹")
	}
	// 不同 path → 不同指纹
	if fpA == fpC {
		t.Error("version=0: 不同 path 应产生不同指纹")
	}
	// 相同 URL → 相同指纹
	fpA2 := driveDownloadFingerprint("node-1", 0, 1024, urlA)
	if fpA != fpA2 {
		t.Error("version=0: 相同 URL 应产生相同指纹")
	}
}

// TestCrossPlatformCoverageFingerprintVersionedStableAcrossResign
// 验证 version>0 时重签名（仅 query 变化）不影响指纹。
func TestCrossPlatformCoverageFingerprintVersionedStableAcrossResign(t *testing.T) {
	urlOld := "https://oss.example.com/files/abc/content?Signature=old&Expires=1"
	urlNew := "https://oss.example.com/files/abc/content?Signature=new&Expires=2"

	fpOld := driveDownloadFingerprint("node-2", 5, 2048, urlOld)
	fpNew := driveDownloadFingerprint("node-2", 5, 2048, urlNew)

	if fpOld != fpNew {
		t.Error("version>0: 重签名不应改变指纹（只取 path）")
	}
}

// ──────────────────────────────────────────────────────────
// driveTransferDownload: 整合 fetch 在 totalSize<threshold 路径
// ──────────────────────────────────────────────────────────

func TestCrossPlatformCoverageDriveTransferDownload_ProbeTotalBelowThreshold_WithFetchNil(t *testing.T) {
	// 服务端返回 206 + total < threshold → 回退整流且 fetch lambda 中 fetch==nil
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Range", "bytes 0-0/5")
		w.WriteHeader(http.StatusPartialContent)
		_, _ = w.Write([]byte{0})
	}))
	defer srv.Close()

	SetHTTPGetFile(func(ctx context.Context, url string, headers map[string]string, destPath string) error {
		return os.WriteFile(destPath, []byte("tiny"), 0o644)
	})
	defer SetHTTPGetFile(nil)

	dest := filepath.Join(t.TempDir(), "nil-fetch-threshold.bin")
	opts := driveDownloadOptions{partSize: 10, parallel: 2, knownSize: 100}
	// fetch=nil: 当 totalSize<threshold 整流时，401/403 不会再重取凭证
	if err := driveTransferDownload(context.Background(), nil, srv.URL, nil, dest, opts); err != nil {
		t.Fatalf("fetch==nil totalSize<threshold 应正常整流: %v", err)
	}
}

// 测试 unused import io
func TestCrossPlatformCoverageFetchRangeInto_HeadersPassThrough(t *testing.T) {
	content := makeTestContent(100)
	var gotHeaders http.Header
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotHeaders = r.Header
		var start, end int64
		fmt.Sscanf(r.Header.Get("Range"), "bytes=%d-%d", &start, &end)
		w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, end, len(content)))
		w.WriteHeader(http.StatusPartialContent)
		_, _ = w.Write(content[start : end+1])
	}))
	defer srv.Close()

	f, _ := os.CreateTemp(t.TempDir(), "hdr-*")
	defer f.Close()
	if err := f.Truncate(100); err != nil {
		t.Fatal(err)
	}
	headers := map[string]string{"dentry-token": "tk123", "x-custom": "val"}
	err := fetchRangeInto(context.Background(), srv.URL, headers, f, driveDownloadPart{offset: 0, length: 50}, 0)
	if err != nil {
		t.Fatalf("headers 透传测试失败: %v", err)
	}
	if gotHeaders.Get("dentry-token") != "tk123" || gotHeaders.Get("x-custom") != "val" {
		t.Errorf("headers 应透传: %v", gotHeaders)
	}
}

// 确保 io 和 errors 包的使用
var _ = io.Discard
var _ = errors.New

// ──────────────────────────────────────────────────────────
// downloadOnePart: auth retry 成功后 continue、backoff 期间 ctx 取消
// ──────────────────────────────────────────────────────────

func TestCrossPlatformCoverageDownloadOnePart_AuthRefreshSuccess(t *testing.T) {
	// 第一次 401 → refresh 成功 → 第二次成功
	content := makeTestContent(100)
	var tokenGen atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("dentry-token") != fmt.Sprintf("tok-%d", tokenGen.Load()) {
			w.WriteHeader(http.StatusUnauthorized)
			fmt.Fprint(w, "expired")
			return
		}
		var start, end int64
		fmt.Sscanf(r.Header.Get("Range"), "bytes=%d-%d", &start, &end)
		if end >= int64(len(content)) {
			end = int64(len(content)) - 1
		}
		w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, end, len(content)))
		w.WriteHeader(http.StatusPartialContent)
		_, _ = w.Write(content[start : end+1])
	}))
	defer srv.Close()

	// 初始 token 无效（gen=0），refresh 后更新为 gen=1
	tokenGen.Store(1)
	creds := &driveCredentialState{
		url:            srv.URL,
		headers:        map[string]string{"dentry-token": "tok-0"},
		initialVersion: 3,
		fetch: func(ctx context.Context) (string, map[string]string, int, error) {
			return srv.URL, map[string]string{"dentry-token": fmt.Sprintf("tok-%d", tokenGen.Load())}, 3, nil
		},
	}

	f, _ := os.CreateTemp(t.TempDir(), "auth-success-*")
	defer f.Close()
	if err := f.Truncate(100); err != nil {
		t.Fatal(err)
	}

	err := downloadOnePart(context.Background(), creds, f, driveDownloadPart{index: 0, offset: 0, length: 50}, 0)
	if err != nil {
		t.Fatalf("auth refresh 成功后应下载成功: %v", err)
	}
}

func TestCrossPlatformCoverageDownloadOnePart_ContextCancelDuringBackoff(t *testing.T) {
	// 模拟：第一次非 auth 错误（500）→ 进入退避等待（500ms）→ ctx 超时取消
	// 用短 context timeout 确保在退避期间触发 ctx.Done
	responded := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprint(w, "server error")
		select {
		case responded <- struct{}{}:
		default:
		}
	}))
	defer srv.Close()

	f, _ := os.Create(filepath.Join(t.TempDir(), "backoff-cancel.tmp"))
	defer f.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	creds := &driveCredentialState{url: srv.URL}

	go func() {
		<-responded // 等待第一次 HTTP 响应完成
		// 小延迟确保 client 已收到响应并进入 backoff select
		cancel()
	}()

	err := downloadOnePart(ctx, creds, f, driveDownloadPart{index: 0, offset: 0, length: 10}, 0)
	if err == nil {
		t.Fatal("退避期间 ctx 取消应返回错误")
	}
}

// ──────────────────────────────────────────────────────────
// downloadRangedParts: Truncate 失败、f.Sync 失败、workers 边界
// ──────────────────────────────────────────────────────────

func TestCrossPlatformCoverageDownloadRangedParts_WorkersCapToRemaining(t *testing.T) {
	// 预置 checkpoint 让大部分分片已完成 → remaining < parallel → workers 被限制
	content := makeTestContent(100)
	srv := rangeTestServer(t, content, "", nil)
	defer srv.Close()

	dest := filepath.Join(t.TempDir(), "cap-workers.bin")
	partPath := dest + drivePartFileSuffix
	metaPath := dest + drivePartMetaSuffix

	// 4 个分片中只剩 1 个未完成 → workers=min(4,1)=1
	pre := make([]byte, 100)
	copy(pre[0:30], content[0:30])
	copy(pre[30:60], content[30:60])
	copy(pre[60:90], content[60:90])
	if err := os.WriteFile(partPath, pre, 0o644); err != nil {
		t.Fatal(err)
	}
	fp := driveDownloadFingerprint("", 0, 100, srv.URL)
	cp := &driveDownloadCheckpoint{
		Version: driveCheckpointVersion, Fingerprint: fp,
		TotalSize: 100, PartSize: 30, Completed: []bool{true, true, true, false},
	}
	if err := cp.save(metaPath); err != nil {
		t.Fatal(err)
	}

	creds := &driveCredentialState{url: srv.URL}
	opts := driveDownloadOptions{partSize: 30, parallel: 4, resume: true, knownSize: 100}
	if err := downloadRangedParts(context.Background(), creds, dest, 100, opts); err != nil {
		t.Fatalf("workers cap 测试失败: %v", err)
	}
	verifyFile(t, dest, content)
}

// 所有分片已完成（remaining==0）→ workers clamped to 1、无分片下载、直接完成
func TestCrossPlatformCoverageDownloadRangedParts_AllCompleted(t *testing.T) {
	content := makeTestContent(100)
	srv := rangeTestServer(t, content, "", nil)
	defer srv.Close()

	dest := filepath.Join(t.TempDir(), "all-done.bin")
	partPath := dest + drivePartFileSuffix
	metaPath := dest + drivePartMetaSuffix

	// 预置完整的分片文件和全部完成的 checkpoint
	if err := os.WriteFile(partPath, content, 0o644); err != nil {
		t.Fatal(err)
	}
	fp := driveDownloadFingerprint("", 0, 100, srv.URL)
	cp := &driveDownloadCheckpoint{
		Version: driveCheckpointVersion, Fingerprint: fp,
		TotalSize: 100, PartSize: 30, Completed: []bool{true, true, true, true},
	}
	if err := cp.save(metaPath); err != nil {
		t.Fatal(err)
	}

	creds := &driveCredentialState{url: srv.URL}
	opts := driveDownloadOptions{partSize: 30, parallel: 4, resume: true, knownSize: 100}
	if err := downloadRangedParts(context.Background(), creds, dest, 100, opts); err != nil {
		t.Fatalf("全部完成应直接成功: %v", err)
	}
	verifyFile(t, dest, content)
}

// driveTransferDownload 中 totalSize<threshold 路径触发 401 重取凭证（fetch!=nil）
func TestCrossPlatformCoverageDriveTransferDownload_ProbeTotalBelowThreshold_FetchCalled(t *testing.T) {
	// 服务端返回 206 + total < threshold
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Range", "bytes 0-0/5")
		w.WriteHeader(http.StatusPartialContent)
		_, _ = w.Write([]byte{0})
	}))
	defer srv.Close()

	var getCalls atomic.Int32
	SetHTTPGetFile(func(ctx context.Context, url string, headers map[string]string, destPath string) error {
		if getCalls.Add(1) == 1 {
			return &httpStatusError{StatusCode: 401, Body: "expired"}
		}
		return os.WriteFile(destPath, []byte("ok"), 0o644)
	})
	defer SetHTTPGetFile(nil)

	var fetchCalls atomic.Int32
	fetch := func(ctx context.Context) (string, map[string]string, int, error) {
		fetchCalls.Add(1)
		return srv.URL, map[string]string{"x-tok": "new"}, 0, nil
	}

	dest := filepath.Join(t.TempDir(), "threshold-fetch.bin")
	opts := driveDownloadOptions{partSize: 10, parallel: 2, knownSize: 100}
	if err := driveTransferDownload(context.Background(), fetch, srv.URL, nil, dest, opts); err != nil {
		t.Fatalf("totalSize<threshold + 401 重取应成功: %v", err)
	}
	if fetchCalls.Load() == 0 {
		t.Error("应调用 fetch 重取凭证")
	}
}

// driveTransferDownload 中 totalSize<threshold 路径 fetch==nil 时 401 应直接报错
func TestCrossPlatformCoverageDriveTransferDownload_ProbeTotalBelowThreshold_FetchNilAuth(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Range", "bytes 0-0/5")
		w.WriteHeader(http.StatusPartialContent)
		_, _ = w.Write([]byte{0})
	}))
	defer srv.Close()

	SetHTTPGetFile(func(ctx context.Context, url string, headers map[string]string, destPath string) error {
		return &httpStatusError{StatusCode: 401, Body: "expired"}
	})
	defer SetHTTPGetFile(nil)

	dest := filepath.Join(t.TempDir(), "nil-fetch-auth.bin")
	opts := driveDownloadOptions{partSize: 10, parallel: 2, knownSize: 100}
	// fetch=nil → 401 时内部 lambda 中 fetch==nil 返回 "无法自动刷新" → downloadSingleWithAuthRetry 返回原始错误
	err := driveTransferDownload(context.Background(), nil, srv.URL, nil, dest, opts)
	if err == nil || !isAuthStatusError(err) {
		t.Fatalf("fetch==nil + 401 应返回鉴权错误: %v", err)
	}
}

// ──────────────────────────────────────────────────────────
// Cross-platform coverage: OS operation hook injection tests
// ──────────────────────────────────────────────────────────

func TestCrossPlatformCoverageDriveTransferSaveJsonMarshalFailure(t *testing.T) {
	origMarshal := driveJsonMarshal
	t.Cleanup(func() { driveJsonMarshal = origMarshal })
	driveJsonMarshal = func(v any) ([]byte, error) { return nil, errors.New("injected marshal failure") }

	cp := &driveDownloadCheckpoint{Version: 1, TotalSize: 100, PartSize: 10, Completed: []bool{true}}
	err := cp.save(filepath.Join(t.TempDir(), "test.meta"))
	if err == nil || !strings.Contains(err.Error(), "injected marshal failure") {
		t.Fatalf("json.Marshal 失败应传播: %v", err)
	}
}

func TestCrossPlatformCoverageDriveTransferSaveRenameFailure(t *testing.T) {
	origRename := driveOsRename
	t.Cleanup(func() { driveOsRename = origRename })
	driveOsRename = func(string, string) error { return errors.New("injected rename failure") }

	cp := &driveDownloadCheckpoint{Version: 1, TotalSize: 100, PartSize: 10, Completed: []bool{true}}
	err := cp.save(filepath.Join(t.TempDir(), "test.meta"))
	if err == nil || !strings.Contains(err.Error(), "injected rename failure") {
		t.Fatalf("rename 失败应传播: %v", err)
	}
}

func TestCrossPlatformCoverageDriveTransferTruncateFailure(t *testing.T) {
	origTruncate := driveFileTruncate
	t.Cleanup(func() { driveFileTruncate = origTruncate })
	driveFileTruncate = func(f *os.File, size int64) error { return errors.New("injected truncate failure") }

	content := makeTestContent(100)
	srv := rangeTestServer(t, content, "", nil)
	defer srv.Close()

	creds := &driveCredentialState{url: srv.URL}
	dest := filepath.Join(t.TempDir(), "truncate-fail.bin")
	opts := driveDownloadOptions{partSize: 30, parallel: 2, resume: false, knownSize: 100}
	err := downloadRangedParts(context.Background(), creds, dest, 100, opts)
	if err == nil || !strings.Contains(err.Error(), "预分配分片临时文件失败") {
		t.Fatalf("Truncate 失败应报错: %v", err)
	}
}

func TestCrossPlatformCoverageDriveTransferSyncFailure(t *testing.T) {
	origSync := driveFileSync
	t.Cleanup(func() { driveFileSync = origSync })
	driveFileSync = func(f *os.File) error { return errors.New("injected sync failure") }

	content := makeTestContent(100)
	srv := rangeTestServer(t, content, "", nil)
	defer srv.Close()

	creds := &driveCredentialState{url: srv.URL}
	dest := filepath.Join(t.TempDir(), "sync-fail.bin")
	opts := driveDownloadOptions{partSize: 30, parallel: 2, resume: false, knownSize: 100}
	err := downloadRangedParts(context.Background(), creds, dest, 100, opts)
	if err == nil || !strings.Contains(err.Error(), "injected sync failure") {
		t.Fatalf("Sync 失败应传播: %v", err)
	}
}

func TestCrossPlatformCoverageDriveTransferStatFailure(t *testing.T) {
	origStat := driveFileStat
	t.Cleanup(func() { driveFileStat = origStat })
	driveFileStat = func(f *os.File) (os.FileInfo, error) { return nil, errors.New("injected stat failure") }

	content := makeTestContent(100)
	srv := rangeTestServer(t, content, "", nil)
	defer srv.Close()

	creds := &driveCredentialState{url: srv.URL}
	dest := filepath.Join(t.TempDir(), "stat-fail.bin")
	opts := driveDownloadOptions{partSize: 30, parallel: 2, resume: false, knownSize: 100}
	err := downloadRangedParts(context.Background(), creds, dest, 100, opts)
	if err == nil || !strings.Contains(err.Error(), "injected stat failure") {
		t.Fatalf("Stat 失败应传播: %v", err)
	}
}

func TestCrossPlatformCoverageDriveTransferSizeMismatch(t *testing.T) {
	origStat := driveFileStat
	t.Cleanup(func() { driveFileStat = origStat })
	// 返回与 totalSize 不一致的文件信息
	driveFileStat = func(f *os.File) (os.FileInfo, error) {
		// 返回真实 stat，但我们在调用前先 truncate 文件为不同大小
		return f.Stat()
	}
	// 注入 Sync 后将文件 truncate 为不同大小
	origSync := driveFileSync
	driveFileSync = func(f *os.File) error {
		// Sync 成功后破坏文件大小
		if err := origSync(f); err != nil {
			return err
		}
		return f.Truncate(50) // 破坏大小，让 Stat 返回 50 != 100
	}
	t.Cleanup(func() { driveFileSync = origSync })

	content := makeTestContent(100)
	srv := rangeTestServer(t, content, "", nil)
	defer srv.Close()

	creds := &driveCredentialState{url: srv.URL}
	dest := filepath.Join(t.TempDir(), "size-mismatch.bin")
	opts := driveDownloadOptions{partSize: 30, parallel: 2, resume: false, knownSize: 100}
	err := downloadRangedParts(context.Background(), creds, dest, 100, opts)
	if err == nil || !strings.Contains(err.Error(), "下载完成但文件长度不符") {
		t.Fatalf("文件大小不符应报错: %v", err)
	}
}

func TestCrossPlatformCoverageDriveTransferFinalRenameFailure(t *testing.T) {
	origRename := driveOsRename
	t.Cleanup(func() { driveOsRename = origRename })
	// 只在最终重命名时失败（checkpoint save 也用 driveOsRename，所以禁用 resume 跳过）
	driveOsRename = func(src, dst string) error {
		if strings.HasSuffix(src, drivePartFileSuffix) {
			return errors.New("injected final rename failure")
		}
		return origRename(src, dst)
	}

	content := makeTestContent(100)
	srv := rangeTestServer(t, content, "", nil)
	defer srv.Close()

	creds := &driveCredentialState{url: srv.URL}
	dest := filepath.Join(t.TempDir(), "rename-fail.bin")
	opts := driveDownloadOptions{partSize: 30, parallel: 2, resume: false, knownSize: 100, overwrite: true}
	err := downloadRangedParts(context.Background(), creds, dest, 100, opts)
	if err == nil || !strings.Contains(err.Error(), "重命名下载文件失败") {
		t.Fatalf("最终 rename 失败应报错: %v", err)
	}
}

func TestCrossPlatformCoverageDriveTransferCheckpointSaveFailure(t *testing.T) {
	origRename := driveOsRename
	t.Cleanup(func() { driveOsRename = origRename })
	// checkpoint save 的 rename 失败（.tmp → .dwspart.meta）
	driveOsRename = func(src, dst string) error {
		if strings.HasSuffix(dst, drivePartMetaSuffix) {
			return errors.New("injected checkpoint rename failure")
		}
		return origRename(src, dst)
	}

	content := makeTestContent(100)
	srv := rangeTestServer(t, content, "", nil)
	defer srv.Close()

	creds := &driveCredentialState{url: srv.URL}
	dest := filepath.Join(t.TempDir(), "cp-save-fail.bin")
	// resume=true 才会在分片完成后调用 cp.save
	opts := driveDownloadOptions{partSize: 30, parallel: 1, resume: true, knownSize: 100}
	err := downloadRangedParts(context.Background(), creds, dest, 100, opts)
	if err == nil || !strings.Contains(err.Error(), "写入下载断点信息失败") {
		t.Fatalf("checkpoint save 失败应报错: %v", err)
	}
}

func TestCrossPlatformCoverageDriveTransferProbeAuthExhausted(t *testing.T) {
	// 服务端永远返回 401，刷新成功但第二次仍 401 → "下载凭证刷新后仍鉴权失败"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		fmt.Fprint(w, "always-unauthorized")
	}))
	defer srv.Close()

	creds := &driveCredentialState{
		url: srv.URL,
		fetch: func(ctx context.Context) (string, map[string]string, int, error) {
			// 刷新成功但服务端仍拒绝
			return srv.URL, nil, 0, nil
		},
	}
	_, _, err := probeRangeSupport(context.Background(), creds)
	if err == nil || !strings.Contains(err.Error(), "下载凭证刷新后仍鉴权失败") {
		t.Fatalf("连续 401 应报凭证刷新后失败: %v", err)
	}
}

func TestCrossPlatformCoverageDriveTransferParseContentRangeNoDash(t *testing.T) {
	// range 部分没有 dash → "非法 Content-Range 区间"
	_, _, _, err := parseContentRange("bytes 12345/67890")
	if err == nil || !strings.Contains(err.Error(), "非法 Content-Range 区间") {
		t.Fatalf("无 dash 应报区间错误: %v", err)
	}
}

func TestCrossPlatformCoverageDriveTransferDownloadOnePartBackoffCtxDone(t *testing.T) {
	// 服务端返回 500，触发重试退避；在退避等待中取消 context
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprint(w, "server error")
	}))
	defer srv.Close()

	f, _ := os.CreateTemp(t.TempDir(), "backoff-ctx-*")
	defer f.Close()
	if err := f.Truncate(100); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	// 第一次失败后，在退避 sleep 中取消
	go func() {
		for hits.Load() < 1 {
			// spin-wait
		}
		cancel()
	}()

	creds := &driveCredentialState{url: srv.URL}
	err := downloadOnePart(ctx, creds, f, driveDownloadPart{index: 0, offset: 0, length: 10}, 0)
	if err == nil {
		t.Fatal("退避中 ctx 取消应返回错误")
	}
}

func TestCrossPlatformCoverageDriveTransferFetchRangeIntoCopyError(t *testing.T) {
	// 服务端返回 206 但中断 body → io.Copy 错误
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Range", "bytes 0-9/100")
		w.Header().Set("Content-Length", "10")
		w.WriteHeader(http.StatusPartialContent)
		// 写入部分数据后立即关闭连接（触发 io.Copy 中 unexpected EOF）
		hj, ok := w.(http.Hijacker)
		if !ok {
			// 回退：写入少于期望的数据量，触发短读而非 io.Copy err
			_, _ = w.Write([]byte("par"))
			return
		}
		conn, buf, _ := hj.Hijack()
		_, _ = buf.WriteString("par")
		_ = buf.Flush()
		_ = conn.Close()
	}))
	defer srv.Close()

	f, _ := os.CreateTemp(t.TempDir(), "copy-err-*")
	defer f.Close()
	if err := f.Truncate(100); err != nil {
		t.Fatal(err)
	}

	err := fetchRangeInto(context.Background(), srv.URL, nil, f, driveDownloadPart{offset: 0, length: 10}, 0)
	// 应产生 io.Copy 错误或短读错误
	if err == nil {
		t.Fatal("中断的 body 应产生错误")
	}
}

func TestCrossPlatformCoverageDriveTransferWorkerCtxAlreadyCancelled(t *testing.T) {
	// 多 worker 场景：一个 worker 失败后 cancel，其他 worker 取到 part 时 runCtx 已取消
	var reqCount atomic.Int32
	blockFirst := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := reqCount.Add(1)
		if n == 1 {
			// 第一个请求阻塞一下，让其他 worker 有机会拿到 part
			<-blockFirst
		}
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprint(w, "error")
	}))
	defer srv.Close()

	// 在短暂延迟后释放第一个请求
	go func() {
		for reqCount.Load() < 2 {
			// spin-wait 直到有第二个请求进来
		}
		close(blockFirst)
	}()

	creds := &driveCredentialState{url: srv.URL}
	dest := filepath.Join(t.TempDir(), "worker-ctx.bin")
	// 大量分片，多 worker：确保某个 worker 失败后其他 worker 在取 part 时触发 runCtx.Err()
	opts := driveDownloadOptions{partSize: 5, parallel: 4, resume: false, knownSize: 100}
	err := downloadRangedParts(context.Background(), creds, dest, 100, opts)
	if err == nil {
		t.Fatal("分片失败应报错")
	}
}

// ──────────────────────────────────────────────────────────
// drive_transfer.go 覆盖率补全：downloadOnePart select ctx.Done 分支
// ──────────────────────────────────────────────────────────

func TestCrossPlatformCoverageDriveTransferDownloadOnePartSelectCtxDone(t *testing.T) {
	// 目标：覆盖 downloadOnePart 中 select { case <-ctx.Done(): return err } 分支。
	// 策略：server 返回 500（非 auth）→ 代码通过 ctx.Err() 检查（此时 ctx 未取消）
	//        → 进入 select 等待 backoff(500ms)，此时 goroutine 延迟 50ms 后 cancel ctx
	//        → select 收到 ctx.Done() → 返回 err

	var responded atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		responded.Add(1)
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprint(w, "server error")
	}))
	defer srv.Close()

	f, _ := os.CreateTemp(t.TempDir(), "select-ctx-done-*")
	defer f.Close()
	if err := f.Truncate(100); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	creds := &driveCredentialState{url: srv.URL}

	// 等第一次 HTTP 响应返回，然后延迟 50ms 确保代码已通过 ctx.Err() 检查并进入 select
	go func() {
		for responded.Load() < 1 {
			time.Sleep(time.Millisecond)
		}
		// 关键延迟：确保代码已经通过 line 676 的 ctx.Err() 检查（此时返回 nil）
		// 并进入 select 等待 time.After(500ms)
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	err := downloadOnePart(ctx, creds, f, driveDownloadPart{index: 0, offset: 0, length: 10}, 0)
	if err == nil {
		t.Fatal("select 中 ctx 取消应返回错误")
	}
}

// ──────────────────────────────────────────────────────────
// drive_transfer.go 覆盖率补全：worker 取到 part 后 runCtx 已取消
// ──────────────────────────────────────────────────────────

func TestCrossPlatformCoverageDriveTransferWorkerRunCtxAlreadyCancelled(t *testing.T) {
	// 目标：覆盖 downloadRangedParts worker 中 runCtx.Err() != nil → return 分支。
	// 策略：2 workers + 多个分片；server 第一次请求成功，第二次返回 500 触发 fail()+cancel；
	//        第一个 worker 完成后循环取下一个 part，此时 runCtx 已 cancelled。
	content := makeTestContent(100)
	var reqCount atomic.Int32
	gate := make(chan struct{}) // 控制第一个请求的时序

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := reqCount.Add(1)
		if n == 1 {
			// 第一个请求：正常返回分片数据
			var start, end int64
			fmt.Sscanf(r.Header.Get("Range"), "bytes=%d-%d", &start, &end)
			if end >= int64(len(content)) {
				end = int64(len(content)) - 1
			}
			// 等待 gate 信号，确保第二个 worker 已拿到 part 并发起请求
			<-gate
			w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, end, len(content)))
			w.WriteHeader(http.StatusPartialContent)
			_, _ = w.Write(content[start : end+1])
		} else if n == 2 {
			// 第二个请求：失败，触发 fail() 取消 runCtx
			close(gate) // 释放第一个请求
			w.WriteHeader(http.StatusInternalServerError)
			fmt.Fprint(w, "error")
		} else {
			// 后续请求（第一个 worker 循环后用已取消的 ctx 发起）立即返回错误
			w.WriteHeader(http.StatusInternalServerError)
			fmt.Fprint(w, "cancelled")
		}
	}))
	defer srv.Close()

	// 释放 gate 以防第二个请求没按预期到达
	go func() {
		time.Sleep(2 * time.Second)
		select {
		case <-gate:
		default:
			close(gate)
		}
	}()

	creds := &driveCredentialState{url: srv.URL}
	dest := filepath.Join(t.TempDir(), "worker-runctx.bin")
	// 10 分片 * 10 bytes = 100 bytes；parallel=2
	opts := driveDownloadOptions{partSize: 10, parallel: 2, resume: false, knownSize: 100}
	err := downloadRangedParts(context.Background(), creds, dest, 100, opts)
	if err == nil {
		t.Fatal("worker 失败应报错")
	}
}

// ──────────────────────────────────────────────────────────
// drive.go 覆盖率补全：download 命令 logf 回调、fetchCred 回调、非 Canceled 错误路径
// ──────────────────────────────────────────────────────────

func TestCrossPlatformCoverageDriveDownloadRangedPathLogfAndFetchCred(t *testing.T) {
	// 目标：覆盖 drive.go download 命令中的：
	//   - dlOpts.logf 回调（line 501-503）
	//   - fetchCred 回调（line 540-545）
	//   - return err 非 Canceled 路径（line 552）
	// 策略：
	//   MCP step1: 返回 download info 带 fileSize=100, resourceUrl 指向 test server
	//   Test server: probe 返回 401 → 触发 fetchCred
	//   MCP step2: fetchCred 调用 → 返回错误（覆盖 ferr != nil 分支 line 542-544）
	//   由于 fetchCred 失败，probeRangeSupport 返回错误，driveTransferDownload 返回该错误
	//   该错误不是 context.Canceled → 走 line 552 的 return err

	// 保存并替换 driveRangeClient 以控制 probe 行为
	origClient := driveRangeClient
	t.Cleanup(func() { driveRangeClient = origClient })

	driveRangeClient = &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			// 始终返回 401 触发凭证刷新
			return &http.Response{
				StatusCode: http.StatusUnauthorized,
				Header:     make(http.Header),
				Body:       io.NopCloser(strings.NewReader("token expired")),
			}, nil
		}),
	}

	dest := filepath.Join(t.TempDir(), "ranged-logf.bin")
	// MCP responses: step1 返回带 fileSize 的下载信息（需 >= 2*partSize=2MB 才走 ranged 路径）
	// step2 fetchCred 返回错误
	mcpResp := `{"resourceUrl":"https://fake.invalid/file.bin","fileSize":3000000}`
	caller := &scriptedToolCaller{steps: []scriptedToolStep{
		{text: mcpResp},                     // step1: download_file 成功
		{err: errors.New("refresh failed")}, // step2: fetchCred 调用失败
	}}

	err := executeDriveEdge(t, caller,
		"download", "--node", "node-1", "--output", dest, "--part-size", "1MB", "--parallel", "1")
	if err == nil {
		t.Fatal("fetchCred 失败应导致错误")
	}
	// 确保不是 context.Canceled 错误（覆盖 line 552）
	if errors.Is(err, context.Canceled) {
		t.Fatal("错误不应是 context.Canceled")
	}
}

func TestCrossPlatformCoverageDriveDownloadRangedFetchCredSuccess(t *testing.T) {
	// 目标：覆盖 fetchCred 的成功路径（line 545: return parseDownloadInfo(t)）
	// 策略：probe 第一次 401 → fetchCred 成功返回新凭证 → probe 第二次成功 →
	//       进入 ranged download → 完成下载

	content := makeTestContent(100)
	origClient := driveRangeClient
	t.Cleanup(func() { driveRangeClient = origClient })

	var probeAttempt atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := probeAttempt.Add(1)
		if n == 1 {
			// 第一次 probe → 401 触发 fetchCred
			w.WriteHeader(http.StatusUnauthorized)
			fmt.Fprint(w, "expired")
			return
		}
		// 后续请求正常处理 range
		var start, end int64
		if _, err := fmt.Sscanf(r.Header.Get("Range"), "bytes=%d-%d", &start, &end); err != nil {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(content)
			return
		}
		if end >= int64(len(content)) {
			end = int64(len(content)) - 1
		}
		w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, end, len(content)))
		w.WriteHeader(http.StatusPartialContent)
		_, _ = w.Write(content[start : end+1])
	}))
	defer srv.Close()

	driveRangeClient = srv.Client()

	dest := filepath.Join(t.TempDir(), "ranged-fetchcred-ok.bin")
	// MCP step1: download info with fileSize >= 2*partSize (2MB) to trigger ranged path
	// MCP step2: fetchCred returns new valid info (same server URL)
	mcpResp1 := fmt.Sprintf(`{"resourceUrl":"%s/file.bin","fileSize":3000000}`, srv.URL)
	mcpResp2 := fmt.Sprintf(`{"resourceUrl":"%s/file.bin","fileSize":3000000}`, srv.URL)
	caller := &scriptedToolCaller{steps: []scriptedToolStep{
		{text: mcpResp1}, // step1: download_file
		{text: mcpResp2}, // step2: fetchCred (refresh)
	}}

	err := executeDriveEdge(t, caller,
		"download", "--node", "node-1", "--output", dest, "--part-size", "1MB", "--parallel", "1", "--no-resume")
	if err != nil {
		t.Fatalf("ranged download 应成功: %v", err)
	}
}

// ──────────────────────────────────────────────────────────
// drive.go 覆盖率补全：download-version 命令的对称路径
// ──────────────────────────────────────────────────────────

func TestCrossPlatformCoverageDriveDownloadVersionRangedPathLogfAndFetchCred(t *testing.T) {
	// 覆盖 download-version 中的 logf（line 595-597）、fetchCred（line 632-637）、
	// 和 return err（line 644）

	origClient := driveRangeClient
	t.Cleanup(func() { driveRangeClient = origClient })

	driveRangeClient = &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusUnauthorized,
				Header:     make(http.Header),
				Body:       io.NopCloser(strings.NewReader("token expired")),
			}, nil
		}),
	}

	dest := filepath.Join(t.TempDir(), "version-ranged.bin")
	mcpResp := `{"downloadUrl":"https://fake.invalid/file.bin","fileSize":3000000}`
	caller := &scriptedToolCaller{steps: []scriptedToolStep{
		{text: mcpResp},                     // step1: download_file_version
		{err: errors.New("refresh failed")}, // step2: fetchCred 失败
	}}

	err := executeDriveEdge(t, caller,
		"download-version", "--node", "node-1", "--version", "3", "--output", dest,
		"--part-size", "1MB", "--parallel", "1")
	if err == nil {
		t.Fatal("download-version fetchCred 失败应报错")
	}
	if errors.Is(err, context.Canceled) {
		t.Fatal("错误不应是 context.Canceled")
	}
}

func TestCrossPlatformCoverageDriveDownloadVersionRangedFetchCredSuccess(t *testing.T) {
	// 覆盖 download-version 的 fetchCred 成功路径（line 637）

	totalSize := int64(3000000)
	content := makeTestContent(int(totalSize))
	origClient := driveRangeClient
	t.Cleanup(func() { driveRangeClient = origClient })

	var probeAttempt atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := probeAttempt.Add(1)
		if n == 1 {
			w.WriteHeader(http.StatusUnauthorized)
			fmt.Fprint(w, "expired")
			return
		}
		var start, end int64
		if _, err := fmt.Sscanf(r.Header.Get("Range"), "bytes=%d-%d", &start, &end); err != nil {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(content)
			return
		}
		if end >= int64(len(content)) {
			end = int64(len(content)) - 1
		}
		w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, end, len(content)))
		w.WriteHeader(http.StatusPartialContent)
		_, _ = w.Write(content[start : end+1])
	}))
	defer srv.Close()

	driveRangeClient = srv.Client()

	dest := filepath.Join(t.TempDir(), "version-fetchcred-ok.bin")
	mcpResp1 := fmt.Sprintf(`{"downloadUrl":"%s/file.bin","fileSize":3000000}`, srv.URL)
	mcpResp2 := fmt.Sprintf(`{"downloadUrl":"%s/file.bin","fileSize":3000000}`, srv.URL)
	caller := &scriptedToolCaller{steps: []scriptedToolStep{
		{text: mcpResp1},
		{text: mcpResp2},
	}}

	err := executeDriveEdge(t, caller,
		"download-version", "--node", "node-1", "--version", "3", "--output", dest,
		"--part-size", "1MB", "--parallel", "1", "--no-resume")
	if err != nil {
		t.Fatalf("download-version ranged 应成功: %v", err)
	}
}

// ──────────────────────────────────────────────────────────
// drive.go 覆盖率补全：uploadToDrive refetch lambda
// ──────────────────────────────────────────────────────────

func TestCrossPlatformCoverageDriveUploadRefetchLambda(t *testing.T) {
	// 目标：覆盖 drive.go uploadToDrive 中 refetch lambda（line 2063-2065）
	// 策略：httpPutFile 第一次返回 401 → 触发 refetch → callMCPToolReturnTextOnServer
	//        → scriptedCaller step2 返回新凭证 → 第二次 PUT 成功

	oldPut := httpPutFile
	putCalls := 0
	httpPutFile = func(ctx context.Context, url string, headers map[string]string, filePath string, fileSize int64) error {
		putCalls++
		if putCalls == 1 {
			return &httpStatusError{StatusCode: 401, Body: "token expired"}
		}
		return nil
	}
	t.Cleanup(func() { httpPutFile = oldPut })

	file := filepath.Join(t.TempDir(), "upload.txt")
	_ = os.WriteFile(file, []byte("content"), 0o600)

	// step1: get_upload_info 成功
	// step2: refetch (第二次 get_upload_info) 成功
	// step3: commit_upload 成功
	payload1 := `{"uploadId":"u1","resourceUrl":"https://upload.invalid/put1"}`
	payload2 := `{"uploadId":"u2","resourceUrl":"https://upload.invalid/put2"}`
	caller := &scriptedToolCaller{steps: []scriptedToolStep{
		{text: payload1},
		{text: payload2},
		{text: `{}`},
	}}

	err := executeDriveEdge(t, caller, "upload", "--file", file, "--folder", "folder-1")
	if err != nil {
		t.Fatalf("upload with refetch 应成功: %v", err)
	}
	if putCalls != 2 {
		t.Fatalf("PUT 应被调用 2 次（首次 401 + 重试），实际: %d", putCalls)
	}
}

// ──────────────────────────────────────────────────────────
// 覆盖率补全：drive.go:501-503 logf 闭包体（通过命令路径触发分片下载）
// ──────────────────────────────────────────────────────────

func TestCrossPlatformCoverageDriveDownloadCmdLogfInvocation(t *testing.T) {
	// 目标：覆盖 drive.go download 命令中 dlOpts.logf 闭包体（line 501-503）。
	// 策略：通过 executeDriveEdge 走完整命令路径，MCP 返回 fileSize >= 2*partSize(2MB)，
	//        server 正确支持 Range，使 driveTransferDownload 走入 downloadRangedParts，
	//        logf 在分片启动时被调用。

	totalSize := 2200000 // > 2*1MB threshold
	content := makeTestContent(totalSize)

	origClient := driveRangeClient
	t.Cleanup(func() { driveRangeClient = origClient })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rng := r.Header.Get("Range")
		if rng == "" {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(content)
			return
		}
		var start, end int64
		if _, err := fmt.Sscanf(rng, "bytes=%d-%d", &start, &end); err != nil || start >= int64(len(content)) {
			w.WriteHeader(http.StatusRequestedRangeNotSatisfiable)
			return
		}
		if end >= int64(len(content)) {
			end = int64(len(content)) - 1
		}
		w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, end, len(content)))
		w.WriteHeader(http.StatusPartialContent)
		_, _ = w.Write(content[start : end+1])
	}))
	defer srv.Close()

	driveRangeClient = srv.Client()

	dest := filepath.Join(t.TempDir(), "cmd-logf.bin")
	mcpResp := fmt.Sprintf(`{"resourceUrl":"%s/file.bin","fileSize":%d}`, srv.URL, totalSize)
	caller := &scriptedToolCaller{steps: []scriptedToolStep{
		{text: mcpResp},
	}}

	err := executeDriveEdge(t, caller,
		"download", "--node", "node-1", "--output", dest, "--part-size", "1MB", "--parallel", "2", "--no-resume")
	if err != nil {
		t.Fatalf("ranged download via cmd 应成功: %v", err)
	}
	// 验证文件内容正确
	got, _ := os.ReadFile(dest)
	if len(got) != totalSize {
		t.Fatalf("产物大小 %d != %d", len(got), totalSize)
	}
}

// ──────────────────────────────────────────────────────────
// 覆盖率补全：drive.go:553 download 命令 cancel + --no-resume else 分支
// ──────────────────────────────────────────────────────────

func TestCrossPlatformCoverageDriveDownloadCancelNoResume(t *testing.T) {
	// 目标：覆盖 download 命令中 context.Canceled + noResume=true 的 else 分支（line 553）。
	// 条件：driveTransferDownload 返回 context.Canceled + --no-resume 已设置。
	// 由于 fileSize 很小（< 2*16MB），走 downloadSingleWithAuthRetry → httpGetFile。
	// mock httpGetFile 返回 context.Canceled。

	oldGet := httpGetFile
	httpGetFile = func(_ context.Context, _ string, _ map[string]string, _ string) error {
		return context.Canceled
	}
	t.Cleanup(func() { httpGetFile = oldGet })

	dest := filepath.Join(t.TempDir(), "cancel-noresume.bin")
	mcpResp := `{"resourceUrl":"https://fake.invalid/f.bin","fileSize":100}`
	caller := &scriptedToolCaller{steps: []scriptedToolStep{
		{text: mcpResp},
	}}

	err := executeDriveEdge(t, caller,
		"download", "--node", "node-1", "--output", dest, "--no-resume")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("应返回 context.Canceled, got: %v", err)
	}
}

// ──────────────────────────────────────────────────────────
// 覆盖率补全：drive.go:652 download-version 命令 cancel + --no-resume else 分支
// ──────────────────────────────────────────────────────────

func TestCrossPlatformCoverageDriveDownloadVersionCancelNoResume(t *testing.T) {
	// 目标：覆盖 download-version 命令中 context.Canceled + noResume=true 的 else 分支（line 652）。

	oldGet := httpGetFile
	httpGetFile = func(_ context.Context, _ string, _ map[string]string, _ string) error {
		return context.Canceled
	}
	t.Cleanup(func() { httpGetFile = oldGet })

	dest := filepath.Join(t.TempDir(), "ver-cancel-noresume.bin")
	mcpResp := `{"downloadUrl":"https://fake.invalid/f.bin","fileSize":100}`
	caller := &scriptedToolCaller{steps: []scriptedToolStep{
		{text: mcpResp},
	}}

	err := executeDriveEdge(t, caller,
		"download-version", "--node", "node-1", "--version", "3", "--output", dest, "--no-resume")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("应返回 context.Canceled, got: %v", err)
	}
}

// ──────────────────────────────────────────────────────────
// 覆盖率补全：drive_transfer.go:604 worker goroutine ctx cancel early return
// ──────────────────────────────────────────────────────────

func TestCrossPlatformCoverageDriveTransferWorkerCtxCancelBeforeProcess(t *testing.T) {
	// 目标：覆盖 downloadRangedParts worker 中 "if runCtx.Err() != nil { return }"。
	// 通过结构化 seam 让 worker 在收到唯一分片后确定性观察到取消状态；
	// 不再依赖微秒级 timeout 与 goroutine 调度概率。
	var checks atomic.Int32
	testseam.Swap(t, &driveWorkerContextErr, func(context.Context) error {
		checks.Add(1)
		return context.Canceled
	})
	var requests atomic.Int32
	testseam.Swap(t, &driveRangeClient, &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			requests.Add(1)
			return nil, errors.New("worker context guard did not stop the request")
		}),
	})

	creds := &driveCredentialState{url: "http://127.0.0.1:1/fake"}
	dest := filepath.Join(t.TempDir(), "worker-context-guard.bin")
	opts := driveDownloadOptions{partSize: 1, parallel: 1, resume: false, knownSize: 1}
	if err := downloadRangedParts(context.Background(), creds, dest, 1, opts); err != nil {
		t.Fatalf("downloadRangedParts context guard: %v", err)
	}
	if checks.Load() != 1 {
		t.Fatalf("worker context checks = %d, want 1", checks.Load())
	}
	if requests.Load() != 0 {
		t.Fatalf("worker requests = %d, want 0", requests.Load())
	}
}

// ──────────────────────────────────────────────────────────
// driveCredentialState.refresh: 版本校验（P1 修复验证）
// ──────────────────────────────────────────────────────────

// 刷新后版本一致 → 正常继续
func TestCrossPlatformCoverageRefreshVersionMatch(t *testing.T) {
	cs := &driveCredentialState{
		url:            "u0",
		initialVersion: 5,
		fetch: func(ctx context.Context) (string, map[string]string, int, error) {
			return "u1", nil, 5, nil
		},
	}
	_, _, gen := cs.current()
	if err := cs.refresh(context.Background(), gen); err != nil {
		t.Fatalf("版本一致应成功: %v", err)
	}
	url, _, _ := cs.current()
	if url != "u1" {
		t.Errorf("刷新后 URL 应更新, got %q", url)
	}
}

// 刷新后版本变化 → 返回错误终止
func TestCrossPlatformCoverageRefreshVersionMismatch(t *testing.T) {
	cs := &driveCredentialState{
		url:            "u0",
		initialVersion: 5,
		fetch: func(ctx context.Context) (string, map[string]string, int, error) {
			return "u1", nil, 8, nil
		},
	}
	_, _, gen := cs.current()
	err := cs.refresh(context.Background(), gen)
	if err == nil {
		t.Fatal("版本变更应返回错误")
	}
	if !strings.Contains(err.Error(), "文件版本已变更") {
		t.Fatalf("错误信息应包含版本变更提示, got: %v", err)
	}
	if !strings.Contains(err.Error(), "5") || !strings.Contains(err.Error(), "8") {
		t.Fatalf("错误信息应包含新旧版本号, got: %v", err)
	}
	// URL 不应更新（拒绝了变更）
	url, _, _ := cs.current()
	if url != "u0" {
		t.Errorf("版本变更时 URL 不应更新, got %q", url)
	}
}

// initialVersion=0（旧 MCP 不返回 version）→ 激进策略：返回 sentinel error
func TestCrossPlatformCoverageRefreshVersionZeroInitial(t *testing.T) {
	cs := &driveCredentialState{
		url:            "u0",
		initialVersion: 0, // 未知初始版本
		fetch: func(ctx context.Context) (string, map[string]string, int, error) {
			return "u1", nil, 99, nil // 刷新返回任意版本
		},
	}
	_, _, gen := cs.current()
	err := cs.refresh(context.Background(), gen)
	if !errors.Is(err, errCredentialRefreshVersionUnknown) {
		t.Fatalf("initialVersion=0 应返回 errCredentialRefreshVersionUnknown, got: %v", err)
	}
	// 凭证应已更新
	url, _, _ := cs.current()
	if url != "u1" {
		t.Errorf("凭证应已更新, got url=%q", url)
	}
}

// 刷新返回 version=0（MCP 不返回 version）→ 激进策略：返回 sentinel error
func TestCrossPlatformCoverageRefreshVersionZeroReturned(t *testing.T) {
	cs := &driveCredentialState{
		url:            "u0",
		initialVersion: 5,
		fetch: func(ctx context.Context) (string, map[string]string, int, error) {
			return "u1", nil, 0, nil // 刷新未返回版本
		},
	}
	_, _, gen := cs.current()
	err := cs.refresh(context.Background(), gen)
	if !errors.Is(err, errCredentialRefreshVersionUnknown) {
		t.Fatalf("version=0 应返回 errCredentialRefreshVersionUnknown, got: %v", err)
	}
	// 凭证应已更新
	url, _, _ := cs.current()
	if url != "u1" {
		t.Errorf("凭证应已更新, got url=%q", url)
	}
}

// 分片下载中 refresh 触发版本变更检测 → 下载终止
func TestCrossPlatformCoverageDownloadRangedParts_VersionMismatchAbort(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := calls.Add(1)
		if n == 1 {
			// 第一次（probeRangeSupport 不经此路径）/首分片请求 → 401
			w.WriteHeader(http.StatusUnauthorized)
			fmt.Fprint(w, "expired")
			return
		}
		// 不应走到这里
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	creds := &driveCredentialState{
		url:            srv.URL,
		initialVersion: 3,
		fetch: func(ctx context.Context) (string, map[string]string, int, error) {
			// 刷新时版本变了
			return srv.URL, nil, 7, nil
		},
	}
	dest := filepath.Join(t.TempDir(), "version-abort.bin")
	opts := driveDownloadOptions{partSize: 50, parallel: 1, resume: false}
	err := downloadRangedParts(context.Background(), creds, dest, 100, opts)
	if err == nil {
		t.Fatal("版本变更应导致下载失败")
	}
	if !strings.Contains(err.Error(), "文件版本已变更") {
		t.Fatalf("应包含版本变更错误, got: %v", err)
	}
}

// ──────────────────────────────────────────────────────────
// fetchRangeInto: Content-Range total 校验（P1 修复验证）
// ──────────────────────────────────────────────────────────

// Content-Range total 与 expectedTotal 一致 → 正常通过
func TestCrossPlatformCoverageFetchRangeInto_ContentRangeTotalMatch(t *testing.T) {
	content := makeTestContent(200)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var start, end int64
		fmt.Sscanf(r.Header.Get("Range"), "bytes=%d-%d", &start, &end)
		w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, end, len(content)))
		w.WriteHeader(http.StatusPartialContent)
		_, _ = w.Write(content[start : end+1])
	}))
	defer srv.Close()

	f, err := os.CreateTemp(t.TempDir(), "total-match-*")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := f.Truncate(200); err != nil {
		t.Fatal(err)
	}
	part := driveDownloadPart{index: 0, offset: 0, length: 50}
	if err := fetchRangeInto(context.Background(), srv.URL, nil, f, part, 200); err != nil {
		t.Fatalf("total 一致不应报错: %v", err)
	}
}

// Content-Range total 与 expectedTotal 不一致 → 返回错误
func TestCrossPlatformCoverageFetchRangeInto_ContentRangeTotalMismatch(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var start, end int64
		fmt.Sscanf(r.Header.Get("Range"), "bytes=%d-%d", &start, &end)
		// 服务端声称 total=500，但调用方期望 200
		w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/500", start, end))
		w.WriteHeader(http.StatusPartialContent)
		data := make([]byte, end-start+1)
		_, _ = w.Write(data)
	}))
	defer srv.Close()

	f, err := os.CreateTemp(t.TempDir(), "total-mismatch-*")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := f.Truncate(200); err != nil {
		t.Fatal(err)
	}
	part := driveDownloadPart{index: 0, offset: 0, length: 50}
	err = fetchRangeInto(context.Background(), srv.URL, nil, f, part, 200)
	if err == nil {
		t.Fatal("total 不匹配应返回错误")
	}
	if !strings.Contains(err.Error(), "总长不匹配") {
		t.Fatalf("错误信息应包含'总长不匹配': %v", err)
	}
	if !strings.Contains(err.Error(), "500") || !strings.Contains(err.Error(), "200") {
		t.Fatalf("错误信息应包含实际和期望值: %v", err)
	}
}

// expectedTotal=0（调用方不知道总长）→ 跳过校验
func TestCrossPlatformCoverageFetchRangeInto_ContentRangeTotalSkipWhenZeroExpected(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var start, end int64
		fmt.Sscanf(r.Header.Get("Range"), "bytes=%d-%d", &start, &end)
		w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/9999", start, end))
		w.WriteHeader(http.StatusPartialContent)
		data := make([]byte, end-start+1)
		_, _ = w.Write(data)
	}))
	defer srv.Close()

	f, err := os.CreateTemp(t.TempDir(), "total-skip-*")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := f.Truncate(100); err != nil {
		t.Fatal(err)
	}
	part := driveDownloadPart{index: 0, offset: 0, length: 50}
	// expectedTotal=0 → 不校验 total
	if err := fetchRangeInto(context.Background(), srv.URL, nil, f, part, 0); err != nil {
		t.Fatalf("expectedTotal=0 应跳过 total 校验: %v", err)
	}
}

// Content-Range total=-1（"*" 未知）→ crTotal<0 → 跳过校验
func TestCrossPlatformCoverageFetchRangeInto_ContentRangeTotalUnknown(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var start, end int64
		fmt.Sscanf(r.Header.Get("Range"), "bytes=%d-%d", &start, &end)
		// total="*" → parseContentRange 返回 total=-1
		w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/*", start, end))
		w.WriteHeader(http.StatusPartialContent)
		data := make([]byte, end-start+1)
		_, _ = w.Write(data)
	}))
	defer srv.Close()

	f, err := os.CreateTemp(t.TempDir(), "total-unknown-*")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := f.Truncate(100); err != nil {
		t.Fatal(err)
	}
	part := driveDownloadPart{index: 0, offset: 0, length: 50}
	// crTotal=-1 (不 > 0) → 跳过 total 校验
	if err := fetchRangeInto(context.Background(), srv.URL, nil, f, part, 200); err != nil {
		t.Fatalf("total=* 应跳过校验: %v", err)
	}
}

// TestCrossPlatformCoverageFetchRangeIntoMissingContentRange 验证 fetchRangeInto 在
// 206 响应缺少 Content-Range 头时返回明确错误，拒绝无法验证偏移一致性的分片。
func TestCrossPlatformCoverageFetchRangeIntoMissingContentRange(t *testing.T) {
	content := makeTestContent(1024)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var start, end int64
		fmt.Sscanf(r.Header.Get("Range"), "bytes=%d-%d", &start, &end)
		// 返回 206 但不带 Content-Range
		w.WriteHeader(http.StatusPartialContent)
		_, _ = w.Write(content[start : end+1])
	}))
	defer srv.Close()

	f, err := os.CreateTemp(t.TempDir(), "missing-cr-*")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := f.Truncate(1024); err != nil {
		t.Fatal(err)
	}

	part := driveDownloadPart{index: 0, offset: 0, length: 512}
	err = fetchRangeInto(context.Background(), srv.URL, nil, f, part, 1024)
	if err == nil {
		t.Fatal("应当返回错误：分片响应缺少 Content-Range")
	}
	if !strings.Contains(err.Error(), "缺少 Content-Range") {
		t.Fatalf("错误信息应包含'缺少 Content-Range': %v", err)
	}
}

// TestCrossPlatformCoverageDriveDownloadFetchCredParseError 覆盖 drive.go download 命令
// fetchCred 闭包中 parseDriveDownloadInfo 返回 error 的路径（line 547-549）。
func TestCrossPlatformCoverageDriveDownloadFetchCredParseError(t *testing.T) {
	origClient := driveRangeClient
	t.Cleanup(func() { driveRangeClient = origClient })

	driveRangeClient = &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusUnauthorized,
				Header:     make(http.Header),
				Body:       io.NopCloser(strings.NewReader("expired")),
			}, nil
		}),
	}

	dest := filepath.Join(t.TempDir(), "parse-err.bin")
	// step1: 正常返回（进入分片路径）；step2: fetchCred 返回可解析但无 URL 的 JSON
	caller := &scriptedToolCaller{steps: []scriptedToolStep{
		{text: `{"resourceUrl":"https://fake.invalid/f","fileSize":3000000}`},
		{text: `{"result":{}}`}, // parseDriveDownloadInfo 会返回 "downloadUrl 为空" 错误
	}}

	err := executeDriveEdge(t, caller,
		"download", "--node", "n1", "--output", dest, "--part-size", "1MB", "--parallel", "1")
	if err == nil {
		t.Fatal("parseDriveDownloadInfo 失败应导致错误")
	}
	if !strings.Contains(err.Error(), "downloadUrl") && !strings.Contains(err.Error(), "下载链接") {
		t.Fatalf("错误应包含解析失败信息: %v", err)
	}
}

// TestCrossPlatformCoverageDriveDownloadVersionFetchCredParseError 覆盖 download-version
// fetchCred 闭包中 parseDriveDownloadInfo 返回 error 的路径（line 650-652）。
func TestCrossPlatformCoverageDriveDownloadVersionFetchCredParseError(t *testing.T) {
	origClient := driveRangeClient
	t.Cleanup(func() { driveRangeClient = origClient })

	driveRangeClient = &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusUnauthorized,
				Header:     make(http.Header),
				Body:       io.NopCloser(strings.NewReader("expired")),
			}, nil
		}),
	}

	dest := filepath.Join(t.TempDir(), "ver-parse-err.bin")
	caller := &scriptedToolCaller{steps: []scriptedToolStep{
		{text: `{"resourceUrl":"https://fake.invalid/f","fileSize":3000000}`},
		{text: `{"result":{}}`},
	}}

	err := executeDriveEdge(t, caller,
		"download-version", "--node", "n1", "--version", "3", "--output", dest, "--part-size", "1MB", "--parallel", "1")
	if err == nil {
		t.Fatal("parseDriveDownloadInfo 失败应导致错误")
	}
}

// ──────────────────────────────────────────────────────────
// 激进策略：version=0 时凭证刷新后清空分片从头下载
// ──────────────────────────────────────────────────────────

// TestCrossPlatformCoverageRefreshVersionBothZero 双方 version=0 → 返回 sentinel error，凭证已更新
func TestCrossPlatformCoverageRefreshVersionBothZero(t *testing.T) {
	cs := &driveCredentialState{
		url:            "u0",
		initialVersion: 0,
		fetch: func(ctx context.Context) (string, map[string]string, int, error) {
			return "u1", map[string]string{"k": "v"}, 0, nil
		},
	}
	_, _, gen := cs.current()
	err := cs.refresh(context.Background(), gen)
	if !errors.Is(err, errCredentialRefreshVersionUnknown) {
		t.Fatalf("双方 version=0 应返回 sentinel error, got: %v", err)
	}
	// 凭证应已更新
	url, headers, newGen := cs.current()
	if url != "u1" {
		t.Errorf("URL 应已更新, got %q", url)
	}
	if headers["k"] != "v" {
		t.Errorf("headers 应已更新, got %v", headers)
	}
	if newGen != gen+1 {
		t.Errorf("gen 应递增, got %d (was %d)", newGen, gen)
	}
}

// TestCrossPlatformCoverageDownloadRangedParts_VersionUnknownCleansCheckpoint
// version=0 的凭证刷新导致分片下载中止，checkpoint 和临时文件被清理。
func TestCrossPlatformCoverageDownloadRangedParts_VersionUnknownCleansCheckpoint(t *testing.T) {
	content := makeTestContent(100)
	// 服务端：第一次请求正常（probe），后续分片请求返回 401 触发 refresh
	var probeCount atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rng := r.Header.Get("Range")
		if rng == "bytes=0-0" && probeCount.Add(1) == 1 {
			// probe 请求正常
			w.Header().Set("Content-Range", fmt.Sprintf("bytes 0-0/%d", len(content)))
			w.WriteHeader(http.StatusPartialContent)
			_, _ = w.Write(content[0:1])
			return
		}
		// 分片请求全部返回 401 触发 credential refresh
		w.WriteHeader(http.StatusUnauthorized)
		fmt.Fprint(w, "expired")
	}))
	defer srv.Close()

	dest := filepath.Join(t.TempDir(), "version-unknown-clean.bin")
	partPath := dest + drivePartFileSuffix
	metaPath := dest + drivePartMetaSuffix

	// fetch 返回 version=0（旧 MCP），initialVersion=0（opts.version 未设置）
	fetch := func(ctx context.Context) (string, map[string]string, int, error) {
		return srv.URL, nil, 0, nil // version=0
	}
	opts := driveDownloadOptions{partSize: 30, parallel: 1, resume: true, knownSize: 100}
	// initialVersion 来自 opts.version（默认 0）

	err := driveTransferDownload(context.Background(), fetch, srv.URL, nil, dest, opts)
	if err == nil {
		t.Fatal("version=0 刷新应导致下载失败")
	}
	if !errors.Is(err, errCredentialRefreshVersionUnknown) {
		t.Fatalf("应包含 sentinel error, got: %v", err)
	}
	// 验证 checkpoint 和临时文件被清理
	if _, statErr := os.Stat(metaPath); !os.IsNotExist(statErr) {
		t.Error("checkpoint 应被清理")
	}
	if _, statErr := os.Stat(partPath); !os.IsNotExist(statErr) {
		t.Error("分片临时文件应被清理")
	}
}

// TestCrossPlatformCoverageProbeRangeSupport_VersionUnknownNonFatal
// 探测阶段 version=0 的 refresh 不应阻断探测（无已完成分片需保护）。
func TestCrossPlatformCoverageProbeRangeSupport_VersionUnknownNonFatal(t *testing.T) {
	var attempts atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := attempts.Add(1)
		if n == 1 {
			// 第一次：401 触发 refresh
			w.WriteHeader(http.StatusUnauthorized)
			fmt.Fprint(w, "expired")
			return
		}
		// 第二次（refresh 后重试）：正常 206
		w.Header().Set("Content-Range", "bytes 0-0/500")
		w.WriteHeader(http.StatusPartialContent)
		_, _ = w.Write([]byte{0})
	}))
	defer srv.Close()

	creds := &driveCredentialState{
		url:            srv.URL,
		initialVersion: 0, // version 未知
		fetch: func(ctx context.Context) (string, map[string]string, int, error) {
			return srv.URL, nil, 0, nil // 仍然未知
		},
	}
	total, resp, err := probeRangeSupport(context.Background(), creds)
	if err != nil {
		t.Fatalf("探测阶段 version=0 不应报错: %v", err)
	}
	if resp != nil {
		t.Error("不应返回全量 resp")
	}
	if total != 500 {
		t.Errorf("应返回正确 total=500, got %d", total)
	}
	if attempts.Load() != 2 {
		t.Errorf("应请求 2 次（401→refresh→retry 成功）, got %d", attempts.Load())
	}
}
