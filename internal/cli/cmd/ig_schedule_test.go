package cmd

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/bilalbayram/metacli/internal/config"
	"github.com/bilalbayram/metacli/internal/graph"
)

func TestIGPublishFeedCommandSchedulesWhenPublishAtProvided(t *testing.T) {
	wasCalled := false
	useIGDependencies(t,
		func(string) (*ProfileCredentials, error) {
			return &ProfileCredentials{
				Name:      "prod",
				Profile:   config.Profile{GraphVersion: "v25.0"},
				Token:     "test-token",
				AppSecret: "test-secret",
			}, nil
		},
		func() *graph.Client {
			wasCalled = true
			return graph.NewClient(nil, "")
		},
	)

	statePath := t.TempDir() + "/ig-schedules.json"
	publishAt := time.Now().UTC().Add(2 * time.Hour).Format(time.RFC3339)

	output := &bytes.Buffer{}
	errOutput := &bytes.Buffer{}
	cmd := NewIGCommand(testRuntime("prod"))
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
	cmd.SetOut(output)
	cmd.SetErr(errOutput)
	cmd.SetArgs([]string{
		"publish", "feed",
		"--ig-user-id", "17841400008460056",
		"--media-url", "https://cdn.example.com/feed.jpg",
		"--caption", "hello #meta",
		"--media-type", "IMAGE",
		"--publish-at", publishAt,
		"--schedule-state-path", statePath,
	})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute scheduled publish: %v", err)
	}
	if wasCalled {
		t.Fatal("graph client should not execute when publish-at scheduling is used")
	}

	envelope := decodeEnvelope(t, output.Bytes())
	assertEnvelopeBasics(t, envelope, "meta ig publish feed")
	data, ok := envelope["data"].(map[string]any)
	if !ok {
		t.Fatalf("expected object payload, got %T", envelope["data"])
	}
	if got := data["mode"]; got != "scheduled" {
		t.Fatalf("unexpected mode %v", got)
	}
	schedule, ok := data["schedule"].(map[string]any)
	if !ok {
		t.Fatalf("expected schedule object, got %T", data["schedule"])
	}
	if got := schedule["status"]; got != "scheduled" {
		t.Fatalf("unexpected status %v", got)
	}
}

