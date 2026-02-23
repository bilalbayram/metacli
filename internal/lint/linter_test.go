package lint

import (
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
