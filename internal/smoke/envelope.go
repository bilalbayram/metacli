package smoke

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
)

const (
	ContractVersion = "smoke.v2"
	CommandRun      = "meta smoke run"
)

type Envelope struct {
	ContractVersion string     `json:"contract_version"`
	Command         string     `json:"command"`
	Success         bool       `json:"success"`
	ExitCode        int        `json:"exit_code"`
	Data            any        `json:"data,omitempty"`
	Error           *ErrorInfo `json:"error,omitempty"`
}

type ErrorInfo struct {
	Type    string `json:"type"`
	Message string `json:"message"`
}

func NewSuccessEnvelope(command string, data any) Envelope {
	return Envelope{
		ContractVersion: ContractVersion,
		Command:         strings.TrimSpace(command),
		Success:         true,
		ExitCode:        ExitCodeSuccess,
		Data:            data,
	}
}

func NewErrorEnvelope(command string, code int, err error) Envelope {
	if code <= 0 {
		code = ExitCodeUnknown
	}
	message := ""
	if err != nil {
		message = err.Error()
	}
	return Envelope{
		ContractVersion: ContractVersion,
		Command:         strings.TrimSpace(command),
		Success:         false,
		ExitCode:        code,
		Error: &ErrorInfo{
			Type:    errorType(code),
			Message: message,
		},
	}
}

func WriteEnvelope(w io.Writer, format string, envelope Envelope) error {
	if w == nil {
		return errors.New("writer is nil")
	}
	if strings.TrimSpace(envelope.ContractVersion) == "" {
		envelope.ContractVersion = ContractVersion
	}
	if strings.ToLower(strings.TrimSpace(format)) != "json" {
		return fmt.Errorf("smoke commands require --output json, got %q", format)
	}
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	encoder.SetEscapeHTML(false)
	return encoder.Encode(envelope)
}

func errorType(code int) string {
	switch code {
	case ExitCodeRuntime:
		return "runtime_error"
	case ExitCodeInput:
		return "input_error"
	case ExitCodePolicy:
		return "policy_error"
	case ExitCodeWarning:
		return "warning_error"
	default:
		return "error"
	}
}
