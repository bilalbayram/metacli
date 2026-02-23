package ig

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const (
	ScheduleStateSchemaVersion = 1
	ScheduleStatusScheduled    = "scheduled"
	ScheduleStatusCanceled     = "canceled"
	ScheduleStatusFailed       = "failed"
)

const missedScheduleError = "scheduled publish time elapsed without execution"

type ScheduledPublishRecord struct {
	ScheduleID     string `json:"schedule_id"`
	Profile        string `json:"profile"`
	Version        string `json:"version"`
	Surface        string `json:"surface"`
	IdempotencyKey string `json:"idempotency_key,omitempty"`
	IGUserID       string `json:"ig_user_id"`
	MediaURL       string `json:"media_url"`
	Caption        string `json:"caption"`
	MediaType      string `json:"media_type"`
	StrictMode     bool   `json:"strict_mode"`
	PublishAt      string `json:"publish_at"`
	Status         string `json:"status"`
	RetryCount     int    `json:"retry_count"`
	LastError      string `json:"last_error,omitempty"`
	CreatedAt      string `json:"created_at"`
	UpdatedAt      string `json:"updated_at"`
}

type SchedulePublishOptions struct {
	Profile        string
	Version        string
	Surface        string
	IdempotencyKey string
	IGUserID       string
	MediaURL       string
	Caption        string
	MediaType      string
	StrictMode     bool
	PublishAt      string
}

type SchedulePublishResult struct {
	Mode                string                 `json:"mode"`
	Surface             string                 `json:"surface"`
	DuplicateSuppressed bool                   `json:"duplicate_suppressed"`
	Schedule            ScheduledPublishRecord `json:"schedule"`
}

type ScheduleListOptions struct {
	Status string
}

type ScheduleListResult struct {
	Total     int                      `json:"total"`
	Schedules []ScheduledPublishRecord `json:"schedules"`
}

type ScheduleCancelOptions struct {
	ScheduleID string
}

type ScheduleRetryOptions struct {
	ScheduleID string
	PublishAt  string
}

type ScheduleTransitionResult struct {
	Operation string                 `json:"operation"`
	Schedule  ScheduledPublishRecord `json:"schedule"`
}

type ScheduleService struct {
	Path string
	Now  func() time.Time
}

type scheduleState struct {
	SchemaVersion int                      `json:"schema_version"`
	NextSequence  int                      `json:"next_sequence"`
	Schedules     []ScheduledPublishRecord `json:"schedules"`
}

func NewScheduleService(path string) *ScheduleService {
	return &ScheduleService{
		Path: strings.TrimSpace(path),
		Now:  time.Now,
	}
}

func DefaultScheduleStatePath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve user home directory: %w", err)
	}
	return filepath.Join(home, ".meta", "ig", "schedules.json"), nil
}

func (s *ScheduleService) Schedule(options SchedulePublishOptions) (*SchedulePublishResult, error) {
	if s == nil {
		return nil, errors.New("schedule service is required")
	}
	if strings.TrimSpace(s.Path) == "" {
		return nil, errors.New("schedule state path is required")
	}

	now := s.nowUTC()
	normalized, publishAt, err := normalizeSchedulePublishOptions(options, now)
	if err != nil {
		return nil, err
	}

	state, err := loadScheduleState(s.Path)
	if err != nil {
		return nil, err
	}
	reconcileMissedSchedules(&state, now)

	if normalized.IdempotencyKey != "" {
		duplicate, found, err := findDuplicateSchedule(state.Schedules, normalized, publishAt)
		if err != nil {
			return nil, err
		}
		if found {
			return &SchedulePublishResult{
				Mode:                "scheduled",
				Surface:             duplicate.Surface,
				DuplicateSuppressed: true,
				Schedule:            duplicate,
			}, nil
		}
	}

	scheduleID := nextScheduleID(state.NextSequence, normalized.Surface, normalized.IGUserID, normalized.MediaURL, publishAt)
	record := ScheduledPublishRecord{
		ScheduleID:     scheduleID,
		Profile:        normalized.Profile,
		Version:        normalized.Version,
		Surface:        normalized.Surface,
		IdempotencyKey: normalized.IdempotencyKey,
		IGUserID:       normalized.IGUserID,
		MediaURL:       normalized.MediaURL,
		Caption:        normalized.Caption,
		MediaType:      normalized.MediaType,
		StrictMode:     normalized.StrictMode,
		PublishAt:      publishAt.UTC().Format(time.RFC3339),
		Status:         ScheduleStatusScheduled,
		RetryCount:     0,
		CreatedAt:      now.Format(time.RFC3339),
		UpdatedAt:      now.Format(time.RFC3339),
	}

	state.NextSequence++
	state.Schedules = append(state.Schedules, record)

	if err := saveScheduleState(s.Path, state); err != nil {
		return nil, err
	}

	return &SchedulePublishResult{
		Mode:                "scheduled",
		Surface:             normalized.Surface,
		DuplicateSuppressed: false,
		Schedule:            record,
	}, nil
}

