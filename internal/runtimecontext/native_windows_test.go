//go:build windows

package runtimecontext

import "unsafe"

func nativeInitializerForTest(call func(int32, uintptr) int32) nativeInitializer {
	return func(environment int32, callback uintptr) int32 {
		return call(environment, callback)
	}
}

func invokeNativeCallbackForTest(callback any, pointer unsafe.Pointer, code int32) uintptr {
	return callback.(nativeCallbackFunc)(pointer, code)
}
