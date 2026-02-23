package cmd

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/bilalbayram/metacli/internal/config"
	"github.com/bilalbayram/metacli/internal/graph"
	"github.com/spf13/cobra"
)

type trackBSmokeGraphCall struct {
	method   string
	path     string
	status   int
	response string
	assert   func(*testing.T, *http.Request)
}

func TestTrackBPublishWorkflowSmokeCLICommands(t *testing.T) {
	statePath := t.TempDir() + "/ig-schedules.json"
	publishAt := time.Now().UTC().Add(2 * time.Hour).Format(time.RFC3339)
	retryAt := time.Now().UTC().Add(4 * time.Hour).Format(time.RFC3339)

	expectedCalls := []trackBSmokeGraphCall{
		{
			method:   http.MethodPost,
			path:     "/v25.0/17841400008460056/media",
			status:   http.StatusOK,
			response: `{"id":"creation_upload_01","status_code":"IN_PROGRESS"}`,
			assert: func(t *testing.T, req *http.Request) {
				form := parseTrackBSmokeRequestForm(t, req)
				if got := form.Get("image_url"); got != "https://cdn.example.com/upload.jpg" {
					t.Fatalf("unexpected upload image_url %q", got)
				}
				if got := form.Get("caption"); got != "Track B upload #meta" {
					t.Fatalf("unexpected upload caption %q", got)
				}
			},
		},
		{
			method:   http.MethodGet,
			path:     "/v25.0/creation_upload_01",
			status:   http.StatusOK,
			response: `{"id":"creation_upload_01","status":"FINISHED","status_code":"FINISHED"}`,
			assert: func(t *testing.T, req *http.Request) {
				if got := req.URL.Query().Get("fields"); got != "id,status,status_code" {
					t.Fatalf("unexpected status fields query %q", got)
				}
			},
		},
		{
			method:   http.MethodPost,
			path:     "/v25.0/17841400008460056/media",
			status:   http.StatusServiceUnavailable,
			response: `{}`,
			assert: func(t *testing.T, req *http.Request) {
				form := parseTrackBSmokeRequestForm(t, req)
				if got := form.Get("image_url"); got != "https://cdn.example.com/publish.jpg" {
					t.Fatalf("unexpected publish upload image_url %q", got)
				}
				if got := form.Get("idempotency_key"); got != "publish_retry_01" {
					t.Fatalf("unexpected publish upload idempotency_key %q", got)
				}
			},
		},
		{
			method:   http.MethodPost,
			path:     "/v25.0/17841400008460056/media",
			status:   http.StatusOK,
			response: `{"id":"creation_publish_01","status_code":"IN_PROGRESS"}`,
			assert: func(t *testing.T, req *http.Request) {
				form := parseTrackBSmokeRequestForm(t, req)
				if got := form.Get("image_url"); got != "https://cdn.example.com/publish.jpg" {
					t.Fatalf("unexpected publish retry upload image_url %q", got)
				}
				if got := form.Get("idempotency_key"); got != "publish_retry_01" {
					t.Fatalf("unexpected publish retry upload idempotency_key %q", got)
				}
			},
		},
		{
			method:   http.MethodGet,
			path:     "/v25.0/creation_publish_01",
			status:   http.StatusOK,
			response: `{"id":"creation_publish_01","status":"FINISHED","status_code":"FINISHED"}`,
			assert: func(t *testing.T, req *http.Request) {
				if got := req.URL.Query().Get("fields"); got != "id,status,status_code" {
					t.Fatalf("unexpected publish status fields query %q", got)
				}
			},
		},
		{
			method:   http.MethodPost,
			path:     "/v25.0/17841400008460056/media_publish",
			status:   http.StatusOK,
			response: `{"id":"media_publish_01"}`,
			assert: func(t *testing.T, req *http.Request) {
				form := parseTrackBSmokeRequestForm(t, req)
				if got := form.Get("creation_id"); got != "creation_publish_01" {
					t.Fatalf("unexpected publish creation_id %q", got)
				}
				if got := form.Get("idempotency_key"); got != "publish_retry_01" {
					t.Fatalf("unexpected publish idempotency_key %q", got)
				}
			},
		},
	}

	callIndex := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if callIndex >= len(expectedCalls) {
			t.Fatalf("unexpected extra request %s %s", req.Method, req.URL.Path)
		}
		expected := expectedCalls[callIndex]
		callIndex++

		if req.Method != expected.method {
			t.Fatalf("call %d method mismatch: got %s want %s", callIndex, req.Method, expected.method)
		}
		if req.URL.Path != expected.path {
			t.Fatalf("call %d path mismatch: got %s want %s", callIndex, req.URL.Path, expected.path)
		}
		if expected.assert != nil {
			expected.assert(t, req)
		}

		status := expected.status
		if status == 0 {
			status = http.StatusOK
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		if _, err := w.Write([]byte(expected.response)); err != nil {
			t.Fatalf("write response body: %v", err)
		}
	}))
	defer server.Close()

	useIGDependencies(t,
		func(profile string) (*ProfileCredentials, error) {
			if profile != "prod" {
				t.Fatalf("unexpected profile %q", profile)
			}
			return &ProfileCredentials{
				Name: "prod",
				Profile: config.Profile{
					Domain:       config.DefaultDomain,
					GraphVersion: "v25.0",
				},
				Token:     "test-token",
				AppSecret: "test-secret",
			}, nil
		},
		func() *graph.Client {
			client := graph.NewClient(server.Client(), server.URL)
			client.InitialBackoff = 0
			client.MaxBackoff = 0
			client.Sleep = func(time.Duration) {}
			return client
		},
	)

	captionEnvelope := runTrackBSmokeSuccessCommand(t, NewIGCommand(testRuntime("prod")), []string{
		"caption", "validate",
		"--caption", "Track B smoke #meta",
	}, "meta ig caption validate")
	captionData := trackBSmokeEnvelopeData(t, captionEnvelope)
	if got := captionData["valid"]; got != true {
		t.Fatalf("expected caption valid=true, got %v", got)
	}

	uploadEnvelope := runTrackBSmokeSuccessCommand(t, NewIGCommand(testRuntime("prod")), []string{
		"media", "upload",
		"--ig-user-id", "17841400008460056",
		"--media-url", "https://cdn.example.com/upload.jpg",
		"--caption", "Track B upload #meta",
		"--media-type", "IMAGE",
	}, "meta ig media upload")
	creationID := trackBSmokeDataString(t, uploadEnvelope, "creation_id")
	if creationID != "creation_upload_01" {
		t.Fatalf("unexpected creation_id %q", creationID)
	}

	statusEnvelope := runTrackBSmokeSuccessCommand(t, NewIGCommand(testRuntime("prod")), []string{
		"media", "status",
		"--creation-id", creationID,
	}, "meta ig media status")
	statusData := trackBSmokeEnvelopeData(t, statusEnvelope)
	if got := statusData["status_code"]; got != "FINISHED" {
		t.Fatalf("unexpected status_code %v", got)
	}

	publishEnvelope := runTrackBSmokeSuccessCommand(t, NewIGCommand(testRuntime("prod")), []string{
		"publish", "feed",
		"--ig-user-id", "17841400008460056",
		"--media-url", "https://cdn.example.com/publish.jpg",
		"--caption", "Track B publish #meta",
		"--media-type", "IMAGE",
		"--idempotency-key", "publish_retry_01",
	}, "meta ig publish feed")
	publishData := trackBSmokeEnvelopeData(t, publishEnvelope)
	if got := publishData["mode"]; got != "immediate" {
		t.Fatalf("unexpected publish mode %v", got)
	}
	if got := publishData["creation_id"]; got != "creation_publish_01" {
		t.Fatalf("unexpected publish creation_id %v", got)
	}
	if got := publishData["media_id"]; got != "media_publish_01" {
		t.Fatalf("unexpected publish media_id %v", got)
	}
	if got := publishData["status_code"]; got != "FINISHED" {
		t.Fatalf("unexpected publish status_code %v", got)
	}
	if got := publishData["idempotency_key"]; got != "publish_retry_01" {
		t.Fatalf("unexpected publish idempotency_key %v", got)
	}

	createScheduleEnvelope := runTrackBSmokeSuccessCommand(t, NewIGCommand(testRuntime("prod")), []string{
		"publish", "feed",
		"--ig-user-id", "17841400008460056",
		"--media-url", "https://cdn.example.com/schedule.jpg",
		"--caption", "Track B schedule #meta",
		"--media-type", "IMAGE",
		"--idempotency-key", "schedule_01",
		"--publish-at", publishAt,
		"--schedule-state-path", statePath,
	}, "meta ig publish feed")
	createScheduleData := trackBSmokeEnvelopeData(t, createScheduleEnvelope)
	if got := createScheduleData["mode"]; got != "scheduled" {
		t.Fatalf("unexpected schedule mode %v", got)
	}
	if got := createScheduleData["duplicate_suppressed"]; got != false {
		t.Fatalf("unexpected duplicate_suppressed %v", got)
	}
	createSchedule := trackBSmokeScheduleData(t, createScheduleData)
	scheduleID, _ := createSchedule["schedule_id"].(string)
	if strings.TrimSpace(scheduleID) == "" {
		t.Fatal("expected schedule_id from create schedule command")
	}

	duplicateScheduleEnvelope := runTrackBSmokeSuccessCommand(t, NewIGCommand(testRuntime("prod")), []string{
		"publish", "feed",
		"--ig-user-id", "17841400008460056",
		"--media-url", "https://cdn.example.com/schedule.jpg",
		"--caption", "Track B schedule #meta",
		"--media-type", "IMAGE",
		"--idempotency-key", "schedule_01",
		"--publish-at", publishAt,
		"--schedule-state-path", statePath,
	}, "meta ig publish feed")
	duplicateScheduleData := trackBSmokeEnvelopeData(t, duplicateScheduleEnvelope)
	if got := duplicateScheduleData["duplicate_suppressed"]; got != true {
		t.Fatalf("expected duplicate_suppressed=true, got %v", got)
	}
	duplicateSchedule := trackBSmokeScheduleData(t, duplicateScheduleData)
	if got := duplicateSchedule["schedule_id"]; got != scheduleID {
		t.Fatalf("expected duplicate schedule_id %q, got %v", scheduleID, got)
	}

	conflictEnvelope := runTrackBSmokeFailCommand(t, NewIGCommand(testRuntime("prod")), []string{
		"publish", "feed",
		"--ig-user-id", "17841400008460056",
		"--media-url", "https://cdn.example.com/schedule-conflict.jpg",
		"--caption", "Track B schedule #meta",
		"--media-type", "IMAGE",
		"--idempotency-key", "schedule_01",
		"--publish-at", publishAt,
		"--schedule-state-path", statePath,
	}, "meta ig publish feed", "idempotency key")
	conflictErrorBody, ok := conflictEnvelope["error"].(map[string]any)
	if !ok {
		t.Fatalf("expected error object, got %T", conflictEnvelope["error"])
	}
	if got := conflictErrorBody["type"]; got != "ig_idempotency_conflict" {
		t.Fatalf("unexpected conflict error type %v", got)
	}
	if got := conflictErrorBody["retryable"]; got != false {
		t.Fatalf("expected retryable=false on conflict, got %v", got)
	}

	listEnvelope := runTrackBSmokeSuccessCommand(t, NewIGCommand(testRuntime("prod")), []string{
		"publish", "schedule", "list",
		"--schedule-state-path", statePath,
	}, "meta ig publish schedule list")
	listData := trackBSmokeEnvelopeData(t, listEnvelope)
	if got := listData["total"]; got != float64(1) {
		t.Fatalf("unexpected schedule total %v", got)
	}
	listSchedules, ok := listData["schedules"].([]any)
	if !ok || len(listSchedules) != 1 {
		t.Fatalf("expected one schedule item, got %#v", listData["schedules"])
	}
	listedSchedule, ok := listSchedules[0].(map[string]any)
	if !ok {
		t.Fatalf("expected schedule object, got %T", listSchedules[0])
	}
	if got := listedSchedule["schedule_id"]; got != scheduleID {
		t.Fatalf("expected listed schedule_id %q, got %v", scheduleID, got)
	}
	if got := listedSchedule["status"]; got != "scheduled" {
		t.Fatalf("unexpected listed status %v", got)
	}

	cancelEnvelope := runTrackBSmokeSuccessCommand(t, NewIGCommand(testRuntime("prod")), []string{
		"publish", "schedule", "cancel",
		"--schedule-id", scheduleID,
		"--schedule-state-path", statePath,
	}, "meta ig publish schedule cancel")
	cancelData := trackBSmokeEnvelopeData(t, cancelEnvelope)
	canceledSchedule := trackBSmokeScheduleData(t, cancelData)
	if got := canceledSchedule["status"]; got != "canceled" {
		t.Fatalf("unexpected cancel status %v", got)
	}

	retryEnvelope := runTrackBSmokeSuccessCommand(t, NewIGCommand(testRuntime("prod")), []string{
		"publish", "schedule", "retry",
		"--schedule-id", scheduleID,
		"--publish-at", retryAt,
		"--schedule-state-path", statePath,
	}, "meta ig publish schedule retry")
	retryData := trackBSmokeEnvelopeData(t, retryEnvelope)
	retriedSchedule := trackBSmokeScheduleData(t, retryData)
	if got := retriedSchedule["status"]; got != "scheduled" {
		t.Fatalf("unexpected retry status %v", got)
	}
	if got := retriedSchedule["retry_count"]; got != float64(1) {
		t.Fatalf("unexpected retry_count %v", got)
	}
	if got := retriedSchedule["publish_at"]; got != retryAt {
		t.Fatalf("unexpected retry publish_at %v", got)
	}

	finalListEnvelope := runTrackBSmokeSuccessCommand(t, NewIGCommand(testRuntime("prod")), []string{
		"publish", "schedule", "list",
		"--status", "scheduled",
		"--schedule-state-path", statePath,
	}, "meta ig publish schedule list")
	finalListData := trackBSmokeEnvelopeData(t, finalListEnvelope)
	if got := finalListData["total"]; got != float64(1) {
		t.Fatalf("unexpected final schedule total %v", got)
	}

	if callIndex != len(expectedCalls) {
		t.Fatalf("expected %d graph calls, got %d", len(expectedCalls), callIndex)
	}
}

