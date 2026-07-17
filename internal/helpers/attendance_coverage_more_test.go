package helpers

import (
	"encoding/json"
	"io"
	"testing"

	"github.com/spf13/cobra"
)

func TestCrossPlatformCoverageAttendanceResponseAndDateCoverage(t *testing.T) {
	previous := deps
	InitDeps(&productExampleCaller{})
	deps.Out.w = io.Discard
	deps.Out.errW = io.Discard
	t.Cleanup(func() { deps = previous })

	printGroupModifyDeeplink(nil)
	for _, value := range []map[string]any{
		{},
		{"corpId": "corp"},
		{"corpId": "corp", "groupId": 1},
		{"result": map[string]any{"corpId": "corp", "id": json.Number("2")}},
		{"result": map[string]any{"groupSettingVO": map[string]any{"corpId": "corp", "groupId": float64(3)}}},
		{"result": map[string]any{"groupVO": map[string]any{"corpId": "corp", "groupId": int64(4)}}},
	} {
		printGroupModifyDeeplink(value)
		_ = pickStringField(value, "corpId")
		_ = pickInt64StringField(value, "missing", "groupId", "id")
	}
	_ = jsonStringToMap(`{"ok":true}`)
	_ = jsonStringToMap(`not-json`)
	_ = parseUserList(" one, ,two ")
	for _, tc := range []struct{ value, name string }{
		{"2026-01-02 03:04:05", "begin"}, {"2026-01-02", "begin"},
		{"2026-01-02", "endTime"}, {"bad", "end"},
	} {
		_, _ = parseDateToTimestamp(tc.value, tc.name)
	}
	for _, scene := range []string{"checkRemind", "fastCheck", "checkResultNotify", "lackRemind", "personalAttendStatNotify", "bossAttendStatNotify", "bad"} {
		_ = isAttendanceSettingSceneAllowed(scene)
	}
}

func TestCrossPlatformCoverageAttendanceFlagFallbackCoverage(t *testing.T) {
	newCommand := func() *cobra.Command {
		cmd := &cobra.Command{Use: "flags"}
		cmd.Flags().Int64("primary64", 7, "")
		cmd.Flags().Int64("alias64", 8, "")
		cmd.Flags().Int("primary", 7, "")
		cmd.Flags().Int("alias", 8, "")
		cmd.Flags().Bool("primary-bool", false, "")
		cmd.Flags().Bool("alias-bool", false, "")
		return cmd
	}
	for _, configure := range []func(*cobra.Command){
		func(*cobra.Command) {},
		func(c *cobra.Command) {
			_ = c.Flags().Set("primary64", "9")
			_ = c.Flags().Set("primary", "9")
			_ = c.Flags().Set("primary-bool", "true")
		},
		func(c *cobra.Command) {
			_ = c.Flags().Set("primary64", "0")
			_ = c.Flags().Set("primary", "0")
			_ = c.Flags().Set("primary-bool", "false")
		},
		func(c *cobra.Command) {
			_ = c.Flags().Set("alias64", "10")
			_ = c.Flags().Set("alias", "10")
			_ = c.Flags().Set("alias-bool", "true")
		},
		func(c *cobra.Command) {
			_ = c.Flags().Set("alias64", "0")
			_ = c.Flags().Set("alias", "0")
			_ = c.Flags().Set("alias-bool", "false")
		},
	} {
		cmd := newCommand()
		configure(cmd)
		_ = int64FlagOrFallback(cmd, "primary64", "alias64")
		_ = intFlagOrFallback(cmd, "primary", "alias")
		_ = boolFlagOrFallback(cmd, "primary-bool", "alias-bool")
	}
}

