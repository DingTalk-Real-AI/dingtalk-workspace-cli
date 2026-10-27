// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0.

package helpers

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	authpkg "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/auth"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/jsonutil"
	mindcore "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/mind"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/profilemetadata"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/config"
	"github.com/spf13/cobra"
)

func callMindResult(cmd *cobra.Command, tool string, args map[string]any) (output.CommandResult, error) {
	if deps.Caller.DryRun() {
		preview := map[string]any{"executed": false, "tool": tool}
		for _, key := range []string{"requestId", "nodeId", "pageId", "topicId", "parentTopicId", "beforeTopicId", "name", "folderId", "workspaceId", "expectedVersion", "mode", "depth", "maxResults", "deleteMode"} {
			if value, ok := args[key]; ok {
				preview[key] = value
			}
		}
		if source, ok := args["source"].(string); ok {
			parsed, err := mindcore.ParseSource([]byte(source))
			if err != nil {
				return nil, err
			}
			count := 0
			for _, p := range parsed.Pages {
				count += len(p.Nodes)
			}
			preview["sourceBytes"] = len(source)
			preview["pageCount"] = len(parsed.Pages)
			preview["topicCount"] = count
		}
		return output.Success(preview, output.WithDryRun()), nil
	}
	wireArgs, err := mindWireArgs(args)
	if err != nil {
		return nil, err
	}
	text, err := CallMCPToolTextOnServerContext(cmd.Context(), "mind", tool, wireArgs)
	if err != nil {
		// The shared parser retains raw business JSON in CLIError.Message. Keep
		// gateway/auth/network classification, but expose business diagnostics as
		// structured receipt data instead of reducing them to a generic exception.
		var business *CLIError
		if errors.As(err, &business) && business.Code == CodeMCPToolError {
			if receipt, decodeErr := decodeMindReply(business.Message); decodeErr == nil {
				return mindFailure(tool, receipt, "upstream_error", "MCP returned a business error"), nil
			}
		}
		return nil, WrapErrorWithOperation(err, "mind/"+tool)
	}
	receipt, err := decodeMindReply(text)
	if err != nil {
		return mindFailure(tool, nil, "invalid_response", err.Error()), nil
	}
	if success, exists := receipt["success"]; exists && success != true {
		return mindFailure(tool, receipt, "upstream_error", "success must be true"), nil
	}
	if tool == "list_mind_nodes" {
		return mindQueryResult(tool, args, receipt), nil
	}
	return mindBindRecoveryProfile(cmd, mindWriteResult(tool, args, receipt))
}

// Accept only the two documented projections: the flattened MCP result, or a
// ServiceResult envelope containing result. Never recursively unwrap arbitrary
// data/content fields; doing so can hide conflicting failure indicators.
func decodeMindReply(text string) (map[string]any, error) {
	if err := jsonutil.RejectDuplicateObjectKeys([]byte(text)); err != nil {
		return nil, fmt.Errorf("invalid response JSON: %w", err)
	}
	decoder := json.NewDecoder(strings.NewReader(text))
	decoder.UseNumber()
	var value map[string]any
	if err := decoder.Decode(&value); err != nil {
		return nil, err
	}
	if value == nil {
		return nil, fmt.Errorf("response must be an object")
	}
	if raw, wrapped := value["result"]; wrapped {
		success, ok := value["success"].(bool)
		if !ok {
			return nil, fmt.Errorf("ServiceResult requires boolean success")
		}
		if !success {
			// Top-level failure code/message remain authoritative; the result
			// DTO carries stage/category/logId even on success=false.
			failure := make(map[string]any)
			if inner, ok := raw.(map[string]any); ok {
				for _, key := range []string{"errorStage", "errorCategory", "errorOperationId", "logId", "requestId"} {
					if v, exists := inner[key]; exists {
						failure[key] = v
					}
				}
			}
			for k, v := range value {
				if k != "result" {
					failure[k] = v
				}
			}
			return mindDecodeFlatResult(failure)
		}
		if _, exists := value["status"]; exists {
			return nil, fmt.Errorf("conflicting flat and wrapped response")
		}
		inner, ok := raw.(map[string]any)
		if !ok || len(inner) == 0 {
			return nil, fmt.Errorf("ServiceResult result must be a nonempty object")
		}
		if s, exists := inner["success"]; exists && s != true {
			return nil, fmt.Errorf("conflicting success indicators")
		}
		if trace, ok := value["traceId"]; ok {
			if _, exists := inner["traceId"]; !exists {
				inner["traceId"] = trace
			}
		}
		return mindDecodeFlatResult(inner)
	}
	if len(value) == 0 {
		return nil, fmt.Errorf("empty response cannot establish an operation result")
	}
	return mindDecodeFlatResult(value)
}

