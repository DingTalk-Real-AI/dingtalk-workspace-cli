package helpers

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/cli"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
)

// ──────────────────────────────────────────────────────────
// dws markdown diff
// ──────────────────────────────────────────────────────────

// diffResult 是 diff 命令的输出结构。
type diffResult struct {
	Mode         string `json:"mode"`
	Changed      bool   `json:"changed"`
	AddedLines   int    `json:"added_lines"`
	DeletedLines int    `json:"deleted_lines"`
	Hunks        int    `json:"hunks"`
	Diff         string `json:"diff"`
}

// diff 命令的限制（包级变量以便覆盖测试注入更小阈值）。
var (
	maxDiffFileSize        int64 = 10 * 1024 * 1024 // 单侧文件大小上限 10MB
	diffDownloadTimeout          = 10 * time.Minute // 下载远端内容超时（与项目其他下载命令一致）
	diffComputeTimeout           = 30 * time.Second // 本地 diff 计算超时
	diffJSONMarshalIndent        = json.MarshalIndent
	runMarkdownUnifiedDiff       = computeUnifiedDiff
)

// formatFileSize 返回人类可读的文件大小。
func formatFileSize(size int64) string {
	if size >= 1024*1024 {
		return fmt.Sprintf("%.1f MB", float64(size)/float64(1024*1024))
	}
	if size >= 1024 {
		return fmt.Sprintf("%.1f KB", float64(size)/float64(1024))
	}
	return fmt.Sprintf("%d B", size)
}

// checkFileSize 校验文件大小是否超过限制。
func checkFileSize(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if info.Size() > maxDiffFileSize {
		return fmt.Errorf("文件大小 %s 超过限制 %s，请使用更小的文件", formatFileSize(info.Size()), formatFileSize(maxDiffFileSize))
	}
	return nil
}

// downloadRemoteContent 下载远端文件内容并返回其文本。
// versionNum <= 0 时走 download_file（用 fileId），> 0 时走 download_file_version（用 nodeId + version）。
func downloadRemoteContent(ctx context.Context, fileID string, versionNum int) (string, error) {
	var text string
	var err error
	if versionNum > 0 {
		text, err = callMCPToolReturnTextOnServer(ctx, "drive", "download_file_version", map[string]any{
			"nodeId":  fileID,
			"version": versionNum,
		})
	} else {
		text, err = callMCPToolReturnTextOnServer(ctx, "drive", "download_file", map[string]any{
			"fileId": fileID,
		})
	}
	if err != nil {
		return "", err
	}

	resourceURL, dlHeaders, err := parseDownloadInfo(text)
	if err != nil {
		return "", err
	}

	// 下载时即限制大小，避免完整下载超大文件后才拦截（约束网络流量与内存占用）
	content, err := diffDownloadLimited(ctx, resourceURL, dlHeaders)
	if err != nil {
		return "", err
	}
	return string(content), nil
}

// diffDownloadLimited 下载远端内容，并在下载过程中强制执行 maxDiffFileSize 上限。
// 包级变量以便测试注入。
var diffDownloadLimited = defaultDiffDownloadLimited

// defaultDiffDownloadLimited 通过 HTTP GET 下载内容：先用 Content-Length 预检，
// 再用 io.LimitReader 将实际读取量限制为 maxDiffFileSize+1 字节，超限即报错，
// 从而约束网络流量与内存占用，而非在完整下载后才校验。
func defaultDiffDownloadLimited(ctx context.Context, url string, headers map[string]string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	client := &http.Client{Timeout: diffDownloadTimeout}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(body))
	}

	// Content-Length 预检：可在读取 body 前提前拦截超大文件
	if resp.ContentLength > maxDiffFileSize {
		return nil, fmt.Errorf("远端文件大小 %s 超过限制 %s，请使用更小的文件", formatFileSize(resp.ContentLength), formatFileSize(maxDiffFileSize))
	}

	// 实际读取限制为 maxDiffFileSize+1 字节，读满即判定超限（防止 Content-Length 缺失或造假）
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxDiffFileSize+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > maxDiffFileSize {
		return nil, fmt.Errorf("远端文件大小超过限制 %s，请使用更小的文件", formatFileSize(maxDiffFileSize))
	}
	return data, nil
}

