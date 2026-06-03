package docjsonml

import (
	"fmt"

	"github.com/google/uuid"
)

// ──────────────────────────────────────────────────────────
// JSONML 轻量 Normalize / Auto-fix
//
// 目标：covering 80/20 of agent mistakes that the validator surfaces.
// 重型修复 (table/list 复杂结构) 由服务端 packSerializer.deserialize 兜底，
// 此处不复刻。
//
// 已实现的修复 (spec §3.4)：
//
//	1. 单 block 作为 body 传入 → 包成 `[[block]]` 数组形态。
//	2. block 节点 attrs 缺 uuid → 注入随机 uuid。
//	3. text-bearing block (p, h1-h6) 子节点是裸字符串 →
//	   包装成 `[span,{data-type:text},[span,{data-type:leaf},STR]]`。
//
// 未实现 / 故意不做：
//
//	- 未知 tag 改写：可能误伤；validator 给最相似 tag 提示即可。
//	- 必填属性默认值：当前没有合理可注入的默认值 (toc styles / container.subType
//	  / table.colsWidth 都需要业务判断)。
//	- uuid 注入到「已显式给出空 attrs（{}）」的 block：视为生产者明确意图，
//	  尊重之；只在 attrs 槽完全缺失（如 `["p", "text"]`）时才创建 attrs 并
//	  注入 uuid。真实 `doc read` 输出中常见 `["h1", {}, ...]` 形态，注入会
//	  污染 doc-read → doc-update 回灌。
//
// 关注点分离：
//
//	缺 root wrapper 的修整（["root", {}, ...]）由 EnsureRootWrappedBody 在
//	body 整体形态层面处理，不在本函数职责内 —— 这样 per-block 修复测试矩阵
//	不被协议层包装污染。
// ──────────────────────────────────────────────────────────

// NormalizeJsonMLBody returns a deep-cloned body with safe auto-fixes
// applied, plus human-readable notes describing every change.
//
// notes is empty when no fix was needed. fixed is the parsed JSON-friendly
// payload ready to be marshaled.
func NormalizeJsonMLBody(body []any) (fixed []any, notes []string) {
	working, ok := deepCloneAny(body).([]any)
	if !ok || len(working) == 0 {
		return working, nil
	}

	var collected []string

	// Fix #1 — single block passed as body.
	// "root" is a structural wrapper (handled by the explicit branch below),
	// not a single block to be wrapped — excluding it prevents double-wrap when
	// the schema-driven validBlockTags set lists "root" alongside real blocks.
	if tag, ok := working[0].(string); ok && tag != "root" && validBlockTags[tag] {
		collected = append(collected,
			fmt.Sprintf("$: wrap single %q block as body array", tag))
		working = []any{working}
	}

	startIdx := 0
	if tag, ok := working[0].(string); ok && tag == "root" {
		// Root-wrapped: skip ["root", {attrs}] and recurse into children.
		startIdx = 1
		if len(working) > 1 {
			if _, attrsOk := working[1].(map[string]any); attrsOk {
				startIdx = 2
			}
		}
	}

	for i := startIdx; i < len(working); i++ {
		fixedChild, childNotes := normalizeBlock(working[i], fmt.Sprintf("$[%d]", i))
		working[i] = fixedChild
		collected = append(collected, childNotes...)
	}
	return working, collected
}

// NormalizeJsonMLNode normalizes a single block node (used by
// `doc block insert/update --format jsonml --element <node>`).
func NormalizeJsonMLNode(node any) (fixed any, notes []string) {
	cloned := deepCloneAny(node)
	return normalizeBlock(cloned, "$")
}

