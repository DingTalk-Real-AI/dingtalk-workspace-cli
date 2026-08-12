// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.

package smart

import (
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut/targetresolver"
)

// contactUser preserves the smart package's internal shape while resolution
// semantics live in targetresolver for reuse by chat and future facades.
type contactUser struct {
	userID         string
	openDingTalkID string
	name           string
}

func resolveUser(rt *shortcut.RuntimeContext, name string) (contactUser, error) {
	return resolveUserByName(rt, name, false)
}

func resolveOpenDingTalkUser(rt *shortcut.RuntimeContext, name string) (contactUser, error) {
	return resolveUserByName(rt, name, true)
}

func resolveUserByName(rt *shortcut.RuntimeContext, name string, includeOpenIDOnly bool) (contactUser, error) {
	requirement := targetresolver.IdentityUserID
	if includeOpenIDOnly {
		requirement = targetresolver.IdentityOpenDingTalkID
	}
	resolved, err := targetresolver.ResolveUser(rt, name, requirement)
	if err != nil {
		return contactUser{}, err
	}
	return fromResolvedUser(resolved.Selected), nil
}

func usersWithUserID(users []contactUser) []contactUser {
	resolved := make([]targetresolver.User, 0, len(users))
	for _, user := range users {
		resolved = append(resolved, toResolvedUser(user))
	}
	filtered := targetresolver.UsersWithUserID(resolved)
	out := make([]contactUser, 0, len(filtered))
	for _, user := range filtered {
		out = append(out, fromResolvedUser(user))
	}
	return out
}

func extractUsers(data map[string]any) []contactUser {
	resolved := targetresolver.ExtractUsers(data)
	if len(resolved) == 0 {
		return nil
	}
	out := make([]contactUser, 0, len(resolved))
	for _, user := range resolved {
		out = append(out, fromResolvedUser(user))
	}
	return out
}

func userLabels(users []contactUser) []string {
	resolved := make([]targetresolver.User, 0, len(users))
	for _, user := range users {
		resolved = append(resolved, toResolvedUser(user))
	}
	return targetresolver.UserLabels(resolved)
}

func fromResolvedUser(user targetresolver.User) contactUser {
	return contactUser{
		userID:         user.UserID,
		openDingTalkID: user.OpenDingTalkID,
		name:           user.Name,
	}
}

func toResolvedUser(user contactUser) targetresolver.User {
	return targetresolver.User{
		UserID:         user.userID,
		OpenDingTalkID: user.openDingTalkID,
		Name:           user.name,
	}
}