func (s *ScheduleService) List(options ScheduleListOptions) (*ScheduleListResult, error) {
	if s == nil {
		return nil, errors.New("schedule service is required")
	}
	if strings.TrimSpace(s.Path) == "" {
		return nil, errors.New("schedule state path is required")
	}

	filterStatus := strings.TrimSpace(options.Status)
	if filterStatus != "" {
		normalizedStatus, err := normalizeScheduleStatus(filterStatus)
		if err != nil {
			return nil, err
		}
		filterStatus = normalizedStatus
	}

	state, err := loadScheduleState(s.Path)
	if err != nil {
		return nil, err
	}

	now := s.nowUTC()
	records, changed := reconcileMissedSchedules(&state, now)
	if changed {
		if err := saveScheduleState(s.Path, state); err != nil {
			return nil, err
		}
	}

	records = append([]ScheduledPublishRecord(nil), records...)
	sort.SliceStable(records, func(i, j int) bool {
		if records[i].CreatedAt == records[j].CreatedAt {
			return records[i].ScheduleID < records[j].ScheduleID
		}
		return records[i].CreatedAt < records[j].CreatedAt
	})

	filtered := make([]ScheduledPublishRecord, 0, len(records))
	for _, record := range records {
		if filterStatus != "" && record.Status != filterStatus {
			continue
		}
		filtered = append(filtered, record)
	}

	return &ScheduleListResult{
		Total:     len(filtered),
		Schedules: filtered,
	}, nil
}

func (s *ScheduleService) Cancel(options ScheduleCancelOptions) (*ScheduleTransitionResult, error) {
	if s == nil {
		return nil, errors.New("schedule service is required")
	}
	if strings.TrimSpace(s.Path) == "" {
		return nil, errors.New("schedule state path is required")
	}

	scheduleID := strings.TrimSpace(options.ScheduleID)
	if scheduleID == "" {
		return nil, errors.New("schedule id is required")
	}

	state, err := loadScheduleState(s.Path)
	if err != nil {
		return nil, err
	}

	now := s.nowUTC()
	reconcileMissedSchedules(&state, now)

	record, idx, err := findSchedule(state.Schedules, scheduleID)
	if err != nil {
		return nil, err
	}
	if record.Status != ScheduleStatusScheduled {
		return nil, newStateTransitionError(scheduleID, record.Status, ScheduleStatusCanceled)
	}

	record.Status = ScheduleStatusCanceled
	record.LastError = ""
	record.UpdatedAt = now.Format(time.RFC3339)
	state.Schedules[idx] = record

	if err := saveScheduleState(s.Path, state); err != nil {
		return nil, err
	}

	return &ScheduleTransitionResult{
		Operation: "cancel",
		Schedule:  record,
	}, nil
}

