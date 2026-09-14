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

package interfacesnapshot

import (
	"bytes"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/runtimeannotate"
	"github.com/spf13/cobra"
)

func TestCrossPlatformCoverageCaptureUsesStableNoiseRulesAndFlagScopes(t *testing.T) {
	root := &cobra.Command{Use: "dws", Version: "test"}
	root.PersistentFlags().String("profile", "", "profile")

	service := &cobra.Command{
		Use:        "service",
		Aliases:    []string{"svc", "api", "svc"},
		Hidden:     true,
		Deprecated: "use replacement",
	}
	leaf := &cobra.Command{Use: "run", Run: func(*cobra.Command, []string) {}}
	leaf.Flags().String("name", "", "name")
	leaf.Flags().Bool("force", false, "force")
	if err := leaf.MarkFlagRequired("name"); err != nil {
		t.Fatalf("MarkFlagRequired: %v", err)
	}
	leaf.InitDefaultHelpFlag()
	service.AddCommand(leaf)

	help := &cobra.Command{Use: "help"}
	completion := &cobra.Command{Use: "completion"}
	completion.AddCommand(&cobra.Command{Use: "zsh"})
	root.AddCommand(service, help, completion)

	snapshot := Capture(root)
	wantPaths := []string{"dws", "dws service", "dws service run"}
	if got := sortedCommandPaths(snapshot.Commands); !reflect.DeepEqual(got, wantPaths) {
		t.Fatalf("paths = %v, want %v", got, wantPaths)
	}

	serviceSnapshot := commandIndex(snapshot)["dws service"]
	if !serviceSnapshot.Hidden || serviceSnapshot.Deprecated != "use replacement" {
		t.Fatalf("service metadata = %#v", serviceSnapshot)
	}
	if want := []string{"api", "svc"}; !reflect.DeepEqual(serviceSnapshot.Aliases, want) {
		t.Fatalf("aliases = %v, want %v", serviceSnapshot.Aliases, want)
	}

	leafSnapshot := commandIndex(snapshot)["dws service run"]
	if !leafSnapshot.Runnable {
		t.Fatal("runnable leaf was not recorded as runnable")
	}
	local := flagIndex(leafSnapshot.LocalFlags)
	if len(local) != 2 {
		t.Fatalf("local flags = %#v, want two business flags and no auto help flag", leafSnapshot.LocalFlags)
	}
	if local["name"].Type != "string" || !local["name"].Required {
		t.Fatalf("name flag = %#v, want required string", local["name"])
	}
	if local["force"].Type != "bool" || local["force"].Required {
		t.Fatalf("force flag = %#v, want optional bool", local["force"])
	}
	inherited := flagIndex(leafSnapshot.InheritedFlags)
	if inherited["profile"].Type != "string" {
		t.Fatalf("inherited flags = %#v, want root --profile", leafSnapshot.InheritedFlags)
	}
	if _, exists := inherited["help"]; exists {
		t.Fatal("auto --help leaked into inherited flags")
	}

	var first, second bytes.Buffer
	if err := Write(&first, snapshot); err != nil {
		t.Fatalf("first Write: %v", err)
	}
	if err := Write(&second, Capture(root)); err != nil {
		t.Fatalf("second Write: %v", err)
	}
	if !bytes.Equal(first.Bytes(), second.Bytes()) {
		t.Fatal("capturing the same Cobra root twice produced different JSON")
	}
}

func TestCrossPlatformCoverageReadRejectsUnknownSnapshotFields(t *testing.T) {
	input := bytes.NewBufferString(`{
	  "schema_version": 3,
	  "rules": {"excluded_command_subtrees": [], "excluded_flags": []},
	  "commands": [],
	  "future_field": true
}`)
	if _, err := Read(input); err == nil {
		t.Fatal("Read accepted an unknown field")
	}
}

func TestCrossPlatformCoverageCaptureRoundTripsFrameworkBoolConstParams(t *testing.T) {
	root := &cobra.Command{Use: "dws"}
	leaf := corecmd.New(corecmd.Spec{
		Use:         "send",
		ConstParams: map[string]any{"convThreadEnabled": true, "precheckOnly": false},
		Invoke:      func(*corecmd.Ctx, map[string]any) error { return nil },
	})
	root.AddCommand(leaf)

	snapshot := Capture(root)
	got := commandIndex(snapshot)["dws send"].BoolConstParams
	want := map[string]bool{"convThreadEnabled": true, "precheckOnly": false}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("captured bool ConstParams = %#v, want %#v", got, want)
	}

	var encoded bytes.Buffer
	if err := Write(&encoded, snapshot); err != nil {
		t.Fatalf("Write: %v", err)
	}
	decoded, err := Read(bytes.NewReader(encoded.Bytes()))
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if got := commandIndex(decoded)["dws send"].BoolConstParams; !reflect.DeepEqual(got, want) {
		t.Fatalf("round-tripped bool ConstParams = %#v, want %#v", got, want)
	}
}

