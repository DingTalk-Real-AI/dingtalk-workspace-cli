package runtimecontext

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
	"unsafe"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
)

func TestCrossPlatformCoverageProviderReadyAndHeader(t *testing.T) {
	if initializationEnvironment != 0 || defaultTimeout != 3*time.Second {
		t.Fatalf("runtime defaults = env %d, timeout %v", initializationEnvironment, defaultTimeout)
	}
	provider := newProvider(func(path string, callback func([]byte, int32, error)) (nativeSession, int32, error) {
		if path != "/payload/library" {
			t.Fatalf("path = %q", path)
		}
		callback([]byte("device-value"), 0, nil)
		callback([]byte("duplicate-value"), 0, nil)
		return nativeSession{handle: 7, callback: 9}, 1, nil
	}, func() (string, error) {
		return "/payload/library", nil
	}, time.Second)

	testseam.Swap(t, &defaultProvider, provider)
	result := Resolve()
	if result.State != StateReady || result.token != "device-value" {
		t.Fatalf("result = %#v", result)
	}
	if provider.session.handle != 7 || provider.session.callback != 9 {
		t.Fatalf("native session was not retained: %#v", provider.session)
	}
	if value, ok := result.HeaderValue(); !ok || value != `{"umid":"device-value"}` {
		t.Fatalf("header = %q, %v", value, ok)
	}
	fixture := ReadyResultForTest("fixture-value")
	if value, ok := fixture.HeaderValue(); !ok || value != `{"umid":"fixture-value"}` {
		t.Fatalf("test fixture header = %q, %v", value, ok)
	}
	detail := result.DiagnosticDetail()
	if detail["token_length"] != 12 || detail["token_fingerprint"] == "" {
		t.Fatalf("detail = %#v", detail)
	}
	if strings.Contains(strings.Join(mapValues(detail), " "), result.token) {
		t.Fatalf("diagnostic leaked token: %#v", detail)
	}
	if formatted := fmt.Sprintf("%#v", result); strings.Contains(formatted, result.token) {
		t.Fatalf("formatted result leaked token: %s", formatted)
	}
}

func TestCrossPlatformCoverageProviderFailureOutcomes(t *testing.T) {
	tests := []struct {
		name      string
		starter   nativeStarter
		locator   libraryLocator
		wantState State
		wantError string
	}{
		{
			name: "missing payload",
			locator: func() (string, error) {
				return "", errors.New("runtime payload unavailable")
			},
			wantState: StateUnavailable,
			wantError: "payload_unavailable",
		},
		{
			name:    "load failure",
			locator: successfulLocator,
			starter: func(string, func([]byte, int32, error)) (nativeSession, int32, error) {
				return nativeSession{}, 0, errors.New("load")
			},
			wantState: StateError,
			wantError: "load_failed",
		},
		{
			name:    "symbol unavailable",
			locator: successfulLocator,
			starter: func(string, func([]byte, int32, error)) (nativeSession, int32, error) {
				return nativeSession{handle: 1}, 0, fmt.Errorf("%w: test", errNativeSymbol)
			},
			wantState: StateError,
			wantError: "symbol_unavailable",
		},
		{
			name:    "binding failure",
			locator: successfulLocator,
			starter: func(string, func([]byte, int32, error)) (nativeSession, int32, error) {
				return nativeSession{handle: 1}, 0, fmt.Errorf("%w: test", errNativeBinding)
			},
			wantState: StateError,
			wantError: "binding_failed",
		},
		{
			name:    "rejected",
			locator: successfulLocator,
			starter: func(string, func([]byte, int32, error)) (nativeSession, int32, error) {
				return nativeSession{}, 0, nil
			},
			wantState: StateError,
			wantError: "initialization_rejected",
		},
		{
			name:    "callback error",
			locator: successfulLocator,
			starter: func(_ string, callback func([]byte, int32, error)) (nativeSession, int32, error) {
				callback(nil, 42, nil)
				return nativeSession{}, 1, nil
			},
			wantState: StateError,
			wantError: "callback_failed",
		},
		{
			name:    "invalid value",
			locator: successfulLocator,
			starter: func(_ string, callback func([]byte, int32, error)) (nativeSession, int32, error) {
				callback([]byte("bad\nvalue"), 0, nil)
				return nativeSession{}, 1, nil
			},
			wantState: StateError,
			wantError: "invalid_value",
		},
		{
			name:    "callback copy failure",
			locator: successfulLocator,
			starter: func(_ string, callback func([]byte, int32, error)) (nativeSession, int32, error) {
				callback(nil, 0, errors.New("copy"))
				return nativeSession{}, 1, nil
			},
			wantState: StateError,
			wantError: "invalid_value",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			provider := newProvider(test.starter, test.locator, 20*time.Millisecond)
			result := provider.resolve()
			if result.State != test.wantState || result.ErrorCategory != test.wantError {
				t.Fatalf("result = %#v", result)
			}
			if detail := result.DiagnosticDetail(); detail["error_category"] != test.wantError {
				t.Fatalf("diagnostic detail = %#v", detail)
			}
			if value, ok := result.HeaderValue(); ok || value != "" {
				t.Fatalf("unexpected header = %q, %v", value, ok)
			}
		})
	}
}

