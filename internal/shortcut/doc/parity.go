// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0
package doc

import (
	"fmt"
	goldmarkast "github.com/yuin/goldmark/ast"
	goldmarktext "github.com/yuin/goldmark/text"
	"sort"
	"strconv"
	"strings"
	"time"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
)

// registerDocShortcuts keeps the additions on the owning Doc declarations.
// Old paths remain registered; aliases do not create separate command identities.
func registerDocShortcuts(values ...shortcut.Shortcut) {
	for i := range values {
		values[i] = withDocParityAliases(values[i])
	}
	shortcut.Register(values...)
}

func withDocParityAliases(s shortcut.Shortcut) shortcut.Shortcut {
	switch s.Command {
	case "+create", "+fetch", "+search", "+version-list", "+history-list", "+version-revert", "+history-revert", "+media-insert", "+media-upload", "+media-preview", "+media-download", "+resource-update", "+resource-download", "+resource-delete", "+script":
	default:
		return s
	}

	aliases := map[string]string{}
	s.Flags = append([]shortcut.Flag(nil), s.Flags...)
	for i, f := range s.Flags {
		name := ""
		switch f.Name {
		case "node":
			name = "doc"
		case "name":
			if s.Command == "+create" {
				name = "title"
			}
		case "image":
			name = "url"
		case "cursor":
			name = "page-token"
		case "limit":
			name = "page-size"
		}
		if name == "" {
			continue
		}
		exists := false
		for _, other := range s.Flags {
			if other.Name == name {
				exists = true
			}
			for _, a := range other.Aliases {
				if a == name {
					exists = true
				}
			}
		}
		if exists {
			continue
		}
		s.Flags[i].Aliases = append(append([]string(nil), f.Aliases...), name)
		s.Flags[i].AliasesVisible = false
		s.Flags[i].Desc += "；兼容别名与主参数同时提供时值必须一致"
		aliases[name] = f.Name
	}
	if len(aliases) == 0 {
		return s
	}
	var aliasFlags []string
	for _, flag := range s.Flags {
		for _, primary := range aliases {
			if flag.Name == primary {
				aliasFlags = append(aliasFlags, primary)
				break
			}
		}
	}
	s.Constraints = append(append([]shortcut.Constraint(nil), s.Constraints...), shortcut.Constraint{Kind: shortcut.ConstraintCustom, Flags: aliasFlags, Description: "兼容别名与主参数同时提供时值必须一致"})
	aliasNames := make([]string, 0, len(aliases))
	for alias := range aliases {
		aliasNames = append(aliasNames, alias)
	}
	sort.Strings(aliasNames)
	previous := s.Validate
	s.Validate = func(rt *shortcut.RuntimeContext) error {
		for _, alias := range aliasNames {
			primary := aliases[alias]
			if !rt.Changed(alias) {
				continue
			}
			af := rt.Command().Flags().Lookup(alias)
			pf := rt.Command().Flags().Lookup(primary)
			if rt.Changed(primary) && pf.Value.String() != af.Value.String() {
				return apperrors.NewValidation(fmt.Sprintf("--%s 与 --%s 冲突", primary, alias))
			}
			if err := rt.Command().Flags().Set(primary, af.Value.String()); err != nil {
				return err
			}
		}
		if previous != nil {
			return previous(rt)
		}
		return nil
	}
	return s
}

func docDefaultTitle(content, format string) string {
	if format == "markdown" {
		source := []byte(content)
		root := docMarkdown.Parser().Parse(goldmarktext.NewReader(source))
		for block := root.FirstChild(); block != nil; block = block.NextSibling() {
			heading, ok := block.(*goldmarkast.Heading)
			if !ok || heading.Level != 1 {
				continue
			}
			title := strings.TrimSpace(string(heading.Text(source)))
			if title != "" {
				r := []rune(title)
				if len(r) > 80 {
					r = r[:80]
				}
				return string(r)
			}
		}
	}
	return "未命名文档"
}