func TestIGPublishScheduleLifecycleCommands(t *testing.T) {
	wasCalled := false
	useIGDependencies(t,
		func(string) (*ProfileCredentials, error) {
			return &ProfileCredentials{
				Name:      "prod",
				Profile:   config.Profile{GraphVersion: "v25.0"},
				Token:     "test-token",
				AppSecret: "test-secret",
			}, nil
		},
		func() *graph.Client {
			wasCalled = true
			return graph.NewClient(nil, "")
		},
	)

	statePath := t.TempDir() + "/ig-schedules.json"
	publishAt := time.Now().UTC().Add(2 * time.Hour).Format(time.RFC3339)

	createOut := &bytes.Buffer{}
	createErr := &bytes.Buffer{}
	createCmd := NewIGCommand(testRuntime("prod"))
	createCmd.SilenceErrors = true
	createCmd.SilenceUsage = true
	createCmd.SetOut(createOut)
	createCmd.SetErr(createErr)
	createCmd.SetArgs([]string{
		"publish", "reel",
		"--ig-user-id", "17841400008460056",
		"--media-url", "https://cdn.example.com/reel.mp4",
		"--caption", "hello #reel",
		"--media-type", "REELS",
		"--publish-at", publishAt,
		"--schedule-state-path", statePath,
	})
	if err := createCmd.Execute(); err != nil {
		t.Fatalf("create scheduled reel: %v", err)
	}

	listOut := &bytes.Buffer{}
	listErr := &bytes.Buffer{}
	listCmd := NewIGCommand(testRuntime("prod"))
	listCmd.SilenceErrors = true
	listCmd.SilenceUsage = true
	listCmd.SetOut(listOut)
	listCmd.SetErr(listErr)
	listCmd.SetArgs([]string{
		"publish", "schedule", "list",
		"--schedule-state-path", statePath,
	})
	if err := listCmd.Execute(); err != nil {
		t.Fatalf("list schedules: %v", err)
	}

	listEnvelope := decodeEnvelope(t, listOut.Bytes())
	assertEnvelopeBasics(t, listEnvelope, "meta ig publish schedule list")
	listData, ok := listEnvelope["data"].(map[string]any)
	if !ok {
		t.Fatalf("expected object payload, got %T", listEnvelope["data"])
	}
	schedules, ok := listData["schedules"].([]any)
	if !ok || len(schedules) != 1 {
		t.Fatalf("expected one scheduled item, got %#v", listData["schedules"])
	}
	firstSchedule, ok := schedules[0].(map[string]any)
	if !ok {
		t.Fatalf("expected schedule object, got %T", schedules[0])
	}
	scheduleID, _ := firstSchedule["schedule_id"].(string)
	if strings.TrimSpace(scheduleID) == "" {
		t.Fatal("expected schedule_id")
	}

	cancelOut := &bytes.Buffer{}
	cancelErr := &bytes.Buffer{}
	cancelCmd := NewIGCommand(testRuntime("prod"))
	cancelCmd.SilenceErrors = true
	cancelCmd.SilenceUsage = true
	cancelCmd.SetOut(cancelOut)
	cancelCmd.SetErr(cancelErr)
	cancelCmd.SetArgs([]string{
		"publish", "schedule", "cancel",
		"--schedule-id", scheduleID,
		"--schedule-state-path", statePath,
	})
	if err := cancelCmd.Execute(); err != nil {
		t.Fatalf("cancel schedule: %v", err)
	}
	cancelEnvelope := decodeEnvelope(t, cancelOut.Bytes())
	assertEnvelopeBasics(t, cancelEnvelope, "meta ig publish schedule cancel")
	cancelData, ok := cancelEnvelope["data"].(map[string]any)
	if !ok {
		t.Fatalf("expected object payload, got %T", cancelEnvelope["data"])
	}
	canceledSchedule, ok := cancelData["schedule"].(map[string]any)
	if !ok {
		t.Fatalf("expected schedule object, got %T", cancelData["schedule"])
	}
	if got := canceledSchedule["status"]; got != "canceled" {
		t.Fatalf("unexpected cancel status %v", got)
	}

	retryOut := &bytes.Buffer{}
	retryErr := &bytes.Buffer{}
	retryCmd := NewIGCommand(testRuntime("prod"))
	retryCmd.SilenceErrors = true
	retryCmd.SilenceUsage = true
	retryCmd.SetOut(retryOut)
	retryCmd.SetErr(retryErr)
	retryCmd.SetArgs([]string{
		"publish", "schedule", "retry",
		"--schedule-id", scheduleID,
		"--publish-at", time.Now().UTC().Add(4 * time.Hour).Format(time.RFC3339),
		"--schedule-state-path", statePath,
	})
	if err := retryCmd.Execute(); err != nil {
		t.Fatalf("retry schedule: %v", err)
	}

	retryEnvelope := decodeEnvelope(t, retryOut.Bytes())
	assertEnvelopeBasics(t, retryEnvelope, "meta ig publish schedule retry")
	retryData, ok := retryEnvelope["data"].(map[string]any)
	if !ok {
		t.Fatalf("expected object payload, got %T", retryEnvelope["data"])
	}
	retriedSchedule, ok := retryData["schedule"].(map[string]any)
	if !ok {
		t.Fatalf("expected schedule object, got %T", retryData["schedule"])
	}
	if got := retriedSchedule["status"]; got != "scheduled" {
		t.Fatalf("unexpected retry status %v", got)
	}
	if got := retriedSchedule["retry_count"]; got != float64(1) {
		t.Fatalf("unexpected retry_count %v", got)
	}

	if wasCalled {
		t.Fatal("graph client should not execute for schedule lifecycle commands")
	}
}

