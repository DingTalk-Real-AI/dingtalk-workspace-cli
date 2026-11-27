// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0.
package helpers

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/card/chartgen"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/runtimeannotate"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
	"github.com/spf13/cobra"
)

type chartWriteCloser interface {
	Write([]byte) (int, error)
	Close() error
}

var (
	aicardChartList                    = chartgen.List
	aicardChartOpenInput               = os.Open
	aicardChartReadAll                 = io.ReadAll
	aicardChartMarshalIndent           = json.MarshalIndent
	aicardChartStat                    = os.Stat
	aicardChartStdin         io.Reader = os.Stdin
	aicardChartOpenNew                 = func(path string) (chartWriteCloser, error) {
		return os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	}
	aicardChartRemove = os.Remove
)

func chartLeaf(cmd *cobra.Command, name, description, effect, useWhen, avoidWhen, example string) {
	idempotency := "idempotent"
	if name == "build" {
		idempotency = "non_idempotent"
	}
	DeclareLeafMetadata(cmd, LeafSpec{
		Safety: contract.SafetySpec{Effect: effect, Risk: "low", Confirmation: "not_required", Idempotency: idempotency},
		Contract: LeafContract{
			Identity:    contract.ToolIdentitySpec{ProductID: "aicard", Name: "chart." + name, Group: "chart", CanonicalPath: "aicard.chart." + name, CLIPath: "aicard chart " + name, PrimaryCLIPath: "aicard chart " + name},
			Description: description,
			Interface:   &contract.InterfaceSpec{Mode: "local", Availability: "available", Reason: "Uses an embedded, source-traceable chart preset registry; never sends a card"},
			Selection:   contract.SelectionSpec{AgentSummary: description, UseWhen: []string{useWhen}, AvoidWhen: []string{avoidWhen}, Examples: []string{example}},
			Result:      &contract.ResultSpec{Outcomes: []contract.ResultOutcome{"success", "failure"}, DataSchema: chartResultSchema(name)},
		},
	})
	output.SetCommandRollout(cmd, output.RolloutUnifiedActive)
}

func chartResultSchema(name string) json.RawMessage {
	switch name {
	case "list":
		return json.RawMessage(`{"type":"object","description":"Available AICard chart presets","required":["registryVersion","presets","rendererVerified"],"properties":{"registryVersion":{"type":"string","description":"Embedded chart registry version"},"presets":{"type":"array","description":"Supported chart presets","items":{"type":"object","description":"One source-traceable chart preset","required":["id","alias","family","seriesTypes","sourceSha256","rendererVerified"],"properties":{"id":{"type":"string","description":"Stable G-series preset ID"},"alias":{"type":"string","description":"Semantic preset alias"},"family":{"type":"string","description":"Strict business-data family"},"useWhen":{"type":"string","description":"Suggested analytical purpose for this data family"},"seriesTypes":{"type":"array","description":"Chart series types used by the preset","items":{"type":"string"}},"sourceSha256":{"type":"string","description":"SHA-256 of the authoritative source sample"},"rendererVerified":{"type":"boolean","description":"Whether this preset has client renderer evidence"}},"additionalProperties":false}},"rendererVerified":{"type":"boolean","description":"Whether all returned presets have client renderer evidence"}},"additionalProperties":false}`)
	case "explain":
		return json.RawMessage(`{"type":"object","description":"One chart preset and its strict input example","required":["preset","specVersion","chartSpecExample","rendererVerified"],"properties":{"preset":{"type":"object","description":"Source-traceable chart preset metadata","additionalProperties":true},"specVersion":{"type":"string","description":"Internal ChartSpec version"},"chartSpecExample":{"type":"object","description":"Minimal strict business-data example for this preset","additionalProperties":true},"rendererVerified":{"type":"boolean","description":"Whether this preset has client renderer evidence"}},"additionalProperties":false}`)
	case "build":
		return json.RawMessage(`{"type":"object","description":"Offline chart-card build result","required":["presetId","presetAlias","surfaceId","summary","preflight","rendererVerified","sent","written"],"properties":{"presetId":{"type":"string","description":"Resolved G-series preset ID"},"presetAlias":{"type":"string","description":"Resolved semantic preset alias"},"surfaceId":{"type":"string","description":"Generated Surface ID reused by the saved lifecycle artifact"},"summary":{"type":"string","description":"Fallback summary derived from supplied business facts"},"preflight":{"type":"object","description":"Offline new-card validation result","additionalProperties":true},"rendererVerified":{"type":"boolean","description":"Whether client rendering was verified; always false for this offline command"},"sent":{"type":"boolean","description":"Whether a card was sent; always false for this offline command"},"written":{"type":"boolean","description":"Whether output files were written"},"cardPath":{"type":"string","description":"Written creation-payload path; omitted for dry-run"},"artifactPath":{"type":"string","description":"Written lifecycle-artifact path; omitted for dry-run"}},"additionalProperties":false}`)
	default:
		panic("unknown chart result schema: " + name)
	}
}

