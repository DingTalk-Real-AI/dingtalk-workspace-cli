// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package minutes

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/localio"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut/minutesdata"
)

var minutesWorkflowArtifacts = []string{"basic", "summary", "keywords", "transcript", "todos"}

var (
	minutesMkdirTemp     = os.MkdirTemp
	minutesRemoveAll     = os.RemoveAll
	minutesWriteFile     = os.WriteFile
	minutesStat          = os.Stat
	minutesRename        = os.Rename
	minutesGetwd         = os.Getwd
	minutesEvalSymlinks  = filepath.EvalSymlinks
	minutesRel           = filepath.Rel
	minutesLstat         = os.Lstat
	minutesMkdir         = os.Mkdir
	minutesMarshalIndent = json.MarshalIndent
)

var RecordWrapUp = shortcut.Shortcut{
	Service: "minutes", Command: "+record-wrap-up", Product: "minutes",
	Description: "停止实时录音并有界等待听记产物，失败时保留恢复句柄",
	Intent:      "会议结束时一次完成 stop 与 basic/summary/keywords/transcript/todos 读取；停止已生效但产物未就绪时返回非零 partial 结果和 taskUuid，不会诱导重复 stop。",
	Risk:        shortcut.RiskWrite,
	Safety:      contract.SafetySpec{Effect: "write", Risk: "medium", Confirmation: "user_required", Idempotency: "unknown"},
	Contract: withMinutesDryRun(minutesContract("+record-wrap-up", "停止实时录音并有界等待听记产物，失败时保留恢复句柄",
		"已知正在录制的 taskUuid，会议结束后要停止录制并立即收口转写、纪要、关键词和行动项时使用",
		[]string{"仍要继续录音时使用 +record-pause；只停止、不等待产物时使用 +record-stop"},
		[]string{`dws minutes +record-wrap-up --id <taskUuid>`, `dws minutes +record-wrap-up --id <taskUuid> --artifacts transcript,summary --wait-timeout 120`}), contract.DryRunPreviewPlan, false),
	Flags: []shortcut.Flag{
		{Name: "id", Type: shortcut.FlagString, Desc: "正在录制的听记 taskUuid", Required: true},
		{Name: "artifacts", Type: shortcut.FlagStringSlice, Desc: "停止后等待的产物", Enum: minutesWorkflowArtifacts},
		{Name: "wait-timeout", Type: shortcut.FlagInt, Default: "90", Desc: "等待产物秒数"},
		{Name: "poll-interval", Type: shortcut.FlagInt, Default: "3", Desc: "轮询间隔秒数"},
		{Name: "page-limit", Type: shortcut.FlagInt, Default: "100", Desc: "逐字稿翻页上限"},
	},
	Constraints: []shortcut.Constraint{{Kind: shortcut.ConstraintCustom, Flags: []string{"wait-timeout", "poll-interval", "page-limit"}, Description: "超时、轮询间隔和逐字稿页数上限必须大于 0"}},
	Tips:        []string{`dws minutes +record-wrap-up --id <taskUuid>`, `dws minutes +record-wrap-up --id <taskUuid> --artifacts transcript,summary --wait-timeout 120`},
	Validate:    validateMinutesWorkflowWait,
	Execute:     executeMinutesRecordWrapUp,
}

var UploadAndAnalyze = shortcut.Shortcut{
	Service: "minutes", Command: "+upload-and-analyze", Product: "minutes",
	Description: "本地音视频直传听记并等待分析产物，可选思维导图和发言人洞察",
	Intent:      "从本地媒体一次完成 upload create/PUT/complete/read-back，再等待选定听记产物；后续分析失败仍返回已创建 taskUuid 和恢复动作。",
	Risk:        shortcut.RiskWrite,
	Safety:      contract.SafetySpec{Effect: "write", Risk: "medium", Confirmation: "user_required", Idempotency: "unknown"},
	Contract: withMinutesDryRun(minutesContract("+upload-and-analyze", "本地音视频直传听记并等待分析产物，可选思维导图和发言人洞察",
		"有本地音视频且希望一次创建听记、等待基础分析，并可继续生成思维导图或发言人洞察时使用",
		[]string{"只需上传时使用 +upload；已有 taskUuid 时分别使用 +detail/+mindmap/+speaker-insights"},
		[]string{`dws minutes +upload-and-analyze --file ./meeting.mp3 --title "项目周会"`, `dws minutes +upload-and-analyze --file ./meeting.mp4 --artifacts transcript,summary --mindmap`}), contract.DryRunPreviewPlan, false),
	Flags: []shortcut.Flag{
		{Name: "file", Type: shortcut.FlagString, Desc: "本地音视频文件；与 --resume-id 二选一"},
		{Name: "resume-id", Type: shortcut.FlagString, Desc: "先前已成功上传的 taskUuid；只恢复分析、不重复上传"},
		{Name: "title", Type: shortcut.FlagString, Desc: "听记标题"},
		{Name: "template-id", Type: shortcut.FlagString, Desc: "纪要模板 ID"},
		{Name: "input-language", Type: shortcut.FlagString, Desc: "ASR 输入语言"},
		{Name: "enable-message-card", Type: shortcut.FlagBool, Desc: "生成后推送闪记卡片"},
		{Name: "complete-timeout", Type: shortcut.FlagInt, Default: "90", Desc: "上传 complete 超时秒数"},
		{Name: "poll-interval", Type: shortcut.FlagInt, Default: "3", Desc: "轮询间隔秒数"},
		{Name: "wait-timeout", Type: shortcut.FlagInt, Default: "180", Desc: "等待分析产物秒数"},
		{Name: "artifacts", Type: shortcut.FlagStringSlice, Desc: "等待的分析产物", Enum: minutesWorkflowArtifacts},
		{Name: "page-limit", Type: shortcut.FlagInt, Default: "100", Desc: "逐字稿翻页上限"},
		{Name: "mindmap", Type: shortcut.FlagBool, Desc: "继续创建并等待思维导图"},
		{Name: "speaker-insights", Type: shortcut.FlagBool, Desc: "继续创建并等待发言人总结"},
	},
	Constraints: []shortcut.Constraint{
		{Kind: shortcut.ConstraintExactlyOne, Flags: []string{"file", "resume-id"}},
		{Kind: shortcut.ConstraintCustom, Flags: []string{"complete-timeout", "wait-timeout", "poll-interval", "page-limit"}, Description: "上传超时、分析超时、轮询间隔和逐字稿页数上限必须大于 0"},
	},
	Tips: []string{`dws minutes +upload-and-analyze --file ./meeting.mp3 --title "项目周会"`, `dws minutes +upload-and-analyze --file ./meeting.mp4 --artifacts transcript,summary --mindmap`},
	Validate: func(rt *shortcut.RuntimeContext) error {
		if rt.Int("complete-timeout") <= 0 {
			return apperrors.NewValidation("--complete-timeout 必须大于 0")
		}
		return validateMinutesWorkflowWait(rt)
	},
	Execute: executeMinutesUploadAndAnalyze,
}

