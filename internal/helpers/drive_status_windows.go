//go:build windows

package helpers

import (
	"path/filepath"
	"strings"
)

// isSafeRemoteSegmentPlatform 承载 Windows 专有的单层名称校验。
//
// 除盘符与 Clean 改写外，还拒绝 Win32 非法字符、尾随点/空格以及设备保留名。
// 这些名称即使不造成词法逃逸，也可能无法落盘、落到 ADS，或与另一个名称映射到
// 同一 Win32 对象；文件夹镜像必须在构建权威远端树时统一 fail closed。
func isSafeRemoteSegmentPlatform(name string) bool {
	if filepath.VolumeName(name) != "" {
		return false
	}
	if filepath.Clean(name) != name {
		return false
	}
	if strings.TrimRight(name, " .") != name {
		return false
	}
	for _, char := range name {
		if char < 0x20 || char == 0x7f || strings.ContainsRune(`<>:"|?*`, char) {
			return false
		}
	}
	if isWindowsReservedMirrorName(name) {
		return false
	}
	return true
}

func isWindowsReservedMirrorName(name string) bool {
	base := name
	if index := strings.IndexAny(base, ".:"); index >= 0 {
		base = base[:index]
	}
	base = strings.ToUpper(strings.TrimRight(base, " ."))
	switch base {
	case "CON", "PRN", "AUX", "NUL", "CONIN$", "CONOUT$":
		return true
	}
	if len(base) == 4 && (strings.HasPrefix(base, "COM") || strings.HasPrefix(base, "LPT")) && base[3] >= '1' && base[3] <= '9' {
		return true
	}
	runes := []rune(base)
	return len(runes) == 4 && (strings.HasPrefix(base, "COM") || strings.HasPrefix(base, "LPT")) &&
		strings.ContainsRune("¹²³", runes[3])
}
