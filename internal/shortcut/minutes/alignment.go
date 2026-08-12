// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package minutes

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/localio"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut/minutesdata"
)

var (
	minutesPutFile  = localio.PutFile
	minutesDownload = localio.Download
)

var Search = shortcut.Shortcut{
	Service: "minutes", Command: "+search", Product: "minutes",
	Description: "按范围、标题关键词和时间搜索听记，支持安全全量翻页",
	Intent:      "需要按标题关键词或时间范围搜索自己创建、他人共享或全部可访问听记时使用；对后端返回再做确定性标题匹配，返回稳定 taskUuid 投影与完整性信息。",
	Risk:        shortcut.RiskRead,
	Safety:      contract.SafetySpec{Effect: "read", Risk: "low", Confirmation: "not_required", Idempotency: "idempotent"},
	Contract: minutesContract("+search", "按范围、标题关键词和时间搜索听记，支持安全全量翻页",
		"需要按标题关键词或时间范围搜索自己创建、他人共享或全部可访问听记，并拿到可继续操作的 taskUuid 时使用",
		[]string{"只想无条件浏览一页时可用 +list-mine/+list-shared/+list-all", "DWS 后端不支持 owner/participant 精确过滤，不要伪造该条件"},
		[]string{`dws minutes +search --query "周会" --scope all`, `dws minutes +search --start "2026-08-01T00:00:00+08:00" --scope mine --page-all`}),
	Flags: []shortcut.Flag{
		{Name: "query", Type: shortcut.FlagString, Desc: "标题关键词；shortcut 会对后端结果再次精确包含过滤"},
		{Name: "scope", Type: shortcut.FlagString, Default: "all", Desc: "搜索范围", Enum: []string{"mine", "shared", "all"}},
		{Name: "start", Type: shortcut.FlagString, Desc: "开始时间 RFC3339"},
		{Name: "end", Type: shortcut.FlagString, Desc: "结束时间 RFC3339"},
		{Name: "limit", Type: shortcut.FlagInt, Default: "20", Desc: "每页条数"},
		{Name: "cursor", Type: shortcut.FlagString, Desc: "起始 nextToken"},
		{Name: "page-all", Type: shortcut.FlagBool, Desc: "自动读取全部页"},
		{Name: "page-limit", Type: shortcut.FlagInt, Default: "100", Desc: "自动翻页安全上限"},
	},
	Constraints: []shortcut.Constraint{
		{Kind: shortcut.ConstraintAtLeastOne, Flags: []string{"query", "start", "end"}},
		{Kind: shortcut.ConstraintCustom, Flags: []string{"limit", "page-limit"}, Description: "--limit/--page-limit 必须大于 0"},
		{Kind: shortcut.ConstraintCustom, Flags: []string{"start", "end"}, Description: "时间必须是 RFC3339 且开始时间不能晚于结束时间"},
	},
	Tips: []string{`dws minutes +search --query "周会" --scope all`, `dws minutes +search --start "2026-08-01T00:00:00+08:00" --scope mine --page-all`},
	Validate: func(rt *shortcut.RuntimeContext) error {
		if rt.Int("limit") <= 0 || rt.Int("page-limit") <= 0 {
			return apperrors.NewValidation("--limit 和 --page-limit 必须大于 0")
		}
		_, _, err := minutesTimeRange(rt.Str("start"), rt.Str("end"))
		return err
	},
	Execute: executeMinutesSearch,
}

