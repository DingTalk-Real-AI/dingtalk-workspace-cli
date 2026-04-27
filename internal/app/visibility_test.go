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
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/market"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
	"github.com/spf13/cobra"
)

// overrideVisibleProducts temporarily installs an edition hook exposing the
// given static product list and restores the previous hooks on cleanup.
func overrideVisibleProducts(t *testing.T, products []string) {
	t.Helper()
	prev := edition.Get()
	edition.Override(&edition.Hooks{
		VisibleProducts: func() []string { return products },
	})
	t.Cleanup(func() { edition.Override(prev) })
}

// registerPluginProduct simulates a plugin's `AppendDynamicServer` call so
// the product ID ends up in DirectRuntimeProductIDs() without triggering
// network discovery.
func registerPluginProduct(t *testing.T, id, endpoint string) {
	t.Helper()
	AppendDynamicServer(market.ServerDescriptor{
		Endpoint: endpoint,
		CLI: market.CLIOverlay{
			ID:      id,
			Command: id,
		},
	})
}

// TestHideNonDirectRuntimeCommands_PluginVisibleDespiteStaticVisibleProducts
// is a regression for the dws-wukong plugin-visibility bug: when an edition
// installs a static VisibleProducts hook (Wukong returns 40 hardcoded product
// IDs) and a plugin registers a new product via AppendDynamicServer
// (e.g. `conference-local`), the plugin command must stay visible because the
// dynamic registry takes precedence over the hook's static whitelist.
func TestHideNonDirectRuntimeCommands_PluginVisibleDespiteStaticVisibleProducts(t *testing.T) {
	withCleanDynamicRegistry(t)
	overrideVisibleProducts(t, []string{"calendar"})
	registerPluginProduct(t, "conference-local", "stdio://plugin/conference-local")

	root := &cobra.Command{Use: "dws"}
	calendarCmd := &cobra.Command{Use: "calendar"}
	pluginCmd := &cobra.Command{Use: "conference-local"}
	bogusCmd := &cobra.Command{Use: "bogus-not-a-product"}
	root.AddCommand(calendarCmd, pluginCmd, bogusCmd)

	hideNonDirectRuntimeCommands(root)

	if calendarCmd.Hidden {
		t.Errorf("calendar (static VisibleProducts) must stay visible, got Hidden=true")
	}
	if pluginCmd.Hidden {
		t.Errorf("conference-local (plugin-registered) must stay visible, got Hidden=true")
	}
	if !bogusCmd.Hidden {
		t.Errorf("bogus-not-a-product must be hidden, got Hidden=false")
	}
}

// TestVisibleMCPRootCommands_IncludesPluginProducts asserts that the help
// renderer surfaces plugin products in the "Discovered MCP Services" section
// and does not misclassify them as utility commands.
func TestVisibleMCPRootCommands_IncludesPluginProducts(t *testing.T) {
	withCleanDynamicRegistry(t)
	overrideVisibleProducts(t, []string{"calendar"})
	registerPluginProduct(t, "conference-local", "stdio://plugin/conference-local")

	root := &cobra.Command{Use: "dws"}
	calendarCmd := &cobra.Command{Use: "calendar"}
	pluginCmd := &cobra.Command{Use: "conference-local"}
	authCmd := &cobra.Command{Use: "auth"}
	root.AddCommand(calendarCmd, pluginCmd, authCmd)

	services := visibleMCPRootCommands(root)
	if !containsCommand(services, "conference-local") {
		t.Errorf("visibleMCPRootCommands missing plugin command: %v", commandNames(services))
	}
	if !containsCommand(services, "calendar") {
		t.Errorf("visibleMCPRootCommands missing static product: %v", commandNames(services))
	}

	utilities := visibleUtilityRootCommands(root)
	if containsCommand(utilities, "conference-local") {
		t.Errorf("visibleUtilityRootCommands must not include plugin command, got %v", commandNames(utilities))
	}
	if !containsCommand(utilities, "auth") {
		t.Errorf("visibleUtilityRootCommands must include genuine utility command, got %v", commandNames(utilities))
	}
}

func containsCommand(cmds []*cobra.Command, name string) bool {
	for _, c := range cmds {
		if c.Name() == name {
			return true
		}
	}
	return false
}

func commandNames(cmds []*cobra.Command) []string {
	names := make([]string, 0, len(cmds))
	for _, c := range cmds {
		names = append(names, c.Name())
	}
	return names
}
