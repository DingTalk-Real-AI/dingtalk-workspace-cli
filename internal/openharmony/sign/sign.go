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

package sign

import (
	"errors"
	"fmt"
	"os"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/helpers"
)

var readInputFile = os.ReadFile

// SignFile creates an OpenHarmony self-signed ELF at outputPath.
func SignFile(inputPath, outputPath string) error {
	if inputPath == "" || outputPath == "" {
		return errors.New("input and output paths are required")
	}
	inputInfo, err := os.Stat(inputPath)
	if err != nil {
		return fmt.Errorf("stat input: %w", err)
	}
	if !inputInfo.Mode().IsRegular() {
		return errors.New("input is not a regular file")
	}
	input, err := readInputFile(inputPath)
	if err != nil {
		return fmt.Errorf("read input: %w", err)
	}
	image, err := parseELF(input)
	if err != nil {
		return fmt.Errorf("parse ELF: %w", err)
	}
	rewritten, err := rewriteELF(image)
	if err != nil {
		return fmt.Errorf("rewrite ELF: %w", err)
	}
	block := codeSignBlock(len(rewritten.data), merkleRoot(rewritten.data, rewritten.codeOffset))
	copy(rewritten.data[rewritten.codeOffset:], block)
	return helpers.AtomicWrite(outputPath, rewritten.data, inputInfo.Mode().Perm())
}