var Mindmap = shortcut.Shortcut{
	Service: "minutes", Command: "+mindmap", Product: "minutes",
	Description: "创建听记思维导图并轮询到明确成功、失败或超时",
	Intent:      "已有 taskUuid 且需要思维导图时使用；创建只执行一次，随后只轮询 taskStatus，失败/超时均返回非零和可继续查询的 taskUuid。",
	Risk:        shortcut.RiskWrite,
	Safety:      contract.SafetySpec{Effect: "write", Risk: "medium", Confirmation: "user_required", Idempotency: "unknown"},
	Contract: withMinutesDryRun(minutesContract("+mindmap", "创建听记思维导图并轮询到明确成功、失败或超时",
		"听记内容已就绪，需要触发并等待平台思维导图产物时使用",
		[]string{"只查询已经触发的任务状态时使用原子 mind-graph status；短音频或无有效内容可能明确失败"},
		[]string{`dws minutes +mindmap --id <taskUuid>`, `dws minutes +mindmap --id <taskUuid> --timeout 120 --interval 3`}), contract.DryRunPreviewPlan, false),
	Flags: []shortcut.Flag{
		{Name: "id", Type: shortcut.FlagString, Desc: "听记 taskUuid", Required: true},
		{Name: "timeout", Type: shortcut.FlagInt, Default: "90", Desc: "等待秒数"},
		{Name: "interval", Type: shortcut.FlagInt, Default: "3", Desc: "轮询间隔秒数"},
		{Name: "resume", Type: shortcut.FlagBool, Desc: "只继续轮询，不重复创建任务"},
	},
	Constraints: []shortcut.Constraint{{Kind: shortcut.ConstraintCustom, Flags: []string{"timeout", "interval"}, Description: "--timeout 和 --interval 必须大于 0"}},
	Tips:        []string{`dws minutes +mindmap --id <taskUuid>`, `dws minutes +mindmap --id <taskUuid> --timeout 120 --interval 3`},
	Validate:    validateMinutesPolling,
	Execute:     executeMinutesMindmap,
}

var SpeakerInsights = shortcut.Shortcut{
	Service: "minutes", Command: "+speaker-insights", Product: "minutes",
	Description: "创建发言人段落总结并轮询结果，保留异步任务恢复句柄",
	Intent:      "需要按发言人汇总听记内容时使用；严格要求 create 返回 taskId，读取未就绪时有界重试，失败或超时返回 taskId/taskUuid。",
	Risk:        shortcut.RiskWrite,
	Safety:      contract.SafetySpec{Effect: "write", Risk: "medium", Confirmation: "user_required", Idempotency: "unknown"},
	Contract: withMinutesDryRun(minutesContract("+speaker-insights", "创建发言人段落总结并轮询结果，保留异步任务恢复句柄",
		"逐字稿已有多位发言人，需要触发并读取平台发言人段落总结时使用",
		[]string{"只改发言人昵称时使用 +speaker-replace；无有效发言内容时平台可能不生成结果"},
		[]string{`dws minutes +speaker-insights --id <taskUuid>`, `dws minutes +speaker-insights --id <taskUuid> --timeout 180`}), contract.DryRunPreviewPlan, false),
	Flags: []shortcut.Flag{
		{Name: "id", Type: shortcut.FlagString, Desc: "听记 taskUuid", Required: true},
		{Name: "timeout", Type: shortcut.FlagInt, Default: "180", Desc: "等待秒数"},
		{Name: "interval", Type: shortcut.FlagInt, Default: "3", Desc: "轮询间隔秒数"},
		{Name: "resume", Type: shortcut.FlagBool, Desc: "只继续轮询，不重复创建任务"},
		{Name: "task-id", Type: shortcut.FlagString, Desc: "先前 create 返回的异步 taskId，恢复时用于输出追踪"},
	},
	Constraints: []shortcut.Constraint{{Kind: shortcut.ConstraintCustom, Flags: []string{"timeout", "interval"}, Description: "--timeout 和 --interval 必须大于 0"}},
	Tips:        []string{`dws minutes +speaker-insights --id <taskUuid>`, `dws minutes +speaker-insights --id <taskUuid> --timeout 180`},
	Validate:    validateMinutesPolling,
	Execute:     executeMinutesSpeakerInsights,
}

var PrepareASR = shortcut.Shortcut{
	Service: "minutes", Command: "+prepare-asr", Product: "minutes",
	Description: "读取个人热词、计算差异、按需新增并可显式同步删除后读回验证",
	Intent:      "录音/上传前准备 ASR 专有词表；默认只增加缺失热词，只有 --sync 才删除未出现在目标集合中的现有热词。",
	Risk:        shortcut.RiskWrite,
	Safety:      contract.SafetySpec{Effect: "write", Risk: "medium", Confirmation: "user_required", Idempotency: "idempotent"},
	Contract: withMinutesDryRun(minutesContract("+prepare-asr", "按需新增个人热词并可显式同步删除后读回验证；dry-run 不访问远端",
		"录音或上传前要补充人名、项目名等 ASR 热词，并希望看到差异和最终读回验证时使用",
		[]string{"只查看现有热词时使用原子 hot-word list；不明确接受删除时不要使用 --sync"},
		[]string{`dws minutes +prepare-asr --words "DWS,听记"`, `dws minutes +prepare-asr --words "DWS,听记" --sync`}), contract.DryRunPreviewPlan, false),
	Flags: []shortcut.Flag{
		{Name: "words", Type: shortcut.FlagStringSlice, Desc: "目标热词，逗号分隔", Required: true},
		{Name: "sync", Type: shortcut.FlagBool, Desc: "删除目标集合之外的现有热词"},
	},
	Tips:    []string{`dws minutes +prepare-asr --words "DWS,听记"`, `dws minutes +prepare-asr --words "DWS,听记" --sync`},
	Execute: executeMinutesPrepareASR,
}

