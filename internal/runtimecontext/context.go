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

// Package runtimecontext supplies an optional, process-scoped request context.
package runtimecontext

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/requestmeta"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/runtimepayload"
)

const (
	HeaderName     = requestmeta.DingTalkExtHeader
	PayloadVersion = runtimepayload.PayloadVersion
	Environment    = "online"

	initializationEnvironment int32 = 0
	maxHeaderBytes                  = 8 * 1024
	defaultTimeout                  = 3 * time.Second
)

type State string

const (
	StateReady       State = "ready"
	StateUnavailable State = "unavailable"
	StateTimeout     State = "timeout"
	StateError       State = "error"
)

// Result is the immutable process result. Sensitive values stay private and
// cannot be serialized or formatted accidentally.
type Result struct {
	State           State         `json:"state"`
	PayloadVersion  string        `json:"payload_version"`
	Environment     string        `json:"environment"`
	Elapsed         time.Duration `json:"-"`
	ErrorCategory   string        `json:"error_category,omitempty"`
	token           string
	callbackCode    int32
	hasCallbackCode bool
}

func (r Result) String() string {
	return fmt.Sprintf("runtime context state=%s", r.State)
}

func (r Result) GoString() string { return r.String() }

// HeaderValue returns the compact request header payload when initialization
// completed successfully.
func (r Result) HeaderValue() (string, bool) {
	if r.State != StateReady || r.token == "" {
		return "", false
	}
	payload, err := json.Marshal(struct {
		UMID string `json:"umid"`
	}{UMID: r.token})
	if err != nil || len(payload) > maxHeaderBytes {
		return "", false
	}
	return string(payload), true
}

// DiagnosticDetail returns redacted, stable diagnostic fields.
func (r Result) DiagnosticDetail() map[string]any {
	detail := map[string]any{
		"state":             r.State,
		"payload_version":   r.PayloadVersion,
		"environment":       r.Environment,
		"elapsed_ms":        r.Elapsed.Milliseconds(),
		"token_length":      len(r.token),
		"token_fingerprint": "",
	}
	if r.token != "" {
		sum := sha256.Sum256([]byte(r.token))
		detail["token_fingerprint"] = hex.EncodeToString(sum[:])[:12]
	}
	if r.ErrorCategory != "" {
		detail["error_category"] = r.ErrorCategory
	}
	if r.hasCallbackCode {
		detail["callback_code"] = r.callbackCode
	}
	return detail
}

type callbackResult struct {
	token string
	code  int32
	err   error
}

type nativeSession struct {
	handle   uintptr
	callback uintptr
}

type nativeStarter func(string, func([]byte, int32, error)) (nativeSession, int32, error)
type libraryLocator func() (string, error)

var (
	errNativeLoad    = errors.New("runtime library load failed")
	errNativeSymbol  = errors.New("runtime symbol unavailable")
	errNativeBinding = errors.New("runtime binding failed")
)

type provider struct {
	once    sync.Once
	result  Result
	session nativeSession
	start   nativeStarter
	locate  libraryLocator
	timeout time.Duration
}

func newProvider(start nativeStarter, locate libraryLocator, timeout time.Duration) *provider {
	return &provider{start: start, locate: locate, timeout: timeout}
}

var defaultProvider = newProvider(startNative, resolveLibraryPath, defaultTimeout)

// Resolve initializes and returns the process-scoped context. It never
// retries: every caller observes the same immutable outcome.
func Resolve() Result {
	return defaultProvider.resolve()
}

func (p *provider) resolve() Result {
	p.once.Do(func() {
		p.result = p.initialize()
	})
	return p.result
}