// computeUnifiedDiff 使用 Go stdlib patience diff 算法计算 unified diff，并统计变更行数。
func computeUnifiedDiff(left, right string, contextLines int) (string, int, int, int, bool) {
	out := UnifiedDiff("left", []byte(left), "right", []byte(right), contextLines)
	if len(out) == 0 {
		return "", 0, 0, 0, false
	}

	text := string(out)
	added, deleted, hunks := 0, 0, 0
	// 只统计首个 @@ 之后的 hunk 区行：头部三行（diff/---/+++）不参与计数，
	// hunk 区内每行必带单字符前缀，内容行以 --/++ 开头也不会被误判为文件头而漏计
	inHunk := false
	for _, line := range strings.Split(text, "\n") {
		if strings.HasPrefix(line, "@@") {
			hunks++
			inHunk = true
			continue
		}
		if !inHunk {
			continue
		}
		if strings.HasPrefix(line, "+") {
			added++
		} else if strings.HasPrefix(line, "-") {
			deleted++
		}
	}
	changed := added > 0 || deleted > 0
	return text, added, deleted, hunks, changed
}

// ensureMarkdownDiffType 校验 markdown diff 的目标文件类型。
// markdown 产品域面向 .md 文件，非 md 文件拦截并回引到对应产品命令。
// 类型探测复用 fetchFileInfo（显式路由 drive server 的 get_file_info），
// 探测失败或类型未知时不阻断，让后续 MCP 工具自行报错。
func ensureMarkdownDiffType(ctx context.Context, nodeID string) error {
	info := fetchFileInfo(ctx, nodeID)
	switch info.extension {
	case "", "md", "markdown":
		return nil
	case "adoc":
		return fmt.Errorf("该文件为钉钉在线文档 (adoc)，不支持 markdown diff\n请使用 dws doc 对应命令（如 dws doc version list / dws doc export）")
	case "axls":
		return fmt.Errorf("该文件为钉钉在线表格 (axls)，不支持 markdown diff\n请使用 dws sheet 对应命令")
	case "amind", "adraw":
		return fmt.Errorf("该文件为钉钉在线%s (%s)，暂不支持历史版本管理\nmarkdown diff 与 dws drive list --versions / dws drive download --version 均不支持该类型", describeDingTalkDocType(info.extension), info.extension)
	default:
		return fmt.Errorf("该文件为 %s 文件，markdown diff 仅支持 .md 文件\n普通文件的历史版本请使用 dws drive list --versions / dws drive download --version / dws drive revert", info.extension)
	}
}

