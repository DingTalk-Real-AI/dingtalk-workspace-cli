package helpers

import (
	"reflect"
	"strings"
	"testing"
)

func TestCrossPlatformCoverageDriveStarCommands(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want guardedMutationCall
	}{
		{
			name: "add",
			args: []string{"star", "add", "--node", "node-1"},
			want: guardedMutationCall{
				productID: "drive",
				toolName:  "mark_star",
				args:      map[string]any{"nodeId": "node-1"},
			},
		},
		{
			name: "add with url value passed through",
			args: []string{"star", "add", "--node", "https://alidocs.dingtalk.com/i/nodes/node-2"},
			want: guardedMutationCall{
				productID: "drive",
				toolName:  "mark_star",
				args:      map[string]any{"nodeId": "https://alidocs.dingtalk.com/i/nodes/node-2"},
			},
		},
		{
			name: "remove",
			args: []string{"star", "remove", "--node", "node-3"},
			want: guardedMutationCall{
				productID: "drive",
				toolName:  "unmark_star",
				args:      map[string]any{"nodeId": "node-3"},
			},
		},
		{
			name: "remove rm alias",
			args: []string{"star", "rm", "--node", "node-4"},
			want: guardedMutationCall{
				productID: "drive",
				toolName:  "unmark_star",
				args:      map[string]any{"nodeId": "node-4"},
			},
		},
		{
			name: "list without filters",
			args: []string{"star", "list"},
			want: guardedMutationCall{
				productID: "drive",
				toolName:  "get_star_list",
				args:      map[string]any{},
			},
		},
		{
			name: "list with all filters",
			args: []string{
				"star", "list",
				"--limit", "10",
				"--cursor", "cur-1",
				"--order-by", "createTime",
				"--sort", "desc",
				"--resource-types", "DENTRY,WORKSPACE",
				"--content-types", "doc,sheet",
			},
			want: guardedMutationCall{
				productID: "drive",
				toolName:  "get_star_list",
				args: map[string]any{
					"limit":                10,
					"cursor":               "cur-1",
					"orderBy":              "createTime",
					"sortType":             "desc",
					"supportResourceTypes": []string{"DENTRY", "WORKSPACE"},
					"contentTypes":         []string{"doc", "sheet"},
				},
			},
		},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			caller := &guardedMutationCaller{}
			err := executeGuardedMutationCommand(t, caller, newDriveCommand, test.args...)
			if err != nil {
				t.Fatalf("drive %s returned error: %v", strings.Join(test.args, " "), err)
			}
			if len(caller.calls) != 1 || !reflect.DeepEqual(caller.calls[0], test.want) {
				t.Fatalf("tool calls = %#v, want %#v", caller.calls, test.want)
			}
		})
	}
}

func TestCrossPlatformCoverageDriveStarRequiredFlags(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		wantErr string
	}{
		{
			name:    "add without node",
			args:    []string{"star", "add"},
			wantErr: "--node",
		},
		{
			name:    "remove without node",
			args:    []string{"star", "remove"},
			wantErr: "--node",
		},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			caller := &guardedMutationCaller{}
			err := executeGuardedMutationCommand(t, caller, newDriveCommand, test.args...)
			if err == nil || !strings.Contains(err.Error(), test.wantErr) {
				t.Fatalf("err = %v, want message containing %q", err, test.wantErr)
			}
			if len(caller.calls) != 0 {
				t.Fatalf("tool calls = %#v, want none", caller.calls)
			}
		})
	}
}

func TestCrossPlatformCoverageDriveStarListRecordsRawArgs(t *testing.T) {
	caller := &depthArgsRecordingCaller{steps: []scriptedToolStep{
		{text: `{"items":[{"nodeId":"n1","name":"doc1"}],"hasMore":false}`},
	}}
	err := executeDriveCommand(t, caller,
		"star", "list", "--limit", "5", "--cursor", "cur-9", "--sort", "asc")
	if err != nil {
		t.Fatal(err)
	}
	if len(caller.calls) != 1 {
		t.Fatalf("calls = %#v, want 1", caller.calls)
	}
	args := caller.calls[0]
	if args["limit"] != 5 || args["cursor"] != "cur-9" || args["sortType"] != "asc" {
		t.Fatalf("star list raw args = %#v", args)
	}
	if _, ok := args["orderBy"]; ok {
		t.Fatalf("unexpected orderBy in %#v", args)
	}
}

func TestCrossPlatformCoverageDriveStarAddScriptedSuccess(t *testing.T) {
	caller := &scriptedToolCaller{format: "json", steps: []scriptedToolStep{
		{text: `{"success":true}`},
	}}
	installScriptedCaller(t, caller)
	root := newDriveCommand()
	root.SilenceErrors = true
	root.SilenceUsage = true
	root.SetArgs([]string{"star", "add", "--node", "node-1"})
	if err := root.Execute(); err != nil {
		t.Fatalf("star add returned error: %v", err)
	}
	if caller.calls != 1 {
		t.Fatalf("tool calls = %d, want 1", caller.calls)
	}
}
