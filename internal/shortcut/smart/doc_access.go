// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package smart

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
)

var (
	resolveDocPermissionUser = resolveUser
	resolveDocShareUser      = resolveOpenDingTalkUser
	legacyShareDoc           shortcut.Shortcut
)

func docSmartContract(command, description, intent string, examples []string, dryRun bool) corecmd.ContractDecl {
	name := "shortcut_" + strings.ReplaceAll(strings.TrimPrefix(command, "+"), "-", "_")
	cliPath := "doc " + command
	decl := corecmd.ContractDecl{
		Description: description,
		Interface: &contract.InterfaceSpec{
			Mode:         contract.InterfaceModeComposite,
			Availability: contract.InterfaceAvailable,
			Reason:       "Reviewed cross-product Doc Shortcut composite: contact resolution, document permissions, messaging, confirmation, and output ledger cannot be represented by one MCP interface.",
		},
		Selection: contract.SelectionSpec{
			AgentSummary: description,
			UseWhen:      []string{intent},
			AvoidWhen: []string{
				"已知 userId 且只需原子权限变更时可直接使用 doc permission 命令；知识库容器成员使用 wiki；普通文件权限使用 drive",
			},
			Examples: examples,
		},
		Identity: contract.ToolIdentitySpec{
			ProductID: "doc", Name: name, CanonicalPath: "doc." + name,
			CLIPath: cliPath, PrimaryCLIPath: cliPath,
		},
	}
	if dryRun {
		decl.DryRun = &contract.DryRunSpec{PreviewKind: contract.DryRunPreviewPlan, RemoteReads: true}
	}
	return decl
}

func canonicalizeShareDoc() {
	legacyShareDoc = ShareDoc
	// Preserve the historical identity and string-typed --to flag while using
	// the same pre-resolve-and-send implementation as the canonical command.
	legacyShareDoc.Execute = executeShare
	ShareDoc.Command = "+share"
	ShareDoc.Aliases = nil
	ShareDoc.Description = "按姓名发送文档链接，不改变文档权限"
	ShareDoc.Intent = "当用户已有文档 URL、要按姓名私信给一个或多个人但不改变权限时使用；第一次发消息前先完成全部姓名唯一解析。"
	ShareDoc.Contract = docSmartContract("+share", ShareDoc.Description, ShareDoc.Intent,
		[]string{`dws doc +share --to 张三 --url https://alidocs.dingtalk.com/i/nodes/<DOC_ID> --note "请帮忙 review"`}, false)
	ShareDoc.Flags = []shortcut.Flag{
		{Name: "to", Type: shortcut.FlagString, Desc: "接收人姓名列表（多个姓名用逗号分隔）", Required: true},
		{Name: "url", Type: shortcut.FlagString, Desc: "文档链接", Required: true},
		{Name: "note", Type: shortcut.FlagString, Desc: "附言"},
		shortcut.AIMessageTagFlag(),
	}
	ShareDoc.Tips = []string{`dws doc +share --to 张三 --url https://alidocs.dingtalk.com/i/nodes/<DOC_ID> --note "请帮忙 review"`}
	ShareDoc.Execute = executeShare
}

var AccessGrant = shortcut.Shortcut{
	Service: "doc", Command: "+access-grant", Product: "doc",
	Description: "按姓名解析后批量授予文档权限",
	Intent:      "当用户要给一个或多位同事授予单篇文档 READER/DOWNLOADER/EDITOR/MANAGER 权限时使用；所有姓名唯一解析成功后才执行一次批量授权。",
	Risk:        shortcut.RiskWrite,
	Safety:      contract.SafetySpec{Effect: "write", Risk: "medium", Confirmation: "user_required", Idempotency: "unknown"},
	Contract: docSmartContract("+access-grant", "按姓名解析后批量授予文档权限",
		"当用户要给一个或多位同事授予单篇文档 READER/DOWNLOADER/EDITOR/MANAGER 权限时使用；所有姓名唯一解析成功后才执行一次批量授权。",
		[]string{`dws doc +access-grant --node <DOC_ID> --to 张三,李四 --role READER`}, false),
	Flags: permissionFlags(true),
	Tips:  []string{`dws doc +access-grant --node <DOC_ID> --to 张三,李四 --role READER`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		users, err := resolveDocUsers(rt, false)
		if err != nil {
			return err
		}
		params := permissionParams(rt, users)
		if rt.DryRun() {
			return rt.Output(docAccessEnvelope("doc.access_grant", map[string]any{"executed": false, "resolved": resolvedUserLedger(users), "params": params}))
		}
		result, err := rt.CallMCPWriteData("doc", "add_permission", params)
		if err != nil {
			return err
		}
		return rt.Output(docAccessEnvelope("doc.access_grant", map[string]any{"resolved": resolvedUserLedger(users), "result": result}))
	},
}

