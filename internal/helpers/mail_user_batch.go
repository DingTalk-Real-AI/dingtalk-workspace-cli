// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package helpers

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
	"github.com/spf13/cobra"
)

type mailUserBatchItem struct {
	id       string
	email    string
	user     map[string]json.RawMessage
	notFound bool
	failure  *output.ErrorInfo
}

func callMailUserBatchLookupResult(cmd *cobra.Command, tool string, args map[string]any) (output.CommandResult, error) {
	if deps.Caller.DryRun() {
		return output.Success(map[string]any{"tool": tool, "arguments": args, "executed": false}, output.WithDryRun()), nil
	}
	// Read the original flags: the framework's RPC binding drops empty entries.
	// Keep their indexes so they can be reported without rejecting valid inputs.
	emails, err := cmd.Flags().GetStringSlice("org-emails")
	if err != nil {
		return nil, err
	}
	items := make([]*mailUserBatchItem, 0, len(emails))
	byEmail := make(map[string]*mailUserBatchItem)
	for i, email := range emails {
		email = strings.TrimSpace(email)
		if email == "" {
			items = append(items, &mailUserBatchItem{id: fmt.Sprintf("org-emails[%d]", i), failure: &output.ErrorInfo{Type: "validation", Message: "email must not be empty"}})
			continue
		}
		key := strings.ToLower(email)
		if byEmail[key] == nil {
			item := &mailUserBatchItem{id: email, email: email}
			items = append(items, item)
			byEmail[key] = item
		}
	}
	// Invalid input can itself equal an empty-slot label. Keep per-item IDs
	// distinct so the partial-result invariant cannot discard valid neighbors.
	for _, item := range items {
		if item.email == "" {
			for byEmail[strings.ToLower(item.id)] != nil {
				item.id = "empty:" + item.id
			}
		}
	}
	data, err := fetchMailUserLookupData(cmd, tool, args)
	if err != nil {
		if !mailUserBatchRejectedAddress(err) {
			return nil, err
		}
		// The current service rejects a whole batch for one invalid address.
		// Only that explicit error allows bounded single-address read fallback.
		for _, item := range items {
			if item.failure != nil {
				continue
			}
			if cmd.Context().Err() != nil {
				break
			}
			single, singleErr := fetchMailUserLookupData(cmd, "get_user_by_org_email", map[string]any{"orgEmail": item.email})
			if singleErr != nil {
				item.failure = &output.ErrorInfo{Type: "api", Message: singleErr.Error()}
				if mailUserLookupGlobalFailure(singleErr) {
					break
				}
				continue
			}
			var user map[string]json.RawMessage
			if len(single["result"]) == 0 {
				item.notFound = true
				continue
			}
			if json.Unmarshal(single["result"], &user) != nil {
				continue
			}
			var uid *int64
			if raw := user["uid"]; len(raw) != 0 && json.Unmarshal(raw, &uid) != nil {
				continue
			}
			if uid == nil || *uid <= 0 {
				item.notFound = true
				continue
			}
			// Single lookup already identifies the requested address. If the
			// service also returns an address, require it to agree.
			var returnedEmail string
			if raw := user["orgEmail"]; len(raw) != 0 {
				if json.Unmarshal(raw, &returnedEmail) != nil || !strings.EqualFold(returnedEmail, item.email) {
					continue
				}
			}
			item.user = user
		}
		return mailUserBatchResult(items)
	}

	// Decode each channel and record independently. An invalid channel or row
	// cannot erase a confirmed result from another row/channel.
	var result map[string]json.RawMessage
	_ = json.Unmarshal(data["result"], &result)
	var users, missing []json.RawMessage
	_ = json.Unmarshal(result["users"], &users)
	_ = json.Unmarshal(result["notFoundOrgEmails"], &missing)
	for _, raw := range users {
		var user map[string]json.RawMessage
		if json.Unmarshal(raw, &user) != nil {
			continue
		}
		var email string
		var uid int64
		if json.Unmarshal(user["orgEmail"], &email) != nil || json.Unmarshal(user["uid"], &uid) != nil || uid <= 0 {
			continue
		}
		if item := byEmail[strings.ToLower(strings.TrimSpace(email))]; item != nil && item.user == nil {
			item.user = user
		}
	}
	for _, raw := range missing {
		var email string
		if json.Unmarshal(raw, &email) != nil {
			continue
		}
		if item := byEmail[strings.ToLower(strings.TrimSpace(email))]; item != nil && item.user == nil {
			item.notFound = true
		}
	}
	return mailUserBatchResult(items)
}

