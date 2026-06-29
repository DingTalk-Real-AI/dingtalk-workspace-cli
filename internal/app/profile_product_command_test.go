package app

import (
	"bytes"
	"context"
	"sync"
	"testing"

	authpkg "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/auth"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/compat"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/executor"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/market"
	"github.com/spf13/cobra"
)

func TestProductCommandsAcceptGlobalProfileFlag(t *testing.T) {
	const selectedProfile = "corp_profile_matrix"

	products := []struct {
		name string
		path []string
		tool string
	}{
		{name: "aitable", path: []string{"aitable", "profile-test", "probe"}, tool: "aitable_profile_probe"},
		{name: "attendance", path: []string{"attendance", "profile-test", "probe"}, tool: "attendance_profile_probe"},
		{name: "calendar", path: []string{"calendar", "profile-test", "probe"}, tool: "calendar_profile_probe"},
		{name: "contact", path: []string{"contact", "profile-test", "probe"}, tool: "contact_profile_probe"},
		{name: "devdoc", path: []string{"devdoc", "profile-test", "probe"}, tool: "devdoc_profile_probe"},
		{name: "ding", path: []string{"ding", "profile-test", "probe"}, tool: "ding_profile_probe"},
		{name: "report", path: []string{"report", "profile-test", "probe"}, tool: "report_profile_probe"},
		{name: "todo", path: []string{"todo", "profile-test", "probe"}, tool: "todo_profile_probe"},
	}

	descriptors := make([]market.ServerDescriptor, 0, len(products))
	for _, product := range products {
		descriptors = append(descriptors, profileFlagProductDescriptor(product.name, product.tool))
	}

	capture := &profileFlagRunner{}
	oldLoadDynamicCommands := loadDynamicCommandsFn
	loadDynamicCommandsFn = func(_ context.Context, _ executor.Runner) []*cobra.Command {
		SetDynamicServers(descriptors)
		return compat.BuildDynamicCommands(descriptors, capture, nil, nil)
	}
	authpkg.SetRuntimeProfile("")
	ResetRuntimeTokenCache()
	t.Cleanup(func() {
		loadDynamicCommandsFn = oldLoadDynamicCommands
		SetDynamicServers(nil)
		authpkg.SetRuntimeProfile("")
		ResetRuntimeTokenCache()
	})

	for _, product := range products {
		t.Run(product.name, func(t *testing.T) {
			capture.reset()
			authpkg.SetRuntimeProfile("")

			cmd := NewRootCommand()
			var out bytes.Buffer
			cmd.SetOut(&out)
			cmd.SetErr(&out)
			args := append([]string{"-f", "json"}, product.path...)
			args = append(args, "--profile", selectedProfile)
			cmd.SetArgs(args)

			// Arrange / Act: execute a product command with root --profile after the leaf.
			if err := cmd.Execute(); err != nil {
				t.Fatalf("Execute(%v) error = %v\noutput:\n%s", args, err, out.String())
			}

			// Assert: the product tool runs under the selected profile without leaking it as a business arg.
			call := capture.last()
			if call == nil {
				t.Fatal("expected product command to invoke runner")
			}
			if call.product != product.name {
				t.Fatalf("canonical product = %q, want %q", call.product, product.name)
			}
			if call.tool != product.tool {
				t.Fatalf("tool = %q, want %q", call.tool, product.tool)
			}
			if call.profile != selectedProfile {
				t.Fatalf("runtime profile at execution = %q, want %q", call.profile, selectedProfile)
			}
			if _, ok := call.params["profile"]; ok {
				t.Fatalf("--profile leaked into business params: %#v", call.params)
			}
		})
	}
}

func profileFlagProductDescriptor(product, tool string) market.ServerDescriptor {
	return market.ServerDescriptor{
		Key:         product,
		DisplayName: product,
		Endpoint:    "https://example.invalid/" + product,
		CLI: market.CLIOverlay{
			ID:      product,
			Command: product,
			Groups: map[string]market.CLIGroupDef{
				"profile-test": {Description: "profile-test"},
			},
			ToolOverrides: map[string]market.CLIToolOverride{
				tool: {
					CLIName:          "probe",
					Group:            "profile-test",
					Description:      tool,
					RejectPositional: true,
				},
			},
		},
	}
}

type profileFlagCall struct {
	product string
	tool    string
	profile string
	params  map[string]any
}

type profileFlagRunner struct {
	mu    sync.Mutex
	calls []profileFlagCall
}

func (r *profileFlagRunner) Run(_ context.Context, invocation executor.Invocation) (executor.Result, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	params := make(map[string]any, len(invocation.Params))
	for key, value := range invocation.Params {
		params[key] = value
	}
	r.calls = append(r.calls, profileFlagCall{
		product: invocation.CanonicalProduct,
		tool:    invocation.Tool,
		profile: authpkg.RuntimeProfile(),
		params:  params,
	})
	return executor.Result{Invocation: invocation}, nil
}

func (r *profileFlagRunner) reset() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls = nil
}

func (r *profileFlagRunner) last() *profileFlagCall {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.calls) == 0 {
		return nil
	}
	call := r.calls[len(r.calls)-1]
	return &call
}
