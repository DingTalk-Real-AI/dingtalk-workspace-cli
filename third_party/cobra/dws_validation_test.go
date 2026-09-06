package cobra

import (
	"errors"
	"io"
	"testing"
)

func TestDWSNativeValidationErrorHandler(t *testing.T) {
	for _, stage := range []ValidationStage{ValidationStageArgs, ValidationStageRequiredFlags, ValidationStageFlagGroups} {
		for _, fallback := range []bool{false, true} {
			t.Run(string(stage)+"/nil="+fmtBool(fallback), func(t *testing.T) {
				root := &Command{Use: "root", SilenceErrors: true, SilenceUsage: true}
				root.SetOut(io.Discard)
				root.SetErr(io.Discard)
				pre, runs, calls, ancestor := 0, 0, 0, 0
				group := &Command{Use: "group"}
				leaf := &Command{Use: "leaf", PreRun: func(*Command, []string) { pre++ }, Run: func(*Command, []string) { runs++ }}
				root.AddCommand(group)
				group.AddCommand(leaf)
				switch stage {
				case ValidationStageArgs:
					leaf.Args = func(*Command, []string) error { return errors.New("bad arguments") }
				case ValidationStageRequiredFlags:
					leaf.Flags().String("name", "", "")
					if err := leaf.MarkFlagRequired("name"); err != nil {
						t.Fatal(err)
					}
				case ValidationStageFlagGroups:
					leaf.Flags().String("left", "", "")
					leaf.Flags().String("right", "", "")
					leaf.MarkFlagsOneRequired("left", "right")
				}
				root.SetValidationErrorFunc(func(*Command, ValidationStage, error) error { ancestor++; return errors.New("wrong handler") })
				replacement := errors.New("classified")
				var original error
				group.SetValidationErrorFunc(func(cmd *Command, got ValidationStage, err error) error {
					calls++
					original = err
					if cmd != leaf || got != stage {
						t.Fatalf("handler owner/stage = %v/%s", cmd, got)
					}
					if fallback {
						return nil
					}
					return replacement
				})
				root.SetArgs([]string{"group", "leaf"})
				selected, err := root.ExecuteC()
				want := replacement
				if fallback {
					want = original
				}
				if selected != leaf || err == nil || err != want || calls != 1 || ancestor != 0 || runs != 0 {
					t.Fatalf("selected=%v err=%v calls=%d ancestor=%d runs=%d", selected, err, calls, ancestor, runs)
				}
				wantPre := 1
				if stage == ValidationStageArgs {
					wantPre = 0
				}
				if pre != wantPre {
					t.Fatalf("pre=%d want=%d", pre, wantPre)
				}
			})
		}
	}
}

func TestDWSValidationHandlerExcludesBusinessHooks(t *testing.T) {
	for _, phase := range []string{"success", "persistent-pre", "pre", "run", "post", "persistent-post"} {
		t.Run(phase, func(t *testing.T) {
			root := &Command{Use: "root", SilenceErrors: true, SilenceUsage: true}
			root.SetOut(io.Discard)
			root.SetErr(io.Discard)
			business := errors.New("business failure")
			fail := func(*Command, []string) error { return business }
			leaf := &Command{Use: "leaf", RunE: func(*Command, []string) error { return nil }}
			root.AddCommand(leaf)
			switch phase {
			case "persistent-pre":
				root.PersistentPreRunE = fail
			case "pre":
				leaf.PreRunE = fail
			case "run":
				leaf.RunE = fail
			case "post":
				leaf.PostRunE = fail
			case "persistent-post":
				root.PersistentPostRunE = fail
			}
			root.SetValidationErrorFunc(func(*Command, ValidationStage, error) error {
				t.Fatal("business/success path called validation adapter")
				return nil
			})
			root.SetArgs([]string{"leaf"})
			_, err := root.ExecuteC()
			if phase == "success" {
				if err != nil {
					t.Fatal(err)
				}
			} else if err != business {
				t.Fatalf("business error identity = %v", err)
			}
		})
	}
}

func TestDWSLegacyArgsValidationHandler(t *testing.T) {
	for _, fallback := range []bool{false, true} {
		t.Run(fmtBool(fallback), func(t *testing.T) {
			root := &Command{Use: "root", SilenceErrors: true, SilenceUsage: true}
			root.AddCommand(&Command{Use: "leaf", Run: func(*Command, []string) { t.Fatal("unknown command executed") }})
			calls := 0
			var original error
			classified := errors.New("classified legacy Args")
			root.SetValidationErrorFunc(func(cmd *Command, stage ValidationStage, err error) error {
				calls++
				original = err
				if cmd != root || stage != ValidationStageArgs {
					t.Fatalf("owner/stage=%v/%s", cmd, stage)
				}
				if fallback {
					return nil
				}
				return classified
			})
			root.SetArgs([]string{"unknown"})
			cmd, err := root.ExecuteC()
			want := classified
			if fallback {
				want = original
			}
			if cmd != root || err == nil || err != want || calls != 1 {
				t.Fatalf("cmd=%v err=%v calls=%d", cmd, err, calls)
			}
		})
	}
}
