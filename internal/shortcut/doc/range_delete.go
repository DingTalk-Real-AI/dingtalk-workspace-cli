// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0
package doc

import (
	"encoding/json"
	"fmt"
	"strings"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
)

func validateDocBlockRange(rt *shortcut.RuntimeContext) error {
	start, end := rt.Str("start-block-id"), rt.Str("end-block-id")
	if start == "" && end == "" {
		return nil
	}
	if rt.Str("command") != "block_delete" && rt.Str("command") != "block_replace" {
		return apperrors.NewValidation("起止范围仅支持block_delete或block_replace")
	}
	if start == "" || end == "" || rt.Str("block-id") != "" {
		return apperrors.NewValidation("起止范围必须成对，且不能与block-id混用")
	}
	return nil
}

// Range replacement is deliberately sequential: replace the first root block,
// verify it, then remove the remaining selected blocks. It never rewrites the
// whole document or claims an atomic transaction.
func executeDocRangeReplace(rt *shortcut.RuntimeContext, node, content string) error {
	if rt.DryRun() {
		return rt.Output(docEnvelope("doc.range_replace", map[string]any{"executed": false, "atomic": false, "startBlockId": rt.Str("start-block-id"), "endBlockId": rt.Str("end-block-id"), "steps": []string{"resolve_range", "replace_first", "verify_first", "delete_remaining", "verify_remaining"}}))
	}
	data, err := rt.CallMCPData(productDoc, "get_document_content", map[string]any{"nodeId": node, "format": "jsonml"})
	if err != nil {
		return err
	}
	ids, err := docRangeIDs(data, rt.Str("start-block-id"), rt.Str("end-block-id"))
	if err != nil {
		return err
	}
	params := map[string]any{"nodeId": node, "blockId": ids[0]}
	format := rt.Str("doc-format")
	if format == "jsonml" {
		params["format"], params["jsonml"] = "jsonml", content
	} else {
		params["element"] = map[string]any{"blockType": "paragraph", "paragraph": map[string]any{"text": content}}
	}
	first, err := runVerifiedDocMutation(rt, "doc.range_replace", "update_document_block", params, node, "list_document_blocks", map[string]any{"nodeId": node, "format": blockVerificationFormat(format), "__allBlocks": true}, func(_, read map[string]any) bool { return blockContentEquals(read, ids[0], content, format) })
	if err != nil {
		return err
	}
	if len(ids) > 1 {
		_, err = runVerifiedDocMutation(rt, "doc.range_replace", "delete_document_block", map[string]any{"nodeId": node, "blockId": strings.Join(ids[1:], ",")}, node, "list_document_blocks", map[string]any{"nodeId": node, "format": blockVerificationFormat(format), "__allBlocks": true}, func(_, read map[string]any) bool {
			if !blockContentEquals(read, ids[0], content, format) {
				return false
			}
			for _, id := range ids[1:] {
				if findCanonicalBlock(read, id, format) != nil {
					return false
				}
			}
			return true
		})
		if err != nil {
			return docPartialWriteError("doc.range_replace", "doc_range_replace_partial", "delete_remaining", "首块已替换；剩余块删除未确认，请按原ID读回，不要重试整次替换", err, map[string]any{"nodeId": node, "replacedBlockId": ids[0], "remainingBlockIds": ids[1:], "firstStep": first}, nil, map[string]any{"available": false, "reason": "inspect exact selected IDs before recovery"})
		}
	}
	return rt.Output(docEnvelope("doc.range_replace", map[string]any{"nodeId": node, "atomic": false, "verified": true, "replacedBlockId": ids[0], "deletedBlockIds": ids[1:]}))
}

