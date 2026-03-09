package ig

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/bilalbayram/metacli/internal/graph"
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

func TestScheduleServiceSuppressesDuplicateByIdempotencyKey(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 2, 23, 9, 0, 0, 0, time.UTC)
	statePath := filepath.Join(t.TempDir(), "schedules.json")
	service := NewScheduleService(statePath)
	service.Now = func() time.Time {
		return now
	}

	options := SchedulePublishOptions{
		Profile:        "prod",
		Version:        "v25.0",
		Surface:        PublishSurfaceFeed,
		IdempotencyKey: "feed_01",
		IGUserID:       "17841400008460056",
		MediaURL:       "https://cdn.example.com/feed.jpg",
		Caption:        "hello #meta",
		MediaType:      MediaTypeImage,
		StrictMode:     true,
		PublishAt:      now.Add(2 * time.Hour).Format(time.RFC3339),
	}

	first, err := service.Schedule(options)
	if err != nil {
		t.Fatalf("schedule publish: %v", err)
	}
	if first.DuplicateSuppressed {
		t.Fatal("first schedule should not be duplicate-suppressed")
	}

	second, err := service.Schedule(options)
	if err != nil {
		t.Fatalf("schedule duplicate publish: %v", err)
	}
	if !second.DuplicateSuppressed {
		t.Fatal("expected duplicate_suppressed=true")
	}
	if second.Schedule.ScheduleID != first.Schedule.ScheduleID {
		t.Fatalf("expected duplicate to reuse schedule id %q, got %q", first.Schedule.ScheduleID, second.Schedule.ScheduleID)
	}

	list, err := service.List(ScheduleListOptions{})
	if err != nil {
		t.Fatalf("list schedules: %v", err)
	}
	if list.Total != 1 {
		t.Fatalf("expected one persisted schedule, got %d", list.Total)
	}
}

func TestScheduleServiceSuppressesDuplicateAcrossRerunsWithoutExplicitIdempotencyKey(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 2, 23, 9, 0, 0, 0, time.UTC)
	statePath := filepath.Join(t.TempDir(), "schedules.json")
	service := NewScheduleService(statePath)
	service.Now = func() time.Time {
		return now
	}

	options := SchedulePublishOptions{
		Profile:    "prod",
		Version:    "v25.0",
		Surface:    PublishSurfaceFeed,
		IGUserID:   "17841400008460056",
		MediaURL:   "https://cdn.example.com/feed.jpg",
		Caption:    "hello #meta",
		MediaType:  MediaTypeImage,
		StrictMode: true,
		PublishAt:  now.Add(2 * time.Hour).Format(time.RFC3339),
	}

	first, err := service.Schedule(options)
	if err != nil {
		t.Fatalf("schedule publish: %v", err)
	}
	if first.DuplicateSuppressed {
		t.Fatal("first schedule should not be duplicate-suppressed")
	}
	if strings.TrimSpace(first.Schedule.IdempotencyKey) == "" {
		t.Fatal("expected deterministic idempotency_key for scheduled publish")
	}

	rerunService := NewScheduleService(statePath)
	rerunService.Now = func() time.Time {
		return now
	}
	second, err := rerunService.Schedule(options)
	if err != nil {
		t.Fatalf("schedule duplicate publish on rerun: %v", err)
	}
	if !second.DuplicateSuppressed {
		t.Fatal("expected duplicate_suppressed=true on rerun")
	}
	if second.Schedule.ScheduleID != first.Schedule.ScheduleID {
		t.Fatalf("expected duplicate to reuse schedule id %q, got %q", first.Schedule.ScheduleID, second.Schedule.ScheduleID)
	}

	list, err := rerunService.List(ScheduleListOptions{})
	if err != nil {
		t.Fatalf("list schedules: %v", err)
	}
	if list.Total != 1 {
		t.Fatalf("expected one persisted schedule, got %d", list.Total)
	}
}

