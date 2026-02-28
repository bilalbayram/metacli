package marketing

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/bilalbayram/metacli/internal/graph"
)

type CreativeService struct {
	Client *graph.Client
}

const (
	defaultCreativeVideoWaitTimeout  = 10 * time.Minute
	defaultCreativeVideoPollInterval = 5 * time.Second
)

type CreativeUploadInput struct {
	AccountID string
	FilePath  string
	FileName  string
}

type CreativeCreateInput struct {
	AccountID string
	Params    map[string]string
}

type CreativeVideoUploadInput struct {
	AccountID    string
	FilePath     string
	FileName     string
	WaitReady    bool
	Timeout      time.Duration
	PollInterval time.Duration
}

type CreativeVideoStatusInput struct {
	VideoID string
}

type CreativeUploadResult struct {
	Operation   string         `json:"operation"`
	AccountID   string         `json:"account_id"`
	FileName    string         `json:"file_name"`
	ImageHash   string         `json:"image_hash"`
	ImageID     string         `json:"image_id,omitempty"`
	ImageURL    string         `json:"image_url,omitempty"`
	RequestPath string         `json:"request_path"`
	Response    map[string]any `json:"response"`
}

type CreativeMutationResult struct {
	Operation   string         `json:"operation"`
	CreativeID  string         `json:"creative_id"`
	RequestPath string         `json:"request_path"`
	Response    map[string]any `json:"response"`
}

type CreativeVideoStatusResult struct {
	VideoID         string         `json:"video_id"`
	Ready           bool           `json:"ready"`
	FinalStatus     string         `json:"final_status,omitempty"`
	UploadPhase     map[string]any `json:"upload_phase,omitempty"`
	ProcessingPhase map[string]any `json:"processing_phase,omitempty"`
	PublishingPhase map[string]any `json:"publishing_phase,omitempty"`
	RequestPath     string         `json:"request_path"`
	Response        map[string]any `json:"response"`
}

type CreativeVideoUploadResult struct {
	Operation         string         `json:"operation"`
	AccountID         string         `json:"account_id"`
	FileName          string         `json:"file_name"`
	VideoID           string         `json:"video_id"`
	WaitReady         bool           `json:"wait_ready"`
	Ready             bool           `json:"ready"`
	FinalStatus       string         `json:"final_status,omitempty"`
	UploadPhase       map[string]any `json:"upload_phase,omitempty"`
	ProcessingPhase   map[string]any `json:"processing_phase,omitempty"`
	PublishingPhase   map[string]any `json:"publishing_phase,omitempty"`
	RequestPath       string         `json:"request_path"`
	StatusRequestPath string         `json:"status_request_path,omitempty"`
	Response          map[string]any `json:"response"`
	StatusResponse    map[string]any `json:"status_response,omitempty"`
}

func NewCreativeService(client *graph.Client) *CreativeService {
	if client == nil {
		client = graph.NewClient(nil, "")
	}
	return &CreativeService{Client: client}
}

func (s *CreativeService) Upload(ctx context.Context, version string, token string, appSecret string, input CreativeUploadInput) (*CreativeUploadResult, error) {
	if s == nil || s.Client == nil {
		return nil, errors.New("creative service client is required")
	}

	accountID, err := normalizeAdAccountID(input.AccountID)
	if err != nil {
		return nil, err
	}

	filePath := strings.TrimSpace(input.FilePath)
	if filePath == "" {
		return nil, errors.New("creative file path is required")
	}
	fileName, err := normalizeCreativeFileName(input.FileName, filePath)
	if err != nil {
		return nil, err
	}

	fileBytes, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("read creative file %q: %w", filePath, err)
	}
	if len(fileBytes) == 0 {
		return nil, errors.New("creative upload file cannot be empty")
	}

	path := fmt.Sprintf("act_%s/adimages", accountID)
	response, err := s.Client.Do(ctx, graph.Request{
		Method:  "POST",
		Path:    path,
		Version: strings.TrimSpace(version),
		Form: map[string]string{
			"filename": fileName,
			"bytes":    base64.StdEncoding.EncodeToString(fileBytes),
		},
		AccessToken: token,
		AppSecret:   appSecret,
	})
	if err != nil {
		return nil, err
	}

	imageData, err := normalizeCreativeUploadImageData(response.Body, fileName)
	if err != nil {
		return nil, err
	}

	imageHash, _ := imageData["hash"].(string)
	imageID, _ := imageData["id"].(string)
	imageURL, _ := imageData["url"].(string)

	return &CreativeUploadResult{
		Operation:   "upload",
		AccountID:   fmt.Sprintf("act_%s", accountID),
		FileName:    fileName,
		ImageHash:   strings.TrimSpace(imageHash),
		ImageID:     strings.TrimSpace(imageID),
		ImageURL:    strings.TrimSpace(imageURL),
		RequestPath: path,
		Response:    response.Body,
	}, nil
}

