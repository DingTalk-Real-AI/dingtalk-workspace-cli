//go:build darwin || linux

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