func TestIGPublishScheduleCancelRejectsInvalidTransition(t *testing.T) {
	useIGDependencies(t,
		func(string) (*ProfileCredentials, error) {
			return &ProfileCredentials{
				Name:      "prod",
				Profile:   config.Profile{GraphVersion: "v25.0"},
				Token:     "test-token",
				AppSecret: "test-secret",
			}, nil
		},
		func() *graph.Client {
			return graph.NewClient(nil, "")
		},
	)

	statePath := t.TempDir() + "/ig-schedules.json"
	publishAt := time.Now().UTC().Add(2 * time.Hour).Format(time.RFC3339)

	createCmd := NewIGCommand(testRuntime("prod"))
	createCmd.SilenceErrors = true
	createCmd.SilenceUsage = true
	createCmd.SetOut(&bytes.Buffer{})
	createCmd.SetErr(&bytes.Buffer{})
	createCmd.SetArgs([]string{
		"publish", "story",
		"--ig-user-id", "17841400008460056",
		"--media-url", "https://cdn.example.com/story.mp4",
		"--caption", "hello #story",
		"--media-type", "STORIES",
		"--publish-at", publishAt,
		"--schedule-state-path", statePath,
	})
	if err := createCmd.Execute(); err != nil {
		t.Fatalf("create schedule: %v", err)
	}

	listOut := &bytes.Buffer{}
	listCmd := NewIGCommand(testRuntime("prod"))
	listCmd.SilenceErrors = true
	listCmd.SilenceUsage = true
	listCmd.SetOut(listOut)
	listCmd.SetErr(&bytes.Buffer{})
	listCmd.SetArgs([]string{
		"publish", "schedule", "list",
		"--schedule-state-path", statePath,
	})
	if err := listCmd.Execute(); err != nil {
		t.Fatalf("list schedules: %v", err)
	}
	listEnvelope := decodeEnvelope(t, listOut.Bytes())
	listData, _ := listEnvelope["data"].(map[string]any)
	schedules, _ := listData["schedules"].([]any)
	firstSchedule, _ := schedules[0].(map[string]any)
	scheduleID, _ := firstSchedule["schedule_id"].(string)

	cancelCmd := NewIGCommand(testRuntime("prod"))
	cancelCmd.SilenceErrors = true
	cancelCmd.SilenceUsage = true
	cancelCmd.SetOut(&bytes.Buffer{})
	cancelCmd.SetErr(&bytes.Buffer{})
	cancelCmd.SetArgs([]string{
		"publish", "schedule", "cancel",
		"--schedule-id", scheduleID,
		"--schedule-state-path", statePath,
	})
	if err := cancelCmd.Execute(); err != nil {
		t.Fatalf("cancel schedule: %v", err)
	}

	errOut := &bytes.Buffer{}
	cancelAgain := NewIGCommand(testRuntime("prod"))
	cancelAgain.SilenceErrors = true
	cancelAgain.SilenceUsage = true
	cancelAgain.SetOut(&bytes.Buffer{})
	cancelAgain.SetErr(errOut)
	cancelAgain.SetArgs([]string{
		"publish", "schedule", "cancel",
		"--schedule-id", scheduleID,
		"--schedule-state-path", statePath,
	})

	err := cancelAgain.Execute()
	if err == nil {
		t.Fatal("expected transition error")
	}
	if !strings.Contains(err.Error(), "cannot transition from canceled to canceled") {
		t.Fatalf("unexpected error: %v", err)
	}
	envelope := decodeEnvelope(t, errOut.Bytes())
	if got := envelope["command"]; got != "meta ig publish schedule cancel" {
		t.Fatalf("unexpected command field %v", got)
	}
	if envelope["success"] != false {
		t.Fatalf("expected success=false, got %v", envelope["success"])
	}
}

