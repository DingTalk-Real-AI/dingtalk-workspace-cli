// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package cli

import (
	"context"
	"math"
	"testing"
)

func TestEvaluateToolSearchIndependentScoresGradedSafetyAndWorkflow(t *testing.T) {
	engine := newToolSearchTestEngine(t, nil)
	cases := []ToolSearchIndependentCase{
		{
			ID: "chat", Query: "给群里发送消息", Language: "chinese_only",
			Qrels:     []ToolSearchIndependentQrel{{Canonical: "chat.send", Relevance: 3}},
			Forbidden: []string{"doc.read"}, AlternativeGold: []string{"chat.send"},
			ConfusionFamily: []string{"chat.read_status"},
		},
		{
			ID: "workflow", Query: "发送消息并读取文档", Language: "chinese_only",
			Qrels: []ToolSearchIndependentQrel{{Canonical: "chat.send", Relevance: 3}, {Canonical: "doc.read", Relevance: 2}},
			Workflow: &ToolSearchIndependentWorkflow{
				Subqueries: []string{"群聊发送消息", "读取在线文档"},
				Required:   []string{"chat.send", "doc.read"},
			},
		},
	}
	report, err := EvaluateToolSearchIndependent(context.Background(), engine, cases)
	if err != nil {
		t.Fatal(err)
	}
	if report.Version != "tool-search-independent-evaluation.v1" || report.Overall.Cases != 2 {
		t.Fatalf("report = %#v", report)
	}
	if report.Overall.RecallAt5 != 1 || report.Overall.NDCGAt5 <= 0 || report.Overall.MeanReciprocalRankAt5 <= 0 {
		t.Fatalf("ranking = %#v", report.Overall)
	}
	if report.Safety.ForbiddenCases != 1 || report.Safety.ForbiddenExposureAt5 != 0 || report.Safety.AlternativeRecallAt5 != 1 ||
		report.Safety.SiblingConfusionCases != 1 || report.Safety.SiblingExposureAt5 != 1 {
		t.Fatalf("safety = %#v", report.Safety)
	}
	if report.Workflow.Cases != 1 || report.Workflow.CompleteAt5 != 1 || report.Workflow.RequiredRecallAt5 != 1 {
		t.Fatalf("workflow = %#v", report.Workflow)
	}
}

func TestIndependentRankingMetricsUseGradedRankDiscounts(t *testing.T) {
	accumulator := &independentRankingAccumulator{}
	accumulator.observe([]ToolReference{
		{CanonicalPath: "irrelevant"},
		{CanonicalPath: "relevant.low"},
	}, []ToolSearchIndependentQrel{
		{Canonical: "relevant.high", Relevance: 3},
		{Canonical: "relevant.low", Relevance: 2},
	})
	metrics := accumulator.result()
	wantNDCG := (3 / math.Log2(3)) / (7 + 3/math.Log2(3))
	if metrics.RecallAt1 != 0 || metrics.RecallAt5 != 0.5 || metrics.MeanReciprocalRankAt5 != 0.5 || math.Abs(metrics.NDCGAt5-wantNDCG) > 1e-12 || metrics.ZeroResultRate != 0 {
		t.Fatalf("metrics = %#v, want R1=0 R5=.5 MRR5=.5 NDCG5=%.12f", metrics, wantNDCG)
	}

	zero := &independentRankingAccumulator{}
	zero.observe(nil, []ToolSearchIndependentQrel{{Canonical: "relevant", Relevance: 3}})
	zeroMetrics := zero.result()
	if zeroMetrics.ZeroResultRate != 1 || zeroMetrics.RecallAt5 != 0 || zeroMetrics.NDCGAt5 != 0 {
		t.Fatalf("zero metrics = %#v", zeroMetrics)
	}
}
