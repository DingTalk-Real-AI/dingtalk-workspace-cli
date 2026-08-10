package helpers

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/runtimeannotate"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
)

func registerHelperFlagAliases(cmd *cobra.Command, canonical string, aliases ...string) {
	if cmd == nil || canonical == "" || len(aliases) == 0 {
		return
	}
	flags := cmd.Flags()
	canonicalFlag := flags.Lookup(canonical)
	if canonicalFlag == nil {
		return
	}
	for _, alias := range aliases {
		if alias == "" || alias == canonical {
			continue
		}
		aliasFlag := flags.Lookup(alias)
		if aliasFlag == nil {
			flags.String(alias, "", canonicalFlag.Usage+" (alias)")
			aliasFlag = flags.Lookup(alias)
		}
		if aliasFlag == nil {
			continue
		}
		if aliasFlag.Value.Type() != canonicalFlag.Value.Type() {
			panic(fmt.Sprintf(
				"flag %q alias %q type = %s, want %s",
				canonical, alias, aliasFlag.Value.Type(), canonicalFlag.Value.Type()))
		}
		aliasFlag.Hidden = true
		runtimeannotate.SetFlagAnnotation(
			aliasFlag,
			runtimeannotate.AnnotationFlagAliasOf,
			canonical,
		)
		runtimeannotate.SetFlagAnnotation(
			aliasFlag,
			runtimeannotate.AnnotationFlagAliasOrigin,
			runtimeannotate.FlagAliasOriginCorecmdV1,
		)
	}
}

func syncHelperFlagAliases(cmd *cobra.Command) error {
	if cmd == nil {
		return nil
	}
	return syncHelperFlagAliasesInSet(cmd.Flags())
}

func syncHelperFlagAliasesInSet(flags *pflag.FlagSet) error {
	if flags == nil {
		return nil
	}
	var firstErr error
	flags.VisitAll(func(aliasFlag *pflag.Flag) {
		if firstErr != nil || aliasFlag == nil {
			return
		}
		canonical := helperAliasTarget(aliasFlag)
		if canonical == "" {
			return
		}
		canonicalFlag := flags.Lookup(canonical)
		if canonicalFlag == nil {
			return
		}
		if canonicalFlag.Changed && aliasFlag.Changed {
			if canonicalFlag.Value.String() != aliasFlag.Value.String() {
				firstErr = apperrors.NewValidation(fmt.Sprintf(
					"--%s conflicts with --%s; pass only one spelling or use the same value",
					canonicalFlag.Name, aliasFlag.Name))
			}
			return
		}
		if aliasFlag.Changed && !canonicalFlag.Changed {
			firstErr = flags.Set(canonicalFlag.Name, aliasFlag.Value.String())
			return
		}
		if canonicalFlag.Changed && !aliasFlag.Changed {
			firstErr = flags.Set(aliasFlag.Name, canonicalFlag.Value.String())
		}
	})
	return firstErr
}

func helperAliasTarget(flag *pflag.Flag) string {
	if flag == nil {
		return ""
	}
	aliasOf := exactHelperFlagAnnotation(flag, runtimeannotate.AnnotationFlagAliasOf)
	origin := exactHelperFlagAnnotation(flag, runtimeannotate.AnnotationFlagAliasOrigin)
	if aliasOf == "" || origin != runtimeannotate.FlagAliasOriginCorecmdV1 {
		return ""
	}
	return aliasOf
}

func exactHelperFlagAnnotation(flag *pflag.Flag, key string) string {
	values := flag.Annotations[key]
	if len(values) != 1 {
		return ""
	}
	if values[0] == "" || values[0] != strings.TrimSpace(values[0]) {
		return ""
	}
	return values[0]
}
