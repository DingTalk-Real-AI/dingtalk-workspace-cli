package cobra

import (
	"errors"
	"io"
	"reflect"
	"testing"
)

// Exercise the dependency boundary without DWS adapters. A nil error-handler
// result must still stop traversal on the original parser error.
func TestDWSTraverseFlagErrorHandler(t *testing.T) {
	for _, direct := range []bool{false, true} {
		for _, fallback := range []bool{false, true} {
			t.Run(fmtBool(direct)+"/"+fmtBool(fallback), func(t *testing.T) {
				root := &Command{Use: "root", TraverseChildren: true, SilenceErrors: true, SilenceUsage: true}
				root.SetOut(io.Discard)
				root.SetErr(io.Discard)
				group := &Command{Use: "group"}
				group.Flags().Int("count", 0, "")
				business, calls := 0, 0
				group.AddCommand(&Command{Use: "leaf", Run: func(*Command, []string) { business++ }})
				root.AddCommand(group)
				var parserErr error
				handled := errors.New("classified")
				root.SetFlagErrorFunc(func(cmd *Command, err error) error {
					calls++
					if cmd != group {
						t.Fatalf("parsing node = %v, want group", cmd)
					}
					parserErr = err
					if fallback {
						return nil
					}
					return handled
				})
				args := []string{"group", "--count=bad", "leaf"}
				var err error
				if direct {
					var cmd *Command
					var remaining []string
					cmd, remaining, err = root.Traverse(args)
					if cmd != nil || !reflect.DeepEqual(remaining, args[1:]) {
						t.Fatalf("traversal result = %v, %v", cmd, remaining)
					}
				} else {
					root.SetArgs(args)
					_, err = root.ExecuteC()
				}
				want := handled
				if fallback {
					want = parserErr
				}
				if err == nil || err != want || calls != 1 || business != 0 {
					t.Fatalf("err=%v want=%v handler=%d business=%d", err, want, calls, business)
				}
			})
		}
	}
}

func fmtBool(value bool) string {
	if value {
		return "true"
	}
	return "false"
}