func chartRecord(p chartgen.Preset) map[string]any {
	purpose := map[string]string{
		"category":     "Compare aligned categories or trends across series; use the alias to select grouping, stacking, or orientation",
		"named_values": "Compare shares or ordered stages; pie and funnel aliases select the visual",
		"points":       "Inspect relationships or distribution of paired observations",
		"matrix":       "Compare values across two categorical dimensions",
		"ohlc":         "Show open, high, low, and close values by period",
		"radar":        "Compare groups across bounded indicators",
		"measure":      "Show one measure against a known range",
		"flow":         "Show weighted directed flow between named nodes",
	}[p.Family]
	return map[string]any{"id": p.ID, "alias": p.Alias, "family": p.Family, "useWhen": purpose, "seriesTypes": p.SeriesTypes, "sourceSha256": p.SourceSHA256, "rendererVerified": false}
}

func chartExample(p chartgen.Preset) any {
	switch p.Family {
	case "category":
		series := []map[string]any{}
		for i := range p.SeriesTypes {
			series = append(series, map[string]any{"name": fmt.Sprintf("Series %d", i+1), "unit": "items", "values": []float64{2, 3}})
		}
		data := map[string]any{"kind": "category", "categories": []string{"A", "B"}, "series": series}
		if p.ID == "G31" {
			data["stackValues"] = "raw"
		}
		if p.ID == "G34" {
			data["cumulativeMode"] = "increments"
		}
		return data
	case "named_values":
		return map[string]any{"kind": "named_values", "items": []map[string]any{{"name": "A", "value": 3}, {"name": "B", "value": 2}}}
	case "points":
		groups := []map[string]any{}
		for i := range p.SeriesTypes {
			groups = append(groups, map[string]any{"name": fmt.Sprintf("Group %d", i+1), "points": [][]float64{{1, 2}, {2, 3}}})
		}
		return map[string]any{"kind": "points", "xLabel": "X", "yLabel": "Y", "groups": groups}
	case "matrix":
		return map[string]any{"kind": "matrix", "xLabels": []string{"A", "B"}, "yLabels": []string{"C", "D"}, "cells": []map[string]any{{"x": 0, "y": 0, "value": 1}, {"x": 1, "y": 0, "value": 2}, {"x": 0, "y": 1, "value": 3}, {"x": 1, "y": 1, "value": 4}}}
	case "ohlc":
		return map[string]any{"kind": "ohlc", "categories": []string{"A", "B"}, "name": "Price", "candles": [][]float64{{5, 6, 4, 7}, {6, 7, 5, 8}}}
	case "radar":
		groups := []map[string]any{}
		for i := 0; i < p.RadarGroupCount; i++ {
			groups = append(groups, map[string]any{"name": fmt.Sprintf("Group %d", i+1), "values": []float64{2, 3, 4}})
		}
		return map[string]any{"kind": "radar", "indicators": []map[string]any{{"name": "A", "max": 10}, {"name": "B", "max": 10}, {"name": "C", "max": 10}}, "groups": groups}
	case "measure":
		return map[string]any{"kind": "measure", "name": "Score", "value": 5, "min": 0, "max": 10}
	case "flow":
		return map[string]any{"kind": "flow", "nodes": []string{"A", "B"}, "links": []map[string]any{{"source": "A", "target": "B", "value": 2}}}
	}
	return nil
}

func newAicardChartCommand() *cobra.Command {
	root := newGroupCommand(&cobra.Command{Use: "chart", Short: "Build a chart card from business facts", RunE: groupRunE})
	list := &cobra.Command{Use: "list", Short: "List the 43 available visual presets", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		presets, err := aicardChartList()
		if err != nil {
			return aicardFailure(cmd, "internal", err.Error(), nil)
		}
		items := make([]map[string]any, 0, len(presets))
		for _, p := range presets {
			items = append(items, chartRecord(p))
		}
		return output.StoreResult(cmd.Context(), output.Success(map[string]any{"registryVersion": "1.0", "presets": items, "rendererVerified": false}))
	}}
	chartLeaf(
		list,
		"list",
		"List 43 chart presets and their data families",
		"read",
		"Discover the supported AICard chart presets before choosing a visualization",
		"A chart preset is already chosen and its strict business-data shape is known",
		"dws aicard chart list --format json",
	)
	explain := &cobra.Command{Use: "explain <id-or-alias>", Short: "Explain one chart preset and its business-data shape", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		p, err := chartgen.Lookup(args[0])
		if err != nil {
			return aicardFailure(cmd, "validation", err.Error(), nil)
		}
		return output.StoreResult(cmd.Context(), output.Success(map[string]any{"preset": chartRecord(p), "specVersion": "1.0", "chartSpecExample": map[string]any{"specVersion": "1.0", "presetId": p.ID, "title": "Business title", "data": chartExample(p)}, "rendererVerified": false}))
	}}
	chartLeaf(
		explain,
		"explain",
		"Explain a chart preset's strict input family and limits",
		"read",
		"Inspect the strict business-data shape for one known AICard chart preset or alias",
		"List every preset or build the final chart payload instead of inspecting one preset",
		"dws aicard chart explain G01 --format json",
	)
	build := &cobra.Command{Use: "build", Short: "Validate facts and write a complete A2UI card", Args: cobra.NoArgs, RunE: runAicardChartBuild}
	build.Flags().String("file", "", "Internal ChartSpec JSON path, or - for stdin")
	build.Flags().String("out", "", "New A2UI message-array file, required unless --dry-run; also writes .artifact.json for FINISH")
	build.Flags().Bool("dry-run", false, "Validate and report without writing files or sending")
	runtimeannotate.AnnotateRuntimeRequiredFlags(build, "file")
	runtimeannotate.AnnotateRuntimeFlagRequiredValue(build, "out", false)
	runtimeannotate.AnnotateRuntimeFlagRequiredWhen(build, "out", "--dry-run is false")
	chartLeaf(
		build,
		"build",
		"Build an offline chart card and saved FINISH artifact",
		"write",
		"Build an offline AICard chart payload from a complete internal ChartSpec after preset selection",
		"Only discover presets or inspect one preset's input shape without building a payload",
		"dws aicard chart build --file ./chart-spec.json --out ./chart-card.json --format json",
	)
	root.AddCommand(list, explain, build)
	return root
}

