// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package smart

import (
	"errors"
	"strings"
	"testing"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/helpers"
)

func TestCrossPlatformCoverageDocAppendRejectsOversizedText(t *testing.T) {
	// +doc-append takes --text from argv only (no @file, no stdin) and writes via
	// CallMCP, which returns just an error and so cannot report partial progress
	// across chunks. Rather than grow a second chunk loop here, oversized input
	// is pointed at the command that already chunks, repeats table headers and
	// verifies.
	fake := &stubMailboxCaller{}
	long := strings.Repeat("字", helpers.DefaultMarkdownChunkRunes+1)
	err := runShortcutErr(t, fake, "doc", "+doc-append", "--doc", "node-1", "--text", long, "--yes")
	var typed *apperrors.Error
	if err == nil || !errors.As(err, &typed) || typed.Reason != "doc_append_content_too_long" {
		t.Fatalf("expected a fail-closed validation error, got %#v", err)
	}
	if !strings.Contains(strings.Join(typed.Actions, " "), "+update") {
		t.Errorf("error must point at the chunking command: %#v", typed.Actions)
	}

	// Exactly at the limit is still allowed, so the guard is a ceiling rather
	// than an off-by-one refusal.
	atLimit := strings.Repeat("字", helpers.DefaultMarkdownChunkRunes)
	if err := runShortcutErr(t, fake, "doc", "+doc-append", "--doc", "node-1", "--text", atLimit, "--yes"); err != nil {
		t.Fatalf("content at the limit must be accepted: %v", err)
	}
}
