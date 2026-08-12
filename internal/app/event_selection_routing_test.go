// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package app

import (
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/cli"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
)

func TestCrossPlatformCoverageEventAgentSelectionBoundaries(t *testing.T) {
	_ = NewRootCommand()

	eventProduct, ok := contract.LookupProductDecl("event")
	if !ok {
		t.Fatal("event ProductDecl is not registered")
	}
	assertSelectionContains(t, "event product", eventProduct.Selection.AgentSummary,
		[]string{"IM", "OA"})
	assertSelectionContains(t, "event product use_when", strings.Join(eventProduct.Selection.UseWhen, "\n"),
		[]string{"消息", "群生命周期", "OA"})
	assertSelectionContains(t, "event product avoid_when", strings.Join(eventProduct.Selection.AvoidWhen, "\n"),
		[]string{"chat", "oa", "dev app event"})

	listenMeta, ok := cli.ResolveMeta("event +listen-im")
	if !ok {
		t.Fatal("event +listen-im metadata is not registered")
	}
	assertSelectionContains(t, "event.listen_im use_when", strings.Join(listenMeta.Selection.UseWhen, "\n"),
		[]string{"@我", "message/reaction/read/recall"})
	assertSelectionContains(t, "event.listen_im avoid_when", strings.Join(listenMeta.Selection.AvoidWhen, "\n"),
		[]string{"OA 审批事件", "群标题", "Filter DSL", "event consume", "历史消息"})

	consumeMeta, ok := cli.ResolveMeta("event consume")
	if !ok {
		t.Fatal("event consume metadata is not registered")
	}
	consumeUse := strings.Join(consumeMeta.Selection.UseWhen, "\n")
	assertSelectionContains(t, "event.consume use_when", consumeUse,
		[]string{"OA", "群", "EventKey", "Filter DSL", "subscribe_id", "transport envelope", "高级多事件"})
	consumeAvoid := strings.Join(consumeMeta.Selection.AvoidWhen, "\n")
	assertSelectionContains(t, "event.consume avoid_when", consumeAvoid,
		[]string{"event +listen-im", "历史聊天", "oa", "dev app event"})

	schemaMeta, ok := cli.ResolveMeta("event schema")
	if !ok {
		t.Fatal("event schema metadata is not registered")
	}
	assertSelectionContains(t, "event.schema use_when", strings.Join(schemaMeta.Selection.UseWhen, "\n"),
		[]string{"IM", "OA", "--flatten"})

	for productID, want := range map[string]string{
		"chat": "event +listen-im",
		"oa":   "event consume",
	} {
		decl, found := contract.LookupProductDecl(productID)
		if !found {
			t.Fatalf("%s ProductDecl is not registered", productID)
		}
		assertSelectionContains(t, productID+" avoid_when", strings.Join(decl.Selection.AvoidWhen, "\n"), []string{want})
	}
}

func assertSelectionContains(t *testing.T, label, text string, fragments []string) {
	t.Helper()
	for _, fragment := range fragments {
		if !strings.Contains(text, fragment) {
			t.Errorf("%s = %q, want fragment %q", label, text, fragment)
		}
	}
}
