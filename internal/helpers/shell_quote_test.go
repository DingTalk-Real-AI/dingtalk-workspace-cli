package helpers

import (
	"runtime"
	"testing"
)

// TestCrossPlatformCoverageShellQuoteArg 表驱动锁定 argv 引用规则。分两类断言：
//   - 安全值原样返回（保持恢复命令可读，且在 PowerShell/cmd 下同样可复制）；
//   - 含任何 shell 元字符的值整体单引号包裹，内嵌单引号按 POSIX 标准写法处理
//     （闭合、拼反斜杠转义的单引号、重开）。
//
// 往返正确性另有 Unix 下用真 sh 求值的对照测试（shell_quote_roundtrip_unix_test.go）。
//
// TestCrossPlatformCoverage 前缀是门禁约定：平台覆盖率门禁只跑该前缀（与 TestAllShortcuts）的
// 测试，shell_quote.go 的覆盖全靠这一条，去掉前缀会让 Coverage (macOS)/(Windows) 直接红。
// 详见 drive_latest_incomplete_test.go 头部说明。
func TestCrossPlatformCoverageShellQuoteArg(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		// —— 无需引用：白名单字符 ——
		{name: "plain_id", in: "ws-1", want: "ws-1"},
		{name: "digits", in: "1234567890", want: "1234567890"},
		{name: "upper_lower", in: "abcXYZ", want: "abcXYZ"},
		{name: "all_safe_punct", in: "_@+=:,./-", want: "_@+=:,./-"},
		{name: "path_like", in: "/tmp/a.b/c-d_e", want: "/tmp/a.b/c-d_e"},

		// —— 必须引用：真实业务形态 ——
		// workspace 常见形态就是带查询串的 URL：? 与 & 都不在白名单，& 在裸拼接下会拆命令。
		{name: "url_with_query", in: "https://alidocs.dingtalk.com/i/nodes/x?spaceId=1&type=doc",
			want: `'https://alidocs.dingtalk.com/i/nodes/x?spaceId=1&type=doc'`},

		// —— 必须引用：shell 元字符 ——
		{name: "space", in: "sp 7", want: `'sp 7'`},
		{name: "tab", in: "a\tb", want: "'a\tb'"},
		{name: "newline", in: "a\nb", want: "'a\nb'"},
		{name: "semicolon", in: "a;id", want: `'a;id'`},
		{name: "ampersand", in: "a&b", want: `'a&b'`},
		{name: "pipe", in: "a|b", want: `'a|b'`},
		{name: "command_substitution", in: "$(id)", want: `'$(id)'`},
		{name: "backtick", in: "`id`", want: "'`id`'"},
		{name: "variable", in: "$HOME", want: `'$HOME'`},
		// % 不在白名单：cmd.exe 会展开 %VAR%，故含 % 的值一律视为需引用。
		{name: "windows_variable", in: "%PATH%", want: `'%PATH%'`},
		{name: "url_percent_escape", in: "a%20b", want: `'a%20b'`},
		{name: "glob", in: "*.txt", want: `'*.txt'`},
		{name: "redirect", in: "a>b", want: `'a>b'`},
		{name: "backslash", in: `a\b`, want: `'a\b'`},
		{name: "double_quote", in: `a"b`, want: `'a"b'`},
		{name: "history_expansion", in: "a!b", want: `'a!b'`},
		{name: "brace", in: "{a,b}", want: `'{a,b}'`},
		{name: "paren", in: "(a)", want: `'(a)'`},
		{name: "tilde", in: "~/x", want: `'~/x'`},
		{name: "hash", in: "a#b", want: `'a#b'`},

		// —— 单引号：唯一无法直接放进单引号串的字符 ——
		{name: "single_quote", in: "it's", want: `'it'\''s'`},
		{name: "only_single_quote", in: "'", want: `''\'''`},
		{name: "two_single_quotes", in: "a'b'c", want: `'a'\''b'\''c'`},

		// —— 边界 ——
		// 空串必须显式成 ''，否则参数会从 argv 里整个消失（--workspace 吞掉下一个 token）。
		{name: "empty", in: "", want: `''`},
		// 非 ASCII 一律引用：保守优于漏字符，中文目录名很常见。
		{name: "cjk", in: "报表", want: `'报表'`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := ShellQuoteArg(tc.in); got != tc.want {
				t.Fatalf("ShellQuoteArg(%q)\n got: %s\nwant: %s", tc.in, got, tc.want)
			}
		})
	}
}

