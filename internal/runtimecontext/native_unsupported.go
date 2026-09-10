//go:build !darwin && !linux && !windows

package runtimecontext

import "errors"

func startNative(string, func([]byte, int32, error)) (nativeSession, int32, error) {
	return nativeSession{}, 0, errors.New("unsupported runtime platform")
}
