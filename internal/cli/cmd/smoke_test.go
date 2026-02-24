package cmd

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/bilalbayram/metacli/internal/config"
	"github.com/bilalbayram/metacli/internal/graph"
	"github.com/bilalbayram/metacli/internal/smoke"
)

type fakeSmokeCall struct {
	Method   string
	Path     string
	Response *graph.Response
	Err      error
}

type fakeSmokeGraphClient struct {
	t     *testing.T
	calls []fakeSmokeCall
	index int
}

func (f *fakeSmokeGraphClient) Do(_ context.Context, req graph.Request) (*graph.Response, error) {
	f.t.Helper()
	if f.index >= len(f.calls) {
		f.t.Fatalf("unexpected extra smoke request %s %s", req.Method, req.Path)
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

func (f *fakeSmokeGraphClient) assertAllCallsConsumed() {
	f.t.Helper()
	if f.index != len(f.calls) {
		f.t.Fatalf("expected %d calls, got %d", len(f.calls), f.index)
	}
}

func TestSmokeRunStrictPolicyReturnsBlockingExit(t *testing.T) {
	client := &fakeSmokeGraphClient{
		t: t,
		calls: []fakeSmokeCall{
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
				Response: &graph.Response{
					Body: map[string]any{
						"id": "aud_1001",
					},
				},
			},
		},
	}

	useSmokeDependencies(
		t,
		func(profile string) (*ProfileCredentials, error) {
			if profile != "prod" {
				t.Fatalf("unexpected profile %q", profile)
			}
			return &ProfileCredentials{
				Name: "prod",
				Profile: config.Profile{
					GraphVersion: config.DefaultGraphVersion,
				},
				Token: "test-token",
			}, nil
		},
		func(smoke.GraphClient) *smoke.Runner {
			return smoke.NewRunner(client)
		},
	)

	stdout, stderr, err := executeSmokeCommand(runtimeWithProfile("prod"), "run", "--account-id", "1234", "--optional-policy", "strict")
	if err == nil {
		t.Fatal("expected smoke run strict policy to fail")
	}
	var exitErr *smoke.ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("expected smoke ExitError, got %T", err)
	}
	if exitErr.Code != smoke.ExitCodePolicy {
		t.Fatalf("unexpected exit code: got=%d want=%d", exitErr.Code, smoke.ExitCodePolicy)
	}
	if stderr != "" {
		t.Fatalf("expected empty stderr, got %q", stderr)
	}
	client.assertAllCallsConsumed()

	envelope := decodeOpsEnvelope(t, []byte(stdout))
	if envelope.Command != smoke.CommandRun {
		t.Fatalf("unexpected command: %s", envelope.Command)
	}
	if envelope.Success {
		t.Fatal("expected success=false")
	}
	if envelope.ExitCode != smoke.ExitCodePolicy {
		t.Fatalf("unexpected envelope exit code: got=%d want=%d", envelope.ExitCode, smoke.ExitCodePolicy)
	}
	if envelope.Error == nil || envelope.Error.Type != "blocking_findings" {
		t.Fatalf("unexpected envelope error payload: %+v", envelope.Error)
	}
}