func (s *CreativeService) UploadVideo(ctx context.Context, version string, token string, appSecret string, input CreativeVideoUploadInput) (*CreativeVideoUploadResult, error) {
	if s == nil || s.Client == nil {
		return nil, errors.New("creative service client is required")
	}

	accountID, err := normalizeAdAccountID(input.AccountID)
	if err != nil {
		return nil, err
	}

	filePath := strings.TrimSpace(input.FilePath)
	if filePath == "" {
		return nil, errors.New("creative video file path is required")
	}
	fileName, err := normalizeCreativeFileName(input.FileName, filePath)
	if err != nil {
		return nil, err
	}

	fileBytes, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("read creative video file %q: %w", filePath, err)
	}
	if len(fileBytes) == 0 {
		return nil, errors.New("creative video upload file cannot be empty")
	}

	requestPath := fmt.Sprintf("act_%s/advideos", accountID)
	response, err := s.Client.Do(ctx, graph.Request{
		Method:      "POST",
		Path:        requestPath,
		Version:     strings.TrimSpace(version),
		Form:        map[string]string{"name": fileName},
		Multipart:   &graph.MultipartFile{FieldName: "source", FileName: fileName, FileBytes: fileBytes},
		AccessToken: token,
		AppSecret:   appSecret,
	})
	if err != nil {
		return nil, err
	}

	videoID := strings.TrimSpace(stringValue(response.Body["id"]))
	if videoID == "" {
		videoID = strings.TrimSpace(stringValue(response.Body["video_id"]))
	}
	if videoID == "" {
		return nil, errors.New("creative video upload response did not include id")
	}

	snapshot := summarizeCreativeVideoStatus(response.Body)
	result := &CreativeVideoUploadResult{
		Operation:       "upload-video",
		AccountID:       fmt.Sprintf("act_%s", accountID),
		FileName:        fileName,
		VideoID:         videoID,
		WaitReady:       input.WaitReady,
		Ready:           snapshot.Ready,
		FinalStatus:     snapshot.FinalStatus,
		UploadPhase:     snapshot.UploadPhase,
		ProcessingPhase: snapshot.ProcessingPhase,
		PublishingPhase: snapshot.PublishingPhase,
		RequestPath:     requestPath,
		Response:        response.Body,
	}

	if !input.WaitReady || snapshot.Ready {
		return result, nil
	}

	timeout := input.Timeout
	if timeout == 0 {
		timeout = defaultCreativeVideoWaitTimeout
	}
	if timeout <= 0 {
		return nil, errors.New("timeout must be greater than zero when waiting for creative video readiness")
	}
	pollInterval := input.PollInterval
	if pollInterval == 0 {
		pollInterval = defaultCreativeVideoPollInterval
	}
	if pollInterval <= 0 {
		return nil, errors.New("poll interval must be greater than zero when waiting for creative video readiness")
	}

	startedAt := time.Now()
	for {
		statusResult, err := s.VideoStatus(ctx, version, token, appSecret, CreativeVideoStatusInput{VideoID: videoID})
		if err != nil {
			return nil, err
		}

		result.Ready = statusResult.Ready
		result.FinalStatus = statusResult.FinalStatus
		result.UploadPhase = statusResult.UploadPhase
		result.ProcessingPhase = statusResult.ProcessingPhase
		result.PublishingPhase = statusResult.PublishingPhase
		result.StatusRequestPath = statusResult.RequestPath
		result.StatusResponse = statusResult.Response

		if statusResult.Ready {
			return result, nil
		}

		if isCreativeVideoFailureStatus(statusResult.FinalStatus) {
			return nil, fmt.Errorf("creative video %s processing failed with status %q", videoID, statusResult.FinalStatus)
		}

		if time.Since(startedAt) >= timeout {
			lastStatus := strings.TrimSpace(statusResult.FinalStatus)
			if lastStatus == "" {
				lastStatus = "unknown"
			}
			return nil, fmt.Errorf("creative video %s readiness wait timed out after %s (last status: %s)", videoID, timeout, lastStatus)
		}

		waitDuration := pollInterval
		remaining := timeout - time.Since(startedAt)
		if remaining < waitDuration {
			waitDuration = remaining
		}
		if waitDuration <= 0 {
			waitDuration = time.Millisecond
		}

		timer := time.NewTimer(waitDuration)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil, ctx.Err()
		case <-timer.C:
		}
	}
}

