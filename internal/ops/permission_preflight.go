package ops

import (
	"fmt"
	"strings"

	"github.com/bilalbayram/metacli/internal/auth"
)

type PermissionPreflightSnapshot struct {
	Enabled        bool   `json:"enabled"`
	OptionalPolicy string `json:"optional_policy,omitempty"`
	SkipReason     string `json:"skip_reason,omitempty"`
	ProfileName    string `json:"profile_name,omitempty"`
	Domain         string `json:"domain,omitempty"`
	GraphVersion   string `json:"graph_version,omitempty"`
	TokenType      string `json:"token_type,omitempty"`
	BusinessID     string `json:"business_id,omitempty"`
	AppID          string `json:"app_id,omitempty"`
	PageID         string `json:"page_id,omitempty"`
	SourceProfile  string `json:"source_profile,omitempty"`
	TokenRef       string `json:"token_ref,omitempty"`
	AppSecretRef   string `json:"app_secret_ref,omitempty"`
	LoadError      string `json:"load_error,omitempty"`
}

func evaluatePermissionPolicyPreflight(snapshot PermissionPreflightSnapshot) Check {
	check := Check{
		Name:   checkNamePermissionPolicyPreflight,
		Status: CheckStatusPass,
	}

	optionalPolicy := NormalizeOptionalModulePolicy(snapshot.OptionalPolicy)
	if optionalPolicy == "" {
		optionalPolicy = OptionalModulePolicySkip
	}

	if !snapshot.Enabled {
		skipReason := strings.TrimSpace(snapshot.SkipReason)
		if skipReason == "" {
			skipReason = "auth profile data not provided"
		}
		if optionalPolicy == OptionalModulePolicyStrict {
			check.Status = CheckStatusFail
			check.Blocking = true
			check.Message = fmt.Sprintf("preflight failed: optional module unavailable under policy=%s: %s", optionalPolicy, skipReason)
			return check
		}
		check.Message = fmt.Sprintf("preflight skipped: %s (policy=%s)", skipReason, optionalPolicy)
		return check
	}

	if strings.TrimSpace(snapshot.LoadError) != "" {
		check.Status = CheckStatusFail
		check.Blocking = true
		check.Message = fmt.Sprintf("preflight failed: %s", snapshot.LoadError)
		return check
	}

	violations := make([]string, 0)
	if strings.TrimSpace(snapshot.ProfileName) == "" {
		violations = append(violations, "profile_name is required")
	}
	if strings.TrimSpace(snapshot.Domain) == "" {
		violations = append(violations, "domain is required")
	}
	if strings.TrimSpace(snapshot.GraphVersion) == "" {
		violations = append(violations, "graph_version is required")
	}
	if strings.TrimSpace(snapshot.TokenType) == "" {
		violations = append(violations, "token_type is required")
	}
	if strings.TrimSpace(snapshot.TokenRef) == "" {
		violations = append(violations, "token_ref is required")
	} else if _, _, err := auth.ParseSecretRef(snapshot.TokenRef); err != nil {
		violations = append(violations, fmt.Sprintf("token_ref is invalid: %v", err))
	}

	requiredScopes, accessRequirements := requiredPolicyForTokenType(snapshot.TokenType)
	if len(requiredScopes) == 0 {
		violations = append(violations, fmt.Sprintf("token_type %q is unsupported", snapshot.TokenType))
	}

	switch snapshot.TokenType {
	case auth.TokenTypeSystemUser:
		if strings.TrimSpace(snapshot.BusinessID) == "" {
			violations = append(violations, "business_id is required for system_user token_type")
		}
		if strings.TrimSpace(snapshot.AppID) == "" {
			violations = append(violations, "app_id is required for system_user token_type")
		}
	case auth.TokenTypeUser:
		if strings.TrimSpace(snapshot.AppID) == "" {
			violations = append(violations, "app_id is required for user token_type")
		}
	case auth.TokenTypePage:
		if strings.TrimSpace(snapshot.PageID) == "" {
			violations = append(violations, "page_id is required for page token_type")
		}
		if strings.TrimSpace(snapshot.SourceProfile) == "" {
			violations = append(violations, "source_profile is required for page token_type")
		}
	case auth.TokenTypeApp:
		if strings.TrimSpace(snapshot.AppID) == "" {
			violations = append(violations, "app_id is required for app token_type")
		}
		if strings.TrimSpace(snapshot.AppSecretRef) == "" {
			violations = append(violations, "app_secret_ref is required for app token_type")
		} else if _, _, err := auth.ParseSecretRef(snapshot.AppSecretRef); err != nil {
			violations = append(violations, fmt.Sprintf("app_secret_ref is invalid: %v", err))
		}
	}

	if len(violations) > 0 {
		check.Status = CheckStatusFail
		check.Blocking = true
		check.Message = fmt.Sprintf("preflight failed: %s", strings.Join(violations, "; "))
		return check
	}

	check.Message = fmt.Sprintf(
		"preflight passed: profile=%s token_type=%s required_scopes=%s access_requirements=%s",
		snapshot.ProfileName,
		snapshot.TokenType,
		strings.Join(requiredScopes, ","),
		strings.Join(accessRequirements, ","),
	)
	return check
}

func requiredPolicyForTokenType(tokenType string) ([]string, []string) {
	switch tokenType {
	case auth.TokenTypeSystemUser:
		return []string{"ads_management", "business_management"}, []string{"business_id", "app_id"}
	case auth.TokenTypeUser:
		return []string{"ads_read"}, []string{"app_id"}
	case auth.TokenTypePage:
		return []string{"pages_read_engagement"}, []string{"page_id", "source_profile"}
	case auth.TokenTypeApp:
		return []string{"ads_management"}, []string{"app_id", "app_secret_ref"}
	default:
		return nil, nil
	}
}
