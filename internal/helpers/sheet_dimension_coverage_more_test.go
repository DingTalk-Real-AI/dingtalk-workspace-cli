package helpers

import (
	"io"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func dimensionCoverageCommand(t *testing.T, name string) *cobra.Command {
	t.Helper()
	for _, cmd := range newDimensionCmds() {
		if cmd.Name() == name {
			cmd.SilenceErrors = true
			cmd.SilenceUsage = true
			cmd.SetOut(io.Discard)
			cmd.SetErr(io.Discard)
			return cmd
		}
	}
	t.Fatalf("dimension command %q not found", name)
	return nil
}

func executeDimensionCoverage(t *testing.T, name string, args ...string) error {
	t.Helper()
	cmd := dimensionCoverageCommand(t, name)
	cmd.SetArgs(args)
	return cmd.Execute()
}

func TestCrossPlatformCoverageDimensionValidationRemainingCoverage(t *testing.T) {
	installScriptedCaller(t, &scriptedToolCaller{dry: true})
	common := []string{"--node", "node", "--sheet-id", "sheet"}
	if err := executeDimensionCoverage(t, "insert-dimension", append(common, "--dimension", "ROWS", "--position", "1", "--length", "5001")...); err == nil {
		t.Fatal("oversized insert returned nil")
	}
	for _, tc := range []struct {
		dimension string
		extra     []string
		wantErr   bool
	}{
		{"ROW", []string{"--start-index", "1", "--end-index", "2", "--destination-index", "3"}, false},
		{"COLUMN", []string{"--start-index", "A", "--end-index", "B", "--destination-index", "C"}, false},
		{"ROWS", []string{"--end-index", "2", "--destination-index", "3"}, true},
		{"ROWS", []string{"--start-index", "1", "--destination-index", "3"}, true},
		{"ROWS", []string{"--start-index", "1", "--end-index", "2"}, true},
	} {
		args := append(append([]string{}, common...), "--dimension", tc.dimension)
		args = append(args, tc.extra...)
		err := executeDimensionCoverage(t, "move-dimension", args...)
		if (err != nil) != tc.wantErr {
			t.Errorf("move %s %v error=%v", tc.dimension, tc.extra, err)
		}
	}

	add := dimensionCoverageCommand(t, "add-dimension")
	_ = add.Flags().Set("dimension", "ROWS")
	add.Flags().Lookup("length").Value = invalidIntFlagValue{}
	if err := add.RunE(add, nil); err == nil {
		t.Fatal("invalid add length returned nil")
	}
	if err := executeDimensionCoverage(t, "add-dimension", append(common, "--dimension", "ROWS", "--length", "5001")...); err == nil {
		t.Fatal("oversized add returned nil")
	}
	if err := executeDimensionCoverage(t, "delete-dimension", append(common, "--dimension", "ROWS", "--position", "1", "--length", "5001")...); err == nil {
		t.Fatal("oversized delete returned nil")
	}
	if err := executeDimensionCoverage(t, "update-dimension", append(common, "--dimension", "ROWS", "--start-index", "1", "--length", "5001", "--hidden")...); err == nil {
		t.Fatal("oversized update returned nil")
	}
	if err := executeDimensionCoverage(t, "update-dimension", append(common, "--dimension", "ROWS", "--start-index", "1", "--length", "1")...); err == nil {
		t.Fatal("update without property returned nil")
	}
}

// --size-type 的枚举按维度区分：行高有 auto，列宽只有 pixel / standard（对齐飞书）。
func TestUpdateDimensionSizeTypeEnumIsDimensionScoped(t *testing.T) {
	installScriptedCaller(t, &scriptedToolCaller{dry: true})
	common := []string{"--node", "node", "--sheet-id", "sheet", "--start-index", "1", "--length", "2"}

	for _, tc := range []struct {
		name      string
		dimension string
		sizeType  string
		wantErr   string
	}{
		// standard / auto 由服务端决定尺寸，不再要求 --hidden 或 --pixel-size
		{"rows-standard", "ROWS", "standard", ""},
		{"rows-auto", "ROWS", "auto", ""},
		{"columns-standard", "COLUMNS", "standard", ""},
		{"columns-auto-rejected", "COLUMNS", "auto", "仅支持 pixel / standard"},
		// 非法枚举的提示文案必须按维度给出不同的合法值列表
		{"rows-bogus-hint", "ROWS", "bogus", "必须为 pixel / standard / auto"},
		{"columns-bogus-hint", "COLUMNS", "bogus", "必须为 pixel / standard"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			args := append(append([]string{}, common...), "--dimension", tc.dimension, "--size-type", tc.sizeType)
			err := executeDimensionCoverage(t, "update-dimension", args...)
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("err = %v, want nil", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("err = %v, want contains %q", err, tc.wantErr)
			}
		})
	}

	// pixel 仍需 --hidden 或 --pixel-size 之一
	args := append(append([]string{}, common...), "--dimension", "ROWS", "--size-type", "pixel")
	if err := executeDimensionCoverage(t, "update-dimension", args...); err == nil {
		t.Fatal("size-type=pixel 且未给 --pixel-size/--hidden 时应报错")
	}
}

