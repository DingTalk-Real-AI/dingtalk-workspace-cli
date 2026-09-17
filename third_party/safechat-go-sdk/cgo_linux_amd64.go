//go:build linux && amd64

package safechat

/*
#cgo CFLAGS: -I${SRCDIR}/csrc -I${SRCDIR}/csrc/include -DDLL_EXPORT -D_GNU_SOURCE
#cgo LDFLAGS: -L${SRCDIR}/lib/linux_amd64 -lsafechat -lpthread -ldl -lm
*/
import "C"