func TestScheduleServiceRejectsIdempotencyConflict(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 2, 23, 9, 0, 0, 0, time.UTC)
	statePath := filepath.Join(t.TempDir(), "schedules.json")
	service := NewScheduleService(statePath)
	service.Now = func() time.Time {
		return now
	}

	_, err := service.Schedule(SchedulePublishOptions{
		Profile:        "prod",
		Version:        "v25.0",
		Surface:        PublishSurfaceFeed,
		IdempotencyKey: "feed_01",
		IGUserID:       "17841400008460056",
		MediaURL:       "https://cdn.example.com/feed-1.jpg",
		Caption:        "hello #meta",
		MediaType:      MediaTypeImage,
		StrictMode:     true,
		PublishAt:      now.Add(2 * time.Hour).Format(time.RFC3339),
	})
	if err != nil {
		t.Fatalf("schedule publish: %v", err)
	}

	_, err = service.Schedule(SchedulePublishOptions{
		Profile:        "prod",
		Version:        "v25.0",
		Surface:        PublishSurfaceFeed,
		IdempotencyKey: "feed_01",
		IGUserID:       "17841400008460056",
		MediaURL:       "https://cdn.example.com/feed-2.jpg",
		Caption:        "hello #meta",
		MediaType:      MediaTypeImage,
		StrictMode:     true,
		PublishAt:      now.Add(2 * time.Hour).Format(time.RFC3339),
	})
	if err == nil {
		t.Fatal("expected idempotency conflict")
	}

	var apiErr *graph.APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("expected structured api error, got %T", err)
	}
	if apiErr.Type != igErrorTypeIdempotencyConflict {
		t.Fatalf("unexpected error type %q", apiErr.Type)
	}
	if apiErr.Retryable {
		t.Fatalf("expected retryable=false, got %+v", apiErr)
	}
}

func scheduleTestRecord(service *ScheduleService, now time.Time, surface string, mediaType string, offset time.Duration) (*SchedulePublishResult, error) {
	return service.Schedule(SchedulePublishOptions{
		Profile:    "prod",
		Version:    "v25.0",
		Surface:    surface,
		IGUserID:   "17841400008460056",
		MediaURL:   "https://cdn.example.com/media-" + surface,
		Caption:    "hello #" + surface,
		MediaType:  mediaType,
		StrictMode: true,
		PublishAt:  now.Add(offset).Format(time.RFC3339),
	})
}

func TestScheduleServiceExecuteDueCompletesRecords(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 2, 23, 9, 0, 0, 0, time.UTC)
	statePath := filepath.Join(t.TempDir(), "schedules.json")
	service := NewScheduleService(statePath)
	service.Now = func() time.Time { return now }

	if _, err := scheduleTestRecord(service, now, PublishSurfaceFeed, MediaTypeImage, 30*time.Minute); err != nil {
		t.Fatalf("schedule feed: %v", err)
	}
	if _, err := scheduleTestRecord(service, now, PublishSurfaceReel, MediaTypeReels, 45*time.Minute); err != nil {
		t.Fatalf("schedule reel: %v", err)
	}

	now = now.Add(2 * time.Hour)

	callCount := 0
	result, err := service.ExecuteDue(context.Background(), ScheduleExecuteOptions{}, func(_ context.Context, record ScheduledPublishRecord) (string, error) {
		callCount++
		return "media_" + record.Surface, nil
	})
	if err != nil {
		t.Fatalf("execute due: %v", err)
	}

	if callCount != 2 {
		t.Fatalf("expected 2 publish calls, got %d", callCount)
	}
	if result.Total != 2 {
		t.Fatalf("expected total=2, got %d", result.Total)
	}
	if result.Completed != 2 {
		t.Fatalf("expected completed=2, got %d", result.Completed)
	}
	if result.Failed != 0 {
		t.Fatalf("expected failed=0, got %d", result.Failed)
	}
	for _, rec := range result.Records {
		if rec.Status != ScheduleStatusCompleted {
			t.Fatalf("expected completed status, got %q", rec.Status)
		}
		if rec.MediaID != "media_"+rec.Surface {
			t.Fatalf("unexpected media_id %q", rec.MediaID)
		}
	}

	list, err := service.List(ScheduleListOptions{Status: ScheduleStatusCompleted})
	if err != nil {
		t.Fatalf("list completed: %v", err)
	}
	if list.Total != 2 {
		t.Fatalf("expected 2 completed records in state, got %d", list.Total)
	}
}

