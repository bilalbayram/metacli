package smoke

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bilalbayram/metacli/internal/graph"
)

type failureSignatureFixture struct {
	Name              string                 `json:"name"`
	Step              string                 `json:"step"`
	Optional          bool                   `json:"optional"`
	Blocking          bool                   `json:"blocking"`
	Error             failureErrorFixture    `json:"error"`
	Expected          failureExpectedFixture `json:"expected"`
	ExpectedSignature string                 `json:"expected_signature"`
}

type failureErrorFixture struct {
	Kind       string `json:"kind"`
	Type       string `json:"type,omitempty"`
	Message    string `json:"message"`
	StatusCode int    `json:"status_code,omitempty"`
	Code       int    `json:"code,omitempty"`
	Subcode    int    `json:"subcode,omitempty"`
	FBTraceID  string `json:"fbtrace_id,omitempty"`
}

type failureExpectedFixture struct {
	Type         string `json:"type"`
	Message      string `json:"message"`
	StatusCode   int    `json:"status_code,omitempty"`
	Code         int    `json:"code,omitempty"`
	ErrorSubcode int    `json:"error_subcode,omitempty"`
}

func TestFailureFromErrorRegressionFixtures(t *testing.T) {
	t.Parallel()

	fixtures := loadFailureSignatureFixtures(t)
	for _, fixture := range fixtures {
		fixture := fixture
		t.Run(fixture.Name, func(t *testing.T) {
			t.Parallel()

			err := fixtureErr(fixture.Error)
			got := failureFromError(fixture.Step, fixture.Optional, fixture.Blocking, err)

			if got.Step != fixture.Step {
				t.Fatalf("unexpected step: got=%s want=%s", got.Step, fixture.Step)
			}
			if got.Optional != fixture.Optional {
				t.Fatalf("unexpected optional: got=%t want=%t", got.Optional, fixture.Optional)
			}
			if got.Blocking != fixture.Blocking {
				t.Fatalf("unexpected blocking: got=%t want=%t", got.Blocking, fixture.Blocking)
			}
			if got.Type != fixture.Expected.Type {
				t.Fatalf("unexpected failure type: got=%s want=%s", got.Type, fixture.Expected.Type)
			}
			if got.Message != fixture.Expected.Message {
				t.Fatalf("unexpected failure message: got=%q want=%q", got.Message, fixture.Expected.Message)
			}
			if got.StatusCode != fixture.Expected.StatusCode {
				t.Fatalf("unexpected status_code: got=%d want=%d", got.StatusCode, fixture.Expected.StatusCode)
			}
			if got.Code != fixture.Expected.Code {
				t.Fatalf("unexpected code: got=%d want=%d", got.Code, fixture.Expected.Code)
			}
			if got.ErrorSubcode != fixture.Expected.ErrorSubcode {
				t.Fatalf("unexpected error_subcode: got=%d want=%d", got.ErrorSubcode, fixture.Expected.ErrorSubcode)
			}

			signature := failureSignature(got)
			if signature != fixture.ExpectedSignature {
				t.Fatalf("unexpected failure signature: got=%q want=%q", signature, fixture.ExpectedSignature)
			}
		})
	}
}

func loadFailureSignatureFixtures(t *testing.T) []failureSignatureFixture {
	t.Helper()

	path := filepath.Join("testdata", "failure-signatures.json")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read failure signature fixtures %s: %v", path, err)
	}

	var fixtures []failureSignatureFixture
	if err := json.Unmarshal(raw, &fixtures); err != nil {
		t.Fatalf("decode failure signature fixtures %s: %v", path, err)
	}
	if len(fixtures) == 0 {
		t.Fatalf("failure signature fixtures %s are empty", path)
	}
	return fixtures
}

func fixtureErr(definition failureErrorFixture) error {
	switch strings.ToLower(strings.TrimSpace(definition.Kind)) {
	case "", "generic":
		return errors.New(definition.Message)
	case "api":
		return &graph.APIError{
			Type:         definition.Type,
			Code:         definition.Code,
			ErrorSubcode: definition.Subcode,
			Message:      definition.Message,
			FBTraceID:    definition.FBTraceID,
			StatusCode:   definition.StatusCode,
		}
	default:
		return fmt.Errorf("unsupported fixture error kind %q", definition.Kind)
	}
}

func failureSignature(failure Failure) string {
	return fmt.Sprintf(
		"type=%s|status=%d|code=%d|subcode=%d|message=%s",
		strings.TrimSpace(failure.Type),
		failure.StatusCode,
		failure.Code,
		failure.ErrorSubcode,
		strings.TrimSpace(failure.Message),
	)
}
