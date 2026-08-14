package helpers

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

const (
	// --latest 上限与 --limit 服务端每页硬上限 50 对齐。
	driveLatestMax = 50
	// 钉盘单层 latest 扫描上限（50×20 页）。
	driveLatestScanMax = 1000
)

// validateDriveListLatest --latest 边界与互斥校验。
func validateDriveListLatest(cmd *cobra.Command, latest int) error {
	if latest < 1 || latest > driveLatestMax {
		return &CLIError{
			Code:    CodeInvalidParam,
			Message: fmt.Sprintf("--latest 必须为 1~%d 的整数，当前: %d", driveLatestMax, latest),
		}
	}
	for _, f := range []string{"order-by", "order"} {
		if cmd.Flags().Changed(f) {
			return driveLatestExclusiveError(f, latest)
		}
	}
	if cmd.Flags().Changed("limit") || cmd.Flags().Changed("max") {
		return driveLatestExclusiveError("limit", latest)
	}
	if v := flagOrFallback(cmd, "cursor", "next-token"); v != "" {
		return driveLatestExclusiveError("cursor", latest)
	}
	return nil
}

func driveLatestExclusiveError(flag string, latest int) error {
	return &CLIError{
		Code:    CodeInvalidParam,
		Message: fmt.Sprintf("--latest 不能与 --%s 同时使用：Top-N 排序语义由 latest 独占；如需自定义排序请改用 --order-by modifyTime --order desc --limit %d", flag, latest),
	}
}

// foldersOnly=true 时对目录取 Top-N（--type folder --latest 组合），否则对文件取 Top-N。
func applyDriveListLatest(items []map[string]any, latest int, foldersOnly bool) []map[string]any {
	files := make([]map[string]any, 0, len(items))
	for _, item := range items {
		if (isDriveDepthFolder(item) || isDocDepthFolder(item)) != foldersOnly {
			continue
		}
		files = append(files, item)
	}
	sort.SliceStable(files, func(i, j int) bool {
		ti, _ := files[i]["sortTime"].(int64)
		tj, _ := files[j]["sortTime"].(int64)
		if ti != tj {
			return ti > tj
		}
		ri, _ := files[i]["rel_path"].(string)
		rj, _ := files[j]["rel_path"].(string)
		if ri != rj {
			return ri < rj
		}
		return driveDepthItemID(files[i]) < driveDepthItemID(files[j])
	})
	if len(files) > latest {
		files = files[:latest]
	}
	return files
}

func stripDriveDepthDecorations(items []map[string]any) {
	for _, item := range items {
		delete(item, "depth")
		delete(item, "parentId")
		delete(item, "rel_path")
		delete(item, "sortTime")
	}
}

func driveItemModifiedMillis(item map[string]any) (int64, bool) {
	for _, k := range []string{"modifiedTime", "modifyTime", "modified_time", "gmtModified", "lastModifiedTime", "updateTime"} {
		if v, ok := item[k]; ok {
			if ms, ok := toMillis(v); ok {
				return ms, true
			}
		}
	}
	return 0, false
}

func toMillis(v any) (int64, bool) {
	switch t := v.(type) {
	case float64:
		if t <= 0 {
			return 0, false
		}
		return int64(t), true
	case json.Number:
		if n, err := t.Int64(); err == nil && n > 0 {
			return n, true
		}
	case string:
		s := strings.TrimSpace(t)
		if s == "" {
			return 0, false
		}
		if n, err := strconv.ParseInt(s, 10, 64); err == nil && n > 0 {
			return n, true
		}
		if tm, err := time.Parse(time.RFC3339, s); err == nil {
			return tm.UnixMilli(), true
		}
	}
	return 0, false
}

// runDriveListLatest 钉盘单层 --latest 扫描。
func runDriveListLatest(cmd *cobra.Command, baseArgs map[string]any, rootFolder string, latest int, pattern string, quiet bool) error {
	buildArgs := func(pageToken string) map[string]any {
		args := map[string]any{
			"maxResults": float64(driveDepthPageSize),
			"orderBy":    "modifyTime",
			"order":      "desc",
		}
		for k, v := range baseArgs {
			args[k] = v
		}
		if rootFolder != "" {
			args["parentId"] = rootFolder
		}
		if pageToken != "" {
			args["nextToken"] = pageToken
		}
		return args
	}
	if deps.Caller.DryRun() {
		return deps.Out.PrintJSON(map[string]any{
			"tool":   "list_files",
			"args":   buildArgs(""),
			"latest": latest,
			"note":   "dry-run：latest 为客户端能力，凑够 N 条即停，最多扫描 1000 条",
		})
	}
	ctx := cmd.Context()
	if ctx == nil {
		ctx = context.Background()
	}
	collected := make([]map[string]any, 0, latest)
	scanned := 0
	pageToken := ""
	maxPages := driveLatestScanMax/driveDepthPageSize + 1
	for pages := 0; pages < maxPages; pages++ {
		text, err := callMCPToolReturnText(ctx, "list_files", buildArgs(pageToken))
		if err != nil {
			return fmt.Errorf("latest 扫描第 %d 页失败: %w", pages+1, err)
		}
		items, next, _ := parseDriveDepthPage(text)
		for _, item := range items {
			scanned++
			if isDriveDepthFolder(item) {
				continue
			}
			name, _ := item["name"].(string)
			if name == "" {
				name, _ = item["fileName"].(string)
			}
			if pattern != "" && !matchDriveNamePattern(name, pattern) {
				continue
			}
			collected = append(collected, item)
			if len(collected) >= latest {
				break
			}
		}
		if len(collected) >= latest || next == "" || scanned >= driveLatestScanMax {
			break
		}
		pageToken = next
		if !quiet {
			fmt.Fprintf(os.Stderr, "[drive-list] latest 扫描中: 已扫 %d 条，命中 %d/%d\n", scanned, len(collected), latest)
		}
	}
	if len(collected) < latest {
		hint := fmt.Sprintf("dws drive list --folder <子目录ID> --latest %d", latest)
		if pattern != "" {
			hint = fmt.Sprintf("dws drive list --folder <子目录ID> --pattern %q --latest %d", pattern, latest)
		}
		fmt.Fprintf(os.Stderr, "[drive-list] 已扫描 %d 条，找到 %d/%d 条；建议缩小范围：%s\n", scanned, len(collected), latest, hint)
	}
	return deps.Out.PrintJSON(map[string]any{"items": collected})
}
