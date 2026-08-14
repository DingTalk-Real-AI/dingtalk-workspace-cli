//go:build !windows

package helpers

import "os"

// driveReplaceFile 在同一文件系统内以 source 替换既有非目录 target。
// Unix 的 rename(2) 原生提供原子替换语义。
func driveReplaceFile(source, target string) error {
	return os.Rename(source, target)
}
