//go:build linux && arm64

package safechat

/*
#cgo CFLAGS: -I${SRCDIR}/csrc -I${SRCDIR}/csrc/include -DDLL_EXPORT -D_GNU_SOURCE
#cgo LDFLAGS: -L${SRCDIR}/lib/linux_arm64 -lsafechat -lpthread -ldl -lm
*/
import "C"