func (s *CreativeService) VideoStatus(ctx context.Context, version string, token string, appSecret string, input CreativeVideoStatusInput) (*CreativeVideoStatusResult, error) {
	if s == nil || s.Client == nil {
		return nil, errors.New("creative service client is required")
	}

	videoID, err := normalizeGraphID("video id", input.VideoID)
	if err != nil {
		return nil, err
	}

	requestPath := videoID
	response, err := s.Client.Do(ctx, graph.Request{
		Method:  "GET",
		Path:    requestPath,
		Version: strings.TrimSpace(version),
		Query: map[string]string{
			"fields": "id,status",
		},
		AccessToken: token,
		AppSecret:   appSecret,
	})
	if err != nil {
		return nil, err
	}

	resolvedVideoID := videoID
	if responseID, ok := response.Body["id"].(string); ok && strings.TrimSpace(responseID) != "" {
		resolvedVideoID = strings.TrimSpace(responseID)
	}

	snapshot := summarizeCreativeVideoStatus(response.Body)
	return &CreativeVideoStatusResult{
		VideoID:         resolvedVideoID,
		Ready:           snapshot.Ready,
		FinalStatus:     snapshot.FinalStatus,
		UploadPhase:     snapshot.UploadPhase,
		ProcessingPhase: snapshot.ProcessingPhase,
		PublishingPhase: snapshot.PublishingPhase,
		RequestPath:     requestPath,
		Response:        response.Body,
	}, nil
}

func (s *CreativeService) Create(ctx context.Context, version string, token string, appSecret string, input CreativeCreateInput) (*CreativeMutationResult, error) {
	if s == nil || s.Client == nil {
		return nil, errors.New("creative service client is required")
	}

	accountID, err := normalizeAdAccountID(input.AccountID)
	if err != nil {
		return nil, err
	}
	form, err := normalizeCreativeMutationParams(input.Params)
	if err != nil {
		return nil, err
	}

	path := fmt.Sprintf("act_%s/adcreatives", accountID)
	response, err := s.Client.Do(ctx, graph.Request{
		Method:      "POST",
		Path:        path,
		Version:     strings.TrimSpace(version),
		Form:        form,
		AccessToken: token,
		AppSecret:   appSecret,
	})
	if err != nil {
		return nil, err
	}

	creativeID, _ := response.Body["id"].(string)
	if strings.TrimSpace(creativeID) == "" {
		return nil, errors.New("creative create response did not include id")
	}

	return &CreativeMutationResult{
		Operation:   "create",
		CreativeID:  creativeID,
		RequestPath: path,
		Response:    response.Body,
	}, nil
}

func normalizeCreativeMutationParams(params map[string]string) (map[string]string, error) {
	normalized := map[string]string{}
	for key, value := range params {
		trimmedKey := strings.TrimSpace(key)
		if trimmedKey == "" {
			return nil, errors.New("creative mutation param key cannot be empty")
		}
		normalized[trimmedKey] = strings.TrimSpace(value)
	}
	if len(normalized) == 0 {
		return nil, errors.New("creative mutation payload cannot be empty")
	}
	return normalized, nil
}

func normalizeCreativeUploadImageData(response map[string]any, fileName string) (map[string]any, error) {
	rawImages, ok := response["images"]
	if !ok {
		return nil, errors.New("creative upload response did not include images")
	}

	images, ok := rawImages.(map[string]any)
	if !ok {
		return nil, errors.New("creative upload response images field must be an object")
	}
	if len(images) == 0 {
		return nil, errors.New("creative upload response images field was empty")
	}

	if imageData, ok := imageEntryMap(images[fileName]); ok {
		if strings.TrimSpace(stringValue(imageData["hash"])) == "" {
			return nil, errors.New("creative upload response did not include image hash")
		}
		return imageData, nil
	}

	if len(images) == 1 {
		for _, value := range images {
			imageData, ok := imageEntryMap(value)
			if !ok {
				return nil, errors.New("creative upload response image entry was not an object")
			}
			if strings.TrimSpace(stringValue(imageData["hash"])) == "" {
				return nil, errors.New("creative upload response did not include image hash")
			}
			return imageData, nil
		}
	}

	return nil, fmt.Errorf("creative upload response did not include image entry for %q", fileName)
}

