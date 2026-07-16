// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package main

import (
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/cli"
)

func TestBuildSmokeRegistryIncludesEveryPublicPathDeterministically(t *testing.T) {
	registry := cli.EffectiveCommandRegistry{Commands: []cli.CommandSpec{
		{
			CanonicalPath:  "internal.hidden",
			PrimaryCLIPath: "internal hidden",
			Aliases:        []string{"internal old"},
			Visibility:     cli.SchemaVisibilityInternal,
		},
		{
			CanonicalPath:  "sample.run",
			PrimaryCLIPath: "sample run",
			Aliases:        []string{"sample old", "sample execute"},
			Visibility:     cli.SchemaVisibilityPublic,
		},
		{
			CanonicalPath:  "sample.no_alias",
			PrimaryCLIPath: "sample no-alias",
			Visibility:     cli.SchemaVisibilityPublic,
		},
	}}

	got, err := buildSmokeRegistry(registry)
	if err != nil {
		t.Fatalf("buildSmokeRegistry() error = %v", err)
	}
	if got.Version != 1 || got.RegistryHash != registry.SourceHash() || len(got.Commands) != 2 {
		t.Fatalf("buildSmokeRegistry() = %#v", got)
	}
	if command := got.Commands[0]; command.CanonicalPath != "sample.no_alias" || command.PrimaryCLIPath != "sample no-alias" || command.AliasCLIPaths == nil || len(command.AliasCLIPaths) != 0 {
		t.Fatalf("first public command = %#v", command)
	}
	if command := got.Commands[1]; command.CanonicalPath != "sample.run" || command.PrimaryCLIPath != "sample run" || len(command.AliasCLIPaths) != 2 || command.AliasCLIPaths[0] != "sample execute" || command.AliasCLIPaths[1] != "sample old" {
		t.Fatalf("second public command = %#v", command)
	}
}

func TestBuildSmokeRegistryRejectsRegistryWithoutPublicCommands(t *testing.T) {
	_, err := buildSmokeRegistry(cli.EffectiveCommandRegistry{Commands: []cli.CommandSpec{{
		CanonicalPath:  "sample.run",
		PrimaryCLIPath: "sample run",
		Visibility:     cli.SchemaVisibilityInternal,
	}}})
	if err == nil {
		t.Fatal("buildSmokeRegistry() error = nil")
	}
}
