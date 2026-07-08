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

// SourceAnnotation records where a command tree came from. Edition overlays
// use it to distinguish runtime-authored commands from helper fallbacks that
// happen to share the same top-level product name.
const SourceAnnotation = "dws.source"

// SourceEnvelope marks a command as authored by the runtime discovery envelope.
const SourceEnvelope = "envelope"

// MarkEnvelopeSource stamps cmd with runtime discovery provenance.
func MarkEnvelopeSource(cmd *cobra.Command) {
	if cmd == nil {
		return
	}
	if cmd.Annotations == nil {
		cmd.Annotations = map[string]string{}
	}
	cmd.Annotations[SourceAnnotation] = SourceEnvelope
}

// IsEnvelopeSourced reports whether cmd was authored by the runtime discovery
// envelope.
func IsEnvelopeSourced(cmd *cobra.Command) bool {
	return cmd != nil && cmd.Annotations[SourceAnnotation] == SourceEnvelope
}

// KindAnnotation is the annotation key for marking command kinds.
const KindAnnotation = "dws.kind"

// KindGroup marks a command created as a group container.
const KindGroup = "group"

// MarkGroup stamps cmd as a group container.
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