func runTrackBSmokeSuccessCommand(t *testing.T, cmd *cobra.Command, args []string, commandName string) map[string]any {
	t.Helper()

	stdout, stderr, err := executeTrackBSmokeCommand(t, cmd, args)
	if err != nil {
		t.Fatalf("execute %s: %v", commandName, err)
	}
	if stderr.Len() != 0 {
		t.Fatalf("expected empty stderr for %s, got %q", commandName, stderr.String())
	}

	envelope := decodeEnvelope(t, stdout.Bytes())
	if got := envelope["contract_version"]; got != "1.0" {
		t.Fatalf("expected contract_version=1.0 for %s, got %v", commandName, got)
	}
	assertEnvelopeBasics(t, envelope, commandName)
	return envelope
}

func runTrackBSmokeFailCommand(t *testing.T, cmd *cobra.Command, args []string, commandName string, expectedErrorSubstring string) map[string]any {
	t.Helper()

	stdout, stderr, err := executeTrackBSmokeCommand(t, cmd, args)
	if err == nil {
		t.Fatalf("expected command error for %s", commandName)
	}
	if !strings.Contains(err.Error(), expectedErrorSubstring) {
		t.Fatalf("unexpected command error for %s: %v", commandName, err)
	}
	if stdout.Len() != 0 {
		t.Fatalf("expected empty stdout for %s, got %q", commandName, stdout.String())
	}

	envelope := decodeEnvelope(t, stderr.Bytes())
	if got := envelope["command"]; got != commandName {
		t.Fatalf("unexpected envelope command %v", got)
	}
	if got := envelope["contract_version"]; got != "1.0" {
		t.Fatalf("expected contract_version=1.0 for %s, got %v", commandName, got)
	}
	if got := envelope["success"]; got != false {
		t.Fatalf("expected success=false for %s, got %v", commandName, got)
	}
	return envelope
}

