package smoke

import (
	"context"
	"net/http"
	"testing"

	"github.com/bilalbayram/metacli/internal/graph"
)

type fakeCall struct {
	Method   string
	Path     string
	Response *graph.Response
	Err      error
}

type fakeGraphClient struct {
	t     *testing.T
	calls []fakeCall
	index int
}

func (f *fakeGraphClient) Do(_ context.Context, req graph.Request) (*graph.Response, error) {
	f.t.Helper()
	if f.index >= len(f.calls) {
		f.t.Fatalf("unexpected extra request %s %s", req.Method, req.Path)
	}

	expected := f.calls[f.index]
	f.index++

	if req.Method != expected.Method {
		f.t.Fatalf("request %d method mismatch: got=%s want=%s", f.index, req.Method, expected.Method)
	}
	if req.Path != expected.Path {
		f.t.Fatalf("request %d path mismatch: got=%s want=%s", f.index, req.Path, expected.Path)
	}
	if expected.Err != nil {
		return nil, expected.Err
	}
	if expected.Response == nil {
		return &graph.Response{}, nil
	}
	return expected.Response, nil
}

func (f *fakeGraphClient) assertAllCallsConsumed() {
	f.t.Helper()
	if f.index != len(f.calls) {
		f.t.Fatalf("expected %d calls, got %d", len(f.calls), f.index)
	}
}

func TestRunnerStrictPolicyBlocksWhenCatalogIsUnavailable(t *testing.T) {
	client := &fakeGraphClient{
		t: t,
		calls: []fakeCall{
			{
				Method: http.MethodGet,
				Path:   "act_1234",
				Response: &graph.Response{
					Body: map[string]any{
						"id":             "act_1234",
						"name":           "Primary",
						"currency":       "usd",
						"account_status": float64(1),
					},
					RateLimit: graph.RateLimit{
						AppUsage: map[string]any{
							"call_count":    float64(11),
							"total_cputime": float64(7),
							"total_time":    float64(6),
						},
					},
				},
			},
			{
				Method: http.MethodPost,
				Path:   "act_1234/campaigns",
				Response: &graph.Response{
					Body: map[string]any{
						"id": "cmp_1001",
					},
					RateLimit: graph.RateLimit{
						AppUsage: map[string]any{
							"call_count":    float64(13),
							"total_cputime": float64(9),
							"total_time":    float64(8),
						},
					},
				},
			},
			{
				Method: http.MethodPost,
				Path:   "act_1234/customaudiences",
				Response: &graph.Response{
					Body: map[string]any{
						"id": "aud_5001",
					},
					RateLimit: graph.RateLimit{
						AppUsage: map[string]any{
							"call_count":    float64(19),
							"total_cputime": float64(10),
							"total_time":    float64(9),
						},
					},
				},
			},
		},
	}

	runner := NewRunner(client)
	result, err := runner.Run(context.Background(), RunInput{
		ProfileName:    "prod",
		Version:        "v25.0",
		AccountID:      "act_1234",
		Token:          "token",
		OptionalPolicy: OptionalPolicyStrict,
	})
	if err != nil {
		t.Fatalf("run smoke runner: %v", err)
	}
	client.assertAllCallsConsumed()

	report := result.Report
	if report.Outcome != RunOutcomeBlocking {
		t.Fatalf("unexpected outcome: %s", report.Outcome)
	}
	if report.Summary.Blocking != 1 {
		t.Fatalf("unexpected blocking summary: %+v", report.Summary)
	}
	if report.Summary.FailedSteps != 1 {
		t.Fatalf("unexpected failed step count: %+v", report.Summary)
	}
	if code := RunExitCode(report); code != ExitCodePolicy {
		t.Fatalf("unexpected exit code: got=%d want=%d", code, ExitCodePolicy)
	}
	if len(report.CreatedResources) != 2 {
		t.Fatalf("expected 2 created resources, got %d", len(report.CreatedResources))
	}
	if report.RateLimit.MaxAppCallCount != 19 {
		t.Fatalf("unexpected max app call count: %d", report.RateLimit.MaxAppCallCount)
	}
	if len(report.Steps) != 4 {
		t.Fatalf("unexpected step count: %d", len(report.Steps))
	}
	lastStep := report.Steps[3]
	if lastStep.Name != stepNameCatalogUpload {
		t.Fatalf("unexpected last step name: %s", lastStep.Name)
	}
	if lastStep.Status != StepStatusFailed || !lastStep.Blocking {
		t.Fatalf("expected blocking catalog failure step, got %+v", lastStep)
	}
	if report.Capabilities[1].Name != capabilityCatalog || report.Capabilities[1].Status != CapabilityStatusUnavailable {
		t.Fatalf("unexpected catalog capability state: %+v", report.Capabilities[1])
	}
}

