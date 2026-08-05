// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package doc

import (
	"fmt"
	"strings"
	"unicode/utf16"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
)

var Review = shortcut.Shortcut{
	Service: "doc", Command: "+review", Product: productComment,
	Description: "聚合未解决评论、引用原文和块上下文",
	Intent:      "当用户要确定性查看一篇文档仍待处理的 review 意见时使用；聚合 unresolved 评论与 block 上下文，不调用模型生成总结。",
	Risk:        shortcut.RiskRead,
	Safety:      contract.SafetySpec{Effect: "read", Risk: "low", Confirmation: "not_required", Idempotency: "idempotent"},
	Contract: docContract("+review", "聚合未解决评论、引用原文和块上下文",
		"当用户要确定性查看一篇文档仍待处理的 review 意见时使用；聚合 unresolved 评论与 block 上下文，不调用模型生成总结。",
		[]string{`dws doc +review --node <DOC_ID>`}),
	Flags: []shortcut.Flag{{Name: "node", Type: shortcut.FlagString, Desc: "文档 ID 或 URL", Required: true}},
	Tips:  []string{`dws doc +review --node <DOC_ID>`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		node := rt.Str("node")
		comments, err := rt.CallMCPData(productComment, "list_comments", map[string]any{"nodeId": node, "resolveStatus": "unresolved"})
		if err != nil {
			return err
		}
		blocks, err := rt.CallMCPData(productDoc, "list_document_blocks", map[string]any{"nodeId": node, "format": "element"})
		if err != nil {
			return err
		}
		items := projectReviewComments(comments, blocks)
		global, inline := 0, 0
		for _, item := range items {
			if item["blockId"] == "" {
				global++
			} else {
				inline++
			}
		}
		return rt.Output(map[string]any{"status": "unresolved", "counts": map[string]any{"total": len(items), "global": global, "inline": inline}, "comments": items})
	},
}

var CommentUpdate = shortcut.Shortcut{
	Service: "doc", Command: "+comment-update", Product: productComment,
	Description: "更新指定文档评论正文和 mention",
	Intent:      "当用户要修改一条已有评论的文字内容或 @ 用户列表，且已知 commentKey 时使用；不会创建回复或改变解决状态。",
	Risk:        shortcut.RiskWrite,
	Safety:      contract.SafetySpec{Effect: "write", Risk: "medium", Confirmation: "not_required", Idempotency: "unknown"},
	Contract: docContract("+comment-update", "更新指定文档评论正文和 mention",
		"当用户要修改一条已有评论的文字内容或 @ 用户列表，且已知 commentKey 时使用；不会创建回复或改变解决状态。",
		[]string{`dws doc +comment-update --node <DOC_ID> --comment-key <COMMENT_KEY> --content "已按最新数据修正"`}),
	Flags: []shortcut.Flag{
		{Name: "node", Type: shortcut.FlagString, Desc: "文档 ID 或 URL", Required: true},
		{Name: "comment-key", Type: shortcut.FlagString, Desc: "评论 commentKey", Required: true},
		{Name: "content", Type: shortcut.FlagString, Desc: "更新后的评论正文", Required: true},
		{Name: "mention", Type: shortcut.FlagStringSlice, Desc: "被 @ 的用户 uid 列表"},
	},
	Tips: []string{`dws doc +comment-update --node <DOC_ID> --comment-key <COMMENT_KEY> --content "已按最新数据修正"`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		params := map[string]any{"nodeId": rt.Str("node"), "commentKey": rt.Str("comment-key"), "content": rt.Str("content")}
		if rt.Changed("mention") {
			params["mentionedUserIds"] = rt.StrSlice("mention")
		}
		return rt.CallMCP("update_comment", params)
	},
}

