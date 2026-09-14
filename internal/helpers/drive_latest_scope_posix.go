//go:build !windows

package helpers

// driveLatestScopeValue 按**本次构建的目标 shell** 渲染恢复命令里的用户值：返回可内联的片段，
// 第二个返回值为 false 时表示该值不能安全进入可执行命令，调用方须改用占位符。
//
// 非 Windows 构建面向 POSIX shell，单引号可靠地关闭所有展开，故含元字符的值引用后内联即安全。
// Windows 构建见 drive_latest_scope_windows.go —— 两个平台的策略本体都是
// shell_quote.go 里的纯函数，可在任意平台被测试直接调用；本文件只做编译期绑定。
func driveLatestScopeValue(value string) (string, bool) {
	return driveLatestPosixScopeValue(value)
}
