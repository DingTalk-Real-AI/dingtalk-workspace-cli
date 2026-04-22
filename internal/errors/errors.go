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

package errors

import (
	"bytes"
	"encoding/json"
	stderrors "errors"
	"fmt"
	"io"
	"strings"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/config"
)

// Category represents a stable error class with a documented exit code.
type Category string

const (
	CategoryAPI        Category = "api"
	CategoryAuth       Category = "auth"
	CategoryValidation Category = "validation"
	CategoryDiscovery  Category = "discovery"
	CategoryInternal   Category = "internal"
)

// Error is the structured repository-local error model for the Go rewrite.
type Error struct {
	Category   Category
	Message    string
	Operation  string
	ServerKey  string
	Retryable  bool
	Reason     string
	Hint       string
	Actions    []string
	Snapshot   string
	RPCCode    int               `json:"rpc_code,omitempty"`
	RPCData    json.RawMessage   `json:"rpc_data,omitempty"`
	ServerDiag ServerDiagnostics `json:"-"`
	Cause      error             `json:"-"`
}

func (e *Error) Error() string {
	return e.Message
}

// Unwrap returns the underlying cause, enabling errors.Is and errors.As chains.
func (e *Error) Unwrap() error {
	return e.Cause
}

// Option mutates a structured error before it is returned.
type Option func(*Error)

// ExitCode returns the documented process exit code for the error category.
// exit=4 is reserved exclusively for PATError (see internal/errors/pat.go
// ExitCodePermission and docs/pat/contract.md §1); Discovery therefore
// uses 6 so hosts can tell "catalog lookup broke" apart from "PAT
// permission insufficient".
func (e *Error) ExitCode() int {
	switch e.Category {
	case CategoryAPI:
		return 1
	case CategoryAuth:
		return 2
	case CategoryValidation:
		return 3
	case CategoryDiscovery:
		return 6
	default:
		return 5
	}
}

// WithOperation records the operation that failed.
func WithOperation(operation string) Option {
	return func(err *Error) {
		err.Operation = operation
	}
}

// WithServerKey records the server identifier associated with the failure.
func WithServerKey(serverKey string) Option {
	return func(err *Error) {
		err.ServerKey = serverKey
	}
}

// WithRetryable marks whether the error can be retried safely.
func WithRetryable(retryable bool) Option {
	return func(err *Error) {
		err.Retryable = retryable
	}
}

// WithReason records a stable machine-readable failure reason.
func WithReason(reason string) Option {
	return func(err *Error) {
		err.Reason = reason
	}
}

// WithHint records a short recovery hint for humans and agents.
func WithHint(hint string) Option {
	return func(err *Error) {
		err.Hint = hint
	}
}

// WithActions records suggested next actions for recovery.
func WithActions(actions ...string) Option {
	return func(err *Error) {
		out := make([]string, 0, len(actions))
		for _, action := range actions {
			if action == "" {
				continue
			}
			out = append(out, action)
		}
		if len(out) > 0 {
			err.Actions = out
		}
	}
}

// WithSnapshot records the recovery snapshot path associated with the failure.
func WithSnapshot(path string) Option {
	return func(err *Error) {
		err.Snapshot = path
	}
}

// WithRPCCode records the original JSON-RPC error code.
func WithRPCCode(code int) Option {
	return func(err *Error) {
		err.RPCCode = code
	}
}

// WithRPCData records the original JSON-RPC error data payload.
func WithRPCData(data json.RawMessage) Option {
	if len(bytes.TrimSpace(data)) == 0 {
		return func(*Error) {} // no-op, consistent with other Options
	}
	return func(err *Error) {
		err.RPCData = data
	}
}

// WithCause wraps the original error so it can be retrieved via errors.Unwrap.
func WithCause(err error) Option {
	return func(e *Error) {
		e.Cause = err
	}
}

func newError(category Category, message string, opts ...Option) error {
	err := &Error{
		Category: category,
		Message:  message,
	}
	for _, opt := range opts {
		if opt != nil {
			opt(err)
		}
	}
	return err
}

// NewAPI returns an API-category error.
func NewAPI(message string, opts ...Option) error {
	return newError(CategoryAPI, message, opts...)
}

