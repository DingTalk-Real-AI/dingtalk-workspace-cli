package clitrack

import (
	"bytes"
	"io"
	"os"
)

// captureStdout 拦截 stdout,将 fn 的输出同时写到原 stdout 和 buffer。
// 返回捕获到的输出内容和 fn 的 error。Pipe 创建失败时降级为不捕获,
// 保证 fn 一定被执行。
//
// 注意:这会临时替换 os.Stdout,对依赖 TTY 检测、进度条、ANSI 颜色的 CLI
// 可能有副作用——这正是 CaptureOutput 默认关闭的原因。
func captureStdout(fn func() error) (string, error) {
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		return "", fn()
	}
	os.Stdout = w

	var buf bytes.Buffer
	done := make(chan struct{})
	go func() {
		_, _ = io.Copy(io.MultiWriter(old, &buf), r)
		close(done)
	}()

	execErr := fn()

	w.Close()
	<-done
	os.Stdout = old

	return buf.String(), execErr
}
