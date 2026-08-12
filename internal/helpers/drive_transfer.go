package helpers

// ──────────────────────────────────────────────────────────
// drive 传输增强：中心协议（token 化凭证）+ 分片下载（Range）
//
// 下载：文件大小 ≥ 2×part-size 时自动分片并发下载（对齐 aws s3 cp /
// ossutil 惯例），断点续传默认开启（<dest>.dwspart 临时文件 + checkpoint
// 元信息），服务端不支持 Range 时自动回退整流；401/403（凭证过期）自动
// 重新调用 MCP 取新凭证后续传。
// 上传：中心协议（uploadType=httpToCenterWithToken）与 OSS 走同一 PUT
// 路径（URL 服务端拼好、headers 透传、客户端零追加），401/403 重取凭证
// 重试一次；服务端超限错误补充可读提示。
// ──────────────────────────────────────────────────────────

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/spf13/cobra"
)

const (
	driveDownloadDefaultPartSize = 16 * 1024 * 1024 // --part-size 默认 16MB
	driveDownloadMinPartSize     = 1 * 1024 * 1024
	driveDownloadMaxPartSize     = 1024 * 1024 * 1024
	driveDownloadDefaultParallel = 4
	driveDownloadMaxParallel     = 8
	// 单分片常规失败指数退避重试次数（不含 401/403 凭证刷新）。
	driveDownloadPartRetries = 3
	// 单分片 401/403 凭证刷新重试上限（防止无效凭证死循环）。
	driveDownloadPartAuthRetries = 2

	drivePartFileSuffix     = ".dwspart"
	drivePartMetaSuffix     = ".dwspart.meta"
	driveCheckpointVersion  = 1
	uploadTypeCenterToken   = "httpToCenterWithToken"
	driveTransferBodyErrCap = 2048 // 错误响应 body 截断长度
)

// driveRangeClient 分片下载/探测专用 HTTP 客户端。
// 整流路径仍走可注入的 httpGetFile，保持既有测试注入点不变。
var driveRangeClient = &http.Client{Timeout: 10 * time.Minute}

// errCredentialRefreshVersionUnknown 凭证刷新后无法验证文件版本一致性（双方之一
// version=0），激进策略要求清空已完成分片从头下载。
var errCredentialRefreshVersionUnknown = errors.New("凭证刷新后无法验证文件版本一致性，需从头下载")

// Testable OS operation hooks (package-level for coverage injection).
var (
	driveJsonMarshal  = json.Marshal
	driveOsRename     = os.Rename
	driveFileTruncate = (*os.File).Truncate
	driveFileSync     = (*os.File).Sync
	driveFileStat     = (*os.File).Stat
)

var driveWorkerContextErr = func(ctx context.Context) error { return ctx.Err() }

// ──────────────────────────────────────────────────────────
// HTTP 状态错误
// ──────────────────────────────────────────────────────────

// httpStatusError 表示非 2xx 的 HTTP 响应，供上层按状态码分支（401/403 重取凭证等）。
type httpStatusError struct {
	StatusCode int
	Body       string
}

func (e *httpStatusError) Error() string {
	return fmt.Sprintf("HTTP %d: %s", e.StatusCode, e.Body)
}

// isAuthStatusError 判断错误是否为 401/403（凭证过期/无效）。
// 兼容 typed httpStatusError 与文本形态（测试注入或历史包装的 "HTTP 401/403" 错误）。
func isAuthStatusError(err error) bool {
	if err == nil {
		return false
	}
	var se *httpStatusError
	if errors.As(err, &se) {
		return se.StatusCode == http.StatusUnauthorized || se.StatusCode == http.StatusForbidden
	}
	msg := err.Error()
	return strings.Contains(msg, "HTTP 401") || strings.Contains(msg, "HTTP 403")
}

// ──────────────────────────────────────────────────────────
// 参数解析
// ──────────────────────────────────────────────────────────