var AccessChange = shortcut.Shortcut{
	Service: "doc", Command: "+access-change", Product: "doc",
	Description: "预检已有协作者后变更文档角色",
	Intent:      "当用户要修改文档上已有协作者的权限角色时使用；先按姓名解析并读取当前权限，目标不是现有协作者时停止，不把 update 当 add。",
	Risk:        shortcut.RiskWrite,
	Safety:      contract.SafetySpec{Effect: "write", Risk: "medium", Confirmation: "user_required", Idempotency: "unknown"},
	Contract: docSmartContract("+access-change", "预检已有协作者后变更文档角色",
		"当用户要修改文档上已有协作者的权限角色时使用；先按姓名解析并读取当前权限，目标不是现有协作者时停止，不把 update 当 add。",
		[]string{`dws doc +access-change --node <DOC_ID> --to 张三 --role EDITOR`}, false),
	Flags: permissionFlags(true),
	Tips:  []string{`dws doc +access-change --node <DOC_ID> --to 张三 --role EDITOR`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		users, err := resolveDocUsers(rt, false)
		if err != nil {
			return err
		}
		current, err := rt.CallMCPData("doc", "list_permission", map[string]any{"nodeId": rt.Str("node")})
		if err != nil {
			return err
		}
		missing := usersMissingPermission(current, users)
		if len(missing) > 0 {
			return apperrors.NewValidation(fmt.Sprintf("以下用户不是当前直接协作者，不能 change；请改用 access-grant: %v", missing))
		}
		params := permissionParams(rt, users)
		if rt.DryRun() {
			return rt.Output(docAccessEnvelope("doc.access_change", map[string]any{"executed": false, "preflight": "existing_collaborators", "params": params}))
		}
		result, err := rt.CallMCPWriteData("doc", "update_permission", params)
		if err != nil {
			return err
		}
		return rt.Output(docAccessEnvelope("doc.access_change", result))
	},
}

var AccessRevoke = shortcut.Shortcut{
	Service: "doc", Command: "+access-revoke", Product: "doc",
	Description: "预检并移除指定协作者的文档权限",
	Intent:      "当用户明确要撤销一位或多位现有协作者对单篇文档的直接权限时使用；先解析姓名并读取权限预检，再执行高风险移除。",
	Risk:        shortcut.RiskHighWrite,
	Safety:      contract.SafetySpec{Effect: "destructive", Risk: "high", Confirmation: "user_required", Idempotency: "unknown"},
	Contract: docSmartContract("+access-revoke", "预检并移除指定协作者的文档权限",
		"当用户明确要撤销一位或多位现有协作者对单篇文档的直接权限时使用；先解析姓名并读取权限预检，再执行高风险移除。",
		[]string{`dws doc +access-revoke --node <DOC_ID> --to 张三`}, false),
	Flags: permissionFlags(false),
	Tips:  []string{`dws doc +access-revoke --node <DOC_ID> --to 张三`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		users, err := resolveDocUsers(rt, false)
		if err != nil {
			return err
		}
		current, err := rt.CallMCPData("doc", "list_permission", map[string]any{"nodeId": rt.Str("node")})
		if err != nil {
			return err
		}
		missing := usersMissingPermission(current, users)
		if len(missing) > 0 {
			return apperrors.NewValidation(fmt.Sprintf("以下用户没有可移除的直接权限: %v", missing))
		}
		params := map[string]any{"nodeId": rt.Str("node"), "userIds": docUserIDs(users)}
		if rt.Str("workspace") != "" {
			params["workspaceId"] = rt.Str("workspace")
		}
		if rt.DryRun() {
			return rt.Output(docAccessEnvelope("doc.access_revoke", map[string]any{"executed": false, "preflight": "existing_collaborators", "params": params}))
		}
		result, err := rt.CallMCPWriteData("doc", "remove_permission", params)
		if err != nil {
			return err
		}
		return rt.Output(docAccessEnvelope("doc.access_revoke", result))
	},
}

