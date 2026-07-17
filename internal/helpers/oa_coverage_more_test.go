package helpers

import "testing"

func TestCrossPlatformCoverageOARemainingTimeAndRevertBranches(t *testing.T) {
	installScriptedCaller(t, &scriptedToolCaller{dry: true})
	for _, args := range [][]string{
		{"approval", "list-pending", "--start", "bad", "--end", "2030-01-01T10:00:00+08:00"},
		{"approval", "list-pending", "--start", "2030-01-01T09:00:00+08:00", "--end", "bad"},
		{"approval", "list-pending", "--start", "2030-01-01T10:00:00+08:00", "--end", "2030-01-01T09:00:00+08:00"},
		{"approval", "list-initiated", "--process-code", "code", "--start", "bad", "--end", "2030-01-01T10:00:00+08:00"},
		{"approval", "list-initiated", "--process-code", "code", "--start", "2030-01-01T09:00:00+08:00", "--end", "bad"},
		{"approval", "list-initiated", "--process-code", "code", "--start", "2030-01-01T10:00:00+08:00", "--end", "2030-01-01T09:00:00+08:00"},
	} {
		if err := executeFilterCoverage(t, newOaCommand(), args...); err == nil {
			t.Fatalf("args=%v returned nil", args)
		}
	}

	if err := executeFilterCoverage(t, newOaCommand(),
		"approval", "list-pending",
		"--start", "2030-01-01T09:00:00+08:00", "--end", "2030-01-01T10:00:00+08:00",
		"--page", "2", "--size", "20", "--query", "travel",
	); err != nil {
		t.Fatalf("pending options: %v", err)
	}
	if err := executeFilterCoverage(t, newOaCommand(),
		"approval", "revert-task", "--instance-id", "instance", "--task-id", "12",
		"--target-activity-id", "activity", "--action", "REVERT_FOR_APPROVAL", "--remark", "retry",
	); err != nil {
		t.Fatalf("revert task: %v", err)
	}
}