func validateDocSearch(rt *shortcut.RuntimeContext) error {
	if rt.Changed("folder") && strings.TrimSpace(rt.Str("folder")) == "" {
		return apperrors.NewValidation("folder不能为空")
	}
	if rt.Changed("folder") && (!rt.Bool("page-all") || rt.Changed("cursor") || rt.Changed("page-token")) {
		return apperrors.NewValidation("folder筛选要求--page-all从首页读取完整候选，不能使用cursor")
	}
	for _, pair := range [][2]string{{"created-after", "created-from"}, {"created-before", "created-to"}, {"visited-after", "visited-from"}, {"visited-before", "visited-to"}} {
		if !rt.Changed(pair[0]) {
			continue
		}
		if rt.Changed(pair[1]) {
			return apperrors.NewValidation("时间文本参数与毫秒参数不能同时指定: --" + pair[0] + " / --" + pair[1])
		}
		value, err := parseDocSearchTime(rt.Str(pair[0]))
		if err != nil {
			return err
		}
		if err := rt.Command().Flags().Set(pair[1], strconv.FormatInt(value, 10)); err != nil {
			return err
		}
	}
	for _, pair := range [][2]string{{"created-from", "created-to"}, {"visited-from", "visited-to"}} {
		if rt.Changed(pair[0]) && rt.Changed(pair[1]) && rt.Int(pair[0]) > rt.Int(pair[1]) {
			return apperrors.NewValidation("时间起点不能晚于终点")
		}
	}
	return validateDocAutoPagination(rt)
}
func parseDocSearchTime(raw string) (int64, error) {
	// Preserve the previously published millisecond aliases as well as text dates.
	if value, err := strconv.ParseInt(raw, 10, 64); err == nil {
		return value, nil
	}
	for _, layout := range []string{time.RFC3339, "2006-01-02"} {
		if value, err := time.Parse(layout, raw); err == nil {
			return value.UnixMilli(), nil
		}
	}
	return 0, apperrors.NewValidation("时间须为带时区RFC3339或YYYY-MM-DD（UTC）")
}

func searchDocsProjectWithMetadata(data map[string]any) []map[string]any {
	out := searchDocsProject(data)
	raw := docResolveList(data)
	// Associate by stable ID; never align by index after a projector skipped an item.
	byID := map[string]map[string]any{}
	for _, item := range raw {
		if m, ok := item.(map[string]any); ok {
			if id, present := docFirst(m, "nodeId", "node_id", "id", "docId", "doc_id"); present {
				byID[fmt.Sprint(id)] = m
			}
		}
	}
	for _, row := range out {
		if id, ok := row["nodeId"]; ok {
			if m := byID[fmt.Sprint(id)]; m != nil {
				row["metadata"] = m
			}
		}
	}
	return out
}

func filterDocSearchFolder(rt *shortcut.RuntimeContext, result map[string]any) (map[string]any, error) {
	if result["complete"] != true {
		return nil, apperrors.NewAPI("搜索候选未完整，无法证明文件夹筛选结果")
	}
	nodes, err := collectDocPages(rt, "list_nodes", "nodes", map[string]any{"folderId": rt.Str("folder")}, listNodesProject, docPageOptions{PageAll: true, PageSize: 50, MaxPages: rt.Int("max-pages"), MaxItems: rt.Int("max-items")})
	if err != nil {
		return nil, err
	}
	if nodes["complete"] != true {
		return nil, apperrors.NewAPI("文件夹成员未完整，无法筛选")
	}
	allowed := map[string]bool{}
	for _, n := range nodes["nodes"].([]map[string]any) {
		id, _ := n["nodeId"].(string)
		if id == "" {
			return nil, apperrors.NewAPI("文件夹成员缺少nodeId")
		}
		allowed[id] = true
	}
	items := []map[string]any{}
	for _, item := range result["documents"].([]map[string]any) {
		id, _ := item["nodeId"].(string)
		if id == "" {
			return nil, apperrors.NewAPI("搜索结果缺少nodeId")
		}
		if allowed[id] {
			items = append(items, item)
		}
	}
	result["documents"] = items
	result["count"] = len(items)
	result["folderScope"] = "direct_children"
	return result, nil
}
