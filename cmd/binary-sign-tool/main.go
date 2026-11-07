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

package main

import (
	"fmt"
	"os"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/openharmony/sign"
)

var (
	runCLI      = run
	exitProcess = os.Exit
)

func main() {
	if err := runCLI(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		exitProcess(1)
	}
}

func run(args []string) error {
	if len(args) == 0 || args[0] != "sign" {
		return fmt.Errorf("usage: binary-sign-tool sign -inFile INPUT -outFile OUTPUT -selfSign 1")
	}
	var inputPath, outputPath, selfSign string
	var seenInput, seenOutput, seenSelfSign bool
	for index := 1; index < len(args); {
		switch args[index] {
		case "-inFile":
			if seenInput {
				return fmt.Errorf("duplicate argument %q", args[index])
			}
			seenInput = true
			var err error
			inputPath, index, err = nextArgument(args, index, "-inFile")
			if err != nil {
				return err
			}
		case "-outFile":
			if seenOutput {
				return fmt.Errorf("duplicate argument %q", args[index])
			}
			seenOutput = true
			var err error
			outputPath, index, err = nextArgument(args, index, "-outFile")
			if err != nil {
				return err
			}
		case "-selfSign":
			if seenSelfSign {
				return fmt.Errorf("duplicate argument %q", args[index])
			}
			seenSelfSign = true
			var err error
			selfSign, index, err = nextArgument(args, index, "-selfSign")
			if err != nil {
				return err
			}
		default:
			return fmt.Errorf("unknown argument %q", args[index])
		}
	}
	if inputPath == "" || outputPath == "" || selfSign != "1" {
		return fmt.Errorf("only self-sign mode is supported: sign -inFile INPUT -outFile OUTPUT -selfSign 1")
	}
	return sign.SignFile(inputPath, outputPath)
}

func nextArgument(args []string, index int, name string) (string, int, error) {
	if index+1 >= len(args) || args[index+1] == "" {
		return "", index, fmt.Errorf("missing value for %s", name)
	}
	return args[index+1], index + 2, nil
}
