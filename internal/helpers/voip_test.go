package helpers

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"reflect"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
)

// runVoipCommandOutput 经准备后的命令树与统一渲染器返回实际 JSON 信封。
func runVoipCommandOutput(t *testing.T, args ...string) (map[string]any, error) {
	t.Helper()
	ctx, _ := output.WithResultStore(context.Background())
	root := newVoipCommand()
	root.SetContext(ctx)
	root.PersistentFlags().Bool("yes", false, "")
	root.PersistentFlags().Bool("dry-run", false, "")
	root.PersistentFlags().String("format", "json", "")
	var stdout strings.Builder
	root.SetOut(&stdout)
	root.SetErr(io.Discard)
	root.SetArgs(args)
	executed, err := corecmd.ExecuteCForTest(root)
	if err != nil {
		return nil, err
	}
	_, emitted, err := output.EmitStoredResult(executed)
	if err != nil {
		return nil, err
	}
	if !emitted {
		// 校验类失败（required flag）没有统一结果，直接返回错误即可。
		return nil, nil
	}
	var envelope map[string]any
	if err := json.Unmarshal([]byte(stdout.String()), &envelope); err != nil {
		t.Fatalf("解析统一结果失败：%v\n%s", err, stdout.String())
	}
	return envelope, nil
}

func TestVoipMinutesAutoStartRequiresEnableFlag(t *testing.T) {
	InitDepsForTest(t, &sportFakeCaller{blocks: 1, text: `{"success":true}`})
	_, err := runVoipCommandOutput(t, "minutes", "auto-start")
	if err == nil || !strings.Contains(err.Error(), "--enable") {
		t.Fatalf("期望 --enable 必填错误，实际: %v", err)
	}
}

func TestVoipMinutesAutoStartEnableSendsTrue(t *testing.T) {
	fake := &sportFakeCaller{blocks: 1, text: `{"success":true,"errorCode":"","errorMsg":""}`}
	InitDepsForTest(t, fake)
	envelope, err := runVoipCommandOutput(t, "--yes", "minutes", "auto-start", "--enable")
	if err != nil {
		t.Fatal(err)
	}
	if len(fake.calls) != 1 || fake.calls[0] != "conference/set_voip_minutes_auto_start" {
		t.Fatalf("应恰好调用一次 conference/set_voip_minutes_auto_start，实际: %v", fake.calls)
	}
	if len(fake.arguments) != 1 || !reflect.DeepEqual(fake.arguments[0], map[string]any{"autoStart": true}) {
		t.Fatalf("应下发 autoStart=true，实际: %v", fake.arguments)
	}
	want := map[string]any{"success": true, "errorCode": "", "errorMsg": ""}
	if envelope["outcome"] != "success" || !reflect.DeepEqual(envelope["data"], want) {
		t.Fatalf("统一结果不符：envelope=%v", envelope)
	}
}

func TestVoipMinutesAutoStartDisableSendsFalse(t *testing.T) {
	fake := &sportFakeCaller{blocks: 1, text: `{"success":true}`}
	InitDepsForTest(t, fake)
	if _, err := runVoipCommandOutput(t, "--yes", "minutes", "auto-start", "--enable=false"); err != nil {
		t.Fatal(err)
	}
	if len(fake.arguments) != 1 || !reflect.DeepEqual(fake.arguments[0], map[string]any{"autoStart": false}) {
		t.Fatalf("应下发 autoStart=false，实际: %v", fake.arguments)
	}
}

func TestVoipMinutesAutoStartDryRunSkipsToolCall(t *testing.T) {
	fake := &sportFakeCaller{dryRun: true}
	InitDepsForTest(t, fake)
	envelope, err := runVoipCommandOutput(t, "--dry-run", "minutes", "auto-start", "--enable")
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]any{"tool": "set_voip_minutes_auto_start", "arguments": map[string]any{"autoStart": true}, "executed": false}
	if envelope["dry_run"] != true || !reflect.DeepEqual(envelope["data"], want) || len(fake.calls) != 0 {
		t.Fatalf("dry-run 不应执行工具：envelope=%v calls=%v", envelope, fake.calls)
	}
}

func TestVoipMinutesAutoStartRequiresConfirmationWithoutYes(t *testing.T) {
	InitDepsForTest(t, &sportFakeCaller{blocks: 1, text: `{"success":true}`})
	_, err := runVoipCommandOutput(t, "minutes", "auto-start", "--enable")
	if err == nil || !strings.Contains(err.Error(), "需要用户确认") {
		t.Fatalf("写命令未带 --yes 应触发确认门，实际: %v", err)
	}
}

func TestVoipRootCommandHidden(t *testing.T) {
	root := newVoipCommand()
	if !root.Hidden {
		t.Fatal("voip 根命令应保持 Hidden（misc 长尾形态）")
	}
}

func TestVoipMinutesAutoStartRejectsUnconfirmedResult(t *testing.T) {
	for _, text := range []string{"", "{}", "null", "[]", `{"success":"true"}`, `{"success":false}`, `{"success":true} trailing`} {
		t.Run(text, func(t *testing.T) {
			fake := &sportFakeCaller{blocks: 1, text: text}
			InitDepsForTest(t, fake)
			_, err := runVoipCommandOutput(t, "--yes", "minutes", "auto-start", "--enable")
			if err == nil || len(fake.calls) != 1 {
				t.Fatalf("err=%v calls=%v", err, fake.calls)
			}
			if text == "" || text == "{}" || text == "null" || text == "[]" || text == `{"success":"true"}` {
				var typed *apperrors.Error
				if !errors.As(err, &typed) || typed.ExecutionStarted == nil || !*typed.ExecutionStarted || typed.Retryable {
					t.Fatalf("unknown write error=%#v", err)
				}
			}
		})
	}
}

func TestVoipMinutesAutoStartMissingCallerAndRemoteFailure(t *testing.T) {
	InitDepsForTest(t, nil)
	if _, err := voipMinutesAutoStartCall(nil, "set_voip_minutes_auto_start", nil); err == nil {
		t.Fatal("missing caller accepted")
	}
	fake := &sportFakeCaller{err: errors.New("remote failed")}
	InitDepsForTest(t, fake)
	if _, err := runVoipCommandOutput(t, "--yes", "minutes", "auto-start", "--enable"); err == nil {
		t.Fatal("remote error accepted")
	}
}