var Download = shortcut.Shortcut{
	Service: "minutes", Command: "+download", Product: "minutes",
	Description: "批量取得听记音视频地址并安全下载到本地",
	Intent:      "已知一个或多个 taskUuid，需要取得音视频 OSS 地址或以 no-clobber、原子发布方式保存到工作目录时使用。",
	Risk:        shortcut.RiskRead,
	Safety:      contract.SafetySpec{Effect: "read", Risk: "low", Confirmation: "not_required", Idempotency: "idempotent"},
	Contract: minutesContract("+download", "批量取得听记音视频地址并安全下载到本地",
		"已知一个或多个听记 taskUuid，需要下载其音频/视频或显式获取临时 URL 时使用",
		[]string{"只要查看摘要或逐字稿时不要下载媒体", "媒体未就绪、已过期或无痕模式时会明确失败，不返回空成功"},
		[]string{`dws minutes +download --id <taskUuid> --output ./meeting-media`, `dws minutes +download --ids <uuid1,uuid2> --output-dir ./minutes-downloads`}),
	Flags: []shortcut.Flag{
		{Name: "id", Type: shortcut.FlagString, Desc: "单个 taskUuid"},
		{Name: "ids", Type: shortcut.FlagStringSlice, Desc: "多个 taskUuid，最多 50 个"},
		{Name: "url-only", Type: shortcut.FlagBool, Desc: "只返回临时签名 URL，不下载"},
		{Name: "output", Type: shortcut.FlagString, Desc: "单个目标的工作目录内相对输出路径"},
		{Name: "output-dir", Type: shortcut.FlagString, Default: "minutes-downloads", Desc: "批量下载目录"},
	},
	Constraints: []shortcut.Constraint{
		{Kind: shortcut.ConstraintExactlyOne, Flags: []string{"id", "ids"}},
		{Kind: shortcut.ConstraintCustom, Flags: []string{"id", "ids"}, Description: "去重后 taskUuid 数量必须为 1..50"},
		{Kind: shortcut.ConstraintCustom, Flags: []string{"ids", "output"}, Description: "批量下载不能使用 --output"},
		{Kind: shortcut.ConstraintCustom, Flags: []string{"output", "output-dir"}, Description: "输出必须是安全的工作目录相对路径"},
	},
	Tips: []string{`dws minutes +download --id <taskUuid> --output ./meeting-media`, `dws minutes +download --ids <uuid1,uuid2> --output-dir ./minutes-downloads`},
	Validate: func(rt *shortcut.RuntimeContext) error {
		ids := minutesIDs(rt)
		if len(ids) == 0 || len(ids) > 50 {
			return apperrors.NewValidation("taskUuid 数量必须为 1..50")
		}
		if len(ids) > 1 && rt.Changed("output") {
			return apperrors.NewValidation("批量下载不能使用 --output，请使用 --output-dir")
		}
		if rt.Changed("output") {
			return localio.ValidateOutput(rt.Str("output"))
		}
		return localio.ValidateOutput(rt.Str("output-dir"))
	},
	Execute: executeMinutesDownload,
}

var Upload = shortcut.Shortcut{
	Service: "minutes", Command: "+upload", Product: "minutes",
	Description: "把本地音视频完整上传并创建听记，失败时取消会话",
	Intent:      "需要从本地音视频直接创建听记时使用；完成 create、预签名 PUT、complete 和最终详情读回，不需要中转 Drive。",
	Risk:        shortcut.RiskWrite,
	Safety:      contract.SafetySpec{Effect: "write", Risk: "medium", Confirmation: "user_required", Idempotency: "unknown"},
	Contract: withMinutesDryRun(minutesContract("+upload", "把本地音视频完整上传并创建听记，失败时取消会话",
		"用户有本地音视频，希望直接上传生成听记并取得可读回的 taskUuid 时使用",
		[]string{"只需要管理已有 upload session 时使用原子 upload create/complete/cancel", "文件为空、过大或不希望创建远端听记时不要执行"},
		[]string{`dws minutes +upload --file ./meeting.mp3 --title "项目周会"`, `dws minutes +upload --file ./meeting.mp4 --input-language zh`}), contract.DryRunPreviewPlan, false),
	Flags: []shortcut.Flag{
		{Name: "file", Type: shortcut.FlagString, Desc: "本地音视频文件", Required: true},
		{Name: "title", Type: shortcut.FlagString, Desc: "听记标题"},
		{Name: "template-id", Type: shortcut.FlagString, Desc: "纪要模板 ID"},
		{Name: "input-language", Type: shortcut.FlagString, Desc: "ASR 输入语言"},
		{Name: "enable-message-card", Type: shortcut.FlagBool, Desc: "生成后推送闪记卡片"},
		{Name: "complete-timeout", Type: shortcut.FlagInt, Default: "90", Desc: "等待服务端确认上传完成的秒数"},
		{Name: "poll-interval", Type: shortcut.FlagInt, Default: "2", Desc: "complete 重试间隔秒数"},
	},
	Constraints: []shortcut.Constraint{{Kind: shortcut.ConstraintCustom, Flags: []string{"complete-timeout", "poll-interval"}, Description: "--complete-timeout 和 --poll-interval 必须大于 0"}},
	Tips:        []string{`dws minutes +upload --file ./meeting.mp3 --title "项目周会"`, `dws minutes +upload --file ./meeting.mp4 --input-language zh`},
	Validate: func(rt *shortcut.RuntimeContext) error {
		if rt.Int("complete-timeout") <= 0 || rt.Int("poll-interval") <= 0 {
			return apperrors.NewValidation("--complete-timeout 和 --poll-interval 必须大于 0")
		}
		return nil
	},
	Execute: executeMinutesUpload,
}

