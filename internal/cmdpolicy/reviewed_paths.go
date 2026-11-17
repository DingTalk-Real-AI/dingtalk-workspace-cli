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

package cmdpolicy

// ReviewedPath is one exact, reviewed command path with the reason it is in
// its set. No prefixes or wildcards: a broader entry could silently change the
// treatment of a future command. Every path must resolve to a live command;
// the app-level test fails on a stale entry.
type ReviewedPath struct {
	Path   string
	Reason string
}

// discoveryEntries stay available whenever a valid policy is active. They are
// read-only discovery and diagnostics and carry no business side effect.
var discoveryEntries = []ReviewedPath{
	{Path: "schema", Reason: "Agents discover the permitted command surface through Schema; it returns no business data."},
	{Path: "help", Reason: "Help for permitted commands must stay reachable."},
	{Path: "version", Reason: "Reports the local binary version only."},
	{Path: PolicyShowPath, Reason: "Diagnoses the active command restriction policy."},
}

// invalidStateEntries are the only commands left when the rule set is invalid.
// Schema is excluded: without a valid policy the permitted surface is unknown.
var invalidStateEntries = []ReviewedPath{
	{Path: "help", Reason: "Help must stay reachable to diagnose an invalid policy."},
	{Path: "version", Reason: "Reports the local binary version only."},
	{Path: PolicyShowPath, Reason: "Reports why the policy is invalid."},
}

// channelEntries are commands whose real operation is chosen by their
// arguments or that change which code the CLI runs. Their own Safety
// declaration describes the command, not the channel it opens, so they are
// never opened by Allow globs, effect/risk ceilings, or AllowUnannotated. A
// rule must name one in AllowChannels to open it.
//
// New generic-invocation, plugin-management, or self-update commands must be
// added here in the same change that introduces them.
var channelEntries = []ReviewedPath{
	{Path: "api", Reason: "Calls an arbitrary OpenAPI method and path."},
	{Path: "mcp/published/invoke", Reason: "Invokes a remote MCP tool selected by its arguments."},
	{Path: "mcp/url/get", Reason: "Returns an MCP server URL that can be used outside the CLI."},
	{Path: "upgrade", Reason: "Replaces the running binary and can drop the compiled-in policy."},
	{Path: "plugin/list", Reason: "Plugin management."},
	{Path: "plugin/install", Reason: "Plugin management: installs code the CLI would load."},
	{Path: "plugin/info", Reason: "Plugin management."},
	{Path: "plugin/enable", Reason: "Plugin management."},
	{Path: "plugin/disable", Reason: "Plugin management."},
	{Path: "plugin/remove", Reason: "Plugin management."},
	{Path: "plugin/validate", Reason: "Plugin management."},
	{Path: "plugin/create", Reason: "Plugin management."},
	{Path: "plugin/dev", Reason: "Plugin management: registers a local development plugin."},
	{Path: "plugin/build", Reason: "Plugin management."},
	{Path: "plugin/config/set", Reason: "Plugin management: plugin configuration."},
	{Path: "plugin/config/get", Reason: "Plugin management: plugin configuration."},
	{Path: "plugin/config/list", Reason: "Plugin management: plugin configuration."},
	{Path: "plugin/config/unset", Reason: "Plugin management: plugin configuration."},
}

// PolicyShowPath is the diagnostic command that reports the active policy.
const PolicyShowPath = "config/policy/show"

func reviewedSet(entries []ReviewedPath) map[string]bool {
	set := make(map[string]bool, len(entries))
	for _, entry := range entries {
		set[entry.Path] = true
	}
	return set
}

var (
	discoverySet    = reviewedSet(discoveryEntries)
	invalidStateSet = reviewedSet(invalidStateEntries)
	channelSet      = reviewedSet(channelEntries)
)

// DiscoveryEntries returns the reviewed always-available discovery commands.
func DiscoveryEntries() []ReviewedPath { return append([]ReviewedPath(nil), discoveryEntries...) }

// ChannelEntries returns the reviewed channel entries.
func ChannelEntries() []ReviewedPath { return append([]ReviewedPath(nil), channelEntries...) }

// IsChannelEntry reports whether path is a reviewed channel entry.
func IsChannelEntry(path string) bool { return channelSet[path] }
