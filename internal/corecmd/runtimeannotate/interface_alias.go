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

package runtimeannotate

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// CLI flag alias evidence is kept in this narrow file so the base-owned
// Interface Snapshot helper can add the protocol constants to an older stable
// worktree without replacing that revision's complete runtimeannotate package.
// Only corecmd.FlagSpec.Aliases writes the exact origin; neither field is a
// Schema synonym or final payload-equivalence proof.
const (
	AnnotationFlagAliasOf     = "dws.compat.alias_of"
	AnnotationFlagAliasOrigin = "dws.compat.alias_origin"
	FlagAliasOriginCorecmdV1  = "corecmd.flag_spec_aliases.v1"
)

// ReviewedHiddenAliasTarget resolves framework-owned alias evidence without
// exposing its protocol tokens to Schema assembly. Only corecmd FlagSpec
// aliases carry enough evidence to rewrite an executable hidden spelling to a
// public flag; ad-hoc annotations remain executable-only compatibility.
func ReviewedHiddenAliasTarget(cmd *cobra.Command, flag *pflag.Flag) (string, bool, error) {
	if flag == nil || !flag.Hidden {
		return "", false, nil
	}
	aliasOf, hasAliasOf := flag.Annotations[AnnotationFlagAliasOf]
	origin, hasOrigin := flag.Annotations[AnnotationFlagAliasOrigin]
	if !hasAliasOf && !hasOrigin {
		return "", false, nil
	}
	if !hasOrigin || len(origin) != 1 || origin[0] != FlagAliasOriginCorecmdV1 {
		return "", false, nil
	}
	if !hasAliasOf || len(aliasOf) != 1 || aliasOf[0] == "" || aliasOf[0] != strings.TrimSpace(aliasOf[0]) {
		return "", false, fmt.Errorf("hidden input %q has malformed reviewed alias target", flag.Name)
	}
	targetName := aliasOf[0]
	if targetName == flag.Name {
		return "", false, fmt.Errorf("hidden input %q cannot alias itself", flag.Name)
	}
	target := CommandFlag(cmd, targetName)
	if target == nil {
		return "", false, fmt.Errorf("hidden alias %q targets unknown executable input %q", flag.Name, targetName)
	}
	if target.Hidden {
		return "", false, fmt.Errorf("hidden alias %q targets hidden input %q", flag.Name, targetName)
	}
	if values := target.Annotations[AnnotationFlagAliasOf]; len(values) > 0 {
		return "", false, fmt.Errorf("hidden alias %q targets alias input %q", flag.Name, targetName)
	}
	if flag.Value == nil || target.Value == nil || flag.Value.Type() != target.Value.Type() {
		return "", false, fmt.Errorf("hidden alias %q and public input %q have incompatible types", flag.Name, targetName)
	}
	return targetName, true, nil
}
