//go:build !windows

package helpers

// isSafeRemoteSegmentPlatform 在非 Windows 平台没有额外约束：filepath.VolumeName
// 恒为空字符串，而调用方已过滤掉 ""、"."、".." 与任意平台分隔符，剩下的名称
// filepath.Clean 不会再改写。
func isSafeRemoteSegmentPlatform(string) bool { return true }
