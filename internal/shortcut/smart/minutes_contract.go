// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package smart

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut/minutesdata"
)

func minutesTranscriptResult() *contract.ResultSpec {
	return &contract.ResultSpec{
		Outcomes: []contract.ResultOutcome{
			contract.ResultOutcomeSuccess,
			contract.ResultOutcomeFailure,
		},
		DataSchema: json.RawMessage(`{"type":"object","description":"带分页完整性证据的听记逐字稿","properties":{"taskUuid":{"type":"string","description":"听记稳定 taskUuid"},"direction":{"type":"string","description":"逐字稿段落排序方向"},"complete":{"type":"boolean","description":"是否已证明逐字稿分页读取完整"},"pages":{"type":"integer","description":"本次实际读取的分页数量"},"paragraphCount":{"type":"integer","description":"跨页去重后的逐字稿段落数量"},"duplicateCount":{"type":"integer","description":"跨页读取时移除的重复段落数量"},"paragraphList":{"type":"array","description":"跨页去重后的逐字稿段落","items":{"type":"object","description":"一条逐字稿段落","additionalProperties":true}}},"required":["taskUuid","direction","complete","pages","paragraphCount","duplicateCount","paragraphList"],"additionalProperties":true}`),
	}
}

func minutesDetailResult() *contract.ResultSpec {
	return &contract.ResultSpec{
		Outcomes: []contract.ResultOutcome{
			contract.ResultOutcomeSuccess,
			contract.ResultOutcomePartialFailure,
			contract.ResultOutcomeFailure,
		},
		DataSchema: json.RawMessage(`{"type":"object","description":"一条或多条听记的制品聚合结果","properties":{"taskUuid":{"type":"string","description":"单条模式的听记稳定 taskUuid"},"complete":{"type":"boolean","description":"所有请求制品是否均已完整读取"},"failureCount":{"type":"integer","description":"单条模式中读取失败的制品数量"},"basic":{"type":"object","description":"听记基础信息","additionalProperties":true},"summary":{"type":"object","description":"听记 AI 摘要","additionalProperties":true},"keywords":{"type":"object","description":"听记关键词","additionalProperties":true},"transcript":{"type":"object","description":"带 complete/pages/nextToken 的逐字稿结果","additionalProperties":true},"todos":{"type":"object","description":"听记行动项结果","additionalProperties":true},"operation":{"type":"string","description":"批量模式的稳定操作名称"},"requested":{"type":"integer","description":"批量模式请求的听记数量"},"succeeded":{"type":"integer","description":"批量模式完整成功的听记数量"},"failed":{"type":"integer","description":"批量模式存在制品失败的听记数量"},"results":{"type":"array","description":"批量模式的逐条聚合结果","items":{"type":"object","description":"一条听记的制品聚合结果","additionalProperties":true}},"failures":{"type":"array","description":"批量模式的逐项失败台账","items":{"type":"object","description":"包含 taskUuid、artifact 和错误原因的失败记录","additionalProperties":true}}},"required":["complete"],"additionalProperties":true}`),
	}
}

func minutesTranscriptPagination() *contract.PaginationSpec {
	return &contract.PaginationSpec{
		Kind:                  contract.PaginationKindCursor,
		CursorParameter:       "cursor",
		MetaPath:              contract.PaginationMetaPath,
		EndpointExhaustedPath: contract.PaginationExhaustedPath,
		NextTokenPath:         contract.PaginationNextTokenPath,
	}
}

func withMinutesTranscriptResult(decl corecmd.ContractDecl) corecmd.ContractDecl {
	decl.Result = minutesTranscriptResult()
	decl.Pagination = minutesTranscriptPagination()
	return decl
}

