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

// Command interface-baseline prints a deterministic snapshot of every public
// Cobra command, alias, and directly-defined flag in DWS CLI.
package main

import (
	"fmt"
	"sort"
	"strings"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/app"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

func main() {
	root := app.NewRootCommand()
	root.InitDefaultHelpCmd()
	emit(root, nil)
}

func emit(cmd *cobra.Command, parentPath []string) {
	path := parentPath
	if cmd.HasParent() {
		path = append(append([]string(nil), parentPath...), cmd.Name())
	}

	label := "root"
	if len(path) > 0 {
		label = strings.Join(path, ".")
	}
	fmt.Printf("[%s]\n", label)

	children := publicChildren(cmd)
	if len(children) > 0 {
		names := make([]string, 0, len(children))
		for _, child := range children {
			names = append(names, child.Name())
		}
		fmt.Printf("  commands: %s\n", strings.Join(names, ", "))
	}

	if len(cmd.Aliases) > 0 {
		aliases := append([]string(nil), cmd.Aliases...)
		sort.Strings(aliases)
		fmt.Printf("  aliases: %s\n", strings.Join(aliases, ", "))
	}

	if flags := directFlags(cmd); len(flags) > 0 {
		fmt.Printf("  flags: %s\n", strings.Join(flags, ", "))
	}

	for _, child := range children {
		fmt.Println()
		emit(child, path)
	}
}

func publicChildren(cmd *cobra.Command) []*cobra.Command {
	var children []*cobra.Command
	for _, child := range cmd.Commands() {
		if child.Hidden {
			continue
		}
		children = append(children, child)
	}
	sort.Slice(children, func(i, j int) bool {
		return children[i].Name() < children[j].Name()
	})
	return children
}

func directFlags(cmd *cobra.Command) []string {
	var flags []string
	cmd.InitDefaultHelpFlag()
	cmd.LocalFlags().VisitAll(func(flag *pflag.Flag) {
		if flag.Hidden {
			return
		}
		name := "--" + flag.Name
		if flag.Shorthand != "" {
			name = "-" + flag.Shorthand + "/" + name
		}
		flags = append(flags, name+":"+flag.Value.Type())
	})
	sort.Strings(flags)
	return flags
}
