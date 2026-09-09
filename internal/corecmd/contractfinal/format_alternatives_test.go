package contractfinal

import (
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/runtimeannotate"
	"github.com/spf13/cobra"
	"testing"
)

func TestCrossPlatformCoverageParamFormatAlternativesValidation(t *testing.T) {
	for _, tc := range []struct {
		name              string
		branches          []contract.FormatAlternative
		nonstring, format bool
	}{
		{name: "single", branches: []contract.FormatAlternative{{Format: "date"}}},
		{name: "empty branch", branches: []contract.FormatAlternative{{Format: "date"}, {}}},
		{name: "duplicate", branches: []contract.FormatAlternative{{Format: "date"}, {Format: "date"}}},
		{name: "whitespace", branches: []contract.FormatAlternative{{Format: " date"}, {Format: "date-time"}}},
		{name: "nonstring", branches: []contract.FormatAlternative{{Format: "date"}, {Format: "date-time"}}, nonstring: true},
		{name: "top format", branches: []contract.FormatAlternative{{Format: "date"}, {Format: "date-time"}}, format: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cmd := &cobra.Command{Use: "sample"}
			cmd.Flags().String("first", "", "")
			if tc.nonstring {
				cmd.Flags().Int("time", 0, "")
			} else {
				cmd.Flags().String("time", "", "")
			}
			if tc.format {
				runtimeannotate.AnnotateRuntimeFlagFormat(cmd, "time", "date-time")
			}
			err := ApplyParamDecls(cmd, []contract.ParamDecl{{Name: "first", Property: "firstValue"}, {Name: "time", AnyOf: tc.branches}})
			if err == nil {
				t.Fatal("invalid anyOf accepted")
			}
			if len(cmd.Flags().Lookup("first").Annotations) != 0 {
				t.Fatal("invalid declaration partially applied")
			}
		})
	}
}

func TestCrossPlatformCoverageParamFormatAlternativesClone(t *testing.T) {
	cmd := &cobra.Command{Use: "sample"}
	input := contract.ContractFinalPayload{Parameters: []contract.ParamDecl{{Name: "time", AnyOf: []contract.FormatAlternative{{Format: "date"}, {Format: "date-time"}}}}}
	RegisterRuntimeContractFinal(cmd, input)
	input.Parameters[0].AnyOf[0].Format = "uri"
	got, _ := RuntimeContractFinal(cmd)
	if got.Parameters[0].AnyOf[0].Format != "date" {
		t.Fatal("registration retained caller slice")
	}
	got.Parameters[0].AnyOf[0].Format = "uri"
	again, _ := RuntimeContractFinal(cmd)
	if again.Parameters[0].AnyOf[0].Format != "date" {
		t.Fatal("read exposed stored slice")
	}
}

func TestCrossPlatformCoverageParamFormatAlternativesAnnotation(t *testing.T) {
	cmd := &cobra.Command{Use: "sample"}
	cmd.Flags().String("time", "", "")
	branches := []contract.FormatAlternative{{Format: "date-time"}, {Format: "date"}}
	if err := ApplyParamDecls(cmd, []contract.ParamDecl{{Name: "time", AnyOf: branches}}); err != nil {
		t.Fatal(err)
	}
	got := cmd.Flags().Lookup("time").Annotations[runtimeannotate.AnnotationFlagAnyOf]
	if len(got) != 1 || got[0] != `[{"format":"date"},{"format":"date-time"}]` {
		t.Fatalf("annotation=%v", got)
	}
	if branches[0].Format != "date-time" {
		t.Fatal("annotation sorting mutated declaration")
	}
}
