//go:build windows && amd64

package safechat

/*
#cgo CFLAGS: -I${SRCDIR}/csrc -I${SRCDIR}/csrc/include -DDLL_EXPORT -D_WIN32 -DWIN32
#cgo LDFLAGS: ${SRCDIR}/lib/windows_amd64/libsafechat.a -lws2_32 -lgdi32 -lcrypt32 -ladvapi32 -luser32 -lmsvcrt
*/
import "C"
