// Copyright 2026 Alibaba Group
// SPDX-License-Identifier: Apache-2.0

package aitable

import (
	"strings"
	"testing"
)

func TestCrossPlatformCoverageAITableListProjectionExplicitEmptyIsSuccessE2E(t *testing.T) {
	tests := []struct {
		name    string
		command string
		args    []string
		payload string
	}{
		{name: "base list", command: "+base-list", payload: `{"bases":[]}`},
		{name: "base search", command: "+base-search", args: []string{"--query", "none"}, payload: `{"data":{"bases":[]}}`},
		{name: "template search", command: "+template-search", payload: `{"data":{"templates":[]}}`},
		{name: "view get", command: "+view-get", args: []string{"--base-id", "base", "--table-id", "table"}, payload: `{"views":[]}`},
		{name: "form list", command: "+form-list", args: []string{"--base-id", "base", "--table-id", "table"}, payload: `{"data":{"formViews":[]}}`},
		{name: "workflow list", command: "+workflow-list", args: []string{"--base-id", "base"}, payload: `{"data":{"list":[]}}`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			caller := &upsertByKeyCaller{steps: []upsertByKeyStep{{text: test.payload}}}
			out, err := runAITableCompositeCLI(t, caller, test.command, test.args...)
			if err != nil || !strings.Contains(out, `"count": 0`) || len(caller.calls) != 1 {
				t.Fatalf("explicit empty list = output:%q err:%v calls:%#v", out, err, caller.calls)
			}
		})
	}
}

func TestCrossPlatformCoverageAITableListProjectionUnknownIsNotEmptySuccessE2E(t *testing.T) {
	commands := []struct {
		name    string
		command string
		args    []string
	}{
		{name: "base list", command: "+base-list"},
		{name: "base search", command: "+base-search", args: []string{"--query", "none"}},
		{name: "template search", command: "+template-search"},
		{name: "view get", command: "+view-get", args: []string{"--base-id", "base", "--table-id", "table"}},
		{name: "form list", command: "+form-list", args: []string{"--base-id", "base", "--table-id", "table"}},
		{name: "workflow list", command: "+workflow-list", args: []string{"--base-id", "base"}},
	}
	payloads := []struct {
		name string
		text string
	}{
		{name: "empty response", text: ""},
		{name: "missing collection", text: `{"success":true}`},
		{name: "wrong collection type", text: `{"data":{"list":{}}}`},
	}
	for _, command := range commands {
		for _, payload := range payloads {
			t.Run(command.name+"/"+payload.name, func(t *testing.T) {
				caller := &upsertByKeyCaller{steps: []upsertByKeyStep{{text: payload.text}}}
				out, err := runAITableCompositeCLI(t, caller, command.command, command.args...)
				if err == nil || out != "" || len(caller.calls) != 1 {
					t.Fatalf("unknown list result was accepted = output:%q err:%v calls:%#v", out, err, caller.calls)
				}
			})
		}
	}
}

func TestCrossPlatformCoverageAITableListProjectionMalformedItemIsNotSilentlyDroppedE2E(t *testing.T) {
	tests := []struct {
		name    string
		command string
		args    []string
		payload string
	}{
		{name: "base without id", command: "+base-list", payload: `{"bases":[{"baseName":"orphan"}]}`},
		{name: "template scalar", command: "+template-search", payload: `{"templates":["bad"]}`},
		{name: "view without id", command: "+view-get", args: []string{"--base-id", "base", "--table-id", "table"}, payload: `{"views":[{"viewName":"orphan"}]}`},
		{name: "form without id", command: "+form-list", args: []string{"--base-id", "base", "--table-id", "table"}, payload: `{"forms":[{"viewName":"orphan"}]}`},
		{name: "workflow without id", command: "+workflow-list", args: []string{"--base-id", "base"}, payload: `{"workflows":[{"name":"orphan"}]}`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			caller := &upsertByKeyCaller{steps: []upsertByKeyStep{{text: test.payload}}}
			out, err := runAITableCompositeCLI(t, caller, test.command, test.args...)
			if err == nil || out != "" || len(caller.calls) != 1 {
				t.Fatalf("malformed list item was accepted = output:%q err:%v calls:%#v", out, err, caller.calls)
			}
		})
	}
}