func mindFailure(tool string, receipt map[string]any, subtype, reason string) output.CommandResult {
	info := &output.ErrorInfo{Type: "api", Subtype: subtype, Message: reason, Operation: "mind/" + tool, ServerKey: "mind", Origin: "upstream", Hint: "保留原 request-id 和全部参数排查；结果未知时不要换 request-id 重新写入"}
	if receipt != nil {
		info.Details = map[string]any{"receipt": receipt}
		info.RequestID, _ = receipt["requestId"].(string)
		info.TraceID, _ = receipt["traceId"].(string)
		info.UpstreamCode = receipt["errorCode"]
		for _, key := range []string{"errorMessage", "errorMsg"} {
			if message, ok := receipt[key].(string); ok && message != "" {
				info.Message = reason + ": " + message
				break
			}
		}
		if detail, ok := receipt["errorDetail"].(map[string]any); ok {
			info.Stage, _ = detail["stage"].(string)
			if code, exists := detail["code"]; exists {
				info.UpstreamCode = code
			}
			if message, ok := detail["message"].(string); ok && message != "" {
				info.Message = reason + ": " + message
			}
		}
		if hint, ok := receipt["hint"].(string); ok && hint != "" {
			info.Hint = hint
		}
	}
	return output.Failure(info)
}
func mindString(receipt map[string]any, key string) string {
	value, _ := receipt[key].(string)
	return value
}
func mindReplyVersion(receipt map[string]any, key string, required bool) error {
	value, exists := receipt[key]
	if !exists || value == nil {
		if required {
			return fmt.Errorf("missing %s", key)
		}
		return nil
	}
	text, ok := value.(string)
	if !ok {
		return fmt.Errorf("%s must remain a decimal string", key)
	}
	if err := mindcore.ValidateVersion(text); err != nil {
		return fmt.Errorf("%s: %w", key, err)
	}
	return nil
}
func mindMatch(args, receipt map[string]any, keys ...string) error {
	for _, key := range keys {
		expected, ok := args[key].(string)
		if !ok || expected == "" {
			continue
		}
		if mindString(receipt, key) != expected {
			return fmt.Errorf("response %s does not match request", key)
		}
	}
	return nil
}
func mindMap(value any) (map[string]string, error) {
	fields, ok := value.(map[string]any)
	if !ok || len(fields) == 0 {
		return nil, fmt.Errorf("missing or empty ID mapping")
	}
	result := map[string]string{}
	for k, v := range fields {
		text, ok := v.(string)
		if k == "" || !ok || text == "" {
			return nil, fmt.Errorf("ID mapping must contain nonempty string keys and values")
		}
		result[k] = text
	}
	return result, nil
}
func mindWriteResult(tool string, args, receipt map[string]any) output.CommandResult {
	invalid := func(err error) output.CommandResult {
		return mindFailure(tool, receipt, "invalid_response", err.Error())
	}
	if err := mindMatch(args, receipt, "requestId", "nodeId", "pageId"); err != nil {
		return invalid(err)
	}
	status := mindString(receipt, "status")
	if tool == "create_mind" {
		if status == "UNKNOWN" {
			return mindFailure(tool, receipt, "unknown_effect", "脑图创建结果未知")
		}
		if mindString(receipt, "nodeId") == "" {
			return invalid(fmt.Errorf("missing created file nodeId"))
		}
		if status == "INITIALIZING" && receipt["requestedContentApplied"] == false {
			return output.Pending(mindBusinessData(receipt), &output.OperationInfo{ID: mindString(receipt, "requestId"), State: status, NextCommand: mindResumeCommand(tool, args)})
		}
		if status != "READY" || receipt["requestedContentApplied"] != true {
			return invalid(fmt.Errorf("creation requires READY and requestedContentApplied=true"))
		}
		if err := mindReplyVersion(receipt, "revision", true); err != nil {
			return invalid(err)
		}
		source, err := mindcore.ParseSource([]byte(args["source"].(string)))
		if err != nil {
			return invalid(err)
		}
		pages, err := mindMap(receipt["pageIdMap"])
		if err != nil {
			return invalid(err)
		}
		nodes, err := mindMap(receipt["nodeIdMap"])
		if err != nil {
			return invalid(err)
		}
		for _, p := range source.Pages {
			if pages[p.ID] == "" {
				return invalid(fmt.Errorf("pageIdMap misses %q", p.ID))
			}
			for _, n := range p.Nodes {
				if nodes[n.ID] == "" {
					return invalid(fmt.Errorf("nodeIdMap misses %q", n.ID))
				}
			}
		}
		return output.Success(mindBusinessData(receipt))
	}
	if status == "UNKNOWN" || status == "CONFLICT" {
		return mindFailure(tool, receipt, strings.ToLower(status), "脑图节点操作未完成："+status)
	}
	if err := mindReplyVersion(receipt, "readVersion", true); err != nil {
		return invalid(err)
	}
	if expected, ok := args["expectedVersion"].(string); ok && expected != receipt["readVersion"] {
		return invalid(fmt.Errorf("readVersion differs from expectedVersion"))
	}
	if err := mindReplyVersion(receipt, "committedVersion", status == "APPLIED" || status == "COMMITTED_UNVERIFIED"); err != nil {
		return invalid(err)
	}
	if status == "COMMITTED_UNVERIFIED" && receipt["applied"] == false {
		return output.Pending(mindBusinessData(receipt), &output.OperationInfo{ID: mindString(receipt, "requestId"), State: status, NextCommand: mindResumeCommand(tool, args)})
	}
	if err := mindMatch(args, receipt, "topicId"); err != nil {
		return invalid(err)
	}
	if status == "NO_CHANGE" && tool == "update_mind_node" && receipt["applied"] == false {
		node, ok := receipt["node"].(map[string]any)
		if !ok || node["id"] != args["topicId"] || node["text"] != args["patch"].(map[string]any)["text"] {
			return invalid(fmt.Errorf("NO_CHANGE must confirm the requested node and text"))
		}
		return output.Success(mindBusinessData(receipt))
	}
	if status != "APPLIED" || receipt["applied"] != true {
		return invalid(fmt.Errorf("mutation requires APPLIED and applied=true"))
	}
	if err := mindValidateApplied(tool, args, receipt); err != nil {
		return invalid(err)
	}
	return output.Success(mindBusinessData(receipt))
}

