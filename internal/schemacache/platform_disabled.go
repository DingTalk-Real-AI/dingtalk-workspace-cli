//go:build !((darwin || linux) && (amd64 || arm64))

package schemacache

func openPlatform(string, *Counters, bool) (backend, error) {
	// Keep this stub free of os.UserCacheDir and all filesystem calls.
	return nil, ErrDisabled
}