var GrantAndShare = shortcut.Shortcut{
	Service: "doc", Command: "+grant-and-share", Product: "doc",
	Description: "确保目标角色后按姓名逐人发送文档链接",
	Intent:      "当用户要确保多人获得指定文档角色后再私信链接时使用；缺少权限时授权、角色不足时升级，无法识别当前角色则停止，再只向权限已经足够的人发送。",
	Risk:        shortcut.RiskWrite,
	Safety:      contract.SafetySpec{Effect: "write", Risk: "medium", Confirmation: "user_required", Idempotency: "unknown"},
	Contract: docSmartContract("+grant-and-share", "确保目标角色后按姓名逐人发送文档链接",
		"当用户要确保多人获得指定文档角色后再私信链接时使用；缺少权限时授权、角色不足时升级，无法识别当前角色则停止，再只向权限已经足够的人发送。",
		[]string{`dws doc +grant-and-share --node <DOC_ID> --url https://alidocs.dingtalk.com/i/nodes/<DOC_ID> --to 张三,李四 --role READER`}, false),
	Flags: append(permissionFlags(true),
		shortcut.Flag{Name: "url", Type: shortcut.FlagString, Desc: "文档链接", Required: true},
		shortcut.Flag{Name: "note", Type: shortcut.FlagString, Desc: "附言"},
		shortcut.AIMessageTagFlag(),
	),
	Tips:    []string{`dws doc +grant-and-share --node <DOC_ID> --url https://alidocs.dingtalk.com/i/nodes/<DOC_ID> --to 张三,李四 --role READER`},
	Execute: executeGrantAndShare,
}

func permissionFlags(withRole bool) []shortcut.Flag {
	flags := []shortcut.Flag{
		{Name: "node", Type: shortcut.FlagString, Desc: "文档 ID 或 URL", Required: true},
		{Name: "to", Type: shortcut.FlagStringSlice, Desc: "协作者姓名列表", Required: true},
	}
	if withRole {
		flags = append(flags, shortcut.Flag{Name: "role", Type: shortcut.FlagString, Default: "READER", Desc: "目标角色", Enum: []string{"READER", "DOWNLOADER", "EDITOR", "MANAGER"}})
	}
	flags = append(flags, shortcut.Flag{Name: "workspace", Type: shortcut.FlagString, Desc: "可选知识库 ID"})
	return flags
}

func resolveDocUsers(rt *shortcut.RuntimeContext, requireOpenID bool) ([]contactUser, error) {
	names := rt.StrSlice("to")
	if len(names) == 0 {
		names = strings.Split(rt.Str("to"), ",")
	}
	users := make([]contactUser, 0, len(names))
	seen := map[string]bool{}
	for _, name := range names {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		var user contactUser
		var err error
		if requireOpenID {
			user, err = resolveDocShareUser(rt, name)
		} else {
			user, err = resolveDocPermissionUser(rt, name)
		}
		if err != nil {
			return nil, err
		}
		if user.userID == "" && !requireOpenID {
			return nil, apperrors.NewValidation(fmt.Sprintf("%s 缺少 userId，不能管理文档权限", name))
		}
		key := user.userID + "\x00" + user.openDingTalkID
		if !seen[key] {
			seen[key] = true
			users = append(users, user)
		}
	}
	if len(users) == 0 {
		return nil, apperrors.NewValidation("--to 至少需要一个可唯一解析的姓名")
	}
	return users, nil
}

func permissionParams(rt *shortcut.RuntimeContext, users []contactUser) map[string]any {
	params := map[string]any{"nodeId": rt.Str("node"), "roleId": strings.ToUpper(rt.Str("role")), "userIds": docUserIDs(users)}
	if rt.Str("workspace") != "" {
		params["workspaceId"] = rt.Str("workspace")
	}
	return params
}

func docUserIDs(users []contactUser) []string {
	ids := make([]string, 0, len(users))
	for _, user := range users {
		ids = append(ids, user.userID)
	}
	return ids
}

func resolvedUserLedger(users []contactUser) []map[string]any {
	out := make([]map[string]any, 0, len(users))
	for _, user := range users {
		out = append(out, map[string]any{"name": user.name, "userId": user.userID, "openDingTalkId": user.openDingTalkID})
	}
	return out
}

func usersMissingPermission(current map[string]any, users []contactUser) []string {
	missingUsers := usersWithoutPermission(current, users)
	var missing []string
	for _, user := range missingUsers {
		missing = append(missing, user.name+"("+user.userID+")")
	}
	return missing
}

func usersWithoutPermission(current map[string]any, users []contactUser) []contactUser {
	present := map[string]bool{}
	collectPermissionUserIDs(current, present)
	missing := make([]contactUser, 0, len(users))
	for _, user := range users {
		if user.userID == "" || !present[user.userID] {
			missing = append(missing, user)
		}
	}
	return missing
}