// parsePartSize 解析 --part-size 的人类可读值（16MB、512KB、1GB；纯数字按字节）。
func parsePartSize(s string) (int64, error) {
	v := strings.TrimSpace(strings.ToUpper(s))
	if v == "" {
		return 0, fmt.Errorf("--part-size 不能为空（示例: 16MB、512KB、1GB）")
	}
	unit := int64(1)
	switch {
	case strings.HasSuffix(v, "GB"):
		unit, v = 1<<30, strings.TrimSuffix(v, "GB")
	case strings.HasSuffix(v, "MB"):
		unit, v = 1<<20, strings.TrimSuffix(v, "MB")
	case strings.HasSuffix(v, "KB"):
		unit, v = 1<<10, strings.TrimSuffix(v, "KB")
	case strings.HasSuffix(v, "G"):
		unit, v = 1<<30, strings.TrimSuffix(v, "G")
	case strings.HasSuffix(v, "M"):
		unit, v = 1<<20, strings.TrimSuffix(v, "M")
	case strings.HasSuffix(v, "K"):
		unit, v = 1<<10, strings.TrimSuffix(v, "K")
	case strings.HasSuffix(v, "B"):
		v = strings.TrimSuffix(v, "B")
	}
	n, err := strconv.ParseInt(strings.TrimSpace(v), 10, 64)
	if err != nil || n <= 0 {
		return 0, fmt.Errorf("--part-size 格式非法: %q（示例: 16MB、512KB、1GB）", s)
	}
	size := n * unit
	if size < driveDownloadMinPartSize || size > driveDownloadMaxPartSize {
		return 0, fmt.Errorf("--part-size 取值范围 %s - %s，当前值: %s",
			formatByteSize(driveDownloadMinPartSize), formatByteSize(driveDownloadMaxPartSize), s)
	}
	return size, nil
}

func formatByteSize(n int64) string {
	switch {
	case n >= 1<<30 && n%(1<<30) == 0:
		return fmt.Sprintf("%dGB", n/(1<<30))
	case n >= 1<<20 && n%(1<<20) == 0:
		return fmt.Sprintf("%dMB", n/(1<<20))
	case n >= 1<<10 && n%(1<<10) == 0:
		return fmt.Sprintf("%dKB", n/(1<<10))
	default:
		return fmt.Sprintf("%dB", n)
	}
}

// driveDownloadOptions 分片下载选项（由 drive download 的 flag 解析而来）。
type driveDownloadOptions struct {
	partSize  int64
	parallel  int
	resume    bool
	knownSize int64  // MCP 返回的 fileSize；未知(0)或小于阈值时直接整流
	nodeID    string // 节点唯一标识（dentryUuid），用于生成 checkpoint 指纹
	version   int    // 文件版本号；0 表示最新版
	logf      func(format string, args ...any)
}

// driveDownloadOptionsFromFlags 解析并校验 --part-size / --parallel / --no-resume。
func driveDownloadOptionsFromFlags(cmd *cobra.Command) (driveDownloadOptions, error) {
	raw, _ := cmd.Flags().GetString("part-size")
	partSize, err := parsePartSize(raw)
	if err != nil {
		return driveDownloadOptions{}, err
	}
	parallel, _ := cmd.Flags().GetInt("parallel")
	if parallel < 1 || parallel > driveDownloadMaxParallel {
		return driveDownloadOptions{}, fmt.Errorf("--parallel 取值范围 1-%d，当前值: %d", driveDownloadMaxParallel, parallel)
	}
	noResume, _ := cmd.Flags().GetBool("no-resume")
	return driveDownloadOptions{partSize: partSize, parallel: parallel, resume: !noResume}, nil
}

// parseDownloadFileSize 从 download_file 返回中提取 fileSize（缺失/非法返回 0）。
func parseDownloadFileSize(text string) int64 {
	var data map[string]any
	if json.Unmarshal([]byte(text), &data) != nil {
		return 0
	}
	if r, ok := data["result"].(map[string]any); ok {
		data = r
	}
	switch v := data["fileSize"].(type) {
	case float64:
		return int64(v)
	case string:
		n, _ := strconv.ParseInt(v, 10, 64)
		return n
	}
	return 0
}

// parseDownloadFileVersion 从 MCP download_file 响应中提取文件当前版本号。
// 返回 0 表示未获取到（兼容旧版 MCP 不返回 version 的场景）。
func parseDownloadFileVersion(text string) int {
	var data map[string]any
	if json.Unmarshal([]byte(text), &data) != nil {
		return 0
	}
	if r, ok := data["result"].(map[string]any); ok {
		data = r
	}
	switch v := data["version"].(type) {
	case float64:
		return int(v)
	case string:
		n, _ := strconv.Atoi(v)
		return n
	}
	return 0
}

