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
	}
	var apiErr *graph.APIError
	if errors.As(err, &apiErr) {
		errorInfo.Type = apiErr.Type
		errorInfo.Code = apiErr.Code
		errorInfo.ErrorSubcode = apiErr.ErrorSubcode
		errorInfo.Message = apiErr.Message
		errorInfo.FBTraceID = apiErr.FBTraceID
		errorInfo.Retryable = apiErr.Retryable
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
