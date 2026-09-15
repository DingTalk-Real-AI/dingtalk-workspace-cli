// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package whiteboard

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
	whiteboardcore "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/whiteboard"
)

const (
	defaultDiffDetailLimit = 100
	maximumDiffDetailLimit = 1000
	maximumDiffOutputBytes = 2 * 1024 * 1024
)

func diffResultSpec() *contract.ResultSpec {
	return &contract.ResultSpec{
		Outcomes: []contract.ResultOutcome{contract.ResultOutcomeSuccess, contract.ResultOutcomeFailure},
		DataSchema: json.RawMessage(`{
			"type":"object",
			"description":"当前白板页面与 proposed OpenNodes 更新的完整本地预检摘要及受预算约束的确定性明细",
			"properties":{
				"comparison":{"type":"string","const":"current_vs_proposed","description":"当前快照与 proposed source 的比较方向"},
				"mode":{"type":"string","enum":["append","overwrite"],"description":"proposed source 的真实更新模式"},
				"complete":{"type":"boolean","const":true,"description":"快照和全部分类计数均完整"},
				"identityStrategy":{"type":"string","enum":["explicit_id_map","unmatched"],"description":"逻辑节点身份匹配策略"},
				"target":{"type":"object","description":"本次读取和比较的稳定白板目标及可用 revision","additionalProperties":true},
				"comparisonPolicy":{"type":"object","description":"字段规范化与数字比较策略","additionalProperties":true},
				"summary":{"type":"object","description":"基于完整集合计算的 execution、logical、media 与风险计数","additionalProperties":true},
				"executionDiff":{"type":"object","description":"真实 append 或 overwrite 执行会新增和删除的节点明细，以及保留计数","additionalProperties":true},
				"logicalDiff":{"type":"object","description":"显式身份映射支持的逻辑新增、修改、删除、未匹配明细与 unchanged 计数","additionalProperties":true},
				"media":{"type":"object","description":"Vector、Icon 及 query-only 媒体变化明细","additionalProperties":true},
				"preflight":{"type":"object","description":"本地预检能力边界；服务端仍需执行最终校验","additionalProperties":true},
				"blockers":{"type":"array","description":"受全局预算约束的逐节点阻断证据","items":{"type":"object","additionalProperties":true}},
				"warnings":{"type":"array","description":"受全局预算约束的逐节点或目标级告警证据","items":{"type":"object","additionalProperties":true}},
				"blockerSummary":{"type":"array","description":"不截断的按 reason 聚合阻断计数","items":{"type":"object","additionalProperties":true}},
				"warningSummary":{"type":"array","description":"不截断的按 reason 聚合告警计数","items":{"type":"object","additionalProperties":true}},
				"detailTotal":{"type":"integer","minimum":0,"description":"预算投影前的全部变化明细记录数"},
				"detailReturned":{"type":"integer","minimum":0,"description":"实际返回的变化明细记录数"},
				"detailLimit":{"type":"integer","minimum":1,"maximum":1000,"description":"调用方指定的全局变化明细条数预算"},
				"detailsTruncated":{"type":"boolean","description":"条数或 2 MiB 上限是否裁剪了明细"},
				"summaryComplete":{"type":"boolean","const":true,"description":"摘要和分类总数始终基于完整集合"},
				"truncationReasons":{"type":"array","description":"发生明细裁剪的稳定原因","items":{"type":"string","enum":["detail_limit","payload_bytes"]}},
				"detailCounts":{"type":"object","description":"每个明细类别的完整 total 和实际 returned","additionalProperties":true},
				"sourceDigest":{"type":"string","description":"规范化 proposed source 的 sha256 摘要，可传给 +update --expected-source-digest"},
				"snapshotDigest":{"type":"string","description":"本次完整 Query 快照的规范化 sha256 摘要"}
			},
			"required":["comparison","mode","complete","identityStrategy","target","comparisonPolicy","summary","executionDiff","logicalDiff","media","preflight","blockers","warnings","blockerSummary","warningSummary","detailTotal","detailReturned","detailLimit","detailsTruncated","summaryComplete","truncationReasons","detailCounts","sourceDigest","snapshotDigest"],
			"additionalProperties":false
		}`),
		SensitivePaths: []string{
			"target.nodeId", "target.partId", "target.pageId",
			"executionDiff.added.id", "executionDiff.deleted.id",
			"logicalDiff.modified.beforeId", "logicalDiff.modified.afterId",
			"logicalDiff.modified.changedFields.before", "logicalDiff.modified.changedFields.after",
			"media.added.resourceId", "media.removed.resourceId",
			"media.replaced.beforeResourceId", "media.replaced.afterResourceId",
			"media.metadataChanged.beforeUrl", "media.metadataChanged.afterUrl",
			"media.unsupported.resourceId", "blockers.id", "warnings.id",
		},
	}
}