func TestCrossPlatformCoverageCaptureIgnoresHandwrittenLegacyConstParamsAnnotations(t *testing.T) {
	root := &cobra.Command{Use: "dws"}
	direct := &cobra.Command{
		Use: "direct",
		Run: func(*cobra.Command, []string) {},
		Annotations: map[string]string{
			"dws.compat.bool_const_params":   `{"forged":true}`,
			"dws.compat.const_params_origin": "corecmd.const_params.v1",
		},
	}
	businessConstructor := func() *cobra.Command {
		payloadKey := strings.Join([]string{"dws.compat.bool_", "const_params"}, "")
		originKey := strings.Join([]string{"dws.compat.const_", "params_origin"}, "")
		origin := strings.Join([]string{"corecmd.const_", "params.v1"}, "")
		return &cobra.Command{
			Use: "business",
			Run: func(*cobra.Command, []string) {},
			Annotations: map[string]string{
				payloadKey: `{"forged":true}`,
				originKey:  origin,
			},
		}
	}
	root.AddCommand(direct, businessConstructor())

	snapshot := Capture(root)
	if err := snapshot.Validate(); err != nil {
		t.Fatalf("handwritten legacy annotations affected snapshot validity: %v", err)
	}
	for _, path := range []string{"dws direct", "dws business"} {
		if got := commandIndex(snapshot)[path].BoolConstParams; got != nil {
			t.Fatalf("%s forged bool ConstParams evidence = %#v", path, got)
		}
	}
	var encoded bytes.Buffer
	if err := Write(&encoded, snapshot); err != nil {
		t.Fatalf("Write snapshot without forged evidence: %v", err)
	}
}

func TestCrossPlatformCoverageReadRejectsMalformedBoolConstParams(t *testing.T) {
	for _, payload := range []string{`{"fixed":"true"}`, `{}`} {
		input := bytes.NewBufferString(`{
		  "schema_version": 3,
		  "rules": {"excluded_command_subtrees": [], "excluded_flags": []},
		  "commands": [{
		    "path": "dws send",
		    "runnable": true,
		    "aliases": [],
		    "local_flags": [],
		    "inherited_flags": [],
		    "bool_const_params": ` + payload + `
		  }]
		}`)
		if _, err := Read(input); err == nil {
			t.Fatalf("Read accepted malformed bool_const_params %s", payload)
		}
	}
}

func TestCrossPlatformCoverageCaptureRoundTripsFrameworkFlagAlias(t *testing.T) {
	if FlagAliasOfAnnotation != runtimeannotate.AnnotationFlagAliasOf {
		t.Fatalf(
			"flag alias annotation key drifted: snapshot=%q runtime=%q",
			FlagAliasOfAnnotation,
			runtimeannotate.AnnotationFlagAliasOf,
		)
	}
	root := &cobra.Command{Use: "dws"}
	leaf := &cobra.Command{Use: "send", Run: func(*cobra.Command, []string) {}}
	corecmd.RegisterFlags(leaf, []corecmd.FlagSpec{{
		Name:    "message-id",
		Usage:   "message ID",
		Aliases: []string{"open-conversation-id"},
	}})
	root.AddCommand(leaf)

	snapshot := Capture(root)
	flag := flagIndex(commandIndex(snapshot)["dws send"].LocalFlags)["open-conversation-id"]
	if flag.AliasOf != "message-id" {
		t.Fatalf("captured alias_of = %q, want message-id", flag.AliasOf)
	}

	var encoded bytes.Buffer
	if err := Write(&encoded, snapshot); err != nil {
		t.Fatalf("Write: %v", err)
	}
	decoded, err := Read(bytes.NewReader(encoded.Bytes()))
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	decodedFlag := flagIndex(commandIndex(decoded)["dws send"].LocalFlags)["open-conversation-id"]
	if decodedFlag.AliasOf != "message-id" {
		t.Fatalf("round-tripped alias_of = %q, want message-id", decodedFlag.AliasOf)
	}
}

