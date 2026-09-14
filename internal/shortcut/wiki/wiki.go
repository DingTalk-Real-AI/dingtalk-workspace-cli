// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

// Package wiki declares reviewed, truth-preserving shortcuts for Wiki spaces,
// members, nodes, and activity feeds.
package wiki

import (
	"fmt"
	"strings"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
)

var collectionAvoid = []string{"需要原始 MCP 响应或未公开底层参数时改用对应原子命令；缺失业务数组不是合法空结果"}

func readShortcut(command, description, intent, collection, example string, flags []shortcut.Flag, params []contract.ParamDecl, execute func(*shortcut.RuntimeContext) error) shortcut.Shortcut {
	result := wikiObjectResult(description)
	if collection != "" {
		result = wikiCollectionResult(collection, description)
	}
	return shortcut.Shortcut{OutputRollout: output.RolloutUnifiedActive, Service: "wiki", Command: command, Product: "wiki", Description: description, Intent: intent, Risk: shortcut.RiskRead, Safety: wikiReadSafety(), Contract: wikiContract(command, description, intent, collectionAvoid, []string{example}, result, nil, params...), Flags: flags, Execute: execute}
}

func writeShortcut(command, description, intent, example string, risk shortcut.Risk, safety contract.SafetySpec, flags []shortcut.Flag, params []contract.ParamDecl, execute func(*shortcut.RuntimeContext) error) shortcut.Shortcut {
	return shortcut.Shortcut{OutputRollout: output.RolloutUnifiedActive, Service: "wiki", Command: command, Product: "wiki", Description: description, Intent: intent, Risk: risk, Safety: safety, Contract: wikiContract(command, description, intent, []string{"只需读取或影响范围未确认时不要执行写操作"}, []string{example}, wikiObjectResult(description), nil, params...), Flags: flags, Execute: execute}
}

var SpaceList = readShortcut("+space-list", "严格分页列出知识库并保留类型范围证据", "按 orgWikiSpace 或 myWikiSpace 范围浏览有权访问的知识库，并保留请求类型和服务端分页证据；只有显式 wikiSpaces:[] 才是空结果。", "spaces", "dws wiki +space-list --type orgWikiSpace --limit 50 --page-all --format json", []shortcut.Flag{
	{Name: "type", Type: shortcut.FlagString, Default: "orgWikiSpace", Desc: "知识库类型", Enum: []string{"orgWikiSpace", "myWikiSpace"}},
	{Name: "limit", Type: shortcut.FlagString, Desc: "每页数量 1-50（默认 20）"}, {Name: "cursor", Type: shortcut.FlagString, Desc: "分页游标", Aliases: []string{"page-token"}, AliasesVisible: true},
}, []contract.ParamDecl{{Name: "type", Property: "wikiSpaceType"}, {Name: "limit", Property: "pageSize"}, {Name: "cursor", Property: "pageToken"}}, func(rt *shortcut.RuntimeContext) error {
	pageSize, err := wikiStringInt(rt, "limit", 20, 1, 50)
	if err != nil {
		return err
	}
	items, page, err := collectWikiPages(rt, "wiki/list_wikiSpaces", pageSize, []string{"wikiSpaces", "spaces"}, func(cursor string, size int) (map[string]any, error) {
		params := map[string]any{"wikiSpaceType": rt.Str("type"), "pageSize": size}
		if cursor != "" {
			params["pageToken"] = cursor
		}
		return rt.CallMCPData("wiki", "list_wikiSpaces", params)
	})
	if err != nil {
		return err
	}
	spaces := projectWikiRows(items, map[string][]string{"workspaceId": {"workspaceId", "spaceId", "id"}, "name": {"name", "spaceName", "title"}, "description": {"description", "desc"}, "createTime": {"createTime", "createdAt"}, "url": {"spaceUrl", "url"}, "spaceType": {"spaceType", "wikiSpaceType", "type"}})
	out := map[string]any{"count": len(spaces), "requestedType": rt.Str("type"), "spaces": spaces}
	addWikiPagination(out, page)
	return rt.Output(out)
})

