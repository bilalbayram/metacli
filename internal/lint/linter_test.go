package lint

import (
	"strings"
	"testing"

	"github.com/bilalbayram/metacli/internal/schema"
)

func TestLintStrictFailsDeprecatedAndUnknown(t *testing.T) {
	t.Parallel()

	linter, err := New(&schema.Pack{
		Domain:  "marketing",
		Version: "v25.0",
		Entities: map[string][]string{
			"insights": {"impressions", "clicks"},
		},
		EndpointParams: map[string][]string{
			"insights": {"level", "date_preset"},
		},
		DeprecatedParams: map[string][]string{
			"insights": {"legacy_param"},
		},
	})
	if err != nil {
		t.Fatalf("new linter: %v", err)
	}

	result := linter.Lint(&RequestSpec{
		Method: "GET",
		Path:   "/act_1/insights",
		Params: map[string]string{
			"legacy_param": "1",
			"unknown":      "1",
		},
		Fields: []string{"impressions", "unknown_field"},
	}, true)

	if len(result.Errors) != 3 {
		t.Fatalf("expected 3 errors, got %d (%v)", len(result.Errors), result.Errors)
	}
}

func TestLintNonStrictProducesWarnings(t *testing.T) {
	t.Parallel()

	linter, err := New(&schema.Pack{
		Domain:  "marketing",
		Version: "v25.0",
		Entities: map[string][]string{
			"campaign": {"id", "name"},
		},
		EndpointParams: map[string][]string{
			"campaigns": {"fields"},
		},
	})
	if err != nil {
		t.Fatalf("new linter: %v", err)
	}

	result := linter.Lint(&RequestSpec{
		Method: "GET",
		Path:   "/act_1/campaigns",
		Params: map[string]string{
			"bad_param": "1",
		},
		Fields: []string{"id", "bad_field"},
	}, false)

	if len(result.Errors) != 0 {
		t.Fatalf("expected no errors, got %v", result.Errors)
	}
	if len(result.Warnings) != 2 {
		t.Fatalf("expected 2 warnings, got %d", len(result.Warnings))
	}
}

func TestLintStrictMutationAllowsKnownParams(t *testing.T) {
	t.Parallel()

	linter, err := New(&schema.Pack{
		Domain:  "marketing",
		Version: "v25.0",
		EndpointParams: map[string][]string{
			"campaigns.post":        {"name", "status", "objective"},
			"adsets.post":           {"name", "campaign_id", "billing_event"},
			"ads.post":              {"name", "adset_id", "creative"},
			"adcreatives.post":      {"name", "object_story_spec"},
			"customaudiences.post":  {"name", "subtype", "customer_file_source"},
			"product_catalogs.post": {"name", "vertical"},
		},
	})
	if err != nil {
		t.Fatalf("new linter: %v", err)
	}

	tests := []struct {
		name   string
		path   string
		params map[string]string
	}{
		{
			name: "campaign",
			path: "/act_1/campaigns",
			params: map[string]string{
				"name":      "Campaign A",
				"status":    "PAUSED",
				"objective": "OUTCOME_AWARENESS",
			},
		},
		{
			name: "adset",
			path: "/act_1/adsets",
			params: map[string]string{
				"name":          "Adset A",
				"campaign_id":   "123",
				"billing_event": "IMPRESSIONS",
			},
		},
		{
			name: "ad",
			path: "/act_1/ads",
			params: map[string]string{
				"name":     "Ad A",
				"adset_id": "123",
				"creative": `{"creative_id":"456"}`,
			},
		},
		{
			name: "creative",
			path: "/act_1/adcreatives",
			params: map[string]string{
				"name":              "Creative A",
				"object_story_spec": `{"page_id":"1"}`,
			},
		},
		{
			name: "audience",
			path: "/act_1/customaudiences",
			params: map[string]string{
				"name":                 "Audience A",
				"subtype":              "CUSTOM",
				"customer_file_source": "USER_PROVIDED_ONLY",
			},
		},
		{
			name: "catalog",
			path: "/act_1/product_catalogs",
			params: map[string]string{
				"name":     "Catalog A",
				"vertical": "commerce",
			},
		},
	}

	for _, testCase := range tests {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			result := linter.Lint(&RequestSpec{
				Method: "POST",
				Path:   testCase.path,
				Params: testCase.params,
			}, true)

			if len(result.Errors) != 0 {
				t.Fatalf("expected no errors, got %v", result.Errors)
			}
			if len(result.Warnings) != 0 {
				t.Fatalf("expected no warnings, got %v", result.Warnings)
			}
		})
	}
}

func TestLintStrictMutationRejectsUnknownAndDeprecatedParams(t *testing.T) {
	t.Parallel()

	linter, err := New(&schema.Pack{
		Domain:  "marketing",
		Version: "v25.0",
		EndpointParams: map[string][]string{
			"campaigns.post": {"name", "status"},
		},
		DeprecatedParams: map[string][]string{
			"campaigns.post": {"legacy_param"},
		},
	})
	if err != nil {
		t.Fatalf("new linter: %v", err)
	}

	result := linter.Lint(&RequestSpec{
		Method: "POST",
		Path:   "/act_1/campaigns",
		Params: map[string]string{
			"name":         "Campaign A",
			"legacy_param": "1",
			"unexpected":   "1",
		},
	}, true)

	if len(result.Errors) != 2 {
		t.Fatalf("expected 2 errors, got %d (%v)", len(result.Errors), result.Errors)
	}
	if !hasMessageContaining(result.Errors, `deprecated param "legacy_param"`) {
		t.Fatalf("expected deprecated param error, got %v", result.Errors)
	}
	if !hasMessageContaining(result.Errors, `unknown param "unexpected"`) {
		t.Fatalf("expected unknown param error, got %v", result.Errors)
	}
}

func TestLintNonStrictMutationWarnsOnUnknownParam(t *testing.T) {
	t.Parallel()

	linter, err := New(&schema.Pack{
		Domain:  "marketing",
		Version: "v25.0",
		EndpointParams: map[string][]string{
			"ads.post": {"name", "adset_id", "creative"},
		},
	})
	if err != nil {
		t.Fatalf("new linter: %v", err)
	}

	result := linter.Lint(&RequestSpec{
		Method: "POST",
		Path:   "/act_1/ads",
		Params: map[string]string{
			"name":      "Ad A",
			"adset_id":  "123",
			"creative":  `{"creative_id":"456"}`,
			"bad_param": "1",
		},
	}, false)

	if len(result.Errors) != 0 {
		t.Fatalf("expected no errors, got %v", result.Errors)
	}
	if len(result.Warnings) != 1 {
		t.Fatalf("expected 1 warning, got %d (%v)", len(result.Warnings), result.Warnings)
	}
	if !hasMessageContaining(result.Warnings, `unknown param "bad_param"`) {
		t.Fatalf("expected unknown param warning, got %v", result.Warnings)
	}
}

func hasMessageContaining(messages []string, needle string) bool {
	for _, message := range messages {
		if strings.Contains(message, needle) {
			return true
		}
	}
	return false
}
