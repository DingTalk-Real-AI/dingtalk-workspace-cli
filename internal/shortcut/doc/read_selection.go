// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0
package doc

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
)

func documentTopBlocks(data map[string]any) ([]any, error) {
	raw := nestedString(data, "jsonml")
	if raw == "" {
		return nil, apperrors.NewAPI("读取缺少完整JSONML，不能进行本地章节/块范围计算")
	}
	var root []any
	if json.Unmarshal([]byte(raw), &root) != nil || len(root) < 2 || root[0] != "root" {
		return nil, apperrors.NewAPI("需要完整JSONML root，不能把fragment或未知结构当成全文")
	}
	seen := map[string]bool{}
	for _, v := range root[2:] {
		b, ok := v.([]any)
		if !ok || len(b) < 2 {
			return nil, apperrors.NewAPI("文档顶层块结构无效")
		}
		id := jsonMLBlockIdentity(b)
		if id == "" || seen[id] {
			return nil, apperrors.NewAPI("文档顶层块缺少唯一ID")
		}
		seen[id] = true
	}
	return root[2:], nil
}
func jsonMLPlainText(v any) string {
	var out strings.Builder
	var walk func(any)
	walk = func(v any) {
		switch x := v.(type) {
		case string:
			out.WriteString(x)
		case []any:
			if len(x) > 2 {
				for _, c := range x[2:] {
					walk(c)
				}
			}
		}
	}
	walk(v)
	return out.String()
}
func headingLevel(v any) int {
	b, ok := v.([]any)
	if !ok || len(b) < 2 {
		return 0
	}
	tag, _ := b[0].(string)
	if len(tag) == 2 && tag[0] == 'h' {
		n, _ := strconv.Atoi(tag[1:])
		if n >= 1 && n <= 6 {
			return n
		}
	}
	return 0
}
func selectDocChapter(data map[string]any, startID string, before, after int) (map[string]any, error) {
	blocks, err := documentTopBlocks(data)
	if err != nil {
		return nil, err
	}
	start := -1
	for i, b := range blocks {
		if jsonMLBlockIdentity(b.([]any)) == startID {
			start = i
			break
		}
	}
	if start < 0 {
		return nil, apperrors.NewValidation("章节标题block ID未找到")
	}
	level := headingLevel(blocks[start])
	if level == 0 {
		return nil, apperrors.NewValidation("chapter起点必须是标题块")
	}
	end := start + 1
	for end < len(blocks) {
		l := headingLevel(blocks[end])
		if l > 0 && l <= level {
			break
		}
		end++
	}
	lo, hi := max(0, start-before), min(len(blocks), end+after)
	fragment := append([]any{"fragment", map[string]any{"scope": "chapter", "startBlockId": startID}}, blocks[lo:hi]...)
	encoded, _ := json.Marshal(fragment)
	result := map[string]any{}
	for k, v := range data {
		result[k] = v
	}
	result["jsonml"] = string(encoded)
	result["selectedBlockCount"] = hi - lo
	return result, nil
}
func projectKeywordBlocks(data map[string]any, query string, useRegex bool, before, after int) (map[string]any, error) {
	blocks, err := documentTopBlocks(data)
	if err != nil {
		return nil, err
	}
	match := func(s string) bool {
		for _, part := range stringSliceNonEmpty(strings.Split(query, "|")) {
			if strings.Contains(strings.ToLower(s), strings.ToLower(part)) {
				return true
			}
		}
		return false
	}
	if useRegex {
		rx, err := regexp.Compile("(?i)" + query)
		if err != nil {
			return nil, apperrors.NewValidation("无效正则: " + err.Error())
		}
		match = rx.MatchString
	}
	matches := []map[string]any{}
	for i, b := range blocks {
		if !match(jsonMLPlainText(b)) {
			continue
		}
		lo, hi := max(0, i-before), min(len(blocks), i+after+1)
		id := jsonMLBlockIdentity(b.([]any))
		matches = append(matches, map[string]any{"blockId": id, "content": jsonMLPlainText(b), "contextBlocks": blocks[lo:hi], "contextUnit": "blocks"})
	}
	return map[string]any{"matches": matches, "count": len(matches), "contextUnit": "blocks"}, nil
}
func validateDocSelection(rt *shortcut.RuntimeContext) error {
	if rt.Str("scope") == "chapter" && rt.Changed("max-depth") {
		return apperrors.NewValidation("chapter返回完整子树，不接受max-depth截断")
	}
	if rt.Str("context-unit") == "blocks" && rt.Str("scope") != "keyword" && rt.Str("scope") != "chapter" {
		return apperrors.NewValidation("blocks上下文仅支持keyword/chapter")
	}

	if rt.Str("scope") == "chapter" && rt.Str("start-block-id") == "" {
		return apperrors.NewValidation("--scope chapter需要--start-block-id")
	}
	if rt.Bool("regex") && rt.Str("scope") != "keyword" {
		return apperrors.NewValidation("--regex仅支持keyword范围")
	}
	if rt.Bool("regex") && rt.Str("context-unit") != "blocks" {
		return apperrors.NewValidation("--regex需要--context-unit blocks；保留旧字符上下文行为")
	}
	if rt.Bool("regex") {
		if len(rt.Str("keyword")) > 4096 {
			return apperrors.NewValidation("正则过长")
		}
		if _, err := regexp.Compile("(?i)" + rt.Str("keyword")); err != nil {
			return apperrors.NewValidation(fmt.Sprintf("无效正则: %v", err))
		}
	}
	if rt.Str("context-unit") == "blocks" || rt.Str("scope") == "chapter" {
		if rt.Int("context-before") < 0 || rt.Int("context-after") < 0 {
			return apperrors.NewValidation("块上下文数量不得为负")
		}
	}
	return nil
}

func projectDocComments(data map[string]any) []map[string]any {
	items := findDocCommentItems(data)
	result := make([]map[string]any, 0, len(items))
	for _, item := range items {
		if row, ok := item.(map[string]any); ok {
			result = append(result, row)
		}
	}
	return result
}
func findDocCommentItems(data map[string]any) []any {
	for _, key := range []string{"commentList", "comments", "items", "list"} {
		if items, ok := data[key].([]any); ok {
			return items
		}
	}
	for _, key := range []string{"data", "result"} {
		if child, ok := data[key].(map[string]any); ok {
			if items := findDocCommentItems(child); items != nil {
				return items
			}
		}
	}
	return nil
}

func readCompleteDocComments(rt *shortcut.RuntimeContext, node string) (map[string]any, error) {
	var shapeErr error
	result, err := collectDocPages(rt, "list_comments", "comments", map[string]any{"nodeId": node}, func(data map[string]any) []map[string]any {
		raw := findDocCommentItems(data)
		if raw == nil {
			shapeErr = apperrors.NewAPI("评论响应缺少显式列表，不能视作零评论")
		}
		rows := projectDocComments(data)
		if len(rows) != len(raw) {
			shapeErr = apperrors.NewAPI("评论列表含无法识别的项")
		}
		for _, row := range rows {
			if nestedString(row, "commentKey") == "" {
				shapeErr = apperrors.NewAPI("评论缺少稳定commentKey")
			}
		}
		return rows
	}, docPageOptions{Product: productComment, PageAll: true, PageSize: 50, MaxPages: 20, MaxItems: 1000, PageSizeParam: "pageSize", CursorParam: "nextToken"})
	if err != nil {
		return nil, err
	}
	if shapeErr != nil {
		return nil, shapeErr
	}
	return result, nil
}
