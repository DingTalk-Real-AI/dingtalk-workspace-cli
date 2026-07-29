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
	"errors"
	"io"
	"strings"
	"testing"

	authpkg "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/auth"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/agentproduct"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
	"github.com/spf13/cobra"
)

func TestUnsetAgentProductKeepsOpenSourceDefault(t *testing.T) {
	t.Setenv("DWS_CONFIG_DIR", t.TempDir())
	t.Setenv(agentproduct.EnvName, "")

	headers := resolveIdentityHeaders()
	if got := headers[agentproduct.HeaderName]; got != edition.DefaultOSSClawType {
		t.Fatalf("%s = %q, want %q", agentproduct.HeaderName, got, edition.DefaultOSSClawType)
	}
}

func TestParseAgentProductReturnsStableValidationError(t *testing.T) {
	const invalidValue = "DO_NOT ECHO"

	got, err := parseAgentProduct(invalidValue)
	if got != "" {
		t.Fatalf("parseAgentProduct() = %q, want empty", got)
	}

	var appErr *apperrors.Error
	if !errors.As(err, &appErr) {
		t.Fatalf("parseAgentProduct() error type = %T, want *errors.Error", err)
	}
	if appErr.Category != apperrors.CategoryValidation {
		t.Fatalf("category = %q, want validation", appErr.Category)
	}
	if appErr.Reason != "invalid_agent_product" {
		t.Fatalf("reason = %q, want invalid_agent_product", appErr.Reason)
	}
	if strings.Contains(err.Error(), invalidValue) {
		t.Fatalf("error must not echo invalid value: %v", err)
	}
}

func TestResolveIdentityHeadersAgentProductPrecedence(t *testing.T) {
	t.Setenv("DWS_CONFIG_DIR", t.TempDir())

	oldEdition := edition.Get()
	t.Cleanup(func() { edition.Override(oldEdition) })
	edition.Override(&edition.Hooks{
		MergeHeaders: func(headers map[string]string) map[string]string {
			headers[agentproduct.HeaderName] = "wukong"
			headers["x-edition-header"] = "preserved"
			return headers
		},
		EnterpriseCredentialHeaders: func(headers map[string]string) map[string]string {
			headers[agentproduct.HeaderName] = "enterprise-default"
			headers["x-enterprise-header"] = "preserved"
			return headers
		},
	})

	t.Run("unset keeps edition default", func(t *testing.T) {
		t.Setenv(agentproduct.EnvName, "")
		headers := resolveIdentityHeaders()
		if got := headers[agentproduct.HeaderName]; got != "wukong" {
			t.Fatalf("%s = %q, want wukong", agentproduct.HeaderName, got)
		}
	})

	t.Run("valid override is final", func(t *testing.T) {
		t.Setenv(agentproduct.EnvName, " qwenwork ")
		headers := resolveIdentityHeaders()
		if got := headers[agentproduct.HeaderName]; got != "qwenwork" {
			t.Fatalf("%s = %q, want qwenwork", agentproduct.HeaderName, got)
		}
		if got := headers["x-edition-header"]; got != "preserved" {
			t.Fatalf("edition header = %q, want preserved", got)
		}
		if got := headers["x-enterprise-header"]; got != "preserved" {
			t.Fatalf("enterprise header = %q, want preserved", got)
		}
		if got := headers["x-dingtalk-source"]; got != "github" {
			t.Fatalf("x-dingtalk-source = %q, want github", got)
		}
	})

	t.Run("invalid library input falls back to edition", func(t *testing.T) {
		t.Setenv(agentproduct.EnvName, "qwen work")
		headers := resolveIdentityHeaders()
		if got := headers[agentproduct.HeaderName]; got != "wukong" {
			t.Fatalf("%s = %q, want wukong", agentproduct.HeaderName, got)
		}
	})
}

func TestApplyAgentProductOverrideAllocatesHeaders(t *testing.T) {
	t.Setenv(agentproduct.EnvName, "qwenwork")

	headers := applyAgentProductOverride(nil)
	if got := headers[agentproduct.HeaderName]; got != "qwenwork" {
		t.Fatalf("%s = %q, want qwenwork", agentproduct.HeaderName, got)
	}
}

