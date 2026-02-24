package ig

import (
	"errors"
	"testing"

	"github.com/bilalbayram/metacli/internal/config"
	"github.com/bilalbayram/metacli/internal/graph"
)

func TestResolvePublishBindingUsesProfileDefault(t *testing.T) {
	t.Parallel()

	resolution, err := ResolvePublishBinding(PublishBindingOptions{
		ProfileName: "prod",
		Profile: config.Profile{
			PageID:   "123",
			IGUserID: "17841400008461234",
		},
	})
	if err != nil {
		t.Fatalf("resolve publish binding: %v", err)
	}
	if resolution.IGUserID != "17841400008461234" {
		t.Fatalf("unexpected ig user id %q", resolution.IGUserID)
	}
	if resolution.Source != publishBindingSourceProfile {
		t.Fatalf("unexpected binding source %q", resolution.Source)
	}
}

func TestResolvePublishBindingRejectsAmbiguousOverride(t *testing.T) {
	t.Parallel()

	_, err := ResolvePublishBinding(PublishBindingOptions{
		ProfileName:       "prod",
		RequestedIGUserID: "17841400008460056",
		Profile: config.Profile{
			IGUserID: "17841400008461234",
		},
	})
	if err == nil {
		t.Fatal("expected ambiguity error")
	}

	var apiErr *graph.APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("expected APIError, got %T", err)
	}
	if apiErr.Type != igErrorTypeBindingResolution {
		t.Fatalf("unexpected error type %q", apiErr.Type)
	}
	if apiErr.Remediation == nil || apiErr.Remediation.Category != graph.RemediationCategoryConflict {
		t.Fatalf("unexpected remediation %+v", apiErr.Remediation)
	}
}

func TestResolvePublishBindingRejectsMissingBinding(t *testing.T) {
	t.Parallel()

	_, err := ResolvePublishBinding(PublishBindingOptions{
		ProfileName: "prod",
		Profile: config.Profile{
			PageID: "123",
		},
	})
	if err == nil {
		t.Fatal("expected missing binding error")
	}

	var apiErr *graph.APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("expected APIError, got %T", err)
	}
	if apiErr.Type != igErrorTypeBindingResolution {
		t.Fatalf("unexpected error type %q", apiErr.Type)
	}
	if apiErr.Remediation == nil || apiErr.Remediation.Category != graph.RemediationCategoryValidation {
		t.Fatalf("unexpected remediation %+v", apiErr.Remediation)
	}
}

func TestValidatePublishCapabilityRejectsFacebookMode(t *testing.T) {
	t.Parallel()

	err := ValidatePublishCapability("prod", config.Profile{
		AuthMode: "facebook",
		Scopes:   []string{"instagram_basic", "instagram_content_publish"},
	})
	if err == nil {
		t.Fatal("expected capability gate error")
	}

	var apiErr *graph.APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("expected APIError, got %T", err)
	}
	if apiErr.Type != igErrorTypeCapabilityGate {
		t.Fatalf("unexpected error type %q", apiErr.Type)
	}
	if apiErr.Remediation == nil || apiErr.Remediation.Category != graph.RemediationCategoryPermission {
		t.Fatalf("unexpected remediation %+v", apiErr.Remediation)
	}
}

func TestValidatePublishCapabilityRejectsMissingScopes(t *testing.T) {
	t.Parallel()

	err := ValidatePublishCapability("prod", config.Profile{
		AuthMode: "instagram",
		Scopes:   []string{"instagram_basic"},
	})
	if err == nil {
		t.Fatal("expected missing scopes gate error")
	}

	var apiErr *graph.APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("expected APIError, got %T", err)
	}
	if apiErr.Type != igErrorTypeCapabilityGate {
		t.Fatalf("unexpected error type %q", apiErr.Type)
	}
	if apiErr.Remediation == nil || apiErr.Remediation.Category != graph.RemediationCategoryPermission {
		t.Fatalf("unexpected remediation %+v", apiErr.Remediation)
	}
}

func TestNormalizePublishPreflightErrorProducesStructuredRemediation(t *testing.T) {
	t.Parallel()

	err := NormalizePublishPreflightError(errors.New("auth preflight failed for profile \"prod\": profile token is missing required scopes for profile \"prod\": instagram_content_publish"))
	var apiErr *graph.APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("expected APIError, got %T", err)
	}
	if apiErr.Type != igErrorTypePreflightGate {
		t.Fatalf("unexpected error type %q", apiErr.Type)
	}
	if apiErr.Remediation == nil || apiErr.Remediation.Category != graph.RemediationCategoryPermission {
		t.Fatalf("unexpected remediation %+v", apiErr.Remediation)
	}
}
