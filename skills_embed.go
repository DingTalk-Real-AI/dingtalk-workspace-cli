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

// Package dws is the module-root package. Its sole purpose is to bake the
// skills/ documentation tree into the binary at build time so that
// `dws skill setup` installs the skill version shipped with THIS binary,
// independent of any local checkout. See internal/app/skill_setup.go.
package dws

import "embed"

// EmbeddedSkills holds only the installable mono and multi skill trees compiled
// into the binary. Build-only inputs such as internal/cli/schema_hints are
// intentionally excluded. The `all:` prefix is required so dot/underscore
// entries — e.g. references/best_practices/_common — are included rather than
// skipped.
//
//go:embed all:skills/mono all:skills/multi
var EmbeddedSkills embed.FS