// NewAuth returns an auth-category error.
func NewAuth(message string, opts ...Option) error {
	return newError(CategoryAuth, message, opts...)
}

// NewValidation returns a validation-category error.
func NewValidation(message string, opts ...Option) error {
	return newError(CategoryValidation, message, opts...)
}

// NewDiscovery returns a discovery-category error.
func NewDiscovery(message string, opts ...Option) error {
	return newError(CategoryDiscovery, message, opts...)
}

// NewInternal returns an internal-category error.
func NewInternal(message string, opts ...Option) error {
	return newError(CategoryInternal, message, opts...)
}

// ExitCoder is implemented by errors that provide their own exit code.
// Edition-specific error types (e.g. PATError, CLIError) implement this
// so the framework can resolve exit codes without importing edition packages.
type ExitCoder interface {
	ExitCode() int
}

// RawStderrError is implemented by errors that must output raw content
// directly to stderr, bypassing all CLI formatting (e.g. "Error:" prefix).
// PAT authorization errors use this to pass JSON through to the desktop runtime.
type RawStderrError interface {
	error
	RawStderr() string
}

// ExitCode maps any error to a stable exit code.
func ExitCode(err error) int {
	var typed *Error
	if stderrors.As(err, &typed) {
		return typed.ExitCode()
	}
	var ec ExitCoder
	if stderrors.As(err, &ec) {
		return ec.ExitCode()
	}
	return 5
}

// PrintJSON writes a machine-readable JSON error object.
func PrintJSON(w io.Writer, err error) error {
	errorPayload := map[string]any{
		"code":     ExitCode(err),
		"category": category(err),
		"message":  err.Error(),
	}

	var typed *Error
	if stderrors.As(err, &typed) {
		if typed.Reason != "" {
			errorPayload["reason"] = typed.Reason
		}
		if typed.Operation != "" {
			errorPayload["operation"] = typed.Operation
		}
		if typed.ServerKey != "" {
			errorPayload["server_key"] = typed.ServerKey
		}
		if typed.Retryable {
			errorPayload["retryable"] = true
		}
		if typed.Hint != "" {
			errorPayload["hint"] = typed.Hint
		}
		if len(typed.Actions) > 0 {
			errorPayload["actions"] = typed.Actions
		}
		if typed.Snapshot != "" {
			errorPayload["snapshot_path"] = typed.Snapshot
		}
		if typed.RPCCode != 0 {
			errorPayload["rpc_code"] = typed.RPCCode
		}
		if len(typed.RPCData) > 0 {
			var parsed any
			if json.Unmarshal(typed.RPCData, &parsed) == nil {
				errorPayload["rpc_data"] = parsed
			}
		}
		if !typed.ServerDiag.IsEmpty() {
			if typed.ServerDiag.TraceID != "" {
				errorPayload["trace_id"] = typed.ServerDiag.TraceID
			}
			if typed.ServerDiag.ServerErrorCode != "" {
				errorPayload["server_error_code"] = typed.ServerDiag.ServerErrorCode
				// Add user-friendly hint for specific server error codes
				switch typed.ServerDiag.ServerErrorCode {
				case "TOKEN_VERIFIED_FAILED", "CLI_ORG_NOT_AUTHORIZED":
					errorPayload["friendly_hint"] = "该组织尚未开启 CLI 数据访问权限，请联系组织主管理员开启。"
					errorPayload["action_url"] = config.GetDeveloperSettingsURL()
				}
			}
			if typed.ServerDiag.TechnicalDetail != "" {
				errorPayload["technical_detail"] = typed.ServerDiag.TechnicalDetail
			}
		}
		if typed.Cause != nil {
			errorPayload["cause"] = typed.Cause.Error()
		}
	}
	payload := map[string]any{"error": errorPayload}

	data, marshalErr := json.MarshalIndent(payload, "", "  ")
	if marshalErr != nil {
		_, writeErr := fmt.Fprintf(w, "{\"error\":{\"code\":5,\"category\":\"internal\",\"message\":\"failed to encode error output\"}}\n")
		return writeErr
	}

	_, writeErr := fmt.Fprintln(w, string(data))
	return writeErr
}