func executeTrackBSmokeCommand(t *testing.T, cmd *cobra.Command, args []string) (*bytes.Buffer, *bytes.Buffer, error) {
	t.Helper()

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	cmd.SetArgs(args)
	err := cmd.Execute()
	return stdout, stderr, err
}

func trackBSmokeEnvelopeData(t *testing.T, envelope map[string]any) map[string]any {
	t.Helper()

	data, ok := envelope["data"].(map[string]any)
	if !ok {
		t.Fatalf("expected object envelope data, got %T", envelope["data"])
	}
	return data
}

func trackBSmokeDataString(t *testing.T, envelope map[string]any, key string) string {
	t.Helper()

	data := trackBSmokeEnvelopeData(t, envelope)
	value, ok := data[key].(string)
	if !ok {
		t.Fatalf("expected %s to be a string, got %T", key, data[key])
	}
	return value
}

func trackBSmokeScheduleData(t *testing.T, data map[string]any) map[string]any {
	t.Helper()

	schedule, ok := data["schedule"].(map[string]any)
	if !ok {
		t.Fatalf("expected schedule object, got %T", data["schedule"])
	}
	return schedule
}

func parseTrackBSmokeRequestForm(t *testing.T, req *http.Request) url.Values {
	t.Helper()

	body, err := io.ReadAll(req.Body)
	if err != nil {
		t.Fatalf("read request body: %v", err)
	}
	form, err := url.ParseQuery(string(body))
	if err != nil {
		t.Fatalf("parse request body: %v", err)
	}
	return form
}
