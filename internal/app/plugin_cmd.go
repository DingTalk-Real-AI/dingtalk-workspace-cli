// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package app

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/plugin"
	"github.com/spf13/cobra"
)

func newPluginCommand() *cobra.Command {
	pluginCmd := newPlaceholderParent("plugin", "Manage plugins")

	pluginCmd.AddCommand(
		newPluginListCommand(),
		newPluginInstallCommand(),
		newPluginInfoCommand(),
		newPluginEnableCommand(),
		newPluginDisableCommand(),
		newPluginRemoveCommand(),
		newPluginValidateCommand(),
		newPluginCreateCommand(),
		newPluginDevCommand(),
		newPluginConfigCommand(),
		newPluginBuildCommand(),
	)

	return pluginCmd
}

func newPluginListCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:               "list",
		Short:             "List installed plugins",
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			loader := plugin.NewLoader(RawVersion())
			plugins := loader.ListInstalled()

			wantJSON, _ := cmd.Flags().GetBool("json")
			if wantJSON {
				return output.WriteJSON(cmd.OutOrStdout(), plugins)
			}

			if len(plugins) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "No plugins installed.")
				return nil
			}

			w := cmd.OutOrStdout()
			fmt.Fprintf(w, "%-35s %-12s %-10s %-10s %s\n",
				"NAME", "VERSION", "TYPE", "STATUS", "DESCRIPTION")
			fmt.Fprintln(w, strings.Repeat("-", 85))
			for _, p := range plugins {
				fmt.Fprintf(w, "%-35s %-12s %-10s %-10s %s\n",
					p.Name, p.Version, p.Type, statusStr(p.Enabled), p.Description)
			}
			return nil
		},
	}
	cmd.Flags().Bool("json", false, "Output in JSON format")
	return cmd
}

func newPluginInstallCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "install",
		Short: "Install a plugin",
		Example: `  dws plugin install --dir ./conference
  dws plugin install --git https://github.com/DingTalk-Real-AI/conference.git`,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			dirPath, _ := cmd.Flags().GetString("dir")
			gitURL, _ := cmd.Flags().GetString("git")

			if dirPath == "" && gitURL == "" {
				return apperrors.NewValidation("specify install source: --dir <path> or --git <url>")
			}

			loader := plugin.NewLoader(RawVersion())

			if gitURL != "" {
				p, err := loader.InstallFromGit(gitURL)
				if err != nil {
					return apperrors.NewInternal(fmt.Sprintf("install failed: %v", err))
				}
				fmt.Fprintf(cmd.OutOrStdout(), "Installed %s (%s)\n", p.Manifest.Name, p.Manifest.Version)
				return nil
			}

			p, err := loader.InstallFromDir(dirPath)
			if err != nil {
				return apperrors.NewInternal(fmt.Sprintf("install failed: %v", err))
			}

			fmt.Fprintf(cmd.OutOrStdout(), "Installed %s (%s)\n", p.Manifest.Name, p.Manifest.Version)
			return nil
		},
	}
	cmd.Flags().String("dir", "", "Install from a local directory")
	cmd.Flags().String("git", "", "Install from a Git repository")
	return cmd
}

func newPluginInfoCommand() *cobra.Command {
	return &cobra.Command{
		Use:               "info <name>",
		Short:             "Show plugin details",
		Args:              cobra.ExactArgs(1),
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			loader := plugin.NewLoader(RawVersion())
			plugins := loader.ListInstalled()

			for _, p := range plugins {
				if p.Name == name {
					w := cmd.OutOrStdout()
					fmt.Fprintf(w, "Name:         %s\n", p.Name)
					fmt.Fprintf(w, "Version:      %s\n", p.Version)
					fmt.Fprintf(w, "Type:         %s\n", p.Type)
					fmt.Fprintf(w, "Status:       %s\n", statusStr(p.Enabled))
					fmt.Fprintf(w, "Path:         %s\n", p.Path)
					if p.Description != "" {
						fmt.Fprintf(w, "Description:  %s\n", p.Description)
					}
					return nil
				}
			}
			return apperrors.NewValidation(fmt.Sprintf("plugin %q not found", name))
		},
	}
}