var Update = shortcut.Shortcut{
	Service: "minutes", Command: "+update", Product: "minutes",
	Description: "读取现状、预览差异、更新听记标题并读回验证",
	Intent:      "已知 taskUuid，需要安全重命名听记标题并确认远端最终值时使用。",
	Risk:        shortcut.RiskWrite,
	Safety:      contract.SafetySpec{Effect: "write", Risk: "medium", Confirmation: "user_required", Idempotency: "idempotent"},
	Contract: withMinutesDryRun(minutesContract("+update", "读取现状、更新听记标题并读回验证；dry-run 输出不访问远端的执行计划",
		"需要修改一条听记的标题，并通过最终读回证明修改生效时使用",
		[]string{"要修改纪要正文时使用 +summary"},
		[]string{`dws minutes +update --id <taskUuid> --title "Q3 项目复盘"`}), contract.DryRunPreviewPlan, false),
	Flags: []shortcut.Flag{
		{Name: "id", Type: shortcut.FlagString, Desc: "听记 taskUuid", Required: true},
		{Name: "title", Type: shortcut.FlagString, Desc: "新标题", Required: true},
	},
	Tips:    []string{`dws minutes +update --id <taskUuid> --title "Q3 项目复盘"`},
	Execute: executeMinutesUpdate,
}

var ApplyPermission = shortcut.Shortcut{
	Service: "minutes", Command: "+apply-permission", Product: "minutes",
	Description: "为当前用户申请语义化的听记访问权限",
	Intent:      "打开听记无权限时，用 view/download/edit 语义申请对应权限，不需要记 policy 数字。",
	Risk:        shortcut.RiskWrite,
	Safety:      contract.SafetySpec{Effect: "write", Risk: "medium", Confirmation: "user_required", Idempotency: "unknown"},
	Contract: withMinutesDryRun(minutesContract("+apply-permission", "为当前用户申请语义化的听记访问权限",
		"当前用户访问某条听记被拒绝，需要向所有者申请查看、下载或编辑权限时使用",
		[]string{"所有者给其他成员授权时使用 +share", "要移除权限时使用 +unshare"},
		[]string{`dws minutes +apply-permission --id <taskUuid> --permission view`, `dws minutes +apply-permission --id <taskUuid> --permission edit`}), contract.DryRunPreviewPlan, false),
	Flags: []shortcut.Flag{
		{Name: "id", Type: shortcut.FlagString, Desc: "听记 taskUuid", Required: true},
		{Name: "permission", Type: shortcut.FlagString, Desc: "申请权限", Required: true, Enum: []string{"view", "download", "edit"}},
	},
	Tips:    []string{`dws minutes +apply-permission --id <taskUuid> --permission view`, `dws minutes +apply-permission --id <taskUuid> --permission edit`},
	Execute: executeMinutesApplyPermission,
}

var Summary = shortcut.Shortcut{
	Service: "minutes", Command: "+summary", Product: "minutes",
	Description: "读取当前纪要、校验图片引用、全量覆盖并读回验证",
	Intent:      "需要安全编辑听记纪要正文时使用；支持字面量、@file 和 stdin，保护现有 Markdown 图片并验证最终全文。",
	Risk:        shortcut.RiskWrite,
	Safety:      contract.SafetySpec{Effect: "write", Risk: "medium", Confirmation: "user_required", Idempotency: "idempotent"},
	Contract: withMinutesDryRun(minutesContract("+summary", "校验本地纪要输入、全量覆盖并读回验证；dry-run 不访问远端",
		"需要用确认后的完整 Markdown 覆盖一条听记纪要，并保护已有图片引用时使用",
		[]string{"只改标题时使用 +update", "没有完整目标内容时不要执行全量覆盖"},
		[]string{`dws minutes +summary --id <taskUuid> --content "项目周会纪要"`}), contract.DryRunPreviewPlan, false),
	Flags: []shortcut.Flag{
		{Name: "id", Type: shortcut.FlagString, Desc: "听记 taskUuid", Required: true},
		{Name: "content", Type: shortcut.FlagString, Desc: "完整纪要字面量、@相对文件或 - 表示 stdin", Required: true},
	},
	Tips:    []string{`dws minutes +summary --id <taskUuid> --content @summary.md`, `dws minutes +summary --id <taskUuid> --content -`},
	Execute: executeMinutesSummary,
}

