package cmd

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/bilalbayram/metacli/internal/config"
	"github.com/bilalbayram/metacli/internal/lint"
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
			if err := ensureOpsOutput(runtime, ops.CommandInit); err != nil {
				return writeOpsError(cmd, runtime, ops.CommandInit, ops.WrapExit(ops.ExitCodeInput, err))
			}

			resolvedPath, err := resolveStatePath(statePath)
			if err != nil {
				return writeOpsError(cmd, runtime, ops.CommandInit, ops.WrapExit(ops.ExitCodeState, err))
			}

			result, err := ops.Initialize(resolvedPath)
			if err != nil {
				return writeOpsError(cmd, runtime, ops.CommandInit, err)
			}

			envelope := ops.NewSuccessEnvelope(ops.CommandInit, result)
			if err := ops.WriteEnvelope(cmd.OutOrStdout(), opsEnvelopeOutputFormat(runtime), envelope); err != nil {
				return writeOpsError(cmd, runtime, ops.CommandInit, ops.WrapExit(ops.ExitCodeUnknown, fmt.Errorf("write success envelope: %w", err)))
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&statePath, "state-path", "", "Path to baseline state JSON file")
	return cmd
}

func newOpsRunCommand(runtime Runtime) *cobra.Command {
	var statePath string
	var rateTelemetryPath string
	var preflightConfigPath string
	var preflightOptionalPolicy string
	var runtimeResponsePath string
	var lintRequestPath string

	cmd := &cobra.Command{
		Use:   "run",
		Short: "Run operations report skeleton against baseline state",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := ensureOpsOutput(runtime, ops.CommandRun); err != nil {
				return writeOpsError(cmd, runtime, ops.CommandRun, ops.WrapExit(ops.ExitCodeInput, err))
			}

			resolvedPath, err := resolveStatePath(statePath)
			if err != nil {
				return writeOpsError(cmd, runtime, ops.CommandRun, ops.WrapExit(ops.ExitCodeState, err))
			}

			if err := ops.ValidateOptionalModulePolicy(preflightOptionalPolicy); err != nil {
				return writeOpsError(cmd, runtime, ops.CommandRun, ops.WrapExit(ops.ExitCodeInput, err))
			}
			normalizedPreflightOptionalPolicy := ops.NormalizeOptionalModulePolicy(preflightOptionalPolicy)

			runOptions := ops.RunOptions{
				OptionalModulePolicy: normalizedPreflightOptionalPolicy,
			}
			preflightSnapshot := buildPermissionPreflightSnapshot(runtime.ProfileName(), preflightConfigPath, normalizedPreflightOptionalPolicy)
			runOptions.PermissionPreflight = &preflightSnapshot

			if strings.TrimSpace(rateTelemetryPath) != "" {
				snapshot, err := loadRateLimitTelemetrySnapshot(rateTelemetryPath)
				if err != nil {
					return writeOpsError(cmd, runtime, ops.CommandRun, ops.WrapExit(ops.ExitCodeInput, err))
				}
				runOptions.RateLimitTelemetry = &snapshot
			}
			if strings.TrimSpace(runtimeResponsePath) != "" {
				snapshot, err := loadRuntimeResponseShapeSnapshot(runtimeResponsePath)
				if err != nil {
					return writeOpsError(cmd, runtime, ops.CommandRun, ops.WrapExit(ops.ExitCodeInput, err))
				}
				runOptions.RuntimeResponse = &snapshot
			}
			if strings.TrimSpace(lintRequestPath) != "" {
				spec, err := lint.LoadRequestSpec(lintRequestPath)
				if err != nil {
					return writeOpsError(cmd, runtime, ops.CommandRun, ops.WrapExit(ops.ExitCodeInput, err))
				}
				runOptions.LintRequestSpec = spec
				runOptions.LintRequestSpecFile = lintRequestPath
			}

			result, err := ops.RunWithOptions(resolvedPath, runOptions)
			if err != nil {
				return writeOpsError(cmd, runtime, ops.CommandRun, err)
			}

			envelope := ops.NewSuccessEnvelope(ops.CommandRun, result)
			if code := ops.RunExitCode(result.Report); code != ops.ExitCodeSuccess {
				envelope.Success = false
				envelope.ExitCode = code
				switch ops.RunOutcomeForReport(result.Report) {
				case ops.RunOutcomeWarning:
					envelope.Error = &ops.ErrorInfo{
						Type:    "warning_findings",
						Message: fmt.Sprintf("ops run reported %d warning finding(s)", result.Report.Summary.Warnings),
					}
				case ops.RunOutcomeBlocking:
					envelope.Error = &ops.ErrorInfo{
						Type:    "blocking_findings",
						Message: fmt.Sprintf("ops run reported %d blocking finding(s)", result.Report.Summary.Blocking),
					}
				default:
					envelope.Error = &ops.ErrorInfo{
						Type:    "runtime_error",
						Message: "ops run failed with runtime error",
					}
				}
			}
			if err := ops.WriteEnvelope(cmd.OutOrStdout(), opsEnvelopeOutputFormat(runtime), envelope); err != nil {
				return writeOpsError(cmd, runtime, ops.CommandRun, ops.WrapExit(ops.ExitCodeUnknown, fmt.Errorf("write success envelope: %w", err)))
			}
			if !envelope.Success {
				return ops.WrapExit(envelope.ExitCode, errors.New(envelope.Error.Message))
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&statePath, "state-path", "", "Path to baseline state JSON file")
	cmd.Flags().StringVar(&rateTelemetryPath, "rate-telemetry-file", "", "Path to rate-limit telemetry JSON snapshot file")
	cmd.Flags().StringVar(&preflightConfigPath, "preflight-config-path", "", "Path to auth config file used for permission preflight")
	cmd.Flags().StringVar(&preflightOptionalPolicy, "preflight-optional-policy", ops.OptionalModulePolicyStrict, "Policy for optional preflight modules: strict|skip")
	cmd.Flags().StringVar(&runtimeResponsePath, "runtime-response-file", "", "Path to runtime response shape snapshot JSON file")
	cmd.Flags().StringVar(&lintRequestPath, "lint-request-file", "", "Path to lint request spec JSON file linked to runtime drift check")
	return cmd
}

func ensureOpsOutput(runtime Runtime, command string) error {
	format := strings.ToLower(strings.TrimSpace(selectedOutputFormat(runtime)))
	switch command {
	case ops.CommandRun:
		if format == "json" || format == "jsonl" || format == "csv" {
			return nil
		}
		return fmt.Errorf("meta ops run requires --output json|jsonl|csv, got %q", format)
	default:
		if format == "json" {
			return nil
		}
		return fmt.Errorf("ops commands require --output json, got %q", format)
	}
}

func resolveStatePath(path string) (string, error) {
	path = strings.TrimSpace(path)
	if path != "" {
		return path, nil
	}
	return ops.DefaultStatePath()
}

func writeOpsError(cmd *cobra.Command, runtime Runtime, command string, err error) error {
	code := ops.ExitCode(err)
	if code == ops.ExitCodeSuccess {
		code = ops.ExitCodeUnknown
	}

	envelope := ops.NewErrorEnvelope(command, code, err)
	if writeErr := ops.WriteEnvelope(cmd.ErrOrStderr(), opsEnvelopeOutputFormat(runtime), envelope); writeErr != nil {
		return ops.WrapExit(code, fmt.Errorf("%w (secondary output error: %v)", err, writeErr))
	}

	var exitErr *ops.ExitError
	if errors.As(err, &exitErr) {
		return err
	}
	return ops.WrapExit(code, err)
}

func loadRateLimitTelemetrySnapshot(path string) (ops.RateLimitTelemetrySnapshot, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return ops.RateLimitTelemetrySnapshot{}, errors.New("rate telemetry file path is required")
	}

	body, err := os.ReadFile(path)
	if err != nil {
		return ops.RateLimitTelemetrySnapshot{}, fmt.Errorf("read rate telemetry file %s: %w", path, err)
	}

	var snapshot ops.RateLimitTelemetrySnapshot
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&snapshot); err != nil {
		return ops.RateLimitTelemetrySnapshot{}, fmt.Errorf("decode rate telemetry file %s: %w", path, err)
	}
	var trailing struct{}
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return ops.RateLimitTelemetrySnapshot{}, fmt.Errorf("decode rate telemetry file %s: multiple JSON values", path)
		}
		return ops.RateLimitTelemetrySnapshot{}, fmt.Errorf("decode rate telemetry file %s: %w", path, err)
	}
	return snapshot, nil
}

