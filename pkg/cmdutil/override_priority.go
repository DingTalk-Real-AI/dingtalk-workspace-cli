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

package cmdutil

import (
	"strconv"
	"strings"

	"github.com/spf13/cobra"
)

// OverridePriorityAnnotation is the cobra.Command.Annotations key used to
// declare a command's merge-time override priority. Higher values win when
// same-named leaves collide during merge. Exported so overlays and helpers
// can reference the same key as the core merge logic without spelling drift.
const OverridePriorityAnnotation = "dws.override-priority"

// SetOverridePriority stamps the override priority annotation on cmd. A
// positive value asks the merge layer to promote this command over a
// same-named leaf with a lower (or unset) priority.
func SetOverridePriority(cmd *cobra.Command, priority int) {
	if cmd == nil {
		return
	}
	if cmd.Annotations == nil {
		cmd.Annotations = map[string]string{}
	}
	cmd.Annotations[OverridePriorityAnnotation] = strconv.Itoa(priority)
}

// OverridePriority returns the override priority annotation value on cmd,
// or 0 if the annotation is absent or malformed.
func OverridePriority(cmd *cobra.Command) int {
	if cmd == nil || cmd.Annotations == nil {
		return 0
	}
	raw := strings.TrimSpace(cmd.Annotations[OverridePriorityAnnotation])
	if raw == "" {
		return 0
	}
	value, err := strconv.Atoi(raw)
	if err != nil {
		return 0
	}
	return value
}