func TestCrossPlatformCoverageConvertClassCheckTimeCoverage(t *testing.T) {
	class := map[string]any{
		"sections": []any{
			"bad",
			map[string]any{"times": "bad"},
			map[string]any{"times": []any{
				"bad", map[string]any{}, map[string]any{"checkTime": nil},
				map[string]any{"checkTime": "08:00"}, map[string]any{"checkTime": "bad"},
				map[string]any{"checkTime": float64(1)}, map[string]any{"checkTime": true},
			}},
		},
		"setting": map[string]any{"topRestTimeList": []any{
			"bad", map[string]any{"checkTime": "09:00"},
		}},
	}
	convertClassCheckTime(class)
	convertClassCheckTime(map[string]any{"sections": "bad", "setting": "bad"})
	convertClassCheckTime(map[string]any{"setting": map[string]any{"topRestTimeList": "bad"}})
}

func validAttendanceFlagValue(valueType string) string {
	switch valueType {
	case selfSettingSaveBool:
		return "true"
	case selfSettingSaveInt:
		return "1"
	default:
		return `{"enabled":true}`
	}
}

func TestCrossPlatformCoverageSelfSettingSaveCoverage(t *testing.T) {
	for _, spec := range selfSettingSaveFlagSpecs {
		cmd := &cobra.Command{Use: "self"}
		registerSelfSettingSaveFlags(cmd)
		_ = cmd.Flags().Set(spec.flagName, validAttendanceFlagValue(spec.valueType))
		if _, err := collectSelfSettingSaveRequest(cmd, spec.scenes[0], "user"); err != nil {
			t.Fatalf("%s: %v", spec.flagName, err)
		}
		if selfSettingFlagSupportsScene(spec, "bad") {
			t.Fatalf("%s unexpectedly supports bad scene", spec.flagName)
		}
		_, _ = collectSelfSettingSaveRequest(cmd, "bad", "user")
	}
	empty := &cobra.Command{Use: "self"}
	registerSelfSettingSaveFlags(empty)
	_, _ = collectSelfSettingSaveRequest(empty, "checkRemind", "user")
	jsonCommand := &cobra.Command{Use: "self"}
	registerSelfSettingSaveFlags(jsonCommand)
	_ = jsonCommand.Flags().Set("check-remind-setting", "")
	_, _ = collectSelfSettingSaveRequest(jsonCommand, "checkRemind", "user")
	_ = jsonCommand.Flags().Set("check-remind-setting", "{")
	_, _ = collectSelfSettingSaveRequest(jsonCommand, "checkRemind", "user")
	_, _ = readSelfSettingSaveFlagValue(jsonCommand, selfSettingSaveFlagSpec{flagName: "check-remind-setting", valueType: "unsupported"})

	for _, scope := range []string{"", "企业", "全公司", "所有人", "invalid"} {
		cmd := &cobra.Command{Use: "scope"}
		cmd.Flags().String("scope", "", "")
		_ = cmd.Flags().Set("scope", scope)
		_ = validateGlobalSettingScope(cmd)
	}
}

func TestCrossPlatformCoverageGlobalSettingSaveCoverage(t *testing.T) {
	for _, spec := range globalSettingSaveFlagSpecs {
		cmd := &cobra.Command{Use: "global"}
		registerGlobalSettingSaveFlags(cmd)
		_ = cmd.Flags().Set(spec.flagName, validAttendanceFlagValue(spec.valueType))
		if _, err := collectGlobalSettingSaveRequest(cmd, spec.scenes[0]); err != nil {
			t.Fatalf("%s: %v", spec.flagName, err)
		}
		if globalSettingFlagSupportsScene(spec, "bad") {
			t.Fatalf("%s unexpectedly supports bad scene", spec.flagName)
		}
		_, _ = collectGlobalSettingSaveRequest(cmd, "bad")
	}
	empty := &cobra.Command{Use: "global"}
	registerGlobalSettingSaveFlags(empty)
	_, _ = collectGlobalSettingSaveRequest(empty, "checkRemind")
	_, _ = readGlobalSettingSaveFlagValue(empty, globalSettingSaveFlagSpec{flagName: "missing", valueType: "unsupported"})
}