func executeDocMultiCopy(rt *shortcut.RuntimeContext, node string) error {
	ids, err := docCopyIDs(rt.Str("block-id"))
	if err != nil {
		return err
	}
	data, err := rt.CallMCPData(productDoc, "get_document_content", map[string]any{"nodeId": node, "format": "jsonml"})
	if err != nil {
		return err
	}
	ref := rt.Str("after-block-id")
	top, err := documentTopBlocks(data)
	if err != nil {
		return err
	}
	byID := map[string][]any{}
	for _, value := range top {
		b := value.([]any)
		byID[jsonMLBlockIdentity(b)] = b
	}
	if byID[ref] == nil {
		return apperrors.NewValidation("复制目标锚点不存在")
	}
	// Resolve every source before the first write, including duplicates and resources.
	sources := make([][]any, 0, len(ids))
	for _, id := range ids {
		b := byID[id]
		if b == nil {
			return apperrors.NewValidation("复制源块不存在: " + id)
		}
		if containsResourceReference(b) {
			return apperrors.NewValidation("含资源引用的块不能直接复制，请先迁移资源")
		}
		sources = append(sources, b)
	}
	completed := []map[string]any{}
	for i, source := range sources {
		expected := canonicalBlockContent(source, "jsonml")
		stripBlockIDs(source)
		// source was decoded from JSON and only had identity keys removed.
		encoded, _ := json.Marshal(source)
		step, err := runVerifiedDocMutation(rt, "doc.multi_copy", "insert_document_block", map[string]any{"nodeId": node, "referenceBlockId": ref, "where": "after", "format": "jsonml", "jsonml": string(encoded)}, node, "list_document_blocks", map[string]any{"nodeId": node, "format": "jsonml", "__allBlocks": true}, func(result, read map[string]any) bool {
			return verifyInsertedCanonicalBlock(result, read, ref, "after", expected, "jsonml", 0)
		})
		if err != nil {
			progress := map[string]any{"nodeId": node, "completed": completed, "failedSourceId": ids[i]}
			if step != nil {
				progress["lastStep"] = step
			}
			return docPartialWriteError("doc.multi_copy", "doc_multi_copy_partial", "copy", fmt.Sprintf("第%d个源块复制未确认；不要重试整个序列", i+1), err, progress, nil, map[string]any{"available": false, "reason": "inspect completed/new IDs before retrying remaining sources"})
		}
		newID := nestedString(step, "blockId", "elementId")
		if newID == "" {
			return docPartialWriteError("doc.multi_copy", "doc_multi_copy_missing_id", "resolve_inserted_id", "已写入但响应缺少新块ID，停止后续复制", nil, map[string]any{"completed": completed, "lastStep": step}, nil, nil)
		}
		completed = append(completed, map[string]any{"sourceBlockId": ids[i], "newBlockId": newID})
		ref = newID
	}
	return rt.Output(docEnvelope("doc.multi_copy", map[string]any{"nodeId": node, "atomic": false, "verified": true, "copies": completed}))
}

func docCopyIDs(raw string) ([]string, error) {
	ids := strings.Split(raw, ",")
	if len(ids) > 20 {
		return nil, apperrors.NewValidation("一次最多复制20个块")
	}
	seen := map[string]bool{}
	for i, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" || seen[id] {
			return nil, apperrors.NewValidation("复制源ID不能为空或重复")
		}
		seen[id], ids[i] = true, id
	}
	return ids, nil
}
func docRangeIDs(data map[string]any, start, end string) ([]string, error) {
	blocks, err := documentTopBlocks(data)
	if err != nil {
		return nil, err
	}
	lo, hi := -1, -1
	if start == "0" {
		lo = 0
	}
	if end == "-1" {
		hi = len(blocks) - 1
	}
	for i, b := range blocks {
		id := jsonMLBlockIdentity(b.([]any))
		if id == start {
			lo = i
		}
		if id == end {
			hi = i
		}
	}
	if lo < 0 || hi < lo || hi >= len(blocks) {
		return nil, apperrors.NewValidation("范围锚点不存在或顺序无效")
	}
	if hi-lo+1 > 50 {
		return nil, apperrors.NewValidation("一次范围删除最多50个顶层块")
	}
	ids := []string{}
	for _, b := range blocks[lo : hi+1] {
		ids = append(ids, jsonMLBlockIdentity(b.([]any)))
	}
	return ids, nil
}
func executeDocRangeDelete(rt *shortcut.RuntimeContext, node string) error {
	if rt.DryRun() {
		return rt.Output(docEnvelope("doc.update", map[string]any{"executed": false, "nodeId": node, "command": "block_delete", "startBlockId": rt.Str("start-block-id"), "endBlockId": rt.Str("end-block-id"), "resolution": "read_at_execution"}))
	}
	data, err := rt.CallMCPData(productDoc, "get_document_content", map[string]any{"nodeId": node, "format": "jsonml"})
	if err != nil {
		return err
	}
	ids, err := docRangeIDs(data, rt.Str("start-block-id"), rt.Str("end-block-id"))
	if err != nil {
		return err
	}
	return executeVerifiedDocMutation(rt, "doc.update", "delete_document_block", map[string]any{"nodeId": node, "blockId": strings.Join(ids, ",")}, node, "list_document_blocks", map[string]any{"nodeId": node, "format": "element", "__allBlocks": true}, func(_, read map[string]any) bool {
		for _, id := range ids {
			if findBlock(read, id) != nil {
				return false
			}
		}
		return true
	})
}