var ExportPack = shortcut.Shortcut{
	Service: "minutes", Command: "+export-pack", Product: "minutes",
	Description: "把完整听记产物写入受控目录并生成不含签名 URL 的 manifest",
	Intent:      "需要离线归档 basic/summary/keywords/transcript/todos，可选媒体文件时使用；全部必需产物验证通过后才原子发布目录。",
	Risk:        shortcut.RiskRead,
	Safety:      contract.SafetySpec{Effect: "read", Risk: "low", Confirmation: "not_required", Idempotency: "idempotent"},
	Contract: minutesContract("+export-pack", "把完整听记产物写入受控目录并生成不含签名 URL 的 manifest",
		"已知 taskUuid，需要把多个已验证产物和完整性 manifest 安全归档到工作目录时使用",
		[]string{"只下载媒体时使用 +download；目标目录已存在时本命令拒绝覆盖"},
		[]string{`dws minutes +export-pack --id <taskUuid> --output ./minutes-export`, `dws minutes +export-pack --id <taskUuid> --output ./minutes-export --include-media`}),
	Flags: []shortcut.Flag{
		{Name: "id", Type: shortcut.FlagString, Desc: "听记 taskUuid", Required: true},
		{Name: "output", Type: shortcut.FlagString, Desc: "工作目录内的新归档目录", Required: true},
		{Name: "artifacts", Type: shortcut.FlagStringSlice, Desc: "要导出的产物", Enum: minutesWorkflowArtifacts},
		{Name: "include-media", Type: shortcut.FlagBool, Desc: "同时下载音视频"},
		{Name: "page-limit", Type: shortcut.FlagInt, Default: "100", Desc: "逐字稿翻页上限"},
	},
	Constraints: []shortcut.Constraint{
		{Kind: shortcut.ConstraintCustom, Flags: []string{"output"}, Description: "输出必须是安全的工作目录相对路径"},
		{Kind: shortcut.ConstraintCustom, Flags: []string{"page-limit"}, Description: "--page-limit 必须大于 0"},
	},
	Tips: []string{`dws minutes +export-pack --id <taskUuid> --output ./minutes-export`, `dws minutes +export-pack --id <taskUuid> --output ./minutes-export --include-media`},
	Validate: func(rt *shortcut.RuntimeContext) error {
		if rt.Int("page-limit") <= 0 {
			return apperrors.NewValidation("--page-limit 必须大于 0")
		}
		return localio.ValidateOutput(rt.Str("output"))
	},
	Execute: executeMinutesExportPack,
}

var Share = shortcut.Shortcut{
	Service: "minutes", Command: "+share", Product: "minutes",
	Description: "按成员逐项授予一个或多个听记权限，输出可审计的部分写入 ledger",
	Intent:      "所有者已确认成员钉钉 UID 和权限，需批量授权时使用；逐成员调用以区分成功/失败，默认首错停止。",
	Risk:        shortcut.RiskWrite,
	Safety:      contract.SafetySpec{Effect: "write", Risk: "medium", Confirmation: "user_required", Idempotency: "unknown"},
	Contract: withMinutesDryRun(minutesContract("+share", "按成员逐项授予一个或多个听记权限，输出可审计的部分写入 ledger",
		"听记所有者已确认稳定 member UID，需要授予 view/download/edit 权限并审计每个成员结果时使用",
		[]string{"当前用户自己申请权限时使用 +apply-permission；只有姓名而无稳定 UID 时先用 contact 命令消歧"},
		[]string{`dws minutes +share --ids <uuid1,uuid2> --member-uids <uid1,uid2> --permission view`, `dws minutes +share --id <uuid> --member-uids <uid> --permission edit --cover`}), contract.DryRunPreviewPlan, false),
	Flags: minutesShareFlags(true),
	Constraints: []shortcut.Constraint{
		{Kind: shortcut.ConstraintExactlyOne, Flags: []string{"id", "ids"}},
		{Kind: shortcut.ConstraintCustom, Flags: []string{"id", "ids"}, Description: "听记去重后必须为 1..50 个"},
		{Kind: shortcut.ConstraintCustom, Flags: []string{"member-uids"}, Description: "成员 UID 去重后必须为 1..50 个"},
	},
	Tips:     []string{`dws minutes +share --ids <uuid1,uuid2> --member-uids <uid1,uid2> --permission view`, `dws minutes +share --id <uuid> --member-uids <uid> --permission edit --cover`},
	Validate: validateMinutesShare,
	Execute:  executeMinutesShare,
}

var Unshare = shortcut.Shortcut{
	Service: "minutes", Command: "+unshare", Product: "minutes",
	Description: "按成员逐项移除一个或多个听记权限，输出可审计的部分写入 ledger",
	Intent:      "所有者明确要撤销稳定成员 UID 的听记访问时使用；默认首错停止，任何部分失败都返回非零。",
	Risk:        shortcut.RiskWrite,
	Safety:      contract.SafetySpec{Effect: "write", Risk: "medium", Confirmation: "user_required", Idempotency: "unknown"},
	Contract: withMinutesDryRun(minutesContract("+unshare", "按成员逐项移除一个或多个听记权限，输出可审计的部分写入 ledger",
		"听记所有者已确认稳定 member UID，需要撤销其对一个或多个听记的访问权限时使用",
		[]string{"要授权时使用 +share；成员或听记 ID 未确认时不要撤销"},
		[]string{`dws minutes +unshare --ids <uuid1,uuid2> --member-uids <uid1,uid2>`, `dws minutes +unshare --id <uuid> --member-uids <uid> --failure-policy continue`}), contract.DryRunPreviewPlan, false),
	Flags: minutesShareFlags(false),
	Constraints: []shortcut.Constraint{
		{Kind: shortcut.ConstraintExactlyOne, Flags: []string{"id", "ids"}},
		{Kind: shortcut.ConstraintCustom, Flags: []string{"id", "ids"}, Description: "听记去重后必须为 1..50 个"},
		{Kind: shortcut.ConstraintCustom, Flags: []string{"member-uids"}, Description: "成员 UID 去重后必须为 1..50 个"},
	},
	Tips:     []string{`dws minutes +unshare --ids <uuid1,uuid2> --member-uids <uid1,uid2>`, `dws minutes +unshare --id <uuid> --member-uids <uid> --failure-policy continue`},
	Validate: validateMinutesShare,
	Execute:  executeMinutesUnshare,
}

