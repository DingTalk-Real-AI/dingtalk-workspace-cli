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

package transport

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"strings"
	"sync"
	"sync/atomic"
)

// StdioClient manages a local MCP server subprocess, communicating via
// stdin/stdout using JSON-RPC 2.0 (newline-delimited).
type StdioClient struct {
	command string
	args    []string
	env     map[string]string

	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout *bufio.Reader
	stderr io.ReadCloser

	mu      sync.Mutex // serializes JSON-RPC requests
	nextID  int64
	started bool
}

// NewStdioClient creates a StdioClient for the given command.
// The subprocess is not started until Start() is called.
func NewStdioClient(command string, args []string, env map[string]string) *StdioClient {
	return &StdioClient{
		command: command,
		args:    args,
		env:     env,
	}
}

// Start launches the subprocess.
func (s *StdioClient) Start(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.started {
		return nil
	}

	cmd := exec.CommandContext(ctx, s.command, s.args...)

	// Build environment: inherit current env + merge plugin-specific vars.
	cmd.Env = os.Environ()
	for k, v := range s.env {
		cmd.Env = append(cmd.Env, k+"="+v)
	}

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("stdio: create stdin pipe: %w", err)
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		stdin.Close()
		return fmt.Errorf("stdio: create stdout pipe: %w", err)
	}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		stdin.Close()
		stdout.Close()
		return fmt.Errorf("stdio: create stderr pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("stdio: start process %q: %w", s.command, err)
	}

	s.cmd = cmd
	s.stdin = stdin
	s.stdout = bufio.NewReaderSize(stdout, 64*1024)
	s.stderr = stderr
	s.started = true

	// Drain stderr in background for debug logging.
	go s.drainStderr()

	return nil
}

// Stop kills the subprocess and waits for it to exit.
// A non-zero exit code after Kill is expected and not treated as an error.
func (s *StdioClient) Stop() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.started || s.cmd == nil {
		return nil
	}

	s.stdin.Close()

	killed := false
	if s.cmd.Process != nil {
		_ = s.cmd.Process.Kill()
		killed = true
	}
	err := s.cmd.Wait()
	s.started = false
	// A killed process always exits with a non-zero status; suppress that
	// expected error to avoid noisy warnings on normal shutdown.
	if killed {
		return nil
	}
	return err
}

// Initialize sends the JSON-RPC initialize request.
func (s *StdioClient) Initialize(ctx context.Context) (InitializeResult, error) {
	params := map[string]any{
		"protocolVersion": supportedProtocolVersions[0],
		"capabilities":    map[string]any{},
		"clientInfo": map[string]any{
			"name":    "dws-cli",
			"version": "1.0.0",
		},
	}

	var result InitializeResult
	if err := s.call(ctx, "initialize", params, &result); err != nil {
		return InitializeResult{}, err
	}
	return result, nil
}

// ListTools sends the tools/list JSON-RPC request.
func (s *StdioClient) ListTools(ctx context.Context) (ToolsListResult, error) {
	var result ToolsListResult
	if err := s.call(ctx, "tools/list", nil, &result); err != nil {
		return ToolsListResult{}, err
	}
	return result, nil
}

// CallTool sends the tools/call JSON-RPC request.
func (s *StdioClient) CallTool(ctx context.Context, tool string, arguments map[string]any) (ToolCallResult, error) {
	params := map[string]any{
		"name":      tool,
		"arguments": arguments,
	}

	var result ToolCallResult
	if err := s.call(ctx, "tools/call", params, &result); err != nil {
		return ToolCallResult{}, err
	}
	return result, nil
}

// call sends a JSON-RPC request and reads the response. It is serialized
// by the mutex to ensure one request at a time over the stdio pipe.
func (s *StdioClient) call(ctx context.Context, method string, params any, result any) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.started {
		return fmt.Errorf("stdio: process not started")
	}

	id := atomic.AddInt64(&s.nextID, 1)

	req := requestEnvelope{
		JSONRPC: "2.0",
		ID:      int(id),
		Method:  method,
		Params:  params,
	}

	reqData, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("stdio: marshal request: %w", err)
	}

	// Write request line.
	reqData = append(reqData, '\n')
	if _, err := s.stdin.Write(reqData); err != nil {
		return fmt.Errorf("stdio: write request: %w", err)
	}

	// Read response line (respects context cancellation).
	type readResult struct {
		line []byte
		err  error
	}
	ch := make(chan readResult, 1)
	go func() {
		line, err := s.stdout.ReadBytes('\n')
		ch <- readResult{line, err}
	}()

	select {
	case <-ctx.Done():
		return fmt.Errorf("stdio: %w", ctx.Err())
	case rr := <-ch:
		if rr.err != nil {
			return fmt.Errorf("stdio: read response: %w", rr.err)
		}

		var resp responseEnvelope
		if err := json.Unmarshal(rr.line, &resp); err != nil {
			return fmt.Errorf("stdio: unmarshal response: %w", err)
		}

		if resp.Error != nil {
			return fmt.Errorf("stdio: RPC error %d: %s", resp.Error.Code, resp.Error.Message)
		}

		if result != nil {
			if err := json.Unmarshal(resp.Result, result); err != nil {
				return fmt.Errorf("stdio: unmarshal result: %w", err)
			}
		}

		return nil
	}
}

// drainStderr reads stderr in the background and logs lines at debug level.
func (s *StdioClient) drainStderr() {
	scanner := bufio.NewScanner(s.stderr)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line != "" {
			slog.Debug("stdio: subprocess stderr", "command", s.command, "line", line)
		}
	}
}
