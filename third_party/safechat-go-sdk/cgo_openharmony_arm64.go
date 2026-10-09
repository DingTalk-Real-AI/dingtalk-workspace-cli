//go:build openharmony && arm64

package safechat

/*
#cgo CFLAGS: -I${SRCDIR}/csrc -I${SRCDIR}/csrc/include -DDLL_EXPORT -D_GNU_SOURCE
#cgo LDFLAGS: -L${SRCDIR}/lib/openharmony_arm64 -lsafechat -lpthread -ldl -lm
*/
import "C"