func newPluginEnableCommand() *cobra.Command {
	return &cobra.Command{
		Use:               "enable <name>",
		Short:             "Enable a plugin",
		Args:              cobra.ExactArgs(1),
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			loader := plugin.NewLoader(RawVersion())
			if err := loader.SetEnabled(args[0], true); err != nil {
				return apperrors.NewValidation(err.Error())
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Plugin %s enabled.\n", args[0])
			return nil
		},
	}
}

func newPluginDisableCommand() *cobra.Command {
	return &cobra.Command{
		Use:               "disable <name>",
		Short:             "Disable a plugin (managed plugins can be disabled but not removed)",
		Args:              cobra.ExactArgs(1),
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			loader := plugin.NewLoader(RawVersion())
			if err := loader.SetEnabled(args[0], false); err != nil {
				return apperrors.NewValidation(err.Error())
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Plugin %s disabled.\n", args[0])
			return nil
		},
	}
}

func newPluginRemoveCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:               "remove <name>",
		Short:             "Remove a user plugin (managed plugins cannot be removed)",
		Args:              cobra.ExactArgs(1),
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			keepData, _ := cmd.Flags().GetBool("keep-data")
			loader := plugin.NewLoader(RawVersion())
			if err := loader.RemovePlugin(args[0], keepData); err != nil {
				return apperrors.NewValidation(err.Error())
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Plugin %s removed.\n", args[0])
			return nil
		},
	}
	cmd.Flags().Bool("keep-data", false, "Keep plugin data directory")
	return cmd
}

func newPluginValidateCommand() *cobra.Command {
	return &cobra.Command{
		Use:               "validate <dir>",
		Short:             "Validate a plugin.json",
		Args:              cobra.ExactArgs(1),
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			dir := args[0]
			m, err := plugin.ParseManifest(dir + "/plugin.json")
			if err != nil {
				return apperrors.NewValidation(fmt.Sprintf("parse failed: %v", err))
			}
			if err := m.Validate(RawVersion()); err != nil {
				return apperrors.NewValidation(fmt.Sprintf("validation failed: %v", err))
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Valid: %s (%s)\n", m.Name, m.Version)
			return nil
		},
	}
}

func newPluginCreateCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "create <name>",
		Short: "Scaffold a new plugin directory",
		Example: `  dws plugin create my-tool
  dws plugin create my-tool --type managed --description "My awesome tool"`,
		Args:              cobra.ExactArgs(1),
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			desc, _ := cmd.Flags().GetString("description")
			pluginType, _ := cmd.Flags().GetString("type")

			if pluginType == "" {
				pluginType = "user"
			}
			if pluginType != "managed" && pluginType != "user" {
				return apperrors.NewValidation("type must be 'managed' or 'user'")
			}

			// Validate name format
			m := &plugin.Manifest{Name: name, Version: "0.1.0", Type: pluginType}
			if err := m.Validate(""); err != nil {
				return apperrors.NewValidation(fmt.Sprintf("invalid plugin name: %v", err))
			}

			dir := filepath.Join(".", name)
			if _, err := os.Stat(dir); err == nil {
				return apperrors.NewValidation(fmt.Sprintf("directory %q already exists", dir))
			}

			// Create directory structure
			dirs := []string{
				dir,
				filepath.Join(dir, "skills", name),
				filepath.Join(dir, "hooks"),
			}
			for _, d := range dirs {
				if err := os.MkdirAll(d, 0o755); err != nil {
					return apperrors.NewInternal(fmt.Sprintf("failed to create directory: %v", err))
				}
			}

			// Write plugin.json
			pluginJSON := fmt.Sprintf(`{
  "name": %q,
  "version": "0.1.0",
  "description": %q,
  "type": %q,
  "minCLIVersion": %q,
  "mcpServers": {
    %q: {
      "type": "stdio",
      "command": "${DWS_PLUGIN_ROOT}/bin/server",
      "args": []
    }
  },
  "build": {
    "command": "echo 'TODO: replace with your build command, e.g.: bun build --compile src/server.ts --outfile bin/server'",
    "output": "bin/server"
  },
  "skills": "./skills/",
  "hooks": "./hooks/hooks.json"
}
`, name, desc, pluginType, RawVersion(), name)

			if err := os.WriteFile(filepath.Join(dir, "plugin.json"), []byte(pluginJSON), 0o644); err != nil {
				return apperrors.NewInternal(fmt.Sprintf("failed to write plugin.json: %v", err))
			}

			// Write SKILL.md template
			skillMD := fmt.Sprintf(`---
name: %s
description: %s
cli_version: ">=%s"
---

# %s

## Intent Recognition

Use this skill when the user mentions:
- TODO: add your intent keywords here

## Command Decision Tree

| User Intent | Command | Required Parameters |
|-------------|---------|---------------------|
| TODO | `+"`dws %s <sub-command>`"+` | `+"`--param`"+` |

## Parameter Rules

### TODO: parameter type
- Format description
- Conversion rules
`, name, desc, RawVersion(), name, name)

			if err := os.WriteFile(filepath.Join(dir, "skills", name, "SKILL.md"), []byte(skillMD), 0o644); err != nil {
				return apperrors.NewInternal(fmt.Sprintf("failed to write SKILL.md: %v", err))
			}

			// Write hooks.json template
			hooksJSON := `{
  "hooks": []
}
`
			if err := os.WriteFile(filepath.Join(dir, "hooks", "hooks.json"), []byte(hooksJSON), 0o644); err != nil {
				return apperrors.NewInternal(fmt.Sprintf("failed to write hooks.json: %v", err))
			}

			w := cmd.OutOrStdout()
			fmt.Fprintf(w, "Created plugin scaffold at ./%s/\n", name)
			fmt.Fprintf(w, "  %s/\n", name)
			fmt.Fprintf(w, "  ├── plugin.json\n")
			fmt.Fprintf(w, "  ├── skills/%s/SKILL.md\n", name)
			fmt.Fprintf(w, "  └── hooks/hooks.json\n")
			fmt.Fprintln(w)
			fmt.Fprintf(w, "Next steps:\n")
			fmt.Fprintf(w, "  1. Edit plugin.json to configure your MCP servers\n")
			fmt.Fprintf(w, "  2. Edit skills/%s/SKILL.md to describe your commands\n", name)
			fmt.Fprintf(w, "  3. Run: dws plugin validate ./%s\n", name)
			fmt.Fprintf(w, "  4. Run: dws plugin dev ./%s\n", name)
			return nil
		},
	}
	cmd.Flags().String("description", "", "Plugin description")
	cmd.Flags().String("type", "user", "Plugin type: managed or user")
	return cmd
}

func newPluginDevCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "dev <dir>",
		Short: "Register a local directory as a dev plugin",
		Long: `Registers a plugin from a local source directory for development.
The plugin is loaded directly from the source directory on next CLI invocation,
without copying files to ~/.dws/plugins/. Use 'dws plugin dev --off <name>'
to unregister.`,
		Example: `  dws plugin dev ./my-tool
  dws plugin dev --off my-tool`,
		Args:              cobra.ExactArgs(1),
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			off, _ := cmd.Flags().GetBool("off")
			loader := plugin.NewLoader(RawVersion())

			if off {
				// Unregister dev plugin
				name := args[0]
				if err := loader.UnregisterDevPlugin(name); err != nil {
					return apperrors.NewValidation(err.Error())
				}
				fmt.Fprintf(cmd.OutOrStdout(), "Dev plugin %q unregistered.\n", name)
				return nil
			}

			// Register dev plugin
			dir := args[0]
			absDir, err := filepath.Abs(dir)
			if err != nil {
				return apperrors.NewValidation(fmt.Sprintf("invalid path: %v", err))
			}

			// Validate the plugin first
			m, err := plugin.ParseManifest(filepath.Join(absDir, "plugin.json"))
			if err != nil {
				return apperrors.NewValidation(fmt.Sprintf("invalid plugin at %s: %v", dir, err))
			}
			if err := m.Validate(RawVersion()); err != nil {
				return apperrors.NewValidation(fmt.Sprintf("validation failed: %v", err))
			}

			if err := loader.RegisterDevPlugin(m.Name, absDir); err != nil {
				return apperrors.NewInternal(fmt.Sprintf("failed to register: %v", err))
			}

			fmt.Fprintf(cmd.OutOrStdout(), "Dev plugin %q registered from %s\n", m.Name, absDir)
			fmt.Fprintf(cmd.OutOrStdout(), "It will be loaded on next dws invocation.\n")
			fmt.Fprintf(cmd.OutOrStdout(), "To unregister: dws plugin dev --off %s\n", m.Name)
			return nil
		},
	}
	cmd.Flags().Bool("off", false, "Unregister a dev plugin")
	return cmd
}