// Diff reads one complete page and compares it locally with a proposed update.
var Diff = shortcut.Shortcut{
	OutputRollout: output.RolloutUnifiedActive,
	Service:       "whiteboard",
	Command:       "+diff",
	Product:       serverWhiteboard,
	Description:   "读取当前白板并预览 proposed OpenNodes 更新的节点、媒体与风险变化",
	Intent:        "在执行 whiteboard +update 前读取同一页面并展示真实执行影响；有显式 identity map 时同时展示逻辑修改",
	Risk:          shortcut.RiskRead,
	Safety:        whiteboardReadSafety(),
	Contract: whiteboardContract(
		"+diff", "shortcut_diff", "读取当前白板并预览 proposed OpenNodes 更新的节点、媒体与风险变化",
		"Reviewed CLI-only composite adapter reads exactly one complete embedded or standalone page, then computes deterministic execution, logical, media and risk differences locally without invoking any write tool.",
		diffResultSpec(), nil,
		[]contract.ParamDecl{
			{Name: "node", Property: "nodeId"},
			{Name: "part-id", Property: "partId"},
			{Name: "page-id", Property: "pageId", RequiredWhen: "操作独立白板时"},
			{Name: "source", Property: "source"},
			{Name: "identity-map", Property: "identityMap"},
			{Name: "comparison", Property: "comparison", Enum: []string{"semantic", "exact"}},
			{Name: "detail-limit", Property: "detailLimit"},
		},
		"在 whiteboard +update 前比较当前页面与 proposed source，审阅新增、删除、逻辑修改、媒体变化、warning 和 blocker",
		"只读取快照使用 whiteboard +query；真正写入使用 whiteboard +update；大结果只有下载 URL 时首版会失败关闭",
		"dws whiteboard +diff --node <WHITEBOARD_NODE_ID> --page-id <PAGE_ID> --source @whiteboard.json",
	),
	Flags: []shortcut.Flag{
		{Name: "node", Type: shortcut.FlagString, Desc: "承载文档或独立白板的节点 ID/URL；去除空白后不能为空", Required: true},
		{Name: "part-id", Type: shortcut.FlagString, Desc: "文档内白板 part ID；显式非空时选择内嵌分支并禁止 page-id"},
		{Name: "page-id", Type: shortcut.FlagString, Desc: "独立白板页面 ID；未提供 part-id 时必填，内嵌分支禁止提供", RequiredWhen: "操作独立白板时"},
		{Name: "source", Type: shortcut.FlagString, Desc: "proposed OpenNodes V1 update JSON；支持字面量、@相对文件或 - 从 stdin 读取", Required: true, Input: []string{"file", "stdin"}},
		{Name: "identity-map", Type: shortcut.FlagString, Desc: "可选 version=1 逻辑 ID 到当前真实节点 ID 的显式映射；支持字面量、@相对文件或 - 从 stdin 读取", Input: []string{"file", "stdin"}},
		{Name: "comparison", Type: shortcut.FlagString, Default: "semantic", Enum: []string{"semantic", "exact"}, Desc: "比较策略；semantic 对 x/y 允许 0.5px 规范化偏差，exact 对所有数字精确比较"},
		{Name: "detail-limit", Type: shortcut.FlagInt, Default: strconv.Itoa(defaultDiffDetailLimit), Desc: "整个响应的变化明细预算，范围 1..1000；完整摘要不受影响，序列化 data 仍受 2 MiB 硬上限约束"},
	},
	Constraints: []shortcut.Constraint{
		{Kind: shortcut.ConstraintCustom, Flags: []string{"part-id", "page-id"}, Description: "显式非空 part-id 选择内嵌分支并禁止 page-id；未提供 part-id 时选择独立分支并要求非空 page-id"},
		{Kind: shortcut.ConstraintCustom, Flags: []string{"source"}, Description: "source 必须是可由 +update 接受的单一 OpenNodes V1 update 对象"},
		{Kind: shortcut.ConstraintCustom, Flags: []string{"detail-limit"}, Description: "detail-limit 必须在 1..1000 范围内"},
	},
	Tips: []string{
		"dws whiteboard +diff --node <DOC_ID> --part-id <WHITEBOARD_PART_ID> --source @whiteboard.json",
		"dws whiteboard +diff --node <WHITEBOARD_NODE_ID> --page-id <PAGE_ID> --source @whiteboard.json --identity-map @identity-map.json",
	},
	Validate: func(rt *shortcut.RuntimeContext) error {
		_, err := parseWhiteboardSource(rt.Str("source"))
		if err != nil {
			return err
		}
		if _, err := whiteboardDiffQueryCall(rt); err != nil {
			return err
		}
		if _, err := parseIdentityMap(rt.Str("identity-map"), rt.Changed("identity-map")); err != nil {
			return err
		}
		return validateDiffDetailLimit(rt.Int("detail-limit"))
	},
	Execute: func(rt *shortcut.RuntimeContext) error {
		parsed, err := parseWhiteboardSource(rt.Str("source"))
		if err != nil {
			return err
		}
		call, err := whiteboardDiffQueryCall(rt)
		if err != nil {
			return err
		}
		identity, err := parseIdentityMap(rt.Str("identity-map"), rt.Changed("identity-map"))
		if err != nil {
			return err
		}
		limit := rt.Int("detail-limit")
		if err := validateDiffDetailLimit(limit); err != nil {
			return err
		}
		data, err := rt.CallMCPData(serverWhiteboard, call.Tool, call.Args)
		if err != nil {
			return err
		}
		var projected map[string]any
		if call.Kind == whiteboardcore.KindEmbedded {
			projected, err = projectWhiteboardQuery(data, rt.Str("node"), rt.Str("part-id"))
		} else {
			projected, err = projectStandaloneWhiteboardQuery(data, call.Args)
		}
		if err != nil {
			return err
		}
		if _, downloadOnly := projected["resultDownloadUrl"]; downloadOnly {
			return apperrors.NewAPI(
				"当前白板结果过大，+diff 首版不能取得完整快照",
				apperrors.WithReason("snapshot_download_required"),
				apperrors.WithHint("请缩小白板页面后重试；首版不会下载 resultDownloadUrl，也不会返回不完整 diff"),
				apperrors.WithRetryable(false),
			)
		}
		result, err := buildWhiteboardDiff(projected, call, parsed, identity, rt.Str("comparison"), limit)
		if err != nil {
			return err
		}
		return rt.Output(result)
	},
}

