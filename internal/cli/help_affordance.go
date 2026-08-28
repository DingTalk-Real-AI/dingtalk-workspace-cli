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

package cli

import (
	"fmt"
	"io"
	"strings"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
	"github.com/spf13/cobra"
)

// RenderHelpAffordances appends the Agent-facing metadata that Cobra cannot
// express in its native command template. Leaf guidance and Safety come from
// ResolveMeta (the assembled Schema source); references come from the owning
// ProductDecl. Direct product/service Help renders references only.
func RenderHelpAffordances(cmd *cobra.Command) {
	if cmd == nil || cmd.Root() == nil || cmd == cmd.Root() {
		return
	}

	cliPath := commandCLIPath(cmd)
	productID := ""
	if SchemaSourceRootRegistered() {
		if meta, ok := ResolveMeta(cliPath); ok {
			renderSelectionGuidance(cmd, meta.Selection)
			if meta.Safety.ShouldRender() {
				renderSafety(cmd, meta.Safety)
			}
			productID = strings.TrimSpace(meta.Identity.ProductID)
		}
	}

	// Only a direct child is a service Help page. Intermediate command groups
	// intentionally stay compact; leaves inherit references through ProductID.
	if productID == "" && cmd.Parent() == cmd.Root() {
		productID = strings.TrimSpace(cmd.Name())
	}
	if productID == "" {
		return
	}
	decl, ok := contract.LookupProductDecl(productID)
	if !ok {
		return
	}
	renderHelpReferences(cmd, decl.HelpReferences)
}

func commandCLIPath(cmd *cobra.Command) string {
	return strings.TrimSpace(strings.TrimPrefix(cmd.CommandPath(), cmd.Root().Name()+" "))
}

func renderSelectionGuidance(cmd *cobra.Command, selection CommandSelection) {
	w := cmd.OutOrStdout()
	renderGuidanceList(w, "When to use:", selection.UseWhen)
	renderGuidanceList(w, "Avoid when:", selection.AvoidWhen)
	renderGuidanceList(w, "Prerequisites:", selection.Prerequisites)
	renderGuidanceList(w, "Tips:", selection.Tips)
}

func renderGuidanceList(w io.Writer, heading string, values []string) {
	items := nonEmptyStrings(values)
	if len(items) == 0 {
		return
	}
	fmt.Fprintf(w, "\n%s\n", heading)
	for _, item := range items {
		fmt.Fprintf(w, "  - %s\n", item)
	}
}

func renderHelpReferences(cmd *cobra.Command, refs contract.HelpReferences) {
	w := cmd.OutOrStdout()
	if skills := nonEmptyStrings(refs.RelatedSkills); len(skills) > 0 {
		fmt.Fprintln(w, "\nRelated skills:")
		for _, skill := range skills {
			fmt.Fprintf(w, "  - %s\n", skill)
		}
	}
	if len(refs.Documentation) > 0 {
		fmt.Fprintln(w, "\nDocumentation:")
		for _, document := range refs.Documentation {
			if label, url := strings.TrimSpace(document.Label), strings.TrimSpace(document.URL); label != "" && url != "" {
				fmt.Fprintf(w, "  - %s: %s\n", label, url)
			}
		}
	}
}

func nonEmptyStrings(values []string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			out = append(out, value)
		}
	}
	return out
}
