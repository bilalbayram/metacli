package cmd

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/bilalbayram/metacli/internal/auth"
	"github.com/bilalbayram/metacli/internal/config"
	"github.com/spf13/cobra"
)

type checkStatus string

const (
	checkPass checkStatus = "pass"
	checkWarn checkStatus = "warn"
	checkFail checkStatus = "fail"
)

type doctorCheck struct {
	Name    string      `json:"name"`
	Status  checkStatus `json:"status"`
	Message string      `json:"message"`
	Profile string      `json:"profile,omitempty"`
}

type doctorCheckResult struct {
	Status  string        `json:"status"`
	Checks  []doctorCheck `json:"checks"`
	Summary doctorSummary `json:"summary"`
}

type doctorSummary struct {
	Total int `json:"total"`
	Pass  int `json:"pass"`
	Warn  int `json:"warn"`
	Fail  int `json:"fail"`
}

type doctorDeps struct {
	configPath   string
	secretStore  auth.SecretStore
	httpClient   auth.HTTPClient
	graphBaseURL string
}

func runDoctorChecks(cmd *cobra.Command, runtime Runtime, deps *doctorDeps) error {
	commandName := "meta doctor"

	configPath := ""
	var secretStore auth.SecretStore
	var httpClient auth.HTTPClient
	graphBaseURL := ""

	if deps != nil {
		configPath = deps.configPath
		secretStore = deps.secretStore
		httpClient = deps.httpClient
		graphBaseURL = deps.graphBaseURL
	}

	if configPath == "" {
		p, err := config.DefaultPath()
		if err != nil {
			checks := []doctorCheck{{
				Name:    "config_file",
				Status:  checkFail,
				Message: fmt.Sprintf("cannot resolve config path: %v", err),
			}}
			return writeSuccess(cmd, runtime, commandName, buildResult(checks), nil, nil)
		}
		configPath = p
	}

	if secretStore == nil {
		secretStore = auth.NewSecretStore()
	}

	var checks []doctorCheck

	// Check 1: Config file
	cfg, err := config.Load(configPath)
	if err != nil {
		checks = append(checks, doctorCheck{
			Name:    "config_file",
			Status:  checkFail,
			Message: fmt.Sprintf("config load failed: %v", err),
		})
		return writeSuccess(cmd, runtime, commandName, buildResult(checks), nil, nil)
	}
	checks = append(checks, doctorCheck{
		Name:    "config_file",
		Status:  checkPass,
		Message: fmt.Sprintf("loaded %s (schema_version=%d)", configPath, cfg.SchemaVersion),
	})

	// Check 2: Profile completeness
	profileFilter := runtime.ProfileName()
	profiles := resolveProfilesToCheck(cfg, profileFilter)

	if profileFilter != "" {
		if _, exists := cfg.Profiles[profileFilter]; !exists {
			checks = append(checks, doctorCheck{
				Name:    "profile_completeness",
				Status:  checkFail,
				Message: fmt.Sprintf("profile %q not found", profileFilter),
				Profile: profileFilter,
			})
			return writeSuccess(cmd, runtime, commandName, buildResult(checks), nil, nil)
		}
	}

	if len(cfg.Profiles) == 0 {
		checks = append(checks, doctorCheck{
			Name:    "profile_completeness",
			Status:  checkWarn,
			Message: "no profiles configured",
		})
	} else {
		if cfg.DefaultProfile == "" {
			checks = append(checks, doctorCheck{
				Name:    "profile_completeness",
				Status:  checkWarn,
				Message: "no default_profile set",
			})
		}

		now := time.Now().UTC()
		for _, name := range profiles {
			profile := cfg.Profiles[name]
			if expiresAt, err := time.Parse(time.RFC3339, profile.ExpiresAt); err == nil {
				if expiresAt.Before(now) {
					checks = append(checks, doctorCheck{
						Name:    "profile_completeness",
						Status:  checkWarn,
						Message: fmt.Sprintf("token expired at %s", profile.ExpiresAt),
						Profile: name,
					})
				} else {
					checks = append(checks, doctorCheck{
						Name:    "profile_completeness",
						Status:  checkPass,
						Message: "profile valid",
						Profile: name,
					})
				}
			} else {
				checks = append(checks, doctorCheck{
					Name:    "profile_completeness",
					Status:  checkPass,
					Message: "profile valid",
					Profile: name,
				})
			}
		}
	}

	// Check 3: Secret store access
	for _, name := range profiles {
		profile := cfg.Profiles[name]
		tokenErr := checkSecretAccess(secretStore, profile.TokenRef)
		provider := config.ResolveProvider(profile.Provider)
		secretErrors := make([]string, 0, 3)
		if tokenErr != nil {
			secretErrors = append(secretErrors, fmt.Sprintf("token_ref: %v", tokenErr))
		}
		switch provider {
		case config.ProviderMeta:
			if err := checkSecretAccess(secretStore, profile.AppSecretRef); err != nil {
				secretErrors = append(secretErrors, fmt.Sprintf("app_secret_ref: %v", err))
			}
		case config.ProviderLinkedIn:
			if err := checkSecretAccess(secretStore, profile.ClientSecretRef); err != nil {
				secretErrors = append(secretErrors, fmt.Sprintf("client_secret_ref: %v", err))
			}
			if err := checkSecretAccess(secretStore, profile.RefreshTokenRef); err != nil {
				secretErrors = append(secretErrors, fmt.Sprintf("refresh_token_ref: %v", err))
			}
		}

		if len(secretErrors) > 0 {
			msg := "secret store access failed:"
			for _, detail := range secretErrors {
				msg += " " + detail
			}
			checks = append(checks, doctorCheck{
				Name:    "secret_store",
				Status:  checkFail,
				Message: msg,
				Profile: name,
			})
		} else {
			checks = append(checks, doctorCheck{
				Name:    "secret_store",
				Status:  checkPass,
				Message: "secrets accessible",
				Profile: name,
			})
		}
	}

	// Check 4: Token validity
	svc := auth.NewService(configPath, secretStore, httpClient, graphBaseURL)
	ctx := context.Background()
	for _, name := range profiles {
		profile := cfg.Profiles[name]
		if config.ResolveProvider(profile.Provider) == config.ProviderLinkedIn {
			status, message := doctorLinkedInTokenStatus(profile)
			checks = append(checks, doctorCheck{
				Name:    "token_validity",
				Status:  status,
				Message: message,
				Profile: name,
			})
			continue
		}

		_, err := svc.ValidateProfile(ctx, name)
		if err != nil {
			checks = append(checks, doctorCheck{
				Name:    "token_validity",
				Status:  checkFail,
				Message: fmt.Sprintf("token validation failed: %v", err),
				Profile: name,
			})
			continue
		}
		checks = append(checks, doctorCheck{
			Name:    "token_validity",
			Status:  checkPass,
			Message: "token valid",
			Profile: name,
		})
	}

	return writeSuccess(cmd, runtime, commandName, buildResult(checks), nil, nil)
}