func validateMinutesWorkflowWait(rt *shortcut.RuntimeContext) error {
	if rt.Int("wait-timeout") <= 0 || rt.Int("poll-interval") <= 0 || rt.Int("page-limit") <= 0 {
		return apperrors.NewValidation("等待超时、轮询间隔和逐字稿页数上限必须大于 0")
	}
	return nil
}

func validateMinutesPolling(rt *shortcut.RuntimeContext) error {
	if rt.Int("timeout") <= 0 || rt.Int("interval") <= 0 {
		return apperrors.NewValidation("--timeout 和 --interval 必须大于 0")
	}
	return nil
}

func selectedWorkflowArtifacts(rt *shortcut.RuntimeContext) []string {
	selected := rt.StrSlice("artifacts")
	if len(selected) == 0 {
		return append([]string(nil), minutesWorkflowArtifacts...)
	}
	return uniqueStrings(selected)
}

func executeMinutesRecordWrapUp(rt *shortcut.RuntimeContext) error {
	id := rt.Str("id")
	artifacts := selectedWorkflowArtifacts(rt)
	if rt.DryRun() {
		return rt.Output(minutesDryRunPayload(contract.DryRunPreviewPlan, "minutes.record_wrap_up", map[string]any{"taskUuid": id, "stages": []string{"record.stop", "artifacts.wait"}, "artifacts": artifacts}))
	}
	stopData, err := rt.CallMCPWriteDataStrict("minutes", listeningNoteCmdTool, map[string]any{"cmd": "end", "uuid": id})
	if err != nil {
		return err
	}
	if _, err := minutesdata.RecordResult("end", id, stopData); err != nil {
		return err
	}
	bundle, failures, attempts := waitMinutesArtifacts(rt, id, artifacts, rt.Int("page-limit"), time.Duration(rt.Int("wait-timeout"))*time.Second, time.Duration(rt.Int("poll-interval"))*time.Second)
	payload := map[string]any{
		"operation": "minutes.record_wrap_up", "complete": len(failures) == 0, "taskUuid": id,
		"stages":    []map[string]any{{"name": "record.stop", "complete": true}, {"name": "artifacts.wait", "complete": len(failures) == 0, "attempts": attempts}},
		"artifacts": bundle, "failures": failures,
	}
	if len(failures) > 0 {
		payload["recovery"] = map[string]any{"taskUuid": id, "nextAction": "dws minutes +detail --id <taskUuid>"}
	}
	return outputWorkflowResult(rt, payload, len(failures) > 0, "minutes_record_wrap_up_incomplete", "artifacts")
}

func executeMinutesUploadAndAnalyze(rt *shortcut.RuntimeContext) error {
	artifacts := selectedWorkflowArtifacts(rt)
	if rt.DryRun() {
		upload := map[string]any{"operation": "minutes.upload", "resume": rt.Str("resume-id") != "", "taskUuid": rt.Str("resume-id"), "executed": false}
		if rt.Str("resume-id") == "" {
			var err error
			upload, err = performMinutesUpload(rt)
			if err != nil {
				return err
			}
		}
		return rt.Output(minutesDryRunPayload(contract.DryRunPreviewPlan, "minutes.upload_and_analyze", map[string]any{"upload": upload, "artifacts": artifacts, "mindmap": rt.Bool("mindmap"), "speakerInsights": rt.Bool("speaker-insights")}))
	}
	id := strings.TrimSpace(rt.Str("resume-id"))
	upload := map[string]any{"operation": "minutes.upload", "complete": true, "resumed": true, "taskUuid": id, "verified": true}
	if id == "" {
		var err error
		upload, err = performMinutesUpload(rt)
		if err != nil {
			return err
		}
		id, _ = upload["taskUuid"].(string)
	}
	bundle, failures, attempts := waitMinutesArtifacts(rt, id, artifacts, rt.Int("page-limit"), time.Duration(rt.Int("wait-timeout"))*time.Second, time.Duration(rt.Int("poll-interval"))*time.Second)
	stages := []map[string]any{{"name": "upload", "complete": true, "result": upload}, {"name": "artifacts.wait", "complete": len(failures) == 0, "attempts": attempts}}
	if rt.Bool("mindmap") {
		result, runErr := runMinutesMindmap(rt, id, time.Duration(rt.Int("wait-timeout"))*time.Second, time.Duration(rt.Int("poll-interval"))*time.Second, true)
		stage := map[string]any{"name": "mindmap", "complete": runErr == nil, "result": result}
		if runErr != nil {
			stage["error"] = runErr.Error()
			failures = append(failures, map[string]any{"artifact": "mindmap", "error": runErr.Error()})
		}
		stages = append(stages, stage)
	}
	if rt.Bool("speaker-insights") {
		result, runErr := runMinutesSpeakerInsights(rt, id, time.Duration(rt.Int("wait-timeout"))*time.Second, time.Duration(rt.Int("poll-interval"))*time.Second, true, "")
		stage := map[string]any{"name": "speaker-insights", "complete": runErr == nil, "result": result}
		if runErr != nil {
			stage["error"] = runErr.Error()
			failures = append(failures, map[string]any{"artifact": "speaker-insights", "error": runErr.Error()})
		}
		stages = append(stages, stage)
	}
	payload := map[string]any{"operation": "minutes.upload_and_analyze", "complete": len(failures) == 0, "taskUuid": id, "stages": stages, "artifacts": bundle, "failures": failures}
	if len(failures) > 0 {
		payload["recovery"] = map[string]any{"taskUuid": id, "nextAction": "resume analysis with +detail/+mindmap/+speaker-insights; do not upload again"}
	}
	return outputWorkflowResult(rt, payload, len(failures) > 0, "minutes_upload_analysis_incomplete", "analysis")
}

