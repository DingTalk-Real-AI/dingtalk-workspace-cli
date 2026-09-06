package roothelp

import (
	"encoding/base64"
	"reflect"
	"strings"
	"testing"
)

func TestCrossPlatformCoverageHelpSnapshotBindingAndMalformedInputs(t *testing.T) {
	model := Model{Services: []Command{{Name: "calendar", Short: "calendar"}}, Utilities: []Command{{Name: "help", Short: "help"}}, Flags: []Flag{{Label: "--format string", Usage: "format"}}, Long: "guidance"}
	s := Snapshot{Version: 1, Edition: "open", Commit: strings.Repeat("a", 40), CoreSHA256: strings.Repeat("b", 64), English: model, Chinese: model}
	s.Chinese.Long = "中文提示"
	encoded, err := EncodeSnapshot(s)
	if err != nil {
		t.Fatal(err)
	}
	for _, locale := range []string{"en", "zh"} {
		got, err := DecodeSnapshot(encoded, s.CoreSHA256, s.Commit, s.Edition, locale)
		want := s.English
		if locale == "zh" {
			want = s.Chinese
		}
		if err != nil || !reflect.DeepEqual(got, want) {
			t.Fatalf("%s round trip: %v", locale, err)
		}
	}
	data, _ := base64.RawStdEncoding.DecodeString(encoded)
	for _, tc := range []struct{ name, value, core, commit, edition, locale string }{
		{"different core", encoded, strings.Repeat("c", 64), s.Commit, "open", "en"},
		{"different commit", encoded, s.CoreSHA256, strings.Repeat("c", 40), "open", "en"},
		{"different edition", encoded, s.CoreSHA256, s.Commit, "private", "en"},
		{"unsupported locale", encoded, s.CoreSHA256, s.Commit, "open", "ja"},
		{"base64 newline", encoded + "\n", s.CoreSHA256, s.Commit, "open", "en"},
		{"duplicate identity field", base64.RawStdEncoding.EncodeToString([]byte(strings.Replace(string(data), `{"Version":1,`, `{"Version":9,"Version":1,`, 1))), s.CoreSHA256, s.Commit, "open", "en"},
		{"trailing value", base64.RawStdEncoding.EncodeToString(append(data, []byte("{}")...)), s.CoreSHA256, s.Commit, "open", "en"},
		{"oversized", strings.Repeat("A", base64.RawStdEncoding.EncodedLen(MaxSnapshotBytes)+1), s.CoreSHA256, s.Commit, "open", "en"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := DecodeSnapshot(tc.value, tc.core, tc.commit, tc.edition, tc.locale); err == nil {
				t.Fatal("accepted invalid help snapshot")
			}
		})
	}
	s.English = Model{}
	if _, err := EncodeSnapshot(s); err == nil {
		t.Fatal("accepted incomplete English help")
	}
}
