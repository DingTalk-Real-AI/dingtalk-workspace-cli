//go:build darwin || linux

package runtimecontext

import (
	"fmt"
	"unsafe"

	"github.com/ebitengine/purego"
)

const initSymbol = "k9Xm2pQv"

type nativeInitializer func(purego.CDecl, int32, uintptr) int32
type nativeCallbackFunc func(purego.CDecl, unsafe.Pointer, int32) uintptr

var (
	openNativeLibrary = func(path string) (uintptr, error) {
		return purego.Dlopen(path, purego.RTLD_NOW|purego.RTLD_LOCAL)
	}
	lookupNativeSymbol   = purego.Dlsym
	makeNativeCallback   = purego.NewCallback
	bindNativeInitialize = func(symbol uintptr) nativeInitializer {
		var initialize nativeInitializer
		purego.RegisterFunc(&initialize, symbol)
		return initialize
	}
)

func startNative(path string, callback func([]byte, int32, error)) (session nativeSession, accepted int32, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("%w: %v", errNativeBinding, recovered)
		}
	}()
	handle, err := openNativeLibrary(path)
	if err != nil {
		return nativeSession{}, 0, fmt.Errorf("%w: %v", errNativeLoad, err)
	}
	session.handle = handle
	symbol, err := lookupNativeSymbol(handle, initSymbol)
	if err != nil {
		return session, 0, fmt.Errorf("%w: %v", errNativeSymbol, err)
	}
	callbackPointer := makeNativeCallback(nativeCallbackFunc(func(_ purego.CDecl, token unsafe.Pointer, code int32) uintptr {
		raw, copyErr := copyCString(token)
		callback(raw, code, copyErr)
		return 0
	}))
	session.callback = callbackPointer
	initialize := bindNativeInitialize(symbol)
	accepted = initialize(purego.CDecl{}, initializationEnvironment, callbackPointer)
	return session, accepted, nil
}