var SpaceSearch = wikiWithInterfaceReason(readShortcut("+space-search", "严格搜索知识库", "按名称关键词定位知识库；严格验证搜索数组，避免内部异常被误报为零命中。", "spaces", "dws wiki +space-search --query \"产品文档\" --format json", []shortcut.Flag{
	{Name: "query", Type: shortcut.FlagString, Required: true, Desc: "搜索关键词"}, {Name: "limit", Type: shortcut.FlagString, Desc: "返回数量 1-20（默认 10）"},
}, []contract.ParamDecl{{Name: "query", Property: "query"}, {Name: "limit", Property: "limit"}}, func(rt *shortcut.RuntimeContext) error {
	pageSize, err := wikiStringInt(rt, "limit", 10, 1, 20)
	if err != nil {
		return err
	}
	data, err := rt.CallMCPData("wiki", "search_wikiSpaces", map[string]any{"keyword": rt.Str("query"), "pageSize": pageSize})
	if err != nil {
		return err
	}
	items, page, err := requireWikiCollection(data, "wiki/search_wikiSpaces", "wikiSpaces", "spaces")
	if err != nil {
		return err
	}
	spaces := projectWikiRows(items, map[string][]string{"workspaceId": {"workspaceId", "spaceId", "id"}, "name": {"name", "spaceName", "title"}, "description": {"description", "desc"}, "url": {"spaceUrl", "url"}})
	out := map[string]any{"count": len(spaces), "spaces": spaces}
	addWikiPagination(out, page)
	return rt.Output(out)
}), wikiSpaceSearchCompositeReason)

var SpaceGet = readShortcut("+space-get", "获取知识库详情", "已知 workspace ID 或知识库 URL 时读取并验证空间详情。", "", "dws wiki +space-get --workspace <workspaceId> --format json", []shortcut.Flag{{Name: "workspace", Type: shortcut.FlagString, Required: true, Desc: "知识库 ID 或 URL"}}, []contract.ParamDecl{{Name: "workspace", Property: "workspaceId"}}, func(rt *shortcut.RuntimeContext) error {
	data, err := rt.CallMCPData("wiki", "get_wikiSpace", map[string]any{"workspaceId": rt.Str("workspace")})
	if err != nil {
		return err
	}
	object, err := requireWikiObject(data, "wiki/get_wikiSpace")
	if err != nil {
		return err
	}
	if firstWikiString(object, "workspaceId", "spaceId", "id") == "" {
		return wikiResponseError("wiki/get_wikiSpace", "missing_workspace_id", "空间详情缺少 workspaceId")
	}
	return rt.Output(object)
})

var SpaceCreate = writeShortcut("+space-create", "创建知识库并读回验证空间类型", "创建新的知识库容器；必须取得 workspaceId、通过详情读回，并用详情或组织/个人范围列表验证空间类型后才报告成功。", "dws wiki +space-create --name \"产品文档库\" --format json", shortcut.RiskWrite, wikiWriteSafety(false), []shortcut.Flag{{Name: "name", Type: shortcut.FlagString, Required: true, Desc: "知识库名称，不超过 32 个字符。--name 不超过 32 个字符；--desc 不超过 500 个字符"}, {Name: "desc", Type: shortcut.FlagString, Desc: "知识库描述，不超过 500 个字符。--name 不超过 32 个字符；--desc 不超过 500 个字符"}, {Name: "icon", Type: shortcut.FlagString, Desc: "图标标识"}}, []contract.ParamDecl{{Name: "name", Property: "name"}, {Name: "desc", Property: "description"}, {Name: "icon", Property: "icon"}}, func(rt *shortcut.RuntimeContext) error {
	params := map[string]any{"name": rt.Str("name")}
	if rt.Changed("desc") {
		params["description"] = rt.Str("desc")
	}
	if rt.Changed("icon") {
		params["icon"] = rt.Str("icon")
	}
	if rt.DryRun() {
		return rt.Output(map[string]any{"dryRun": true, "executed": false, "operation": "wiki/create_wikiSpace", "arguments": params})
	}
	written, err := rt.CallMCPWriteDataStrict("wiki", "create_wikiSpace", params)
	if err != nil {
		return err
	}
	written, err = requireWikiWrite(written, "wiki/create_wikiSpace")
	if err != nil {
		return err
	}
	id := nestedWikiString(written, "workspaceId", "spaceId", "id")
	if id == "" {
		return wikiResponseError("wiki/create_wikiSpace", "missing_created_id", "创建响应没有 workspaceId；远端效果未知")
	}
	verified, err := rt.CallMCPData("wiki", "get_wikiSpace", map[string]any{"workspaceId": id})
	if err != nil {
		return err
	}
	verified, err = requireWikiObject(verified, "wiki/get_wikiSpace")
	if err != nil {
		return err
	}
	if firstWikiString(verified, "workspaceId", "spaceId", "id") != id {
		return wikiResponseError("wiki/create_wikiSpace", "readback_id_mismatch", "创建后读回的 workspaceId 不一致")
	}
	spaceType, evidence, err := verifyWikiSpaceType(rt, id, verified)
	if err != nil {
		return wikiSpaceTypeVerificationError(id, verified, err)
	}
	return rt.Output(map[string]any{"success": true, "workspaceId": id, "space": verified, "spaceType": spaceType, "spaceTypeVerified": true, "spaceTypeEvidence": evidence})
})