func collectPermissionUserIDs(value any, into map[string]bool) {
	switch typed := value.(type) {
	case map[string]any:
		for key, item := range typed {
			if (key == "userId" || key == "id") && item != nil {
				if id, ok := item.(string); ok && strings.TrimSpace(id) != "" {
					into[strings.TrimSpace(id)] = true
				}
			}
			collectPermissionUserIDs(item, into)
		}
	case []any:
		for _, item := range typed {
			collectPermissionUserIDs(item, into)
		}
	}
}

type permissionChangePlan struct {
	missing []contactUser
	upgrade []contactUser
	unknown []contactUser
}

func planPermissionChanges(current map[string]any, users []contactUser, targetRole string) permissionChangePlan {
	present := map[string]bool{}
	collectPermissionUserIDs(current, present)
	roles := map[string]string{}
	collectPermissionRoles(current, roles)
	targetRank := permissionRoleRank(targetRole)
	plan := permissionChangePlan{}
	for _, user := range users {
		if user.userID == "" || !present[user.userID] {
			plan.missing = append(plan.missing, user)
			continue
		}
		currentRole, ok := roles[user.userID]
		currentRank := permissionRoleRank(currentRole)
		if !ok || currentRank == 0 || targetRank == 0 {
			plan.unknown = append(plan.unknown, user)
			continue
		}
		if currentRank < targetRank {
			plan.upgrade = append(plan.upgrade, user)
		}
	}
	return plan
}

func collectPermissionRoles(value any, into map[string]string) {
	switch typed := value.(type) {
	case map[string]any:
		role := firstPermissionString(typed, "roleId", "roleID", "role", "permissionRole", "permissionType", "roleType")
		userID := firstPermissionString(typed, "userId", "userID", "memberId", "targetId", "uid")
		if userID == "" && role != "" {
			userID = firstPermissionString(typed, "id")
		}
		if userID != "" && role != "" {
			previous := into[userID]
			if previous == "" || permissionRoleRank(role) > permissionRoleRank(previous) {
				into[userID] = role
			}
		}
		for _, item := range typed {
			collectPermissionRoles(item, into)
		}
	case []any:
		for _, item := range typed {
			collectPermissionRoles(item, into)
		}
	}
}

