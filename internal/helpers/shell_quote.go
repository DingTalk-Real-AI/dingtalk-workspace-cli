package helpers

import "strings"

// ShellQuoteArg 按 POSIX sh 规则把 s 引用成可安全放进可复制命令的单个 argv 元素。
//
// 为什么需要它：错误提示里的恢复命令是给用户直接复制到 shell 执行的，其中的查询域取自用户
// 输入（--workspace 的常见形态就是带查询串的 URL）。裸拼接下，合法 URL 里的 `&` 就会把命令
// 拆成后台任务，空格会拆参，`;` 与 `$()` 还能执行额外内容。
//
// 为什么不用 strconv.Quote（internal/auth 侧展示 profile 标识的既有做法）：那是 Go 语法引号，
// 产出双引号串，而 shell 双引号内 `$()`、反引号、`$VAR` 仍会展开——正是要防的场景。auth 那处
// 成立是因为它显式声明「仅作数据展示，不是可执行命令」，本函数的产物恰恰要能执行。
//
// 策略是「必要时才引用」：全由安全字符组成时原样返回，命令保持可读、在 PowerShell/cmd 下同样
// 可复制；只要含一个非安全字符就整体单引号包裹——单引号内 POSIX sh 不做任何展开（$ ` \ ! 全部
// 字面化），是唯一无需逐字符转义的形式。单引号自身无法出现在单引号串内，按 POSIX 标准写法
// 先闭合、拼一个反斜杠转义的单引号、再重开——即把每个单引号替换成下面这四个字符：
//
//	'\''
//
// 空串必须显式引用成一对空单引号，否则该参数会从 argv 里整个消失，后面的 token 会被前一个
// flag 吞掉。
func ShellQuoteArg(s string) string {
	if s == "" {
		return "''"
	}
	if shellValueIsBare(s) {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// shellValueIsBare 报告 s 能否原样出现在命令行里而不改变**任何**常见 shell 的解析：非空，
// 且每个字符都落在保守白名单内。这类值在 POSIX sh、PowerShell 与 cmd.exe 下含义一致，
// 无需引用即可内联进可复制命令。
func shellValueIsBare(s string) bool {
	return s != "" && !strings.ContainsFunc(s, shellNeedsQuote)
}

// driveLatestPosixScopeValue 是 POSIX shell 下把用户值放进可复制命令的策略：单引号内 sh 不做
// 任何展开，因此含元字符的值引用后即可安全内联。第二个返回值恒为 true。
func driveLatestPosixScopeValue(value string) (string, bool) {
	return ShellQuoteArg(value), true
}

// driveLatestWindowsScopeValue 是 Windows 构建下的策略：只内联本身就安全的值，其余一律不进
// 命令，由调用方降级为占位符 + 展示行。
//
// Windows 上不存在「一条命令同时对 cmd.exe 与 PowerShell 都安全」的引用形式：
//   - cmd.exe 根本不把单引号当引号，`--space-id 'sp-7&whoami'` 里的 & 仍然分隔命令，
//     粘贴即执行 whoami；
//   - cmd.exe 的双引号能挡 & | < >，却挡不住 %VAR% 展开；
//   - PowerShell 的单引号是引号，但内嵌单引号写作两个连续单引号，与 POSIX 的
//     闭合-反斜杠转义-重开写法不兼容。
//
// 而生成命令时无法知道用户会粘贴进哪个 shell。既然引用不可靠，就不把不受信任的值放进可执行
// 命令——与 internal/auth 侧「标识仅作数据展示，不是可执行命令」的既有做法同一思路。
func driveLatestWindowsScopeValue(value string) (string, bool) {
	if shellValueIsBare(value) {
		return value, true
	}
	return "", false
}

// shellNeedsQuote 报告 r 是否落在「无需引用」白名单之外。
//
// 用白名单而非黑名单：漏掉一个元字符就是一个注入口，而白名单漏一个字符只会多加一对无害的
// 引号。非 ASCII 一律视为需引用——中文目录名很常见，保守处理不会错。
//
// 白名单的门槛是「在 POSIX sh、PowerShell、cmd.exe 下含义都一致」，因此 % 被刻意排除：
// cmd.exe 会展开 %VAR%，一个看似普通的值（URL 里的 %20、含 %PATH% 的名字）原样内联后在
// cmd 里就会变形甚至泄露环境变量。@ 保留：PowerShell 的 splatting 只在 @( 、@{ 与
// @变量名 形态下生效，而那些字符本身都不在白名单里，且值总出现在 --flag 之后而非语句开头。
func shellNeedsQuote(r rune) bool {
	switch {
	case r >= 'a' && r <= 'z':
		return false
	case r >= 'A' && r <= 'Z':
		return false
	case r >= '0' && r <= '9':
		return false
	}
	return !strings.ContainsRune("_@+=:,./-", r)
}
