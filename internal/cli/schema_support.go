// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package cli

import "github.com/spf13/cobra"

// walkLeafCommands invokes fn for every runnable leaf command in the tree.
func walkLeafCommands(cmd *cobra.Command, fn func(*cobra.Command)) {
	if cmd.Runnable() && !cmd.HasSubCommands() {
		fn(cmd)
		return
	}
	for _, sub := range cmd.Commands() {
		if sub.Name() == "help" {
			continue
		}
		if !sub.IsAvailableCommand() && !hasRuntimeSchemaCommand(sub) {
			continue
		}
		walkLeafCommands(sub, fn)
	}
}

func hasRuntimeSchemaCommand(cmd *cobra.Command) bool {
	if cmd == nil {
		return false
	}
	if _, toolName, _ := runtimeSchemaAnnotations(cmd); toolName != "" && !runtimeSchemaExcluded(cmd) {
		return true
	}
	for _, child := range cmd.Commands() {
		if hasRuntimeSchemaCommand(child) {
			return true
		}
	}
	return false
}
