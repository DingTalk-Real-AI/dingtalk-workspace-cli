//go:build !windows

package helpers

import (
	"os/exec"
	"testing"
)

// TestCrossPlatformCoverageShellQuoteArgRoundTrip 把引用后的串交给真 /bin/sh 求值，证明它被解析回
// **恰好一个、且内容完全相同**的参数。上面那条表驱动测试只能证明「我以为的形态」，这条证明
// 「shell 认的形态」——两者不是一回事，恢复命令是给用户复制到 shell 里跑的，后者才是契约。
//
// 它同时是注入的负向对照：一旦引用失效，$(id) / `id` 会真的执行、空格与 & 会拆参，argc 或
// 取回的值必然不等于原值，测试立刻红。用 build tag 排除 Windows（无 POSIX sh），跨平台形态
// 断言留在 shell_quote_test.go。
func TestCrossPlatformCoverageShellQuoteArgRoundTrip(t *testing.T) {
	sh, err := exec.LookPath("sh")
	if err != nil {
		t.Skipf("POSIX sh unavailable: %v", err)
	}

	values := []string{
		"ws-1",
		"https://alidocs.dingtalk.com/i/nodes/x?spaceId=1&type=doc",
		"sp 7",
		"a;id",
		"a&b",
		"a|b",
		"$(id)",
		"`id`",
		"$HOME",
		"*.txt",
		"a>b",
		`a\b`,
		`a"b`,
		"a!b",
		"{a,b}",
		"(a)",
		"~/x",
		"a#b",
		"it's",
		"'",
		"a'b'c",
		"",
		"报表",
		"a\tb",
		"a\nb",
		"--not-a-flag",
	}

	for _, want := range values {
		t.Run(want, func(t *testing.T) {
			quoted := shellQuoteArg(want)

			// 1) 未被拆参也未被吞掉：位置参数个数必须恰好为 1。
			//    引用失效时 "sp 7" 会变 2 个、"" 会变 0 个、$(id) 会变 id 的输出词数。
			argc, err := exec.Command(sh, "-c", "set -- "+quoted+`; printf %s "$#"`).Output()
			if err != nil {
				t.Fatalf("argc probe failed for %q (quoted %s): %v", want, quoted, err)
			}
			if string(argc) != "1" {
				t.Fatalf("argc = %s, want 1 — %q quoted as %s split or vanished", argc, want, quoted)
			}

			// 2) 内容逐字节相同：展开、命令替换、转义都不得改动值。
			got, err := exec.Command(sh, "-c", "set -- "+quoted+`; printf %s "$1"`).Output()
			if err != nil {
				t.Fatalf("value probe failed for %q (quoted %s): %v", want, quoted, err)
			}
			if string(got) != want {
				t.Fatalf("round-trip mismatch\n  input:  %q\n  quoted: %s\n  shell:  %q", want, quoted, got)
			}
		})
	}
}