func TestSmokeRunSkipPolicyReturnsWarningExit(t *testing.T) {
	client := &fakeSmokeGraphClient{
		t: t,
		calls: []fakeSmokeCall{
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

	useSmokeDependencies(
		t,
		func(string) (*ProfileCredentials, error) {
			return &ProfileCredentials{
				Name: "prod",
				Profile: config.Profile{
					GraphVersion: config.DefaultGraphVersion,
				},
				Token: "test-token",
			}, nil
		},
		func(smoke.GraphClient) *smoke.Runner {
			return smoke.NewRunner(client)
		},
	)

	stdout, stderr, err := executeSmokeCommand(runtimeWithProfile("prod"), "run", "--account-id", "1234", "--optional-policy", "skip")
	if err == nil {
		t.Fatal("expected smoke run skip policy to return warning error")
	}
	var exitErr *smoke.ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("expected smoke ExitError, got %T", err)
	}
	if exitErr.Code != smoke.ExitCodeWarning {
		t.Fatalf("unexpected exit code: got=%d want=%d", exitErr.Code, smoke.ExitCodeWarning)
	}
	if stderr != "" {
		t.Fatalf("expected empty stderr, got %q", stderr)
	}
	client.assertAllCallsConsumed()

	envelope := decodeOpsEnvelope(t, []byte(stdout))
	if envelope.Command != smoke.CommandRun {
		t.Fatalf("unexpected command: %s", envelope.Command)
	}
	if envelope.Success {
		t.Fatal("expected success=false")
	}
	if envelope.ExitCode != smoke.ExitCodeWarning {
		t.Fatalf("unexpected envelope exit code: got=%d want=%d", envelope.ExitCode, smoke.ExitCodeWarning)
	}
	if envelope.Error == nil || envelope.Error.Type != "warning_findings" {
		t.Fatalf("unexpected envelope error payload: %+v", envelope.Error)
	}
}

func TestSmokeRunRejectsInvalidOptionalPolicy(t *testing.T) {
	useSmokeDependencies(
		t,
		func(string) (*ProfileCredentials, error) {
			return &ProfileCredentials{
				Name: "prod",
				Profile: config.Profile{
					GraphVersion: config.DefaultGraphVersion,
				},
				Token: "test-token",
			}, nil
		},
		func(client smoke.GraphClient) *smoke.Runner {
			return smoke.NewRunner(client)
		},
	)

	stdout, stderr, err := executeSmokeCommand(runtimeWithProfile("prod"), "run", "--account-id", "1234", "--optional-policy", "maybe")
	if err == nil {
		t.Fatal("expected invalid policy to fail")
	}
	var exitErr *smoke.ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("expected smoke ExitError, got %T", err)
	}
	if exitErr.Code != smoke.ExitCodeInput {
		t.Fatalf("unexpected exit code: got=%d want=%d", exitErr.Code, smoke.ExitCodeInput)
	}
	if stdout != "" {
		t.Fatalf("expected empty stdout, got %q", stdout)
	}

	envelope := decodeOpsEnvelope(t, []byte(stderr))
	if envelope.Command != smoke.CommandRun {
		t.Fatalf("unexpected command: %s", envelope.Command)
	}
	if envelope.Success {
		t.Fatal("expected success=false")
	}
	if envelope.ExitCode != smoke.ExitCodeInput {
		t.Fatalf("unexpected envelope exit code: got=%d want=%d", envelope.ExitCode, smoke.ExitCodeInput)
	}
	if envelope.Error == nil || envelope.Error.Type != "input_error" {
		t.Fatalf("unexpected envelope error payload: %+v", envelope.Error)
	}
}

func executeSmokeCommand(runtime Runtime, args ...string) (string, string, error) {
	cmd := NewSmokeCommand(runtime)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetArgs(args)
	err := cmd.Execute()
	return stdout.String(), stderr.String(), err
}

func useSmokeDependencies(
	t *testing.T,
	loadFn func(string) (*ProfileCredentials, error),
	runnerFn func(smoke.GraphClient) *smoke.Runner,
) {
	t.Helper()
	originalLoad := smokeLoadProfileCredentials
	originalRunner := smokeNewRunner
	originalClient := smokeNewGraphClient
	t.Cleanup(func() {
		smokeLoadProfileCredentials = originalLoad
		smokeNewRunner = originalRunner
		smokeNewGraphClient = originalClient
	})

	smokeLoadProfileCredentials = loadFn
	smokeNewRunner = runnerFn
	smokeNewGraphClient = func() *graph.Client {
		client := graph.NewClient(nil, "")
		client.MaxRetries = 0
		return client
	}
}
