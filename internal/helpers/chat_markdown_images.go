package helpers

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"path/filepath"
	"strings"
	"time"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/text"
)

// MarkdownImageOptions controls the local-file boundary for a sending entry point.
type MarkdownImageOptions struct {
	SafePath func(string) (string, error)
	DryRun   bool
	Upload   func(context.Context, *personalEmotionImage) (string, error)
}

type MarkdownImageReport struct {
	ImageCount      int
	LocalCount      int
	UniqueCount     int
	CompressedCount int
}

var markdownImageAbsPath = filepath.Abs
var markdownImageSpan = markdownImageDestinationSpan

// ResolveMarkdownLocalImages preserves source Markdown and changes only image destinations.
func ResolveMarkdownLocalImages(ctx context.Context, body string, opts MarkdownImageOptions) (string, MarkdownImageReport, error) {
	refs, report, err := collectMarkdownImageRefs(body, opts.SafePath)
	if err != nil || len(refs) == 0 {
		return body, report, err
	}
	if report.LocalCount == 0 {
		return replaceMarkdownImageRefs(body, refs), report, nil
	}
	if report.LocalCount > 9 {
		return "", report, fmt.Errorf("单条消息最多支持 9 张本地图片")
	}
	if opts.DryRun {
		unique := make(map[string]struct{})
		for _, ref := range refs {
			if ref.path != "" {
				unique[ref.path] = struct{}{}
			}
		}
		report.UniqueCount = len(unique)
		return body, report, nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()
	images, paths, err := prepareMarkdownImages(refs)
	if err != nil {
		return "", report, err
	}
	for _, image := range images {
		if image.compressed {
			report.CompressedCount++
		}
	}
	report.UniqueCount = len(images)
	mediaIDs, err := uploadMarkdownImages(ctx, images, opts.Upload)
	if err != nil {
		return "", report, markdownImageUploadError(err)
	}
	for i := range refs {
		if refs[i].path != "" {
			refs[i].mediaID = mediaIDs[paths[refs[i].path]]
		}
	}
	return replaceMarkdownImageRefs(body, refs), report, nil
}

func collectMarkdownImageRefs(body string, safePath func(string) (string, error)) ([]markdownImageRef, MarkdownImageReport, error) {
	var report MarkdownImageReport
	if !strings.Contains(body, "![") {
		return nil, report, nil
	}
	source := []byte(body)
	root := goldmark.DefaultParser().Parse(text.NewReader(source))
	var refs []markdownImageRef
	err := ast.Walk(root, func(node ast.Node, entering bool) (ast.WalkStatus, error) {
		image, ok := node.(*ast.Image)
		if !entering || !ok {
			return ast.WalkContinue, nil
		}
		report.ImageCount++
		kind, value, classifyErr := classifyMarkdownImageDestination(string(image.Destination))
		if classifyErr != nil {
			return ast.WalkStop, classifyErr
		}
		if kind == "remote" {
			return ast.WalkContinue, nil
		}
		if image.Reference != nil {
			return ast.WalkStop, fmt.Errorf("引用式本地图片暂不支持，请使用 ![说明](路径) 行内写法")
		}
		start, end, spanErr := markdownImageSpan(source, image.Pos())
		if spanErr != nil {
			return ast.WalkStop, spanErr
		}
		ref := markdownImageRef{start: start, end: end}
		if kind == "media" {
			ref.mediaID = value
		} else {
			if safePath != nil {
				resolved, pathErr := safePath(value)
				if pathErr != nil {
					if filepath.IsAbs(value) {
						return ast.WalkStop, errors.New("本地图片路径校验失败：需要相对路径。请根据 DWS 进程工作目录转换附件路径")
					}
					return ast.WalkStop, errors.New("本地图片路径校验失败。请检查路径格式及符号链接，并确认文件位于 DWS 进程工作目录内")
				}
				value = resolved
			}
			absolute, spanErr := markdownImageAbsPath(value)
			if spanErr != nil {
				return ast.WalkStop, errors.New("无法解析本地图片路径。请检查 DWS 进程工作目录是否可访问")
			}
			ref.path = filepath.Clean(absolute)
			report.LocalCount++
		}
		refs = append(refs, ref)
		return ast.WalkContinue, nil
	})
	return refs, report, err
}

// 盘符路径在 URL 解析前识别，避免把 C: 当成网络协议。
func markdownWindowsDrivePath(path string) bool {
	return len(path) >= 3 && ((path[0] >= 'A' && path[0] <= 'Z') || (path[0] >= 'a' && path[0] <= 'z')) && path[1] == ':' && (path[2] == '/' || path[2] == '\\')
}

func classifyMarkdownImageDestination(dest string) (string, string, error) {
	if (strings.HasPrefix(dest, "@") || strings.HasPrefix(dest, "$")) && !strings.ContainsAny(dest, "/\\") {
		return "media", dest, nil
	}
	if strings.HasPrefix(dest, "mediaid:") {
		id := strings.TrimPrefix(dest, "mediaid:")
		if id == "" {
			return "", "", fmt.Errorf("图片 mediaid 不能为空")
		}
		return "media", id, nil
	}
	if markdownWindowsDrivePath(dest) {
		path, err := url.PathUnescape(dest)
		if err != nil {
			return "", "", fmt.Errorf("图片地址格式无效")
		}
		return "local", filepath.FromSlash(path), nil
	}
	parsed, err := url.Parse(dest)
	if err != nil {
		return "", "", fmt.Errorf("图片地址格式无效")
	}
	switch strings.ToLower(parsed.Scheme) {
	case "http", "https":
		return "remote", dest, nil
	case "file":
		if parsed.Host != "" && parsed.Host != "localhost" {
			return "", "", fmt.Errorf("file 图片地址只允许本机")
		}
		// url.Parse already decodes Path once; a second decode would change literal %20 filenames.
		path := parsed.Path
		if path == "" {
			return "", "", fmt.Errorf("file 图片路径无效")
		}
		// 标准 Windows file URI 的 /C:/ 前缀映射回本机盘符路径。
		if strings.HasPrefix(path, "/") && markdownWindowsDrivePath(path[1:]) {
			path = path[1:]
		}
		return "local", filepath.FromSlash(path), nil
	case "":
		if dest == "" {
			return "", "", fmt.Errorf("本地图片路径不能为空")
		}
		// url.Parse validated escapes; decode the full filename so literal ?/# remain part of it.
		path, _ := url.PathUnescape(dest)
		return "local", path, nil
	default:
		return "", "", fmt.Errorf("不支持的图片地址协议: %s", parsed.Scheme)
	}
}