var SpeakerReplace = shortcut.Shortcut{
	Service: "minutes", Command: "+speaker-replace", Product: "minutes",
	Description: "预检逐字稿中的发言人昵称，替换后重新读回验证",
	Intent:      "需要把听记中的一个发言人昵称替换为目标昵称/UID 时使用；这是昵称替换，不是 Lark speaker_id 身份重绑。",
	Risk:        shortcut.RiskWrite,
	Safety:      contract.SafetySpec{Effect: "write", Risk: "medium", Confirmation: "user_required", Idempotency: "idempotent"},
	Contract: withMinutesDryRun(minutesContract("+speaker-replace", "预检逐字稿中的发言人昵称，替换后重新读回验证",
		"逐字稿已经生成且用户明确要把一个源发言人昵称批量替换为目标昵称时使用",
		[]string{"DWS 不是 speaker_id 到用户 open_id 的身份重绑；需要该语义时当前无法对齐", "源昵称不存在或目标身份不确定时不要写"},
		[]string{`dws minutes +speaker-replace --id <taskUuid> --from "发言人1" --to "张三"`}), contract.DryRunPreviewPlan, false),
	Flags: []shortcut.Flag{
		{Name: "id", Type: shortcut.FlagString, Desc: "听记 taskUuid", Required: true},
		{Name: "from", Type: shortcut.FlagString, Desc: "源发言人昵称", Required: true},
		{Name: "to", Type: shortcut.FlagString, Desc: "目标发言人昵称", Required: true},
		{Name: "target-uid", Type: shortcut.FlagString, Desc: "目标钉钉 UID"},
		{Name: "page-limit", Type: shortcut.FlagInt, Default: "100", Desc: "逐字稿预检翻页上限"},
	},
	Constraints: []shortcut.Constraint{
		{Kind: shortcut.ConstraintCustom, Flags: []string{"from", "to"}, Description: "--from/--to 去空白后不能相同"},
		{Kind: shortcut.ConstraintCustom, Flags: []string{"page-limit"}, Description: "--page-limit 必须大于 0"},
	},
	Tips: []string{`dws minutes +speaker-replace --id <taskUuid> --from "发言人1" --to "张三"`},
	Validate: func(rt *shortcut.RuntimeContext) error {
		if strings.TrimSpace(rt.Str("from")) == strings.TrimSpace(rt.Str("to")) {
			return apperrors.NewValidation("--from 与 --to 不能相同")
		}
		if rt.Int("page-limit") <= 0 {
			return apperrors.NewValidation("--page-limit 必须大于 0")
		}
		return nil
	},
	Execute: executeMinutesSpeakerReplace,
}

func executeMinutesSearch(rt *shortcut.RuntimeContext) error {
	start, end, err := minutesTimeRange(rt.Str("start"), rt.Str("end"))
	if err != nil {
		return err
	}
	belonging := map[string]string{"mine": "createdByMe", "shared": "sharedToMe", "all": "noLimit"}[rt.Str("scope")]
	token := rt.Str("cursor")
	seenTokens := map[string]bool{}
	seenIDs := map[string]bool{}
	rows := make([]map[string]any, 0)
	pages := 0
	complete := false
	nextToken := ""
	for {
		if seenTokens[token] {
			return fmt.Errorf("minutes search cursor stalled or cycled")
		}
		seenTokens[token] = true
		params := map[string]any{"belongingConditionId": belonging, "maxResults": rt.Int("limit")}
		if query := strings.TrimSpace(rt.Str("query")); query != "" {
			params["keyword"] = query
		}
		if start != 0 {
			params["createTimeStart"] = float64(start)
		}
		if end != 0 {
			params["createTimeEnd"] = float64(end)
		}
		if token != "" {
			params["nextToken"] = token
		}
		data, err := rt.CallMCPData("minutes", "list_by_keyword_and_time_range", params)
		if err != nil {
			return err
		}
		page, err := minutesdata.ParseListPage(data)
		if err != nil {
			return err
		}
		projected, err := minutesdata.ProjectList(page)
		if err != nil {
			return err
		}
		for _, row := range projected {
			id := row["taskUuid"].(string)
			if seenIDs[id] {
				continue
			}
			seenIDs[id] = true
			rows = append(rows, row)
		}
		pages++
		nextToken = page.NextToken
		if !page.HasMore {
			complete = true
			nextToken = ""
			break
		}
		if !rt.Bool("page-all") {
			break
		}
		if pages >= rt.Int("page-limit") {
			return fmt.Errorf("minutes search exceeded page safety limit %d", rt.Int("page-limit"))
		}
		token = page.NextToken
	}
	scanned := len(rows)
	if query := strings.TrimSpace(rt.Str("query")); query != "" {
		needle := strings.ToLower(query)
		filtered := make([]map[string]any, 0, len(rows))
		for _, row := range rows {
			title, _ := row["title"].(string)
			if strings.Contains(strings.ToLower(title), needle) {
				filtered = append(filtered, row)
			}
		}
		rows = filtered
	}
	payload := map[string]any{"count": len(rows), "scannedCount": scanned, "minutes": rows, "pages": pages, "complete": complete}
	if nextToken != "" {
		payload["nextToken"] = nextToken
	}
	return rt.Output(payload)
}

