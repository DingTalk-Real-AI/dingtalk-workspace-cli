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

package app

import (
	"bytes"
	"errors"
	"io"
	"os"
	"strings"
	"testing"

	authpkg "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/auth"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
	"github.com/spf13/cobra"
)

func TestCrossPlatformCoverageNewCommandRunnerDoesNotBackfillCredentialEnvironment(t *testing.T) {
	t.Setenv(authpkg.EnvClientID, "")
	t.Setenv(authpkg.EnvClientSecret, "")
	authpkg.SetClientID("persisted-looking-id")
	authpkg.SetClientSecret("")
	t.Cleanup(func() {
		authpkg.SetClientID("")
		authpkg.SetClientSecret("")
	})

	_ = newCommandRunnerWithFlags(&GlobalFlags{})
	if os.Getenv(authpkg.EnvClientID) != "" || os.Getenv(authpkg.EnvClientSecret) != "" {
		t.Fatalf("runner mutated credential env: id=%q secret_set=%t", os.Getenv(authpkg.EnvClientID), os.Getenv(authpkg.EnvClientSecret) != "")
	}
}

func TestCrossPlatformCoverageRootCredentialFlagsReplacePriorRuntimeTupleAtomically(t *testing.T) {
	t.Setenv(authpkg.EnvClientID, "")
	t.Setenv(authpkg.EnvClientSecret, "")
	t.Setenv("DWS_CONFIG_DIR", t.TempDir())
	t.Cleanup(CloseFileLogger)
	t.Cleanup(func() { authpkg.SetClientCredentials("", "") })

	authpkg.SetClientCredentials("old-client", "old-secret")
	root := NewRootCommand(t.Context())
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"version", "--client-id", "new-client"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	if got := authpkg.ClientID(); got != "new-client" {
		t.Fatalf("runtime Client ID = %q, want new-client", got)
	}
	if got := authpkg.ClientSecret(); got == "old-secret" {
		t.Fatal("half --client-id reused Client Secret from a previous invocation")
	}

	authpkg.SetClientCredentials("old-client", "old-secret")
	root = NewRootCommand(t.Context())
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"version", "--client-secret", "new-secret"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	if got := authpkg.ClientID(); got == "old-client" {
		t.Fatal("half --client-secret reused Client ID from a previous invocation")
	}
	if got := authpkg.ClientSecret(); got != "new-secret" {
		t.Fatalf("runtime Client Secret was not replaced, has_expected=%t", got == "new-secret")
	}
}

func TestCrossPlatformCoverageRootCredentialFlagsAreScopedToEachExecuteCInvocation(t *testing.T) {
	t.Setenv(authpkg.EnvClientID, "")
	t.Setenv(authpkg.EnvClientSecret, "")
	t.Setenv("DWS_CONFIG_DIR", t.TempDir())
	t.Cleanup(CloseFileLogger)
	t.Cleanup(func() { authpkg.SetClientCredentials("", "") })

	root := NewRootCommand(t.Context())
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	observed := addCredentialCaptureCommand(root)

	root.SetArgs([]string{"capture-credentials", "--client-id", "first-client", "--client-secret", "first-secret"})
	if _, err := root.ExecuteC(); err != nil {
		t.Fatal(err)
	}
	if observed.clientID != "first-client" || observed.clientSecret != "first-secret" {
		t.Fatalf("first invocation flags = id:%q secret_set:%t", observed.clientID, observed.clientSecret != "")
	}

	root.SetArgs([]string{"capture-credentials"})
	if _, err := root.ExecuteC(); err != nil {
		t.Fatal(err)
	}
	if observed.clientID != "" || observed.clientSecret != "" {
		t.Fatalf("second invocation retained credential flags: id:%q secret_set:%t", observed.clientID, observed.clientSecret != "")
	}

	root.SetArgs([]string{"capture-credentials", "--client-secret", "third-secret"})
	if _, err := root.ExecuteC(); err != nil {
		t.Fatal(err)
	}
	if observed.clientID != "" || observed.clientSecret != "third-secret" {
		t.Fatalf("half-pair flags = id:%q has_expected_secret:%t", observed.clientID, observed.clientSecret == "third-secret")
	}
}