var DeleteSpace = writeShortcut("+delete-space", "删除知识库", "用户明确确认后将整个知识库移入回收站；删除前读取目标，删除响应必须有 success=true。", "dws wiki +delete-space --workspace <workspaceId> --format json", shortcut.RiskHighWrite, wikiDeleteSafety(), []shortcut.Flag{{Name: "workspace", Type: shortcut.FlagString, Required: true, Desc: "知识库 ID 或 URL"}}, []contract.ParamDecl{{Name: "workspace", Property: "workspaceId"}}, func(rt *shortcut.RuntimeContext) error {
	preflight, err := rt.CallMCPData("wiki", "get_wikiSpace", map[string]any{"workspaceId": rt.Str("workspace")})
	if err != nil {
		return err
	}
	preflight, err = requireWikiObject(preflight, "wiki/get_wikiSpace")
	if err != nil {
		return err
	}
	if rt.DryRun() {
		return rt.Output(map[string]any{"dryRun": true, "executed": false, "operation": "wiki/delete_wikiSpace", "target": preflight})
	}
	written, err := rt.CallMCPWriteDataStrict("wiki", "delete_wikiSpace", map[string]any{"workspaceId": rt.Str("workspace")})
	if err != nil {
		return err
	}
	written, err = requireWikiWrite(written, "wiki/delete_wikiSpace")
	if err != nil {
		return err
	}
	return rt.Output(map[string]any{"success": true, "workspaceId": rt.Str("workspace"), "deleted": true})
})

func memberFlags(withRole bool) []shortcut.Flag {
	flags := []shortcut.Flag{{Name: "workspace", Type: shortcut.FlagString, Required: true, Desc: "知识库 ID 或 URL"}, {Name: "users", Type: shortcut.FlagStringSlice, Required: true, Desc: "用户 userId，最多 30 个", Aliases: []string{"user"}, AliasesVisible: true}}
	if withRole {
		flags = append(flags, shortcut.Flag{Name: "role", Type: shortcut.FlagString, Required: true, Desc: "角色", Enum: []string{"MANAGER", "EDITOR", "DOWNLOADER", "READER"}})
	}
	return flags
}

func memberWrite(command, tool, description, intent, example string, withRole bool) shortcut.Shortcut {
	params := []contract.ParamDecl{{Name: "workspace", Property: "workspaceId"}, {Name: "users", Property: "userIds"}}
	if withRole {
		params = append(params, contract.ParamDecl{Name: "role", Property: "roleId"})
	}
	return writeShortcut(command, description, intent, example, shortcut.RiskWrite, wikiWriteSafety(false), memberFlags(withRole), params, func(rt *shortcut.RuntimeContext) error {
		users := wikiStringSliceFirst(rt, "users", "user")
		if len(users) == 0 || len(users) > 30 {
			return fmt.Errorf("--users 必须包含 1-30 个 userId")
		}
		args := map[string]any{"workspaceId": rt.Str("workspace"), "userIds": users}
		if withRole {
			args["roleId"] = strings.ToUpper(rt.Str("role"))
		}
		if rt.DryRun() {
			return rt.Output(map[string]any{"dryRun": true, "executed": false, "operation": "wiki/" + tool, "arguments": args})
		}
		written, err := rt.CallMCPWriteDataStrict("wiki", tool, args)
		if err != nil {
			return err
		}
		written, err = requireWikiWrite(written, "wiki/"+tool)
		if err != nil {
			return err
		}
		return rt.Output(map[string]any{
			"success": true, "workspaceId": rt.Str("workspace"), "userCount": len(users), "operation": tool,
			"verifiedBy": "write_terminal_success",
			"verification": map[string]any{
				"status":            "terminal_response_only",
				"readbackAvailable": false,
				"reason":            "member_list_is_capped_and_has_no_cursor",
			},
		})
	})
}

