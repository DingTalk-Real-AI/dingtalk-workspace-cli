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
	"reflect"
	"testing"

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
  "schema_version": 1,
  "rules": {"excluded_command_subtrees": [], "excluded_flags": []},
  "commands": [],
  "future_field": true
}`)
	if _, err := Read(input); err == nil {
		t.Fatal("Read accepted an unknown field")
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
