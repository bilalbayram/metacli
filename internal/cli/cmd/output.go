package cmd

import (
	"errors"
	"fmt"

	"github.com/bilalbayram/metacli/internal/graph"
	"github.com/bilalbayram/metacli/internal/output"
	"github.com/spf13/cobra"
)

func writeSuccess(cmd *cobra.Command, runtime Runtime, commandName string, data any, paging any, rateLimit any) error {
	envelope, err := output.NewEnvelope(commandName, true, data, paging, rateLimit, nil)
	if err != nil {
		return err
	}
	return output.Write(cmd.OutOrStdout(), selectedOutputFormat(runtime), envelope)
}

func writeCommandError(cmd *cobra.Command, runtime Runtime, commandName string, err error) error {
	if err == nil {
		return nil
	}
	errorInfo := &output.ErrorInfo{
		Type:      "error",
		Message:   err.Error(),
		Retryable: false,
		Remediation: &output.Remediation{
			Category: graph.RemediationCategoryUnknown,
			Summary:  "Unhandled command failure.",
			Actions: []string{
				"Review the error message and fix input/configuration before retrying.",
			},
		},
	}
	var apiErr *graph.APIError
	if errors.As(err, &apiErr) {
		errorInfo.Type = apiErr.Type
		errorInfo.Code = apiErr.Code
		errorInfo.ErrorSubcode = apiErr.ErrorSubcode
		errorInfo.StatusCode = apiErr.StatusCode
		errorInfo.Message = apiErr.Message
		errorInfo.FBTraceID = apiErr.FBTraceID
		errorInfo.Retryable = apiErr.Retryable
		errorInfo.Diagnostics = cloneMap(apiErr.Diagnostics)
		errorInfo.Remediation = mapRemediation(apiErr.Remediation)
		if errorInfo.Remediation == nil {
			remediation := graph.ClassifyRemediation(apiErr.StatusCode, apiErr.Code, apiErr.ErrorSubcode, apiErr.Message, apiErr.Diagnostics)
			errorInfo.Remediation = mapRemediation(&remediation)
		}
	}

	envelope, envErr := output.NewEnvelope(commandName, false, nil, nil, nil, errorInfo)
	if envErr != nil {
		return fmt.Errorf("%w (secondary output error: %v)", err, envErr)
	}
	if writeErr := output.Write(cmd.ErrOrStderr(), selectedOutputFormat(runtime), envelope); writeErr != nil {
		return fmt.Errorf("%w (secondary output error: %v)", err, writeErr)
	}
	return err
}

func selectedOutputFormat(runtime Runtime) string {
	if runtime.Output == nil {
		return "json"
	}
	if *runtime.Output == "" {
		return "json"
	}
	return *runtime.Output
}

func mapRemediation(remediation *graph.Remediation) *output.Remediation {
	if remediation == nil {
		return nil
	}

	fields := make([]string, 0, len(remediation.Fields))
	for _, field := range remediation.Fields {
		fields = append(fields, field)
	}

	actions := make([]string, 0, len(remediation.Actions))
	for _, action := range remediation.Actions {
		actions = append(actions, action)
	}

	return &output.Remediation{
		Category: remediation.Category,
		Summary:  remediation.Summary,
		Actions:  actions,
		Fields:   fields,
	}
}

func cloneMap(source map[string]any) map[string]any {
	if len(source) == 0 {
		return nil
	}
	cloned := make(map[string]any, len(source))
	for key, value := range source {
		cloned[key] = cloneAny(value)
	}
	return cloned
}

func cloneAny(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		return cloneMap(typed)
	case []any:
		cloned := make([]any, 0, len(typed))
		for _, item := range typed {
			cloned = append(cloned, cloneAny(item))
		}
		return cloned
	default:
		return typed
	}
}