func chartWriteNew(path string, data []byte) error {
	f, err := aicardChartOpenNew(path)
	if err != nil {
		return err
	}
	if _, err = f.Write(data); err != nil {
		_ = f.Close()
		_ = aicardChartRemove(path)
		return err
	}
	if err = f.Close(); err != nil {
		_ = aicardChartRemove(path)
		return err
	}
	return nil
}

func runAicardChartBuild(cmd *cobra.Command, _ []string) error {
	file, _ := cmd.Flags().GetString("file")
	out, _ := cmd.Flags().GetString("out")
	dry, _ := cmd.Flags().GetBool("dry-run")
	if strings.TrimSpace(file) == "" || !dry && strings.TrimSpace(out) == "" || out == "-" {
		return aicardFailure(cmd, "validation", "--file is required; --out is required unless --dry-run and cannot be -", nil)
	}
	var reader io.Reader
	if file == "-" {
		reader = aicardChartStdin
	} else {
		f, err := aicardChartOpenInput(file)
		if err != nil {
			return aicardFailure(cmd, "validation", err.Error(), nil)
		}
		defer f.Close()
		reader = f
	}
	raw, err := aicardChartReadAll(io.LimitReader(reader, 256*1024+1))
	if err != nil {
		return aicardFailure(cmd, "validation", err.Error(), nil)
	}
	artifact, err := chartgen.Build(raw, "")
	if err != nil {
		return aicardFailure(cmd, "validation", err.Error(), nil)
	}
	protocol, err := aicardLoadProtocol()
	if err != nil {
		return aicardFailure(cmd, "internal", err.Error(), nil)
	}
	card, err := aicardChartMarshalIndent(artifact.CreateMessages, "", "  ")
	if err != nil {
		return aicardFailure(cmd, "internal", err.Error(), nil)
	}
	report, err := aicardLint(protocol, card, false, true)
	if err != nil {
		return aicardFailure(cmd, "internal", err.Error(), nil)
	}
	if !report.Valid {
		return aicardReport(cmd, report)
	}
	preflight, err := aicardPreflight(protocol, report.A2UIMessages, "new-card")
	if err != nil {
		return aicardFailure(cmd, "validation", err.Error(), nil)
	}
	if preflight["valid"] != true {
		return aicardFailure(cmd, "validation", "Generated chart failed offline preflight", preflight)
	}
	result := map[string]any{"presetId": artifact.PresetID, "presetAlias": artifact.PresetAlias, "surfaceId": artifact.SurfaceID, "summary": artifact.Summary, "preflight": preflight, "rendererVerified": false, "sent": false}
	if dry {
		result["written"] = false
		return output.StoreResult(cmd.Context(), output.Success(result, output.WithDryRun()))
	}
	artifactPath := out + ".artifact.json"
	if _, err := aicardChartStat(out); err == nil {
		return aicardFailure(cmd, "validation", "Output already exists: "+out, nil)
	} else if !os.IsNotExist(err) {
		return aicardFailure(cmd, "validation", err.Error(), nil)
	}
	if _, err := aicardChartStat(artifactPath); err == nil {
		return aicardFailure(cmd, "validation", "Artifact already exists: "+artifactPath, nil)
	} else if !os.IsNotExist(err) {
		return aicardFailure(cmd, "validation", err.Error(), nil)
	}
	artifactBytes, err := aicardChartMarshalIndent(artifact, "", "  ")
	if err != nil {
		return aicardFailure(cmd, "internal", err.Error(), nil)
	}
	if err := chartWriteNew(out, append(card, '\n')); err != nil {
		return aicardFailure(cmd, "validation", err.Error(), nil)
	}
	if err := chartWriteNew(artifactPath, append(artifactBytes, '\n')); err != nil {
		_ = aicardChartRemove(out)
		return aicardFailure(cmd, "validation", err.Error(), nil)
	}
	result["written"], result["cardPath"], result["artifactPath"] = true, out, artifactPath
	return output.StoreResult(cmd.Context(), output.Success(result))
}
