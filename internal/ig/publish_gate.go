package ig

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/bilalbayram/metacli/internal/config"
	"github.com/bilalbayram/metacli/internal/graph"
)

const (
	publishBindingSourceFlag    = "flag"
	publishBindingSourceProfile = "profile"
)

var igPublishRequiredScopes = []string{
	"instagram_basic",
	"instagram_content_publish",
}

type PublishBindingOptions struct {
	ProfileName       string
	Profile           config.Profile
	RequestedIGUserID string
}

type PublishBindingResolution struct {
	ProfileName string
	PageID      string
	IGUserID    string
	Source      string
}

func ResolvePublishBinding(options PublishBindingOptions) (PublishBindingResolution, error) {
	profileName := strings.TrimSpace(options.ProfileName)
	if profileName == "" {
		return PublishBindingResolution{}, errors.New("profile is required")
	}

	requestedIGUserID := strings.TrimSpace(options.RequestedIGUserID)
	profileIGUserID := strings.TrimSpace(options.Profile.IGUserID)
	pageID := strings.TrimSpace(options.Profile.PageID)

	if requestedIGUserID != "" {
		if profileIGUserID != "" && requestedIGUserID != profileIGUserID {
			return PublishBindingResolution{}, newPublishBindingAmbiguityError(profileName, requestedIGUserID, profileIGUserID)
		}
		return PublishBindingResolution{
			ProfileName: profileName,
			PageID:      pageID,
			IGUserID:    requestedIGUserID,
			Source:      publishBindingSourceFlag,
		}, nil
	}

	if profileIGUserID != "" {
		return PublishBindingResolution{
			ProfileName: profileName,
			PageID:      pageID,
			IGUserID:    profileIGUserID,
			Source:      publishBindingSourceProfile,
		}, nil
	}

	return PublishBindingResolution{}, newPublishBindingMissingError(profileName, pageID)
}

func ValidatePublishCapability(profileName string, profile config.Profile) error {
	resolvedProfile := strings.TrimSpace(profileName)
	if resolvedProfile == "" {
		return errors.New("profile is required")
	}

	authMode := strings.ToLower(strings.TrimSpace(profile.AuthMode))
	if authMode != "" && authMode != "instagram" && authMode != "both" {
		return &graph.APIError{
			Type:      igErrorTypeCapabilityGate,
			Code:      igErrorCodeCapabilityGate,
			Message:   fmt.Sprintf("profile %q auth_mode=%q does not support instagram publishing", resolvedProfile, profile.AuthMode),
			Retryable: false,
			Remediation: newIGRemediation(
				graph.RemediationCategoryPermission,
				"Profile auth mode does not grant Instagram publishing capability.",
				"Run `meta auth setup --profile <name> --mode instagram` or `--mode both`.",
				"Validate capability with `meta auth validate --profile <name> --require-scopes instagram_basic,instagram_content_publish`.",
			),
		}
	}

	if !hasScopeMetadata(profile.Scopes) {
		return nil
	}

	missingScopes := missingPublishScopes(profile.Scopes)
	if len(missingScopes) == 0 {
		return nil
	}

	return &graph.APIError{
		Type:      igErrorTypeCapabilityGate,
		Code:      igErrorCodeCapabilityGate,
		Message:   fmt.Sprintf("profile %q is missing instagram publish scopes: %s", resolvedProfile, strings.Join(missingScopes, ",")),
		Retryable: false,
		Remediation: newIGRemediation(
			graph.RemediationCategoryPermission,
			"Profile scope metadata is missing required Instagram publish scopes.",
			"Run `meta auth setup --profile <name> --scope-pack ig_publish` or include the missing scopes explicitly.",
			"Validate profile token with `meta auth validate --profile <name> --require-scopes instagram_basic,instagram_content_publish`.",
		),
	}
}

func NormalizePublishPreflightError(err error) error {
	if err == nil {
		return nil
	}

	var apiErr *graph.APIError
	if errors.As(err, &apiErr) {
		return err
	}

	message := strings.TrimSpace(err.Error())
	if !strings.Contains(strings.ToLower(message), "auth preflight failed") {
		return err
	}

	category, summary, actions := classifyPublishPreflightRemediation(message)
	return &graph.APIError{
		Type:      igErrorTypePreflightGate,
		Code:      igErrorCodePreflightGate,
		Message:   message,
		Retryable: false,
		Remediation: newIGRemediation(
			category,
			summary,
			actions...,
		),
	}
}

