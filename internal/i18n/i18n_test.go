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
