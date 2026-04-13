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

func statusStr(enabled bool) string {
	if enabled {
		return "enabled"
	}
	return "disabled"
}