func TestScheduleServiceExecuteDueMarksFailedOnError(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 2, 23, 9, 0, 0, 0, time.UTC)
	statePath := filepath.Join(t.TempDir(), "schedules.json")
	service := NewScheduleService(statePath)
	service.Now = func() time.Time { return now }

	if _, err := scheduleTestRecord(service, now, PublishSurfaceFeed, MediaTypeImage, 30*time.Minute); err != nil {
		t.Fatalf("schedule feed: %v", err)
	}

	now = now.Add(2 * time.Hour)

	result, err := service.ExecuteDue(context.Background(), ScheduleExecuteOptions{}, func(_ context.Context, _ ScheduledPublishRecord) (string, error) {
		return "", errors.New("token expired")
	})
	if err != nil {
		t.Fatalf("execute due: %v", err)
	}

	if result.Total != 1 {
		t.Fatalf("expected total=1, got %d", result.Total)
	}
	if result.Failed != 1 {
		t.Fatalf("expected failed=1, got %d", result.Failed)
	}
	if result.Records[0].Status != ScheduleStatusFailed {
		t.Fatalf("expected failed status, got %q", result.Records[0].Status)
	}
	if result.Records[0].Error != "token expired" {
		t.Fatalf("unexpected error %q", result.Records[0].Error)
	}

	list, err := service.List(ScheduleListOptions{Status: ScheduleStatusFailed})
	if err != nil {
		t.Fatalf("list failed: %v", err)
	}
	if list.Total != 1 {
		t.Fatalf("expected 1 failed record in state, got %d", list.Total)
	}
	if list.Schedules[0].LastError != "token expired" {
		t.Fatalf("unexpected last_error in state %q", list.Schedules[0].LastError)
	}
}

func TestScheduleServiceExecuteDueSkipsFutureRecords(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 2, 23, 9, 0, 0, 0, time.UTC)
	statePath := filepath.Join(t.TempDir(), "schedules.json")
	service := NewScheduleService(statePath)
	service.Now = func() time.Time { return now }

	if _, err := scheduleTestRecord(service, now, PublishSurfaceFeed, MediaTypeImage, 2*time.Hour); err != nil {
		t.Fatalf("schedule feed: %v", err)
	}

	result, err := service.ExecuteDue(context.Background(), ScheduleExecuteOptions{}, func(_ context.Context, _ ScheduledPublishRecord) (string, error) {
		t.Fatal("publish should not be called for future records")
		return "", nil
	})
	if err != nil {
		t.Fatalf("execute due: %v", err)
	}

	if result.Total != 0 {
		t.Fatalf("expected total=0, got %d", result.Total)
	}
}

func TestScheduleServiceExecuteDueDryRun(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 2, 23, 9, 0, 0, 0, time.UTC)
	statePath := filepath.Join(t.TempDir(), "schedules.json")
	service := NewScheduleService(statePath)
	service.Now = func() time.Time { return now }

	if _, err := scheduleTestRecord(service, now, PublishSurfaceFeed, MediaTypeImage, 30*time.Minute); err != nil {
		t.Fatalf("schedule feed: %v", err)
	}

	now = now.Add(2 * time.Hour)

	result, err := service.ExecuteDue(context.Background(), ScheduleExecuteOptions{DryRun: true}, nil)
	if err != nil {
		t.Fatalf("execute due dry-run: %v", err)
	}

	if !result.DryRun {
		t.Fatal("expected dry_run=true")
	}
	if result.Total != 1 {
		t.Fatalf("expected total=1, got %d", result.Total)
	}
	if result.Skipped != 1 {
		t.Fatalf("expected skipped=1, got %d", result.Skipped)
	}
	if result.Completed != 0 {
		t.Fatalf("expected completed=0, got %d", result.Completed)
	}

	list, err := service.List(ScheduleListOptions{})
	if err != nil {
		t.Fatalf("list schedules: %v", err)
	}
	for _, s := range list.Schedules {
		if s.Status == ScheduleStatusCompleted {
			t.Fatal("dry-run should not transition records to completed")
		}
	}
}