func opsEnvelopeOutputFormat(runtime Runtime) string {
	format := strings.ToLower(strings.TrimSpace(selectedOutputFormat(runtime)))
	switch format {
	case "json", "jsonl", "csv":
		return format
	default:
		return "json"
	}
}

func loadRuntimeResponseShapeSnapshot(path string) (ops.RuntimeResponseShapeSnapshot, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return ops.RuntimeResponseShapeSnapshot{}, errors.New("runtime response snapshot file path is required")
	}

	body, err := os.ReadFile(path)
	if err != nil {
		return ops.RuntimeResponseShapeSnapshot{}, fmt.Errorf("read runtime response snapshot file %s: %w", path, err)
	}

	var snapshot ops.RuntimeResponseShapeSnapshot
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&snapshot); err != nil {
		return ops.RuntimeResponseShapeSnapshot{}, fmt.Errorf("decode runtime response snapshot file %s: %w", path, err)
	}
	var trailing struct{}
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return ops.RuntimeResponseShapeSnapshot{}, fmt.Errorf("decode runtime response snapshot file %s: multiple JSON values", path)
		}
		return ops.RuntimeResponseShapeSnapshot{}, fmt.Errorf("decode runtime response snapshot file %s: %w", path, err)
	}
	if err := snapshot.Validate(); err != nil {
		return ops.RuntimeResponseShapeSnapshot{}, fmt.Errorf("validate runtime response snapshot file %s: %w", path, err)
	}

	return snapshot, nil
}