func TestSizeTypeEnumHint(t *testing.T) {
	if got := sizeTypeEnumHint("ROWS"); got != "pixel / standard / auto" {
		t.Fatalf("ROWS hint = %q", got)
	}
	if got := sizeTypeEnumHint("COLUMNS"); got != "pixel / standard" {
		t.Fatalf("COLUMNS hint = %q", got)
	}
}

func TestCrossPlatformCoverageDropdownValidationRemainingCoverage(t *testing.T) {
	installScriptedCaller(t, &scriptedToolCaller{dry: true})
	base := []string{"--node", "node", "--sheet-id", "sheet", "--range", "A1"}
	for _, options := range []string{"[]", `[{"value":"a,b"}]`} {
		if err := executeDimensionCoverage(t, "set-dropdown", append(base, "--options", options)...); err == nil {
			t.Fatalf("options %s returned nil", options)
		}
	}
	if err := executeDimensionCoverage(t, "set-dropdown", append(base, "--options", `[{"value":"a"}]`, "--multi-select")...); err != nil {
		t.Fatalf("multi-select dropdown: %v", err)
	}
}

// --size-type standard/auto 与 --pixel-size 语义冲突（一个说交给服务端、一个
// 指定固定像素），必须在发请求前拒；反向 pixel 模式也必须带像素值。
func TestUpdateDimensionRejectsSizeTypeAndPixelSizeConflict(t *testing.T) {
	for _, tc := range []struct {
		name  string
		flags [][2]string
		want  string
	}{
		{
			"standard-with-pixel-size",
			[][2]string{{"dimension", "ROWS"}, {"size-type", "standard"}, {"pixel-size", "40"}},
			"不能同时指定 --pixel-size",
		},
		{
			"auto-with-pixel-size",
			[][2]string{{"dimension", "ROWS"}, {"size-type", "auto"}, {"pixel-size", "40"}},
			"不能同时指定 --pixel-size",
		},
		{
			"explicit-pixel-without-size",
			[][2]string{{"dimension", "ROWS"}, {"size-type", "pixel"}, {"hidden", "true"}},
			"--size-type pixel 必须配合 --pixel-size",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			caller := &scriptedToolCaller{}
			flags := append([][2]string{{"node", "N"}, {"sheet-id", "S"}, {"start-index", "1"}, {"length", "2"}}, tc.flags...)
			err := runUpdateDimensionForTest(t, caller, flags)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v, want contains %q", err, tc.want)
			}
			if caller.calls != 0 {
				t.Fatalf("calls = %d, want 0（冲突必须在发请求前拒）", caller.calls)
			}
		})
	}

	// 合法组合仍然放行
	for _, tc := range []struct {
		name  string
		flags [][2]string
	}{
		{"standard-alone", [][2]string{{"dimension", "ROWS"}, {"size-type", "standard"}}},
		{"auto-alone", [][2]string{{"dimension", "ROWS"}, {"size-type", "auto"}}},
		{"pixel-with-size", [][2]string{{"dimension", "ROWS"}, {"size-type", "pixel"}, {"pixel-size", "40"}}},
		{"pixel-size-without-size-type", [][2]string{{"dimension", "COLUMNS"}, {"pixel-size", "120"}}},
		{"hidden-only", [][2]string{{"dimension", "ROWS"}, {"hidden", "true"}}},
	} {
		t.Run("ok/"+tc.name, func(t *testing.T) {
			caller := &scriptedToolCaller{steps: []scriptedToolStep{{text: `{"success":true}`}}}
			flags := append([][2]string{{"node", "N"}, {"sheet-id", "S"}, {"start-index", "1"}, {"length", "2"}}, tc.flags...)
			if err := runUpdateDimensionForTest(t, caller, flags); err != nil {
				t.Fatalf("合法组合被拒: %v", err)
			}
		})
	}
}

func runUpdateDimensionForTest(t *testing.T, caller *scriptedToolCaller, flags [][2]string) error {
	t.Helper()
	installScriptedCaller(t, caller)
	installSheetProductArgs(t)
	for _, c := range newDimensionCmds() {
		if c.Name() != "update-dimension" {
			continue
		}
		for _, kv := range flags {
			if err := c.Flags().Set(kv[0], kv[1]); err != nil {
				t.Fatalf("set --%s: %v", kv[0], err)
			}
		}
		return c.RunE(c, nil)
	}
	t.Fatal("update-dimension not found")
	return nil
}