func TestIGPublishFeedScheduleSuppressesDuplicateByIdempotencyKey(t *testing.T) {
	wasCalled := false
	useIGDependencies(t,
		func(string) (*ProfileCredentials, error) {
			return &ProfileCredentials{
				Name:      "prod",
				Profile:   config.Profile{GraphVersion: "v25.0"},
				Token:     "test-token",
				AppSecret: "test-secret",
			}, nil
		},
		func() *graph.Client {
			wasCalled = true
			return graph.NewClient(nil, "")
		},
	)

	statePath := t.TempDir() + "/ig-schedules.json"
	publishAt := time.Now().UTC().Add(2 * time.Hour).Format(time.RFC3339)

	firstOut := &bytes.Buffer{}
	firstCmd := NewIGCommand(testRuntime("prod"))
	firstCmd.SilenceErrors = true
	firstCmd.SilenceUsage = true
	firstCmd.SetOut(firstOut)
	firstCmd.SetErr(&bytes.Buffer{})
	firstCmd.SetArgs([]string{
		"publish", "feed",
		"--ig-user-id", "17841400008460056",
		"--media-url", "https://cdn.example.com/feed.jpg",
		"--caption", "hello #meta",
		"--media-type", "IMAGE",
		"--idempotency-key", "feed_01",
		"--publish-at", publishAt,
		"--schedule-state-path", statePath,
	})
	if err := firstCmd.Execute(); err != nil {
		t.Fatalf("create schedule: %v", err)
	}

	secondOut := &bytes.Buffer{}
	secondCmd := NewIGCommand(testRuntime("prod"))
	secondCmd.SilenceErrors = true
	secondCmd.SilenceUsage = true
	secondCmd.SetOut(secondOut)
	secondCmd.SetErr(&bytes.Buffer{})
	secondCmd.SetArgs([]string{
		"publish", "feed",
		"--ig-user-id", "17841400008460056",
		"--media-url", "https://cdn.example.com/feed.jpg",
		"--caption", "hello #meta",
		"--media-type", "IMAGE",
		"--idempotency-key", "feed_01",
		"--publish-at", publishAt,
		"--schedule-state-path", statePath,
	})
	if err := secondCmd.Execute(); err != nil {
		t.Fatalf("create duplicate schedule: %v", err)
	}

	firstEnvelope := decodeEnvelope(t, firstOut.Bytes())
	firstData, _ := firstEnvelope["data"].(map[string]any)
	firstSchedule, _ := firstData["schedule"].(map[string]any)
	firstScheduleID, _ := firstSchedule["schedule_id"].(string)

	secondEnvelope := decodeEnvelope(t, secondOut.Bytes())
	secondData, _ := secondEnvelope["data"].(map[string]any)
	if got := secondData["duplicate_suppressed"]; got != true {
		t.Fatalf("expected duplicate_suppressed=true, got %v", got)
	}
	secondSchedule, _ := secondData["schedule"].(map[string]any)
	secondScheduleID, _ := secondSchedule["schedule_id"].(string)
	if secondScheduleID != firstScheduleID {
		t.Fatalf("expected same schedule_id %q, got %q", firstScheduleID, secondScheduleID)
	}

	listOut := &bytes.Buffer{}
	listCmd := NewIGCommand(testRuntime("prod"))
	listCmd.SilenceErrors = true
	listCmd.SilenceUsage = true
	listCmd.SetOut(listOut)
	listCmd.SetErr(&bytes.Buffer{})
	listCmd.SetArgs([]string{
		"publish", "schedule", "list",
		"--schedule-state-path", statePath,
	})
	if err := listCmd.Execute(); err != nil {
		t.Fatalf("list schedules: %v", err)
	}
	listEnvelope := decodeEnvelope(t, listOut.Bytes())
	listData, _ := listEnvelope["data"].(map[string]any)
	if got := listData["total"]; got != float64(1) {
		t.Fatalf("expected total=1, got %v", got)
	}

	if wasCalled {
		t.Fatal("graph client should not execute for scheduled publish")
	}
}

func TestIGPublishFeedScheduleSuppressesDuplicateAcrossRerunsWithoutExplicitIdempotencyKey(t *testing.T) {
	wasCalled := false
	useIGDependencies(t,
		func(string) (*ProfileCredentials, error) {
			return &ProfileCredentials{
				Name:      "prod",
				Profile:   config.Profile{GraphVersion: "v25.0"},
				Token:     "test-token",
				AppSecret: "test-secret",
			}, nil
		},
		func() *graph.Client {
			wasCalled = true
			return graph.NewClient(nil, "")
		},
	)

	statePath := t.TempDir() + "/ig-schedules.json"
	publishAt := time.Now().UTC().Add(2 * time.Hour).Format(time.RFC3339)

	firstOut := &bytes.Buffer{}
	firstCmd := NewIGCommand(testRuntime("prod"))
	firstCmd.SilenceErrors = true
	firstCmd.SilenceUsage = true
	firstCmd.SetOut(firstOut)
	firstCmd.SetErr(&bytes.Buffer{})
	firstCmd.SetArgs([]string{
		"publish", "feed",
		"--ig-user-id", "17841400008460056",
		"--media-url", "https://cdn.example.com/feed.jpg",
		"--caption", "hello #meta",
		"--media-type", "IMAGE",
		"--publish-at", publishAt,
		"--schedule-state-path", statePath,
	})
	if err := firstCmd.Execute(); err != nil {
		t.Fatalf("create schedule: %v", err)
	}

	secondOut := &bytes.Buffer{}
	secondCmd := NewIGCommand(testRuntime("prod"))
	secondCmd.SilenceErrors = true
	secondCmd.SilenceUsage = true
	secondCmd.SetOut(secondOut)
	secondCmd.SetErr(&bytes.Buffer{})
	secondCmd.SetArgs([]string{
		"publish", "feed",
		"--ig-user-id", "17841400008460056",
		"--media-url", "https://cdn.example.com/feed.jpg",
		"--caption", "hello #meta",
		"--media-type", "IMAGE",
		"--publish-at", publishAt,
		"--schedule-state-path", statePath,
	})
	if err := secondCmd.Execute(); err != nil {
		t.Fatalf("create duplicate schedule on rerun: %v", err)
	}

	firstEnvelope := decodeEnvelope(t, firstOut.Bytes())
	firstData, _ := firstEnvelope["data"].(map[string]any)
	firstSchedule, _ := firstData["schedule"].(map[string]any)
	firstScheduleID, _ := firstSchedule["schedule_id"].(string)

	secondEnvelope := decodeEnvelope(t, secondOut.Bytes())
	secondData, _ := secondEnvelope["data"].(map[string]any)
	if got := secondData["duplicate_suppressed"]; got != true {
		t.Fatalf("expected duplicate_suppressed=true, got %v", got)
	}
	secondSchedule, _ := secondData["schedule"].(map[string]any)
	secondScheduleID, _ := secondSchedule["schedule_id"].(string)
	if secondScheduleID != firstScheduleID {
		t.Fatalf("expected same schedule_id %q, got %q", firstScheduleID, secondScheduleID)
	}

	listOut := &bytes.Buffer{}
	listCmd := NewIGCommand(testRuntime("prod"))
	listCmd.SilenceErrors = true
	listCmd.SilenceUsage = true
	listCmd.SetOut(listOut)
	listCmd.SetErr(&bytes.Buffer{})
	listCmd.SetArgs([]string{
		"publish", "schedule", "list",
		"--schedule-state-path", statePath,
	})
	if err := listCmd.Execute(); err != nil {
		t.Fatalf("list schedules: %v", err)
	}
	listEnvelope := decodeEnvelope(t, listOut.Bytes())
	listData, _ := listEnvelope["data"].(map[string]any)
	if got := listData["total"]; got != float64(1) {
		t.Fatalf("expected total=1, got %v", got)
	}

	if wasCalled {
		t.Fatal("graph client should not execute for scheduled publish")
	}
}