func whiteboardDiffQueryCall(rt *shortcut.RuntimeContext) (whiteboardcore.Call, error) {
	target := whiteboardcore.Target{
		NodeID: rt.Str("node"), PartID: rt.Str("part-id"), PartIDChanged: rt.Changed("part-id"),
	}
	kind, err := whiteboardcore.ResolveKind(target)
	if err != nil {
		return whiteboardcore.Call{}, err
	}
	if kind == whiteboardcore.KindEmbedded {
		return whiteboardcore.BuildQueryCall(whiteboardcore.QueryOptions{
			Target: target, PageID: rt.Str("page-id"), PageIDChanged: rt.Changed("page-id"),
		})
	}
	return whiteboardcore.BuildQueryCall(whiteboardcore.QueryOptions{
		Target: target, View: "page", ViewChanged: true,
		PageID: rt.Str("page-id"), PageIDChanged: rt.Changed("page-id"),
	})
}

func validateDiffDetailLimit(limit int) error {
	if limit < 1 || limit > maximumDiffDetailLimit {
		return apperrors.NewValidation(
			"--detail-limit 必须在 1..1000 范围内",
			apperrors.WithReason("invalid_detail_limit"),
			apperrors.WithExecutionStarted(false),
			apperrors.WithRetryable(false),
		)
	}
	return nil
}

type identityMapFile struct {
	Version int               `json:"version"`
	Target  identityMapTarget `json:"target"`
	Nodes   map[string]string `json:"nodes"`
}

type identityMapTarget struct {
	NodeID   string `json:"nodeId"`
	PartID   string `json:"partId,omitempty"`
	PageID   string `json:"pageId,omitempty"`
	Revision *int   `json:"revision,omitempty"`
}

func parseIdentityMap(raw string, changed bool) (*identityMapFile, error) {
	if !changed {
		return nil, nil
	}
	if strings.TrimSpace(raw) == "" {
		return nil, apperrors.NewValidation("--identity-map 已显式提供但内容为空", apperrors.WithReason("invalid_identity_map"))
	}
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.DisallowUnknownFields()
	var result identityMapFile
	if err := decoder.Decode(&result); err != nil {
		return nil, apperrors.NewValidation("--identity-map 不是合法的 version=1 JSON: "+err.Error(), apperrors.WithReason("invalid_identity_map"))
	}
	if err := requireJSONEOF(decoder); err != nil {
		return nil, apperrors.NewValidation("--identity-map 必须是单一 JSON 对象", apperrors.WithReason("invalid_identity_map"))
	}
	if result.Version != 1 || strings.TrimSpace(result.Target.NodeID) == "" || result.Nodes == nil {
		return nil, apperrors.NewValidation("--identity-map 必须包含 version=1、非空 target.nodeId 和显式 nodes 对象", apperrors.WithReason("invalid_identity_map"))
	}
	seenReal := make(map[string]string, len(result.Nodes))
	for logical, real := range result.Nodes {
		trimmedLogical := strings.TrimSpace(logical)
		trimmedReal := strings.TrimSpace(real)
		if trimmedLogical == "" || trimmedReal == "" || logical != trimmedLogical || real != trimmedReal {
			return nil, apperrors.NewValidation("--identity-map nodes 的逻辑 ID 和真实 ID 都必须非空", apperrors.WithReason("invalid_identity_map"))
		}
		if previous, exists := seenReal[trimmedReal]; exists {
			return nil, apperrors.NewValidation(fmt.Sprintf("--identity-map 真实节点 %q 同时映射到 %q 和 %q", trimmedReal, previous, trimmedLogical), apperrors.WithReason("identity_map_not_bijective"))
		}
		seenReal[trimmedReal] = trimmedLogical
	}
	if result.Target.Revision != nil && *result.Target.Revision < 0 {
		return nil, apperrors.NewValidation("--identity-map target.revision 必须是非负整数", apperrors.WithReason("invalid_identity_map"))
	}
	return &result, nil
}