func mailUserBatchRejectedAddress(err error) bool {
	var body struct{ ErrorCode, ErrorMsg string }
	var apiErr *apperrors.Error
	if errors.As(err, &apiErr) {
		// The runtime classifies business failures before helpers see them.
		if apiErr.Category != apperrors.CategoryAPI {
			return false
		}
		body.ErrorCode, body.ErrorMsg = apiErr.ServerDiag.ServerErrorCode, apiErr.Message
	} else {
		var cliErr *CLIError
		if !errors.As(err, &cliErr) || cliErr.Code != CodeMCPToolError {
			return false
		}
		if json.Unmarshal([]byte(cliErr.Message), &body) != nil {
			return false
		}
	}
	return body.ErrorCode == "SYSTEM_ERROR" && strings.HasPrefix(body.ErrorMsg, "orgEmails[") && strings.HasSuffix(body.ErrorMsg, "必须是完整有效的企业邮箱地址")
}

func mailUserLookupGlobalFailure(err error) bool {
	var exit interface{ ExitCode() int }
	if errors.As(err, &exit) && (exit.ExitCode() == ExitAuth || exit.ExitCode() == ExitPermission) {
		return true
	}
	var apiErr *apperrors.Error
	if errors.As(err, &apiErr) {
		switch apiErr.Reason {
		case "request_timeout", "request_cancelled", "http_client_timeout", "tls_timeout", "connection_refused", "dns_resolution_failed", "io_timeout", "request_failed":
			return true
		}
		return strings.EqualFold(apiErr.ServerDiag.ServerErrorCode, "noPermission")
	}
	var cliErr *CLIError
	if !errors.As(err, &cliErr) {
		return false
	}
	if cliErr.Code == CodeNetworkTimeout || cliErr.Code == CodeNetworkUnreachable {
		return true
	}
	var body struct{ ErrorCode string }
	_ = json.Unmarshal([]byte(cliErr.Message), &body)
	return strings.EqualFold(body.ErrorCode, "noPermission")
}

func mailUserBatchResult(items []*mailUserBatchItem) (output.CommandResult, error) {
	users := make([]map[string]json.RawMessage, 0)
	missing := make([]string, 0)
	succeeded := make([]any, 0)
	failed := make([]output.PartialFailedEntry, 0)
	unknown := make([]output.PartialUnknownEntry, 0)
	for _, item := range items {
		switch {
		case item.failure != nil:
			failed = append(failed, output.PartialFailedEntry{ID: item.id, Error: item.failure})
		case item.user != nil || item.notFound:
			entry := map[string]any{"id": item.id, "orgEmail": item.email, "found": item.user != nil}
			if item.user != nil {
				users = append(users, item.user)
				entry["user"] = item.user
			} else {
				missing = append(missing, item.email)
			}
			succeeded = append(succeeded, entry)
		default:
			unknown = append(unknown, output.PartialUnknownEntry{ID: item.id, Reason: "lookup did not return a valid employee or confirm not-found; retry this address"})
		}
	}
	if len(failed) == 0 && len(unknown) == 0 {
		return output.Success(map[string]any{"success": true, "result": map[string]any{"users": users, "notFoundOrgEmails": missing}}), nil
	}
	if len(succeeded) == 0 {
		return output.Failure(&output.ErrorInfo{Type: "api", Message: "no email lookup has a confirmed result", Details: map[string]any{"failed": failed, "unknown": unknown}}), nil
	}
	partial, err := output.NewPartialData(len(items), succeeded, failed, unknown)
	if err != nil {
		return nil, err
	}
	return output.Partial(partial), nil
}
