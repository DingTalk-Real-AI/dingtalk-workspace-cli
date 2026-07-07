package cmdutil

import "github.com/spf13/cobra"

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
