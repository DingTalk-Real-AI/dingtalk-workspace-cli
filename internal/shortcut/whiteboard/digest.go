// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package whiteboard

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"regexp"
	"strings"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/whiteboard/opennodes"
)

var whiteboardSourceDigestPattern = regexp.MustCompile(`^sha256:[0-9a-fA-F]{64}$`)

// whiteboardSourceDigest hashes the normalized update intent. Both +diff and
// +update call this function so the optimistic source guard cannot drift from
// the preview representation.
func whiteboardSourceDigest(parsed *parsedUpdate) (string, error) {
	digest, err := opennodes.DigestUpdate(parsed.Overwrite, parsed.Nodes)
	if err != nil {
		return "", apperrors.NewInternal("计算白板 source digest 失败: " + err.Error())
	}
	return digest, nil
}

func validateExpectedSourceDigest(expected string, changed bool, parsed *parsedUpdate) error {
	if !changed {
		return nil
	}
	expected = strings.TrimSpace(expected)
	if !whiteboardSourceDigestPattern.MatchString(expected) {
		return apperrors.NewValidation(
			"--expected-source-digest 必须是 sha256:<64位十六进制> 格式",
			apperrors.WithReason("invalid_expected_source_digest"),
			apperrors.WithExecutionStarted(false),
			apperrors.WithRetryable(false),
		)
	}
	actual, err := whiteboardSourceDigest(parsed)
	if err != nil {
		return err
	}
	if !strings.EqualFold(expected, actual) {
		return apperrors.NewValidation(
			"--source 已不同于 +diff 预览时的内容，请重新执行 dws whiteboard +diff",
			apperrors.WithReason("source_digest_mismatch"),
			apperrors.WithExecutionStarted(false),
			apperrors.WithRetryable(false),
		)
	}
	return nil
}

func whiteboardSnapshotDigest(source map[string]any) (string, error) {
	encoded, err := json.Marshal(source)
	if err != nil {
		return "", apperrors.NewInternal("计算白板 snapshot digest 失败: " + err.Error())
	}
	sum := sha256.Sum256(encoded)
	return "sha256:" + hex.EncodeToString(sum[:]), nil
}