type diffCandidate struct {
	Category string
	Priority int
	SortKey  string
	Value    map[string]any
}

type diffAccumulator struct {
	Candidates []diffCandidate
	Summary    map[string]int
	Blockers   map[string]int
	Warnings   map[string]int
	Unchanged  int
	Preserved  int
}

func newDiffAccumulator() *diffAccumulator {
	return &diffAccumulator{
		Summary:  make(map[string]int),
		Blockers: make(map[string]int),
		Warnings: make(map[string]int),
	}
}

func (a *diffAccumulator) add(category string, priority int, value map[string]any) {
	a.Summary[category]++
	a.Candidates = append(a.Candidates, diffCandidate{
		Category: category, Priority: priority, SortKey: diffRecordSortKey(value), Value: value,
	})
}

func (a *diffAccumulator) risk(blocker bool, reason, hint string, node map[string]any) {
	record := map[string]any{"reason": reason, "hint": hint}
	for _, field := range []string{"id", "type"} {
		if value, ok := node[field]; ok {
			record[field] = value
		}
	}
	if blocker {
		a.Blockers[reason]++
		a.add("blockers", 1, record)
		return
	}
	a.Warnings[reason]++
	a.add("warnings", 7, record)
}

func diffRecordSortKey(value map[string]any) string {
	parts := make([]string, 0, 6)
	for _, key := range []string{"pageId", "type", "logicalId", "afterId", "beforeId", "id", "path", "reason"} {
		if raw, ok := value[key]; ok {
			parts = append(parts, fmt.Sprint(raw))
		}
	}
	return strings.Join(parts, "\x00")
}

func buildWhiteboardDiff(projected map[string]any, call whiteboardcore.Call, proposed *parsedUpdate, identity *identityMapFile, comparison string, limit int) (map[string]any, error) {
	source, ok := projected["source"].(map[string]any)
	if !ok {
		return nil, apperrors.NewAPI("白板 Query 未返回可比较的完整 source", apperrors.WithReason("incomplete_snapshot"))
	}
	page, current, err := singleDiffPage(source)
	if err != nil {
		return nil, err
	}
	target, err := diffTarget(projected, call, page)
	if err != nil {
		return nil, err
	}
	if comparison == "" {
		comparison = "semantic"
	}
	if comparison != "semantic" && comparison != "exact" {
		return nil, apperrors.NewValidation("--comparison 只能是 semantic 或 exact")
	}
	if err := validateIdentityMapForSnapshot(identity, target, current); err != nil {
		return nil, err
	}

	sourceDigest, err := whiteboardSourceDigest(proposed)
	if err != nil {
		return nil, err
	}
	snapshotDigest, err := whiteboardSnapshotDigest(source)
	if err != nil {
		return nil, err
	}
	acc := newDiffAccumulator()
	mode := "append"
	if proposed.Overwrite {
		mode = "overwrite"
	}
	computeExecutionDiff(acc, mode, current, proposed.Nodes)
	computeLogicalDiff(acc, current, proposed.Nodes, identity, comparison)
	computeMediaDiff(acc, mode, current, proposed.Nodes, identity)
	if call.Kind == whiteboardcore.KindEmbedded {
		acc.risk(false, "embedded_preview_not_atomic", "内嵌白板当前没有公开 revision 条件写；预览和更新之间的远端状态可能发生变化", nil)
	}
	if identity != nil && identity.Target.Revision != nil {
		if revision, ok := nonNegativeInt(target["revision"]); ok && revision != *identity.Target.Revision {
			acc.risk(false, "stale_mapping_revision", "identity map revision 与当前白板不同；已重新验证全部真实节点身份", nil)
		}
	}

	result := baseDiffResult(mode, comparison, target, identity != nil, sourceDigest, snapshotDigest, acc, limit)
	selected := selectDiffCandidates(acc.Candidates, limit)
	applyDiffCandidates(result, selected, acc.Candidates)
	if len(selected) < len(acc.Candidates) {
		result["truncationReasons"] = []any{"detail_limit"}
	}
	for {
		encoded, marshalErr := json.Marshal(result)
		if marshalErr != nil {
			return nil, apperrors.NewInternal("编码白板 diff 失败: " + marshalErr.Error())
		}
		if len(encoded) <= maximumDiffOutputBytes {
			break
		}
		if len(selected) == 0 {
			return nil, apperrors.NewInternal(
				"白板 diff 的不可裁剪摘要超过 2 MiB",
				apperrors.WithReason("diff_output_limit_exceeded"),
			)
		}
		selected = selected[:len(selected)-1]
		applyDiffCandidates(result, selected, acc.Candidates)
		addTruncationReason(result, "payload_bytes")
	}
	return result, nil
}