// TestCrossPlatformCoverageDriveLatestScopeValueStrategies 直接测两条平台策略的本体。它们是
// shell_quote.go 里的纯函数、与构建平台无关，因此**在任一平台都能验证 Windows 侧的行为** ——
// 这一点是刻意设计：真 shell 往返测试只能在 Unix 跑，Windows 的安全属性必须由这里钉住，
// 否则「Windows 上不会把不受信任值放进可执行命令」就成了无人复核的断言。
func TestCrossPlatformCoverageDriveLatestScopeValueStrategies(t *testing.T) {
	// 这些值在至少一种目标 shell 下会改变命令解析。
	hostile := []struct {
		name  string
		value string
	}{
		{"cmd_command_separator", "sp-7&whoami"}, // cmd.exe：& 分隔命令，单引号不是引号
		{"cmd_variable", "%PATH%"},               // cmd.exe：无条件展开
		{"workspace_url", "https://x/y?a=1&b=2"}, // 合法 workspace 形态，含 &
		{"space", "sp 7"},
		{"pwsh_statement_separator", "a;id"}, // PowerShell：; 分隔语句
		{"posix_substitution", "$(id)"},
		{"backtick", "`id`"},
		{"posix_variable", "$HOME"},
		{"pipe", "a|b"},
		{"redirect", "a>b"},
		{"caret", "a^b"}, // cmd.exe 转义符
		{"single_quote", "it's"},
		{"cjk", "报表"},
		{"empty", ""},
	}
	for _, tc := range hostile {
		t.Run("hostile/"+tc.name, func(t *testing.T) {
			// POSIX：始终可内联，但必须是引用后的形态（不能等于原值）。
			got, ok := driveLatestPosixScopeValue(tc.value)
			if !ok {
				t.Fatalf("POSIX 策略应始终可内联: %q", tc.value)
			}
			if got == tc.value {
				t.Fatalf("POSIX 策略未引用危险值: %q", tc.value)
			}
			// Windows：一律拒绝内联 —— 没有对 cmd.exe 与 PowerShell 同时安全的引用形式。
			if inline, ok := driveLatestWindowsScopeValue(tc.value); ok {
				t.Fatalf("Windows 策略不得内联危险值 %q（得到 %q）", tc.value, inline)
			}
		})
	}

	// 安全值：两条策略都原样内联，命令保持可读、跨 shell 可复制。
	for _, v := range []string{"ws-1", "sp-7", "folder-root", "modifyTime", "7d", "2026-08-01", "a.b/c-d_e", "a@b"} {
		t.Run("bare/"+v, func(t *testing.T) {
			if got, ok := driveLatestPosixScopeValue(v); !ok || got != v {
				t.Fatalf("POSIX 应原样内联安全值 %q: got %q ok=%v", v, got, ok)
			}
			if got, ok := driveLatestWindowsScopeValue(v); !ok || got != v {
				t.Fatalf("Windows 应原样内联安全值 %q: got %q ok=%v", v, got, ok)
			}
		})
	}

	// 空串在 POSIX 下引用成一对空单引号（否则参数会从 argv 消失）；Windows 侧归入不可内联。
	if got, ok := driveLatestPosixScopeValue(""); !ok || got != "''" {
		t.Fatalf("POSIX 空串应引用成一对空单引号: got %q ok=%v", got, ok)
	}
}

// TestCrossPlatformCoverageShellBareCharsetHasNoMetacharacters 锁定「安全值可原样内联」的根本
// 前提：白名单里不能含任何一种目标 shell 会特殊解释的字符。
//
// 这比「用某个 shell 跑一遍」更本质 —— 内联路径的安全性不来自引用正确，而来自字符集本身无害；
// 而拒绝内联的路径（Windows 侧）连引用都不需要。三套元字符合并检查：POSIX sh、PowerShell、
// cmd.exe。`%` 曾因 URL 转义（%20）被误列入白名单，而 cmd.exe 会无条件展开 %VAR%，这条测试
// 同时是该缺陷的回归锁。
func TestCrossPlatformCoverageShellBareCharsetHasNoMetacharacters(t *testing.T) {
	// 逐字符列出，避免用字符串字面量时漏掉转义细节。
	meta := []rune{
		'&', '|', '<', '>', ';', '(', ')', '{', '}', '[', ']', // 分隔/分组
		'$', '`', '"', '\'', '\\', // 引用与替换
		'^', '%', // cmd.exe：转义符与变量展开
		'*', '?', '~', '#', '!', // 通配、家目录、注释、历史/取反
		' ', '\t', '\n', '\r', // 空白：拆参与换行注入
	}
	for _, r := range meta {
		if !shellNeedsQuote(r) {
			t.Fatalf("shell 元字符 %q 落在「无需引用」白名单内，安全值会被原样内联", string(r))
		}
	}
	// 反向哨兵：常规标识符字符必须留在白名单内，否则恢复命令会被无谓地全量引用/降级。
	for _, r := range []rune{'a', 'Z', '0', '9', '_', '-', '.', '/', ':', ',', '=', '+', '@'} {
		if shellNeedsQuote(r) {
			t.Fatalf("常规字符 %q 被判为需引用，会让恢复命令可读性无谓下降", string(r))
		}
	}
}

// TestCrossPlatformCoverageDriveLatestScopeValueBinding 断言当前构建绑定到了正确的策略。
// 这条测试在每个平台各自成立，把「哪个平台用哪条策略」也纳入 CI（Coverage (Windows) 会跑它）。
func TestCrossPlatformCoverageDriveLatestScopeValueBinding(t *testing.T) {
	const hostile = "sp-7&whoami"
	inline, ok := driveLatestScopeValue(hostile)
	if runtime.GOOS == "windows" {
		if ok {
			t.Fatalf("windows 构建不得内联 %q（得到 %q）", hostile, inline)
		}
		return
	}
	if !ok {
		t.Fatalf("posix 构建应可内联 %q", hostile)
	}
	if inline != `'sp-7&whoami'` {
		t.Fatalf("posix 构建应单引号包裹: %s", inline)
	}
}