func (s *ScheduleService) Retry(options ScheduleRetryOptions) (*ScheduleTransitionResult, error) {
	if s == nil {
		return nil, errors.New("schedule service is required")
	}
	if strings.TrimSpace(s.Path) == "" {
		return nil, errors.New("schedule state path is required")
	}

	scheduleID := strings.TrimSpace(options.ScheduleID)
	if scheduleID == "" {
		return nil, errors.New("schedule id is required")
	}

	state, err := loadScheduleState(s.Path)
	if err != nil {
		return nil, err
	}

	now := s.nowUTC()
	reconcileMissedSchedules(&state, now)

	record, idx, err := findSchedule(state.Schedules, scheduleID)
	if err != nil {
		return nil, err
	}
	if record.Status != ScheduleStatusCanceled && record.Status != ScheduleStatusFailed {
		return nil, newStateTransitionError(scheduleID, record.Status, ScheduleStatusScheduled)
	}

	publishAtRaw := strings.TrimSpace(options.PublishAt)
	if publishAtRaw == "" {
		publishAtRaw = record.PublishAt
	}
	publishAt, err := parsePublishAt(publishAtRaw)
	if err != nil {
		return nil, err
	}
	if !publishAt.After(now) {
		return nil, fmt.Errorf("publish-at must be in the future (%s)", now.Format(time.RFC3339))
	}

	record.PublishAt = publishAt.UTC().Format(time.RFC3339)
	record.Status = ScheduleStatusScheduled
	record.RetryCount++
	record.LastError = ""
	record.UpdatedAt = now.Format(time.RFC3339)
	state.Schedules[idx] = record

	if err := saveScheduleState(s.Path, state); err != nil {
		return nil, err
	}

	return &ScheduleTransitionResult{
		Operation: "retry",
		Schedule:  record,
	}, nil
}

func nextScheduleID(sequence int, surface string, igUserID string, mediaURL string, publishAt time.Time) string {
	seed := fmt.Sprintf("%d|%s|%s|%s|%s", sequence, surface, igUserID, mediaURL, publishAt.UTC().Format(time.RFC3339))
	sum := sha256.Sum256([]byte(seed))
	return fmt.Sprintf("sched_%06d_%s", sequence, hex.EncodeToString(sum[:])[:8])
}

func normalizeSchedulePublishOptions(options SchedulePublishOptions, now time.Time) (SchedulePublishOptions, time.Time, error) {
	surface, err := normalizePublishSurface(options.Surface)
	if err != nil {
		return SchedulePublishOptions{}, time.Time{}, err
	}

	mediaType, err := ValidatePublishMediaTypeForSurface(surface, options.MediaType)
	if err != nil {
		return SchedulePublishOptions{}, time.Time{}, err
	}

	captionValidation := ValidateCaption(options.Caption, options.StrictMode)
	if !captionValidation.Valid {
		return SchedulePublishOptions{}, time.Time{}, errors.New(strings.Join(captionValidation.Errors, "; "))
	}

	normalizedProfile := strings.TrimSpace(options.Profile)
	if normalizedProfile == "" {
		return SchedulePublishOptions{}, time.Time{}, errors.New("profile is required")
	}

	publishAt, err := parsePublishAt(options.PublishAt)
	if err != nil {
		return SchedulePublishOptions{}, time.Time{}, err
	}
	if !publishAt.After(now) {
		return SchedulePublishOptions{}, time.Time{}, fmt.Errorf("publish-at must be in the future (%s)", now.Format(time.RFC3339))
	}

	normalized := SchedulePublishOptions{
		Profile:        normalizedProfile,
		Version:        strings.TrimSpace(options.Version),
		Surface:        surface,
		IdempotencyKey: strings.TrimSpace(options.IdempotencyKey),
		IGUserID:       strings.TrimSpace(options.IGUserID),
		MediaURL:       strings.TrimSpace(options.MediaURL),
		Caption:        options.Caption,
		MediaType:      mediaType,
		StrictMode:     options.StrictMode,
		PublishAt:      publishAt.UTC().Format(time.RFC3339),
	}
	idempotencyKey, err := normalizeIdempotencyKey(normalized.IdempotencyKey)
	if err != nil {
		return SchedulePublishOptions{}, time.Time{}, err
	}
	normalized.IdempotencyKey = idempotencyKey
	if normalized.Version == "" {
		return SchedulePublishOptions{}, time.Time{}, errors.New("version is required")
	}

	if _, _, err := BuildUploadRequest("", "", "", MediaUploadOptions{
		IGUserID:  normalized.IGUserID,
		MediaURL:  normalized.MediaURL,
		Caption:   normalized.Caption,
		MediaType: normalized.MediaType,
	}); err != nil {
		return SchedulePublishOptions{}, time.Time{}, err
	}

	return normalized, publishAt, nil
}

func parsePublishAt(raw string) (time.Time, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return time.Time{}, errors.New("publish-at is required")
	}
	parsed, err := time.Parse(time.RFC3339, trimmed)
	if err != nil {
		return time.Time{}, fmt.Errorf("invalid publish-at %q: expected RFC3339 timestamp", raw)
	}
	return parsed.UTC(), nil
}

