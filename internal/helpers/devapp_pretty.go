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

package helpers

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"
)

// devapp_pretty annotates --format pretty output with human-readable status
// labels. Upstream payloads use a numeric enum for app status and the bare
// INIT/AUDIT/... enum for versionStatus. JSON output is NEVER touched
// (transparency contract); only the pretty view gets `*Text` companions.

var devAppStatusLabels = map[int]string{
	0: "IN_ACTIVE 已停用",
	1: "ACTIVE 已激活",
	2: "WAIT_ACTIVE 待激活",
	3: "EXPIRED 已过期",
}

var devAppVersionStatusLabels = map[string]string{
	"INIT":    "未发布（有待发布变更）",
	"AUDIT":   "发布审核中",
	"RELEASE": "已发布生效",
	"GRAY":    "灰度中",
}

// devAppToolStatusKind classifies which enum family a tool's `status`-like
// fields belong to, so app status 1 (ACTIVE) is never labeled with version
// semantics and vice versa.
var devAppToolStatusKind = map[string]string{
	"get_dev_app":                "app",
	"list_dev_app":               "app",
	"create_dev_app":             "version", // create/update 返回 versionStatus
	"update_dev_app":             "version",
	"create_dev_app_version":     "version",
	"list_dev_app_versions":      "version",
	"get_dev_app_version_detail": "version",
	"get_dev_app_version_status": "version",
	"publish_dev_app_version":    "version",
}

func devAppPrettyWanted(cmd *cobra.Command) bool {
	format, err := cmd.Flags().GetString("format")
	return err == nil && format == "pretty"
}

// devAppPrettyAnnotate walks the response and adds appStatusText /
// versionStatusText next to recognized enum fields, in place.
func devAppPrettyAnnotate(tool string, payload any) {
	kind, ok := devAppToolStatusKind[tool]
	if !ok {
		return
	}
	devAppAnnotateNode(payload, kind)
}

func devAppAnnotateNode(node any, kind string) {
	switch v := node.(type) {
	case map[string]any:
		devAppAnnotateMap(v, kind)
		for _, child := range v {
			devAppAnnotateNode(child, kind)
		}
	case []any:
		for _, child := range v {
			devAppAnnotateNode(child, kind)
		}
	}
}

func devAppAnnotateMap(m map[string]any, kind string) {
	if kind == "app" {
		for _, key := range []string{"status", "appStatus"} {
			if n, ok := devAppAsInt(m[key]); ok {
				if label, ok := devAppStatusLabels[n]; ok {
					m[key+"Text"] = label
				}
			}
		}
		return
	}
	if s, ok := m["versionStatus"].(string); ok {
		if label, ok := devAppVersionStatusLabels[s]; ok {
			m["versionStatusText"] = label
		}
	}
}

func devAppAsInt(v any) (int, bool) {
	switch n := v.(type) {
	case int:
		return n, true
	case int64:
		return int(n), true
	case float64:
		return int(n), true
	case json.Number:
		i, err := n.Int64()
		return int(i), err == nil
	case string:
		var i int
		if _, err := fmt.Sscanf(n, "%d", &i); err == nil {
			return i, true
		}
	}
	return 0, false
}