// ──────────────────────────────────────────────────────────
// 凭证状态（分片过程共享，401/403 时 single-flight 刷新）
// ──────────────────────────────────────────────────────────

// driveCredentialFetcher 重新调用 MCP 获取下载 URL + headers（含 dentry-token）。
type driveCredentialFetcher func(ctx context.Context) (url string, headers map[string]string, version int, err error)

type driveCredentialState struct {
	mu             sync.Mutex
	fetch          driveCredentialFetcher
	url            string
	headers        map[string]string
	gen            int
	initialVersion int // 首次获取的文件版本号；0 表示未知（兼容旧 MCP）
}

func (cs *driveCredentialState) current() (string, map[string]string, int) {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	return cs.url, cs.headers, cs.gen
}

// refresh 重取凭证。仅当调用方持有的 generation 仍是最新时才真正重取
// （其他并发分片已刷新过则直接复用新凭证，避免重复 MCP 调用）。
func (cs *driveCredentialState) refresh(ctx context.Context, gen int) error {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	if cs.gen > gen {
		return nil // 已被其他分片刷新
	}
	if cs.fetch == nil {
		return fmt.Errorf("下载凭证已过期且无法自动刷新")
	}
	url, headers, version, err := cs.fetch(ctx)
	if err != nil {
		return err
	}
	// 版本校验
	if cs.initialVersion > 0 && version > 0 {
		if version != cs.initialVersion {
			// 双方版本已知且不同 → 文件已被覆盖，终止下载防止数据不一致
			return fmt.Errorf("下载凭证刷新后文件版本已变更（%d → %d），终止下载以防数据不一致", cs.initialVersion, version)
		}
		// 版本一致，正常继续
	} else {
		// 激进策略：版本不可验证（至少一方为 0），更新凭证但返回特殊错误
		// 让上层清空已完成分片从头下载
		cs.url, cs.headers = url, headers
		cs.gen++
		return errCredentialRefreshVersionUnknown
	}
	cs.url, cs.headers = url, headers
	cs.gen++
	return nil
}

// ──────────────────────────────────────────────────────────
// 下载入口：整流 / 分片自动分派
// ──────────────────────────────────────────────────────────

// driveTransferDownload 下载入口：按文件大小自动选择整流或分片下载。
//   - 文件大小未知（MCP 未返回 fileSize）或 < 2×partSize → 直接整流
//     （复用可注入的 httpGetFile，保持存量行为与测试注入边界不变，
//     401/403 时重取凭证重试一次）；
//   - 已知大小 ≥ 2×partSize → 首请求以 Range: bytes=0-0 探测：206 且解析出
//     总长 → 分片下载；服务端返回 200（不支持 Range）→ 直接消费该响应整流落盘。
func driveTransferDownload(ctx context.Context, fetch driveCredentialFetcher, rawURL string, headers map[string]string, destPath string, opts driveDownloadOptions) error {
	if opts.partSize <= 0 {
		opts.partSize = driveDownloadDefaultPartSize
	}
	if opts.parallel <= 0 {
		opts.parallel = driveDownloadDefaultParallel
	}
	threshold := 2 * opts.partSize
	if opts.knownSize < threshold {
		// 含 knownSize==0（大小未知）：不发起额外探测请求，保持存量整流行为
		return downloadSingleWithAuthRetry(ctx, fetch, rawURL, headers, destPath)
	}

	creds := &driveCredentialState{fetch: fetch, url: rawURL, headers: headers, initialVersion: opts.version}
	totalSize, fullResp, err := probeRangeSupport(ctx, creds)
	if err != nil {
		return err
	}
	if fullResp != nil {
		// 服务端不支持 Range（忽略探测请求头返回 200 全量流）：直接消费落盘
		defer fullResp.Body.Close()
		return writeStreamToFile(fullResp.Body, destPath)
	}
	curURL, curHeaders, _ := creds.current()
	if totalSize <= 0 || totalSize < threshold {
		// 总长未知（Content-Range 异常）或小于阈值：整流下载
		return downloadSingleWithAuthRetry(ctx, func(fctx context.Context) (string, map[string]string, int, error) {
			if fetch == nil {
				return "", nil, 0, fmt.Errorf("下载凭证已过期且无法自动刷新")
			}
			return fetch(fctx)
		}, curURL, curHeaders, destPath)
	}
	return downloadRangedParts(ctx, creds, destPath, totalSize, opts)
}

