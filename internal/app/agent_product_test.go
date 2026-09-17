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

func TestUnsetAgentProductOmitsHeaderAndKeepsOpenSourceClawType(t *testing.T) {
	t.Setenv("DWS_CONFIG_DIR", t.TempDir())
	t.Setenv(agentproduct.EnvName, "")

	headers := resolveIdentityHeaders()
	if got := headers["claw-type"]; got != edition.DefaultOSSClawType {
		t.Fatalf("claw-type = %q, want %q", got, edition.DefaultOSSClawType)
	}
	if _, ok := headers[agentproduct.HeaderName]; ok {
		t.Fatalf("unset Product must omit %s", agentproduct.HeaderName)
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

func TestResolveIdentityHeadersSeparatesAgentProductFromClawType(t *testing.T) {
	t.Setenv("DWS_CONFIG_DIR", t.TempDir())

	oldEdition := edition.Get()
	t.Cleanup(func() { edition.Override(oldEdition) })
	edition.Override(&edition.Hooks{
		MergeHeaders: func(headers map[string]string) map[string]string {
			headers["claw-type"] = "wukong"
			headers[agentproduct.HeaderName] = "merge-product-must-not-win"
			headers["x-edition-header"] = "preserved"
			return headers
		},
		EnterpriseCredentialHeaders: func(headers map[string]string) map[string]string {
			headers["claw-type"] = "credential-must-not-win"
			headers[agentproduct.HeaderName] = "credential-product-must-not-win"
			headers["x-enterprise-header"] = "preserved"
			return headers
		},
	})

	t.Run("unset omits Product and keeps edition claw-type", func(t *testing.T) {
		t.Setenv(agentproduct.EnvName, "")
		headers := resolveIdentityHeaders()
		if got := headers["claw-type"]; got != "wukong" {
			t.Fatalf("claw-type = %q, want wukong", got)
		}
		if _, ok := headers[agentproduct.HeaderName]; ok {
			t.Fatalf("unset Product must omit %s", agentproduct.HeaderName)
		}
	})

	t.Run("valid Product is final without changing claw-type", func(t *testing.T) {
		t.Setenv(agentproduct.EnvName, " qwenwork ")
		headers := resolveIdentityHeaders()
		if got := headers[agentproduct.HeaderName]; got != "qwenwork" {
			t.Fatalf("%s = %q, want qwenwork", agentproduct.HeaderName, got)
		}
		if got := headers["claw-type"]; got != "wukong" {
			t.Fatalf("claw-type = %q, want wukong", got)
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

	t.Run("invalid library input omits Product and keeps edition claw-type", func(t *testing.T) {
		t.Setenv(agentproduct.EnvName, "qwen work")
		headers := resolveIdentityHeaders()
		if got := headers["claw-type"]; got != "wukong" {
			t.Fatalf("claw-type = %q, want wukong", got)
		}
		if _, ok := headers[agentproduct.HeaderName]; ok {
			t.Fatalf("invalid Product must omit %s", agentproduct.HeaderName)
		}
	})
}

func TestApplyAgentProductHeader(t *testing.T) {
	t.Run("valid value allocates headers", func(t *testing.T) {
		t.Setenv(agentproduct.EnvName, "qwenwork")

		headers := applyAgentProductHeader(nil)
		if got := headers[agentproduct.HeaderName]; got != "qwenwork" {
			t.Fatalf("%s = %q, want qwenwork", agentproduct.HeaderName, got)
		}
	})

	for _, tc := range []struct {
		name  string
		value string
	}{
		{name: "empty value", value: ""},
		{name: "invalid value", value: "qwen work"},
	} {
		t.Run(tc.name+" removes inherited header", func(t *testing.T) {
			t.Setenv(agentproduct.EnvName, tc.value)
			headers := applyAgentProductHeader(map[string]string{
				agentproduct.HeaderName: "must-not-leak",
				"x-preserved":           "yes",
			})
			if _, ok := headers[agentproduct.HeaderName]; ok {
				t.Fatalf("%s must be omitted", agentproduct.HeaderName)
			}
			if got := headers["x-preserved"]; got != "yes" {
				t.Fatalf("x-preserved = %q, want yes", got)
			}
		})
	}
}

func TestCrossPlatformCoverageRootRejectsInvalidAgentProductBeforeEditionHook(t *testing.T) {
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
			headers["claw-type"] = "wukong"
			return headers
		},
		EnterpriseCredentialHeaders: func(headers map[string]string) map[string]string {
			hookCalled = true
			headers["claw-type"] = "enterprise-default"
			return headers
		},
	})

	t.Setenv(agentproduct.EnvName, "qwenwork")
	if got := effectiveClawType(); got != "wukong" {
		t.Fatalf("effectiveClawType() = %q, want wukong", got)
	}
	if hookCalled {
		t.Fatal("EnterpriseCredentialHeaders hook ran during PAT error serialization")
	}
}

func TestAgentProductControlsObservabilityHeaderAndMessageClawTypeOnly(t *testing.T) {
	t.Setenv("DWS_CONFIG_DIR", t.TempDir())
	t.Setenv(agentproduct.EnvName, "qwenwork")

	oldEdition := edition.Get()
	t.Cleanup(func() { edition.Override(oldEdition) })
	edition.Override(&edition.Hooks{
		ClawTypeValue: "message-brand",
		MergeHeaders: func(headers map[string]string) map[string]string {
			headers["claw-type"] = "wukong"
			return headers
		},
	})

	headers := resolveIdentityHeaders()
	if got := headers[agentproduct.HeaderName]; got != "qwenwork" {
		t.Fatalf("HTTP %s = %q, want qwenwork", agentproduct.HeaderName, got)
	}
	if got := headers["claw-type"]; got != "wukong" {
		t.Fatalf("HTTP claw-type = %q, want wukong", got)
	}
	if got := edition.ClawType(); got != "qwenwork" {
		t.Fatalf("message clawType = %q, want qwenwork", got)
	}
}

func TestResolveIdentityHeadersRestoresIdentityAfterNilCredentialHeaders(t *testing.T) {
	t.Setenv("DWS_CONFIG_DIR", t.TempDir())
	t.Setenv(agentproduct.EnvName, "qwenwork")

	oldEdition := edition.Get()
	t.Cleanup(func() { edition.Override(oldEdition) })

	credentialHookCalled := false
	edition.Override(&edition.Hooks{
		MergeHeaders: func(headers map[string]string) map[string]string {
			headers["claw-type"] = "wukong"
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
	if got := headers["claw-type"]; got != "wukong" {
		t.Fatalf("claw-type = %q, want wukong", got)
	}
}

func TestResolveIdentityHeadersRestoresDefaultsAfterNilMergeHeaders(t *testing.T) {
	t.Setenv("DWS_CONFIG_DIR", t.TempDir())
	t.Setenv(agentproduct.EnvName, "")

	oldEdition := edition.Get()
	t.Cleanup(func() { edition.Override(oldEdition) })
	edition.Override(&edition.Hooks{
		MergeHeaders: func(map[string]string) map[string]string {
			return nil
		},
	})

	headers := resolveIdentityHeaders()
	if got := headers["claw-type"]; got != edition.DefaultOSSClawType {
		t.Fatalf("claw-type = %q, want %q", got, edition.DefaultOSSClawType)
	}
	if _, ok := headers[agentproduct.HeaderName]; ok {
		t.Fatalf("unset Product must omit %s", agentproduct.HeaderName)
	}
}

func TestEffectiveClawTypeIgnoresAgentProduct(t *testing.T) {
	oldEdition := edition.Get()
	t.Cleanup(func() { edition.Override(oldEdition) })
	edition.Override(&edition.Hooks{
		MergeHeaders: func(headers map[string]string) map[string]string {
			headers["claw-type"] = "wukong"
			return headers
		},
	})

	t.Setenv(agentproduct.EnvName, "qwenwork")
	if got := effectiveClawType(); got != "wukong" {
		t.Fatalf("effectiveClawType() = %q, want wukong", got)
	}
	t.Setenv(authpkg.AgentCodeEnv, "agent-code")
	if got := apperrors.HostControlBlock()["clawType"]; got != "wukong" {
		t.Fatalf("hostControl.clawType = %q, want wukong", got)
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
