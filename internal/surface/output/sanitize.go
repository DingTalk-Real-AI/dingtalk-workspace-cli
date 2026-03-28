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

package output

import "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/validate"

// SanitizeForTerminal strips ANSI escape sequences, control characters, and
// dangerous Unicode from text before it is printed to a terminal.
// Delegates to the validate package which provides the canonical implementation.
func SanitizeForTerminal(text string) string {
	return validate.SanitizeForTerminal(text)
}