// downloadSingleWithAuthRetry 整流下载；401/403（凭证过期）时重新调用 MCP
// 获取新 URL+token 重试一次，仍失败走既有错误路径。
func downloadSingleWithAuthRetry(ctx context.Context, fetch driveCredentialFetcher, urlStr string, headers map[string]string, destPath string) error {
	err := httpGetFile(ctx, urlStr, headers, destPath)
	if err == nil || !isAuthStatusError(err) || fetch == nil {
		return err
	}
	newURL, newHeaders, _, ferr := fetch(ctx)
	if ferr != nil {
		return err
	}
	return httpGetFile(ctx, newURL, newHeaders, destPath)
}

// probeRangeSupport 发送 Range: bytes=0-0 探测请求验证 206/Content-Range。
// 返回 (totalSize, nil, nil) 表示支持 Range 且已知总长；
// 返回 (0, resp, nil) 表示服务端忽略 Range 返回 200 全量响应（调用方直接消费）；
// 401/403 时刷新凭证重试一次。
func probeRangeSupport(ctx context.Context, creds *driveCredentialState) (int64, *http.Response, error) {
	for attempt := 0; ; attempt++ {
		urlStr, headers, gen := creds.current()
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, urlStr, nil)
		if err != nil {
			return 0, nil, err
		}
		for k, v := range headers {
			req.Header.Set(k, v)
		}
		req.Header.Set("Range", "bytes=0-0")
		resp, err := driveRangeClient.Do(req)
		if err != nil {
			return 0, nil, err
		}
		switch {
		case resp.StatusCode == http.StatusPartialContent:
			total, perr := parseContentRangeTotal(resp.Header.Get("Content-Range"))
			_, _ = io.Copy(io.Discard, resp.Body)
			resp.Body.Close()
			if perr != nil {
				return 0, nil, nil // Content-Range 异常：总长未知，回退整流
			}
			return total, nil, nil
		case resp.StatusCode == http.StatusOK:
			return 0, resp, nil
		case resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden:
			body, _ := io.ReadAll(io.LimitReader(resp.Body, driveTransferBodyErrCap))
			resp.Body.Close()
			if attempt > 0 {
				return 0, nil, fmt.Errorf("下载凭证刷新后仍鉴权失败")
			}
			if rerr := creds.refresh(ctx, gen); rerr != nil && !errors.Is(rerr, errCredentialRefreshVersionUnknown) {
				return 0, nil, fmt.Errorf("重新获取下载凭证失败: %w (原错误: %v)",
					rerr, &httpStatusError{StatusCode: resp.StatusCode, Body: string(body)})
			}
			// errCredentialRefreshVersionUnknown 在探测阶段无需处理（尚无已完成分片）,
			// 凭证已更新，循环继续用新凭证重试探测。
		default:
			body, _ := io.ReadAll(io.LimitReader(resp.Body, driveTransferBodyErrCap))
			resp.Body.Close()
			return 0, nil, &httpStatusError{StatusCode: resp.StatusCode, Body: string(body)}
		}
	}
}

// parseContentRangeTotal 从 "bytes 0-0/12345" 解析总长；"*" 视为未知。
func parseContentRangeTotal(cr string) (int64, error) {
	idx := strings.LastIndex(cr, "/")
	if idx < 0 || idx == len(cr)-1 {
		return 0, fmt.Errorf("非法 Content-Range: %q", cr)
	}
	totalStr := cr[idx+1:]
	if totalStr == "*" {
		return 0, fmt.Errorf("Content-Range 总长未知: %q", cr)
	}
	total, err := strconv.ParseInt(totalStr, 10, 64)
	if err != nil || total <= 0 {
		return 0, fmt.Errorf("非法 Content-Range 总长: %q", cr)
	}
	return total, nil
}

