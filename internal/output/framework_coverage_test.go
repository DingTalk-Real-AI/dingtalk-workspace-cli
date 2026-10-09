package output

import (
	"bytes"
	"context"
	"errors"
	"io"
	"reflect"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

type forgedResult struct {
	env  Envelope
	exit int
}

func (r forgedResult) Outcome() Outcome    { return r.env.Outcome }
func (r forgedResult) ExitCode() int       { return r.exit }
func (r forgedResult) envelope() *Envelope { copy := r.env; return &copy }

type cloneNode struct {
	Name   string
	Next   *cloneNode
	Values []string
	Hidden *string
}

func TestFrameworkResultCloneAndValidationEdges(t *testing.T) {
	if cloneReflectValue(reflect.Value{}, nil).IsValid() {
		t.Fatal("invalid reflect value became valid")
	}
	var nilAny any
	if got := cloneResultData(nilAny); got != nil {
		t.Fatalf("cloneResultData(nil)=%v", got)
	}

	node := &cloneNode{Name: "before", Values: []string{"one"}}
	node.Next = node
	payload := map[string]any{
		"node":      node,
		"nil_ptr":   (*cloneNode)(nil),
		"nil_map":   map[string]string(nil),
		"nil_slice": []string(nil),
		"array":     [2]string{"a", "b"},
		"interface": any([]string{"x"}),
	}
	cloned := cloneResultData(payload).(map[string]any)
	gotNode := cloned["node"].(*cloneNode)
	if gotNode == node || gotNode.Next != gotNode || gotNode.Values[0] != "one" {
		t.Fatalf("cycle-safe clone=%#v", gotNode)
	}
	node.Name, node.Values[0] = "after", "two"
	if gotNode.Name != "before" || gotNode.Values[0] != "one" {
		t.Fatalf("clone was mutated: %#v", gotNode)
	}

	started := true
	retry := int64(3)
	info := &ErrorInfo{
		Type: "api", UpstreamCode: map[string]any{"token": "kept"},
		Params: []string{"p"}, Actions: []string{"a"}, AvailableFlags: []string{"f"},
		ExecutionStarted: &started, RetryAfterSeconds: &retry,
		Details: map[string]any{"nested": []any{"x"}}, RPCData: []any{"rpc"},
	}
	copyInfo := cloneErrorInfo(info)
	if copyInfo == info || copyInfo.ExecutionStarted == info.ExecutionStarted || copyInfo.RetryAfterSeconds == info.RetryAfterSeconds {
		t.Fatal("ErrorInfo pointer fields were not detached")
	}
	if cloneErrorInfo(nil) != nil {
		t.Fatal("cloneErrorInfo(nil) must be nil")
	}
	cyclicMap := map[string]any{}
	cyclicMap["self"] = cyclicMap
	clonedMap := cloneResultData(cyclicMap).(map[string]any)
	if reflect.ValueOf(clonedMap).Pointer() == reflect.ValueOf(cyclicMap).Pointer() {
		t.Fatal("cyclic map was not cloned")
	}
	cyclicSlice := make([]any, 1)
	cyclicSlice[0] = cyclicSlice
	clonedSlice := cloneResultData(cyclicSlice).([]any)
	if reflect.ValueOf(clonedSlice).Pointer() == reflect.ValueOf(cyclicSlice).Pointer() {
		t.Fatal("cyclic slice was not cloned")
	}

	validFailure := Failure(&ErrorInfo{Type: "api", Message: "failed"})
	if err := ValidateResult(validFailure); err != nil {
		t.Fatal(err)
	}
	validPending := Pending(nil, &OperationInfo{ID: "task", State: "processing", NextCommand: "dws task get"})
	if err := ValidateResult(validPending); err != nil {
		t.Fatal(err)
	}
	decorated := Success(nil, WithIdentity("user"), WithDryRun(), ResultOption{})
	if decorated.Outcome() != OutcomeSuccess {
		t.Fatalf("Outcome=%s", decorated.Outcome())
	}
	decoratedEnv, err := EnvelopeFromResult(decorated)
	if err != nil || decoratedEnv.Identity != "user" || !decoratedEnv.DryRun {
		t.Fatalf("decorated=%+v err=%v", decoratedEnv, err)
	}

	badPending := []CommandResult{
		Pending(nil, nil),
		Pending(nil, &OperationInfo{State: "processing", NextCommand: "next"}),
		Pending(nil, &OperationInfo{ID: "id", NextCommand: "next"}),
		Pending(nil, &OperationInfo{ID: "id", State: "processing"}),
	}
	for _, result := range badPending {
		if ValidateResult(result) == nil {
			t.Fatalf("malformed pending accepted: %#v", result)
		}
	}
	if ValidateResult(Partial(nil)) == nil {
		t.Fatal("nil partial accepted")
	}
	if err := StoreResult(context.Background(), Pending(nil, nil)); err == nil {
		t.Fatal("StoreResult accepted malformed result")
	}
	if ValidateResult(forgedResult{env: Envelope{OK: true, Outcome: OutcomeSuccess}, exit: 9}) == nil {
		t.Fatal("forged exit code accepted")
	}
	badOverride := &commandResult{env: Envelope{OK: false, Outcome: OutcomeFailure, Error: &ErrorInfo{Type: "api"}}, exitCodeOverride: true}
	if ValidateResult(badOverride) == nil {
		t.Fatal("non-positive override accepted")
	}
	badWire := &commandResult{env: Envelope{OK: false, Outcome: OutcomeFailure, Error: &ErrorInfo{Type: "api", ExitCode: 5}}, exitCode: 1}
	if ValidateResult(badWire) == nil {
		t.Fatal("wire/process exit mismatch accepted")
	}
}

type redactionEdge struct {
	Password *string `json:"password"`
	Plain    string
	Ignored  string `json:"-"`
	hidden   string
}

type redactCycle struct{ Next *redactCycle }

func TestFrameworkRedactionReflectionEdges(t *testing.T) {
	if redactEnvelope(nil) != nil {
		t.Fatal("redactEnvelope(nil) must be nil")
	}
	seen := map[redactVisit]struct{}{}
	redactReflectValue(reflect.Value{}, "", seen)

	secret := "pointer-secret"
	fixture := &redactionEdge{Password: &secret, Plain: "token=plain-secret", Ignored: "ignored-secret", hidden: "hidden-secret"}
	redactReflectValue(reflect.ValueOf(fixture), "", seen) // not settable
	redactReflectValue(reflect.ValueOf(&fixture).Elem(), "", map[redactVisit]struct{}{})
	if fixture.Password == nil || *fixture.Password != redactedValue || !strings.Contains(fixture.Plain, redactedValue) {
		t.Fatalf("fixture not redacted: %+v", fixture)
	}
	if fixture.Ignored != "ignored-secret" || fixture.hidden != "hidden-secret" {
		t.Fatalf("ignored fields changed: %+v", fixture)
	}

	array := [2]string{"password=array-one", "token=array-two"}
	redactReflectValue(reflect.ValueOf(&array).Elem(), "", map[redactVisit]struct{}{})
	if strings.Contains(strings.Join(array[:], ","), "array-") {
		t.Fatalf("array not redacted: %#v", array)
	}

	cyclicMap := map[string]any{}
	cyclicMap["self"] = cyclicMap
	cyclicMap["password"] = "map-secret"
	redactReflectValue(reflect.ValueOf(cyclicMap), "", map[redactVisit]struct{}{})
	if cyclicMap["password"] != redactedValue {
		t.Fatalf("map secret=%v", cyclicMap["password"])
	}
	cyclicSlice := make([]any, 2)
	cyclicSlice[0] = cyclicSlice
	cyclicSlice[1] = "password=slice-secret"
	redactReflectValue(reflect.ValueOf(cyclicSlice), "", map[redactVisit]struct{}{})
	pointerCycle := &redactCycle{}
	pointerCycle.Next = pointerCycle
	redactReflectValue(reflect.ValueOf(pointerCycle), "", map[redactVisit]struct{}{})

	unsettable := reflect.ValueOf("secret")
	if setRedactedValue(unsettable) {
		t.Fatal("unsettable value was redacted")
	}
	settableString := reflect.New(reflect.TypeOf("")).Elem()
	if !setRedactedValue(settableString) || settableString.String() != redactedValue {
		t.Fatal("settable string was not redacted")
	}
	settablePointer := reflect.New(reflect.TypeOf((*string)(nil))).Elem()
	if !setRedactedValue(settablePointer) || settablePointer.Elem().String() != redactedValue {
		t.Fatal("settable pointer was not redacted")
	}
	settableInt := reflect.New(reflect.TypeOf(0)).Elem()
	if setRedactedValue(settableInt) {
		t.Fatal("integer unexpectedly redacted")
	}

	text := redactRecognizableSecrets("?=unchanged https://u:p@example.test?normal=ok&signature=s")
	if !strings.Contains(text, "?=unchanged") || strings.Contains(text, "u:p") || strings.Contains(text, "signature=s") {
		t.Fatalf("recognized secret redaction=%q", text)
	}
}

func TestFrameworkEnvelopeNilAndNestedValidationEdges(t *testing.T) {
	if (&Pagination{}).Validate() == nil || (*Pagination)(nil).Validate() != nil {
		t.Fatal("pagination nil/two-state validation mismatch")
	}
	if (*ErrorInfo)(nil).Validate() == nil {
		t.Fatal("nil ErrorInfo accepted")
	}
	negative := int64(-1)
	for _, info := range []*ErrorInfo{
		{Type: "unknown"},
		{Type: "api", ExitCode: -1},
		{Type: "api", RetryAfterSeconds: &negative},
	} {
		if info.Validate() == nil {
			t.Fatalf("invalid ErrorInfo accepted: %+v", info)
		}
	}
	if (*PartialData)(nil).Validate() != nil {
		t.Fatal("nil PartialData should be compatible")
	}
	if (*OperationInfo)(nil).ValidateNextCommand() != nil || (*OperationInfo)(nil).ValidateTimeoutState() != nil || (*OperationInfo)(nil).Validate() != nil {
		t.Fatal("nil operation should validate")
	}
	env := NewSuccessEnvelope(nil)
	env.Meta = &Meta{Operation: &OperationInfo{NextCommand: " bad "}, Pagination: &Pagination{}}
	if env.Validate() == nil {
		t.Fatal("nested invalid metadata accepted")
	}
}

func TestFrameworkResultStoreLifecycleEdges(t *testing.T) {
	if _, ok := resultStoreFromContext(nil); ok {
		t.Fatal("nil context had a store")
	}
	if err := ResetResultStore(context.Background()); err == nil {
		t.Fatal("ResetResultStore without root store succeeded")
	}
	ctx, store := WithResultStore(nil)
	ctx2, same := WithResultStore(ctx)
	if ctx2 != ctx || same != store {
		t.Fatal("WithResultStore did not reuse existing store")
	}
	if err := StoreResult(context.Background(), Success(nil)); err == nil {
		t.Fatal("StoreResult without root store succeeded")
	}
	if err := StoreResult(ctx, Success(map[string]any{"id": "x"})); err != nil {
		t.Fatal(err)
	}
	if err := StoreResult(ctx, Success(nil)); err == nil {
		t.Fatal("duplicate result accepted")
	}
	if _, _, err := EmitStoredResult(nil); err != nil {
		t.Fatal(err)
	}
	plain := &cobra.Command{Use: "plain"}
	if _, _, err := EmitStoredResult(plain); err != nil {
		t.Fatal(err)
	}
	plain.SetContext(ctx)
	if _, _, err := EmitStoredResult(plain); err == nil {
		t.Fatal("legacy command emitted unified result")
	}

	active := &cobra.Command{Use: "active"}
	active.SetContext(ctx)
	SetCommandRollout(active, RolloutUnifiedActive)
	var out bytes.Buffer
	active.SetOut(&out)
	code, emitted, err := EmitStoredResult(active)
	if err != nil || code != 0 || !emitted {
		t.Fatalf("EmitStoredResult=(%d,%v,%v)", code, emitted, err)
	}
	code, emitted, err = EmitStoredResult(active)
	if err != nil || code != 0 || !emitted {
		t.Fatalf("second EmitStoredResult=(%d,%v,%v)", code, emitted, err)
	}
	if code, attempted, emitted, risk := StoredEmissionState(store); code != 0 || !attempted || !emitted || !risk {
		t.Fatalf("state=(%d,%v,%v,%v)", code, attempted, emitted, risk)
	}
	if code, ok := StoredExitCode(store); code != 0 || !ok {
		t.Fatalf("StoredExitCode=(%d,%v)", code, ok)
	}
	if err := ResetResultStore(ctx); err != nil {
		t.Fatalf("ResetResultStore: %v", err)
	}
	if code, attempted, emitted, risk := StoredEmissionState(store); code != 0 || attempted || emitted || risk {
		t.Fatalf("reset state=(%d,%v,%v,%v)", code, attempted, emitted, risk)
	}
	if _, ok := StoredExitCode(store); ok {
		t.Fatal("reset store retained an emitted exit code")
	}
	if err := StoreResult(ctx, Success(map[string]any{"id": "after-reset"})); err != nil {
		t.Fatalf("StoreResult after reset: %v", err)
	}
}

func TestFrameworkResultStoreNilStateAndWriteFailure(t *testing.T) {
	code, attempted, emitted, risk := StoredEmissionState(nil)
	if code != 0 || attempted || emitted || risk {
		t.Fatalf("nil state=(%d,%v,%v,%v)", code, attempted, emitted, risk)
	}
	if code, ok := StoredExitCode(nil); code != 0 || ok {
		t.Fatalf("nil exit=(%d,%v)", code, ok)
	}
	ctx, store := WithResultStore(context.Background())
	if err := StoreResult(ctx, Success(nil)); err != nil {
		t.Fatal(err)
	}
	cmd := &cobra.Command{Use: "active"}
	cmd.SetContext(ctx)
	SetCommandRollout(cmd, RolloutUnifiedActive)
	cmd.SetOut(errorWriter{})
	if code, emitted, err := EmitStoredResult(cmd); code != exitCodeInternal || emitted || err == nil {
		t.Fatalf("write failure=(%d,%v,%v)", code, emitted, err)
	}
	if code, attempted, emitted, risk := StoredEmissionState(store); code != exitCodeInternal || !attempted || emitted || risk {
		t.Fatalf("failed state=(%d,%v,%v,%v)", code, attempted, emitted, risk)
	}
	if _, err := EnvelopeFromResult(nil); err == nil {
		t.Fatal("EnvelopeFromResult(nil) succeeded")
	}
}

type errorWriter struct{}

func (errorWriter) Write([]byte) (int, error) { return 0, errors.New("write failed") }

func TestFrameworkEmitterAndRolloutEdges(t *testing.T) {
	if jsonShorthandActive(nil) {
		t.Fatal("nil command activated --json")
	}
	if err := WriteEnvelope(nil, nil, FormatJSON); err != nil {
		t.Fatal(err)
	}
	cmd := &cobra.Command{Use: "cmd"}
	cmd.SetOut(io.Discard)
	cmd.SetErr(errorWriter{})
	cmd.Flags().String("format", "unknown", "")
	if err := WriteEnvelope(cmd, NewSuccessEnvelope(nil), FormatJSON); err == nil {
		t.Fatal("warning write error was ignored")
	}
	if _, _, err := emitResult(nil, nil); err == nil {
		t.Fatal("nil result accepted")
	}
	broken := NewSuccessEnvelope(make(chan int))
	if err := WriteEnvelope(&cobra.Command{Use: "broken"}, broken, FormatJSON); err == nil {
		t.Fatal("WriteEnvelope accepted unrenderable data")
	}
	if _, _, err := emitResult(&cobra.Command{Use: "broken"}, Success(make(chan int))); err == nil {
		t.Fatal("emitResult accepted unrenderable data")
	}

	human := &cobra.Command{Use: "human"}
	human.Flags().String("format", "table", "")
	human.SetErr(errorWriter{})
	if _, risk, err := emitResult(human, Failure(&ErrorInfo{Type: "api", Message: "boom"})); err == nil || risk {
		t.Fatalf("human failure write=(risk=%v, err=%v)", risk, err)
	}
	short := &shortWriter{}
	human.SetErr(short)
	if _, risk, err := emitResult(human, Failure(&ErrorInfo{Type: "api"})); !errors.Is(err, io.ErrShortWrite) || !risk {
		t.Fatalf("human short write=(risk=%v, err=%v)", risk, err)
	}

	machine := &cobra.Command{Use: "machine"}
	machine.SetOut(errorWriter{})
	if _, risk, err := emitResult(machine, Success(nil)); err == nil || risk {
		t.Fatalf("machine write=(risk=%v, err=%v)", risk, err)
	}
	machine.SetOut(&shortWriter{})
	if _, risk, err := emitResult(machine, Success(nil)); !errors.Is(err, io.ErrShortWrite) || !risk {
		t.Fatalf("machine short write=(risk=%v, err=%v)", risk, err)
	}

	if err := renderEnvelope(io.Discard, nil, nil, FormatJSON, "", ""); err != nil {
		t.Fatal(err)
	}
	var fallback bytes.Buffer
	if err := renderEnvelopeInto(&fallback, io.Discard, NewSuccessEnvelope(map[string]any{"id": "x"}), Format("unexpected"), "", ""); err != nil || !strings.Contains(fallback.String(), `"outcome"`) {
		t.Fatalf("unknown format fallback=%q err=%v", fallback.String(), err)
	}
	if got := ExitCodeForEnvelope(&Envelope{Outcome: OutcomeFailure, Error: &ErrorInfo{Type: "permission"}}); got != exitCodePermission {
		t.Fatalf("permission exit=%d", got)
	}
	var ndjson bytes.Buffer
	if err := renderEnvelopeInto(&ndjson, io.Discard, Failure(&ErrorInfo{Type: "api"}).envelope(), FormatNDJSON, "", ""); err != nil || strings.Count(strings.TrimSpace(ndjson.String()), "\n") != 0 {
		t.Fatalf("NDJSON failure=%q err=%v", ndjson.String(), err)
	}
	if _, err := writeAllCount(&shortWriter{}, []byte("payload")); !errors.Is(err, io.ErrShortWrite) {
		t.Fatalf("short write err=%v", err)
	}

	if _, err := ParseRolloutState("bad"); err == nil {
		t.Fatal("invalid rollout parsed")
	}
	if err := ValidateRolloutTransition("bad", RolloutLegacyOnly, false); err == nil {
		t.Fatal("invalid from state accepted")
	}
	if err := ValidateRolloutTransition(RolloutLegacyOnly, "bad", false); err == nil {
		t.Fatal("invalid to state accepted")
	}
	if rolloutRank("bad") != -1 {
		t.Fatal("unknown rollout rank")
	}
	SetCommandRollout(nil, RolloutUnifiedActive)
	if err := ValidateUnifiedFormat(nil); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if recover() == nil {
			t.Fatal("invalid rollout did not panic")
		}
	}()
	SetCommandRollout(&cobra.Command{Use: "bad"}, "bad")
}

func TestFrameworkMCPAdapterErrors(t *testing.T) {
	if _, err := AdaptMCP(nil); err == nil {
		t.Fatal("AdaptMCP(nil) succeeded")
	}
	originalMarshal, originalUnmarshal := marshalMCPResult, unmarshalMCPResult
	t.Cleanup(func() { marshalMCPResult, unmarshalMCPResult = originalMarshal, originalUnmarshal })
	marshalMCPResult = func(any) ([]byte, error) { return nil, errors.New("marshal") }
	if _, err := AdaptMCP(Success(nil)); err == nil {
		t.Fatal("MCP marshal failure ignored")
	}
	marshalMCPResult = func(any) ([]byte, error) { return []byte("{"), nil }
	unmarshalMCPResult = originalUnmarshal
	if _, err := AdaptMCP(Success(nil)); err == nil {
		t.Fatal("MCP decode failure ignored")
	}
}

type shortWriter struct{}

func (*shortWriter) Write(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	return len(p) - 1, nil
}
