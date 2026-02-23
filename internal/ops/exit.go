package ops

import (
	"errors"
	"fmt"
)

const (
	ExitCodeSuccess = 0
	ExitCodeUnknown = 1
	ExitCodeState   = 2
	ExitCodeInput   = 4
	ExitCodePolicy  = 8
)

type ExitError struct {
	Code int
	Err  error
}

func (e *ExitError) Error() string {
	if e == nil {
		return ""
	}
	if e.Err == nil {
		return fmt.Sprintf("command failed with exit code %d", e.Code)
	}
	return e.Err.Error()
}

func (e *ExitError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

func WrapExit(code int, err error) error {
	if err == nil {
		return nil
	}
	return &ExitError{Code: code, Err: err}
}

func ExitCode(err error) int {
	if err == nil {
		return ExitCodeSuccess
	}
	var exitErr *ExitError
	if errors.As(err, &exitErr) && exitErr.Code > 0 {
		return exitErr.Code
	}
	return ExitCodeUnknown
}