func singleDiffPage(source map[string]any) (map[string]any, []map[string]any, error) {
	raw, ok := source["pages"].([]any)
	if !ok || len(raw) != 1 {
		return nil, nil, apperrors.NewAPI("+diff 要求 Query 快照恰好包含一个页面", apperrors.WithReason("diff_page_count_mismatch"))
	}
	page, ok := raw[0].(map[string]any)
	if !ok {
		return nil, nil, apperrors.NewAPI("+diff 页面快照格式无效", apperrors.WithReason("malformed_snapshot_page"))
	}
	rawNodes, ok := page["nodes"].([]any)
	if !ok {
		return nil, nil, apperrors.NewAPI("+diff 页面快照缺少 nodes", apperrors.WithReason("malformed_snapshot_page"))
	}
	nodes := make([]map[string]any, len(rawNodes))
	for index, rawNode := range rawNodes {
		node, valid := rawNode.(map[string]any)
		if !valid {
			return nil, nil, apperrors.NewAPI("+diff 页面节点格式无效", apperrors.WithReason("malformed_snapshot_node"))
		}
		nodes[index] = node
	}
	return page, nodes, nil
}

func diffTarget(projected map[string]any, call whiteboardcore.Call, page map[string]any) (map[string]any, error) {
	target := map[string]any{"kind": string(call.Kind), "nodeId": projected["nodeId"]}
	pageID, _ := nonEmptyString(page["id"])
	if call.Kind == whiteboardcore.KindEmbedded {
		target["partId"] = projected["partId"]
		target["pageId"] = pageID
		return target, nil
	}
	wanted, _ := call.Args["pageId"].(string)
	if pageID == "" || pageID != strings.TrimSpace(wanted) {
		return nil, apperrors.NewAPI("独立白板 source 页面 ID 与请求 page-id 不一致", apperrors.WithReason("query_page_mismatch"))
	}
	target["pageId"] = pageID
	target["revision"] = projected["revision"]
	return target, nil
}

func validateIdentityMapForSnapshot(identity *identityMapFile, target map[string]any, current []map[string]any) error {
	if identity == nil {
		return nil
	}
	kind, _ := target["kind"].(string)
	expectedTarget := map[string]string{"nodeId": identity.Target.NodeID}
	if kind == string(whiteboardcore.KindEmbedded) {
		expectedTarget["partId"] = identity.Target.PartID
		if strings.TrimSpace(identity.Target.PageID) != "" || identity.Target.Revision != nil {
			return apperrors.NewValidation("内嵌白板 --identity-map target 不能包含 pageId 或 revision", apperrors.WithReason("identity_map_target_mismatch"))
		}
	} else {
		expectedTarget["pageId"] = identity.Target.PageID
		if strings.TrimSpace(identity.Target.PartID) != "" {
			return apperrors.NewValidation("独立白板 --identity-map target 不能包含 partId", apperrors.WithReason("identity_map_target_mismatch"))
		}
	}
	for key, expected := range expectedTarget {
		actual, _ := nonEmptyString(target[key])
		if strings.TrimSpace(expected) != actual {
			return apperrors.NewValidation("--identity-map target."+key+" 与本次白板目标不一致", apperrors.WithReason("identity_map_target_mismatch"))
		}
	}
	byID := nodeIndex(current)
	for logical, real := range identity.Nodes {
		if byID[strings.TrimSpace(real)] == nil {
			return apperrors.NewValidation(fmt.Sprintf("--identity-map 逻辑节点 %q 指向当前快照中不存在的真实节点", logical), apperrors.WithReason("stale_identity_mapping"))
		}
	}
	return nil
}

func nodeIndex(nodes []map[string]any) map[string]map[string]any {
	result := make(map[string]map[string]any, len(nodes))
	for _, node := range nodes {
		id, _ := nonEmptyString(node["id"])
		result[id] = node
	}
	return result
}

func nodeDetail(node map[string]any) map[string]any {
	result := map[string]any{}
	for _, key := range []string{"id", "type", "source", "writeSupport"} {
		if value, ok := node[key]; ok {
			result[key] = value
		}
	}
	return result
}