func executeMinutesMindmap(rt *shortcut.RuntimeContext) error {
	if rt.DryRun() {
		return rt.Output(minutesDryRunPayload(contract.DryRunPreviewPlan, "minutes.mindmap", map[string]any{"taskUuid": rt.Str("id"), "stages": []string{"create", "poll"}}))
	}
	payload, err := runMinutesMindmap(rt, rt.Str("id"), time.Duration(rt.Int("timeout"))*time.Second, time.Duration(rt.Int("interval"))*time.Second, !rt.Bool("resume"))
	if err := rt.Output(payload); err != nil {
		return err
	}
	return err
}

func runMinutesMindmap(rt *shortcut.RuntimeContext, id string, timeout, interval time.Duration, create bool) (map[string]any, error) {
	createAcknowledged := false
	if create {
		// create_mind_graph is observed to return either an explicit success
		// envelope or an empty acknowledgement. Empty alone is never success;
		// continue only so the status read-back can prove the final effect.
		created, err := rt.CallMCPWriteData("minutes", "create_mind_graph", map[string]any{"taskUuid": id})
		if err != nil {
			return map[string]any{"operation": "minutes.mindmap", "complete": false, "taskUuid": id, "stage": "create"}, err
		}
		createAcknowledged = minutesdata.RequireWriteAcknowledgement("mind graph create", created) == nil
	}
	deadline := time.Now().Add(timeout)
	attempts := 0
	for {
		attempts++
		data, callErr := rt.CallMCPData("minutes", "query_mind_graph_status", map[string]any{"taskUuid": id})
		if callErr != nil {
			return map[string]any{"operation": "minutes.mindmap", "complete": false, "taskUuid": id, "stage": "poll", "attempts": attempts, "recovery": map[string]any{"taskUuid": id, "nextAction": "dws minutes mind-graph status --id <taskUuid>"}}, callErr
		}
		status, result, parseErr := minutesdata.MindGraphStatus(data)
		if parseErr != nil {
			return map[string]any{"operation": "minutes.mindmap", "complete": false, "taskUuid": id, "stage": "poll", "attempts": attempts}, parseErr
		}
		if status == 1 {
			return map[string]any{"operation": "minutes.mindmap", "complete": true, "taskUuid": id, "taskStatus": status, "attempts": attempts, "createAttempted": create, "createAcknowledged": createAcknowledged, "verified": true, "result": result}, nil
		}
		if status == 2 {
			payload := map[string]any{"operation": "minutes.mindmap", "complete": false, "taskUuid": id, "taskStatus": status, "attempts": attempts, "result": result, "recovery": map[string]any{"taskUuid": id, "nextAction": "inspect source transcript; do not assume an empty mind map"}}
			return payload, minutesCompositeError("minutes_mindmap_failed", "poll", payload)
		}
		if minutesPollDeadlineReached(deadline, interval) {
			payload := map[string]any{"operation": "minutes.mindmap", "complete": false, "taskUuid": id, "taskStatus": status, "attempts": attempts, "recovery": map[string]any{"taskUuid": id, "nextAction": "dws minutes mind-graph status --id <taskUuid>"}}
			return payload, minutesCompositeError("minutes_mindmap_timeout", "poll", payload)
		}
		if err := waitMinutesInterval(rt, interval); err != nil {
			return map[string]any{"operation": "minutes.mindmap", "complete": false, "taskUuid": id, "attempts": attempts}, err
		}
	}
}

func executeMinutesSpeakerInsights(rt *shortcut.RuntimeContext) error {
	if rt.DryRun() {
		return rt.Output(minutesDryRunPayload(contract.DryRunPreviewPlan, "minutes.speaker_insights", map[string]any{"taskUuid": rt.Str("id"), "stages": []string{"create", "poll"}}))
	}
	payload, err := runMinutesSpeakerInsights(rt, rt.Str("id"), time.Duration(rt.Int("timeout"))*time.Second, time.Duration(rt.Int("interval"))*time.Second, !rt.Bool("resume"), rt.Str("task-id"))
	if outputErr := rt.Output(payload); outputErr != nil {
		return outputErr
	}
	return err
}

func runMinutesSpeakerInsights(rt *shortcut.RuntimeContext, id string, timeout, interval time.Duration, create bool, taskID string) (map[string]any, error) {
	status := "resume"
	if create {
		created, err := rt.CallMCPWriteDataStrict("minutes", "create_speaker_summary", map[string]any{"uuids": []string{id}})
		if err != nil {
			return map[string]any{"operation": "minutes.speaker_insights", "complete": false, "taskUuid": id, "stage": "create"}, err
		}
		var parseErr error
		taskID, status, parseErr = minutesdata.SpeakerSummaryTask(created)
		if parseErr != nil {
			return map[string]any{"operation": "minutes.speaker_insights", "complete": false, "taskUuid": id, "stage": "create"}, parseErr
		}
	}
	deadline := time.Now().Add(timeout)
	attempts := 0
	for {
		attempts++
		data, callErr := rt.CallMCPData("minutes", "get_speaker_summary", map[string]any{"uuids": []string{id}})
		if callErr == nil {
			result, resultErr := minutesdata.SpeakerSummaryResult(data)
			if resultErr == nil {
				return map[string]any{"operation": "minutes.speaker_insights", "complete": true, "taskUuid": id, "taskId": taskID, "createStatus": status, "attempts": attempts, "result": result}, nil
			}
			callErr = resultErr
		}
		if !speakerSummaryPending(callErr) {
			payload := map[string]any{"operation": "minutes.speaker_insights", "complete": false, "taskUuid": id, "taskId": taskID, "attempts": attempts, "stage": "poll", "recovery": map[string]any{"taskUuid": id, "taskId": taskID, "nextAction": "dws minutes speaker summary get --ids <taskUuid>"}}
			return payload, callErr
		}
		if minutesPollDeadlineReached(deadline, interval) {
			payload := map[string]any{"operation": "minutes.speaker_insights", "complete": false, "taskUuid": id, "taskId": taskID, "attempts": attempts, "stage": "poll", "recovery": map[string]any{"taskUuid": id, "taskId": taskID, "nextAction": "dws minutes speaker summary get --ids <taskUuid>"}}
			return payload, minutesCompositeError("minutes_speaker_insights_timeout", "poll", payload)
		}
		if err := waitMinutesInterval(rt, interval); err != nil {
			return map[string]any{"operation": "minutes.speaker_insights", "complete": false, "taskUuid": id, "taskId": taskID, "attempts": attempts}, err
		}
	}
}