func executeMinutesDownload(rt *shortcut.RuntimeContext) error {
	ids := minutesIDs(rt)
	results := make([]map[string]any, 0, len(ids))
	failures := make([]map[string]any, 0)
	for _, id := range ids {
		data, err := rt.CallMCPData("minutes", "query_minutes_audio_url", map[string]any{"taskUuid": id})
		if err != nil {
			failures = append(failures, map[string]any{"taskUuid": id, "error": err.Error()})
			continue
		}
		mediaURL, err := minutesdata.MediaURL(data)
		if err != nil {
			failures = append(failures, map[string]any{"taskUuid": id, "error": err.Error()})
			continue
		}
		if rt.Bool("url-only") {
			results = append(results, map[string]any{"taskUuid": id, "url": mediaURL})
			continue
		}
		output := rt.Str("output-dir") + "/"
		if len(ids) == 1 && rt.Changed("output") {
			output = rt.Str("output")
		}
		preferred := id + mediaExtension(mediaURL)
		download, err := minutesDownload(rt.Command().Context(), mediaURL, localio.DownloadOptions{Output: output, PreferredName: preferred})
		if err != nil {
			failures = append(failures, map[string]any{"taskUuid": id, "error": err.Error()})
			continue
		}
		results = append(results, map[string]any{"taskUuid": id, "path": download.RelativePath, "sizeBytes": download.SizeBytes})
	}
	payload := map[string]any{"ok": len(failures) == 0, "requested": len(ids), "succeeded": len(results), "failed": len(failures), "results": results}
	if len(failures) > 0 {
		payload["failures"] = failures
	}
	if err := rt.Output(payload); err != nil {
		return err
	}
	if len(failures) > 0 {
		return minutesCompositeError("minutes_download_incomplete", "download", payload)
	}
	return nil
}

func executeMinutesUpload(rt *shortcut.RuntimeContext) error {
	payload, err := performMinutesUpload(rt)
	if err != nil {
		return err
	}
	return rt.Output(payload)
}

func performMinutesUpload(rt *shortcut.RuntimeContext) (map[string]any, error) {
	info, err := os.Stat(rt.Str("file"))
	if err != nil || !info.Mode().IsRegular() || info.Size() <= 0 {
		return nil, apperrors.NewValidation("--file 必须是非空普通文件")
	}
	plan := map[string]any{"operation": "minutes.upload", "fileName": info.Name(), "sizeBytes": info.Size(), "title": rt.Str("title")}
	if rt.DryRun() {
		return minutesDryRunPayload(contract.DryRunPreviewPlan, "minutes.upload", plan), nil
	}
	params := map[string]any{"fileName": info.Name(), "fileSize": info.Size()}
	if value := strings.TrimSpace(rt.Str("title")); value != "" {
		params["title"] = value
	}
	option := map[string]any{}
	if value := strings.TrimSpace(rt.Str("template-id")); value != "" {
		option["templateId"] = value
	}
	if value := strings.TrimSpace(rt.Str("input-language")); value != "" {
		option["inputLanguage"] = value
	}
	if rt.Changed("enable-message-card") {
		option["enableMessageCard"] = rt.Bool("enable-message-card")
	}
	if len(option) > 0 {
		params["minutesOption"] = option
	}
	created, err := rt.CallMCPWriteDataStrict("minutes", "create_upload_session", params)
	if err != nil {
		return nil, err
	}
	sessionID, presignedURL, err := minutesdata.UploadSession(created)
	if err != nil {
		return nil, err
	}
	transfer, err := minutesPutFile(rt.Command().Context(), presignedURL, rt.Str("file"), 0)
	if err != nil {
		cancelled, cancelErr := cancelMinutesUpload(rt, sessionID)
		return nil, minutesUploadFailure("put", sessionID, cancelled, cancelErr, err)
	}
	completed, completeAttempts, err := completeMinutesUpload(rt, sessionID, time.Duration(rt.Int("complete-timeout"))*time.Second, time.Duration(rt.Int("poll-interval"))*time.Second)
	if err != nil {
		return nil, minutesUploadCompletionUnknown(sessionID, completeAttempts, err)
	}
	taskUUID, err := minutesdata.CompletedTaskUUID(completed)
	if err != nil {
		return nil, minutesUploadFailure("complete_response", sessionID, false, nil, err)
	}
	readBack, err := rt.CallMCPData("minutes", "get_minutes_basic_info", map[string]any{"taskUuid": taskUUID})
	if err != nil {
		return nil, minutesUploadFailure("verify", sessionID, false, nil, err)
	}
	if _, err := minutesdata.Basic(taskUUID, readBack); err != nil {
		return nil, minutesUploadFailure("verify", sessionID, false, nil, err)
	}
	return map[string]any{"operation": "minutes.upload", "complete": true, "sessionId": sessionID, "taskUuid": taskUUID, "sizeBytes": transfer.SizeBytes, "uploadAttempts": transfer.Attempts, "completeAttempts": completeAttempts, "verified": true}, nil
}