func normalizeScheduleStatus(status string) (string, error) {
	normalized := strings.ToLower(strings.TrimSpace(status))
	switch normalized {
	case ScheduleStatusScheduled, ScheduleStatusCanceled, ScheduleStatusFailed:
		return normalized, nil
	case "":
		return "", errors.New("schedule status is required")
	default:
		return "", fmt.Errorf("unsupported schedule status %q: expected scheduled|canceled|failed", status)
	}
}

func (s *ScheduleService) nowUTC() time.Time {
	if s.Now == nil {
		return time.Now().UTC()
	}
	return s.Now().UTC()
}

func newScheduleState() scheduleState {
	return scheduleState{
		SchemaVersion: ScheduleStateSchemaVersion,
		NextSequence:  1,
		Schedules:     []ScheduledPublishRecord{},
	}
}

func loadScheduleState(path string) (scheduleState, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return scheduleState{}, errors.New("schedule state path is required")
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return newScheduleState(), nil
		}
		return scheduleState{}, fmt.Errorf("read schedule state %s: %w", path, err)
	}

	var state scheduleState
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&state); err != nil {
		return scheduleState{}, fmt.Errorf("decode schedule state %s: %w", path, err)
	}
	var trailing struct{}
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return scheduleState{}, fmt.Errorf("decode schedule state %s: multiple JSON values", path)
		}
		return scheduleState{}, fmt.Errorf("decode schedule state %s: %w", path, err)
	}

	if err := state.Validate(); err != nil {
		return scheduleState{}, err
	}
	return state, nil
}

func saveScheduleState(path string, state scheduleState) error {
	path = strings.TrimSpace(path)
	if path == "" {
		return errors.New("schedule state path is required")
	}
	if err := state.Validate(); err != nil {
		return err
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create schedule directory for %s: %w", path, err)
	}

	payload, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal schedule state: %w", err)
	}
	payload = append(payload, '\n')

	tmpFile, err := os.CreateTemp(dir, ".ig-schedules-*.json")
	if err != nil {
		return fmt.Errorf("create temp schedule file: %w", err)
	}
	defer os.Remove(tmpFile.Name())

	if _, err := tmpFile.Write(payload); err != nil {
		tmpFile.Close()
		return fmt.Errorf("write temp schedule file: %w", err)
	}
	if err := tmpFile.Chmod(0o600); err != nil {
		tmpFile.Close()
		return fmt.Errorf("chmod temp schedule file: %w", err)
	}
	if err := tmpFile.Close(); err != nil {
		return fmt.Errorf("close temp schedule file: %w", err)
	}
	if err := os.Rename(tmpFile.Name(), path); err != nil {
		return fmt.Errorf("replace schedule state %s: %w", path, err)
	}
	return nil
}

func (s scheduleState) Validate() error {
	if s.SchemaVersion != ScheduleStateSchemaVersion {
		return fmt.Errorf("unsupported schedule schema_version=%d (expected %d)", s.SchemaVersion, ScheduleStateSchemaVersion)
	}
	if s.NextSequence < 1 {
		return errors.New("schedule next_sequence must be >= 1")
	}

	seen := map[string]struct{}{}
	for idx, record := range s.Schedules {
		if err := validateScheduledRecord(record); err != nil {
			return fmt.Errorf("schedule[%d]: %w", idx, err)
		}
		if _, exists := seen[record.ScheduleID]; exists {
			return fmt.Errorf("schedule[%d]: duplicate schedule_id %q", idx, record.ScheduleID)
		}
		seen[record.ScheduleID] = struct{}{}
	}
	return nil
}