// parseContentRange 解析 "bytes <start>-<end>/<total>" 格式的 Content-Range 头。
// 返回值 start、end 为字节偏移（闭区间），total 为文件总长（"*" 视为 -1）。
func parseContentRange(header string) (start, end, total int64, err error) {
	if header == "" {
		return 0, 0, 0, fmt.Errorf("Content-Range 为空")
	}
	// 去掉 "bytes " 前缀
	const prefix = "bytes "
	if !strings.HasPrefix(header, prefix) {
		return 0, 0, 0, fmt.Errorf("非法 Content-Range 前缀: %q", header)
	}
	rest := header[len(prefix):] // e.g. "0-1048575/104857600"
	// 按 "/" 分割范围与总长
	slashIdx := strings.LastIndex(rest, "/")
	if slashIdx < 0 || slashIdx == len(rest)-1 {
		return 0, 0, 0, fmt.Errorf("非法 Content-Range 格式: %q", header)
	}
	rangePart := rest[:slashIdx]   // "0-1048575"
	totalPart := rest[slashIdx+1:] // "104857600" 或 "*"
	// 解析 total
	if totalPart == "*" {
		total = -1
	} else {
		total, err = strconv.ParseInt(totalPart, 10, 64)
		if err != nil || total <= 0 {
			return 0, 0, 0, fmt.Errorf("非法 Content-Range 总长: %q", header)
		}
	}
	// 按 "-" 分割 start 和 end
	dashIdx := strings.Index(rangePart, "-")
	if dashIdx < 0 {
		return 0, 0, 0, fmt.Errorf("非法 Content-Range 区间: %q", header)
	}
	start, err = strconv.ParseInt(rangePart[:dashIdx], 10, 64)
	if err != nil || start < 0 {
		return 0, 0, 0, fmt.Errorf("非法 Content-Range start: %q", header)
	}
	end, err = strconv.ParseInt(rangePart[dashIdx+1:], 10, 64)
	if err != nil || end < start {
		return 0, 0, 0, fmt.Errorf("非法 Content-Range end: %q", header)
	}
	return start, end, total, nil
}

func writeStreamToFile(r io.Reader, destPath string) error {
	out, err := os.Create(destPath)
	if err != nil {
		return err
	}
	defer out.Close()
	if _, err := io.Copy(out, r); err != nil {
		return err
	}
	return nil
}

// ──────────────────────────────────────────────────────────
// 分片切分与 checkpoint
// ──────────────────────────────────────────────────────────

type driveDownloadPart struct {
	index  int
	offset int64
	length int64
}

// splitDownloadParts 按 partSize 等长切片，末片为余量。
func splitDownloadParts(totalSize, partSize int64) []driveDownloadPart {
	if totalSize <= 0 || partSize <= 0 {
		return nil
	}
	count := int((totalSize + partSize - 1) / partSize)
	parts := make([]driveDownloadPart, 0, count)
	for i := 0; i < count; i++ {
		offset := int64(i) * partSize
		length := partSize
		if offset+length > totalSize {
			length = totalSize - offset
		}
		parts = append(parts, driveDownloadPart{index: i, offset: offset, length: length})
	}
	return parts
}

// driveDownloadCheckpoint 断点续传元信息，随每个分片完成原子落盘。
type driveDownloadCheckpoint struct {
	Version     int    `json:"version"`
	Fingerprint string `json:"fingerprint"`
	TotalSize   int64  `json:"totalSize"`
	PartSize    int64  `json:"partSize"`
	Completed   []bool `json:"completed"`
}

// driveDownloadFingerprint 基于节点 ID + 版本号 + 文件总长 + 资源 URL 计算指纹。
// version>0 时只取 URL path（重签名不影响 checkpoint 复用）；version==0（最新版）时
// 取完整 path+query——中心协议相同 path 可能对应不同实际版本，query 中的签名/token
// 标识了具体资源快照，防止错误复用旧 checkpoint。
// resourceURL 为空时不影响其他字段的指纹计算（安全降级）。
func driveDownloadFingerprint(nodeID string, version int, totalSize int64, resourceURL string) string {
	urlComponent := ""
	if resourceURL != "" {
		if u, err := url.Parse(resourceURL); err == nil && u != nil {
			if version == 0 {
				// 最新版：含 query 以区分不同签名（不同实际版本）
				urlComponent = u.RequestURI()
			} else {
				// 指定版本：只取 path，重签名不应废弃 checkpoint
				urlComponent = u.Path
			}
		}
	}
	sum := sha256.Sum256([]byte(fmt.Sprintf("%s|%d|%d|%s", nodeID, version, totalSize, urlComponent)))
	return hex.EncodeToString(sum[:])
}