// newMarkdownDiffCmd 创建 markdown diff 子命令。
func newMarkdownDiffCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "diff",
		Short: "比较 Markdown 内容差异",
		Long: `比较远端 Markdown 文件的两个版本，或远端版本与本地文件，生成 unified diff。

模式:
  remote_vs_remote: --version V1 --version2 V2     (两个历史版本)
  remote_vs_remote: --version V1                   (历史版本 vs 最新)
  remote_vs_local:  --file ./local.md              (最新 vs 本地)
  remote_vs_local:  --version V1 --file ./local.md (历史版本 vs 本地)

历史版本号通过 dws drive list --versions 获取。
--file 与 --version2 不能同时使用。

限制:
  - 仅支持 .md 文件（在线文档/表格请使用 doc/sheet 命令）
  - 单侧文件大小上限: 10 MB
  - 下载超时: 10 分钟
  - diff 计算超时: 30 秒`,
		Example: `  # 比较两个历史版本
  dws markdown diff --node <dentryUuid> --version 3 --version2 5

  # 历史版本 vs 最新版本
  dws markdown diff --node <dentryUuid> --version 3

  # 最新版本 vs 本地文件
  dws markdown diff --node <dentryUuid> --file ./draft.md

  # 历史版本 vs 本地文件
  dws markdown diff --node <dentryUuid> --version 3 --file ./draft.md`,
		RunE: func(cmd *cobra.Command, args []string) error {
			nodeID, err := mustFlagOrFallback(cmd, "node", "url", "id", "node-id", "doc-id", "file-id")
			if err != nil {
				return err
			}
			version1, _ := cmd.Flags().GetInt("version")
			version2, _ := cmd.Flags().GetInt("version2")
			localFile, _ := cmd.Flags().GetString("file")
			contextLines, _ := cmd.Flags().GetInt("context")

			// fail-fast 参数校验（置于 dry-run 之前）：显式传了版本号但非正整数时立即报错，
			// 避免静默降级为“最新版本”；--context 允许 0（无上下文），仅拒绝负值
			if cmd.Flags().Changed("version") && version1 <= 0 {
				return fmt.Errorf("--version 必须为正整数，当前值: %d", version1)
			}
			if cmd.Flags().Changed("version2") && version2 <= 0 {
				return fmt.Errorf("--version2 必须为正整数，当前值: %d", version2)
			}
			if contextLines < 0 {
				return fmt.Errorf("--context 不能为负数，当前值: %d", contextLines)
			}

			// 互斥校验：--file 与 --version2 不能同时使用
			if localFile != "" && version2 > 0 {
				return fmt.Errorf("--file 与 --version2 不能同时使用")
			}

			// 模式判定
			var mode string
			if localFile != "" {
				mode = "remote_vs_local"
			} else {
				mode = "remote_vs_remote"
			}

			// remote_vs_remote 模式至少需要一个版本号，否则两侧均取最新版本，diff 必为空
			if localFile == "" && version1 == 0 && version2 == 0 {
				return fmt.Errorf("remote_vs_remote 模式至少需要指定 --version 或 --version2 之一")
			}

			if deps.Caller.DryRun() {
				deps.Out.PrintKeyValue("操作", "Markdown 内容 Diff")
				deps.Out.PrintKeyValue("模式", mode)
				deps.Out.PrintKeyValue("节点ID", nodeID)
				if version1 > 0 {
					deps.Out.PrintKeyValue("左侧版本", fmt.Sprintf("%d", version1))
				} else {
					deps.Out.PrintKeyValue("左侧版本", "最新")
				}
				if localFile != "" {
					deps.Out.PrintKeyValue("右侧", localFile)
				} else if version2 > 0 {
					deps.Out.PrintKeyValue("右侧版本", fmt.Sprintf("%d", version2))
				} else {
					deps.Out.PrintKeyValue("右侧版本", "最新")
				}
				return nil
			}

			// 下载超时 context（与项目其他下载命令一致：10 分钟）
			ctx, cancel := context.WithTimeout(context.Background(), diffDownloadTimeout)
			defer cancel()

			// 类型守卫置于 dry-run 之后，确保 dry-run 不产生任何服务端调用
			if err := ensureMarkdownDiffType(ctx, nodeID); err != nil {
				return err
			}

			// remote_vs_local 模式：先校验本地文件大小，避免下载后才发现过大
			if localFile != "" {
				if err := checkFileSize(localFile); err != nil {
					return err
				}
			}

			// 输出格式读全局 --format（默认 json），json 时输出结构化结果，其余输出文本摘要
			isJSON := deps.Caller.Format() == "json"
			// 进度属带外诊断信息，统一写入 stderr，保证 stdout 在两种模式下都是纯净输出
			progress := func(msg string) { fmt.Fprintln(os.Stderr, msg) }

			// 下载左侧内容
			progress("[1/3] 获取左侧内容...")
			leftContent, err := downloadRemoteContent(ctx, nodeID, version1)
			if err != nil {
				return err
			}

			// 获取右侧内容
			var rightContent string
			if localFile != "" {
				// remote_vs_local: 读取本地文件（大小已校验）
				progress("[2/3] 读取本地文件...")
				data, err := os.ReadFile(localFile)
				if err != nil {
					return fmt.Errorf("读取本地文件失败: %w", err)
				}
				rightContent = string(data)
			} else {
				// remote_vs_remote: 下载右侧远端内容
				progress("[2/3] 获取右侧内容...")
				rightContent, err = downloadRemoteContent(ctx, nodeID, version2)
				if err != nil {
					return err
				}
			}

			// 计算 diff（带超时保护）
			progress("[3/3] 计算差异...")
			type diffOutput struct {
				text    string
				added   int
				deleted int
				hunks   int
				changed bool
			}
			resultCh := make(chan diffOutput, 1)
			// Capture seams before the goroutine so test restorers cannot race
			// against a still-running compute after the timeout path returns.
			computeDiff := runMarkdownUnifiedDiff
			marshalIndent := diffJSONMarshalIndent
			go func() {
				text, added, deleted, hunks, changed := computeDiff(leftContent, rightContent, contextLines)
				resultCh <- diffOutput{text, added, deleted, hunks, changed}
			}()
			select {
			case res := <-resultCh:
				diffText, added, deleted, hunks, changed := res.text, res.added, res.deleted, res.hunks, res.changed
				result := diffResult{
					Mode:         mode,
					Changed:      changed,
					AddedLines:   added,
					DeletedLines: deleted,
					Hunks:        hunks,
					Diff:         diffText,
				}

				// 输出
				if isJSON {
					data, err := marshalIndent(result, "", "  ")
					if err != nil {
						return fmt.Errorf("JSON 序列化失败: %w", err)
					}
					deps.Out.PrintRaw(string(data))
				} else {
					deps.Out.PrintKeyValue("模式", result.Mode)
					if result.Changed {
						deps.Out.PrintKeyValue("是否有变更", "是")
					} else {
						deps.Out.PrintKeyValue("是否有变更", "否")
					}
					deps.Out.PrintKeyValue("新增行数", fmt.Sprintf("%d", result.AddedLines))
					deps.Out.PrintKeyValue("删除行数", fmt.Sprintf("%d", result.DeletedLines))
					deps.Out.PrintKeyValue("差异块数", fmt.Sprintf("%d", result.Hunks))
					if result.Changed {
						deps.Out.PrintRaw("")
						deps.Out.PrintRaw(result.Diff)
					}
				}
			case <-time.After(diffComputeTimeout):
				return fmt.Errorf("diff 计算超时（%s），文件可能过大，请尝试减小 --context 或使用更小的文件", diffComputeTimeout)
			}
			return nil
		},
	}

	cmd.Flags().String("node", "", "文件 ID (dentryUuid) 或 URL (必填)")
	cmd.Flags().Int("version", 0, "左侧历史版本号 (可选，不传=最新版本)")
	cmd.Flags().Int("version2", 0, "右侧历史版本号 (可选，不传=最新版本；不能与 --file 同时使用)")
	cmd.Flags().String("file", "", "本地 .md 文件路径 (可选，指定后进入 remote_vs_local 模式)")
	cmd.Flags().Int("context", 3, "diff 上下文行数 (默认 3)")

	// --node 隐藏别名（与 version/fetch 子命令一致）
	cmd.Flags().String("url", "", "")
	cmd.Flags().String("id", "", "")
	cmd.Flags().String("node-id", "", "")
	cmd.Flags().String("doc-id", "", "")
	cmd.Flags().String("file-id", "", "")
	_ = cmd.Flags().MarkHidden("url")
	_ = cmd.Flags().MarkHidden("id")
	_ = cmd.Flags().MarkHidden("node-id")
	_ = cmd.Flags().MarkHidden("doc-id")
	_ = cmd.Flags().MarkHidden("file-id")

	RegisterCrossProductAliases(cmd)

	cli.AnnotateRuntimeRequiredFlags(cmd, "node")
	DeclareLeafMetadata(cmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "read", Risk: "low",
			Confirmation: "not_required", Idempotency: "idempotent",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "markdown",
				Name:           "diff",
				CanonicalPath:  "markdown.diff",
				CLIPath:        "markdown diff",
				PrimaryCLIPath: "markdown diff",
			},
			Description: "比较远端 Markdown 文件的两个版本，或远端版本与本地文件，生成 unified diff",
			DryRun:      &contract.DryRunSpec{PreviewKind: "plan", RemoteReads: false},
			Interface: &contract.InterfaceSpec{
				Mode:         "composite",
				Availability: "available",
				Reason:       "Local diff workflow: download remote version(s) and/or read a local file, then compute unified diff client-side.",
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "比较 Markdown 文件版本或本地草稿差异",
				UseWhen:      []string{"需要对比远端 .md 历史版本，或远端最新/历史版本与本地草稿的差异时"},
				AvoidWhen:    []string{"在线文档/表格差异请用 doc/sheet；普通二进制文件版本请用 drive list --versions / drive download --version"},
				Examples:     []string{"dws markdown diff --node <nodeId> --version 3 --version2 5"},
			},
			Parameters: []contract.ParamDecl{
				{Name: "node", Property: "nodeId", Required: boolPtr(true)},
				{Name: "version", Property: "version", InterfaceType: "integer"},
				{Name: "version2", Property: "version2", InterfaceType: "integer"},
				{Name: "file", Property: "file"},
				{Name: "context", Property: "context", InterfaceType: "integer"},
			},
		},
	})

	return cmd
}

// fetchFileInfo 通过 get_file_info 获取扩展名（markdown diff 类型守卫用）。
// 探测失败返回零值，由调用方决定是否阻断。
type markdownFileInfo struct {
	name      string
	extension string
}

func fetchFileInfo(ctx context.Context, nodeID string) (info markdownFileInfo) {
	text, err := callMCPToolReturnTextOnServer(ctx, "drive", "get_file_info", map[string]any{"fileId": nodeID})
	if err != nil {
		return
	}
	var resp map[string]any
	if err := json.Unmarshal([]byte(text), &resp); err != nil {
		return
	}
	data := resp
	if result, ok := resp["result"].(map[string]any); ok {
		data = result
	}
	if name, ok := data["name"].(string); ok {
		info.name = name
	}
	if ext, ok := data["extension"].(string); ok {
		info.extension = strings.ToLower(strings.TrimPrefix(ext, "."))
	}
	return
}

func describeDingTalkDocType(ext string) string {
	switch strings.ToLower(ext) {
	case "adoc":
		return "文档"
	case "axls":
		return "表格"
	case "amind":
		return "脑图"
	case "adraw":
		return "画图"
	default:
		return "文件"
	}
}
