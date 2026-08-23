//go:build windows

package helpers

// driveLatestScopeValue 按**本次构建的目标 shell** 渲染恢复命令里的用户值：返回可内联的片段，
// 第二个返回值为 false 时表示该值不能安全进入可执行命令，调用方须改用占位符。
//
// Windows 构建下没有对 cmd.exe 与 PowerShell 同时成立的引用形式（cmd.exe 不认单引号，双引号
// 又挡不住 %VAR% 展开），故只内联本身就安全的值；理由与取舍详见
// driveLatestWindowsScopeValue 的注释。POSIX 构建见 drive_latest_scope_posix.go。
func driveLatestScopeValue(value string) (string, bool) {
	return driveLatestWindowsScopeValue(value)
}
