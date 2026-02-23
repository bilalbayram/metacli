package ig

import (
	"strings"
	"testing"
)

func TestValidateCaptionRejectsEmptyCaption(t *testing.T) {
	t.Parallel()

	result := ValidateCaption("   ", false)
	if result.Valid {
		t.Fatal("expected invalid caption")
	}
	if len(result.Errors) != 1 || result.Errors[0] != "caption is required" {
		t.Fatalf("unexpected errors: %#v", result.Errors)
	}
}

func TestValidateCaptionWarnsNearLimitInNonStrictMode(t *testing.T) {
	t.Parallel()

	result := ValidateCaption(strings.Repeat("a", CaptionWarningCharacters+5), false)
	if !result.Valid {
		t.Fatalf("expected non-strict validation to stay valid, got errors: %#v", result.Errors)
	}
	if len(result.Warnings) == 0 {
		t.Fatal("expected warning for near-limit caption")
	}
	if result.Errors == nil || len(result.Errors) != 0 {
		t.Fatalf("expected no errors, got %#v", result.Errors)
	}
}

func TestValidateCaptionStrictModePromotesWarningsToErrors(t *testing.T) {
	t.Parallel()

	result := ValidateCaption(strings.Repeat("a", CaptionWarningCharacters+5), true)
	if result.Valid {
		t.Fatal("expected strict mode to fail on warnings")
	}
	if len(result.Errors) == 0 {
		t.Fatal("expected strict mode errors")
	}
	if !strings.Contains(result.Errors[0], "strict mode:") {
		t.Fatalf("expected strict mode prefix in error, got %q", result.Errors[0])
	}
	if len(result.Warnings) != 0 {
		t.Fatalf("expected strict mode warnings to be promoted, got %#v", result.Warnings)
	}
}

func TestValidateCaptionRejectsTooManyHashtags(t *testing.T) {
	t.Parallel()

	tokens := make([]string, 0, MaxCaptionHashtags+1)
	for idx := 0; idx < MaxCaptionHashtags+1; idx++ {
		tokens = append(tokens, "#tag")
	}
	caption := strings.Join(tokens, " ")

	result := ValidateCaption(caption, false)
	if result.Valid {
		t.Fatal("expected invalid caption")
	}
	if len(result.Errors) == 0 {
		t.Fatal("expected hashtag limit error")
	}
	if !strings.Contains(result.Errors[0], "exceeds 30 hashtags") {
		t.Fatalf("unexpected errors: %#v", result.Errors)
	}
}