func TestScheduleServiceExecuteDueRespectsLimit(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 2, 23, 9, 0, 0, 0, time.UTC)
	statePath := filepath.Join(t.TempDir(), "schedules.json")
	service := NewScheduleService(statePath)
	service.Now = func() time.Time { return now }

	for i := 0; i < 3; i++ {
		if _, err := service.Schedule(SchedulePublishOptions{
			Profile:    "prod",
			Version:    "v25.0",
			Surface:    PublishSurfaceFeed,
			IGUserID:   "17841400008460056",
			MediaURL:   "https://cdn.example.com/feed-" + strings.Repeat("x", i+1),
			Caption:    "hello #meta",
			MediaType:  MediaTypeImage,
			StrictMode: true,
			PublishAt:  now.Add(30 * time.Minute).Format(time.RFC3339),
		}); err != nil {
			t.Fatalf("schedule %d: %v", i, err)
		}
	}

	now = now.Add(2 * time.Hour)

	result, err := service.ExecuteDue(context.Background(), ScheduleExecuteOptions{Limit: 2}, func(_ context.Context, _ ScheduledPublishRecord) (string, error) {
		return "media_123", nil
	})
	if err != nil {
		t.Fatalf("execute due: %v", err)
	}

	if result.Total != 2 {
		t.Fatalf("expected total=2, got %d", result.Total)
	}
	if result.Completed != 2 {
		t.Fatalf("expected completed=2, got %d", result.Completed)
	}

	list, err := service.List(ScheduleListOptions{Status: ScheduleStatusScheduled})
	if err != nil {
		t.Fatalf("list scheduled: %v", err)
	}
	if list.Total != 0 {
		t.Fatalf("expected 0 remaining scheduled (reconciled as failed), got %d", list.Total)
	}
}

func TestScheduleServiceExecuteDueOrdersByPublishAtBeforeApplyingLimit(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 2, 23, 9, 0, 0, 0, time.UTC)
	statePath := filepath.Join(t.TempDir(), "schedules.json")
	service := NewScheduleService(statePath)
	service.Now = func() time.Time { return now }

	later, err := scheduleTestRecord(service, now, PublishSurfaceFeed, MediaTypeImage, 2*time.Hour)
	if err != nil {
		t.Fatalf("schedule later record: %v", err)
	}
	earlier, err := scheduleTestRecord(service, now, PublishSurfaceReel, MediaTypeReels, 1*time.Hour)
	if err != nil {
		t.Fatalf("schedule earlier record: %v", err)
	}

	now = now.Add(3 * time.Hour)

	var executed []string
	result, err := service.ExecuteDue(context.Background(), ScheduleExecuteOptions{Limit: 1}, func(_ context.Context, record ScheduledPublishRecord) (string, error) {
		executed = append(executed, record.ScheduleID)
		return "media_" + record.Surface, nil
	})
	if err != nil {
		t.Fatalf("execute due: %v", err)
	}

	if result.Total != 1 {
		t.Fatalf("expected total=1, got %d", result.Total)
	}
	if len(executed) != 1 {
		t.Fatalf("expected one executed record, got %d", len(executed))
	}
	if executed[0] != earlier.Schedule.ScheduleID {
		t.Fatalf("expected earliest publish_at record %q, got %q", earlier.Schedule.ScheduleID, executed[0])
	}

	state, err := loadScheduleState(statePath)
	if err != nil {
		t.Fatalf("load state: %v", err)
	}

	completed := map[string]ScheduledPublishRecord{}
	for _, record := range state.Schedules {
		completed[record.ScheduleID] = record
	}
	if got := completed[earlier.Schedule.ScheduleID].Status; got != ScheduleStatusCompleted {
		t.Fatalf("expected earlier record completed, got %q", got)
	}
	if got := completed[later.Schedule.ScheduleID].Status; got != ScheduleStatusScheduled {
		t.Fatalf("expected later record to remain scheduled, got %q", got)
	}
}

