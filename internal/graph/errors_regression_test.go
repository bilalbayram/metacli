package graph

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

type remediationRegressionFixture struct {
	Name              string         `json:"name"`
	StatusCode        int            `json:"status_code"`
	Code              int            `json:"code"`
	Subcode           int            `json:"subcode"`
	Diagnostics       map[string]any `json:"diagnostics,omitempty"`
	Expected          Remediation    `json:"expected"`
	ExpectedSignature string         `json:"expected_signature"`
}

func TestClassifyRemediationRegressionFixtures(t *testing.T) {
	t.Parallel()

	fixtures := loadRemediationRegressionFixtures(t)
	for _, fixture := range fixtures {
		fixture := fixture
		t.Run(fixture.Name, func(t *testing.T) {
			t.Parallel()

			got := ClassifyRemediation(
				fixture.StatusCode,
				fixture.Code,
				fixture.Subcode,
				"fixture regression probe",
				fixture.Diagnostics,
			)
			if !reflect.DeepEqual(got, fixture.Expected) {
				t.Fatalf(
					"unexpected remediation contract:\n got=%s\nwant=%s",
					mustMarshalJSON(t, got),
					mustMarshalJSON(t, fixture.Expected),
				)
			}

			signature := remediationSignature(fixture.StatusCode, fixture.Code, fixture.Subcode, got)
			if signature != fixture.ExpectedSignature {
				t.Fatalf("unexpected remediation signature: got=%q want=%q", signature, fixture.ExpectedSignature)
			}
		})
	}
}

func loadRemediationRegressionFixtures(t *testing.T) []remediationRegressionFixture {
	t.Helper()

	path := filepath.Join("testdata", "subcode-remediation-regression.json")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read remediation fixtures %s: %v", path, err)
	}

	var fixtures []remediationRegressionFixture
	if err := json.Unmarshal(raw, &fixtures); err != nil {
		t.Fatalf("decode remediation fixtures %s: %v", path, err)
	}
	if len(fixtures) == 0 {
		t.Fatalf("remediation fixtures %s are empty", path)
	}
	return fixtures
}

func remediationSignature(statusCode int, code int, subcode int, remediation Remediation) string {
	fields := "-"
	if len(remediation.Fields) > 0 {
		fields = strings.Join(remediation.Fields, ",")
	}
	return fmt.Sprintf(
		"status=%d|code=%d|subcode=%d|category=%s|fields=%s",
		statusCode,
		code,
		subcode,
		remediation.Category,
		fields,
	)
}

func mustMarshalJSON(t *testing.T, value any) string {
	t.Helper()

	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal fixture json: %v", err)
	}
	return string(raw)
}
