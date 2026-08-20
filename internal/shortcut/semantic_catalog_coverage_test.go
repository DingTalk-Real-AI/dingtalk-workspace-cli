// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package shortcut

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/helpers"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
)

func TestCrossPlatformCoverageSemanticCatalogRejectsInvalidRecords(t *testing.T) {
	cases := map[string]string{
		"json": `{`,
		"header": `{
			"version": 2,
			"service": "chat",
			"default_availability": "available",
			"shortcuts": {}
		}`,
		"command": `{
			"version": 1,
			"service": "chat",
			"default_availability": "available",
			"shortcuts": {
				"messages": {
					"disposition": "semantic_adapter",
					"semantic_delta": "reviewed",
					"risk": "read",
					"public": true,
					"reviewed": true
				}
			}
		}`,
		"review": `{
			"version": 1,
			"service": "chat",
			"default_availability": "available",
			"shortcuts": {
				"+messages": {
					"disposition": "semantic_adapter",
					"semantic_delta": "",
					"risk": "read",
					"public": true,
					"reviewed": false
				}
			}
		}`,
		"disposition": `{
			"version": 1,
			"service": "chat",
			"default_availability": "available",
			"shortcuts": {
				"+messages": {
					"disposition": "unknown",
					"semantic_delta": "reviewed",
					"risk": "read",
					"public": true,
					"reviewed": true
				}
			}
		}`,
		"risk": `{
			"version": 1,
			"service": "chat",
			"default_availability": "available",
			"shortcuts": {
				"+messages": {
					"disposition": "semantic_adapter",
					"semantic_delta": "reviewed",
					"risk": "unknown",
					"public": true,
					"reviewed": true
				}
			}
		}`,
		"availability": `{
			"version": 1,
			"service": "chat",
			"default_availability": "unknown",
			"shortcuts": {
				"+messages": {
					"disposition": "semantic_adapter",
					"semantic_delta": "reviewed",
					"risk": "read",
					"public": false,
					"reviewed": true
				}
			}
		}`,
		"alias": `{
			"version": 1,
			"service": "chat",
			"default_availability": "available",
			"shortcuts": {
				"+messages": {
					"disposition": "alias_internal",
					"semantic_delta": "reviewed",
					"risk": "read",
					"public": false,
					"reviewed": true
				}
			}
		}`,
		"public unavailable": `{
			"version": 1,
			"service": "chat",
			"default_availability": "available",
			"shortcuts": {
				"+messages": {
					"disposition": "semantic_adapter",
					"semantic_delta": "reviewed",
					"risk": "read",
					"availability": "unavailable",
					"public": true,
					"reviewed": true
				}
			}
		}`,
		"compatibility-visible public": `{
			"version": 1,
			"service": "chat",
			"default_availability": "available",
			"shortcuts": {
				"+messages": {
					"disposition": "semantic_adapter",
					"semantic_delta": "reviewed",
					"risk": "read",
					"public": true,
					"compatibility_visible": true,
					"reviewed": true
				}
			}
		}`,
		"compatibility-visible available": `{
			"version": 1,
			"service": "chat",
			"default_availability": "available",
			"shortcuts": {
				"+messages": {
					"disposition": "semantic_adapter",
					"semantic_delta": "reviewed",
					"risk": "read",
					"public": false,
					"compatibility_visible": true,
					"reviewed": true
				}
			}
		}`,
	}
	original := semanticCatalogJSON
	t.Cleanup(func() { semanticCatalogJSON = original })
	for name, payload := range cases {
		t.Run(name, func(t *testing.T) {
			semanticCatalogJSON = []byte(payload)
			defer func() {
				if recover() == nil {
					t.Fatal("invalid semantic catalog did not panic")
				}
			}()
			_ = mustLoadSemanticCatalog()
		})
	}
}

func TestCrossPlatformCoverageSemanticCatalogRejectsCrossSourceDuplicates(t *testing.T) {
	valid := []byte(`{
		"version": 1,
		"service": "duplicate-test",
		"default_availability": "available",
		"shortcuts": {
			"+same": {
				"disposition": "semantic_adapter",
				"semantic_delta": "reviewed",
				"risk": "read",
				"public": true,
				"reviewed": true
			}
		}
	}`)
	defer func() {
		if recover() == nil {
			t.Fatal("duplicate semantic records did not panic")
		}
	}()
	_ = mustLoadSemanticCatalogs(valid, valid)
}

func TestCrossPlatformCoveragePublicCatalogSemanticAndGeneratedLookups(t *testing.T) {
	if !InPublicCatalog("chat", "+messages-send") {
		t.Fatal("reviewed public semantic shortcut is missing")
	}
	if InPublicCatalog("chat", "+conversation-mute-at-all") {
		t.Fatal("reviewed unavailable semantic shortcut is public")
	}
	if InPublicCatalog("unknown", "+missing") {
		t.Fatal("unknown shortcut is public")
	}
}

func TestCrossPlatformCoverageCompatibilityVisibleStaysNonPublic(t *testing.T) {
	item, ok := applyReviewedSemanticCatalog(Shortcut{Service: "attendance", Command: "+get-summary"})
	if !ok || item.Hidden || !item.CompatibilityVisible || item.Availability != AvailabilityUnavailable {
		t.Fatalf("compatibility-visible semantic item = %#v, found=%v", item, ok)
	}
	if InPublicCatalog(item.Service, item.Command) {
		t.Fatal("compatibility-visible unavailable shortcut entered the public catalog")
	}
}

func TestCrossPlatformCoverageRuntimeReadDataBranches(t *testing.T) {
	caller := &runtimeReadCoverageCaller{}
	old := helpers.GetCaller()
	t.Cleanup(func() { helpers.InitDeps(old) })
	helpers.InitDeps(caller)
	rt := &RuntimeContext{}

	caller.text = ""
	if got, err := rt.callMCPReadData("im", "search_groups", nil); err != nil || len(got) != 0 {
		t.Fatalf("empty read = %#v, %v", got, err)
	}
	if caller.args == nil {
		t.Fatal("nil read parameters were not normalized")
	}

	caller.err = errors.New("read failed")
	if _, err := rt.callMCPReadData("im", "search_groups", nil); err == nil {
		t.Fatal("read error was swallowed")
	}
	caller.err = nil

	caller.text = `not-json`
	if _, err := rt.callMCPReadData("im", "search_groups", map[string]any{"keyword": "x"}); err == nil {
		t.Fatal("invalid read JSON was accepted")
	}

	caller.text = `{"result":{"groups":[]}}`
	got, err := rt.callMCPReadData("im", "search_groups", map[string]any{"keyword": "x"})
	if err != nil || !strings.Contains(caller.text, "groups") || got["result"] == nil {
		t.Fatalf("valid read = %#v, %v", got, err)
	}
}

type runtimeReadCoverageCaller struct {
	args map[string]any
	text string
	err  error
}

func (c *runtimeReadCoverageCaller) CallTool(
	_ context.Context,
	_ string,
	_ string,
	args map[string]any,
) (*edition.ToolResult, error) {
	c.args = args
	if c.err != nil {
		return nil, c.err
	}
	return &edition.ToolResult{
		Content: []edition.ContentBlock{{Type: "text", Text: c.text}},
	}, nil
}

func (c *runtimeReadCoverageCaller) Format() string { return "json" }
func (c *runtimeReadCoverageCaller) DryRun() bool   { return false }
func (c *runtimeReadCoverageCaller) Fields() string { return "" }
func (c *runtimeReadCoverageCaller) JQ() string     { return "" }
