// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"math"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/cli"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
)

func TestToolSearchDeliveryActionSiblingRouting(t *testing.T) {
	registerSchemaRuntimeDelivery()
	engine, err := cli.NewDeliveryToolSearchEngine()
	if err != nil {
		t.Fatalf("NewDeliveryToolSearchEngine() error = %v", err)
	}
	tests := []struct {
		query string
		gold  string
	}{
		{query: "创建一场新的会议日程", gold: "calendar.create_calendar_event"},
		{query: "给钉盘文件添加同事阅读权限", gold: "drive.add_permission"},
		// The new main Catalog declares shortcut_fetch as a reviewed equivalent
		// read surface, so this query has graded-equivalent Top-1 answers.
		{query: "读取钉钉在线文字文档正文", gold: "doc.shortcut_fetch|doc.get_document_content"},
		{query: "查询当前用户待审批任务 ID", gold: "oa.list_pending_tasks"},
		{query: "同意指定审批任务", gold: "oa.approve_processInstance"},
		{query: "按群名关键词搜索群聊", gold: "chat.search_groups"},
	}
	for _, test := range tests {
		t.Run(test.gold, func(t *testing.T) {
			response, searchErr := engine.Search(context.Background(), cli.ToolSearchRequest{Query: test.query, Limit: 5})
			if searchErr != nil {
				t.Fatalf("Search() error = %v", searchErr)
			}
			if len(response.Candidates) == 0 || !containsCanonicalAlternative(test.gold, response.Candidates[0].CanonicalPath) {
				t.Fatalf("Search(%q) candidates = %#v, want %s first", test.query, response.Candidates, test.gold)
			}
		})
	}
}

func containsCanonicalAlternative(alternatives, candidate string) bool {
	for _, alternative := range strings.Split(alternatives, "|") {
		if candidate == alternative {
			return true
		}
	}
	return false
}

func TestToolSearchDeliveryDiagnosticTrustAndChineseSlices(t *testing.T) {
	registerSchemaRuntimeDelivery()
	workflows := []cli.ToolSearchWorkflowEvaluationCase{{
		ID:         "chat-send-file-and-check-read",
		Query:      "给群里发文件，并确认群成员是否已读",
		Subqueries: []string{"以当前用户身份给群聊发送本地文件", "查询指定消息的已读状态和人员"},
		Required:   []string{"chat.send_personal_message", "chat.query_msg_read_status"},
	}}
	report, err := cli.BuildDeliveryToolSearchDiagnosticComparison(context.Background(), workflows)
	if err != nil {
		t.Fatalf("BuildDeliveryToolSearchDiagnosticComparison() error = %v", err)
	}
	if report.IntentProxy.Cases != 1123 || report.IntentExcludedOverBudget != 10 {
		t.Fatalf("intent population = %+v excluded=%d", report.IntentProxy, report.IntentExcludedOverBudget)
	}
	if math.Abs(report.IntentProxy.RecallAt5-0.8637577916295637) > 1e-12 {
		t.Fatalf("Recall@5 = %.12f", report.IntentProxy.RecallAt5)
	}
	if report.IntentLanguageSlices["chinese_only"].Cases != 402 || report.IntentLanguageSlices["mixed_chinese_ascii"].Cases != 721 {
		t.Fatalf("language slices = %+v", report.IntentLanguageSlices)
	}
	identity := report.Trust.Identity
	if identity.CanonicalCases != 1098 || identity.CanonicalPassRate != 1 || identity.PrimaryCLIPassRate != 1 ||
		identity.AliasCases == 0 || identity.AliasPassRate != 1 || identity.NFKCCases != 1098 || identity.NFKCPassRate != 1 ||
		identity.ExactFilteredPassRate != 1 {
		t.Fatalf("identity trust = %+v", identity)
	}
	integrity := report.Trust.Integrity
	if integrity.CatalogBindingFailures != 0 || integrity.UnknownCandidateCount != 0 || integrity.IneligibleCandidateCount != 0 || integrity.ResponseBudgetViolations != 0 {
		t.Fatalf("integrity violations = %+v", integrity)
	}
	if report.Context.ToolCount != 1098 || report.Context.ReductionVsFullSchema < 0.99 {
		t.Fatalf("context comparison = %+v", report.Context)
	}
}

