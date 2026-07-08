package registry

import (
	"fmt"
	"io"
	"testing"
)

func TestIsConnectionError(t *testing.T) {
	connCases := []error{
		io.EOF,
		io.ErrClosedPipe,
		fmt.Errorf("write: broken pipe"),
		fmt.Errorf("read tcp: connection reset by peer"),
		fmt.Errorf("dial tcp: connection refused"),
		fmt.Errorf("use of closed network connection"),
		fmt.Errorf("mcp session closed"),
		fmt.Errorf("signal: killed"),
		fmt.Errorf("process exited with status 1"),
	}
	for _, err := range connCases {
		if !isConnectionError(err) {
			t.Errorf("expected connection error for %q", err)
		}
	}

	notConn := []error{
		nil,
		fmt.Errorf("tool 'foo' not found"),
		fmt.Errorf("invalid parameter: a must be an integer"),
		fmt.Errorf("file does not exist: /etc/config"),
		fmt.Errorf("permission denied"),
	}
	for _, err := range notConn {
		if isConnectionError(err) {
			t.Errorf("did not expect connection error for %v", err)
		}
	}
}

func TestIsParameterError(t *testing.T) {
	paramMsgs := []string{
		"invalid parameter: a",
		"missing required field 'b'",
		"unknown property foo",
		"schema validation failed",
		"invalid_params",
		"invalid type for parameter x",
	}
	for _, m := range paramMsgs {
		if !isParameterError(m) {
			t.Errorf("expected parameter error for %q", m)
		}
	}

	// Ordinary tool failures that merely contain generic words must NOT be
	// reframed as parameter errors.
	nonParamMsgs := []string{
		"file not found: config missing",
		"the requested type is unavailable",
		"unknown host",
		"invalid state transition",
		"resource temporarily unavailable",
	}
	for _, m := range nonParamMsgs {
		if isParameterError(m) {
			t.Errorf("did not expect parameter error for %q", m)
		}
	}
}
