//go:build darwin || linux || windows

package runtimecontext

import (
	"errors"
	"runtime"
	"testing"
	"unsafe"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
)

func TestCrossPlatformCoverageNativeAdapter(t *testing.T) {
	t.Run("default bindings", func(t *testing.T) {
		library, symbolName := "libc.so.6", "strlen"
		switch runtime.GOOS {
		case "darwin":
			library = "/usr/lib/libSystem.B.dylib"
		case "windows":
			library, symbolName = "kernel32.dll", "GetCurrentProcessId"
		}
		handle, err := openNativeLibrary(library)
		if err != nil {
			t.Fatalf("openNativeLibrary(%q): %v", library, err)
		}
		symbol, err := lookupNativeSymbol(handle, symbolName)
		if err != nil {
			t.Fatalf("lookupNativeSymbol(%q): %v", symbolName, err)
		}
		if initialize := bindNativeInitialize(symbol); initialize == nil {
			t.Fatal("bindNativeInitialize returned nil")
		}
	})

	t.Run("load failure", func(t *testing.T) {
		testseam.Swap(t, &openNativeLibrary, func(string) (uintptr, error) {
			return 0, errors.New("load")
		})
		if _, _, err := startNative("missing", func([]byte, int32, error) {}); !errors.Is(err, errNativeLoad) {
			t.Fatalf("startNative error = %v", err)
		}
	})

	t.Run("symbol failure", func(t *testing.T) {
		testseam.Swap(t, &openNativeLibrary, func(string) (uintptr, error) { return 11, nil })
		testseam.Swap(t, &lookupNativeSymbol, func(handle uintptr, name string) (uintptr, error) {
			if handle != 11 || name != initSymbol {
				t.Fatalf("lookup = %d, %q", handle, name)
			}
			return 0, errors.New("symbol")
		})
		session, _, err := startNative("payload", func([]byte, int32, error) {})
		if !errors.Is(err, errNativeSymbol) || session.handle != 11 {
			t.Fatalf("startNative = %#v, %v", session, err)
		}
	})

	t.Run("callback and initialize", func(t *testing.T) {
		testseam.Swap(t, &openNativeLibrary, func(string) (uintptr, error) { return 12, nil })
		testseam.Swap(t, &lookupNativeSymbol, func(uintptr, string) (uintptr, error) { return 13, nil })
		var nativeCallback any
		testseam.Swap(t, &makeNativeCallback, func(fn any) uintptr {
			nativeCallback = fn
			return 14
		})
		testseam.Swap(t, &bindNativeInitialize, func(symbol uintptr) nativeInitializer {
			if symbol != 13 {
				t.Fatalf("symbol = %d", symbol)
			}
			return nativeInitializerForTest(func(environment int32, callback uintptr) int32 {
				if environment != initializationEnvironment || callback != 14 {
					t.Fatalf("initialize = env %d, callback %d", environment, callback)
				}
				value := make([]byte, maxHeaderBytes)
				copy(value, "native-value")
				invokeNativeCallbackForTest(nativeCallback, unsafe.Pointer(&value[0]), 23)
				return 1
			})
		})
		var got string
		var code int32
		session, accepted, err := startNative("payload", func(raw []byte, callbackCode int32, copyErr error) {
			if copyErr != nil {
				t.Fatal(copyErr)
			}
			got, code = string(raw), callbackCode
		})
		if err != nil || accepted != 1 || session.handle != 12 || session.callback != 14 || got != "native-value" || code != 23 {
			t.Fatalf("startNative = %#v, %d, %q, %d, %v", session, accepted, got, code, err)
		}
	})

	t.Run("binding panic", func(t *testing.T) {
		testseam.Swap(t, &openNativeLibrary, func(string) (uintptr, error) { return 15, nil })
		testseam.Swap(t, &lookupNativeSymbol, func(uintptr, string) (uintptr, error) { return 16, nil })
		testseam.Swap(t, &makeNativeCallback, func(any) uintptr { return 17 })
		testseam.Swap(t, &bindNativeInitialize, func(uintptr) nativeInitializer {
			panic("binding")
		})
		if _, _, err := startNative("payload", func([]byte, int32, error) {}); !errors.Is(err, errNativeBinding) {
			t.Fatalf("startNative error = %v", err)
		}
	})
}
