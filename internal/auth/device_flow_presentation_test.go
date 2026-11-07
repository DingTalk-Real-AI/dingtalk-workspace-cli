package auth

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/i18n"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/tui"
)

func TestCrossPlatformCoverageDeviceAuthorizationPresentation(t *testing.T) {
	previousLang := i18n.Lang()
	t.Cleanup(func() { i18n.SetLang(previousLang) })
	plain := func(s string) string { return s }
	testseam.Swap(t, &dfBold, plain)
	testseam.Swap(t, &dfYellow, plain)
	testseam.Swap(t, &dfDim, plain)
	base := "https://example.test/verify?caller=dws&callerUmt=" + strings.Repeat("synthetic-value_", 20)
	complete := base + "&user_code=DEMO-CODE#authorize"
	for _, locale := range []struct{ lang, code, expiry, completeLabel, manualLabel string }{
		{"en", "authorization code: DEMO-CODE", "Authorization code will expire in 900 seconds.", "Authorization link (code included):", "Link for entering the code manually:"},
		{"zh", "授权码: DEMO-CODE", "授权码将在 900 秒后过期。", "授权链接（已填入授权码）：", "手动输入授权码的链接："},
	} {
		for _, links := range []struct{ name, base, complete string }{
			{"both", base, complete},
			{"manual_only", base, ""},
			{"complete_only", "", complete},
			{"neither", "", ""},
		} {
			t.Run(locale.lang+"/"+links.name, func(t *testing.T) {
				i18n.SetLang(locale.lang)
				var output bytes.Buffer
				dfPrintDeviceAuthorization(&output, &DeviceAuthResponse{
					UserCode: "DEMO-CODE", ExpiresIn: 900,
					VerificationURI: links.base, VerificationURIComplete: links.complete,
				})
				got := output.String()
				if !strings.HasPrefix(got, "  "+locale.code+"\n  "+locale.expiry+"\n\n") {
					t.Fatal("authorization code and expiry must appear together before links")
				}
				if strings.ContainsAny(got, "╭╮╰╯│─\x1b\r") {
					t.Fatal("authorization instructions contain a frame or terminal controls")
				}
				var urls []string
				for _, line := range strings.Split(got, "\n") {
					if strings.HasPrefix(line, "https://") {
						urls = append(urls, line)
					} else if tui.PlainRuneWidth(line) > 60 {
						t.Fatal("instruction text exceeds a narrow terminal")
					}
				}
				var wantURLs []string
				for _, link := range []struct{ url, label string }{
					{links.complete, locale.completeLabel}, {links.base, locale.manualLabel},
				} {
					if link.url == "" {
						if strings.Contains(got, link.label) {
							t.Fatal("missing URL has a dangling label")
						}
						continue
					}
					wantURLs = append(wantURLs, link.url)
					if !strings.Contains(got, fmt.Sprintf("  %s\n%s\n\n", link.label, link.url)) {
						t.Fatal("URL must be complete, unstyled, and on its own logical line")
					}
				}
				if strings.Join(urls, "\n") != strings.Join(wantURLs, "\n") {
					t.Fatal("expected one complete link followed by one manual link, without truncation or wrapping")
				}
			})
		}
	}
}