func normalizeCreativeFileName(fileName string, filePath string) (string, error) {
	normalized := strings.TrimSpace(fileName)
	if normalized == "" {
		normalized = filepath.Base(strings.TrimSpace(filePath))
	}
	if normalized == "" || normalized == "." {
		return "", errors.New("creative file name is required")
	}
	if strings.ContainsRune(normalized, filepath.Separator) || strings.Contains(normalized, "/") {
		return "", fmt.Errorf("invalid creative file name %q: path separators are not allowed", normalized)
	}
	return normalized, nil
}

func imageEntryMap(value any) (map[string]any, bool) {
	typed, ok := value.(map[string]any)
	return typed, ok
}

func stringValue(value any) string {
	typed, _ := value.(string)
	return typed
}

type creativeVideoStatusSummary struct {
	UploadPhase     map[string]any
	ProcessingPhase map[string]any
	PublishingPhase map[string]any
	FinalStatus     string
	Ready           bool
}

func summarizeCreativeVideoStatus(response map[string]any) creativeVideoStatusSummary {
	statusObject := mapValue(response["status"])

	uploadPhase := mapValue(response["upload_phase"])
	if uploadPhase == nil {
		uploadPhase = mapValue(response["uploading_phase"])
	}
	if uploadPhase == nil {
		uploadPhase = mapValue(statusObject["upload_phase"])
	}
	if uploadPhase == nil {
		uploadPhase = mapValue(statusObject["uploading_phase"])
	}

	processingPhase := mapValue(response["processing_phase"])
	if processingPhase == nil {
		processingPhase = mapValue(statusObject["processing_phase"])
	}
	publishingPhase := mapValue(response["publishing_phase"])
	if publishingPhase == nil {
		publishingPhase = mapValue(statusObject["publishing_phase"])
	}

	videoStatus := firstNonEmptyStatus(statusString(response["video_status"]), statusString(statusObject["video_status"]))

	statusCandidates := []string{
		phaseStatus(processingPhase),
		phaseStatus(publishingPhase),
		videoStatus,
		statusString(response["status"]),
		phaseStatus(uploadPhase),
	}
	finalStatus := strings.TrimSpace(firstNonEmptyStatus(statusCandidates...))

	ready := boolValue(response["ready"]) ||
		boolValue(fieldValue(processingPhase, "ready")) ||
		boolValue(fieldValue(publishingPhase, "ready")) ||
		statusIndicatesReady(videoStatus) ||
		statusIndicatesReady(statusString(response["status"])) ||
		statusIndicatesReady(phaseStatus(processingPhase)) ||
		statusIndicatesReady(phaseStatus(publishingPhase))

	if finalStatus == "" && ready {
		finalStatus = "READY"
	}

	return creativeVideoStatusSummary{
		UploadPhase:     cloneMap(uploadPhase),
		ProcessingPhase: cloneMap(processingPhase),
		PublishingPhase: cloneMap(publishingPhase),
		FinalStatus:     finalStatus,
		Ready:           ready,
	}
}

func phaseStatus(phase map[string]any) string {
	if len(phase) == 0 {
		return ""
	}
	return statusString(phase["status"])
}

func statusString(value any) string {
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed)
	case map[string]any:
		for _, key := range []string{"video_status", "status", "state"} {
			if nested := strings.TrimSpace(stringValue(typed[key])); nested != "" {
				return nested
			}
		}
	}
	return ""
}

func statusIndicatesReady(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "ready", "published", "complete", "completed", "finished", "available", "success":
		return true
	default:
		return false
	}
}

func isCreativeVideoFailureStatus(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "error", "failed", "failure", "rejected", "aborted", "blocked":
		return true
	default:
		return false
	}
}

func firstNonEmptyStatus(values ...string) string {
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func mapValue(value any) map[string]any {
	typed, _ := value.(map[string]any)
	return typed
}

func fieldValue(data map[string]any, key string) any {
	if len(data) == 0 {
		return nil
	}
	return data[key]
}

func boolValue(value any) bool {
	switch typed := value.(type) {
	case bool:
		return typed
	case string:
		switch strings.ToLower(strings.TrimSpace(typed)) {
		case "true", "1", "yes":
			return true
		default:
			return false
		}
	default:
		return false
	}
}

func cloneMap(source map[string]any) map[string]any {
	if len(source) == 0 {
		return nil
	}
	cloned := make(map[string]any, len(source))
	for key, value := range source {
		cloned[key] = value
	}
	return cloned
}