func executeMinutesUpdate(rt *shortcut.RuntimeContext) error {
	id, title := rt.Str("id"), strings.TrimSpace(rt.Str("title"))
	if rt.DryRun() {
		return rt.Output(minutesDryRunPayload(contract.DryRunPreviewPlan, "minutes.update", map[string]any{
			"taskUuid": id, "after": title, "remotePreconditions": []string{"read current title", "write title", "read back exact title"},
		}))
	}
	beforeData, err := rt.CallMCPData("minutes", "get_minutes_basic_info", map[string]any{"taskUuid": id})
	if err != nil {
		return err
	}
	before, err := minutesdata.Basic(id, beforeData)
	if err != nil {
		return err
	}
	oldTitle, _ := before["title"].(string)
	if oldTitle == title {
		return rt.Output(map[string]any{"taskUuid": id, "changed": false, "verified": true, "title": title})
	}
	if _, err := rt.CallMCPWriteDataStrict("minutes", "update_minutes_title", map[string]any{"taskUuid": id, "title": title}); err != nil {
		return err
	}
	afterData, err := rt.CallMCPData("minutes", "get_minutes_basic_info", map[string]any{"taskUuid": id})
	if err != nil {
		return err
	}
	after, err := minutesdata.Basic(id, afterData)
	if err != nil || after["title"] != title {
		return minutesCompositeError("minutes_title_verification_failed", "verify", map[string]any{"taskUuid": id, "expectedTitle": title})
	}
	return rt.Output(map[string]any{"taskUuid": id, "changed": true, "verified": true, "before": oldTitle, "after": title})
}

func executeMinutesApplyPermission(rt *shortcut.RuntimeContext) error {
	policy := map[string]float64{"edit": 2, "download": 3, "view": 4}[rt.Str("permission")]
	plan := map[string]any{"taskUuid": rt.Str("id"), "permission": rt.Str("permission"), "policyId": policy}
	if rt.DryRun() {
		return rt.Output(minutesDryRunPayload(contract.DryRunPreviewPlan, "minutes.apply_permission", plan))
	}
	data, err := rt.CallMCPWriteDataStrict("minutes", "apply_minutes_permission", map[string]any{"taskUuid": rt.Str("id"), "policyId": policy})
	if err != nil {
		return err
	}
	if err := minutesdata.RequireWriteAcknowledgement("apply permission", data); err != nil {
		return err
	}
	plan["requested"], plan["result"] = true, data["result"]
	return rt.Output(plan)
}

func executeMinutesSummary(rt *shortcut.RuntimeContext) error {
	id := rt.Str("id")
	after, err := localio.ReadTextInput(rt.Str("content"), rt.Command().InOrStdin(), 8<<20)
	if err != nil {
		return apperrors.NewValidation(err.Error())
	}
	if strings.TrimSpace(after) == "" {
		return apperrors.NewValidation("纪要内容不能为空")
	}
	if rt.DryRun() {
		return rt.Output(minutesDryRunPayload(contract.DryRunPreviewPlan, "minutes.summary", map[string]any{
			"taskUuid": id, "afterBytes": len(after), "remotePreconditions": []string{"read current summary", "preserve existing Markdown images", "write summary", "read back exact summary"},
		}))
	}
	beforeData, err := rt.CallMCPData("minutes", "get_minutes_ai_summary", map[string]any{"taskUuid": id})
	if err != nil {
		return err
	}
	before, err := minutesdata.SummaryText(beforeData)
	if err != nil {
		return err
	}
	if missing := missingMarkdownImages(before, after); len(missing) > 0 {
		return apperrors.NewValidation(fmt.Sprintf("新纪要丢失 %d 个原有 Markdown 图片引用", len(missing)))
	}
	if before == after {
		return rt.Output(map[string]any{"taskUuid": id, "changed": false, "verified": true})
	}
	if _, err := rt.CallMCPWriteDataStrict("minutes", "update_minutes_summary", map[string]any{"taskUuid": id, "summaryText": after}); err != nil {
		return err
	}
	readBackData, err := rt.CallMCPData("minutes", "get_minutes_ai_summary", map[string]any{"taskUuid": id})
	if err != nil {
		return err
	}
	readBack, err := minutesdata.SummaryText(readBackData)
	if err != nil || readBack != after {
		return minutesCompositeError("minutes_summary_verification_failed", "verify", map[string]any{"taskUuid": id, "expectedBytes": len(after)})
	}
	return rt.Output(map[string]any{"taskUuid": id, "changed": true, "verified": true, "beforeBytes": len(before), "afterBytes": len(after), "preservedImages": true})
}