var MemberAdd = memberWrite("+member-add", "add_member", "添加知识库成员", "向知识库授予一个或多个用户容器级角色；仅以写接口 success=true 作为终态证据，并明确成员列表无法完成精确读回。", "dws wiki +member-add --workspace <workspaceId> --users <userId> --role READER --format json", true)
var MemberUpdate = memberWrite("+member-update", "update_member", "更新知识库成员角色", "调整已有成员的知识库容器级角色；仅以写接口 success=true 作为终态证据，并明确成员列表无法完成精确读回。", "dws wiki +member-update --workspace <workspaceId> --users <userId> --role EDITOR --format json", true)
var MemberRemove = memberWrite("+member-remove", "remove_member", "移除知识库成员", "移除一个或多个用户的知识库容器级访问；仅以写接口 success=true 作为终态证据，并明确成员列表无法完成精确读回。", "dws wiki +member-remove --workspace <workspaceId> --users <userId> --format json", false)

var MemberList = readShortcut("+member-list", "严格列出知识库成员", "列出知识库成员及角色；后端不提供可续游标且单次真实上限为 50，不伪造 page-all。", "members", "dws wiki +member-list --workspace <workspaceId> --format json", []shortcut.Flag{{Name: "workspace", Type: shortcut.FlagString, Required: true, Desc: "知识库 ID 或 URL"}, {Name: "limit", Type: shortcut.FlagInt, Default: "30", Desc: "返回上限 1-50"}, {Name: "filter-role", Type: shortcut.FlagStringSlice, Desc: "角色过滤"}}, []contract.ParamDecl{{Name: "workspace", Property: "workspaceId"}, {Name: "limit", Property: "maxResults"}, {Name: "filter-role", Property: "filterRoleIds"}}, func(rt *shortcut.RuntimeContext) error {
	if rt.Int("limit") < 1 || rt.Int("limit") > 50 {
		return fmt.Errorf("--limit 必须在 1-50 之间；服务端不支持超过 50 或游标续页")
	}
	params := map[string]any{"workspaceId": rt.Str("workspace"), "maxResults": rt.Int("limit")}
	if rt.Changed("filter-role") {
		params["filterRoleIds"] = rt.StrSlice("filter-role")
	}
	data, err := rt.CallMCPData("wiki", "list_member", params)
	if err != nil {
		return err
	}
	items, page, err := requireWikiCollection(data, "wiki/list_member", "members")
	if err != nil {
		return err
	}
	members := projectWikiRows(items, map[string][]string{"id": {"id", "userId"}, "name": {"name", "nick"}, "role": {"role", "roleId"}, "type": {"type"}, "outer": {"outer"}})
	out := map[string]any{"count": len(members), "members": members}
	addWikiPagination(out, page)
	return rt.Output(out)
})

func init() {
	DeleteSpace.Aliases = []string{"+space-delete"}
	SpaceCreate.Validate = func(rt *shortcut.RuntimeContext) error {
		if len([]rune(rt.Str("name"))) > 32 {
			return fmt.Errorf("--name 不能超过 32 个字符")
		}
		if len([]rune(rt.Str("desc"))) > 500 {
			return fmt.Errorf("--desc 不能超过 500 个字符")
		}
		return nil
	}
	SpaceCreate.Constraints = []shortcut.Constraint{{Kind: shortcut.ConstraintCustom, Flags: []string{"name", "desc"}, Description: "--name 不超过 32 个字符；--desc 不超过 500 个字符"}}
	enableWikiAutoPage(&SpaceList)
	SpaceList.Contract.Result = wikiSpaceListResult()
	SpaceList.Contract.Pagination = wikiCursorPagination()
	SpaceCreate.Contract.Result = wikiSpaceCreateResult()
	shortcut.Register(SpaceList, SpaceSearch, SpaceGet, SpaceCreate, DeleteSpace, MemberAdd, MemberUpdate, MemberList, MemberRemove)
}