func computeExecutionDiff(acc *diffAccumulator, mode string, current, proposed []map[string]any) {
	for _, node := range proposed {
		acc.add("executionAdded", 6, nodeDetail(node))
	}
	if mode == "append" {
		acc.Preserved = len(current)
		return
	}
	for _, node := range current {
		if source, _ := nonEmptyString(node["source"]); source == "master" {
			acc.Preserved++
			continue
		}
		acc.add("executionDeleted", executionDeletePriority(node), nodeDetail(node))
		if locked, _ := node["locked"].(bool); locked {
			acc.risk(true, "locked_node_would_be_deleted", "overwrite 将删除锁定的 page-owned 节点；请先解除锁定或改用 append", node)
		}
		if !isWritableNode(node) {
			acc.risk(true, "query_only_node_would_be_deleted", "overwrite 将删除 proposed OpenNodes V1 无法重建的 query-only 节点", node)
		}
	}
}

func executionDeletePriority(node map[string]any) int {
	if !isWritableNode(node) {
		return 2
	}
	return 4
}

func isWritableNode(node map[string]any) bool {
	if support, ok := nonEmptyString(node["writeSupport"]); ok {
		return support != "readOnly"
	}
	nodeType, _ := nonEmptyString(node["type"])
	return containsString([]string{"shape", "text", "connector", "stickyNote", "frame", "group", "vector", "icon", "path"}, nodeType)
}

func computeLogicalDiff(acc *diffAccumulator, current, proposed []map[string]any, identity *identityMapFile, comparison string) {
	if identity == nil {
		for _, node := range current {
			record := nodeDetail(node)
			record["side"] = "current"
			acc.add("logicalUnmatched", 5, record)
		}
		for _, node := range proposed {
			record := nodeDetail(node)
			record["side"] = "proposed"
			acc.add("logicalUnmatched", 5, record)
		}
		return
	}
	currentByID := nodeIndex(current)
	proposedByID := nodeIndex(proposed)
	mappedReal := make(map[string]struct{}, len(identity.Nodes))
	for logical, real := range identity.Nodes {
		logical = strings.TrimSpace(logical)
		real = strings.TrimSpace(real)
		mappedReal[real] = struct{}{}
		before := currentByID[real]
		after := proposedByID[logical]
		if after == nil {
			record := nodeDetail(before)
			record["logicalId"] = logical
			record["beforeId"] = real
			acc.add("logicalDeleted", 5, record)
			continue
		}
		if !isWritableNode(before) {
			record := nodeDetail(before)
			record["logicalId"] = logical
			record["side"] = "current"
			record["reason"] = "query_only_node_uncomparable"
			acc.add("logicalUnmatched", 5, record)
			continue
		}
		changed, unresolved := compareMappedNodes(before, after, identity, comparison)
		if len(unresolved) > 0 {
			record := map[string]any{"logicalId": logical, "beforeId": real, "afterId": logical, "type": after["type"], "side": "pair", "reason": "unresolved_reference", "references": stringsToAny(unresolved)}
			acc.add("logicalUnmatched", 5, record)
			continue
		}
		if len(changed) == 0 {
			acc.Unchanged++
			continue
		}
		acc.add("logicalModified", 5, map[string]any{
			"logicalId": logical, "beforeId": real, "afterId": logical, "type": after["type"],
			"match":         map[string]any{"strategy": "explicit_id_map", "confidence": 1},
			"changedFields": mapsToAny(changed),
		})
	}
	for _, node := range proposed {
		logical, _ := nonEmptyString(node["id"])
		if _, mapped := identity.Nodes[logical]; !mapped {
			record := nodeDetail(node)
			record["logicalId"] = logical
			acc.add("logicalAdded", 6, record)
		}
	}
	for _, node := range current {
		real, _ := nonEmptyString(node["id"])
		if _, mapped := mappedReal[real]; !mapped {
			record := nodeDetail(node)
			record["side"] = "current"
			acc.add("logicalUnmatched", 5, record)
		}
	}
}

func stringsToAny(values []string) []any {
	result := make([]any, len(values))
	for index, value := range values {
		result[index] = value
	}
	return result
}

