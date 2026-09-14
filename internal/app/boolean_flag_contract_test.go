// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package app

import (
	stderrors "errors"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/pipeline"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/pipeline/handlers"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

func TestAllDistributionBooleanFlagTypesNormalizeDetachedLiterals(t *testing.T) {
	root := NewSchemaSourceRootCommand()
	unique := make(map[string]pipeline.FlagInfo)
	var visit func(*cobra.Command)
	visit = func(command *cobra.Command) {
		for _, spec := range pipeline.FlagInfoFromCommand(command) {
			if spec.Type != "bool" && spec.Type != "boolean" {
				continue
			}
			key := strings.Join([]string{spec.Name, spec.Shorthand, spec.Type}, "\x00")
			unique[key] = spec
		}
		for _, child := range command.Commands() {
			visit(child)
		}
	}
	visit(root)

	keys := make([]string, 0, len(unique))
	for key := range unique {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	if len(keys) < 80 {
		t.Fatalf("boolean flag contract coverage is unexpectedly small: %d", len(keys))
	}

	for _, key := range keys {
		spec := unique[key]
		for _, value := range []string{"true", "false"} {
			t.Run(spec.Name+"/"+value, func(t *testing.T) {
				ctx := &pipeline.Context{
					Command:   "dws contract probe",
					Args:      []string{"--" + spec.Name, value},
					FlagSpecs: []pipeline.FlagInfo{spec},
				}
				if err := (handlers.BoolValueHandler{}).Handle(ctx); err != nil {
					t.Fatalf("BoolValueHandler.Handle() error = %v", err)
				}
				want := []string{"--" + spec.Name + "=" + value}
				if !reflect.DeepEqual(ctx.Args, want) {
					t.Fatalf("normalized args = %v, want %v", ctx.Args, want)
				}

				flags := pflag.NewFlagSet(spec.Name, pflag.ContinueOnError)
				flags.Bool(spec.Name, false, "")
				if err := flags.Parse(ctx.Args); err != nil {
					t.Fatalf("pflag rejected normalized args %v: %v", ctx.Args, err)
				}
				got, err := flags.GetBool(spec.Name)
				if err != nil || got != (value == "true") || !flags.Changed(spec.Name) {
					t.Fatalf("parsed %s = %v, changed=%v, error=%v", spec.Name, got, flags.Changed(spec.Name), err)
				}
			})
		}
	}
	t.Logf("verified detached boolean syntax for %d distinct distribution flag contracts", len(keys))
}

func TestBooleanSyntaxPreservesDefaultsRequiredAndChangedContracts(t *testing.T) {
	tests := []struct {
		name        string
		path        string
		flag        string
		value       string
		wantDefault string
		wantValue   string
	}{
		{name: "root default false", path: "chat bot find", flag: "dry-run", value: "false", wantDefault: "false", wantValue: "false"},
		{name: "root mock default false", path: "chat bot find", flag: "mock", value: "true", wantDefault: "false", wantValue: "true"},
		{name: "local force default false", path: "upgrade", flag: "force", value: "false", wantDefault: "false", wantValue: "false"},
		{name: "local default true", path: "sheet find", flag: "match-case", value: "false", wantDefault: "true", wantValue: "false"},
		{name: "csv auto convert explicit false", path: "sheet csv-put", flag: "auto-convert", value: "false", wantDefault: "true", wantValue: "false"},
		{name: "required explicit false", path: "contact dept create", flag: "create-dept-group", value: "false", wantDefault: "false", wantValue: "false"},
		{name: "changed false remains explicit", path: "sheet csv-put", flag: "allow-overwrite", value: "false", wantDefault: "false", wantValue: "false"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root := NewSchemaSourceRootCommand()
			leaf := resolveParamLeaf(root, test.path)
			if leaf == nil {
				t.Fatalf("command %q is not runnable", test.path)
			}
			flag := booleanContractFlag(leaf, test.flag)
			if flag == nil || flag.DefValue != test.wantDefault || flag.Changed {
				t.Fatalf("initial --%s contract = %#v, want default %q and unchanged", test.flag, flag, test.wantDefault)
			}

			pathArgs := strings.Fields(test.path)
			rawArgs := append(append([]string(nil), pathArgs...), "--"+test.flag, test.value)
			ctx, err := pipeline.RunPreParseArgs(root, newPipelineEngine(), rawArgs)
			if err != nil {
				t.Fatalf("RunPreParseArgs(%v) error = %v", rawArgs, err)
			}
			if ctx == nil {
				t.Fatal("RunPreParseArgs returned nil context")
			}
			flagArgs := ctx.Args[len(pathArgs):]
			if err := leaf.ParseFlags(flagArgs); err != nil {
				t.Fatalf("ParseFlags(%v) error = %v", flagArgs, err)
			}
			flag = booleanContractFlag(leaf, test.flag)
			if flag == nil || flag.Value.String() != test.wantValue || !flag.Changed {
				t.Fatalf("final --%s contract = %#v, want value %q and changed", test.flag, flag, test.wantValue)
			}
		})
	}
}

func TestDetachedDryRunValuesReachTheExpectedFinalDispatchBoundary(t *testing.T) {
	base := []string{
		"mail", "folder", "update",
		"--email", "fixture@example.com", "--id", "folder-1", "--name", "Fixture Folder",
	}

	bareArgs := append(append([]string(nil), base...), "--dry-run")
	_, barePreview, bareAttempts, bareErr := executeParamAliasDryRunE2E(t, bareArgs...)
	if bareErr != nil || !barePreview.DryRun || barePreview.Executed || len(bareAttempts) != 0 {
		t.Fatalf("bare dry-run = preview:%#v attempts:%#v error:%v", barePreview, bareAttempts, bareErr)
	}

	trueArgs := append(append([]string(nil), base...), "--dry-run", "TRUE")
	trueCtx, truePreview, trueAttempts, trueErr := executeParamAliasDryRunE2E(t, trueArgs...)
	if trueErr != nil || !reflect.DeepEqual(truePreview, barePreview) || len(trueAttempts) != 0 {
		t.Fatalf("detached true = context:%#v preview:%#v attempts:%#v error:%v", trueCtx, truePreview, trueAttempts, trueErr)
	}
	if !hasBooleanCorrection(trueCtx, "--dry-run TRUE", "--dry-run=true") {
		t.Fatalf("detached true correction = %#v", trueCtx)
	}

	falseCases := []struct {
		name string
		args []string
	}{
		{name: "detached", args: append(append([]string(nil), base...), "--dry-run", "false")},
		{name: "explicit", args: append(append([]string(nil), base...), "--dry-run=false")},
	}
	var wantAttempts []any
	for _, test := range falseCases {
		t.Run(test.name, func(t *testing.T) {
			ctx, _, attempts, err := executeParamAliasDryRunE2E(t, test.args...)
			if err == nil || !strings.Contains(err.Error(), "dry-run reached the injected command runner") {
				t.Fatalf("dry-run=false dispatch error = %v", err)
			}
			if len(attempts) != 1 || attempts[0].DryRun {
				t.Fatalf("dry-run=false attempts = %#v", attempts)
			}
			if test.name == "detached" && !hasBooleanCorrection(ctx, "--dry-run false", "--dry-run=false") {
				t.Fatalf("detached false correction = %#v", ctx)
			}
			serialized := []any{attempts[0].CanonicalProduct, attempts[0].Tool, attempts[0].Params, attempts[0].DryRun}
			if wantAttempts == nil {
				wantAttempts = serialized
			} else if !reflect.DeepEqual(serialized, wantAttempts) {
				t.Fatalf("detached and explicit false dispatch differ\nwant=%#v\ngot=%#v", wantAttempts, serialized)
			}
		})
	}
}

func TestContradictoryBooleanValuesFailBeforeDestructiveDispatch(t *testing.T) {
	caller := &paramAliasCaptureCaller{}
	ctx, err := executeParamAliasE2E(t, caller,
		"mail", "thread", "trash",
		"--email", "user@example.com", "--id", "conversation-1",
		"--yes", "true", "--yes=false",
	)
	var conflict *pipeline.BoolValueConflictError
	if !stderrors.As(err, &conflict) {
		t.Fatalf("conflicting confirmation error = %v, want BoolValueConflictError (ctx=%#v)", err, ctx)
	}
	if conflict.Flag != "yes" || !reflect.DeepEqual(conflict.Values, []string{"false", "true"}) {
		t.Fatalf("conflict = %#v", conflict)
	}
	if len(caller.calls) != 0 {
		t.Fatalf("conflicting confirmation reached destructive dispatch: %#v", caller.calls)
	}
}

func booleanContractFlag(command *cobra.Command, name string) *pflag.Flag {
	if command == nil {
		return nil
	}
	if flag := command.Flags().Lookup(name); flag != nil {
		return flag
	}
	return command.InheritedFlags().Lookup(name)
}

func hasBooleanCorrection(ctx *pipeline.Context, original, corrected string) bool {
	if ctx == nil {
		return false
	}
	for _, correction := range ctx.Corrections {
		if correction.Handler == "boolvalue" && correction.Original == original && correction.Corrected == corrected {
			return true
		}
	}
	return false
}