func speakerSummaryPending(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "query empty") || strings.Contains(message, "processing") || strings.Contains(message, "not ready") || strings.Contains(message, "result is empty") || strings.Contains(message, "business error: code 000") || strings.Contains(message, "暂无")
}

func executeMinutesPrepareASR(rt *shortcut.RuntimeContext) error {
	desired := uniqueStrings(rt.StrSlice("words"))
	if rt.DryRun() {
		return rt.Output(minutesDryRunPayload(contract.DryRunPreviewPlan, "minutes.prepare_asr", map[string]any{
			"desired": desired, "sync": rt.Bool("sync"), "remotePreconditions": []string{"read current hot words", "compute additions and deletions", "apply writes", "read back exact set"},
		}))
	}
	currentData, err := rt.CallMCPData("minutes", "list_my_hotwords", map[string]any{})
	if err != nil {
		return err
	}
	current, err := minutesdata.HotWords(currentData)
	if err != nil {
		return err
	}
	toAdd := stringDifference(desired, current)
	toDelete := []string{}
	if rt.Bool("sync") {
		toDelete = stringDifference(current, desired)
	}
	plan := map[string]any{"operation": "minutes.prepare_asr", "before": current, "desired": desired, "add": toAdd, "delete": toDelete, "sync": rt.Bool("sync")}
	stages := []map[string]any{}
	if len(toAdd) > 0 {
		data, writeErr := rt.CallMCPWriteDataStrict("minutes", "add_personal_hot_word", map[string]any{"hotWordList": toAdd})
		if writeErr == nil {
			writeErr = minutesdata.RequireWriteAcknowledgement("hot-word add", data)
		}
		stages = append(stages, map[string]any{"name": "add", "complete": writeErr == nil, "words": toAdd})
		if writeErr != nil {
			plan["complete"], plan["stages"], plan["recovery"] = false, stages, map[string]any{"nextAction": "review current hot words before retrying"}
			return outputWorkflowResult(rt, plan, true, "minutes_prepare_asr_add_failed", "add")
		}
	}
	if len(toDelete) > 0 {
		data, writeErr := rt.CallMCPWriteDataStrict("minutes", "delete_personal_hotword", map[string]any{"hotWordList": toDelete})
		if writeErr == nil {
			writeErr = minutesdata.RequireWriteAcknowledgement("hot-word delete", data)
		}
		stages = append(stages, map[string]any{"name": "delete", "complete": writeErr == nil, "words": toDelete})
		if writeErr != nil {
			plan["complete"], plan["stages"], plan["recovery"] = false, stages, map[string]any{"nextAction": "list hot words; additions may already be applied"}
			return outputWorkflowResult(rt, plan, true, "minutes_prepare_asr_delete_failed", "delete")
		}
	}
	verifiedData, err := rt.CallMCPData("minutes", "list_my_hotwords", map[string]any{})
	if err != nil {
		plan["complete"], plan["stages"] = false, stages
		if outputErr := rt.Output(plan); outputErr != nil {
			return outputErr
		}
		return err
	}
	verified, err := minutesdata.HotWords(verifiedData)
	if err != nil {
		return err
	}
	missing := stringDifference(desired, verified)
	unexpected := []string{}
	if rt.Bool("sync") {
		unexpected = stringDifference(verified, desired)
	}
	plan["complete"], plan["verified"], plan["after"], plan["stages"] = len(missing) == 0 && len(unexpected) == 0, len(missing) == 0 && len(unexpected) == 0, verified, stages
	if len(missing) > 0 || len(unexpected) > 0 {
		plan["missing"], plan["unexpected"] = missing, unexpected
		return outputWorkflowResult(rt, plan, true, "minutes_prepare_asr_verification_failed", "verify")
	}
	return rt.Output(plan)
}