func outputMinutesTranscriptResult(rt *shortcut.RuntimeContext, payload map[string]any, result minutesdata.TranscriptResult, readErr error) error {
	if !output.UsesUnifiedResult(rt.Command()) {
		if err := rt.Output(payload); err != nil {
			return err
		}
		if readErr != nil {
			taskUUID, _ := payload["taskUuid"].(string)
			return minutesTranscriptReadError(taskUUID, result, readErr)
		}
		return nil
	}

	business := make(map[string]any, len(payload))
	for key, value := range payload {
		if key == "nextToken" {
			continue
		}
		business[key] = value
	}
	meta := &output.Meta{Count: output.NewCount(len(result.Paragraphs))}
	pagination, paginationErr := output.NewPagination(result.Complete, result.NextToken)
	if paginationErr == nil {
		pagination.Pages = result.Pages
		pagination.Items = len(result.Paragraphs)
		meta.Pagination = pagination
	}
	if readErr != nil {
		taskUUID, _ := payload["taskUuid"].(string)
		details := map[string]any{
			"taskUuid":       taskUUID,
			"pages":          result.Pages,
			"paragraphCount": len(result.Paragraphs),
			"cause":          readErr.Error(),
		}
		if result.NextToken != "" {
			details["nextToken"] = result.NextToken
		}
		options := []output.ResultOption{output.WithMeta(meta)}
		if paginationErr != nil {
			options = nil
		}
		started := true
		return output.StoreResult(rt.Command().Context(), output.Failure(&output.ErrorInfo{
			Type:             "api",
			Subtype:          "minutes_transcript_incomplete",
			Message:          fmt.Sprintf("逐字稿读取不完整：已读取 %d 页、%d 个段落", result.Pages, len(result.Paragraphs)),
			Hint:             "使用 meta.pagination.next_token 继续读取；若该字段缺失，请从当前听记首页重试。",
			Operation:        "minutes/get_minutes_transcription",
			Origin:           "mcp",
			Stage:            "pagination",
			ExecutionStarted: &started,
			Details:          details,
			TechnicalDetail:  readErr.Error(),
		}, options...))
	}
	if paginationErr != nil {
		return fmt.Errorf("minutes transcript pagination result is invalid: %w", paginationErr)
	}
	return output.StoreResult(rt.Command().Context(), output.Success(business, output.WithMeta(meta)))
}

func minutesSmartContract(command, description, useWhen string, avoidWhen, examples, aliases []string) corecmd.ContractDecl {
	name := "shortcut_" + strings.ReplaceAll(strings.TrimPrefix(command, "+"), "-", "_")
	identityAliases := make([]string, 0, len(aliases))
	for _, alias := range aliases {
		identityAliases = append(identityAliases, "minutes "+alias)
	}
	return corecmd.ContractDecl{
		Identity: contract.ToolIdentitySpec{
			ProductID:      "minutes",
			Name:           name,
			CanonicalPath:  "minutes." + name,
			CLIPath:        "minutes " + command,
			PrimaryCLIPath: "minutes " + command,
			Aliases:        identityAliases,
		},
		Description: description,
		Interface: &contract.InterfaceSpec{
			Mode:         contract.InterfaceModeComposite,
			Availability: contract.InterfaceAvailable,
			Reason:       "The executable Shortcut owns strict Minutes response validation, orchestration, completeness and final output; no single RPC represents the final command contract.",
		},
		Selection: contract.SelectionSpec{
			AgentSummary: description,
			UseWhen:      []string{useWhen},
			AvoidWhen:    avoidWhen,
			Examples:     examples,
		},
	}
}

// finalizeMinutesSmartShortcut keeps Minutes shortcuts that live in the smart
// package under the same declare=delivery contract as the Minutes package.
func finalizeMinutesSmartShortcut(value shortcut.Shortcut) shortcut.Shortcut {
	value.Contract.Selection.AgentSummary = value.Description
	value.Contract.Selection.UseWhen = []string{value.Intent}
	for _, constraint := range value.Constraints {
		if constraint.Kind != shortcut.ConstraintCustom {
			continue
		}
		evidence := strings.TrimSpace(constraint.Description)
		if evidence == "" {
			continue
		}
		for flagIndex := range value.Flags {
			flag := &value.Flags[flagIndex]
			if !smartMinutesContains(constraint.Flags, flag.Name) || strings.Contains(flag.Desc, evidence) {
				continue
			}
			flag.Desc = strings.TrimRight(flag.Desc, "；。 ") + "；约束：" + evidence
		}
		for parameterIndex := range value.Contract.Parameters {
			parameter := &value.Contract.Parameters[parameterIndex]
			if !smartMinutesContains(constraint.Flags, parameter.Name) || strings.Contains(parameter.Description, evidence) {
				continue
			}
			parameter.Description = strings.TrimRight(parameter.Description, "；。 ") + "；约束：" + evidence
		}
	}
	return value
}

func smartMinutesContains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
