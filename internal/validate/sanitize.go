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

package validate

import (
	"regexp"
	"strings"
)

// ansiEscape matches ANSI CSI sequences (ESC[ ... letter) and OSC sequences (ESC] ... BEL).
var ansiEscape = regexp.MustCompile(`\x1b\[[0-9;?>=!]*[a-zA-Z]|\x1b\][^\x07]*\x07`)

// SanitizeForTerminal strips ANSI escape sequences, C0 control characters
// (except \n and \t), and dangerous Unicode from text. Apply to table-format
// output and stderr messages, but NOT to json output where consumers need raw data.
//
// API responses may contain injected ANSI sequences that clear the screen,
// fake a colored "OK" status, or change the terminal title. In AI Agent
// scenarios, such injections can pollute the LLM's context window.
func SanitizeForTerminal(text string) string {
	if strings.ContainsRune(text, '\x1b') {
		text = ansiEscape.ReplaceAllString(text, "")
	}
	var b strings.Builder
	b.Grow(len(text))
	for _, r := range text {
		switch {
		case r == '\n' || r == '\t':
			b.WriteRune(r)
		case r < 0x20 || r == 0x7f:
			continue
		case isDangerousUnicode(r):
			continue
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}
