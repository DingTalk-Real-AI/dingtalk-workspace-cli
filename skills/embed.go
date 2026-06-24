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

// Package skills embeds the dws skill sources into the binary so that
// `dws skill setup` always installs the skill set matching the running
// binary version, regardless of the working directory. Before this, setup
// resolved its source from cwd/exe-adjacent checkouts, so an upgraded
// binary could silently re-install stale skills (issue PeterGuy326#8).
package skills

import "embed"

// FS holds the mono and multi skill trees. The all: prefix keeps
// underscore-prefixed directories (e.g. references/best_practices/_common)
// that plain embed patterns would skip.
//
//go:embed all:mono all:multi
var FS embed.FS