func TestCrossPlatformCoverageValidateTokenAndHeaderBoundaries(t *testing.T) {
	invalid := [][]byte{
		nil,
		{},
		[]byte("bad\tvalue"),
		{0xff},
		[]byte(strings.Repeat("x", maxHeaderBytes)),
	}
	for _, value := range invalid {
		if _, err := validateToken(value); err == nil {
			t.Fatalf("validateToken accepted %q", value)
		}
	}
	value, err := validateToken([]byte(`quoted"value`))
	if err != nil {
		t.Fatal(err)
	}
	result := Result{State: StateReady, token: value}
	if header, ok := result.HeaderValue(); !ok || header != `{"umid":"quoted\"value"}` {
		t.Fatalf("header = %q, %v", header, ok)
	}
	code := int32(71)
	diagnostic := Result{State: StateError, callbackCode: code, hasCallbackCode: true}.DiagnosticDetail()
	if diagnostic["callback_code"] != code || diagnostic["token_length"] != 0 || diagnostic["token_fingerprint"] != "" {
		t.Fatalf("diagnostic = %#v", diagnostic)
	}
	if header, ok := (Result{State: StateReady, token: strings.Repeat("x", maxHeaderBytes)}).HeaderValue(); ok || header != "" {
		t.Fatalf("oversized header = %q, %v", header, ok)
	}
}

func TestCrossPlatformCoverageProviderTimeoutIgnoresLateCallback(t *testing.T) {
	late := make(chan func([]byte, int32, error), 1)
	provider := newProvider(func(_ string, callback func([]byte, int32, error)) (nativeSession, int32, error) {
		late <- callback
		return nativeSession{handle: 1, callback: 2}, 1, nil
	}, successfulLocator, 5*time.Millisecond)

	result := provider.resolve()
	if result.State != StateTimeout {
		t.Fatalf("result = %#v", result)
	}
	(<-late)([]byte("too-late"), 0, nil)
	if second := provider.resolve(); second.State != StateTimeout || second.token != "" {
		t.Fatalf("late callback changed result: %#v", second)
	}
}

func TestCrossPlatformCoverageProviderConcurrentCallersInitializeOnce(t *testing.T) {
	var calls atomic.Int32
	provider := newProvider(func(_ string, callback func([]byte, int32, error)) (nativeSession, int32, error) {
		calls.Add(1)
		callback([]byte("shared"), 0, nil)
		return nativeSession{}, 1, nil
	}, successfulLocator, time.Second)

	var wait sync.WaitGroup
	for range 16 {
		wait.Add(1)
		go func() {
			defer wait.Done()
			if result := provider.resolve(); result.token != "shared" {
				t.Errorf("result = %#v", result)
			}
		}()
	}
	wait.Wait()
	if got := calls.Load(); got != 1 {
		t.Fatalf("start calls = %d", got)
	}
}

func TestCrossPlatformCoverageRuntimeLibraryName(t *testing.T) {
	tests := map[string]string{
		"darwin/amd64":  "x7k2m9p4q1w8.dylib",
		"darwin/arm64":  "x7k2m9p4q1w8.dylib",
		"linux/amd64":   "libx7k2m9p4q1w8.so",
		"linux/arm64":   "libx7k2m9p4q1w8.so",
		"windows/amd64": "x7k2m9p4q1w864.dll",
		"windows/arm64": "x7k2m9p4q1w864.dll",
	}
	for platform, want := range tests {
		goos, goarch, _ := strings.Cut(platform, "/")
		got, err := libraryName(goos, goarch)
		if err != nil || got != want {
			t.Fatalf("libraryName(%s) = %q, %v", platform, got, err)
		}
	}
	if _, err := libraryName("plan9", "amd64"); err == nil {
		t.Fatal("unsupported platform accepted")
	}
}