func newPluginConfigCommand() *cobra.Command {
	configCmd := newPlaceholderParent("config", "Manage plugin configuration")
	configCmd.AddCommand(
		newPluginConfigSetCommand(),
		newPluginConfigGetCommand(),
		newPluginConfigListCommand(),
		newPluginConfigUnsetCommand(),
	)
	return configCmd
}

func newPluginConfigSetCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "set <plugin-name> <key> <value>",
		Short: "Set a plugin config value",
		Long: `Persistently set a configuration value for a plugin.
The value is stored in ~/.dws/settings.json and automatically injected
as an environment variable when the plugin is loaded.

Environment variables set by the user (e.g. via export) take precedence
over values stored in settings.json.`,
		Example: `  dws plugin config set demo-devtool DASHSCOPE_API_KEY sk-xxx
  dws plugin config set my-plugin API_ENDPOINT https://api.example.com`,
		Args:              cobra.ExactArgs(3),
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			pluginName, key, value := args[0], args[1], args[2]
			loader := plugin.NewLoader(RawVersion())

			// Validate that the plugin exists.
			plugins := loader.ListInstalled()
			found := false
			for _, p := range plugins {
				if p.Name == pluginName {
					found = true
					break
				}
			}
			if !found {
				return apperrors.NewValidation(fmt.Sprintf("plugin %q not found; use 'dws plugin list' to see installed plugins", pluginName))
			}

			loader.SetPluginConfig(pluginName, key, value)
			fmt.Fprintf(cmd.OutOrStdout(), "Config saved: %s.%s\n", pluginName, key)
			return nil
		},
	}
}

func newPluginConfigGetCommand() *cobra.Command {
	return &cobra.Command{
		Use:               "get <plugin-name> <key>",
		Short:             "Get a plugin config value",
		Args:              cobra.ExactArgs(2),
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			pluginName, key := args[0], args[1]
			loader := plugin.NewLoader(RawVersion())

			val, ok := loader.GetPluginConfig(pluginName, key)
			if !ok {
				return apperrors.NewValidation(fmt.Sprintf("config key %q not set for plugin %q", key, pluginName))
			}

			fmt.Fprintln(cmd.OutOrStdout(), val)
			return nil
		},
	}
}

func newPluginConfigListCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:               "list <plugin-name>",
		Short:             "List all config values for a plugin",
		Args:              cobra.ExactArgs(1),
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			pluginName := args[0]
			loader := plugin.NewLoader(RawVersion())

			wantJSON, _ := cmd.Flags().GetBool("json")
			configs := loader.ListPluginConfig(pluginName)

			// Also load the plugin manifest to show declared userConfig keys.
			declaredKeys := loadDeclaredUserConfig(loader, pluginName)

			if wantJSON {
				result := make(map[string]any)
				for k, v := range configs {
					sensitive := false
					if ci, ok := declaredKeys[k]; ok {
						sensitive = ci.Sensitive
					}
					if sensitive {
						result[k] = maskSensitiveValue(v)
					} else {
						result[k] = v
					}
				}
				// Include declared but unset keys.
				for k, ci := range declaredKeys {
					if _, set := configs[k]; !set {
						entry := map[string]any{
							"value":       nil,
							"description": ci.Description,
							"required":    ci.Default == "",
						}
						result[k] = entry
					}
				}
				return output.WriteJSON(cmd.OutOrStdout(), map[string]any{
					"kind":   "plugin_config",
					"plugin": pluginName,
					"config": result,
				})
			}

			w := cmd.OutOrStdout()
			if len(configs) == 0 && len(declaredKeys) == 0 {
				fmt.Fprintf(w, "No configuration for plugin %q.\n", pluginName)
				return nil
			}

			fmt.Fprintf(w, "Configuration for %s:\n\n", pluginName)

			// Show set values.
			for k, v := range configs {
				sensitive := false
				if ci, ok := declaredKeys[k]; ok {
					sensitive = ci.Sensitive
				}
				displayVal := v
				if sensitive {
					displayVal = maskSensitiveValue(v)
				}
				fmt.Fprintf(w, "  %s = %s\n", k, displayVal)
			}

			// Show declared but unset keys.
			for k, ci := range declaredKeys {
				if _, set := configs[k]; !set {
					desc := ""
					if ci.Description != "" {
						desc = "  # " + ci.Description
					}
					fmt.Fprintf(w, "  %s = (not set)%s\n", k, desc)
				}
			}
			return nil
		},
	}
	cmd.Flags().Bool("json", false, "Output in JSON format")
	return cmd
}

