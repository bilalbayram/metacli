package cmd

import (
	"errors"
	"fmt"
	"strings"

	"github.com/bilalbayram/metacli/internal/ops"
	"github.com/spf13/cobra"
)

func NewOpsCommand(runtime Runtime) *cobra.Command {
	opsCmd := &cobra.Command{
		Use:           "ops",
		Short:         "Operations baseline and reporting commands",
		SilenceErrors: true,
		SilenceUsage:  true,
	}
	opsCmd.AddCommand(newOpsInitCommand(runtime))
	opsCmd.AddCommand(newOpsRunCommand(runtime))
	return opsCmd
}

func newOpsInitCommand(runtime Runtime) *cobra.Command {
	var statePath string

	cmd := &cobra.Command{
		Use:   "init",
		Short: "Initialize baseline operations state",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := ensureOpsOutput(runtime); err != nil {
				return writeOpsError(cmd, ops.CommandInit, ops.WrapExit(ops.ExitCodeInput, err))
			}

			resolvedPath, err := resolveStatePath(statePath)
			if err != nil {
				return writeOpsError(cmd, ops.CommandInit, ops.WrapExit(ops.ExitCodeState, err))
			}

			result, err := ops.Initialize(resolvedPath)
			if err != nil {
				return writeOpsError(cmd, ops.CommandInit, err)
			}

			envelope := ops.NewSuccessEnvelope(ops.CommandInit, result)
			if err := ops.WriteEnvelope(cmd.OutOrStdout(), envelope); err != nil {
				return writeOpsError(cmd, ops.CommandInit, ops.WrapExit(ops.ExitCodeUnknown, fmt.Errorf("write success envelope: %w", err)))
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&statePath, "state-path", "", "Path to baseline state JSON file")
	return cmd
}

func newOpsRunCommand(runtime Runtime) *cobra.Command {
	var statePath string

	cmd := &cobra.Command{
		Use:   "run",
		Short: "Run operations report skeleton against baseline state",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := ensureOpsOutput(runtime); err != nil {
				return writeOpsError(cmd, ops.CommandRun, ops.WrapExit(ops.ExitCodeInput, err))
			}

			resolvedPath, err := resolveStatePath(statePath)
			if err != nil {
				return writeOpsError(cmd, ops.CommandRun, ops.WrapExit(ops.ExitCodeState, err))
			}

			result, err := ops.Run(resolvedPath)
			if err != nil {
				return writeOpsError(cmd, ops.CommandRun, err)
			}

			envelope := ops.NewSuccessEnvelope(ops.CommandRun, result)
			if code := ops.RunExitCode(result.Report); code != ops.ExitCodeSuccess {
				envelope.Success = false
				envelope.ExitCode = code
				envelope.Error = &ops.ErrorInfo{
					Type:    "blocking_findings",
					Message: fmt.Sprintf("ops run reported %d blocking finding(s)", result.Report.Summary.Blocking),
				}
			}
			if err := ops.WriteEnvelope(cmd.OutOrStdout(), envelope); err != nil {
				return writeOpsError(cmd, ops.CommandRun, ops.WrapExit(ops.ExitCodeUnknown, fmt.Errorf("write success envelope: %w", err)))
			}
			if !envelope.Success {
				return ops.WrapExit(envelope.ExitCode, errors.New(envelope.Error.Message))
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&statePath, "state-path", "", "Path to baseline state JSON file")
	return cmd
}

func ensureOpsOutput(runtime Runtime) error {
	format := strings.ToLower(strings.TrimSpace(selectedOutputFormat(runtime)))
	if format != "json" {
		return fmt.Errorf("ops commands require --output json, got %q", format)
	}
	return nil
}

func resolveStatePath(path string) (string, error) {
	path = strings.TrimSpace(path)
	if path != "" {
		return path, nil
	}
	return ops.DefaultStatePath()
}

func writeOpsError(cmd *cobra.Command, command string, err error) error {
	code := ops.ExitCode(err)
	if code == ops.ExitCodeSuccess {
		code = ops.ExitCodeUnknown
	}

	envelope := ops.NewErrorEnvelope(command, code, err)
	if writeErr := ops.WriteEnvelope(cmd.ErrOrStderr(), envelope); writeErr != nil {
		return ops.WrapExit(code, fmt.Errorf("%w (secondary output error: %v)", err, writeErr))
	}

	var exitErr *ops.ExitError
	if errors.As(err, &exitErr) {
		return err
	}
	return ops.WrapExit(code, err)
}