var CommentDelete = shortcut.Shortcut{
	Service: "doc", Command: "+comment-delete", Product: productComment,
	Description: "永久删除指定文档评论",
	Intent:      "当用户明确要求永久删除某条文档评论，且已核对 node 与 commentKey 时使用；不可用于标记 resolved。",
	Risk:        shortcut.RiskHighWrite,
	Safety:      contract.SafetySpec{Effect: "destructive", Risk: "high", Confirmation: "user_required", Idempotency: "unknown"},
	Contract: docContract("+comment-delete", "永久删除指定文档评论",
		"当用户明确要求永久删除某条文档评论，且已核对 node 与 commentKey 时使用；不可用于标记 resolved。",
		[]string{`dws doc +comment-delete --node <DOC_ID> --comment-key <COMMENT_KEY>`}),
	Flags: []shortcut.Flag{
		{Name: "node", Type: shortcut.FlagString, Desc: "文档 ID 或 URL", Required: true},
		{Name: "comment-key", Type: shortcut.FlagString, Desc: "评论 commentKey", Required: true},
	},
	Tips: []string{`dws doc +comment-delete --node <DOC_ID> --comment-key <COMMENT_KEY>`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		params := map[string]any{"nodeId": rt.Str("node"), "commentKey": rt.Str("comment-key")}
		if rt.DryRun() {
			return rt.Output(docEnvelope("doc.comment_delete", map[string]any{"executed": false, "params": params}))
		}
		return rt.CallMCP("delete_comment", params)
	},
}

func validateCommentCreate(rt *shortcut.RuntimeContext) error {
	hasBlock := rt.Str("block-id") != ""
	hasOffsets := rt.Changed("start") || rt.Changed("end")
	if hasBlock != hasOffsets || (hasOffsets && (!rt.Changed("start") || !rt.Changed("end"))) {
		return apperrors.NewValidation("--block-id、--start、--end 必须一起提供")
	}
	if hasBlock && rt.Int("end") <= rt.Int("start") {
		return apperrors.NewValidation("--end 必须大于 --start")
	}
	if hasBlock && rt.Str("selection") != "" {
		return apperrors.NewValidation("--selection 与 block-id/start/end 高级通道不能同时提供")
	}
	return nil
}

func executeCommentCreate(rt *shortcut.RuntimeContext) error {
	params := map[string]any{"nodeId": rt.Str("node"), "content": rt.Str("content")}
	if rt.Changed("mention") {
		params["mentionedUserIds"] = rt.StrSlice("mention")
	}
	if rt.Str("block-id") != "" {
		params["blockId"], params["start"], params["end"] = rt.Str("block-id"), rt.Int("start"), rt.Int("end")
		if rt.Str("selected-text") != "" {
			params["selectedText"] = rt.Str("selected-text")
		}
		return rt.CallMCP("create_inline_comment", params)
	}
	if selection := rt.Str("selection"); selection != "" {
		blocks, err := rt.CallMCPData(productDoc, "list_document_blocks", map[string]any{"nodeId": rt.Str("node"), "format": "element"})
		if err != nil {
			return err
		}
		matches := findSelectionMatches(blocks, selection)
		if len(matches) != 1 {
			candidates := make([]map[string]any, 0, len(matches))
			for _, match := range matches {
				candidates = append(candidates, map[string]any{"blockId": match.blockID, "excerpt": match.text})
			}
			return apperrors.NewValidation(fmt.Sprintf("AMBIGUOUS_SELECTION: selection 需要唯一匹配，实际 %d 处；候选=%v", len(matches), candidates))
		}
		params["blockId"], params["start"], params["end"], params["selectedText"] = matches[0].blockID, matches[0].start, matches[0].end, matches[0].selected
		return rt.CallMCP("create_inline_comment", params)
	}
	return rt.CallMCP("create_comment", params)
}

type selectionMatch struct {
	blockID        string
	text, selected string
	start, end     int
}

