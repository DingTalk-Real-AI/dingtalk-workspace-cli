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
	"os"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/app"
)

func main() {
	var checkPath string
	var mergePath string
	flag.StringVar(&checkPath, "check", "", "check current CLI against a historical baseline")
	flag.StringVar(&mergePath, "merge", "", "merge current additions into a historical baseline")
	flag.Parse()

	if checkPath != "" && mergePath != "" {
		fmt.Fprintln(os.Stderr, "--check and --merge are mutually exclusive")
		os.Exit(2)
	}

	root := app.NewRootCommand()
	root.InitDefaultHelpCmd()
	current := snapshot(root)

	switch {
	case checkPath != "":
		baseline, err := readContract(checkPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "read interface baseline: %v\n", err)
			os.Exit(2)
		}
		failures := checkCompatibility(root, baseline)
		if len(failures) > 0 {
			fmt.Fprintln(os.Stderr, "CLI backwards-compatibility check failed:")
			for _, failure := range failures {
				fmt.Fprintf(os.Stderr, "  - %s\n", failure)
			}
			os.Exit(1)
		}
		fmt.Printf(
			"interface compatibility check: ok (%d historical command nodes; additions allowed)\n",
			len(baseline.Commands),
		)
	case mergePath != "":
		baseline, err := readContract(mergePath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "read interface baseline: %v\n", err)
			os.Exit(2)
		}
		merged, failures := mergeContracts(baseline, current)
		if len(failures) > 0 {
			fmt.Fprintln(os.Stderr, "cannot merge incompatible interface changes:")
			for _, failure := range failures {
				fmt.Fprintf(os.Stderr, "  - %s\n", failure)
			}
			os.Exit(1)
		}
		if err := renderContract(os.Stdout, merged); err != nil {
			fmt.Fprintf(os.Stderr, "render merged interface baseline: %v\n", err)
			os.Exit(2)
		}
	default:
		if err := renderContract(os.Stdout, current); err != nil {
			fmt.Fprintf(os.Stderr, "render interface baseline: %v\n", err)
			os.Exit(2)
		}
	}
}
