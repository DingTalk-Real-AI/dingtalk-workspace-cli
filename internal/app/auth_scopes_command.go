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

import (
	"fmt"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/spf13/cobra"
)

// issue253URL points at the tracking issue so the stub error messages are
// actionable. Remove the references once the commands are implemented.
const issue253URL = "https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/issues/253"

// newAuthScopesCommand is the SKELETON for `dws auth scopes` (#253).
//
// Goal: list the OAuth scopes granted to the currently-logged-in identity, so
// users and agents can answer "why am I getting a permission error?" without
// guessing.
//
// TODO(#253): implement.
//   - Source of truth for granted scopes: the stored token. Look at
//     internal/auth (TokenData / identity.go) — the OAuth token response, or a
//     JWT-style access token, carries the granted scope set; surface it here.
//     If the encrypted MCP-default token doesn't expose scopes, say so clearly
//     and point at `dws auth login` with `--client-id/--client-secret`.
//   - Cross-reference with internal/auth/auth_registry.go for the catalogue of
//     scopes the CLI knows about (human-readable names / which products need
//     them), so the output can mark "granted" vs "available but not granted".
//   - Honour the global -f flag via output.WriteCommandPayload; default JSON
//     shape: {"granted":[...],"identity":{...}}.
//   - Optional: a --check <scope> shortcut here, but `dws auth check` (below)
//     is the dedicated entry point.
func newAuthScopesCommand() *cobra.Command {
	return &cobra.Command{
		Use:               "scopes",
		Short:             "列出当前登录身份已授予的 OAuth scope",
		Long:              "列出当前登录身份已授予的 OAuth scope，便于排查权限不足问题。\n\n[未实现 — 见 " + issue253URL + "]",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			// TODO(#253): replace with the real implementation described above.
			return apperrors.NewInternal(fmt.Sprintf("`dws auth scopes` is not implemented yet — see %s", issue253URL))
		},
	}
}

// newAuthCheckCommand is the SKELETON for `dws auth check <scope>` (#253).
//
// Goal: exit 0 if <scope> is granted to the current identity, non-zero with a
// "how to request it" hint otherwise — so scripts/agents can gate on it.
//
// TODO(#253): implement.
//   - Reuse whatever `dws auth scopes` ends up using to read the granted set.
//   - Match <scope> case-insensitively against the granted set; print a short
//     OK/NOT-GRANTED line (and JSON under -f json: {"scope":..,"granted":bool}).
//   - On NOT-GRANTED return a non-zero exit (apperrors with a non-zero code so
//     CI/scripts can branch) and include the admin-approval / re-login hint
//     that `dws auth login` already shows when a scope is missing.
//   - Optional `--for <command>`: resolve the command's required scope(s) via
//     the same metadata `dws schema` reads, then check each — pairs naturally
//     with #251.
func newAuthCheckCommand() *cobra.Command {
	return &cobra.Command{
		Use:               "check <scope>",
		Short:             "检查指定 OAuth scope 是否已授权 (未授权时退出码非 0)",
		Long:              "检查指定 OAuth scope 是否已授予当前登录身份；未授权时退出码非 0 并给出申请引导。\n\n[未实现 — 见 " + issue253URL + "]",
		Args:              cobra.ExactArgs(1),
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			// TODO(#253): replace with the real implementation described above.
			return apperrors.NewInternal(fmt.Sprintf("`dws auth check` is not implemented yet — see %s", issue253URL))
		},
	}
}
