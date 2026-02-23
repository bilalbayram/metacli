package cli

import "fmt"

const (
	ExitCodeUnknown = 1
	ExitCodeConfig  = 2
	ExitCodeAuth    = 3
	ExitCodeInput   = 4
	ExitCodeAPI     = 5
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