func TestScheduleServiceExecuteDueRejectsNegativeLimit(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 2, 23, 9, 0, 0, 0, time.UTC)
	statePath := filepath.Join(t.TempDir(), "schedules.json")
	service := NewScheduleService(statePath)
	service.Now = func() time.Time { return now }

	_, err := service.ExecuteDue(context.Background(), ScheduleExecuteOptions{Limit: -1}, func(_ context.Context, _ ScheduledPublishRecord) (string, error) {
		t.Fatal("publish should not be called when limit is invalid")
		return "", nil
	})
	if err == nil {
		t.Fatal("expected negative limit error")
	}
	if !strings.Contains(err.Error(), "limit must be >= 0") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestScheduleServiceExecuteDuePartialFailure(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 2, 23, 9, 0, 0, 0, time.UTC)
	statePath := filepath.Join(t.TempDir(), "schedules.json")
	service := NewScheduleService(statePath)
	service.Now = func() time.Time { return now }

	if _, err := scheduleTestRecord(service, now, PublishSurfaceFeed, MediaTypeImage, 30*time.Minute); err != nil {
		t.Fatalf("schedule feed: %v", err)
	}
	if _, err := scheduleTestRecord(service, now, PublishSurfaceReel, MediaTypeReels, 45*time.Minute); err != nil {
		t.Fatalf("schedule reel: %v", err)
	}

	now = now.Add(2 * time.Hour)

	callCount := 0
	result, err := service.ExecuteDue(context.Background(), ScheduleExecuteOptions{}, func(_ context.Context, record ScheduledPublishRecord) (string, error) {
		callCount++
		if callCount == 2 {
			return "", errors.New("api error")
		}
		return "media_ok", nil
	})
	if err != nil {
		t.Fatalf("execute due: %v", err)
	}

	if result.Completed != 1 {
		t.Fatalf("expected completed=1, got %d", result.Completed)
	}
	if result.Failed != 1 {
		t.Fatalf("expected failed=1, got %d", result.Failed)
	}

	list, err := service.List(ScheduleListOptions{})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	completedCount := 0
	failedCount := 0
	for _, s := range list.Schedules {
		switch s.Status {
		case ScheduleStatusCompleted:
			completedCount++
			if s.MediaID != "media_ok" {
				t.Fatalf("unexpected media_id %q", s.MediaID)
			}
		case ScheduleStatusFailed:
			failedCount++
			if s.LastError != "api error" {
				t.Fatalf("unexpected last_error %q", s.LastError)
			}
		}
	}
	if completedCount != 1 || failedCount != 1 {
		t.Fatalf("expected 1 completed + 1 failed, got %d + %d", completedCount, failedCount)
	}
}

func TestScheduleServiceExecuteDueSkipsNonScheduledRecords(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 2, 23, 9, 0, 0, 0, time.UTC)
	statePath := filepath.Join(t.TempDir(), "schedules.json")
	service := NewScheduleService(statePath)
	service.Now = func() time.Time { return now }

	scheduled, err := scheduleTestRecord(service, now, PublishSurfaceFeed, MediaTypeImage, 30*time.Minute)
	if err != nil {
		t.Fatalf("schedule feed: %v", err)
	}

	if _, err := service.Cancel(ScheduleCancelOptions{ScheduleID: scheduled.Schedule.ScheduleID}); err != nil {
		t.Fatalf("cancel: %v", err)
	}

	now = now.Add(2 * time.Hour)

	result, err := service.ExecuteDue(context.Background(), ScheduleExecuteOptions{}, func(_ context.Context, _ ScheduledPublishRecord) (string, error) {
		t.Fatal("publish should not be called for canceled records")
		return "", nil
	})
	if err != nil {
		t.Fatalf("execute due: %v", err)
	}

	if result.Total != 0 {
		t.Fatalf("expected total=0, got %d", result.Total)
	}
}