func TestSchemaSearchInspectCatalogVersionContract(t *testing.T) {
	registerSchemaRuntimeDelivery()
	search := cli.NewSchemaCommand()
	var searchOutput bytes.Buffer
	search.SetOut(&searchOutput)
	search.SetArgs([]string{"search", "--query", "chat.query_msg_read_status"})
	if err := search.Execute(); err != nil {
		t.Fatalf("schema search execute: %v", err)
	}
	var response cli.ToolSearchResponse
	if err := json.Unmarshal(searchOutput.Bytes(), &response); err != nil {
		t.Fatalf("decode search response: %v", err)
	}
	if response.Strategy != "exact_guard" || len(response.Candidates) != 1 || response.Candidates[0].CanonicalPath != "chat.query_msg_read_status" {
		t.Fatalf("search response = %#v", response)
	}

	inspect := cli.NewSchemaCommand()
	var inspectOutput bytes.Buffer
	inspect.SetOut(&inspectOutput)
	inspect.SetArgs([]string{
		"chat.query_msg_read_status", "--compact",
		"--expected-source-hash", response.Catalog.SourceHash,
		"--expected-surface-hash", response.Catalog.SurfaceHash,
	})
	if err := inspect.Execute(); err != nil {
		t.Fatalf("schema inspect execute: %v", err)
	}
	var payload cli.SchemaInspectV1Response
	if err := json.Unmarshal(inspectOutput.Bytes(), &payload); err != nil {
		t.Fatalf("decode inspect response: %v", err)
	}
	if payload.Version != "schema-inspect.v1" || payload.Catalog != response.Catalog || payload.ToolSpec["canonical_path"] != "chat.query_msg_read_status" {
		t.Fatalf("inspect envelope = %#v", payload)
	}

	stale := cli.NewSchemaCommand()
	stale.SilenceErrors = true
	stale.SilenceUsage = true
	stale.SetArgs([]string{
		"chat.query_msg_read_status", "--compact",
		"--expected-source-hash", "sha256:stale",
		"--expected-surface-hash", response.Catalog.SurfaceHash,
	})
	err := stale.Execute()
	var typed *apperrors.Error
	if !errors.As(err, &typed) || typed.Reason != "catalog_changed" || typed.Category != apperrors.CategoryDiscovery {
		t.Fatalf("stale inspect error = %#v", err)
	}

	incomplete := cli.NewSchemaCommand()
	incomplete.SilenceErrors = true
	incomplete.SilenceUsage = true
	incomplete.SetArgs([]string{
		"chat.query_msg_read_status",
		"--expected-source-hash", response.Catalog.SourceHash,
	})
	err = incomplete.Execute()
	if !errors.As(err, &typed) || typed.Category != apperrors.CategoryValidation {
		t.Fatalf("incomplete inspect version error = %#v", err)
	}
}

func TestSchemaSearchRejectsIgnoredGlobalOutputFlags(t *testing.T) {
	for _, args := range [][]string{
		{"schema", "search", "--query", "查询群消息已读状态", "--fields", "canonical_path"},
		{"schema", "search", "--query", "查询群消息已读状态", "--jq", ".candidates"},
		{"schema", "search", "--query", "查询群消息已读状态", "--format", "pretty"},
	} {
		root := NewRootCommand()
		root.SilenceErrors = true
		root.SilenceUsage = true
		root.SetArgs(args)
		err := root.Execute()
		var typed *apperrors.Error
		if !errors.As(err, &typed) || typed.Category != apperrors.CategoryValidation {
			t.Fatalf("args %v error = %#v, want validation", args, err)
		}
	}
}
