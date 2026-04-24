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