func executeMinutesExportPack(rt *shortcut.RuntimeContext) error {
	id := rt.Str("id")
	artifacts := selectedWorkflowArtifacts(rt)
	bundle, failures := collectMinutesArtifactsOnce(rt, id, artifacts, rt.Int("page-limit"))
	if len(failures) > 0 {
		payload := map[string]any{"operation": "minutes.export_pack", "complete": false, "taskUuid": id, "failures": failures, "published": false}
		return outputWorkflowResult(rt, payload, true, "minutes_export_pack_incomplete", "collect")
	}
	target, relative, parent, err := prepareExportTarget(rt.Str("output"))
	if err != nil {
		return err
	}
	tempDir, err := minutesMkdirTemp(parent, ".dws-minutes-export-*")
	if err != nil {
		return err
	}
	cleanup := true
	defer func() {
		if cleanup {
			_ = minutesRemoveAll(tempDir)
		}
	}()
	files := map[string]map[string]any{}
	for _, name := range artifacts {
		filename := name + ".json"
		value := bundle[name]
		if name == "summary" {
			filename = "summary.md"
			if err := minutesWriteFile(filepath.Join(tempDir, filename), []byte(value.(string)), 0o600); err != nil {
				return err
			}
		} else if err := writeJSONFile(filepath.Join(tempDir, filename), value); err != nil {
			return err
		}
		info, err := minutesStat(filepath.Join(tempDir, filename))
		if err != nil {
			return err
		}
		files[name] = map[string]any{"file": filename, "sizeBytes": info.Size(), "complete": true}
	}
	if rt.Bool("include-media") {
		mediaData, callErr := rt.CallMCPData("minutes", "query_minutes_audio_url", map[string]any{"taskUuid": id})
		if callErr != nil {
			return callErr
		}
		mediaURL, parseErr := minutesdata.MediaURL(mediaData)
		if parseErr != nil {
			return parseErr
		}
		download, downloadErr := minutesDownload(rt.Command().Context(), mediaURL, localio.DownloadOptions{BaseDir: tempDir, Output: "media/", PreferredName: id + mediaExtension(mediaURL)})
		if downloadErr != nil {
			return downloadErr
		}
		files["media"] = map[string]any{"file": download.RelativePath, "sizeBytes": download.SizeBytes, "complete": true}
	}
	manifest := map[string]any{"version": 1, "operation": "minutes.export_pack", "taskUuid": id, "complete": true, "generatedAt": time.Now().UTC().Format(time.RFC3339), "files": files}
	if err := writeJSONFile(filepath.Join(tempDir, "manifest.json"), manifest); err != nil {
		return err
	}
	if err := minutesRename(tempDir, target); err != nil {
		return fmt.Errorf("发布听记归档目录失败: %w", err)
	}
	cleanup = false
	return rt.Output(map[string]any{"operation": "minutes.export_pack", "complete": true, "taskUuid": id, "published": true, "path": filepath.ToSlash(relative), "manifest": filepath.ToSlash(filepath.Join(relative, "manifest.json")), "files": files})
}

func minutesShareFlags(includePermission bool) []shortcut.Flag {
	flags := []shortcut.Flag{
		{Name: "id", Type: shortcut.FlagString, Desc: "单个听记 taskUuid"},
		{Name: "ids", Type: shortcut.FlagStringSlice, Desc: "多个听记 taskUuid，最多 50 个"},
		{Name: "member-uids", Type: shortcut.FlagStringSlice, Desc: "稳定成员钉钉 UID，最多 50 个", Required: true},
	}
	if includePermission {
		flags = append(flags,
			shortcut.Flag{Name: "permission", Type: shortcut.FlagString, Desc: "授予权限", Required: true, Enum: []string{"view", "download", "edit"}},
			shortcut.Flag{Name: "cover", Type: shortcut.FlagBool, Desc: "覆盖已有权限"},
			shortcut.Flag{Name: "sub-resources", Type: shortcut.FlagStringSlice, Desc: "可选子资源", Enum: []string{"OrigContent", "Summary", "Analysis", "Note"}},
		)
	}
	return append(flags, shortcut.Flag{Name: "failure-policy", Type: shortcut.FlagString, Default: "stop", Desc: "成员失败策略", Enum: []string{"stop", "continue"}})
}

func validateMinutesShare(rt *shortcut.RuntimeContext) error {
	ids, members := minutesIDs(rt), uniqueStrings(rt.StrSlice("member-uids"))
	if len(ids) == 0 || len(ids) > 50 || len(members) == 0 || len(members) > 50 {
		return apperrors.NewValidation("听记和成员 UID 数量必须各为 1..50")
	}
	return nil
}

func executeMinutesShare(rt *shortcut.RuntimeContext) error {
	policy := map[string]float64{"edit": 2, "download": 3, "view": 4}[rt.Str("permission")]
	return executeMinutesPermissionLedger(rt, "share", "add_member_permission", func(member string) map[string]any {
		params := map[string]any{"uuids": minutesIDs(rt), "memberUids": []string{member}, "policyId": policy}
		if rt.Changed("cover") {
			params["coverPermission"] = fmt.Sprintf("%t", rt.Bool("cover"))
		}
		if values := rt.StrSlice("sub-resources"); len(values) > 0 {
			params["roleSubResourceIds"] = values
		}
		return params
	})
}

func executeMinutesUnshare(rt *shortcut.RuntimeContext) error {
	if !rt.DryRun() {
		for _, id := range minutesIDs(rt) {
			data, err := rt.CallMCPData("minutes", "get_minutes_basic_info", map[string]any{"taskUuid": id})
			if err != nil {
				return fmt.Errorf("minutes unshare preflight for %s: %w", id, err)
			}
			if _, err := minutesdata.Basic(id, data); err != nil {
				return fmt.Errorf("minutes unshare preflight for %s: %w", id, err)
			}
		}
	}
	return executeMinutesPermissionLedger(rt, "unshare", "remove_member_permission", func(member string) map[string]any {
		return map[string]any{"uuids": minutesIDs(rt), "memberUids": []string{member}}
	})
}

func executeMinutesPermissionLedger(rt *shortcut.RuntimeContext, operation, tool string, params func(string) map[string]any) error {
	members := uniqueStrings(rt.StrSlice("member-uids"))
	plan := map[string]any{"operation": "minutes." + operation, "taskUuids": minutesIDs(rt), "memberCount": len(members), "members": members}
	if rt.DryRun() {
		return rt.Output(minutesDryRunPayload(contract.DryRunPreviewPlan, "minutes."+operation, plan))
	}
	results := []map[string]any{}
	failures := []map[string]any{}
	unattempted := []string{}
	for index, member := range members {
		data, err := rt.CallMCPWriteDataStrict("minutes", tool, params(member))
		if err == nil {
			if operation == "unshare" {
				err = minutesdata.RequirePermissionMutationAcknowledgement(operation, minutesIDs(rt), []string{member}, data)
			} else {
				err = minutesdata.RequireWriteAcknowledgement(operation, data)
			}
		}
		if err != nil {
			failures = append(failures, map[string]any{"memberUid": member, "error": err.Error()})
			if rt.Str("failure-policy") == "stop" {
				unattempted = append(unattempted, members[index+1:]...)
				break
			}
			continue
		}
		results = append(results, map[string]any{"memberUid": member, "complete": true})
	}
	plan["complete"], plan["succeeded"], plan["failed"], plan["unattempted"], plan["results"], plan["failures"] = len(failures) == 0, len(results), len(failures), unattempted, results, failures
	return outputWorkflowResult(rt, plan, len(failures) > 0, "minutes_permission_partial", operation)
}