func TestIGPublishFeedScheduleIdempotencyConflictWritesStructuredError(t *testing.T) {
	useIGDependencies(t,
		func(string) (*ProfileCredentials, error) {
			return &ProfileCredentials{
				Name:      "prod",
				Profile:   config.Profile{GraphVersion: "v25.0"},
				Token:     "test-token",
				AppSecret: "test-secret",
			}, nil
		},
		func() *graph.Client {
			return graph.NewClient(nil, "")
		},
	)

	statePath := t.TempDir() + "/ig-schedules.json"
	publishAt := time.Now().UTC().Add(2 * time.Hour).Format(time.RFC3339)

	createCmd := NewIGCommand(testRuntime("prod"))
	createCmd.SilenceErrors = true
	createCmd.SilenceUsage = true
	createCmd.SetOut(&bytes.Buffer{})
	createCmd.SetErr(&bytes.Buffer{})
	createCmd.SetArgs([]string{
		"publish", "feed",
		"--ig-user-id", "17841400008460056",
		"--media-url", "https://cdn.example.com/feed-1.jpg",
		"--caption", "hello #meta",
		"--media-type", "IMAGE",
		"--idempotency-key", "feed_01",
		"--publish-at", publishAt,
		"--schedule-state-path", statePath,
	})
	if err := createCmd.Execute(); err != nil {
		t.Fatalf("create schedule: %v", err)
	}

	errOut := &bytes.Buffer{}
	conflictCmd := NewIGCommand(testRuntime("prod"))
	conflictCmd.SilenceErrors = true
	conflictCmd.SilenceUsage = true
	conflictCmd.SetOut(&bytes.Buffer{})
	conflictCmd.SetErr(errOut)
	conflictCmd.SetArgs([]string{
		"publish", "feed",
		"--ig-user-id", "17841400008460056",
		"--media-url", "https://cdn.example.com/feed-2.jpg",
		"--caption", "hello #meta",
		"--media-type", "IMAGE",
		"--idempotency-key", "feed_01",
		"--publish-at", publishAt,
		"--schedule-state-path", statePath,
	})

	err := conflictCmd.Execute()
	if err == nil {
		t.Fatal("expected idempotency conflict")
	}
	envelope := decodeEnvelope(t, errOut.Bytes())
	errorBody, ok := envelope["error"].(map[string]any)
	if !ok {
		t.Fatalf("expected error payload, got %T", envelope["error"])
	}
	if got := errorBody["type"]; got != "ig_idempotency_conflict" {
		t.Fatalf("unexpected error type %v", got)
	}
	if got := errorBody["retryable"]; got != false {
		t.Fatalf("expected retryable=false, got %v", got)
	}
}