func (p *provider) initialize() Result {
	started := time.Now()
	result := Result{PayloadVersion: PayloadVersion, Environment: Environment}
	path, err := p.locate()
	if err != nil {
		result.State = StateUnavailable
		result.ErrorCategory = classifyLocationError(err)
		result.Elapsed = time.Since(started)
		return result
	}

	callback := make(chan callbackResult, 1)
	session, accepted, err := p.start(path, func(raw []byte, code int32, callbackErr error) {
		value := callbackResult{code: code, err: callbackErr}
		if callbackErr == nil {
			value.token, value.err = validateToken(raw)
		}
		select {
		case callback <- value:
		default:
		}
	})
	p.session = session
	if err != nil {
		result.State = StateError
		result.ErrorCategory = classifyStartError(err)
		result.Elapsed = time.Since(started)
		return result
	}
	if accepted == 0 {
		result.State = StateError
		result.ErrorCategory = "initialization_rejected"
		result.Elapsed = time.Since(started)
		return result
	}

	timer := time.NewTimer(p.timeout)
	defer timer.Stop()
	select {
	case value := <-callback:
		result.Elapsed = time.Since(started)
		if value.code != 0 {
			result.State = StateError
			result.ErrorCategory = "callback_failed"
			result.callbackCode = value.code
			result.hasCallbackCode = true
			return result
		}
		if value.err != nil {
			result.State = StateError
			result.ErrorCategory = "invalid_value"
			return result
		}
		result.State = StateReady
		result.token = value.token
		return result
	case <-timer.C:
		result.State = StateTimeout
		result.ErrorCategory = "initialization_timeout"
		result.Elapsed = time.Since(started)
		return result
	}
}

func validateToken(raw []byte) (string, error) {
	if len(raw) == 0 || len(raw) > maxHeaderBytes || !utf8.Valid(raw) {
		return "", errors.New("invalid runtime value")
	}
	value := string(raw)
	for _, r := range value {
		if unicode.IsControl(r) {
			return "", errors.New("invalid runtime value")
		}
	}
	payload, err := json.Marshal(struct {
		UMID string `json:"umid"`
	}{UMID: value})
	if err != nil || len(payload) > maxHeaderBytes {
		return "", errors.New("invalid runtime value")
	}
	return value, nil
}

var (
	osExecutable              = os.Executable
	evalSymlinks              = filepath.EvalSymlinks
	userCacheDir              = os.UserCacheDir
	materializeRuntimePayload = func(cacheDir, goos, goarch string) (string, error) {
		return runtimepayload.Materialize(runtimepayload.Embedded(), cacheDir, goos, goarch)
	}
	currentGOOS   = runtime.GOOS
	currentGOARCH = runtime.GOARCH
)

func resolveLibraryPath() (string, error) {
	name, err := libraryName(currentGOOS, currentGOARCH)
	if err != nil {
		return "", err
	}
	executable, err := osExecutable()
	if err != nil {
		return "", fmt.Errorf("executable path: %w", err)
	}
	executables := []string{executable}
	if resolved, resolveErr := evalSymlinks(executable); resolveErr == nil {
		if resolved != executables[0] {
			executables = append(executables, resolved)
		}
	}
	if cacheDir, cacheErr := userCacheDir(); cacheErr == nil {
		if path, materializeErr := materializeRuntimePayload(cacheDir, currentGOOS, currentGOARCH); materializeErr == nil {
			return path, nil
		}
	}
	// Retain sidecar discovery as a compatibility fallback for older installs
	// and direct development fixtures. New packages carry the payload in dws.
	for _, candidate := range executables {
		executableDir := filepath.Dir(candidate)
		root := filepath.Join(executableDir, ".dws-runtime", PayloadVersion)
		path := filepath.Join(root, name)
		if regularFile(path) && directory(filepath.Join(root, "ps")) && regularFile(filepath.Join(root, "manifest.json")) {
			return path, nil
		}
	}
	return "", errors.New("runtime payload unavailable")
}

func libraryName(goos, goarch string) (string, error) {
	return runtimepayload.LibraryName(goos, goarch)
}

func regularFile(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular()
}

func directory(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

func classifyLocationError(err error) string {
	if err != nil && strings.Contains(err.Error(), "unsupported runtime platform") {
		return "unsupported_platform"
	}
	return "payload_unavailable"
}

func classifyStartError(err error) string {
	switch {
	case errors.Is(err, errNativeSymbol):
		return "symbol_unavailable"
	case errors.Is(err, errNativeBinding):
		return "binding_failed"
	default:
		return "load_failed"
	}
}