// normalizeBlock applies fixes #2 and #3 in-place on a single block node.
// The caller is responsible for handing in a deep-cloned subtree.
func normalizeBlock(node any, path string) (any, []string) {
	arr, ok := node.([]any)
	if !ok || len(arr) == 0 {
		return node, nil
	}
	tag, ok := arr[0].(string)
	if !ok {
		return arr, nil
	}

	var notes []string

	// Fix #2 — inject uuid ONLY when attrs slot is completely missing.
	//
	// 设计：尊重生产者显式给出的 attrs 形态。
	//   - `["p", "text"]`        → attrs 槽缺失，agent 漏写，补 attrs+uuid ✓
	//   - `["p", {}, ...]`       → 已显式给出空 attrs，不动（真实 doc-read 输出常态）
	//   - `["p", {"jc":"left"}]` → attrs 存在但无 uuid，亦不补（生产者既然写了 attrs，应该自己负责 uuid）
	// 这样保证 doc-read → doc-update roundtrip 不会污染原文档节点。
	if validBlockTags[tag] && len(arr) > 1 {
		if _, hadAttrs := arr[1].(map[string]any); !hadAttrs {
			attrs := map[string]any{"uuid": newUUID()}
			arr = append([]any{arr[0], attrs}, arr[1:]...)
			notes = append(notes,
				fmt.Sprintf("%s: insert attrs slot with generated uuid %q", path, attrs["uuid"]))
		}
	}

	childStart := childStartIndex(arr)

	// Fix #3 — text-bearing blocks: wrap raw-string children.
	if isTextBearingBlock(tag) {
		for i := childStart; i < len(arr); i++ {
			if s, isString := arr[i].(string); isString {
				wrapped := wrapTextLeaf(s)
				arr[i] = wrapped
				notes = append(notes,
					fmt.Sprintf("%s[%d]: wrap raw string into span/text/leaf", path, i))
			}
		}
	}

	// Recurse into children that are themselves block nodes (container, refblock, table…)
	for i := childStart; i < len(arr); i++ {
		child, ok := arr[i].([]any)
		if !ok {
			continue
		}
		if len(child) == 0 {
			continue
		}
		childTag, ok := child[0].(string)
		if !ok {
			continue
		}
		// Recurse into block children (container, refblock, table rows/cells —
		// `tr`/`tc` are members of validBlockTags via the schema).
		switch {
		case validBlockTags[childTag]:
			fixed, childNotes := normalizeBlock(child, fmt.Sprintf("%s[%d]", path, i))
			arr[i] = fixed
			notes = append(notes, childNotes...)
		}
	}
	return arr, notes
}

// isTextBearingBlock returns true for block tags whose direct children are
// inline content (and where a bare string is fixable by span-wrapping).
//
// container/refblock/table take block children, not inline — bare strings
// there are a different kind of error and are NOT auto-wrapped.
func isTextBearingBlock(tag string) bool {
	switch tag {
	case "p", "h1", "h2", "h3", "h4", "h5", "h6":
		return true
	}
	return false
}

// wrapTextLeaf builds the canonical text wrapper around a raw string.
//
//	["span",{"data-type":"text"},["span",{"data-type":"leaf"},"<s>"]]
func wrapTextLeaf(s string) []any {
	return []any{
		"span",
		map[string]any{"data-type": "text"},
		[]any{
			"span",
			map[string]any{"data-type": "leaf"},
			s,
		},
	}
}

// newUUID returns a standard RFC 4122 v4 uuid string (e.g.
// "550e8400-e29b-41d4-a716-446655440000"). The server reassigns uuids on
// insert anyway; this exists only to satisfy validators and let agents
// track newly-inserted blocks before the server roundtrip.
func newUUID() string {
	return uuid.NewString()
}

// deepCloneAny clones JSON-shaped values (map[string]any, []any, primitives).
// Required so normalize does not mutate caller-owned input.
func deepCloneAny(v any) any {
	switch x := v.(type) {
	case []any:
		out := make([]any, len(x))
		for i, e := range x {
			out[i] = deepCloneAny(e)
		}
		return out
	case map[string]any:
		out := make(map[string]any, len(x))
		for k, val := range x {
			out[k] = deepCloneAny(val)
		}
		return out
	default:
		return v
	}
}
