// Copyright 2026 Alibaba Group
// SPDX-License-Identifier: Apache-2.0

// Package aitableprotocol contains small, stable validations shared by the
// native AI Table commands and their shortcut adapters.
package aitableprotocol

import (
	"fmt"
	"regexp"
	"strings"
	"unicode/utf16"
)

const MaxFieldNameUTF16Length = 150

var uuidV4Pattern = regexp.MustCompile(`(?i)^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)

// UTF16Length matches Java String.length and the Notable Core protocol.
func UTF16Length(value string) int {
	return len(utf16.Encode([]rune(value)))
}

// ValidateFieldName applies the public AI Table field-name contract.
func ValidateFieldName(value string) error {
	if strings.TrimSpace(value) == "" {
		return fmt.Errorf("字段名称不能为空")
	}
	if length := UTF16Length(value); length > MaxFieldNameUTF16Length {
		return fmt.Errorf("字段名称最多 %d 个 UTF-16 字符，当前为 %d", MaxFieldNameUTF16Length, length)
	}
	return nil
}

// ValidateClientToken accepts the UUID v4 form used by idempotent record creates.
func ValidateClientToken(value string) error {
	if value == "" {
		return nil
	}
	if !uuidV4Pattern.MatchString(value) {
		return fmt.Errorf("client-token 必须是合法的 UUID v4")
	}
	return nil
}