func buildPermissionPreflightSnapshot(profileName string, configPath string, optionalPolicy string) ops.PermissionPreflightSnapshot {
	profileName = strings.TrimSpace(profileName)
	optionalPolicy = ops.NormalizeOptionalModulePolicy(optionalPolicy)
	configPath = strings.TrimSpace(configPath)
	explicitConfigPath := configPath != ""
	if profileName == "" {
		return ops.PermissionPreflightSnapshot{
			Enabled:        false,
			OptionalPolicy: optionalPolicy,
			SkipReason:     "auth profile data not provided",
		}
	}

	if configPath == "" {
		defaultPath, err := config.DefaultPath()
		if err != nil {
			return ops.PermissionPreflightSnapshot{
				Enabled:        true,
				OptionalPolicy: optionalPolicy,
				ProfileName:    profileName,
				LoadError:      err.Error(),
			}
		}
		configPath = defaultPath
	}

	cfg, err := config.Load(configPath)
	if err != nil {
		if !explicitConfigPath && strings.EqualFold(optionalPolicy, ops.OptionalModulePolicySkip) && errors.Is(err, os.ErrNotExist) {
			return ops.PermissionPreflightSnapshot{
				Enabled:        false,
				OptionalPolicy: optionalPolicy,
				ProfileName:    profileName,
				SkipReason:     fmt.Sprintf("config file is not available at %s", configPath),
			}
		}
		return ops.PermissionPreflightSnapshot{
			Enabled:        true,
			OptionalPolicy: optionalPolicy,
			ProfileName:    profileName,
			LoadError:      err.Error(),
		}
	}
	_, profile, err := cfg.ResolveProfile(profileName)
	if err != nil {
		return ops.PermissionPreflightSnapshot{
			Enabled:        true,
			OptionalPolicy: optionalPolicy,
			ProfileName:    profileName,
			LoadError:      err.Error(),
		}
	}

	return ops.PermissionPreflightSnapshot{
		Enabled:        true,
		OptionalPolicy: optionalPolicy,
		ProfileName:    profileName,
		Domain:         profile.Domain,
		GraphVersion:   profile.GraphVersion,
		TokenType:      profile.TokenType,
		BusinessID:     profile.BusinessID,
		AppID:          profile.AppID,
		PageID:         profile.PageID,
		SourceProfile:  profile.SourceProfile,
		TokenRef:       profile.TokenRef,
		AppSecretRef:   profile.AppSecretRef,
	}
}
