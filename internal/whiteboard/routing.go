// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

// Package whiteboard owns the framework-independent routing contract shared by
// the native whiteboard commands and their strict Shortcut counterparts.
package whiteboard

import (
	"fmt"
	"regexp"
	"strings"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
)

const (
	ServerID = "whiteboard"

	EmbeddedQueryTool  = "read_whiteboard_content"
	EmbeddedUpdateTool = "update_whiteboard"

	StandaloneCreateTool = "create_whiteboard"
	StandaloneQueryTool  = "get_whiteboard_detail"
	StandaloneUpdateTool = "update_whiteboard_content"
)

// Kind identifies the target model selected before the first remote call.
type Kind string

const (
	KindEmbedded   Kind = "embedded"
	KindStandalone Kind = "standalone"
)

var requestIDPattern = regexp.MustCompile(`^[A-Za-z0-9._:-]{1,128}$`)

// Target preserves flag presence separately from its value. An explicitly
// empty --part-id is invalid and must never be reinterpreted as standalone.
type Target struct {
	NodeID        string
	PartID        string
	PartIDChanged bool
}

type QueryOptions struct {
	Target
	View          string
	ViewChanged   bool
	PageID        string
	PageIDChanged bool
}

type UpdateOptions struct {
	Target
	PageID                  string
	PageIDChanged           bool
	ExpectedRevision        int
	ExpectedRevisionChanged bool
	RequestID               string
	RequestIDChanged        bool
	Mode                    string
	NodesJSON               string
}

// Call is one resolved MCP invocation.
type Call struct {
	Kind Kind
	Tool string
	Args map[string]any
}

func validation(message string) error {
	return apperrors.NewValidation(message)
}

// ResolveKind implements the public compatibility rule: only an explicitly
// provided, non-empty part id selects the embedded path. Omission selects the
// standalone path; explicit empty input fails closed.
func ResolveKind(target Target) (Kind, error) {
	if strings.TrimSpace(target.NodeID) == "" {
		return "", validation("--node 去除空白后不能为空")
	}
	if !target.PartIDChanged {
		return KindStandalone, nil
	}
	if strings.TrimSpace(target.PartID) == "" {
		return "", validation("--part-id 已显式提供但内容为空")
	}
	return KindEmbedded, nil
}

// BuildQueryCall validates branch-specific flags and returns the only tool that
// may be invoked. Callers must not fall back to the sibling tool on failure.
func BuildQueryCall(options QueryOptions) (Call, error) {
	kind, err := ResolveKind(options.Target)
	if err != nil {
		return Call{}, err
	}
	if kind == KindEmbedded {
		if options.ViewChanged || options.PageIDChanged {
			return Call{}, validation("内嵌白板查询不支持 --view 或 --page-id；请仅提供 --node 和 --part-id")
		}
		return Call{Kind: kind, Tool: EmbeddedQueryTool, Args: map[string]any{
			"nodeId": strings.TrimSpace(options.NodeID),
			"partId": strings.TrimSpace(options.PartID),
		}}, nil
	}

	view := strings.TrimSpace(options.View)
	if options.ViewChanged && view == "" {
		return Call{}, validation("--view 已显式提供但内容为空")
	}
	if view == "" {
		view = "summary"
	}
	switch view {
	case "summary", "all":
		if options.PageIDChanged {
			return Call{}, validation("独立白板仅在 --view page 时允许提供 --page-id")
		}
	case "page":
		if !options.PageIDChanged || strings.TrimSpace(options.PageID) == "" {
			return Call{}, validation("独立白板 --view page 时必须提供非空 --page-id")
		}
	default:
		return Call{}, validation("--view 只能是 summary、page 或 all")
	}
	args := map[string]any{"nodeId": strings.TrimSpace(options.NodeID), "view": view}
	if view == "page" {
		args["pageId"] = strings.TrimSpace(options.PageID)
	}
	return Call{Kind: kind, Tool: StandaloneQueryTool, Args: args}, nil
}

// BuildUpdateCall applies the same compatibility split to writes. The caller
// supplies already-validated OpenNodes mode/nodes so both command surfaces keep
// one routing and conditional-parameter contract.
func BuildUpdateCall(options UpdateOptions) (Call, error) {
	kind, err := ResolveKind(options.Target)
	if err != nil {
		return Call{}, err
	}
	if options.Mode != "append" && options.Mode != "overwrite" {
		return Call{}, validation("白板更新 mode 只能是 append 或 overwrite")
	}
	if strings.TrimSpace(options.NodesJSON) == "" {
		return Call{}, validation("白板更新 nodes 不能为空")
	}

	pageID := strings.TrimSpace(options.PageID)
	if options.PageIDChanged && pageID == "" {
		return Call{}, validation("--page-id 已显式提供但内容为空")
	}
	if kind == KindEmbedded {
		if options.PageIDChanged || options.ExpectedRevisionChanged || options.RequestIDChanged {
			return Call{}, validation("内嵌白板更新不支持 --page-id、--expected-revision 或 --request-id")
		}
		return Call{Kind: kind, Tool: EmbeddedUpdateTool, Args: map[string]any{
			"nodeId": strings.TrimSpace(options.NodeID),
			"partId": strings.TrimSpace(options.PartID),
			"mode":   options.Mode,
			"nodes":  options.NodesJSON,
		}}, nil
	}

	if !options.ExpectedRevisionChanged {
		return Call{}, validation("独立白板更新必须提供 --expected-revision")
	}
	if options.ExpectedRevision < 0 {
		return Call{}, validation("--expected-revision 必须是非负整数")
	}
	if !options.RequestIDChanged || strings.TrimSpace(options.RequestID) == "" {
		return Call{}, validation("独立白板更新必须提供非空 --request-id")
	}
	requestID := strings.TrimSpace(options.RequestID)
	if !requestIDPattern.MatchString(requestID) {
		return Call{}, validation("--request-id 必须为 1-128 个字符，且只能包含字母、数字、点、下划线、冒号和连字符")
	}
	if options.Mode == "overwrite" && (!options.PageIDChanged || pageID == "") {
		return Call{}, validation("独立白板 overwrite 更新必须提供非空 --page-id")
	}
	args := map[string]any{
		"nodeId":           strings.TrimSpace(options.NodeID),
		"mode":             options.Mode,
		"nodes":            options.NodesJSON,
		"expectedRevision": options.ExpectedRevision,
		"requestId":        requestID,
	}
	if options.PageIDChanged {
		args["pageId"] = pageID
	}
	return Call{Kind: kind, Tool: StandaloneUpdateTool, Args: args}, nil
}

// ValidateCreateRequestID enforces the idempotency-key contract shared by
// standalone create and update.
func ValidateCreateRequestID(value string) error {
	value = strings.TrimSpace(value)
	if value == "" {
		return validation("--request-id 去除空白后不能为空")
	}
	if !requestIDPattern.MatchString(value) {
		return validation(fmt.Sprintf("--request-id %q 必须为 1-128 个字符，且只能包含字母、数字、点、下划线、冒号和连字符", value))
	}
	return nil
}
