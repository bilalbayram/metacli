package requirements

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/bilalbayram/metacli/internal/schema"
)

func TestResolverMergesSchemaRuntimeRulesAndProfileContext(t *testing.T) {
	t.Parallel()

	schemaPack := &schema.Pack{
		Domain:  "marketing",
		Version: "v25.0",
		EndpointParams: map[string][]string{
			"campaigns.post": {
				"name",
				"objective",
				"status",
				"daily_budget",
				"special_ad_categories",
			},
		},
		EndpointRequiredParams: map[string][]string{
			"campaigns.post": {"name", "daily_budget"},
		},
	}
	rulePack := &RulePack{
		Domain:  "marketing",
		Version: "v25.0",
		Mutations: map[string]MutationRule{
			"campaigns.post": {
				AddRequired:    []string{"objective", "status"},
				RemoveRequired: []string{"daily_budget"},
				InjectDefaults: map[string]string{
					"status":                "PAUSED",
					"special_ad_categories": "[]",
				},
				RequiredScopes: []string{"ads_management"},
				RequiredContext: map[string][]string{
					"*":           {"account_id"},
					"system_user": {"business_id"},
				},
				DriftPolicy: DriftPolicyError,
			},
		},
	}

	resolver, err := NewResolver(schemaPack, rulePack)
	if err != nil {
		t.Fatalf("new resolver: %v", err)
	}

	resolution, err := resolver.Resolve(ResolveInput{
		Mutation: "campaigns.post",
		Payload: map[string]string{
			"name":      "Launch",
			"objective": "OUTCOME_SALES",
		},
		Profile: ProfileContext{
			ProfileName: "prod",
			TokenType:   "system_user",
			Scopes:      []string{"ads_management"},
			BusinessID:  "biz_1",
			AccountID:   "act_123",
		},
	})
	if err != nil {
		t.Fatalf("resolve requirements: %v", err)
	}

	expectedRequired := []string{"name", "objective", "status"}
	if !slices.Equal(resolution.Requirements.Final, expectedRequired) {
		t.Fatalf("unexpected final requirements: got=%v want=%v", resolution.Requirements.Final, expectedRequired)
	}
	if resolution.Payload.Final["status"] != "PAUSED" {
		t.Fatalf("expected resolver to inject status=PAUSED, got %q", resolution.Payload.Final["status"])
	}
	if resolution.Payload.Final["special_ad_categories"] != "[]" {
		t.Fatalf("expected resolver to inject special_ad_categories=[], got %q", resolution.Payload.Final["special_ad_categories"])
	}
	if len(resolution.Drift) != 0 {
		t.Fatalf("expected no drift, got %v", resolution.Drift)
	}
	if len(resolution.Violations) != 0 {
		t.Fatalf("expected no violations, got %v", resolution.Violations)
	}
	if resolution.Blocking {
		t.Fatal("expected non-blocking resolution")
	}
}

func TestResolverReportsStructuredViolationsAndDrift(t *testing.T) {
	t.Parallel()

	schemaPack := &schema.Pack{
		Domain:  "marketing",
		Version: "v25.0",
		EndpointParams: map[string][]string{
			"campaigns.post": {"name", "objective", "status"},
		},
		EndpointRequiredParams: map[string][]string{
			"campaigns.post": {"name"},
		},
	}
	rulePack := &RulePack{
		Domain:  "marketing",
		Version: "v25.0",
		Mutations: map[string]MutationRule{
			"campaigns.post": {
				AddRequired: []string{"objective", "special_ad_categories"},
				InjectDefaults: map[string]string{
					"special_ad_categories": "[]",
				},
				RequiredContext: map[string][]string{
					"system_user": {"business_id"},
				},
				DriftPolicy: DriftPolicyError,
			},
		},
	}

	resolver, err := NewResolver(schemaPack, rulePack)
	if err != nil {
		t.Fatalf("new resolver: %v", err)
	}

	resolution, err := resolver.Resolve(ResolveInput{
		Mutation: "campaigns.post",
		Payload: map[string]string{
			"name": "Launch",
		},
		Profile: ProfileContext{
			TokenType: "system_user",
			AccountID: "act_123",
		},
	})
	if err != nil {
		t.Fatalf("resolve requirements: %v", err)
	}

	if len(resolution.Drift) != 1 {
		t.Fatalf("expected one drift item, got %d", len(resolution.Drift))
	}
	if resolution.Drift[0].Code != "runtime_param_not_in_schema" {
		t.Fatalf("unexpected drift code: %s", resolution.Drift[0].Code)
	}
	if resolution.Drift[0].Severity != SeverityError {
		t.Fatalf("unexpected drift severity: %s", resolution.Drift[0].Severity)
	}

	codes := make([]string, 0, len(resolution.Violations))
	for _, violation := range resolution.Violations {
		codes = append(codes, violation.Code)
	}
	if !slices.Contains(codes, "missing_required_context") {
		t.Fatalf("expected missing_required_context violation, got %v", codes)
	}
	if !slices.Contains(codes, "missing_required_param") {
		t.Fatalf("expected missing_required_param violation, got %v", codes)
	}
	if !slices.Contains(codes, "runtime_schema_drift") {
		t.Fatalf("expected runtime_schema_drift violation, got %v", codes)
	}
	if !resolution.Blocking {
		t.Fatal("expected blocking resolution")
	}
}

func TestLoadRulePackSupportsEmbeddedAndDirectoryOverride(t *testing.T) {
	t.Parallel()

	embeddedPack, err := LoadRulePack("marketing", "v25.0", "")
	if err != nil {
		t.Fatalf("load embedded rule pack: %v", err)
	}
	if _, exists := embeddedPack.Mutations["campaigns.post"]; !exists {
		t.Fatalf("expected embedded campaigns.post rule, got %v", embeddedPack.Mutations)
	}

	rulesDir := t.TempDir()
	marketingDir := filepath.Join(rulesDir, "marketing")
	if err := os.MkdirAll(marketingDir, 0o755); err != nil {
		t.Fatalf("create rules dir: %v", err)
	}
	customPack := `{
  "domain":"marketing",
  "version":"v25.0",
  "mutations":{
    "campaigns.post":{
      "add_required":["name"],
      "drift_policy":"warning"
    }
  }
}`
	if err := os.WriteFile(filepath.Join(marketingDir, "v25.0.json"), []byte(customPack), 0o644); err != nil {
		t.Fatalf("write custom rule pack: %v", err)
	}

	directoryPack, err := LoadRulePack("marketing", "v25.0", rulesDir)
	if err != nil {
		t.Fatalf("load directory rule pack: %v", err)
	}
	if directoryPack.Mutations["campaigns.post"].DriftPolicy != DriftPolicyWarning {
		t.Fatalf("expected directory override to be used, got drift_policy=%q", directoryPack.Mutations["campaigns.post"].DriftPolicy)
	}
}

func TestLoadRulePackFailsClosedWhenMissing(t *testing.T) {
	t.Parallel()

	_, err := LoadRulePack("marketing", "v25.1", "")
	if err == nil {
		t.Fatal("expected missing embedded rule pack to fail")
	}
}
