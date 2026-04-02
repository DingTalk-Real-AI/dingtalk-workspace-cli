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

// Package cli exposes public entry points for the DWS CLI so that external
// overlay modules can embed and launch the CLI after customising edition hooks.
package cli

import "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/app"

// SetVersion overrides the version, build time and git commit strings
// that are displayed by `dws version` and `dws --version`.
// Typically called by overlay main.go with values injected via ldflags.
func SetVersion(v, buildTime, gitCommit string) {
	app.SetVersion(v, buildTime, gitCommit)
}

// Execute runs the root CLI command and returns the process exit code.
func Execute() int {
	return app.Execute()
}