// loadDriveDownloadCheckpoint 读取并校验 checkpoint；任一字段不匹配返回 nil（从头下载）。
func loadDriveDownloadCheckpoint(metaPath, fingerprint string, totalSize, partSize int64, partCount int) *driveDownloadCheckpoint {
	data, err := os.ReadFile(metaPath)
	if err != nil {
		return nil
	}
	var cp driveDownloadCheckpoint
	if err := json.Unmarshal(data, &cp); err != nil {
		return nil
	}
	if cp.Version != driveCheckpointVersion || cp.Fingerprint != fingerprint ||
		cp.TotalSize != totalSize || cp.PartSize != partSize || len(cp.Completed) != partCount {
		return nil
	}
	return &cp
}

// save 原子写入（临时文件 + rename），避免中断产生半截 checkpoint。
func (cp *driveDownloadCheckpoint) save(metaPath string) error {
	data, err := driveJsonMarshal(cp)
	if err != nil {
		return err
	}
	tmp := metaPath + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return driveOsRename(tmp, metaPath)
}

// ──────────────────────────────────────────────────────────
// 分片下载引擎
// ──────────────────────────────────────────────────────────

// downloadRangedParts 并发分片下载到 <dest>.dwspart，全部完成后校验总长并
// 原子重命名为 destPath、清理 checkpoint；中途失败保留分片产物供断点续传。
func downloadRangedParts(ctx context.Context, creds *driveCredentialState, destPath string, totalSize int64, opts driveDownloadOptions) error {
	parts := splitDownloadParts(totalSize, opts.partSize)
	partPath := destPath + drivePartFileSuffix
	metaPath := destPath + drivePartMetaSuffix
	// 取首次凭证 URL 用于指纹计算，防止同大小文件覆盖后 checkpoint 错误复用
	initialURL, _, _ := creds.current()
	fingerprint := driveDownloadFingerprint(opts.nodeID, opts.version, totalSize, initialURL)

	var cp *driveDownloadCheckpoint
	if opts.resume {
		cp = loadDriveDownloadCheckpoint(metaPath, fingerprint, totalSize, opts.partSize, len(parts))
		// 分片数据文件缺失或长度不符时 checkpoint 作废，从头下载
		if cp != nil {
			if fi, err := os.Stat(partPath); err != nil || fi.Size() != totalSize {
				cp = nil
			}
		}
	} else {
		// --no-resume：清理历史断点产物，从头下载且不写 checkpoint
		_ = os.Remove(partPath)
		_ = os.Remove(metaPath)
	}
	if cp == nil {
		cp = &driveDownloadCheckpoint{
			Version:     driveCheckpointVersion,
			Fingerprint: fingerprint,
			TotalSize:   totalSize,
			PartSize:    opts.partSize,
			Completed:   make([]bool, len(parts)),
		}
	}

	f, err := os.OpenFile(partPath, os.O_RDWR|os.O_CREATE, 0o644)
	if err != nil {
		return fmt.Errorf("创建分片临时文件失败: %w", err)
	}
	if err := driveFileTruncate(f, totalSize); err != nil {
		f.Close()
		return fmt.Errorf("预分配分片临时文件失败: %w", err)
	}

	remaining := 0
	for _, p := range parts {
		if !cp.Completed[p.index] {
			remaining++
		}
	}
	if opts.logf != nil {
		if remaining < len(parts) {
			opts.logf("断点续传: 共 %d 分片（%s/片，并发 %d），已完成 %d，续传 %d",
				len(parts), formatByteSize(opts.partSize), opts.parallel, len(parts)-remaining, remaining)
		} else {
			opts.logf("分片下载: 共 %d 分片（%s/片，并发 %d）", len(parts), formatByteSize(opts.partSize), opts.parallel)
		}
	}

	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	var (
		mu       sync.Mutex // 保护 cp 与 checkpoint 落盘
		wg       sync.WaitGroup
		errOnce  sync.Once
		firstErr error
	)
	fail := func(err error) {
		errOnce.Do(func() {
			firstErr = err
			cancel()
		})
	}

	jobs := make(chan driveDownloadPart)
	workers := opts.parallel
	if workers > remaining {
		workers = remaining
	}
	if workers < 1 {
		workers = 1
	}
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for part := range jobs {
				if driveWorkerContextErr(runCtx) != nil {
					return
				}
				if err := downloadOnePart(runCtx, creds, f, part, totalSize); err != nil {
					fail(fmt.Errorf("分片 %d/%d 下载失败: %w", part.index+1, len(parts), err))
					return
				}
				mu.Lock()
				cp.Completed[part.index] = true
				var saveErr error
				if opts.resume {
					saveErr = cp.save(metaPath)
				}
				mu.Unlock()
				if saveErr != nil {
					fail(fmt.Errorf("写入下载断点信息失败: %w", saveErr))
					return
				}
			}
		}()
	}