func computeMediaDiff(acc *diffAccumulator, mode string, current, proposed []map[string]any, identity *identityMapFile) {
	mappedRealWithProposed := make(map[string]struct{})
	if identity != nil {
		proposedByID := nodeIndex(proposed)
		for logical, real := range identity.Nodes {
			if proposedByID[logical] != nil {
				mappedRealWithProposed[real] = struct{}{}
			}
		}
	}
	if mode == "overwrite" {
		for _, node := range current {
			if source, _ := nonEmptyString(node["source"]); source == "master" {
				continue
			}
			realID, _ := nonEmptyString(node["id"])
			if !isWritableNode(node) && isMediaNode(node) {
				record := mediaRecord(node)
				record["reason"] = "query_only_media_not_reconstructable"
				acc.add("mediaUnsupported", 3, record)
			}
			if hasMediaIdentity(node) {
				if _, paired := mappedRealWithProposed[realID]; !paired {
					acc.add("mediaRemoved", 3, mediaRecord(node))
				}
			}
		}
	}
	if identity == nil {
		for _, node := range proposed {
			if hasMediaIdentity(node) {
				acc.add("mediaAdded", 6, mediaRecord(node))
			}
		}
		return
	}
	currentByID := nodeIndex(current)
	proposedByID := nodeIndex(proposed)
	for logical, real := range identity.Nodes {
		before, after := currentByID[real], proposedByID[logical]
		if before == nil || after == nil {
			continue
		}
		beforeIdentity := mediaIdentity(before)
		afterIdentity := mediaIdentity(after)
		if beforeIdentity == "" && afterIdentity != "" {
			record := mediaRecord(after)
			record["logicalId"] = logical
			acc.add("mediaAdded", 6, record)
		} else if beforeIdentity != "" && afterIdentity == "" {
			record := mediaRecord(before)
			record["logicalId"] = logical
			acc.add("mediaRemoved", 3, record)
		} else if beforeIdentity != "" && beforeIdentity != afterIdentity {
			acc.add("mediaReplaced", 3, map[string]any{"logicalId": logical, "type": after["type"], "beforeResourceId": beforeIdentity, "afterResourceId": afterIdentity})
		} else if beforeIdentity != "" {
			beforeURL, afterURL := mediaURL(before), mediaURL(after)
			if beforeURL != afterURL {
				acc.add("mediaMetadataChanged", 3, map[string]any{"logicalId": logical, "type": after["type"], "resourceId": beforeIdentity, "beforeUrl": beforeURL, "afterUrl": afterURL})
			}
		}
	}
	for _, node := range proposed {
		logical, _ := nonEmptyString(node["id"])
		if _, mapped := identity.Nodes[logical]; !mapped && hasMediaIdentity(node) {
			acc.add("mediaAdded", 6, mediaRecord(node))
		}
	}
}

func isMediaNode(node map[string]any) bool {
	nodeType, _ := nonEmptyString(node["type"])
	return containsString([]string{"vector", "icon", "image", "pdf", "media"}, nodeType)
}

func hasMediaIdentity(node map[string]any) bool { return mediaIdentity(node) != "" }

func mediaIdentity(node map[string]any) string {
	nodeType, _ := nonEmptyString(node["type"])
	if nodeType == "icon" {
		value, _ := nonEmptyString(node["catalogId"])
		return value
	}
	resource, _ := node["resource"].(map[string]any)
	value, _ := nonEmptyString(resource["resourceId"])
	return value
}

func mediaURL(node map[string]any) string {
	resource, _ := node["resource"].(map[string]any)
	value, _ := nonEmptyString(resource["url"])
	return value
}

func mediaRecord(node map[string]any) map[string]any {
	record := nodeDetail(node)
	if identity := mediaIdentity(node); identity != "" {
		record["resourceId"] = identity
	}
	return record
}

func baseDiffResult(mode, comparison string, target map[string]any, explicit bool, sourceDigest, snapshotDigest string, acc *diffAccumulator, limit int) map[string]any {
	strategy := "unmatched"
	if explicit {
		strategy = "explicit_id_map"
	}
	summary := map[string]any{
		"executionAdded": acc.Summary["executionAdded"], "executionDeleted": acc.Summary["executionDeleted"], "executionPreserved": acc.Preserved,
		"logicalAdded": acc.Summary["logicalAdded"], "logicalModified": acc.Summary["logicalModified"], "logicalDeleted": acc.Summary["logicalDeleted"],
		"logicalUnchanged": acc.Unchanged, "logicalUnmatched": acc.Summary["logicalUnmatched"],
		"mediaAdded": acc.Summary["mediaAdded"], "mediaRemoved": acc.Summary["mediaRemoved"], "mediaReplaced": acc.Summary["mediaReplaced"],
		"mediaMetadataChanged": acc.Summary["mediaMetadataChanged"], "mediaUnsupported": acc.Summary["mediaUnsupported"],
		"blockerCount": sumReasonCounts(acc.Blockers), "warningCount": sumReasonCounts(acc.Warnings),
	}
	return map[string]any{
		"comparison": "current_vs_proposed", "mode": mode, "complete": true, "identityStrategy": strategy,
		"target":           target,
		"comparisonPolicy": map[string]any{"mode": comparison, "coordinateTolerancePx": 0.5},
		"summary":          summary,
		"executionDiff":    map[string]any{"added": []any{}, "deleted": []any{}, "preservedCount": acc.Preserved},
		"logicalDiff":      map[string]any{"added": []any{}, "modified": []any{}, "deleted": []any{}, "unchangedCount": acc.Unchanged, "unmatched": []any{}},
		"media":            map[string]any{"added": []any{}, "removed": []any{}, "replaced": []any{}, "metadataChanged": []any{}, "unsupported": []any{}},
		"preflight":        map[string]any{"level": "local_only", "serverPreflightRequired": true},
		"blockers":         []any{}, "warnings": []any{}, "blockerSummary": reasonSummary(acc.Blockers), "warningSummary": reasonSummary(acc.Warnings),
		"detailTotal": len(acc.Candidates), "detailReturned": 0, "detailLimit": limit, "detailsTruncated": false,
		"summaryComplete": true, "truncationReasons": []any{}, "detailCounts": map[string]any{},
		"sourceDigest": sourceDigest, "snapshotDigest": snapshotDigest,
	}
}

