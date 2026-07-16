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

// Package builtin aggregates all built-in shortcut service packages via blank
// imports so their init() registrations run, then re-exports the compiled cobra
// commands. The host application depends only on this package, keeping the
// service packages free to import the core shortcut package without a cycle.
//
// Add a blank import here when a new service package is generated under
// internal/shortcut/<service>/.
package builtin

import (
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"

	// Service packages — each registers its shortcuts from init().
	_ "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut/aitable"
	_ "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut/attendance"
	_ "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut/calendar"
	_ "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut/chat"
	_ "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut/contact"
	_ "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut/devapp"
	_ "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut/ding"
	_ "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut/doc"
	_ "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut/drive"
	_ "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut/mail"
	_ "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut/minutes"
	_ "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut/oa"
	_ "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut/report"
	_ "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut/sheet"
	_ "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut/smart"
	_ "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut/todo"
	_ "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut/wiki"

	"github.com/spf13/cobra"
)

// Commands returns all built-in shortcut commands, grouped by service, ready to
// be merged into the root command tree.
func Commands() []*cobra.Command {
	return shortcut.Commands()
}
