package cmd

import (
	"errors"
	"fmt"
	"strings"

	"github.com/bilalbayram/metacli/internal/config"
	"github.com/bilalbayram/metacli/internal/graph"
	"github.com/bilalbayram/metacli/internal/smoke"
	"github.com/spf13/cobra"
)

var (
	smokeLoadProfileCredentials = loadProfileCredentials
	smokeNewGraphClient         = func() *graph.Client {
		return graph.NewClient(nil, "")
	}
	smokeNewRunner = func(client smoke.GraphClient) *smoke.Runner {
		return smoke.NewRunner(client)
	}
)

func NewSmokeCommand(runtime Runtime) *cobra.Command {
	smokeCmd := &cobra.Command{
		Use:           "smoke",
		Short:         "Deterministic capability-aware smoke workflows",
		SilenceErrors: true,
		SilenceUsage:  true,
	}
	smokeCmd.AddCommand(newSmokeRunCommand(runtime))
	return smokeCmd
}

func newSmokeRunCommand(runtime Runtime) *cobra.Command {
	var (
		profile        string
		version        string
		accountID      string
		catalogID      string
		optionalPolicy string
	)

	cmd := &cobra.Command{
		Use:   "run",
		Short: "Run smoke runner v2 with account-aware reporting",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := ensureSmokeOutput(runtime); err != nil {
				return writeSmokeError(cmd, runtime, smoke.CommandRun, smoke.WrapExit(smoke.ExitCodeInput, err))
			}
			if err := smoke.ValidateOptionalPolicy(optionalPolicy); err != nil {
				return writeSmokeError(cmd, runtime, smoke.CommandRun, smoke.WrapExit(smoke.ExitCodeInput, err))
			}

			creds, resolvedVersion, err := resolveSmokeProfileAndVersion(runtime, profile, version)
			if err != nil {
				return writeSmokeError(cmd, runtime, smoke.CommandRun, smoke.WrapExit(smoke.ExitCodeInput, err))
			}

			runner := smokeNewRunner(smokeNewGraphClient())
			result, err := runner.Run(cmd.Context(), smoke.RunInput{
				ProfileName:    creds.Name,
				Version:        resolvedVersion,
				AccountID:      accountID,
				Token:          creds.Token,
				AppSecret:      creds.AppSecret,
				OptionalPolicy: optionalPolicy,
				CatalogID:      catalogID,
			})
			if err != nil {
				code := smoke.ExitCodeRuntime
				switch {
				case errors.Is(err, smoke.ErrAccountIDMissing):
					code = smoke.ExitCodeInput
				case errors.Is(err, smoke.ErrTokenRequired):
					code = smoke.ExitCodeInput
				case errors.Is(err, smoke.ErrVersionRequired):
					code = smoke.ExitCodeInput
				case errors.Is(err, smoke.ErrInvalidOptionalPolicy):
					code = smoke.ExitCodeInput
				}
				return writeSmokeError(cmd, runtime, smoke.CommandRun, smoke.WrapExit(code, err))
			}

			for _, resource := range result.Report.CreatedResources {
				if err := persistTrackedResource(trackedResourceInput{
					Command:       resource.Command,
					ResourceKind:  resource.ResourceKind,
					ResourceID:    resource.ResourceID,
					CleanupAction: resource.CleanupAction,
					Profile:       creds.Name,
					GraphVersion:  resolvedVersion,
					AccountID:     resource.AccountID,
					Metadata: map[string]string{
						"step": resource.Step,
					},
				}); err != nil {
					return writeSmokeError(cmd, runtime, smoke.CommandRun, smoke.WrapExit(smoke.ExitCodeRuntime, err))
				}
			}

			envelope := smoke.NewSuccessEnvelope(smoke.CommandRun, result)
			if code := smoke.RunExitCode(result.Report); code != smoke.ExitCodeSuccess {
				envelope.Success = false
				envelope.ExitCode = code
				switch smoke.RunOutcomeForReport(result.Report) {
				case smoke.RunOutcomeWarning:
					envelope.Error = &smoke.ErrorInfo{
						Type:    "warning_findings",
						Message: fmt.Sprintf("smoke run reported %d warning finding(s)", result.Report.Summary.Warnings),
					}
				case smoke.RunOutcomeBlocking:
					envelope.Error = &smoke.ErrorInfo{
						Type:    "blocking_findings",
						Message: fmt.Sprintf("smoke run reported %d blocking finding(s)", result.Report.Summary.Blocking),
					}
				default:
					envelope.Error = &smoke.ErrorInfo{
						Type:    "runtime_error",
						Message: "smoke run failed with runtime error",
					}
				}
			}

			if err := smoke.WriteEnvelope(cmd.OutOrStdout(), selectedOutputFormat(runtime), envelope); err != nil {
				return writeSmokeError(cmd, runtime, smoke.CommandRun, smoke.WrapExit(smoke.ExitCodeUnknown, fmt.Errorf("write smoke envelope: %w", err)))
			}
			if !envelope.Success {
				return smoke.WrapExit(envelope.ExitCode, errors.New(envelope.Error.Message))
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&profile, "profile", "", "Profile name")
	cmd.Flags().StringVar(&version, "version", "", "Graph API version")
	cmd.Flags().StringVar(&accountID, "account-id", "", "Ad account id (with or without act_ prefix)")
	cmd.Flags().StringVar(&catalogID, "catalog-id", "", "Catalog id for optional catalog smoke step")
	cmd.Flags().StringVar(&optionalPolicy, "optional-policy", smoke.OptionalPolicyStrict, "Policy for optional smoke modules: strict|skip")
	mustMarkFlagRequired(cmd, "account-id")
	return cmd
}

func ensureSmokeOutput(runtime Runtime) error {
	format := strings.ToLower(strings.TrimSpace(selectedOutputFormat(runtime)))
	if format != "json" {
		return fmt.Errorf("meta smoke run requires --output json, got %q", format)
	}
	return nil
}

func resolveSmokeProfileAndVersion(runtime Runtime, profile string, version string) (*ProfileCredentials, string, error) {
	resolvedProfile := strings.TrimSpace(profile)
	if resolvedProfile == "" {
		resolvedProfile = runtime.ProfileName()
	}
	if resolvedProfile == "" {
		return nil, "", errors.New("profile is required (--profile or global --profile)")
	}

	creds, err := smokeLoadProfileCredentials(resolvedProfile)
	if err != nil {
		return nil, "", err
	}

	resolvedVersion := strings.TrimSpace(version)
	if resolvedVersion == "" {
		resolvedVersion = strings.TrimSpace(creds.Profile.GraphVersion)
	}
	if resolvedVersion == "" {
		resolvedVersion = config.DefaultGraphVersion
	}
	if resolvedVersion == "" {
		return nil, "", errors.New("graph version is required")
	}
	return creds, resolvedVersion, nil
}

func writeSmokeError(cmd *cobra.Command, runtime Runtime, command string, err error) error {
	code := smoke.ExitCode(err)
	if code == smoke.ExitCodeSuccess {
		code = smoke.ExitCodeUnknown
	}

	envelope := smoke.NewErrorEnvelope(command, code, err)
	if writeErr := smoke.WriteEnvelope(cmd.ErrOrStderr(), selectedOutputFormat(runtime), envelope); writeErr != nil {
		return smoke.WrapExit(code, fmt.Errorf("%w (secondary output error: %v)", err, writeErr))
	}

	var exitErr *smoke.ExitError
	if errors.As(err, &exitErr) {
		return err
	}
	return smoke.WrapExit(code, err)
}