dispatch:
	for _, part := range parts {
		if cp.Completed[part.index] {
			continue
		}
		select {
		case jobs <- part:
		case <-runCtx.Done():
			break dispatch
		}
	}
	close(jobs)
	wg.Wait()

	if firstErr != nil {
		f.Close()
		if errors.Is(firstErr, errCredentialRefreshVersionUnknown) {
			// 激进策略：版本不可验证，清空 checkpoint 和分片文件防止错误续传；
			// 用户重跑时将自然从头下载。
			_ = os.Remove(metaPath)
			_ = os.Remove(partPath)
		}
		return firstErr // 其他错误保留 .dwspart 与 checkpoint，重跑同一命令自动续传
	}
	if err := driveFileSync(f); err != nil {
		f.Close()
		return err
	}
	fi, statErr := driveFileStat(f)
	f.Close()
	if statErr != nil {
		return statErr
	}
	if fi.Size() != totalSize {
		return fmt.Errorf("下载完成但文件长度不符: got %d, want %d", fi.Size(), totalSize)
	}
	if err := driveOsRename(partPath, destPath); err != nil {
		return fmt.Errorf("重命名下载文件失败: %w", err)
	}
	_ = os.Remove(metaPath)
	return nil
}

// downloadOnePart 下载单个分片：常规失败指数退避重试 driveDownloadPartRetries 次；
// 401/403 触发凭证 single-flight 刷新（不计入常规重试，上限 driveDownloadPartAuthRetries），
// 刷新后用新凭证续传，不重下其他已完成分片。
func downloadOnePart(ctx context.Context, creds *driveCredentialState, f *os.File, part driveDownloadPart, totalSize int64) error {
	attempt := 0
	authRetries := 0
	backoff := 500 * time.Millisecond
	for {
		urlStr, headers, gen := creds.current()
		err := fetchRangeInto(ctx, urlStr, headers, f, part, totalSize)
		if err == nil {
			return nil
		}
		if ctx.Err() != nil {
			return err
		}
		if isAuthStatusError(err) && authRetries < driveDownloadPartAuthRetries {
			authRetries++
			if rerr := creds.refresh(ctx, gen); rerr != nil {
				return fmt.Errorf("重新获取下载凭证失败: %w (原错误: %v)", rerr, err)
			}
			continue // 凭证刷新不计常规重试、不退避
		}
		attempt++
		if attempt > driveDownloadPartRetries {
			return err
		}
		select {
		case <-time.After(backoff):
		case <-ctx.Done():
			return err
		}
		backoff *= 2
	}
}