func sumReasonCounts(values map[string]int) int {
	total := 0
	for _, count := range values {
		total += count
	}
	return total
}

func reasonSummary(values map[string]int) []any {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	result := make([]any, 0, len(keys))
	for _, key := range keys {
		result = append(result, map[string]any{"reason": key, "count": values[key]})
	}
	return result
}

func selectDiffCandidates(all []diffCandidate, limit int) []diffCandidate {
	ordered := append([]diffCandidate(nil), all...)
	sort.SliceStable(ordered, func(i, j int) bool {
		if ordered[i].Priority != ordered[j].Priority {
			return ordered[i].Priority < ordered[j].Priority
		}
		if ordered[i].Category != ordered[j].Category {
			return ordered[i].Category < ordered[j].Category
		}
		return ordered[i].SortKey < ordered[j].SortKey
	})
	if len(ordered) <= limit {
		return ordered
	}
	categories := make(map[string][]diffCandidate)
	for _, candidate := range ordered {
		categories[candidate.Category] = append(categories[candidate.Category], candidate)
	}
	if limit < len(categories) {
		return ordered[:limit]
	}
	selected := make([]diffCandidate, 0, limit)
	selectedKeys := make(map[string]struct{}, len(categories))
	categoryNames := make([]string, 0, len(categories))
	for category := range categories {
		categoryNames = append(categoryNames, category)
	}
	sort.Slice(categoryNames, func(i, j int) bool {
		left, right := categories[categoryNames[i]][0], categories[categoryNames[j]][0]
		if left.Priority != right.Priority {
			return left.Priority < right.Priority
		}
		return categoryNames[i] < categoryNames[j]
	})
	for _, category := range categoryNames {
		candidate := categories[category][0]
		selected = append(selected, candidate)
		selectedKeys[candidate.Category+"\x00"+candidate.SortKey] = struct{}{}
	}
	for _, candidate := range ordered {
		if len(selected) >= limit {
			break
		}
		key := candidate.Category + "\x00" + candidate.SortKey
		if _, exists := selectedKeys[key]; exists {
			delete(selectedKeys, key)
			continue
		}
		selected = append(selected, candidate)
	}
	sort.SliceStable(selected, func(i, j int) bool {
		if selected[i].Priority != selected[j].Priority {
			return selected[i].Priority < selected[j].Priority
		}
		if selected[i].Category != selected[j].Category {
			return selected[i].Category < selected[j].Category
		}
		return selected[i].SortKey < selected[j].SortKey
	})
	return selected
}

var diffCategoryPaths = map[string][2]string{
	"executionAdded":       {"executionDiff", "added"},
	"executionDeleted":     {"executionDiff", "deleted"},
	"logicalAdded":         {"logicalDiff", "added"},
	"logicalModified":      {"logicalDiff", "modified"},
	"logicalDeleted":       {"logicalDiff", "deleted"},
	"logicalUnmatched":     {"logicalDiff", "unmatched"},
	"mediaAdded":           {"media", "added"},
	"mediaRemoved":         {"media", "removed"},
	"mediaReplaced":        {"media", "replaced"},
	"mediaMetadataChanged": {"media", "metadataChanged"},
	"mediaUnsupported":     {"media", "unsupported"},
	"blockers":             {"", "blockers"},
	"warnings":             {"", "warnings"},
}

func applyDiffCandidates(result map[string]any, selected, all []diffCandidate) {
	for _, path := range diffCategoryPaths {
		if path[0] == "" {
			result[path[1]] = []any{}
		} else {
			result[path[0]].(map[string]any)[path[1]] = []any{}
		}
	}
	totals := make(map[string]int)
	returned := make(map[string]int)
	for _, candidate := range all {
		totals[candidate.Category]++
	}
	for _, candidate := range selected {
		returned[candidate.Category]++
		path := diffCategoryPaths[candidate.Category]
		if path[0] == "" {
			result[path[1]] = append(result[path[1]].([]any), candidate.Value)
		} else {
			parent := result[path[0]].(map[string]any)
			parent[path[1]] = append(parent[path[1]].([]any), candidate.Value)
		}
	}
	counts := make(map[string]any, len(totals))
	for category, total := range totals {
		counts[category] = map[string]any{"total": total, "returned": returned[category]}
	}
	result["detailCounts"] = counts
	result["detailReturned"] = len(selected)
	result["detailsTruncated"] = len(selected) < len(all)
}

func addTruncationReason(result map[string]any, reason string) {
	reasons := result["truncationReasons"].([]any)
	for _, existing := range reasons {
		if existing == reason {
			return
		}
	}
	result["truncationReasons"] = append(reasons, reason)
}