func resolveProfilesToCheck(cfg *config.Config, filter string) []string {
	if filter != "" {
		if _, exists := cfg.Profiles[filter]; exists {
			return []string{filter}
		}
		return nil
	}
	names := make([]string, 0, len(cfg.Profiles))
	for name := range cfg.Profiles {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func checkSecretAccess(store auth.SecretStore, ref string) error {
	if ref == "" {
		return nil
	}
	_, err := store.Get(ref)
	return err
}

func doctorLinkedInTokenStatus(profile config.Profile) (checkStatus, string) {
	expiresAt, err := time.Parse(time.RFC3339, profile.ExpiresAt)
	if err != nil {
		return checkFail, fmt.Sprintf("linkedin token expiry is invalid: %v", err)
	}

	now := time.Now().UTC()
	if !expiresAt.After(now) {
		return checkFail, fmt.Sprintf("linkedin access token expired at %s", profile.ExpiresAt)
	}
	if expiresAt.Before(now.Add(24 * time.Hour)) {
		return checkWarn, fmt.Sprintf("linkedin access token expires soon at %s", profile.ExpiresAt)
	}
	return checkPass, "linkedin token window looks healthy"
}

func buildResult(checks []doctorCheck) doctorCheckResult {
	summary := doctorSummary{Total: len(checks)}
	for _, c := range checks {
		switch c.Status {
		case checkPass:
			summary.Pass++
		case checkWarn:
			summary.Warn++
		case checkFail:
			summary.Fail++
		}
	}

	status := "healthy"
	if summary.Fail > 0 {
		status = "unhealthy"
	} else if summary.Warn > 0 {
		status = "degraded"
	}

	return doctorCheckResult{
		Status:  status,
		Checks:  checks,
		Summary: summary,
	}
}
