package ig

import (
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestScheduleServiceLifecycleTransitions(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 2, 23, 9, 0, 0, 0, time.UTC)
	statePath := filepath.Join(t.TempDir(), "schedules.json")
	service := NewScheduleService(statePath)
	service.Now = func() time.Time {
		return now
	}

	scheduledAt := now.Add(2 * time.Hour).Format(time.RFC3339)
	scheduled, err := service.Schedule(SchedulePublishOptions{
		Profile:    "prod",
		Version:    "v25.0",
		Surface:    PublishSurfaceFeed,
		IGUserID:   "17841400008460056",
		MediaURL:   "https://cdn.example.com/feed.jpg",
		Caption:    "hello #meta",
		MediaType:  MediaTypeImage,
		StrictMode: true,
		PublishAt:  scheduledAt,
	})
	if err != nil {
		t.Fatalf("schedule publish: %v", err)
	}

	if scheduled.Mode != "scheduled" {
		t.Fatalf("unexpected mode %q", scheduled.Mode)
	}
	if scheduled.Schedule.Status != ScheduleStatusScheduled {
		t.Fatalf("unexpected status %q", scheduled.Schedule.Status)
	}
	if scheduled.Schedule.RetryCount != 0 {
		t.Fatalf("unexpected retry count %d", scheduled.Schedule.RetryCount)
	}

	list, err := service.List(ScheduleListOptions{})
	if err != nil {
		t.Fatalf("list schedules: %v", err)
	}
	if list.Total != 1 {
		t.Fatalf("unexpected total %d", list.Total)
	}
	if list.Schedules[0].Status != ScheduleStatusScheduled {
		t.Fatalf("unexpected listed status %q", list.Schedules[0].Status)
	}

	canceled, err := service.Cancel(ScheduleCancelOptions{
		ScheduleID: scheduled.Schedule.ScheduleID,
	})
	if err != nil {
		t.Fatalf("cancel schedule: %v", err)
	}
	if canceled.Schedule.Status != ScheduleStatusCanceled {
		t.Fatalf("unexpected cancel status %q", canceled.Schedule.Status)
	}

	retryAt := now.Add(3 * time.Hour).Format(time.RFC3339)
	retried, err := service.Retry(ScheduleRetryOptions{
		ScheduleID: scheduled.Schedule.ScheduleID,
		PublishAt:  retryAt,
	})
	if err != nil {
		t.Fatalf("retry schedule: %v", err)
	}
	if retried.Schedule.Status != ScheduleStatusScheduled {
		t.Fatalf("unexpected retry status %q", retried.Schedule.Status)
	}
	if retried.Schedule.RetryCount != 1 {
		t.Fatalf("unexpected retry count %d", retried.Schedule.RetryCount)
	}
	if retried.Schedule.PublishAt != retryAt {
		t.Fatalf("unexpected retry publish_at %q", retried.Schedule.PublishAt)
	}
}

func TestScheduleServiceDeterministicStateTransitions(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 2, 23, 9, 0, 0, 0, time.UTC)
	statePath := filepath.Join(t.TempDir(), "schedules.json")
	service := NewScheduleService(statePath)
	service.Now = func() time.Time {
		return now
	}

	scheduledAt := now.Add(30 * time.Minute).Format(time.RFC3339)
	scheduled, err := service.Schedule(SchedulePublishOptions{
		Profile:    "prod",
		Version:    "v25.0",
		Surface:    PublishSurfaceStory,
		IGUserID:   "17841400008460056",
		MediaURL:   "https://cdn.example.com/story.mp4",
		Caption:    "hello #story",
		MediaType:  MediaTypeStories,
		StrictMode: true,
		PublishAt:  scheduledAt,
	})
	if err != nil {
		t.Fatalf("schedule publish: %v", err)
	}

	now = now.Add(2 * time.Hour)

	list, err := service.List(ScheduleListOptions{})
	if err != nil {
		t.Fatalf("list schedules: %v", err)
	}
	if list.Total != 1 {
		t.Fatalf("unexpected total %d", list.Total)
	}
	if list.Schedules[0].Status != ScheduleStatusFailed {
		t.Fatalf("unexpected status %q", list.Schedules[0].Status)
	}
	if list.Schedules[0].LastError != missedScheduleError {
		t.Fatalf("unexpected last_error %q", list.Schedules[0].LastError)
	}

	_, err = service.Cancel(ScheduleCancelOptions{
		ScheduleID: scheduled.Schedule.ScheduleID,
	})
	if err == nil {
		t.Fatal("expected cancel transition error")
	}
	if !strings.Contains(err.Error(), "cannot transition from failed to canceled") {
		t.Fatalf("unexpected cancel error: %v", err)
	}

	_, err = service.Retry(ScheduleRetryOptions{
		ScheduleID: scheduled.Schedule.ScheduleID,
	})
	if err == nil {
		t.Fatal("expected retry validation error")
	}
	if !strings.Contains(err.Error(), "publish-at must be in the future") {
		t.Fatalf("unexpected retry error: %v", err)
	}

	retried, err := service.Retry(ScheduleRetryOptions{
		ScheduleID: scheduled.Schedule.ScheduleID,
		PublishAt:  now.Add(2 * time.Hour).Format(time.RFC3339),
	})
	if err != nil {
		t.Fatalf("retry schedule: %v", err)
	}
	if retried.Schedule.Status != ScheduleStatusScheduled {
		t.Fatalf("unexpected retry status %q", retried.Schedule.Status)
	}
	if retried.Schedule.RetryCount != 1 {
		t.Fatalf("unexpected retry count %d", retried.Schedule.RetryCount)
	}
}