func mindValidateApplied(tool string, args, receipt map[string]any) error {
	read, _ := strconvMindVersion(receipt["readVersion"])
	committed, _ := strconvMindVersion(receipt["committedVersion"])
	if committed <= read {
		return fmt.Errorf("committedVersion must exceed readVersion")
	}
	if tool == "delete_mind_node" {
		known, ok := receipt["deletedCountKnown"].(bool)
		if !ok {
			return fmt.Errorf("missing deletedCountKnown")
		}
		if known {
			n, err := strconvMindVersion(receipt["deletedCount"])
			if err != nil || n < 1 {
				return fmt.Errorf("known deletedCount must be a positive integer")
			}
		}
		return nil
	}
	node, ok := receipt["node"].(map[string]any)
	if !ok {
		return fmt.Errorf("missing confirmed node")
	}
	topic := mindString(receipt, "topicId")
	if topic == "" || mindString(node, "id") != topic {
		return fmt.Errorf("confirmed node ID differs from topicId")
	}
	var text string
	if tool == "create_mind_node" {
		requested := args["node"].(map[string]any)
		text, _ = requested["text"].(string)
		mapping, err := mindMap(receipt["nodeIdMap"])
		if err != nil {
			return err
		}
		if mapping[requested["id"].(string)] != topic {
			return fmt.Errorf("nodeIdMap does not identify created topic")
		}
		if node["parentId"] != args["parentTopicId"] {
			return fmt.Errorf("confirmed parent differs from requested parent")
		}
	} else {
		text, _ = args["patch"].(map[string]any)["text"].(string)
	}
	if value, ok := node["text"].(string); !ok || value != text {
		return fmt.Errorf("confirmed text differs from requested text")
	}
	return nil
}
func strconvMindVersion(value any) (int64, error) {
	switch v := value.(type) {
	case string:
		if err := mindcore.ValidateVersion(v); err != nil {
			return 0, err
		}
		return json.Number(v).Int64()
	case json.Number:
		return v.Int64()
	}
	return 0, fmt.Errorf("invalid version/count type")
}
func mindQueryResult(tool string, args, receipt map[string]any) output.CommandResult {
	invalid := func(reason string) output.CommandResult {
		return mindFailure(tool, receipt, "invalid_response", reason)
	}
	if err := mindMatch(args, receipt, "nodeId"); err != nil {
		return invalid(err.Error())
	}
	if err := mindReplyVersion(receipt, "readVersion", true); err != nil {
		return invalid(err.Error())
	}
	if receipt["schemaVersion"] != "1.0" || receipt["catalogVersion"] != "mind-v1" {
		return invalid("query requires mind-v1 / schema 1.0")
	}
	pages, ok := receipt["items"].([]any)
	if !ok {
		return invalid("query items must be a page array")
	}
	hasMore, ok := receipt["hasMore"].(bool)
	if !ok {
		return invalid("missing hasMore")
	}
	if _, ok := receipt["truncated"].(bool); !ok {
		return invalid("missing truncated")
	}
	token := mindString(receipt, "nextToken")
	if hasMore && token == "" {
		return invalid("hasMore=true requires nextToken")
	}
	count := 0
	for _, raw := range pages {
		page, ok := raw.(map[string]any)
		if !ok || mindString(page, "id") == "" || mindString(page, "rootId") == "" {
			return invalid("invalid page identity")
		}
		if requested, ok := args["pageId"].(string); ok && requested != "" && page["id"] != requested {
			return invalid("response page differs from requested page")
		}
		nodes, ok := page["nodes"].([]any)
		if !ok {
			return invalid("page nodes must be an array")
		}
		count += len(nodes)
		for _, raw := range nodes {
			node, ok := raw.(map[string]any)
			if !ok || mindString(node, "id") == "" || node["type"] != "topic" {
				return invalid("invalid query topic")
			}
		}
	}
	data := map[string]any{}
	for k, v := range mindBusinessData(receipt) {
		if k != "hasMore" && k != "nextToken" {
			data[k] = v
		}
	}
	return output.Success(data, output.WithMeta(&output.Meta{Count: output.NewCount(count), Pagination: &output.Pagination{EndpointExhausted: !hasMore, Pages: 1, Items: count, NextToken: token}}))
}

