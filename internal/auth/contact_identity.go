// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.

package auth

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
)

// ContactProfileIdentity 是 current-user 通讯录响应中的组织身份。
type ContactProfileIdentity struct {
	CorpID   string
	CorpName string
	UserID   string
	UserName string
}

type contactIdentityRecord struct {
	CorpID      string `json:"corpId"`
	OrgName     string `json:"orgName"`
	UserID      string `json:"userId"`
	UserIDLower string `json:"userid"`
	OrgUserID   string `json:"orgUserId"`
	OrgUserName string `json:"orgUserName"`
	Name        string `json:"name"`
}

func decodeContactIdentityRecords(data []byte) ([]contactIdentityRecord, bool) {
	var payload struct {
		Result []struct {
			OrgEmployeeModel contactIdentityRecord `json:"orgEmployeeModel"`
		} `json:"result"`
	}
	if err := json.Unmarshal(data, &payload); err != nil || len(payload.Result) == 0 {
		return nil, false
	}
	records := make([]contactIdentityRecord, 0, len(payload.Result))
	for i := range payload.Result {
		records = append(records, payload.Result[i].OrgEmployeeModel)
	}
	return records, true
}

// ContactProfileIdentityFromToolResult 保留普通登录的兼容选择语义：逐个
// 文本块查找第一个可解析身份，旧响应可继续用 orgUserId 补全 UserID。
func ContactProfileIdentityFromToolResult(result *edition.ToolResult, expectedCorpIDs ...string) (ContactProfileIdentity, bool) {
	if result == nil {
		return ContactProfileIdentity{}, false
	}
	for _, block := range result.Content {
		if strings.TrimSpace(block.Text) == "" {
			continue
		}
		if identity, ok := ContactProfileIdentityFromJSON([]byte(block.Text), expectedCorpIDs...); ok {
			return identity, true
		}
	}
	return ContactProfileIdentity{}, false
}

// ContactProfileIdentityFromJSON 解析普通登录使用的 current-user 响应。
// 这里兼容旧返回缺少 corpId 或仅有 orgUserId 的情况；Managed Exchange
// 必须使用下面更严格的 ManagedIdentityFromToolResult。
func ContactProfileIdentityFromJSON(data []byte, expectedCorpIDs ...string) (ContactProfileIdentity, bool) {
	records, ok := decodeContactIdentityRecords(data)
	if !ok {
		return ContactProfileIdentity{}, false
	}
	identities := make([]ContactProfileIdentity, 0, len(records))
	for _, record := range records {
		identity := ContactProfileIdentity{
			CorpID:   strings.TrimSpace(record.CorpID),
			CorpName: strings.TrimSpace(record.OrgName),
			UserID:   firstContactIdentityValue(record.UserID, record.UserIDLower, record.OrgUserID),
			UserName: firstContactIdentityValue(record.OrgUserName, record.Name),
		}
		if identity.CorpID != "" || identity.CorpName != "" || identity.UserID != "" || identity.UserName != "" {
			identities = append(identities, identity)
		}
	}
	if len(identities) == 0 {
		return ContactProfileIdentity{}, false
	}
	expectedCorpID := ""
	if len(expectedCorpIDs) > 0 {
		expectedCorpID = strings.TrimSpace(expectedCorpIDs[0])
	}
	if expectedCorpID != "" {
		for _, identity := range identities {
			if identity.CorpID == expectedCorpID {
				return identity, true
			}
		}
		// 旧 contact 响应可能不返回 corpId；只有单条记录时才可兼容。
		if len(records) != 1 {
			return ContactProfileIdentity{}, false
		}
	}
	return identities[0], true
}

// ManagedIdentityFromToolResult 对 Managed Exchange 做严格解析：只接受
// expectedCorpID 下恰好一条、且包含 userId/userid 的身份。orgUserId 属于
// 另一个标识域，不能用来证明数字员工 Profile 的 userId。
func ManagedIdentityFromToolResult(result *edition.ToolResult, expectedCorpID string) (ManagedIdentity, error) {
	expectedCorpID = strings.TrimSpace(expectedCorpID)
	if result == nil || expectedCorpID == "" {
		return ManagedIdentity{}, fmt.Errorf("current-user identity is missing or ambiguous")
	}
	matches := make([]contactIdentityRecord, 0, 1)
	for _, block := range result.Content {
		if block.Type != "text" || strings.TrimSpace(block.Text) == "" {
			continue
		}
		records, ok := decodeContactIdentityRecords([]byte(block.Text))
		if !ok {
			continue
		}
		for _, record := range records {
			if strings.TrimSpace(record.CorpID) == expectedCorpID {
				matches = append(matches, record)
			}
		}
	}
	if len(matches) != 1 {
		return ManagedIdentity{}, fmt.Errorf("current-user identity is missing or ambiguous")
	}
	record := matches[0]
	userIDs := make(map[string]struct{}, 2)
	for _, value := range []string{record.UserID, record.UserIDLower} {
		if value = strings.TrimSpace(value); value != "" {
			userIDs[value] = struct{}{}
		}
	}
	if len(userIDs) != 1 {
		return ManagedIdentity{}, fmt.Errorf("current-user identity is missing or ambiguous")
	}
	userID := ""
	for value := range userIDs {
		userID = value
	}
	return ManagedIdentity{
		CorpID:   expectedCorpID,
		CorpName: strings.TrimSpace(record.OrgName),
		UserID:   userID,
		UserName: firstContactIdentityValue(record.OrgUserName, record.Name),
	}, nil
}

func firstContactIdentityValue(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}