func RequiredPublishScopes() []string {
	out := make([]string, len(igPublishRequiredScopes))
	copy(out, igPublishRequiredScopes)
	return out
}

func classifyPublishPreflightRemediation(message string) (string, string, []string) {
	normalized := strings.ToLower(strings.TrimSpace(message))

	if strings.Contains(normalized, "missing required scopes") {
		return graph.RemediationCategoryPermission,
			"Profile token is missing required Instagram publish scopes.",
			[]string{
				"Run `meta auth setup --profile <name> --scope-pack ig_publish` and re-authorize.",
				"Verify token scopes with `meta auth validate --profile <name> --require-scopes instagram_basic,instagram_content_publish`.",
			}
	}

	if strings.Contains(normalized, "ttl is below minimum ttl") || strings.Contains(normalized, "expired") || strings.Contains(normalized, "invalid") {
		return graph.RemediationCategoryAuth,
			"Profile token failed authentication preflight for Instagram publishing.",
			[]string{
				"Run `meta auth validate --profile <name>` to inspect token health.",
				"Refresh credentials with `meta auth setup` and retry.",
			}
	}

	return graph.RemediationCategoryAuth,
		"Instagram publish preflight failed for the selected profile.",
		[]string{
			"Run `meta auth validate --profile <name>` and fix reported violations before retrying.",
		}
}

func hasScopeMetadata(scopes []string) bool {
	for _, scope := range scopes {
		if strings.TrimSpace(scope) != "" {
			return true
		}
	}
	return false
}

func missingPublishScopes(scopes []string) []string {
	granted := map[string]struct{}{}
	for _, scope := range scopes {
		normalized := strings.ToLower(strings.TrimSpace(scope))
		if normalized == "" {
			continue
		}
		granted[normalized] = struct{}{}
	}

	missing := make([]string, 0)
	for _, required := range igPublishRequiredScopes {
		if _, ok := granted[required]; ok {
			continue
		}
		missing = append(missing, required)
	}
	sort.Strings(missing)
	return missing
}

func newPublishBindingMissingError(profileName string, pageID string) *graph.APIError {
	message := fmt.Sprintf("ig user binding is required for profile %q: configure profile ig_user_id or pass --ig-user-id", profileName)
	actions := []string{
		"Run `meta auth discover --profile <name> --mode ig` to list available Instagram bindings.",
		"Persist a default with `meta auth setup --profile <name> --ig-user-id <IG_USER_ID>`.",
		"Or pass `--ig-user-id <IG_USER_ID>` directly on the publish command.",
	}
	if pageID != "" {
		message = fmt.Sprintf("ig user binding is required for profile %q: page_id %q is set but ig_user_id is missing", profileName, pageID)
	}

	return &graph.APIError{
		Type:      igErrorTypeBindingResolution,
		Code:      igErrorCodeBindingResolution,
		Message:   message,
		Retryable: false,
		Remediation: newIGRemediation(
			graph.RemediationCategoryValidation,
			"Instagram publish requires an ig_user_id binding.",
			actions...,
		),
	}
}

func newPublishBindingAmbiguityError(profileName string, requestedIGUserID string, profileIGUserID string) *graph.APIError {
	return &graph.APIError{
		Type:      igErrorTypeBindingResolution,
		Code:      igErrorCodeBindingResolution,
		Message:   fmt.Sprintf("ambiguous ig user binding for profile %q: --ig-user-id=%q conflicts with profile ig_user_id=%q", profileName, requestedIGUserID, profileIGUserID),
		Retryable: false,
		Remediation: newIGRemediation(
			graph.RemediationCategoryConflict,
			"Instagram publish binding is ambiguous.",
			"Use one ig_user_id value for this profile context before retrying.",
			"Align profile binding via `meta auth setup --profile <name> --ig-user-id <IG_USER_ID>` or remove the conflicting flag override.",
		),
	}
}
