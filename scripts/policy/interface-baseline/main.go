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

// Command interface-baseline snapshots and checks the backwards-compatible
// public Cobra surface of DWS CLI.
package main

import (
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/app"
	"github.com/spf13/cobra"
)

func main() {
	os.Exit(run(os.Args[1:], app.NewRootCommand(), os.Stdout, os.Stderr))
}

func run(args []string, root *cobra.Command, stdout, stderr io.Writer) int {
	var checkPath string
	var mergePath string
	flags := flag.NewFlagSet("interface-baseline", flag.ContinueOnError)
	flags.SetOutput(stderr)
	flags.StringVar(&checkPath, "check", "", "check current CLI against a historical baseline")
	flags.StringVar(&mergePath, "merge", "", "merge current additions into a historical baseline")
	if err := flags.Parse(args); err != nil {
		return 2
	}

	if checkPath != "" && mergePath != "" {
		fmt.Fprintln(stderr, "--check and --merge are mutually exclusive")
		return 2
	}

	root.InitDefaultHelpCmd()
	current := snapshot(root)

	switch {
	case checkPath != "":
		baseline, err := readContract(checkPath)
		if err != nil {
			fmt.Fprintf(stderr, "read interface baseline: %v\n", err)
			return 2
		}
		failures := checkCompatibility(root, baseline)
		if len(failures) > 0 {
			fmt.Fprintln(stderr, "CLI backwards-compatibility check failed:")
			for _, failure := range failures {
				fmt.Fprintf(stderr, "  - %s\n", failure)
			}
			return 1
		}
		fmt.Fprintf(stdout,
			"interface compatibility check: ok (%d historical command nodes; additions allowed)\n",
			len(baseline.Commands),
		)
	case mergePath != "":
		baseline, err := readContract(mergePath)
		if err != nil {
			fmt.Fprintf(stderr, "read interface baseline: %v\n", err)
			return 2
		}
		merged, failures := mergeContracts(baseline, current)
		if len(failures) > 0 {
			fmt.Fprintln(stderr, "cannot merge incompatible interface changes:")
			for _, failure := range failures {
				fmt.Fprintf(stderr, "  - %s\n", failure)
			}
			return 1
		}
		if err := renderContract(stdout, merged); err != nil {
			fmt.Fprintf(stderr, "render merged interface baseline: %v\n", err)
			return 2
		}
	default:
		if err := renderContract(stdout, current); err != nil {
			fmt.Fprintf(stderr, "render interface baseline: %v\n", err)
			return 2
		}
	}
	return 0
}