func newPluginConfigUnsetCommand() *cobra.Command {
	return &cobra.Command{
		Use:               "unset <plugin-name> <key>",
		Short:             "Remove a plugin config value",
		Args:              cobra.ExactArgs(2),
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			pluginName, key := args[0], args[1]
			loader := plugin.NewLoader(RawVersion())

			if !loader.UnsetPluginConfig(pluginName, key) {
				return apperrors.NewValidation(fmt.Sprintf("config key %q not set for plugin %q", key, pluginName))
			}

			fmt.Fprintf(cmd.OutOrStdout(), "Config removed: %s.%s\n", pluginName, key)
			return nil
		},
	}
}

// loadDeclaredUserConfig loads the userConfig section from a plugin's manifest.
func loadDeclaredUserConfig(loader *plugin.Loader, pluginName string) map[string]plugin.ConfigItem {
	plugins := loader.ListInstalled()
	for _, p := range plugins {
		if p.Name == pluginName {
			m, err := plugin.ParseManifest(filepath.Join(p.Path, "plugin.json"))
			if err != nil {
				return nil
			}
			return m.UserConfig
		}
	}
	return nil
}

// maskSensitiveValue masks a sensitive value, showing only the first 4
// and last 2 characters for values longer than 8 characters.
func maskSensitiveValue(value string) string {
	if len(value) <= 8 {
		return strings.Repeat("*", len(value))
	}
	return value[:4] + strings.Repeat("*", len(value)-6) + value[len(value)-2:]
}

func newPluginBuildCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "build <dir>",
		Short: "Build plugin's stdio server into a native binary",
		Long: `Runs the build command declared in plugin.json to compile the
plugin's server into a single executable. This ensures plugin users
don't need any language runtime (Node.js, Python, etc.) installed.

The build configuration is read from the "build" field in plugin.json:

  {
    "build": {
      "command": "bun build --compile src/server.ts --outfile bin/server",
      "output": "bin/server"
    }
  }`,
		Example: `  dws plugin build ./my-plugin
  dws plugin build .`,
		Args:              cobra.ExactArgs(1),
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			dir := args[0]
			absDir, err := filepath.Abs(dir)
			if err != nil {
				return apperrors.NewValidation(fmt.Sprintf("invalid path: %v", err))
			}

			m, err := plugin.ParseManifest(filepath.Join(absDir, "plugin.json"))
			if err != nil {
				return apperrors.NewValidation(fmt.Sprintf("invalid plugin at %s: %v", dir, err))
			}

			if m.Build == nil {
				return apperrors.NewValidation(fmt.Sprintf(
					"plugin %q has no \"build\" field in plugin.json.\n"+
						"Add a build config, e.g.:\n\n"+
						"  \"build\": {\n"+
						"    \"command\": \"bun build --compile src/server.js --outfile bin/server\",\n"+
						"    \"output\": \"bin/server\"\n"+
						"  }", m.Name))
			}

			if err := plugin.BuildPlugin(absDir); err != nil {
				return apperrors.NewInternal(err.Error())
			}

			fmt.Fprintf(cmd.OutOrStdout(), "Build succeeded: %s\n", m.Build.Output)
			return nil
		},
	}
}

func statusStr(enabled bool) string {
	if enabled {
		return "enabled"
	}
	return "disabled"
}