func TestCrossPlatformCoverageCaptureRejectsUntrustedFlagAliasEvidence(t *testing.T) {
	for _, origin := range []string{"", "handwritten.alias.v1"} {
		t.Run(origin, func(t *testing.T) {
			root := &cobra.Command{Use: "dws"}
			leaf := &cobra.Command{Use: "send", Run: func(*cobra.Command, []string) {}}
			leaf.Flags().String("message-id", "", "canonical message ID")
			leaf.Flags().String("legacy-id", "", "legacy message ID")
			legacy := leaf.Flags().Lookup("legacy-id")
			legacy.Annotations = map[string][]string{
				FlagAliasOfAnnotation: {"message-id"},
			}
			if origin != "" {
				legacy.Annotations[runtimeannotate.AnnotationFlagAliasOrigin] = []string{origin}
			}
			root.AddCommand(leaf)

			if err := Capture(root).Validate(); err == nil {
				t.Fatal("Validate accepted alias evidence outside corecmd FlagSpec.Aliases")
			}
		})
	}
}

func TestCrossPlatformCoverageCaptureRejectsMultipleNonEmptyFlagAliases(t *testing.T) {
	root := &cobra.Command{Use: "dws"}
	leaf := &cobra.Command{Use: "send", Run: func(*cobra.Command, []string) {}}
	leaf.Flags().String("message-id", "", "canonical message ID")
	leaf.Flags().String("other-id", "", "other ID")
	leaf.Flags().String("legacy-id", "", "legacy ID")
	legacy := leaf.Flags().Lookup("legacy-id")
	legacy.Annotations = map[string][]string{
		FlagAliasOfAnnotation: {"message-id", "other-id"},
	}
	root.AddCommand(leaf)

	snapshot := Capture(root)
	if err := snapshot.Validate(); err == nil {
		t.Fatal("Validate accepted multiple non-empty alias annotation values")
	}
	var encoded bytes.Buffer
	if err := Write(&encoded, snapshot); err == nil {
		t.Fatal("Write accepted multiple non-empty alias annotation values")
	}
}

func TestCrossPlatformCoverageValidateRejectsMalformedFlagAliasState(t *testing.T) {
	tests := []struct {
		name    string
		aliasOf string
	}{
		{name: "self", aliasOf: "legacy-id"},
		{name: "leading dashes", aliasOf: "--message-id"},
		{name: "non exact", aliasOf: "message id"},
		{name: "missing target", aliasOf: "missing-id"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			snapshot := testSnapshot(
				testCommand("dws"),
				testCommandWithFlagScopes(
					"dws send",
					[]Flag{
						{Name: "legacy-id", Type: "string", AliasOf: test.aliasOf},
						{Name: "message-id", Type: "string"},
					},
					nil,
				),
			)
			if err := snapshot.Validate(); err == nil {
				t.Fatalf("Validate accepted alias_of %q", test.aliasOf)
			}
			encoded, err := json.Marshal(snapshot)
			if err != nil {
				t.Fatalf("json.Marshal: %v", err)
			}
			if _, err := Read(bytes.NewReader(encoded)); err == nil {
				t.Fatalf("Read accepted alias_of %q", test.aliasOf)
			}
		})
	}
}

func TestCrossPlatformCoverageValidateRejectsFlagAliasChainTypeAndScopeConflicts(t *testing.T) {
	tests := []struct {
		name      string
		local     []Flag
		inherited []Flag
	}{
		{
			name: "alias chain",
			local: []Flag{
				{Name: "legacy-id", Type: "string", AliasOf: "middle-id"},
				{Name: "middle-id", Type: "string", AliasOf: "message-id"},
				{Name: "message-id", Type: "string"},
			},
		},
		{
			name: "different types",
			local: []Flag{
				{Name: "legacy-id", Type: "string", AliasOf: "message-id"},
				{Name: "message-id", Type: "int"},
			},
		},
		{
			name:      "same flag in local and inherited scopes",
			local:     []Flag{{Name: "legacy-id", Type: "string"}},
			inherited: []Flag{{Name: "legacy-id", Type: "string"}},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			snapshot := testSnapshot(
				testCommand("dws"),
				testCommandWithFlagScopes("dws send", test.local, test.inherited),
			)
			if err := snapshot.Validate(); err == nil {
				t.Fatalf("Validate accepted invalid alias contract: %#v", snapshot.Commands[1])
			}
		})
	}
}

func TestCrossPlatformCoverageCompareBlocksCandidateSiblingAliasCollision(t *testing.T) {
	base := testSnapshot(testCommand("dws"))
	current := testSnapshot(
		testCommand("dws"),
		testCommandWithAliases("dws search", []string{"find"}),
		testCommand("dws find"),
	)
	comparison := Compare(current, base, "base")
	if comparison.Compatible || !hasChangeKind(comparison.Blocking, "command_alias_collision") {
		t.Fatalf("candidate sibling alias collision was not blocked: %#v", comparison)
	}
}
