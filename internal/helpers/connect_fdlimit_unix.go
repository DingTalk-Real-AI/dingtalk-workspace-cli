//go:build !windows

package helpers

import (
	"fmt"
	"os"
	"syscall"
)

var connectGetrlimit = syscall.Getrlimit

func checkFDLimit() {
	var rlim syscall.Rlimit
	if err := connectGetrlimit(syscall.RLIMIT_NOFILE, &rlim); err != nil {
		return
	}
	if rlim.Cur < 512 {
		fmt.Fprintf(os.Stderr, "[connect][warn] 文件描述符上限 %d 偏低，多 agent 并发连接可能不稳定；建议 ulimit -n 1024\n", rlim.Cur)
	}
}
