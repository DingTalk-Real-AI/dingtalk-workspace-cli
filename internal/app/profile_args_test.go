package app

import (
	"os"
	"reflect"
	"testing"
)

func TestNormalizeProfileFlagArgsAcceptsUnquotedCommaContinuation(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want []string
	}{
		{
			name: "root profile before command",
			args: []string{"--mock", "--profile", "corpA,", "corpB", "contact", "user", "get-self"},
			want: []string{"--mock", "--profile", "corpA,corpB", "contact", "user", "get-self"},
		},
		{
			name: "profile after leaf command",
			args: []string{"contact", "user", "get-self", "--profile", "corpA,", "corpB", "--format", "json"},
			want: []string{"contact", "user", "get-self", "--profile", "corpA,corpB", "--format", "json"},
		},
		{
			name: "equals form",
			args: []string{"--profile=corpA,", "corpB", "contact", "user", "get-self"},
			want: []string{"--profile=corpA,corpB", "contact", "user", "get-self"},
		},
		{
			name: "three profiles",
			args: []string{"--profile", "corpA,", "corpB,", "corpC", "contact", "user", "get-self"},
			want: []string{"--profile", "corpA,corpB,corpC", "contact", "user", "get-self"},
		},
		{
			name: "already quoted by shell remains unchanged",
			args: []string{"--profile", "corpA, corpB", "contact", "user", "get-self"},
			want: []string{"--profile", "corpA, corpB", "contact", "user", "get-self"},
		},
		{
			name: "single profile remains unchanged",
			args: []string{"--profile", "corpA", "contact", "user", "get-self"},
			want: []string{"--profile", "corpA", "contact", "user", "get-self"},
		},
		{
			name: "trailing comma before next flag remains validation input",
			args: []string{"--profile", "corpA,", "--format", "json", "contact", "user", "get-self"},
			want: []string{"--profile", "corpA,", "--format", "json", "contact", "user", "get-self"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, _ := normalizeProfileFlagArgs(tc.args)
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("normalizeProfileFlagArgs() = %#v, want %#v", got, tc.want)
			}
		})
	}
}

func TestPreparseProfileFlagUsesNormalizedProfileArgs(t *testing.T) {
	got := preparseProfileFlag([]string{"--profile", "corpA,", "corpB", "contact", "user", "get-self"})
	if got != "corpA,corpB" {
		t.Fatalf("preparseProfileFlag() = %q, want corpA,corpB", got)
	}
}

func TestNormalizeProcessProfileArgsRestoresOriginalArgv(t *testing.T) {
	oldArgs := os.Args
	t.Cleanup(func() { os.Args = oldArgs })

	os.Args = []string{"dws", "--profile", "corpA,", "corpB", "contact", "user", "get-self"}
	restore := normalizeProcessProfileArgs()
	if want := []string{"dws", "--profile", "corpA,corpB", "contact", "user", "get-self"}; !reflect.DeepEqual(os.Args, want) {
		t.Fatalf("os.Args after normalize = %#v, want %#v", os.Args, want)
	}
	restore()
	if want := []string{"dws", "--profile", "corpA,", "corpB", "contact", "user", "get-self"}; !reflect.DeepEqual(os.Args, want) {
		t.Fatalf("os.Args after restore = %#v, want %#v", os.Args, want)
	}
}