// Verbosity controls how much detail PrintHuman includes.
type Verbosity int

const (
	// VerbosityNormal shows essential info: error, hint, actions, trace_id, server_code.
	VerbosityNormal Verbosity = 0
	// VerbosityVerbose adds technical_detail, snapshot, execution context.
	VerbosityVerbose Verbosity = 1
	// VerbosityDebug adds all internal diagnostics (category, operation, reason, rpc_code).
	VerbosityDebug Verbosity = 2
)

// PrintHuman writes a concise human-readable error rendering at normal verbosity.
func PrintHuman(w io.Writer, err error) error {
	return PrintHumanAt(w, err, VerbosityNormal)
}

// PrintHumanAt writes a human-readable error rendering at the given verbosity level.
func PrintHumanAt(w io.Writer, err error, v Verbosity) error {
	if err == nil {
		return nil
	}

	var typed *Error
	if !stderrors.As(err, &typed) {
		_, writeErr := fmt.Fprintf(w, "Error: %s\n", err.Error())
		return writeErr
	}

	// Line 1: Error summary
	lines := []string{
		fmt.Sprintf("Error: [%s] %s", strings.ToUpper(string(typed.Category)), typed.Message),
	}

	// Always shown: hint, actions, retryable
	if typed.Hint != "" {
		lines = append(lines, fmt.Sprintf("Hint: %s", typed.Hint))
	}

	// Add user-friendly hint for specific server error codes
	switch typed.ServerDiag.ServerErrorCode {
	case "TOKEN_VERIFIED_FAILED", "CLI_ORG_NOT_AUTHORIZED":
		lines = append(lines, "Hint: 该组织尚未开启 CLI 数据访问权限，请联系组织主管理员开启。")
		lines = append(lines, "Action: 开启地址: "+config.GetDeveloperSettingsURL())
	}

	if len(typed.Actions) > 0 {
		for _, action := range typed.Actions {
			if strings.TrimSpace(action) == "" {
				continue
			}
			lines = append(lines, fmt.Sprintf("Action: %s", action))
		}
	}
	if typed.Retryable {
		lines = append(lines, "Retryable: true")
	}

	// Always shown when present: Trace ID, Server Code
	if typed.ServerDiag.TraceID != "" {
		lines = append(lines, fmt.Sprintf("Trace ID: %s", typed.ServerDiag.TraceID))
	}
	if typed.ServerDiag.ServerErrorCode != "" {
		lines = append(lines, fmt.Sprintf("Server Code: %s", typed.ServerDiag.ServerErrorCode))
	}

	// Verbose+: technical detail, snapshot, reason, server key
	if v >= VerbosityVerbose {
		if typed.ServerDiag.TechnicalDetail != "" {
			lines = append(lines, fmt.Sprintf("Detail: %s", typed.ServerDiag.TechnicalDetail))
		}
		if typed.Reason != "" {
			lines = append(lines, fmt.Sprintf("Reason: %s", typed.Reason))
		}
		if typed.ServerKey != "" {
			lines = append(lines, fmt.Sprintf("Server: %s", typed.ServerKey))
		}
		if typed.Snapshot != "" {
			lines = append(lines, fmt.Sprintf("Snapshot: %s", typed.Snapshot))
		}
		if typed.Cause != nil {
			lines = append(lines, fmt.Sprintf("Cause: %s", typed.Cause.Error()))
		}
	}

	// Debug: all internal diagnostics
	if v >= VerbosityDebug {
		if typed.Operation != "" {
			lines = append(lines, fmt.Sprintf("Operation: %s", typed.Operation))
		}
		if typed.RPCCode != 0 {
			lines = append(lines, fmt.Sprintf("RPC Code: %d", typed.RPCCode))
		}
		if len(typed.RPCData) > 0 {
			lines = append(lines, fmt.Sprintf("RPC Data: %s", string(typed.RPCData)))
		}
	}

	_, writeErr := fmt.Fprintln(w, strings.Join(lines, "\n"))
	return writeErr
}

func category(err error) string {
	var typed *Error
	if stderrors.As(err, &typed) {
		return string(typed.Category)
	}
	return string(CategoryInternal)
}
