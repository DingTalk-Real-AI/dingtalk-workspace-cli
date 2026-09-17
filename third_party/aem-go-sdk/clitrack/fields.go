package clitrack

import (
	"os"
	"path/filepath"
	"strings"
)

// command 返回 CLI 二进制名(c1),如 "aem"。
func command() string {
	return filepath.Base(os.Args[0])
}

// commandLine 返回完整命令行参数(c2),如 "login --env prod"。
func commandLine() string {
	return strings.Join(os.Args[1:], " ")
}

// shellType 返回当前 Shell 类型(c6),取 $SHELL 的 basename(如 "zsh"、"bash")。
func shellType() string {
	shell := os.Getenv("SHELL")
	if shell == "" {
		return ""
	}
	return filepath.Base(shell)
}

// cwd 返回当前工作目录(c7),失败返回空字符串。
func cwd() string {
	dir, _ := os.Getwd()
	return dir
}

// truncate 按 rune 截断字符串到 maxLen,超出部分用 "..." 替换。
func truncate(s string, maxLen int) string {
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	if maxLen <= 3 {
		return string(runes[:maxLen])
	}
	return string(runes[:maxLen-3]) + "..."
}
