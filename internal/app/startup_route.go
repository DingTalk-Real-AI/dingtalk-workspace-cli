// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package app

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

var (
	startupRouteLstat    = os.Lstat
	startupRouteReadDir  = os.ReadDir
	startupRouteReadFile = os.ReadFile
)

type startupRoute struct {
	topLevel  string
	selective bool
}

type runtimeCommandSurface struct {
	selectiveStartup    bool
	pluginsProvenAbsent bool
}

// inspectRuntimeCommandSurface is a conservative eligibility check. Any
// installed user plugin, dev-plugin registration, unreadable state or edition
// command hook keeps the complete-tree path because those extensions may add
// commands or PreParse rewrites before Cobra dispatches.
func inspectRuntimeCommandSurface() runtimeCommandSurface {
	if hooks := edition.Get(); hooks == nil || hooks.RegisterExtraCommands != nil {
		return runtimeCommandSurface{}
	}
	configDir := defaultConfigDir()
	// User shortcuts are loaded for every normal invocation and own startup
	// diagnostics even when the selected command is a utility such as Schema.
	// Keep the complete-tree path whenever the directory exists so selective
	// construction cannot suppress a malformed shortcut warning or race a
	// shortcut being installed during startup.
	if _, err := startupRouteLstat(filepath.Join(configDir, "shortcuts")); err == nil || !os.IsNotExist(err) {
		return runtimeCommandSurface{}
	}
	entries, err := startupRouteReadDir(filepath.Join(configDir, "plugins", "user"))
	if err != nil && !os.IsNotExist(err) {
		return runtimeCommandSurface{}
	}
	if len(entries) > 0 {
		return runtimeCommandSurface{}
	}
	data, err := startupRouteReadFile(filepath.Join(configDir, "settings.json"))
	if os.IsNotExist(err) {
		return runtimeCommandSurface{selectiveStartup: true, pluginsProvenAbsent: true}
	}
	if err != nil {
		return runtimeCommandSurface{}
	}
	var settings struct {
		DevPlugins map[string]string `json:"devPlugins"`
	}
	if json.Unmarshal(data, &settings) != nil || len(settings.DevPlugins) != 0 {
		return runtimeCommandSurface{}
	}
	return runtimeCommandSurface{selectiveStartup: true, pluginsProvenAbsent: true}
}

// resolveStartupRoute reads only root flags that appear before the command.
// Unknown syntax deliberately falls back to the complete tree so Cobra,
// PreParse, extensions, aliases and completion retain their existing behavior.
func resolveStartupRoute(root *cobra.Command, args []string) startupRoute {
	if root == nil || len(args) == 0 {
		return startupRoute{}
	}
	version := false
	for index := 0; index < len(args); index++ {
		arg := strings.TrimSpace(args[index])
		if arg == "" || arg == "-" || arg == "--" || arg == "--help" || arg == "-h" {
			return startupRoute{}
		}
		if !strings.HasPrefix(arg, "-") {
			if commandNeedsCompleteTree(arg) {
				return startupRoute{}
			}
			return startupRoute{topLevel: arg, selective: true}
		}
		if strings.HasPrefix(arg, "--") {
			name, _, attached := strings.Cut(strings.TrimPrefix(arg, "--"), "=")
			flag := rootFlag(root, name)
			if flag == nil {
				return startupRoute{}
			}
			if name == "version" {
				version = true
			}
			if !attached && flag.NoOptDefVal == "" {
				index++
				if index >= len(args) {
					return startupRoute{}
				}
			}
			continue
		}

		short := strings.TrimPrefix(arg, "-")
		for offset := 0; offset < len(short); offset++ {
			flag := rootShorthandFlag(root, string(short[offset]))
			if flag == nil {
				return startupRoute{}
			}
			if flag.NoOptDefVal != "" {
				continue
			}
			if offset+1 == len(short) {
				index++
				if index >= len(args) {
					return startupRoute{}
				}
			}
			break
		}
	}
	if version {
		return startupRoute{selective: true}
	}
	return startupRoute{}
}

func commandNeedsCompleteTree(name string) bool {
	switch name {
	case "completion", cobra.ShellCompRequestCmd, cobra.ShellCompNoDescRequestCmd:
		return true
	default:
		return false
	}
}

func rootFlag(root *cobra.Command, name string) *pflag.Flag {
	if flag := root.Flags().Lookup(name); flag != nil {
		return flag
	}
	return root.PersistentFlags().Lookup(name)
}

func rootShorthandFlag(root *cobra.Command, shorthand string) *pflag.Flag {
	if flag := root.Flags().ShorthandLookup(shorthand); flag != nil {
		return flag
	}
	return root.PersistentFlags().ShorthandLookup(shorthand)
}

func commandMatchesTopLevel(command *cobra.Command, name string) bool {
	if command == nil {
		return false
	}
	if command.Name() == name {
		return true
	}
	for _, alias := range command.Aliases {
		if alias == name {
			return true
		}
	}
	return false
}
