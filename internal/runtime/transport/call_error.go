package transport

import "fmt"

type CallStage string

const (
	CallStageRequest CallStage = "request"
	CallStageHTTP    CallStage = "http"
	CallStageJSONRPC CallStage = "jsonrpc"
)

// CallError carries recoverable transport metadata without changing the
// repository-local structured error contract.
type CallError struct {
	Stage      CallStage
	HTTPStatus int
	RetryAfter string
	TraceID    string
	RequestID  string
	RPCCode    int
	Cause      error
}

func (e *CallError) Error() string {
	if e == nil {
		return ""
	}
	switch {
	case e.Cause != nil:
		return e.Cause.Error()
	case e.HTTPStatus != 0:
		return fmt.Sprintf("%s failure: http %d", e.Stage, e.HTTPStatus)
	case e.RPCCode != 0:
		return fmt.Sprintf("%s failure: rpc %d", e.Stage, e.RPCCode)
	default:
		return string(e.Stage) + " failure"
	}
}

func (e *CallError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}
