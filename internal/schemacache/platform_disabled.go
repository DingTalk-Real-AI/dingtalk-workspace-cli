//go:build !((darwin && arm64) || (linux && amd64))

package schemacache

func openPlatform(string, *Counters) (backend, error) {
	// Keep this stub free of os.UserCacheDir and all filesystem calls.
	return nil, ErrDisabled
}
