//go:build darwin || (linux && !openharmony)

package runtimecontext

import (
	"unsafe"

	"github.com/ebitengine/purego"
)

func nativeInitializerForTest(call func(int32, uintptr) int32) nativeInitializer {
	return func(_ purego.CDecl, environment int32, callback uintptr) int32 {
		return call(environment, callback)
	}
}

func invokeNativeCallbackForTest(callback any, pointer unsafe.Pointer, code int32) uintptr {
	return callback.(nativeCallbackFunc)(purego.CDecl{}, pointer, code)
}

func agentCallbacksForTest(get func(int32, *int32) uintptr, free func(unsafe.Pointer) uintptr) (uintptr, uintptr) {
	return purego.NewCallback(func(_ purego.CDecl, full int32, code *int32) uintptr { return get(full, code) }),
		purego.NewCallback(func(_ purego.CDecl, pointer unsafe.Pointer) uintptr { return free(pointer) })
}