func TestCrossPlatformCoverageCopyCString(t *testing.T) {
	terminated := make([]byte, maxHeaderBytes)
	copy(terminated, "copied-value")
	got, err := copyCString(unsafe.Pointer(&terminated[0]))
	if err != nil || string(got) != "copied-value" {
		t.Fatalf("copyCString = %q, %v", got, err)
	}
	terminated[0] = 'X'
	if string(got) != "copied-value" {
		t.Fatalf("copyCString retained native memory: %q", got)
	}
	if _, err := copyCString(nil); err == nil {
		t.Fatal("copyCString accepted nil")
	}
	lastByteTerminated := make([]byte, maxHeaderBytes)
	for index := range lastByteTerminated[:maxHeaderBytes-1] {
		lastByteTerminated[index] = 'x'
	}
	got, err = copyCString(unsafe.Pointer(&lastByteTerminated[0]))
	if err != nil || len(got) != maxHeaderBytes-1 {
		t.Fatalf("copyCString last-byte terminator = %d bytes, %v", len(got), err)
	}

	unterminated := make([]byte, maxHeaderBytes)
	for index := range unterminated {
		unterminated[index] = 'x'
	}
	if _, err := copyCString(unsafe.Pointer(&unterminated[0])); err == nil {
		t.Fatal("copyCString accepted unterminated input")
	}

	terminatorBeyondLimit := make([]byte, maxHeaderBytes+1)
	for index := range terminatorBeyondLimit[:maxHeaderBytes] {
		terminatorBeyondLimit[index] = 'x'
	}
	if _, err := copyCString(unsafe.Pointer(&terminatorBeyondLimit[0])); err == nil {
		t.Fatal("copyCString read a terminator beyond the scan limit")
	}
}

func TestCrossPlatformCoverageResolveLibraryPathUsesResolvedExecutable(t *testing.T) {
	testseam.Swap(t, &materializeRuntimePayload, func(string, string, string) (string, error) {
		return "", errors.New("embedded payload disabled for sidecar test")
	})
	root := t.TempDir()
	realDir := filepath.Join(root, "real")
	payloadDir := filepath.Join(realDir, ".dws-runtime", PayloadVersion)
	if err := os.MkdirAll(filepath.Join(payloadDir, "ps"), 0o755); err != nil {
		t.Fatal(err)
	}
	name, err := libraryName(runtimeGOOS(), runtimeGOARCH())
	if err != nil {
		t.Skip(err)
	}
	for _, path := range []string{filepath.Join(payloadDir, name), filepath.Join(payloadDir, "manifest.json")} {
		if err := os.WriteFile(path, []byte("test"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	testseam.Swap(t, &osExecutable, func() (string, error) { return filepath.Join(root, "bin", "dws"), nil })
	testseam.Swap(t, &evalSymlinks, func(string) (string, error) { return filepath.Join(realDir, "dws"), nil })
	got, err := resolveLibraryPath()
	if err != nil || got != filepath.Join(payloadDir, name) {
		t.Fatalf("resolveLibraryPath = %q, %v", got, err)
	}

	t.Run("missing payload", func(t *testing.T) {
		missingDir := filepath.Join(root, "missing")
		testseam.Swap(t, &osExecutable, func() (string, error) { return filepath.Join(missingDir, "dws"), nil })
		testseam.Swap(t, &evalSymlinks, func(string) (string, error) { return filepath.Join(missingDir, "dws"), nil })
		if _, err := resolveLibraryPath(); err == nil || classifyLocationError(err) != "payload_unavailable" {
			t.Fatalf("resolveLibraryPath error = %v", err)
		}
	})

	t.Run("unsupported platform", func(t *testing.T) {
		testseam.Swap(t, &currentGOOS, "plan9")
		if _, err := resolveLibraryPath(); err == nil || classifyLocationError(err) != "unsupported_platform" {
			t.Fatalf("resolveLibraryPath error = %v", err)
		}
	})
	t.Run("executable path", func(t *testing.T) {
		testseam.Swap(t, &osExecutable, func() (string, error) { return "", errors.New("executable") })
		if _, err := resolveLibraryPath(); err == nil || !strings.Contains(err.Error(), "executable path") {
			t.Fatalf("resolveLibraryPath error = %v", err)
		}
	})
}

func TestCrossPlatformCoverageResolveLibraryPathUsesEmbeddedPayload(t *testing.T) {
	if _, err := materializeRuntimePayload(t.TempDir(), runtime.GOOS, runtime.GOARCH); err != nil {
		t.Fatal(err)
	}
	expected := filepath.Join(t.TempDir(), "library")
	testseam.Swap(t, &materializeRuntimePayload, func(string, string, string) (string, error) {
		return expected, nil
	})
	testseam.Swap(t, &userCacheDir, func() (string, error) { return t.TempDir(), nil })
	if path, err := resolveLibraryPath(); err != nil || path != expected {
		t.Fatalf("resolveLibraryPath = %q, %v", path, err)
	}
}

func successfulLocator() (string, error) { return "/payload/library", nil }

func mapValues(input map[string]any) []string {
	result := make([]string, 0, len(input))
	for _, value := range input {
		result = append(result, fmt.Sprint(value))
	}
	return result
}

func runtimeGOOS() string   { return runtime.GOOS }
func runtimeGOARCH() string { return runtime.GOARCH }
