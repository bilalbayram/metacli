package ops

import (
	"encoding/json"
	"errors"
	"io"
	"strings"
)

const (
	ContractVersion = "ops.v1"
	CommandInit     = "meta ops init"
	CommandRun      = "meta ops run"
)

const ReportSchemaVersion = 1

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

type InitResult struct {
	StatePath string        `json:"state_path"`
	State     BaselineState `json:"state"`
}

type RunResult struct {
	StatePath string `json:"state_path"`
	Report    Report `json:"report"`
}

type Report struct {
	SchemaVersion int           `json:"schema_version"`
	Kind          string        `json:"kind"`
	Baseline      BaselineState `json:"baseline"`
	Summary       Summary       `json:"summary"`
	Checks        []Check       `json:"checks"`
}

type Summary struct {
	Total  int `json:"total"`
	Passed int `json:"passed"`
	Failed int `json:"failed"`
}

type Check struct {
	Name    string `json:"name"`
	Status  string `json:"status"`
	Message string `json:"message,omitempty"`
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

func NewReportSkeleton(state BaselineState) Report {
	return Report{
		SchemaVersion: ReportSchemaVersion,
		Kind:          "ops_report",
		Baseline:      state,
		Summary:       Summary{},
		Checks:        []Check{},
	}
}

func WriteEnvelope(w io.Writer, envelope Envelope) error {
	if w == nil {
		return errors.New("writer is nil")
	}
	if strings.TrimSpace(envelope.ContractVersion) == "" {
		envelope.ContractVersion = ContractVersion
	}
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	encoder.SetEscapeHTML(false)
	return encoder.Encode(envelope)
}

func errorType(code int) string {
	switch code {
	case ExitCodeInput:
		return "input_error"
	case ExitCodeState:
		return "state_error"
	default:
		return "error"
	}
}
