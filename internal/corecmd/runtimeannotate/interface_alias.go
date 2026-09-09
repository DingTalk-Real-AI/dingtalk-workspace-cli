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
