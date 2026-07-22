// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

// Command cmd_schema_registry_smoke emits the complete public binary-smoke
// vector from EffectiveCommandRegistry. Policy scripts use this instead of
// deriving command identity or aliases from the generated Catalog.
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/app"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/cli"
)

var (
	newSmokeRoot                = app.NewSchemaSourceRootCommand
	buildEffectiveSmokeRegistry = cli.BuildEffectiveCommandRegistry
	bindEffectiveSmokeRegistry  = cli.BindEffectiveCommandRegistry
	buildSmokeRegistryData      = buildSmokeRegistry
	exitSmokeProcess            = os.Exit
)

type smokeVector struct {
	CanonicalPath  string   `json:"canonical_path"`
	PrimaryCLIPath string   `json:"primary_cli_path"`
	AliasCLIPaths  []string `json:"alias_cli_paths"`
}

type smokeRegistry struct {
	Version      int           `json:"version"`
	RegistryHash string        `json:"registry_hash"`
	Commands     []smokeVector `json:"commands"`
}

func main() {
	root := newSmokeRoot()
	effective, err := buildEffectiveSmokeRegistry(root)
	if err != nil {
		fail(fmt.Errorf("build effective CommandRegistry: %w", err))
	}
	if _, err := bindEffectiveSmokeRegistry(root, effective); err != nil {
		fail(fmt.Errorf("bind effective CommandRegistry: %w", err))
	}
	registry, err := buildSmokeRegistryData(effective)
	if err != nil {
		fail(err)
	}
	encoded, _ := json.Marshal(registry)
	_, _ = os.Stdout.Write(append(encoded, '\n'))
}

func buildSmokeRegistry(effective cli.EffectiveCommandRegistry) (smokeRegistry, error) {
	commands := make([]smokeVector, 0, len(effective.Commands))
	for _, command := range effective.Commands {
		if command.Visibility != cli.SchemaVisibilityPublic {
			continue
		}
		aliases := append([]string{}, command.Aliases...)
		sort.Strings(aliases)
		commands = append(commands, smokeVector{
			CanonicalPath:  command.CanonicalPath,
			PrimaryCLIPath: command.PrimaryCLIPath,
			AliasCLIPaths:  aliases,
		})
	}
	if len(commands) == 0 {
		return smokeRegistry{}, fmt.Errorf("effective CommandRegistry has no public commands")
	}
	sort.Slice(commands, func(i, j int) bool {
		return commands[i].CanonicalPath < commands[j].CanonicalPath
	})
	return smokeRegistry{
		Version:      1,
		RegistryHash: effective.SourceHash(),
		Commands:     commands,
	}, nil
}

func fail(err error) {
	_, _ = fmt.Fprintln(os.Stderr, "error:", err)
	exitSmokeProcess(1)
}