func collectMinutesArtifactsOnce(rt *shortcut.RuntimeContext, id string, artifacts []string, pageLimit int) (map[string]any, []map[string]any) {
	bundle := map[string]any{}
	failures := []map[string]any{}
	for _, artifact := range artifacts {
		var value any
		var err error
		switch artifact {
		case "basic":
			var data map[string]any
			data, err = rt.CallMCPData("minutes", "get_minutes_basic_info", map[string]any{"taskUuid": id})
			if err == nil {
				value, err = minutesdata.Basic(id, data)
			}
		case "summary":
			var data map[string]any
			data, err = rt.CallMCPData("minutes", "get_minutes_ai_summary", map[string]any{"taskUuid": id})
			if err == nil {
				value, err = minutesdata.SummaryText(data)
				if err == nil && strings.TrimSpace(value.(string)) == "" {
					err = fmt.Errorf("minutes summary is explicitly empty; analysis readiness is not proven")
				}
			}
		case "keywords":
			var data map[string]any
			data, err = rt.CallMCPData("minutes", "get_minutes_keywords", map[string]any{"taskUuid": id})
			if err == nil {
				err = minutesdata.ValidateArtifact("keywords", id, data)
				value = data["result"]
			}
		case "transcript":
			var transcript minutesdata.TranscriptResult
			transcript, err = collectTranscriptForMinutes(rt, id, pageLimit)
			value = minutesdata.TranscriptPayload(id, "0", transcript)
			if err == nil && len(transcript.Paragraphs) == 0 {
				err = fmt.Errorf("minutes transcript is explicitly empty; analysis readiness is not proven")
			}
		case "todos":
			var data map[string]any
			data, err = rt.CallMCPData("minutes", "list_minutes_todos", map[string]any{"taskUuid": id})
			if err == nil {
				err = minutesdata.ValidateArtifact("todos", id, data)
				value = data["result"]
			}
		default:
			err = fmt.Errorf("unsupported artifact %q", artifact)
		}
		if err != nil {
			failures = append(failures, map[string]any{"artifact": artifact, "error": err.Error()})
			continue
		}
		bundle[artifact] = value
	}
	return bundle, failures
}

func waitMinutesArtifacts(rt *shortcut.RuntimeContext, id string, artifacts []string, pageLimit int, timeout, interval time.Duration) (map[string]any, []map[string]any, int) {
	deadline := time.Now().Add(timeout)
	attempts := 0
	for {
		attempts++
		bundle, failures := collectMinutesArtifactsOnce(rt, id, artifacts, pageLimit)
		if len(failures) == 0 || minutesPollDeadlineReached(deadline, interval) {
			return bundle, failures, attempts
		}
		if err := waitMinutesInterval(rt, interval); err != nil {
			return bundle, append(failures, map[string]any{"artifact": "wait", "error": err.Error()}), attempts
		}
	}
}

func waitMinutesInterval(rt *shortcut.RuntimeContext, interval time.Duration) error {
	timer := time.NewTimer(interval)
	defer timer.Stop()
	commandContext := rt.Command().Context()
	if commandContext == nil {
		commandContext = context.Background()
	}
	select {
	case <-commandContext.Done():
		return commandContext.Err()
	case <-timer.C:
		return nil
	}
}

func minutesPollDeadlineReached(deadline time.Time, interval time.Duration) bool {
	return !time.Now().Add(interval).Before(deadline)
}

func outputWorkflowResult(rt *shortcut.RuntimeContext, payload map[string]any, failed bool, reason, stage string) error {
	if err := rt.Output(payload); err != nil {
		return err
	}
	if failed {
		return minutesCompositeError(reason, stage, payload)
	}
	return nil
}

func uniqueStrings(values []string) []string {
	seen := map[string]bool{}
	out := []string{}
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

func stringDifference(left, right []string) []string {
	other := map[string]bool{}
	for _, value := range right {
		other[value] = true
	}
	out := []string{}
	for _, value := range left {
		if !other[value] {
			out = append(out, value)
		}
	}
	sort.Strings(out)
	return out
}

func prepareExportTarget(output string) (target, relative, parent string, err error) {
	if err := localio.ValidateOutput(output); err != nil {
		return "", "", "", err
	}
	cwd, err := minutesGetwd()
	if err != nil {
		return "", "", "", err
	}
	realBase, err := minutesEvalSymlinks(cwd)
	if err != nil {
		return "", "", "", err
	}
	relative = filepath.Clean(output)
	target = filepath.Join(realBase, relative)
	parent = filepath.Dir(target)
	rel, err := minutesRel(realBase, target)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", "", "", fmt.Errorf("LOCAL_PATH_UNSAFE: 输出目录逃逸工作目录")
	}
	if err := ensureExportParent(realBase, filepath.Dir(rel)); err != nil {
		return "", "", "", err
	}
	if _, err := minutesLstat(target); err == nil {
		return "", "", "", fmt.Errorf("LOCAL_FILE_EXISTS: 目标目录已存在；请选择新的输出路径")
	} else if !os.IsNotExist(err) {
		return "", "", "", err
	}
	return target, relative, parent, nil
}

func ensureExportParent(base, relative string) error {
	current := base
	if relative == "." || relative == "" {
		return nil
	}
	for _, component := range strings.Split(filepath.Clean(relative), string(filepath.Separator)) {
		current = filepath.Join(current, component)
		info, err := minutesLstat(current)
		if os.IsNotExist(err) {
			if err := minutesMkdir(current, 0o700); err != nil {
				return err
			}
			continue
		}
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
			return fmt.Errorf("LOCAL_PATH_UNSAFE: 输出父路径不是安全目录")
		}
	}
	return nil
}

func writeJSONFile(path string, value any) error {
	raw, err := minutesMarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	raw = append(raw, '\n')
	return minutesWriteFile(path, raw, 0o600)
}

func init() {
	shortcut.Register(finalizeMinutesShortcuts(RecordWrapUp, UploadAndAnalyze, Mindmap, SpeakerInsights, PrepareASR, ExportPack, Share, Unshare)...)
}
