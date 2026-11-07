//go:build darwin && amd64

package safechat

/*
#cgo CFLAGS: -I${SRCDIR}/csrc -I${SRCDIR}/csrc/include -DDLL_EXPORT
#cgo LDFLAGS: -L${SRCDIR}/lib/darwin_amd64 -lsafechat -lpthread -ldl -lm -framework Security
*/
import "C"