func TestRootRejectsInvalidAgentProductBeforeEditionHook(t *testing.T) {
	t.Setenv("DWS_CONFIG_DIR", t.TempDir())
	const invalidValue = "DO_NOT ECHO"
	t.Setenv(agentproduct.EnvName, invalidValue)

	oldEdition := edition.Get()
	t.Cleanup(func() { edition.Override(oldEdition) })

	hookCalled := false
	edition.Override(&edition.Hooks{
		AfterPersistentPreRun: func(_ *cobra.Command, _ []string) error {
			hookCalled = true
			return nil
		},
	})

	root := NewRootCommand()
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"version"})
	err := root.Execute()
	if err == nil {
		t.Fatal("root command accepted invalid DWS_AGENT_PRODUCT")
	}
	if hookCalled {
		t.Fatal("edition AfterPersistentPreRun ran before DWS_AGENT_PRODUCT validation")
	}

	var appErr *apperrors.Error
	if !errors.As(err, &appErr) {
		t.Fatalf("root error type = %T, want *errors.Error", err)
	}
	if appErr.Category != apperrors.CategoryValidation || appErr.Reason != "invalid_agent_product" {
		t.Fatalf("root error = category %q reason %q", appErr.Category, appErr.Reason)
	}
	if strings.Contains(err.Error(), invalidValue) {
		t.Fatalf("root error must not echo invalid value: %v", err)
	}
}

func TestEffectiveClawTypeDoesNotInvokeEnterpriseCredentialHeaders(t *testing.T) {
	oldEdition := edition.Get()
	t.Cleanup(func() { edition.Override(oldEdition) })

	hookCalled := false
	edition.Override(&edition.Hooks{
		MergeHeaders: func(headers map[string]string) map[string]string {
			headers[agentproduct.HeaderName] = "wukong"
			return headers
		},
		EnterpriseCredentialHeaders: func(headers map[string]string) map[string]string {
			hookCalled = true
			headers[agentproduct.HeaderName] = "enterprise-default"
			return headers
		},
	})

	t.Setenv(agentproduct.EnvName, "")
	if got := effectiveClawType(); got != "wukong" {
		t.Fatalf("effectiveClawType() = %q, want wukong", got)
	}
	if hookCalled {
		t.Fatal("EnterpriseCredentialHeaders hook ran during PAT error serialization")
	}
}

func TestAgentProductHeaderIsSeparateFromMessageClawType(t *testing.T) {
	t.Setenv("DWS_CONFIG_DIR", t.TempDir())
	t.Setenv(agentproduct.EnvName, "qwenwork")

	oldEdition := edition.Get()
	t.Cleanup(func() { edition.Override(oldEdition) })
	edition.Override(&edition.Hooks{
		ClawTypeValue: "message-brand",
		MergeHeaders: func(headers map[string]string) map[string]string {
			headers[agentproduct.HeaderName] = "wukong"
			return headers
		},
	})

	if got := resolveIdentityHeaders()[agentproduct.HeaderName]; got != "qwenwork" {
		t.Fatalf("HTTP %s = %q, want qwenwork", agentproduct.HeaderName, got)
	}
	if got := edition.ClawType(); got != "message-brand" {
		t.Fatalf("message clawType = %q, want message-brand", got)
	}
}

func TestResolveIdentityHeadersRestoresAgentProductAfterNilCredentialHeaders(t *testing.T) {
	t.Setenv("DWS_CONFIG_DIR", t.TempDir())
	t.Setenv(agentproduct.EnvName, "qwenwork")

	oldEdition := edition.Get()
	t.Cleanup(func() { edition.Override(oldEdition) })

	credentialHookCalled := false
	edition.Override(&edition.Hooks{
		MergeHeaders: func(headers map[string]string) map[string]string {
			headers[agentproduct.HeaderName] = "wukong"
			return headers
		},
		EnterpriseCredentialHeaders: func(map[string]string) map[string]string {
			credentialHookCalled = true
			return nil
		},
	})

	headers := resolveIdentityHeaders()
	if !credentialHookCalled {
		t.Fatal("EnterpriseCredentialHeaders hook was not called")
	}
	if got := headers[agentproduct.HeaderName]; got != "qwenwork" {
		t.Fatalf("%s = %q, want qwenwork", agentproduct.HeaderName, got)
	}
}

func TestEffectiveClawTypeUsesAgentProductOverride(t *testing.T) {
	oldEdition := edition.Get()
	t.Cleanup(func() { edition.Override(oldEdition) })
	edition.Override(&edition.Hooks{
		MergeHeaders: func(headers map[string]string) map[string]string {
			headers[agentproduct.HeaderName] = "wukong"
			return headers
		},
	})

	t.Setenv(agentproduct.EnvName, "qwenwork")
	if got := effectiveClawType(); got != "qwenwork" {
		t.Fatalf("effectiveClawType() = %q, want qwenwork", got)
	}
	t.Setenv(authpkg.AgentCodeEnv, "agent-code")
	if got := apperrors.HostControlBlock()["clawType"]; got != "qwenwork" {
		t.Fatalf("hostControl.clawType = %q, want qwenwork", got)
	}

	t.Setenv(agentproduct.EnvName, "")
	if got := effectiveClawType(); got != "wukong" {
		t.Fatalf("effectiveClawType() = %q, want wukong", got)
	}

	t.Setenv(agentproduct.EnvName, "invalid product")
	if got := effectiveClawType(); got != "wukong" {
		t.Fatalf("effectiveClawType() with invalid env = %q, want wukong", got)
	}
}
