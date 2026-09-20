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

package app

import "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/buildversion"

var version = "dev"

func init() {
	// ldflags may have overwritten version/buildTime/gitCommit before init.
	buildversion.Set(version, gitCommit, buildTime)
}

// SetVersion overrides the version, build time and git commit strings.
// Called by pkg/cli.SetVersion for overlay modules that inject their own
// version info via ldflags.
func SetVersion(v, bt, gc string) {
	if v != "" {
		version = v
	}
	if bt != "" {
		buildTime = bt
	}
	if gc != "" {
		gitCommit = gc
	}
	buildversion.Set(version, gitCommit, buildTime)
}

// Version returns the current CLI version string, including build metadata
// when injected via ldflags (buildTime, gitCommit).
func Version() string {
	return buildversion.Format(version, gitCommit, buildTime)
}

// RawVersion returns the bare version string without build metadata.
func RawVersion() string { return version }

// BuildTime returns the timestamp injected via ldflags. Reproducible release
// recipes use the commit's committer time in UTC.
func BuildTime() string { return buildTime }

// GitCommit returns the git commit hash injected via ldflags.
func GitCommit() string { return gitCommit }