func TestRunnerSkipPolicySkipsUnavailableOptionalStepsAsWarnings(t *testing.T) {
	client := &fakeGraphClient{
		t: t,
		calls: []fakeCall{
			{
				Method: http.MethodGet,
				Path:   "act_1234",
				Response: &graph.Response{
					Body: map[string]any{
						"id":             "act_1234",
						"name":           "Primary",
						"currency":       "USD",
						"account_status": float64(1),
					},
				},
			},
			{
				Method: http.MethodPost,
				Path:   "act_1234/campaigns",
				Response: &graph.Response{
					Body: map[string]any{
						"id": "cmp_1001",
					},
				},
			},
			{
				Method: http.MethodPost,
				Path:   "act_1234/customaudiences",
				Err: &graph.APIError{
					Type:       "OAuthException",
					Code:       200,
					StatusCode: http.StatusBadRequest,
					Message:    "Permissions error",
				},
			},
		},
	}

	runner := NewRunner(client)
	result, err := runner.Run(context.Background(), RunInput{
		ProfileName:    "prod",
		Version:        "v25.0",
		AccountID:      "1234",
		Token:          "token",
		OptionalPolicy: OptionalPolicySkip,
	})
	if err != nil {
		t.Fatalf("run smoke runner: %v", err)
	}
	client.assertAllCallsConsumed()

	report := result.Report
	if report.Outcome != RunOutcomeWarning {
		t.Fatalf("unexpected outcome: %s", report.Outcome)
	}
	if report.Summary.Warnings != 2 {
		t.Fatalf("unexpected warning summary: %+v", report.Summary)
	}
	if report.Summary.Blocking != 0 {
		t.Fatalf("unexpected blocking summary: %+v", report.Summary)
	}
	if report.Summary.FailedSteps != 0 {
		t.Fatalf("unexpected failed step count: %+v", report.Summary)
	}
	if len(report.Failures) != 0 {
		t.Fatalf("expected no failures, got %+v", report.Failures)
	}
	if len(report.CreatedResources) != 1 {
		t.Fatalf("expected one created resource, got %d", len(report.CreatedResources))
	}
	if report.Steps[2].Status != StepStatusSkipped || !report.Steps[2].Warning {
		t.Fatalf("expected audience step to be warning skip, got %+v", report.Steps[2])
	}
	if report.Steps[3].Status != StepStatusSkipped || !report.Steps[3].Warning {
		t.Fatalf("expected catalog step to be warning skip, got %+v", report.Steps[3])
	}
	if code := RunExitCode(report); code != ExitCodeWarning {
		t.Fatalf("unexpected exit code: got=%d want=%d", code, ExitCodeWarning)
	}
}

func TestRunnerBlocksRemainingStepsAfterRequiredFailure(t *testing.T) {
	client := &fakeGraphClient{
		t: t,
		calls: []fakeCall{
			{
				Method: http.MethodGet,
				Path:   "act_1234",
				Err: &graph.APIError{
					Type:       "OAuthException",
					Code:       190,
					StatusCode: http.StatusUnauthorized,
					Message:    "Invalid OAuth access token",
				},
			},
		},
	}

	runner := NewRunner(client)
	result, err := runner.Run(context.Background(), RunInput{
		ProfileName:    "prod",
		Version:        "v25.0",
		AccountID:      "1234",
		Token:          "token",
		OptionalPolicy: OptionalPolicySkip,
		CatalogID:      "cat_1",
	})
	if err != nil {
		t.Fatalf("run smoke runner: %v", err)
	}
	client.assertAllCallsConsumed()

	report := result.Report
	if report.Outcome != RunOutcomeBlocking {
		t.Fatalf("unexpected outcome: %s", report.Outcome)
	}
	if report.Summary.FailedSteps != 1 || report.Summary.SkippedSteps != 3 {
		t.Fatalf("unexpected summary: %+v", report.Summary)
	}
	if len(report.Failures) != 1 {
		t.Fatalf("expected one failure, got %+v", report.Failures)
	}
	if report.Failures[0].Step != stepNameAccountContext {
		t.Fatalf("unexpected failure step: %+v", report.Failures[0])
	}
	for index := 1; index < len(report.Steps); index++ {
		if report.Steps[index].Status != StepStatusSkipped {
			t.Fatalf("expected blocked step at index %d to be skipped, got %+v", index, report.Steps[index])
		}
	}
}
