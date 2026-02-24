package ops

import (
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
)

const (
	ContractVersion = "ops.v1"
	CommandInit     = "meta ops init"
	CommandRun      = "meta ops run"
	CommandCleanup  = "meta ops cleanup"
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
	SchemaVersion int             `json:"schema_version"`
	Kind          string          `json:"kind"`
	Baseline      BaselineState   `json:"baseline"`
	Summary       Summary         `json:"summary"`
	Outcome       string          `json:"outcome"`
	Sections      []ReportSection `json:"sections"`
	Checks        []Check         `json:"checks"`
}

type Summary struct {
	Total    int `json:"total"`
	Passed   int `json:"passed"`
	Failed   int `json:"failed"`
	Warnings int `json:"warnings"`
	Blocking int `json:"blocking"`
}

type ReportSection struct {
	Name    string  `json:"name"`
	Summary Summary `json:"summary"`
	Outcome string  `json:"outcome"`
	Checks  []Check `json:"checks"`
}

type Check struct {
	Name     string `json:"name"`
	Status   string `json:"status"`
	Blocking bool   `json:"blocking"`
	Message  string `json:"message,omitempty"`
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
		Outcome:       RunOutcomeClean,
		Sections:      composeReportSections(nil),
		Checks:        []Check{},
	}
}

func WriteEnvelope(w io.Writer, format string, envelope Envelope) error {
	if w == nil {
		return errors.New("writer is nil")
	}
	if strings.TrimSpace(envelope.ContractVersion) == "" {
		envelope.ContractVersion = ContractVersion
	}
	switch strings.ToLower(strings.TrimSpace(format)) {
	case "", "json":
		return writeEnvelopeJSON(w, envelope)
	case "jsonl":
		return writeEnvelopeJSONL(w, envelope)
	case "csv":
		return writeEnvelopeCSV(w, envelope)
	default:
		return fmt.Errorf("unsupported ops output format %q; expected json|jsonl|csv", format)
	}
}

func writeEnvelopeJSON(w io.Writer, envelope Envelope) error {
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	encoder.SetEscapeHTML(false)
	return encoder.Encode(envelope)
}

func writeEnvelopeJSONL(w io.Writer, envelope Envelope) error {
	result, ok := runResultFromEnvelope(envelope)
	if !ok {
		return writeEnvelopeJSONLLine(w, envelope)
	}

	sections := result.Report.Sections
	if len(sections) == 0 {
		sections = composeReportSections(result.Report.Checks)
	}
	if len(sections) == 0 {
		return writeEnvelopeJSONLLine(w, envelope)
	}

	for _, section := range sections {
		lineEnvelope := envelope
		lineEnvelope.Data = runSectionLine{
			StatePath:           result.StatePath,
			ReportSchemaVersion: result.Report.SchemaVersion,
			ReportKind:          result.Report.Kind,
			ReportOutcome:       RunOutcomeForReport(result.Report),
			Section:             section,
		}
		if err := writeEnvelopeJSONLLine(w, lineEnvelope); err != nil {
			return err
		}
	}
	return nil
}

func writeEnvelopeJSONLLine(w io.Writer, envelope Envelope) error {
	encoder := json.NewEncoder(w)
	encoder.SetEscapeHTML(false)
	return encoder.Encode(envelope)
}

func writeEnvelopeCSV(w io.Writer, envelope Envelope) error {
	result, ok := runResultFromEnvelope(envelope)
	if !ok {
		return writeEnvelopeErrorCSV(w, envelope)
	}
	return writeRunResultCSV(w, result)
}

func writeRunResultCSV(w io.Writer, result RunResult) error {
	sections := result.Report.Sections
	if len(sections) == 0 {
		sections = composeReportSections(result.Report.Checks)
	}

	cw := csv.NewWriter(w)
	header := []string{
		"state_path",
		"report_outcome",
		"section",
		"section_outcome",
		"section_total",
		"section_passed",
		"section_failed",
		"section_warnings",
		"section_blocking",
		"check_name",
		"check_status",
		"check_blocking",
		"check_message",
	}
	if err := cw.Write(header); err != nil {
		return err
	}

	reportOutcome := RunOutcomeForReport(result.Report)
	for _, section := range sections {
		base := []string{
			result.StatePath,
			reportOutcome,
			section.Name,
			section.Outcome,
			strconv.Itoa(section.Summary.Total),
			strconv.Itoa(section.Summary.Passed),
			strconv.Itoa(section.Summary.Failed),
			strconv.Itoa(section.Summary.Warnings),
			strconv.Itoa(section.Summary.Blocking),
		}
		if len(section.Checks) == 0 {
			record := append(append([]string{}, base...), "", "", "", "")
			if err := cw.Write(record); err != nil {
				return err
			}
			continue
		}
		for _, check := range section.Checks {
			record := append(append([]string{}, base...),
				check.Name,
				check.Status,
				strconv.FormatBool(check.Blocking),
				check.Message,
			)
			if err := cw.Write(record); err != nil {
				return err
			}
		}
	}

	cw.Flush()
	return cw.Error()
}

func writeEnvelopeErrorCSV(w io.Writer, envelope Envelope) error {
	cw := csv.NewWriter(w)
	header := []string{
		"contract_version",
		"command",
		"success",
		"exit_code",
		"outcome",
		"error_type",
		"error_message",
	}
	if err := cw.Write(header); err != nil {
		return err
	}

	outcome := RunOutcomeError
	if envelope.Success {
		outcome = RunOutcomeClean
	}

	errorType := ""
	errorMessage := ""
	if envelope.Error != nil {
		errorType = envelope.Error.Type
		errorMessage = envelope.Error.Message
	}

	record := []string{
		envelope.ContractVersion,
		envelope.Command,
		strconv.FormatBool(envelope.Success),
		strconv.Itoa(envelope.ExitCode),
		outcome,
		errorType,
		errorMessage,
	}
	if err := cw.Write(record); err != nil {
		return err
	}
	cw.Flush()
	return cw.Error()
}

type runSectionLine struct {
	StatePath           string        `json:"state_path"`
	ReportSchemaVersion int           `json:"report_schema_version"`
	ReportKind          string        `json:"report_kind"`
	ReportOutcome       string        `json:"report_outcome"`
	Section             ReportSection `json:"section"`
}

func runResultFromEnvelope(envelope Envelope) (RunResult, bool) {
	switch data := envelope.Data.(type) {
	case RunResult:
		return data, true
	case *RunResult:
		if data == nil {
			return RunResult{}, false
		}
		return *data, true
	case map[string]any:
		encoded, err := json.Marshal(data)
		if err != nil {
			return RunResult{}, false
		}
		var result RunResult
		if err := json.Unmarshal(encoded, &result); err != nil {
			return RunResult{}, false
		}
		return result, true
	default:
		return RunResult{}, false
	}
}

func errorType(code int) string {
	switch code {
	case ExitCodeRuntime:
		return "runtime_error"
	case ExitCodeInput:
		return "input_error"
	case ExitCodeState:
		return "state_error"
	case ExitCodePolicy:
		return "policy_error"
	case ExitCodeWarning:
		return "warning_error"
	default:
		return "error"
	}
}
