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

package contractfinal

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/runtimeannotate"
)

// ApplyParamDecls emits parameter declarations as dws.schema.* annotations on the
// command's flags. Called at assembly time when all flags exist on the tree.
// Each non-blank ParamDecl.Name must resolve to an existing Cobra flag;
// unknown names fail closed so typos cannot silently drop during generation.
func ApplyParamDecls(cmd *cobra.Command, decls []contract.ParamDecl) error {
	if cmd == nil || len(decls) == 0 {
		return nil
	}
	for _, p := range decls {
		name := strings.TrimSpace(p.Name)
		if name == "" {
			continue
		}
		if runtimeannotate.CommandFlag(cmd, name) == nil {
			return fmt.Errorf("ParamDecl %q references unknown flag on %q", name, cmd.CommandPath())
		}
	}
	for _, p := range decls {
		name := strings.TrimSpace(p.Name)
		if name == "" {
			continue
		}
		if prop := strings.TrimSpace(p.Property); prop != "" {
			runtimeannotate.AnnotateRuntimeFlagProperty(cmd, name, prop)
		}
		if p.Required != nil {
			runtimeannotate.AnnotateRuntimeFlagRequiredValue(cmd, name, *p.Required)
		}
		if it := strings.TrimSpace(p.InterfaceType); it != "" {
			runtimeannotate.AnnotateRuntimeFlagInterfaceType(cmd, name, it)
		}
		if desc := strings.TrimSpace(p.Description); desc != "" {
			runtimeannotate.AnnotateRuntimeFlagDescription(cmd, name, desc)
		}
		if rw := strings.TrimSpace(p.RequiredWhen); rw != "" {
			runtimeannotate.AnnotateRuntimeFlagRequiredWhen(cmd, name, rw)
		}
		if len(p.Enum) > 0 {
			runtimeannotate.AnnotateRuntimeFlagEnum(cmd, name, p.Enum...)
		}
	}
	return nil
}