func TestCrossPlatformCoverageRootCredentialFlagsAreClearedAfterExecutionErrors(t *testing.T) {
	t.Setenv(authpkg.EnvClientID, "")
	t.Setenv(authpkg.EnvClientSecret, "")
	t.Setenv("DWS_CONFIG_DIR", t.TempDir())
	t.Cleanup(CloseFileLogger)
	t.Cleanup(func() { authpkg.SetClientCredentials("", "") })

	root := NewRootCommand(t.Context())
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	observed := addCredentialCaptureCommand(root)
	root.AddCommand(&cobra.Command{
		Use: "credential-error",
		RunE: func(cmd *cobra.Command, args []string) error {
			return errors.New("expected execution failure")
		},
	})

	root.SetArgs([]string{"credential-error", "--client-id", "failed-client", "--client-secret", "failed-secret"})
	if _, err := root.ExecuteC(); err == nil {
		t.Fatal("expected command error")
	}

	root.SetArgs([]string{"capture-credentials"})
	if _, err := root.ExecuteC(); err != nil {
		t.Fatal(err)
	}
	if observed.clientID != "" || observed.clientSecret != "" {
		t.Fatalf("execution error leaked credential flags: id:%q secret_set:%t", observed.clientID, observed.clientSecret != "")
	}

	root.SetArgs([]string{"version", "--client-id", "partial", "--not-a-real-flag"})
	if _, err := root.ExecuteC(); err == nil {
		t.Fatal("expected flag parse error")
	}
	root.SetArgs([]string{"capture-credentials"})
	if _, err := root.ExecuteC(); err != nil {
		t.Fatal(err)
	}
	if observed.clientID != "" || observed.clientSecret != "" {
		t.Fatalf("flag parse error leaked credential flags: id:%q secret_set:%t", observed.clientID, observed.clientSecret != "")
	}
}

func TestCrossPlatformCoverageRootCredentialFlagsAreClearedOnPreRunExitPaths(t *testing.T) {
	t.Setenv(authpkg.EnvClientID, "")
	t.Setenv(authpkg.EnvClientSecret, "")
	t.Setenv("DWS_CONFIG_DIR", t.TempDir())
	t.Cleanup(CloseFileLogger)
	t.Cleanup(func() { authpkg.SetClientCredentials("", "") })

	assertNextInvocationDoesNotReuse := func(t *testing.T, root *cobra.Command, args []string, wantErr bool, leakedID, leakedSecret string) {
		t.Helper()
		observed := addCredentialCaptureCommand(root)
		root.SetArgs(args)
		_, err := root.ExecuteC()
		if wantErr && err == nil {
			t.Fatalf("ExecuteC(%v) error = nil", args)
		}
		if !wantErr && err != nil {
			t.Fatalf("ExecuteC(%v) error = %v", args, err)
		}

		root.SetArgs([]string{"capture-credentials"})
		if _, err := root.ExecuteC(); err != nil {
			t.Fatal(err)
		}
		if observed.clientID == leakedID {
			t.Fatalf("next invocation reused Client ID %q after %v", observed.clientID, args)
		}
		if observed.clientSecret == leakedSecret {
			t.Fatalf("next invocation reused Client Secret after %v", args)
		}
	}

	newRoot := func() *cobra.Command {
		root := NewRootCommand(t.Context())
		root.SetOut(io.Discard)
		root.SetErr(io.Discard)
		return root
	}

	t.Run("help", func(t *testing.T) {
		assertNextInvocationDoesNotReuse(t, newRoot(), []string{
			"version", "--client-id", "help-client", "--client-secret", "help-secret", "--help",
		}, false, "help-client", "help-secret")
	})
	t.Run("version flag", func(t *testing.T) {
		assertNextInvocationDoesNotReuse(t, newRoot(), []string{
			"--client-id", "version-client", "--client-secret", "version-secret", "--version",
		}, false, "version-client", "version-secret")
	})
	t.Run("args validation", func(t *testing.T) {
		assertNextInvocationDoesNotReuse(t, newRoot(), []string{
			"api", "GET", "/v1.0/microApp/allApps", "extra", "--client-id", "args-client", "--client-secret", "args-secret",
		}, true, "args-client", "args-secret")
	})
	t.Run("subcommand flag handler", func(t *testing.T) {
		assertNextInvocationDoesNotReuse(t, newRoot(), []string{
			"contact", "user", "get", "--client-id", "contact-client", "--client-secret", "contact-secret", "--not-a-real-flag",
		}, true, "contact-client", "contact-secret")
	})
}

