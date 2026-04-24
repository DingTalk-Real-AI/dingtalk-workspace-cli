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

package compat

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

// buildOverlayRedirect builds a top-level product command whose only behaviour
// is printing "Please use: dws <target>" and returning nil. All args/flags are
// accepted and ignored so users running the old command path get the redirect
// message instead of a parse error.
//
// See discovery-schema-v3 §2.6 (CLIOverlay.RedirectTo).
func buildOverlayRedirect(name, description, target string) *cobra.Command {
	target = strings.TrimSpace(target)
	short := strings.TrimSpace(description)
	if short == "" {
		if target != "" {
			short = fmt.Sprintf("moved → %s", target)
		} else {
			short = "command relocated; see --help for the canonical path"
		}
	}
	cmd := &cobra.Command{
		Use:                name,
		Short:              short,
		Long:               fmt.Sprintf("This command has moved. Please use: dws %s", target),
		DisableFlagParsing: true,
		DisableAutoGenTag:  true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if target == "" {
				_ = cmd.Help()
				return nil
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Please use: dws %s\n", target)
			return nil
		},
	}
	return cmd
}
