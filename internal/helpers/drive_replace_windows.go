//go:build windows

package helpers

import "golang.org/x/sys/windows"

// driveReplaceFile 使用 MoveFileEx(MOVEFILE_REPLACE_EXISTING) 替换既有非目录目标。
// 不使用“先删除再改名”，避免覆盖过程中出现目标暂时缺失或原文件丢失。
func driveReplaceFile(source, target string) error {
	return windows.Rename(source, target)
}