func validateScheduledRecord(record ScheduledPublishRecord) error {
	if strings.TrimSpace(record.ScheduleID) == "" {
		return errors.New("schedule_id is required")
	}
	if strings.TrimSpace(record.Profile) == "" {
		return errors.New("profile is required")
	}
	if strings.TrimSpace(record.Version) == "" {
		return errors.New("version is required")
	}
	if _, err := normalizePublishSurface(record.Surface); err != nil {
		return err
	}
	if _, err := normalizeScheduleStatus(record.Status); err != nil {
		return err
	}
	if _, err := normalizeIdempotencyKey(record.IdempotencyKey); err != nil {
		return err
	}
	if record.RetryCount < 0 {
		return errors.New("retry_count cannot be negative")
	}
	createdAt, err := time.Parse(time.RFC3339, record.CreatedAt)
	if err != nil {
		return fmt.Errorf("invalid created_at %q: expected RFC3339 timestamp", record.CreatedAt)
	}
	updatedAt, err := time.Parse(time.RFC3339, record.UpdatedAt)
	if err != nil {
		return fmt.Errorf("invalid updated_at %q: expected RFC3339 timestamp", record.UpdatedAt)
	}
	if updatedAt.Before(createdAt) {
		return errors.New("updated_at cannot be before created_at")
	}
	if _, err := parsePublishAt(record.PublishAt); err != nil {
		return err
	}
	if _, err := ValidatePublishMediaTypeForSurface(record.Surface, record.MediaType); err != nil {
		return err
	}
	captionValidation := ValidateCaption(record.Caption, record.StrictMode)
	if !captionValidation.Valid {
		return errors.New(strings.Join(captionValidation.Errors, "; "))
	}
	if _, _, err := BuildUploadRequest("", "", "", MediaUploadOptions{
		IGUserID:  record.IGUserID,
		MediaURL:  record.MediaURL,
		Caption:   record.Caption,
		MediaType: record.MediaType,
	}); err != nil {
		return err
	}
	return nil
}

func reconcileMissedSchedules(state *scheduleState, now time.Time) ([]ScheduledPublishRecord, bool) {
	if state == nil {
		return nil, false
	}

	changed := false
	records := make([]ScheduledPublishRecord, len(state.Schedules))
	copy(records, state.Schedules)

	for idx, record := range records {
		if record.Status != ScheduleStatusScheduled {
			continue
		}
		publishAt, err := parsePublishAt(record.PublishAt)
		if err != nil {
			continue
		}
		if publishAt.After(now) {
			continue
		}
		record.Status = ScheduleStatusFailed
		record.LastError = missedScheduleError
		record.UpdatedAt = now.UTC().Format(time.RFC3339)
		records[idx] = record
		changed = true
	}

	if changed {
		state.Schedules = records
	}
	return records, changed
}

func findSchedule(records []ScheduledPublishRecord, scheduleID string) (ScheduledPublishRecord, int, error) {
	for idx, record := range records {
		if record.ScheduleID == scheduleID {
			return record, idx, nil
		}
	}
	return ScheduledPublishRecord{}, -1, newScheduleNotFoundError(scheduleID)
}

func findDuplicateSchedule(records []ScheduledPublishRecord, options SchedulePublishOptions, publishAt time.Time) (ScheduledPublishRecord, bool, error) {
	signature := schedulePayloadSignatureFromOptions(options, publishAt)
	for _, record := range records {
		if record.Profile != options.Profile {
			continue
		}
		recordIdempotencyKey, err := normalizeIdempotencyKey(record.IdempotencyKey)
		if err != nil {
			return ScheduledPublishRecord{}, false, err
		}
		if recordIdempotencyKey != options.IdempotencyKey {
			continue
		}
		if schedulePayloadSignatureFromRecord(record) != signature {
			return ScheduledPublishRecord{}, false, newIdempotencyConflictError(options.IdempotencyKey, record.ScheduleID)
		}
		return record, true, nil
	}
	return ScheduledPublishRecord{}, false, nil
}

func schedulePayloadSignatureFromOptions(options SchedulePublishOptions, publishAt time.Time) string {
	payload := strings.Join([]string{
		options.Profile,
		options.Version,
		options.Surface,
		options.IGUserID,
		options.MediaURL,
		options.Caption,
		options.MediaType,
		fmt.Sprintf("%t", options.StrictMode),
		publishAt.UTC().Format(time.RFC3339),
	}, "\x1f")
	sum := sha256.Sum256([]byte(payload))
	return hex.EncodeToString(sum[:])
}

func schedulePayloadSignatureFromRecord(record ScheduledPublishRecord) string {
	payload := strings.Join([]string{
		record.Profile,
		record.Version,
		record.Surface,
		record.IGUserID,
		record.MediaURL,
		record.Caption,
		record.MediaType,
		fmt.Sprintf("%t", record.StrictMode),
		record.PublishAt,
	}, "\x1f")
	sum := sha256.Sum256([]byte(payload))
	return hex.EncodeToString(sum[:])
}