func executeMinutesSpeakerReplace(rt *shortcut.RuntimeContext) error {
	id, from, to := rt.Str("id"), rt.Str("from"), rt.Str("to")
	if rt.DryRun() {
		return rt.Output(minutesDryRunPayload(contract.DryRunPreviewPlan, "minutes.speaker_replace", map[string]any{
			"taskUuid": id, "from": from, "to": to, "targetUid": strings.TrimSpace(rt.Str("target-uid")),
			"remotePreconditions": []string{"read full transcript", "verify source speaker exists", "replace speaker", "read back full transcript"},
		}))
	}
	before, err := collectTranscriptForMinutes(rt, id, rt.Int("page-limit"))
	if err != nil {
		return err
	}
	beforeSpeakers := speakerCounts(before.Paragraphs)
	if beforeSpeakers[from] == 0 {
		return apperrors.NewValidation(fmt.Sprintf("逐字稿中不存在发言人 %q", from))
	}
	params := map[string]any{"taskUuid": id, "speakerNick": from, "targetNickName": to}
	if uid := strings.TrimSpace(rt.Str("target-uid")); uid != "" {
		params["targetUid"] = uid
	}
	if _, err := rt.CallMCPWriteDataStrict("minutes", "replace_speaker", params); err != nil {
		return err
	}
	after, err := collectTranscriptForMinutes(rt, id, rt.Int("page-limit"))
	if err != nil {
		return err
	}
	afterSpeakers := speakerCounts(after.Paragraphs)
	if afterSpeakers[from] != 0 || afterSpeakers[to] < beforeSpeakers[from] {
		return minutesCompositeError("minutes_speaker_verification_failed", "verify", map[string]any{"taskUuid": id, "fromRemaining": afterSpeakers[from], "targetCount": afterSpeakers[to]})
	}
	return rt.Output(map[string]any{"taskUuid": id, "changed": true, "verified": true, "from": from, "to": to, "affectedParagraphs": beforeSpeakers[from]})
}

func minutesTimeRange(startRaw, endRaw string) (int64, int64, error) {
	parse := func(name, raw string) (int64, error) {
		if strings.TrimSpace(raw) == "" {
			return 0, nil
		}
		value, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			return 0, apperrors.NewValidation(fmt.Sprintf("--%s 必须是 RFC3339 时间: %v", name, err))
		}
		return value.UnixMilli(), nil
	}
	start, err := parse("start", startRaw)
	if err != nil {
		return 0, 0, err
	}
	end, err := parse("end", endRaw)
	if err != nil {
		return 0, 0, err
	}
	if start != 0 && end != 0 && start > end {
		return 0, 0, apperrors.NewValidation("--start 不能晚于 --end")
	}
	return start, end, nil
}

func minutesIDs(rt *shortcut.RuntimeContext) []string {
	values := []string{}
	if id := strings.TrimSpace(rt.Str("id")); id != "" {
		values = append(values, id)
	}
	values = append(values, rt.StrSlice("ids")...)
	seen := map[string]bool{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		for _, item := range strings.Split(value, ",") {
			item = strings.TrimSpace(item)
			if item != "" && !seen[item] {
				seen[item] = true
				out = append(out, item)
			}
		}
	}
	return out
}

func mediaExtension(rawURL string) string {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	ext := strings.ToLower(filepath.Ext(parsed.Path))
	if len(ext) > 10 {
		return ""
	}
	return ext
}

func cancelMinutesUpload(rt *shortcut.RuntimeContext, sessionID string) (bool, error) {
	data, err := rt.CallMCPWriteDataStrict("minutes", "cancel_upload_session", map[string]any{"sessionId": sessionID})
	if err != nil {
		return false, err
	}
	if err := minutesdata.RequireWriteAcknowledgement("cancel upload", data); err != nil {
		return false, err
	}
	return true, nil
}

func completeMinutesUpload(rt *shortcut.RuntimeContext, sessionID string, timeout, interval time.Duration) (map[string]any, int, error) {
	deadline := time.Now().Add(timeout)
	attempts := 0
	for {
		attempts++
		data, err := rt.CallMCPWriteDataStrict("minutes", "complete_upload_session", map[string]any{"sessionId": sessionID})
		if err == nil {
			return data, attempts, nil
		}
		message := strings.ToLower(err.Error())
		retryable := strings.Contains(message, "still uploading") || strings.Contains(message, "wait and retry") || strings.Contains(message, "正在上传")
		if !retryable || time.Now().Add(interval).After(deadline) {
			return nil, attempts, err
		}
		timer := time.NewTimer(interval)
		select {
		case <-rt.Command().Context().Done():
			timer.Stop()
			return nil, attempts, rt.Command().Context().Err()
		case <-timer.C:
		}
	}
}