// fetchRangeInto 拉取 [offset, offset+length) 区间并写入文件对应偏移。
func fetchRangeInto(ctx context.Context, urlStr string, headers map[string]string, f *os.File, part driveDownloadPart, expectedTotal int64) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, urlStr, nil)
	if err != nil {
		return err
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	req.Header.Set("Range", fmt.Sprintf("bytes=%d-%d", part.offset, part.offset+part.length-1))
	resp, err := driveRangeClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusPartialContent {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, driveTransferBodyErrCap))
		return &httpStatusError{StatusCode: resp.StatusCode, Body: string(body)}
	}
	// 校验 Content-Range 响应区间与请求分片一致，防止代理/服务端返回错位数据
	cr := resp.Header.Get("Content-Range")
	if cr == "" {
		return fmt.Errorf("分片响应缺少 Content-Range 头，无法验证数据偏移一致性")
	}
	{
		crStart, crEnd, crTotal, crErr := parseContentRange(cr)
		if crErr != nil {
			return fmt.Errorf("Content-Range 解析失败: %w", crErr)
		}
		wantStart := part.offset
		wantEnd := part.offset + part.length - 1
		if crStart != wantStart || crEnd != wantEnd {
			return fmt.Errorf("Content-Range 区间不匹配: 响应 %d-%d, 期望 %d-%d",
				crStart, crEnd, wantStart, wantEnd)
		}
		if crTotal > 0 && expectedTotal > 0 && crTotal != expectedTotal {
			return fmt.Errorf("Content-Range 总长不匹配: 响应 %d, 期望 %d", crTotal, expectedTotal)
		}
	}
	n, err := io.Copy(io.NewOffsetWriter(f, part.offset), io.LimitReader(resp.Body, part.length))
	if err != nil {
		return err
	}
	if n != part.length {
		return fmt.Errorf("分片长度不符: got %d, want %d", n, part.length)
	}
	return nil
}

// ──────────────────────────────────────────────────────────
// 上传：中心协议识别 + 401/403 重试 + 超限可读提示
// ──────────────────────────────────────────────────────────

// parseDriveUploadType 提取 get_upload_info 返回中的 uploadType（可选字段）。
// "httpToCenterWithToken" 表示中心协议；存量 OSS 返回无该字段，返回空串。
func parseDriveUploadType(text string) string {
	var data map[string]any
	if json.Unmarshal([]byte(text), &data) != nil {
		return ""
	}
	if r, ok := data["result"].(map[string]any); ok {
		data = r
	}
	t, _ := data["uploadType"].(string)
	return t
}

// decorateUploadSizeError 为服务端上传超限错误补充可读提示。
// 不做本地文件大小上限校验（上限为服务端动态权益值，本地硬编码会漂移）。
func decorateUploadSizeError(err error, uploadType string) error {
	var se *httpStatusError
	if !errors.As(err, &se) {
		return err
	}
	if se.StatusCode == http.StatusRequestEntityTooLarge ||
		(uploadType == uploadTypeCenterToken && likelySizeLimitBody(se.Body)) {
		return fmt.Errorf("%w\n提示: 文件大小可能超出空间容量或上传上限，请确认文件大小、清理空间或联系管理员调整容量后重试", err)
	}
	return err
}

func likelySizeLimitBody(body string) bool {
	b := strings.ToLower(body)
	if strings.Contains(b, "超限") || strings.Contains(b, "超出") || strings.Contains(b, "容量") {
		return true
	}
	return strings.Contains(b, "size") && (strings.Contains(b, "limit") || strings.Contains(b, "exceed") || strings.Contains(b, "over"))
}

// driveUploadPut 解析上传凭证并执行 HTTP PUT；401/403（token 过期）时通过 refetch
// 重新获取凭证重试一次。返回最终生效凭证的 uploadId（凭证刷新后以新值为准）。
// 中心协议（uploadType=httpToCenterWithToken）与 OSS 走同一路径：resourceUrl 为
// 服务端拼好的完整 PUT URL（客户端零追加），headers（含 dentry-token）原样透传。
func driveUploadPut(ctx context.Context, credText string, refetch func(context.Context) (string, error), filePath string, fileSize int64) (string, error) {
	resourceURL, uploadID, headers, err := parseDriveUploadInfo(credText)
	if err != nil {
		return "", err
	}
	uploadType := parseDriveUploadType(credText)
	putErr := httpPutFile(ctx, resourceURL, headers, filePath, fileSize)
	if putErr != nil && isAuthStatusError(putErr) && refetch != nil {
		text2, rerr := refetch(ctx)
		if rerr == nil {
			if url2, id2, headers2, perr := parseDriveUploadInfo(text2); perr == nil {
				uploadID = id2
				uploadType = parseDriveUploadType(text2)
				putErr = httpPutFile(ctx, url2, headers2, filePath, fileSize)
			}
		}
	}
	if putErr != nil {
		return "", decorateUploadSizeError(putErr, uploadType)
	}
	return uploadID, nil
}
