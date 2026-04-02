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

var version = "dev"

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
}

// Version returns the current CLI version string, including build metadata
// when injected via ldflags (buildTime, gitCommit).
func Version() string {
	if buildTime != "unknown" || gitCommit != "unknown" {
		return version + " (" + gitCommit + ", " + buildTime + ")"
	}
	return version
}

// RawVersion returns the bare version string without build metadata.
func RawVersion() string { return version }

// BuildTime returns the build timestamp injected via ldflags.
func BuildTime() string { return buildTime }

// GitCommit returns the git commit hash injected via ldflags.
func GitCommit() string { return gitCommit }