func TestCrossPlatformCoverageRootVersionPreservesCobraEarlyExitCompatibility(t *testing.T) {
	t.Setenv("DWS_CONFIG_DIR", t.TempDir())
	t.Cleanup(CloseFileLogger)
	t.Setenv(authpkg.EnvClientID, "")
	t.Setenv(authpkg.EnvClientSecret, "")
	t.Setenv(envDWSAgentHost, "invalid-agent-host")
	t.Cleanup(func() { authpkg.SetClientCredentials("", "") })

	oldEdition := edition.Get()
	t.Cleanup(func() { edition.Override(oldEdition) })
	hookCalled := false
	edition.Override(&edition.Hooks{
		AfterPersistentPreRun: func(*cobra.Command, []string) error {
			hookCalled = true
			return errors.New("version must bypass this hook")
		},
	})

	root := NewRootCommand(t.Context())
	var stdout bytes.Buffer
	root.SetOut(&stdout)
	root.SetErr(io.Discard)
	observed := addCredentialCaptureCommand(root)
	root.SetArgs([]string{"--client-id", "version-client", "--client-secret", "version-secret", "--version", "unexpected-arg"})
	if _, err := root.ExecuteC(); err != nil {
		t.Fatalf("--version must ignore invalid Agent metadata and edition hooks: %v", err)
	}
	if got, want := stdout.String(), "dws version "+Version()+"\n"; got != want {
		t.Fatalf("--version output = %q, want %q", got, want)
	}
	if hookCalled {
		t.Fatal("--version called edition AfterPersistentPreRun")
	}

	stdout.Reset()
	root.SetArgs([]string{"--version=false", "unexpected-arg"})
	if _, err := root.ExecuteC(); err == nil {
		t.Fatal("--version=false bypassed root positional validation")
	}

	// A writer failure skips Cobra's PersistentPostRunE. The next parse must use
	// only its own --version Changed state, not the retained boolean value.
	t.Setenv(envDWSAgentHost, "")
	edition.Override(&edition.Hooks{})
	root.SetOut(runnerCredentialFailWriter{})
	root.SetArgs([]string{"--version"})
	if _, err := root.ExecuteC(); err == nil {
		t.Fatal("expected --version writer failure")
	}
	root.SetOut(&stdout)
	root.SetArgs([]string{"unexpected-arg"})
	if _, err := root.ExecuteC(); err == nil || !strings.Contains(err.Error(), "unexpected-arg") {
		t.Fatalf("next invocation bypassed positional validation after writer failure: %v", err)
	}

	// A later ordinary invocation on the same root must not inherit either the
	// version flag or credentials supplied to the short-circuited call.
	root.SetArgs([]string{"capture-credentials"})
	if _, err := root.ExecuteC(); err != nil {
		t.Fatal(err)
	}
	if observed.clientID == "version-client" {
		t.Fatalf("ordinary invocation reused --version Client ID %q", observed.clientID)
	}
	if observed.clientSecret == "version-secret" {
		t.Fatal("ordinary invocation reused --version Client Secret")
	}
}

func TestCrossPlatformCoverageRootInvocationNilGuardsAndDefaultHelp(t *testing.T) {
	consumeCredentialInvocationFlags(nil, nil, nil)
	consumeRootVersionInvocationFlag(nil, nil)
	installInvocationExitHandlers(nil, nil, nil, nil)

	root := newRootPresentationCommand()
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	if root.RunE == nil {
		t.Fatal("root default RunE is missing")
	}
	if err := root.RunE(root, nil); err != nil {
		t.Fatalf("root default help: %v", err)
	}
	if err := runRootHelp(root, nil); err != nil {
		t.Fatalf("root help helper: %v", err)
	}
}

type capturedCredentialFlags struct {
	clientID     string
	clientSecret string
}

func addCredentialCaptureCommand(root *cobra.Command) *capturedCredentialFlags {
	observed := &capturedCredentialFlags{}
	root.AddCommand(&cobra.Command{
		Use:  "capture-credentials",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			observed.clientID, _ = cmd.Flags().GetString("client-id")
			observed.clientSecret, _ = cmd.Flags().GetString("client-secret")
			return nil
		},
	})
	return observed
}

type runnerCredentialFailWriter struct{}

func (runnerCredentialFailWriter) Write([]byte) (int, error) {
	return 0, errors.New("expected writer failure")
}
