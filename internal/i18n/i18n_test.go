package i18n

import (
	"strings"
	"testing"

	"golang.org/x/text/language"
)

func TestCrossPlatformCoverageLanguageSelectionAndTranslation(t *testing.T) {
	Init()
	SetLang(" zh_CN.UTF-8 ")
	if Lang() != "zh" || LangTag() != language.Chinese {
		t.Fatalf("Chinese language = %q, %v", Lang(), LangTag())
	}
	var knownKey string
	for key := range zhCatalog {
		knownKey = key
		break
	}
	if knownKey == "" || T(knownKey) != zhCatalog[knownKey] {
		t.Fatalf("known Chinese translation was not resolved: %q", knownKey)
	}

	SetLang("EN-us")
	if Lang() != "en" || LangTag() != language.English {
		t.Fatalf("English language = %q, %v", Lang(), LangTag())
	}
	if T("missing.translation.key") != "missing.translation.key" {
		t.Fatal("missing translation did not fall back to its key")
	}

	const fallbackKey = "test.english.fallback"
	oldEnglish, hadEnglish := enCatalog[fallbackKey]
	oldChinese, hadChinese := zhCatalog[fallbackKey]
	enCatalog[fallbackKey] = "Hello %s"
	delete(zhCatalog, fallbackKey)
	t.Cleanup(func() {
		if hadEnglish {
			enCatalog[fallbackKey] = oldEnglish
		} else {
			delete(enCatalog, fallbackKey)
		}
		if hadChinese {
			zhCatalog[fallbackKey] = oldChinese
		}
	})
	SetLang("zh")
	if got := Tf(fallbackKey, "Codex"); got != "Hello Codex" {
		t.Fatalf("Tf() fallback = %q", got)
	}
	if catalogForLang("zh") != nil && len(catalogForLang("zh")) == 0 {
		t.Fatal("Chinese catalog is empty")
	}
	if len(catalogForLang("unsupported")) == 0 {
		t.Fatal("unsupported language did not use English catalog")
	}
}

func TestCrossPlatformCoverageLoadCatalogHandlesValidAndInvalidResources(t *testing.T) {
	if catalog := loadCatalog("en"); len(catalog) == 0 {
		t.Fatal("loadCatalog(en) returned an empty catalog")
	}
	if catalog := loadCatalog("missing"); len(catalog) != 0 {
		t.Fatalf("loadCatalog(missing) = %#v", catalog)
	}
	if catalog := parseCatalog([]byte(`{`)); len(catalog) != 0 {
		t.Fatalf("parseCatalog(invalid) = %#v", catalog)
	}
	if catalog := parseCatalog([]byte(`{"key":"value"}`)); catalog["key"] != "value" {
		t.Fatalf("parseCatalog(valid) = %#v", catalog)
	}
	SetLang(strings.Repeat(" ", 2))
	if Lang() != "en" {
		t.Fatalf("blank language = %q", Lang())
	}
}

func TestAuthLoginSummaryTranslations(t *testing.T) {
	previous := Lang()
	t.Cleanup(func() { SetLang(previous) })

	SetLang("en")
	english := map[string]string{
		"推荐权限已全部授权或没有可授权项": "Recommended permissions are already granted, or none are available",
		"登录成功！": "Login successful!",
		"企业":    "Organization",
		"企业 ID": "Organization ID",
		"用户":    "User",
		"有效期":   "Expires",
		"Token 将自动刷新，无需重复登录": "The token will refresh automatically; no need to log in again",
		"已过期":  "Expired",
		"1 天后":  "in 1 day",
		"1 小时后": "in 1 hour",
		"授权成功": "Authorization successful",
		"请返回终端继续操作。此页面可以关闭。": "Return to the terminal to continue. You may close this page.",
		"钉钉 CLI": "DingTalk CLI",
	}
	for key, want := range english {
		if got := T(key); got != want {
			t.Errorf("T(%q) = %q, want %q", key, got, want)
		}
	}
	if got := Tf("%.0f 天后", 30.0); got != "in 30 days" {
		t.Errorf("English day expiry = %q", got)
	}
	if got := Tf("%.0f 小时后", 2.0); got != "in 2 hours" {
		t.Errorf("English hour expiry = %q", got)
	}

	SetLang("zh")
	if got := T("登录成功！"); got != "登录成功！" {
		t.Errorf("Chinese login success = %q", got)
	}
	if got := Tf("%.0f 天后", 30.0); got != "30 天后" {
		t.Errorf("Chinese day expiry = %q", got)
	}
}