func minutesUploadFailure(stage, sessionID string, cancelled bool, cancelErr, cause error) error {
	details := map[string]any{"stage": stage, "sessionId": sessionID, "cancelled": cancelled}
	if cause != nil {
		details["cause"] = cause.Error()
	}
	if cancelErr != nil {
		details["cancelError"] = cancelErr.Error()
	}
	return apperrors.NewAPI(
		fmt.Sprintf("听记上传在 %s 阶段失败", stage),
		apperrors.WithOperation("minutes/+upload"),
		apperrors.WithOrigin("shortcut"),
		apperrors.WithFailureStage(stage),
		apperrors.WithReason("minutes_upload_failed"),
		apperrors.WithExecutionStarted(true),
		apperrors.WithRetryable(false),
		apperrors.WithDetails(details),
	)
}

func minutesUploadCompletionUnknown(sessionID string, attempts int, cause error) error {
	details := map[string]any{
		"stage":            "complete",
		"sessionId":        sessionID,
		"completeAttempts": attempts,
		"cancelled":        false,
		"remoteEffect":     "unknown",
		"recovery": map[string]any{
			"action":     "verify_before_cancel",
			"nextAction": "保留 sessionId；先重试 complete_upload_session 或查询最近听记确认服务端状态，只有明确证明未完成时才取消会话",
		},
	}
	if cause != nil {
		details["cause"] = cause.Error()
	}
	return apperrors.NewAPI(
		"听记上传 complete 请求的远端结果未知；为避免破坏可能已完成的上传，已保留会话且未自动取消",
		apperrors.WithOperation("minutes/+upload"),
		apperrors.WithOrigin("shortcut"),
		apperrors.WithFailureStage("complete"),
		apperrors.WithReason("minutes_upload_completion_unknown"),
		apperrors.WithExecutionStarted(true),
		apperrors.WithRetryable(false),
		apperrors.WithHint("不要重新上传或直接取消；先使用错误详情中的 sessionId 核验服务端状态"),
		apperrors.WithActions("retry complete_upload_session with the preserved sessionId", "query recent Minutes before cancelling the upload session"),
		apperrors.WithDetails(details),
	)
}

func minutesCompositeError(reason, stage string, details map[string]any) error {
	return apperrors.NewAPI(
		"Minutes shortcut 未完成，详见结构化错误",
		apperrors.WithOperation("minutes/shortcut"),
		apperrors.WithOrigin("shortcut"),
		apperrors.WithFailureStage(stage),
		apperrors.WithReason(reason),
		apperrors.WithRetryable(false),
		apperrors.WithDetails(details),
	)
}

var markdownImageRE = regexp.MustCompile(`!\[[^\]]*\]\(([^)]+)\)`)

func missingMarkdownImages(before, after string) []string {
	remaining := map[string]bool{}
	for _, match := range markdownImageRE.FindAllStringSubmatch(after, -1) {
		remaining[match[1]] = true
	}
	missing := []string{}
	for _, match := range markdownImageRE.FindAllStringSubmatch(before, -1) {
		if !remaining[match[1]] {
			missing = append(missing, match[1])
		}
	}
	sort.Strings(missing)
	return missing
}

func collectTranscriptForMinutes(rt *shortcut.RuntimeContext, taskUUID string, pageLimit int) (minutesdata.TranscriptResult, error) {
	return minutesdata.CollectTranscript(func(nextToken string) (map[string]any, error) {
		params := map[string]any{"taskUuid": taskUUID, "direction": "0"}
		if nextToken != "" {
			params["nextToken"] = nextToken
		}
		return rt.CallMCPData("minutes", "get_minutes_transcription", params)
	}, "", false, pageLimit)
}

func speakerCounts(paragraphs []map[string]any) map[string]int {
	counts := map[string]int{}
	var visit func(any, map[string]bool)
	visit = func(value any, seen map[string]bool) {
		switch typed := value.(type) {
		case map[string]any:
			local := map[string]bool{}
			for _, key := range []string{"speakerNick", "nickName", "nickname", "subSpeakerNickname"} {
				if name, ok := typed[key].(string); ok && strings.TrimSpace(name) != "" {
					local[strings.TrimSpace(name)] = true
				}
			}
			for name := range local {
				seen[name] = true
			}
			for _, child := range typed {
				visit(child, seen)
			}
		case []any:
			for _, child := range typed {
				visit(child, seen)
			}
		}
	}
	for _, paragraph := range paragraphs {
		seen := map[string]bool{}
		visit(paragraph, seen)
		for name := range seen {
			counts[name]++
		}
	}
	return counts
}

func init() {
	shortcut.Register(finalizeMinutesShortcuts(Search, Download, Upload, Update, ApplyPermission, Summary, SpeakerReplace)...)
}
