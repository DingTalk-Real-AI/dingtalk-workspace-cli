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

import "github.com/spf13/cobra"

// SourceAnnotation is the cobra.Command.Annotations key used to record where
// a top-level command came from. Edition overlays (e.g. wukong) read this
// annotation to distinguish envelope-authored dynamic commands from
// helper-fallback commands that merely happen to share a name. Keeping the
// key and value literals in one place prevents spelling drift between the
// core (which sets the annotation) and overlays (which read it).
const SourceAnnotation = "dws.source"

// SourceEnvelope marks a command as authored by the discovery envelope and
// therefore authoritative at runtime. Only commands built from a
// market.ServerDescriptor / CLIOverlay should carry this value. Helper
// fallbacks and other sources must leave the annotation unset.
const SourceEnvelope = "envelope"

// MarkEnvelopeSource stamps the envelope provenance annotation on cmd.
// Safe to call on commands that may not have an Annotations map yet.
// Callers in core code are the only ones that should invoke this — overlays
// read the annotation but must not fabricate envelope provenance.
func MarkEnvelopeSource(cmd *cobra.Command) {
	if cmd == nil {
		return
	}
	if cmd.Annotations == nil {
		cmd.Annotations = map[string]string{}
	}
	cmd.Annotations[SourceAnnotation] = SourceEnvelope
}

// IsEnvelopeSourced reports whether cmd carries the envelope provenance
// annotation. Commands without the annotation are treated as non-authoritative
// (helper fallbacks, overlay-injected stubs, etc.).
func IsEnvelopeSourced(cmd *cobra.Command) bool {
	if cmd == nil || cmd.Annotations == nil {
		return false
	}
	return cmd.Annotations[SourceAnnotation] == SourceEnvelope
}

// KindAnnotation records the structural role of a command. It distinguishes a
// pure group container (a heading whose RunE only prints help) from a runnable
// leaf. Both have a RunE set, so cobra's Runnable() cannot tell them apart —
// this annotation can. Kept here next to SourceAnnotation so the literals stay
// in one place.
const KindAnnotation = "dws.kind"

// KindGroup marks a command created as a group container (see NewGroupCommand).
const KindGroup = "group"

// MarkGroup stamps cmd as a group container. Safe on a command without an
// existing Annotations map.
func MarkGroup(cmd *cobra.Command) {
	if cmd == nil {
		return
	}
	if cmd.Annotations == nil {
		cmd.Annotations = map[string]string{}
	}
	cmd.Annotations[KindAnnotation] = KindGroup
}

// IsGroup reports whether cmd was created as a group container.
func IsGroup(cmd *cobra.Command) bool {
	return cmd != nil && cmd.Annotations[KindAnnotation] == KindGroup
}
