// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package main

import (
	"bytes"
	"context"
	"errors"
	"os"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/cli"
	"github.com/spf13/cobra"
)

func TestCrossPlatformCoverageMainWritesProductionSmokeRegistry(t *testing.T) {
	outputFile, err := os.CreateTemp(t.TempDir(), "schema-registry-smoke-*.json")
	if err != nil {
		t.Fatal(err)
	}
	oldStdout := os.Stdout
	os.Stdout = outputFile
	t.Cleanup(func() {
		os.Stdout = oldStdout
		_ = outputFile.Close()
	})
	main()
	os.Stdout = oldStdout
	if _, err := outputFile.Seek(0, 0); err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	if _, err := output.ReadFrom(outputFile); err != nil {
		t.Fatal(err)
	}
	if output.Len() == 0 {
		t.Fatal("main wrote an empty smoke registry")
	}
}

func TestCrossPlatformCoverageMainReportsSmokeRegistryFailures(t *testing.T) {
	originalRoot := newSmokeRoot
	originalBuild := buildEffectiveSmokeRegistry
	originalBind := bindEffectiveSmokeRegistry
	originalRegistry := buildSmokeRegistryData
	originalExit := exitSmokeProcess
	t.Cleanup(func() {
		newSmokeRoot = originalRoot
		buildEffectiveSmokeRegistry = originalBuild
		bindEffectiveSmokeRegistry = originalBind
		buildSmokeRegistryData = originalRegistry
		exitSmokeProcess = originalExit
	})
	exitSmokeProcess = func(int) { panic("exit") }
	newSmokeRoot = func(...context.Context) *cobra.Command { return &cobra.Command{Use: "dws"} }

	tests := []struct {
		name  string
		setup func()
	}{
		{name: "build", setup: func() {
			buildEffectiveSmokeRegistry = func(*cobra.Command) (cli.EffectiveCommandRegistry, error) {
				return cli.EffectiveCommandRegistry{}, errors.New("build")
			}
		}},
		{name: "bind", setup: func() {
			buildEffectiveSmokeRegistry = func(*cobra.Command) (cli.EffectiveCommandRegistry, error) {
				return cli.EffectiveCommandRegistry{}, nil
			}
			bindEffectiveSmokeRegistry = func(*cobra.Command, cli.EffectiveCommandRegistry) (cli.BoundCommandRegistry, error) {
				return cli.BoundCommandRegistry{}, errors.New("bind")
			}
		}},
		{name: "registry", setup: func() {
			buildEffectiveSmokeRegistry = func(*cobra.Command) (cli.EffectiveCommandRegistry, error) {
				return cli.EffectiveCommandRegistry{}, nil
			}
			bindEffectiveSmokeRegistry = func(*cobra.Command, cli.EffectiveCommandRegistry) (cli.BoundCommandRegistry, error) {
				return cli.BoundCommandRegistry{}, nil
			}
			buildSmokeRegistryData = func(cli.EffectiveCommandRegistry) (smokeRegistry, error) {
				return smokeRegistry{}, errors.New("registry")
			}
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			buildEffectiveSmokeRegistry = originalBuild
			bindEffectiveSmokeRegistry = originalBind
			buildSmokeRegistryData = originalRegistry
			test.setup()
			defer func() {
				if recover() != "exit" {
					t.Fatal("main did not exit")
				}
			}()
			main()
		})
	}
}

func TestCrossPlatformCoverageBuildSmokeRegistryIncludesEveryPublicPathDeterministically(t *testing.T) {
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

func TestCrossPlatformCoverageBuildSmokeRegistryRejectsRegistryWithoutPublicCommands(t *testing.T) {
	_, err := buildSmokeRegistry(cli.EffectiveCommandRegistry{Commands: []cli.CommandSpec{{
		CanonicalPath:  "sample.run",
		PrimaryCLIPath: "sample run",
		Visibility:     cli.SchemaVisibilityInternal,
	}}})
	if err == nil {
		t.Fatal("buildSmokeRegistry() error = nil")
	}
}