// Keep framework success/error transport indicators outside business data.
func mindBusinessData(receipt map[string]any) map[string]any {
	result := map[string]any{}
	for k, v := range receipt {
		switch k {
		case "success", "errorCode", "errorCategory", "errorMessage", "errorMsg", "hint", "actionUrl", "retryable", "recoveryActions":
			continue
		}
		result[k] = v
	}
	return result
}

// A pending result exposes an explicit recovery command with identical business
// arguments. It never automatically replays a write or adds --yes.
func mindResumeCommand(tool string, args map[string]any) string {
	path := map[string]string{"create_mind": "mind create", "create_mind_node": "mind node create", "update_mind_node": "mind node update", "delete_mind_node": "mind node delete"}[tool]
	command := "dws " + path
	for _, binding := range [][2]string{{"name", "name"}, {"source", "source"}, {"folderId", "folder"}, {"workspaceId", "workspace"}, {"nodeId", "node-id"}, {"pageId", "page-id"}, {"topicId", "topic-id"}, {"requestId", "request-id"}, {"parentTopicId", "parent-topic-id"}, {"beforeTopicId", "before-topic-id"}, {"expectedVersion", "expected-version"}, {"node", "topic"}, {"patch", "text"}} {
		raw, ok := args[binding[0]]
		if !ok {
			continue
		}
		var value string
		if binding[0] == "node" {
			data, _ := json.Marshal(raw)
			value = string(data)
		} else if binding[0] == "patch" {
			value, _ = raw.(map[string]any)["text"].(string)
		} else {
			value, _ = raw.(string)
		}
		command += " --" + binding[1] + " " + mindRecoveryQuote(value)
	}
	return command + " --format json"
}

// Bash/zsh ANSI-C quoting keeps a recovery command on one line without changing
// newlines (including trailing ones) or evaluating shell expansions. Ordinary
// arguments use the shared POSIX quoting implementation.
func mindRecoveryQuote(value string) string {
	if !strings.ContainsAny(value, "\n\r") {
		return ShellQuoteArg(value)
	}
	escaped := strings.NewReplacer("\\", "\\\\", "'", "\\'", "\n", "\\n", "\r", "\\r").Replace(value)
	return "$'" + escaped + "'"
}

// Resolve only non-sensitive metadata; never read credentials to build a hint.
// Explicit selection remains usable if the metadata store is unavailable.
func mindBindRecoveryProfile(cmd *cobra.Command, result output.CommandResult) (output.CommandResult, error) {
	if result.Outcome() != output.OutcomePending {
		return result, nil
	}
	selector := authpkg.RuntimeProfile()
	if cmd != nil && cmd.Flags().Lookup("profile") != nil {
		if explicit, err := cmd.Flags().GetString("profile"); err == nil && explicit != "" {
			selector = explicit
		}
	}
	if identity, err := profilemetadata.ResolveReadOnly(config.DefaultConfigDir(), selector); err == nil && identity != nil && identity.CorpID != "" && identity.UserID != "" {
		selector = identity.CorpID + ":" + identity.UserID
	}
	if selector == "" {
		return result, nil
	}
	env, err := output.EnvelopeFromResult(result)
	if err != nil {
		return nil, err
	}
	// EnvelopeFromResult validates that pending results carry meta.operation.
	env.Meta.Operation.NextCommand += " --profile " + mindRecoveryQuote(selector)
	return output.Pending(env.Data, env.Meta.Operation, output.WithMeta(env.Meta)), nil
}