func firstPermissionString(values map[string]any, keys ...string) string {
	for _, key := range keys {
		if value, ok := values[key].(string); ok && strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func permissionRoleRank(role string) int {
	switch strings.ToUpper(strings.TrimSpace(role)) {
	case "READER":
		return 1
	case "DOWNLOADER":
		return 2
	case "EDITOR":
		return 3
	case "MANAGER":
		return 4
	case "OWNER":
		return 5
	default:
		return 0
	}
}

func permissionUserLabels(users []contactUser) []string {
	labels := make([]string, 0, len(users))
	for _, user := range users {
		labels = append(labels, user.name+"("+user.userID+")")
	}
	return labels
}

func executeShare(rt *shortcut.RuntimeContext) error {
	users, err := resolveDocUsers(rt, true)
	if err != nil {
		return err
	}
	if rt.DryRun() {
		return rt.Output(docAccessEnvelope("doc.share", map[string]any{"executed": false, "resolved": resolvedUserLedger(users), "url": rt.Str("url")}))
	}
	return sendDocLinks(rt, users, "doc.share", nil)
}

func executeGrantAndShare(rt *shortcut.RuntimeContext) error {
	users, err := resolveDocUsers(rt, false)
	if err != nil {
		return err
	}
	for _, user := range users {
		if user.openDingTalkID == "" {
			return apperrors.NewValidation(fmt.Sprintf("%s 缺少 openDingTalkId，已在授权前停止", user.name))
		}
	}
	current, err := rt.CallMCPData("doc", "list_permission", map[string]any{"nodeId": rt.Str("node")})
	if err != nil {
		return err
	}
	plan := planPermissionChanges(current, users, rt.Str("role"))
	if len(plan.unknown) > 0 {
		return apperrors.NewValidation(fmt.Sprintf("以下用户的当前文档角色无法识别，已在授权和发消息前停止: %v", permissionUserLabels(plan.unknown)))
	}
	if rt.DryRun() {
		return rt.Output(docAccessEnvelope("doc.grant_and_share", map[string]any{
			"executed": false, "resolved": resolvedUserLedger(users),
			"wouldGrant": permissionUserLabels(plan.missing), "wouldUpgrade": permissionUserLabels(plan.upgrade), "wouldMessage": len(users),
		}))
	}
	permissionStatus := make(map[string]string, len(users))
	for _, user := range users {
		permissionStatus[user.userID] = "unchanged"
	}
	if len(plan.missing) > 0 {
		if _, err := rt.CallMCPWriteData("doc", "add_permission", permissionParams(rt, plan.missing)); err != nil {
			return err
		}
		for _, user := range plan.missing {
			permissionStatus[user.userID] = "granted"
		}
	}
	if len(plan.upgrade) > 0 {
		if _, err := rt.CallMCPWriteData("doc", "update_permission", permissionParams(rt, plan.upgrade)); err != nil {
			if len(plan.missing) > 0 {
				return apperrors.NewAPI(
					"部分新增权限已写入，但既有用户的角色升级失败；消息尚未发送，请勿直接重试整个命令",
					apperrors.WithOperation("doc.grant_and_share"),
					apperrors.WithReason("doc_grant_permission_partial_failure"),
					apperrors.WithFailureStage("update_permission"),
					apperrors.WithExecutionStarted(true),
					apperrors.WithRetryable(false),
					apperrors.WithActions("inspect current permissions before retrying", "revoke newly added permissions if the whole operation should be rolled back"),
					apperrors.WithDetails(map[string]any{
						"status": "partial_success",
						"steps": []map[string]any{
							{"name": "add_permission", "status": "success"},
							{"name": "update_permission", "status": "failed"},
							{"name": "send_messages", "status": "not_started"},
						},
						"data": map[string]any{
							"granted":        permissionUserLabels(plan.missing),
							"upgradePending": permissionUserLabels(plan.upgrade),
							"messagesSent":   0,
						},
						"compensation": map[string]any{"available": true, "action": "revoke_new_permissions", "users": permissionUserLabels(plan.missing)},
					}),
					apperrors.WithCause(err),
				)
			}
			return err
		}
		for _, user := range plan.upgrade {
			permissionStatus[user.userID] = "upgraded"
		}
	}
	return sendDocLinks(rt, users, "doc.grant_and_share", permissionStatus)
}

func sendDocLinks(rt *shortcut.RuntimeContext, users []contactUser, operation string, permissionStatus map[string]string) error {
	body := shareDocBuildText(rt.Str("url"), rt.Str("note"))
	content, _ := json.Marshal(map[string]string{"title": "文档分享", "text": body})
	ledger := make([]map[string]any, 0, len(users))
	failed := 0
	for _, user := range users {
		params := rt.AddAIMessageTag(map[string]any{"receiverOpenDingTalkId": user.openDingTalkID, "msgType": "markdown", "content": string(content)})
		result, err := rt.CallMCPWriteData("chat", "send_personal_message", params)
		permission := permissionStatus[user.userID]
		if permission == "" {
			permission = "unchanged"
		}
		row := map[string]any{"name": user.name, "userId": user.userID, "permission": permission, "message": "success"}
		if err != nil {
			failed++
			row["message"] = "failed"
			row["error"] = err.Error()
		} else {
			row["result"] = result
		}
		ledger = append(ledger, row)
	}
	status := "success"
	succeeded := len(users) - failed
	if failed == len(users) {
		status = "failed"
	} else if failed > 0 {
		status = "partial_success"
	}
	payload := map[string]any{
		"ok": failed == 0, "status": status, "operation": operation,
		"data": map[string]any{
			"requestedCount": len(users), "succeededCount": succeeded, "failedCount": failed,
			"recipients": ledger,
		},
		"warnings": []string{"权限与消息不构成跨产品事务；消息失败不会自动撤销既有权限"},
	}
	if err := rt.Output(payload); err != nil {
		return err
	}
	if failed > 0 {
		return apperrors.NewAPI(
			fmt.Sprintf("文档链接发送未全部完成：%d/%d 个接收人失败", failed, len(users)),
			apperrors.WithOperation(operation),
			apperrors.WithReason("doc_share_message_failed"),
			apperrors.WithExecutionStarted(true),
			apperrors.WithRetryable(false),
			apperrors.WithDetails(map[string]any{
				"requestedCount": len(users), "succeededCount": succeeded, "failedCount": failed,
				"partial": succeeded > 0,
			}),
		)
	}
	return nil
}

func docAccessEnvelope(operation string, data any) map[string]any {
	return map[string]any{"ok": true, "status": "success", "operation": operation, "data": data, "warnings": []string{}, "compensation": map[string]any{"available": false, "reason": "cross-product writes are not transactional"}}
}

func init() {
	shortcut.Register(AccessGrant, AccessChange, AccessRevoke, GrantAndShare)
}
