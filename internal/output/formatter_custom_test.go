package output

import (
	"github.com/spf13/cobra"
	"testing"
)

func TestResolveFieldsShadowing(t *testing.T) {
	rootCmd := &cobra.Command{Use: "dws"}
	var globalFields string
	// Register the global persistent flag.
	rootCmd.PersistentFlags().StringVar(&globalFields, "fields", "", "筛选输出字段 (逗号分隔, 如: name,id,status)")

	// 1. Normal command that relies on the global output filter
	normalCmd := &cobra.Command{Use: "normal"}
	rootCmd.AddCommand(normalCmd)
	rootCmd.SetArgs([]string{"normal", "--fields", "data,status"})
	rootCmd.Execute()

	if fields := ResolveFields(normalCmd); fields != "data,status" {
		t.Errorf("expected 'data,status' for normal cmd, got %q", fields)
	}

	// 2. Command that shadows the global format flag with its own local business logic
	bizCmd := &cobra.Command{Use: "biz"}
	var localFields string
	bizCmd.Flags().StringVar(&localFields, "fields", "", "JSON string array of objects")
	rootCmd.AddCommand(bizCmd)

	// Reset
	rootCmd.SetArgs([]string{"biz", "--fields", "[\"fake\"]"})
	rootCmd.Execute()

	// It should now correctly ignore the localized fields parameter!
	if fields := ResolveFields(bizCmd); fields != "" {
		t.Errorf("expected empty fields for shadowed cmd since it's a business param, got %q", fields)
	}
}
