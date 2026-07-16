package helpers

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/executor"
	"github.com/spf13/cobra"
)

type onboardingCoverageRunner struct {
	responses []map[string]any
	errors    []error
	index     int
}

func (r *onboardingCoverageRunner) Run(context.Context, executor.Invocation) (executor.Result, error) {
	index := r.index
	r.index++
	if index < len(r.errors) && r.errors[index] != nil {
		return executor.Result{}, r.errors[index]
	}
	if index < len(r.responses) {
		return executor.Result{Response: r.responses[index]}, nil
	}
	return executor.Result{}, nil
}

func TestCrossPlatformCoverageConnectOnboardingFailureCoverage(t *testing.T) {
	boom := errors.New("onboarding failure")
	originalStat := connectStdinStat
	originalSleep := connectOnboardingSleep
	t.Cleanup(func() {
		connectStdinStat = originalStat
		connectOnboardingSleep = originalSleep
	})
	connectStdinStat = func() (os.FileInfo, error) { return nil, boom }
	if connectStdinInteractive() {
		t.Fatal("stdin stat error reported interactive")
	}

	var out bytes.Buffer
	if _, err := runConnectOnboarding(&onboardingCoverageRunner{}, &cobra.Command{}, strings.NewReader(""), &out); err == nil {
		t.Fatal("choice EOF returned nil")
	}
	if _, err := onboardExistingApp(&onboardingCoverageRunner{}, &cobra.Command{}, bufio.NewReader(strings.NewReader("")), &out); err == nil {
		t.Fatal("existing app EOF returned nil")
	}
	if _, err := onboardExistingApp(&onboardingCoverageRunner{errors: []error{boom}}, &cobra.Command{}, bufio.NewReader(strings.NewReader("app\n")), &out); err == nil {
		t.Fatal("credential fetch error returned nil")
	}
	if _, err := onboardExistingApp(&onboardingCoverageRunner{responses: []map[string]any{{}}}, &cobra.Command{}, bufio.NewReader(strings.NewReader("app\n")), &out); err == nil {
		t.Fatal("empty fetched credentials returned nil")
	}
	if _, err := onboardExistingApp(&onboardingCoverageRunner{}, &cobra.Command{}, bufio.NewReader(strings.NewReader("\nclient\n\n")), &out); err == nil {
		t.Fatal("empty raw secret returned nil")
	}

	input := func() *bufio.Reader { return bufio.NewReader(strings.NewReader("app\nrobot\ndescription\n")) }
	if _, err := onboardNewApp(&onboardingCoverageRunner{errors: []error{boom}}, &cobra.Command{}, input(), &out); err == nil {
		t.Fatal("submit error returned nil")
	}
	if _, err := onboardNewApp(&onboardingCoverageRunner{responses: []map[string]any{{}}}, &cobra.Command{}, input(), &out); err == nil {
		t.Fatal("empty task ID returned nil")
	}

	connectOnboardingSleep = func(context.Context, time.Duration) error { return nil }
	if _, err := pollRobotCreateResult(&onboardingCoverageRunner{errors: []error{boom}}, &cobra.Command{}, "task", &out); err == nil {
		t.Fatal("poll runner error returned nil")
	}
	if _, err := pollRobotCreateResult(&onboardingCoverageRunner{responses: []map[string]any{{"status": "failed"}}}, &cobra.Command{}, "task", &out); err == nil {
		t.Fatal("failed task returned nil")
	}
	responses := make([]map[string]any, 10)
	for index := range responses {
		responses[index] = map[string]any{"status": "pending"}
	}
	if _, err := pollRobotCreateResult(&onboardingCoverageRunner{responses: responses}, &cobra.Command{}, "task", &out); err == nil {
		t.Fatal("poll timeout returned nil")
	}
}
