// Package runner invokes the dws binary as a subprocess with controlled environment variables.
// It injects DWS_CATALOG_FIXTURE to redirect MCP traffic through the proxy and
// DWS_TRUSTED_DOMAINS to allow connections to the local proxy address.
package runner

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

const (
	// CatalogFixtureEnv is the environment variable dws uses to load a static catalog JSON.
	// When set, dws skips live discovery and uses the fixture file instead.
	CatalogFixtureEnv = "DWS_CATALOG_FIXTURE"

	// TrustedDomainsEnv allows dws to connect to non-dingtalk.com hosts.
	// Must include "127.0.0.1" to allow connections to the local proxy.
	TrustedDomainsEnv = "DWS_TRUSTED_DOMAINS"

	defaultTimeout = 30 * time.Second
)

// Result holds the output of a single dws invocation.
type Result struct {
	Stdout   string
	Stderr   string
	ExitCode int
}

// Runner invokes the dws binary with a controlled environment.
type Runner struct {
	// DWSBinary is the path to the dws executable.
	DWSBinary string
	// CatalogFixturePath is the path to the catalog JSON fixture file.
	// When set, it is injected via DWS_CATALOG_FIXTURE so dws uses the proxy endpoint.
	CatalogFixturePath string
	// ExtraEnv is additional environment variables to pass through (e.g., auth tokens).
	ExtraEnv []string
	// Timeout for each dws invocation. Zero means 30 seconds.
	Timeout time.Duration
}

// Run executes dws with the given arguments and returns the captured output.
// A non-zero exit code is not treated as an error; check Result.ExitCode.
func (r *Runner) Run(ctx context.Context, args []string) (Result, error) {
	timeout := r.Timeout
	if timeout == 0 {
		timeout = defaultTimeout
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, r.DWSBinary, args...)

	env := make([]string, 0, len(r.ExtraEnv)+2)
	env = append(env, r.ExtraEnv...)
	if r.CatalogFixturePath != "" {
		env = append(env, CatalogFixtureEnv+"="+r.CatalogFixturePath)
		// Allow dws to connect to the local proxy (127.0.0.1).
		env = append(env, TrustedDomainsEnv+"=127.0.0.1,*.dingtalk.com")
	}
	cmd.Env = env

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	exitCode := 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
			err = nil // non-zero exit is not a runner-level error
		} else {
			return Result{}, fmt.Errorf("exec dws: %w", err)
		}
	}

	return Result{
		Stdout:   strings.TrimSpace(stdout.String()),
		Stderr:   strings.TrimSpace(stderr.String()),
		ExitCode: exitCode,
	}, nil
}