func findSelectionMatches(value any, selection string) []selectionMatch {
	if selection == "" {
		return nil
	}
	var prefix, suffix string
	omitted := false
	if parts := strings.SplitN(selection, "...", 2); len(parts) == 2 {
		prefix, suffix = parts[0], parts[1]
		omitted = true
	}
	var out []selectionMatch
	appendMatch := func(blockID, text string, startByte, endByte int) {
		out = append(out, selectionMatch{
			blockID: blockID, text: text, selected: text[startByte:endByte],
			start: utf16Length(text[:startByte]), end: utf16Length(text[:endByte]),
		})
	}
	var walk func(any, string)
	walk = func(current any, inheritedID string) {
		switch typed := current.(type) {
		case map[string]any:
			blockID := blockIdentity(typed, inheritedID)
			if text, ok := typed["text"].(string); ok && blockID != "" {
				if !omitted {
					for searchStart := 0; searchStart < len(text); {
						index := strings.Index(text[searchStart:], selection)
						if index < 0 {
							break
						}
						startByte := searchStart + index
						endByte := startByte + len(selection)
						appendMatch(blockID, text, startByte, endByte)
						searchStart = startByte + 1
					}
				} else {
					prefixStarts := []int{0}
					if prefix != "" {
						prefixStarts = nil
						for searchStart := 0; searchStart < len(text); {
							index := strings.Index(text[searchStart:], prefix)
							if index < 0 {
								break
							}
							startByte := searchStart + index
							prefixStarts = append(prefixStarts, startByte)
							searchStart = startByte + 1
						}
					}
					for _, startByte := range prefixStarts {
						suffixSearch := startByte + len(prefix)
						if suffix == "" {
							if startByte < len(text) {
								appendMatch(blockID, text, startByte, len(text))
							}
							continue
						}
						for suffixSearch <= len(text) {
							index := strings.Index(text[suffixSearch:], suffix)
							if index < 0 {
								break
							}
							suffixStart := suffixSearch + index
							appendMatch(blockID, text, startByte, suffixStart+len(suffix))
							suffixSearch = suffixStart + 1
						}
					}
				}
			}
			for _, child := range typed {
				walk(child, blockID)
			}
		case []any:
			for _, child := range typed {
				walk(child, inheritedID)
			}
		}
	}
	walk(value, "")
	return out
}

func utf16Length(value string) int {
	return len(utf16.Encode([]rune(value)))
}

func projectReviewComments(comments, blocks map[string]any) []map[string]any {
	blockText := map[string]string{}
	collectBlockText(blocks, "", blockText)
	var out []map[string]any
	var walk func(any)
	walk = func(value any) {
		switch typed := value.(type) {
		case map[string]any:
			key := firstString(typed, "commentKey", "commentId")
			if key != "" {
				blockID := firstString(typed, "blockId")
				selectedText := firstString(typed, "selectedText", "quote")
				// list_comments currently omits blockId for inline comments but
				// includes isGlobal=false plus the selected quote. Resolve the
				// block deterministically when that quote is unique.
				if blockID == "" && selectedText != "" {
					if matches := findSelectionMatches(blocks, selectedText); len(matches) == 1 {
						blockID = matches[0].blockID
					}
				}
				out = append(out, map[string]any{
					"commentKey":   key,
					"content":      firstString(typed, "content", "text"),
					"selectedText": selectedText,
					"blockId":      blockID,
					"context":      blockText[blockID],
					"replies":      typed["replies"],
				})
				return
			}
			for _, child := range typed {
				walk(child)
			}
		case []any:
			for _, child := range typed {
				walk(child)
			}
		}
	}
	walk(comments)
	return out
}

func collectBlockText(value any, inheritedID string, out map[string]string) {
	switch typed := value.(type) {
	case map[string]any:
		blockID := blockIdentity(typed, inheritedID)
		if text, ok := typed["text"].(string); ok && blockID != "" {
			out[blockID] = text
		}
		for _, child := range typed {
			collectBlockText(child, blockID, out)
		}
	case []any:
		for _, child := range typed {
			collectBlockText(child, inheritedID, out)
		}
	}
}

func firstString(values map[string]any, keys ...string) string {
	for _, key := range keys {
		if text, ok := values[key].(string); ok {
			return text
		}
	}
	return ""
}

func init() {
	shortcut.Register(Review, CommentUpdate, CommentDelete)
}
