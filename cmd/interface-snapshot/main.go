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

// interface-snapshot is an internal CI helper. It is intentionally a separate
// binary so it can be copied into a temporary worktree and compiled against an
// older revision's real Cobra root.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/app"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/i18n"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/interfacesnapshot"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		printUsage(stderr)
		return 2
	}

	switch args[0] {
	case "generate":
		if err := runGenerate(args[1:], stdout, stderr); err != nil {
			fmt.Fprintln(stderr, err)
			return 2
		}
		return 0
	case "compare":
		compatible, err := runCompare(args[1:], stdout, stderr)
		if err != nil {
			fmt.Fprintln(stderr, err)
			return 2
		}
		if !compatible {
			return 1
		}
		return 0
	default:
		fmt.Fprintf(stderr, "unknown command %q\n", args[0])
		printUsage(stderr)
		return 2
	}
}

func runGenerate(args []string, stdout, stderr io.Writer) error {
	flags := flag.NewFlagSet("generate", flag.ContinueOnError)
	flags.SetOutput(stderr)
	output := flags.String("output", "-", "snapshot output path, or - for stdout")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("generate accepts no positional arguments")
	}

	home, err := os.MkdirTemp("", "dws-interface-snapshot-*")
	if err != nil {
		return fmt.Errorf("create isolated home: %w", err)
	}
	defer os.RemoveAll(home)

	environment := map[string]string{
		"DWS_CONFIG_DIR": home,
		"DWS_LANG":       "en",
		"HOME":           home,
		"NO_COLOR":       "1",
		"USERPROFILE":    home,
	}
	type previousEnv struct {
		value string
		set   bool
	}
	previous := make(map[string]previousEnv, len(environment))
	for key, value := range environment {
		oldValue, wasSet := os.LookupEnv(key)
		previous[key] = previousEnv{value: oldValue, set: wasSet}
		if err := os.Setenv(key, value); err != nil {
			return fmt.Errorf("set %s: %w", key, err)
		}
	}
	defer func() {
		for key, old := range previous {
			if old.set {
				_ = os.Setenv(key, old.value)
			} else {
				_ = os.Unsetenv(key)
			}
		}
	}()
	previousLang := i18n.Lang()
	defer i18n.SetLang(previousLang)
	i18n.SetLang("en")

	snapshot := interfacesnapshot.Capture(app.NewRootCommand())
	if *output == "-" {
		return interfacesnapshot.Write(stdout, snapshot)
	}

	file, err := os.Create(filepath.Clean(*output))
	if err != nil {
		return fmt.Errorf("create snapshot %q: %w", *output, err)
	}
	writeErr := interfacesnapshot.Write(file, snapshot)
	closeErr := file.Close()
	if writeErr != nil {
		return fmt.Errorf("write snapshot %q: %w", *output, writeErr)
	}
	if closeErr != nil {
		return fmt.Errorf("close snapshot %q: %w", *output, closeErr)
	}
	return nil
}

func runCompare(args []string, stdout, stderr io.Writer) (bool, error) {
	flags := flag.NewFlagSet("compare", flag.ContinueOnError)
	flags.SetOutput(stderr)
	currentPath := flags.String("current", "", "candidate snapshot path")
	basePath := flags.String("base", "", "target main/development baseline snapshot path")
	stablePath := flags.String("stable", "", "latest stable GA snapshot path")
	if err := flags.Parse(args); err != nil {
		return false, err
	}
	if flags.NArg() != 0 {
		return false, fmt.Errorf("compare accepts no positional arguments")
	}
	if *currentPath == "" {
		return false, fmt.Errorf("compare requires --current")
	}
	if *basePath == "" && *stablePath == "" {
		return false, fmt.Errorf("compare requires --base, --stable, or both")
	}

	current, err := readSnapshot(*currentPath)
	if err != nil {
		return false, fmt.Errorf("read current snapshot: %w", err)
	}
	references := make(map[string]interfacesnapshot.Snapshot, 2)
	if *basePath != "" {
		references["main"], err = readSnapshot(*basePath)
		if err != nil {
			return false, fmt.Errorf("read main/development baseline snapshot: %w", err)
		}
	}
	if *stablePath != "" {
		references["stable"], err = readSnapshot(*stablePath)
		if err != nil {
			return false, fmt.Errorf("read stable snapshot: %w", err)
		}
	}

	report := interfacesnapshot.CompareAll(current, references)
	encoder := json.NewEncoder(stdout)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(report); err != nil {
		return false, fmt.Errorf("write comparison report: %w", err)
	}
	return report.Compatible, nil
}

func readSnapshot(path string) (interfacesnapshot.Snapshot, error) {
	file, err := os.Open(filepath.Clean(path))
	if err != nil {
		return interfacesnapshot.Snapshot{}, err
	}
	defer file.Close()
	return interfacesnapshot.Read(file)
}

func printUsage(w io.Writer) {
	fmt.Fprintln(w, "usage:")
	fmt.Fprintln(w, "  interface-snapshot generate [--output FILE]")
	fmt.Fprintln(w, "  interface-snapshot compare --current FILE [--base FILE] [--stable FILE]")
}
