package helpers

import (
	"testing"
)

func TestCrossPlatformCoverageImageUploadMediaVersionKeepsEmotionCompatibility(t *testing.T) {
	for _, tc := range []struct {
		name      string
		payload   string
		requireV2 bool
		want      string
		wantError bool
	}{
		{"chat_both_versions", `{"success":true,"mediaIdV1":"@legacy","mediaIdV2":"$current"}`, true, "$current", false},
		{"emotion_both_versions", `{"success":true,"mediaIdV1":"@legacy","mediaIdV2":"$current"}`, false, "@legacy", false},
		{"chat_v2_only", `{"success":true,"mediaIdV2":" $current "}`, true, "$current", false},
		{"emotion_v2_only", `{"success":true,"mediaIdV2":"$current"}`, false, "$current", false},
		{"chat_v1_only", `{"success":true,"mediaIdV1":"@legacy"}`, true, "", true},
		{"emotion_v1_only", `{"success":true,"mediaIdV1":"@legacy"}`, false, "@legacy", false},
		{"chat_blank_v2", `{"success":true,"mediaIdV1":"@legacy","mediaIdV2":" "}`, true, "", true},
		{"chat_upload_failed", `{"success":false,"mediaIdV2":"$current"}`, true, "", true},
		{"chat_invalid_json", `not-json`, true, "", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			parse := parsePersonalEmotionUploadMediaID
			if tc.requireV2 {
				parse = parseChatImageUploadMediaID
			}
			got, err := parse(tc.payload)
			if (err != nil) != tc.wantError || got != tc.want {
				t.Fatalf("got=%q err=%v; want=%q wantError=%v", got, err, tc.want, tc.wantError)
			}
		})
	}
}

func TestCrossPlatformCoverageEmotionUploadKeepsLegacyMediaVersion(t *testing.T) {
	caller := &personalEmotionUploadCaller{
		uploadText: `{"success":true,"mediaIdV1":"@legacy","mediaIdV2":"$current"}`,
	}
	installHelpersCoreDeps(t, caller)
	got, err := uploadPersonalEmotionImage(t.Context(), &personalEmotionImage{
		imageType: "png",
		content:   "eA==",
	})
	if err != nil || got != "@legacy" {
		t.Fatalf("emotion upload got=%q err=%v; want legacy mediaId", got, err)
	}
	if caller.uploadCalls != 1 || len(caller.uploadArgs) != 1 || caller.uploadArgs[0]["bizType"] != "chat_emoticon" {
		t.Fatalf("emotion upload calls=%d args=%#v", caller.uploadCalls, caller.uploadArgs)
	}
}
