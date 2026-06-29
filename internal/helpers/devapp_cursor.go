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

import "github.com/spf13/cobra"

// devapp_cursor wires cursor pagination as a PASS-THROUGH: the CLI forwards
// --cursor and --page-size straight into the upstream params, and the upstream
// returns nextCursor/hasMore in its response. No synthesis, no offset/page
// translation, no encode/decode in the CLI — the upstream tools own the cursor
// contract (see docs/cursor-pagination-design.md and docs/upstream-todo.md).
// Until the upstream tools accept `cursor`/`pageSize` and return `nextCursor`,
// paging is in the "pending integration" state, same as the precheckOnly
// rename — no anti-corruption layer bridges the gap.

// registerDevAppCursorFlags adds the two cursor flags every list/search command
// exposes. pageSize defaults to 20.
func registerDevAppCursorFlags(cmd *cobra.Command) {
	cmd.Flags().String("cursor", "", "游标令牌：首次查询留空，续翻传上次出参的 nextCursor")
	cmd.Flags().Int("page-size", 20, "单页条数，默认 20")
}

// devAppApplyCursorParams forwards cursor/pageSize into params as-is. cursor is
// only set when non-empty (first page omits it).
func devAppApplyCursorParams(cmd *cobra.Command, params map[string]any) {
	if cur := devAppStringFlag(cmd, "cursor"); cur != "" {
		params["cursor"] = cur
	}
	size := devAppIntFlag(cmd, "page-size")
	if size < 1 {
		size = 20
	}
	params["pageSize"] = size
}
