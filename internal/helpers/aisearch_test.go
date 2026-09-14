// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.

package helpers

import (
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/runtimeannotate"
)

func TestAisearchPersonFlagsDoNotLeakIntoContentCommands(t *testing.T) {
	root := newAisearchCommand()
	person, _, err := root.Find([]string{"person"})
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"query", "dimension"} {
		flag := person.Flags().Lookup(name)
		if flag == nil || flag.Hidden {
			t.Fatalf("person --%s = %#v, want visible local flag", name, flag)
		}
	}
	if person.Flags().Lookup("query").Shorthand != "" {
		t.Fatal("new Primary --query must not acquire a shorthand")
	}
	keyword := person.Flags().Lookup("keyword")
	if keyword == nil || !keyword.Hidden || keyword.Shorthand != "w" {
		t.Fatalf("person --keyword = %#v, want hidden legacy flag with -w", keyword)
	}
	if got := keyword.Annotations[runtimeannotate.AnnotationFlagAliasOf]; len(got) != 1 || got[0] != "query" {
		t.Fatalf("person --keyword alias_of = %#v, want query", got)
	}
	if got := keyword.Annotations[runtimeannotate.AnnotationFlagAliasOrigin]; len(got) != 1 || got[0] != runtimeannotate.FlagAliasOriginCorecmdV1 {
		t.Fatalf("person --keyword alias_origin = %#v, want %q", got, runtimeannotate.FlagAliasOriginCorecmdV1)
	}
	for _, name := range []string{"name", "q", "text"} {
		flag := person.Flags().Lookup(name)
		if flag == nil || !flag.Hidden {
			t.Fatalf("person --%s = %#v, want preserved hidden guess flag", name, flag)
		}
		if got := flag.Annotations[runtimeannotate.AnnotationFlagAliasOf]; len(got) != 0 {
			t.Fatalf("person --%s unexpectedly gained alias_of metadata: %#v", name, got)
		}
	}

	for _, path := range []string{"enterprise", "behavior"} {
		cmd, _, err := root.Find([]string{path})
		if err != nil {
			t.Fatal(err)
		}
		if flag := cmd.Flags().Lookup("dimension"); flag != nil {
			t.Fatalf("%s unexpectedly accepts person-only --dimension", path)
		}
		for _, name := range []string{"keyword", "query"} {
			flag := cmd.Flags().Lookup(name)
			if flag == nil || !flag.Hidden {
				t.Fatalf("%s --%s = %#v, want hidden compatibility flag", path, name, flag)
			}
		}
	}
}

func TestAisearchPersonQueryPrimaryCompatibility(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{name: "root new", args: []string{"--query", "new"}, want: "new"},
		{name: "root old", args: []string{"--keyword", "old"}, want: "old"},
		{name: "root old shorthand", args: []string{"-w", "old-short"}, want: "old-short"},
		{name: "root both old wins", args: []string{"--query", "new", "--keyword", "old"}, want: "old"},
		{name: "person new", args: []string{"person", "--query", "new"}, want: "new"},
		{name: "person old", args: []string{"person", "--keyword", "old"}, want: "old"},
		{name: "person old shorthand", args: []string{"person", "-w", "old-short"}, want: "old-short"},
		{name: "person both old wins", args: []string{"person", "--query", "new", "--keyword", "old"}, want: "old"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			caller := &scriptedToolCaller{}
			installScriptedCaller(t, caller)
			if err := executeFilterCoverage(t, newAisearchCommand(), tc.args...); err != nil {
				t.Fatal(err)
			}
			if caller.tool != "enterprise_person_search" || caller.args["keyword"] != tc.want {
				t.Fatalf("call = %s %#v, want backend keyword %q", caller.tool, caller.args, tc.want)
			}
		})
	}

	installScriptedCaller(t, &scriptedToolCaller{})
	err := executeFilterCoverage(t, newAisearchCommand(), "person")
	if err == nil || !strings.Contains(err.Error(), "--query") || strings.Contains(err.Error(), "--keyword") {
		t.Fatalf("missing query error = %v, want only new Primary", err)
	}
}
