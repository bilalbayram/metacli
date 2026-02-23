package ig

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/bilalbayram/metacli/internal/graph"
)

const (
	MediaTypeImage          = "IMAGE"
	MediaTypeVideo          = "VIDEO"
	MediaTypeReels          = "REELS"
	MediaStatusCodeFinished = "FINISHED"
)

type MediaUploadOptions struct {
	IGUserID       string
	MediaURL       string
	Caption        string
	MediaType      string
	IsCarouselItem bool
}

type MediaUploadResult struct {
	CreationID  string         `json:"creation_id"`
	RequestPath string         `json:"request_path"`
	MediaType   string         `json:"media_type"`
	Response    map[string]any `json:"response"`
}

type MediaStatusOptions struct {
	CreationID string
}

type MediaStatusResult struct {
	CreationID string         `json:"creation_id"`
	Status     string         `json:"status,omitempty"`
	StatusCode string         `json:"status_code,omitempty"`
	Response   map[string]any `json:"response"`
}

type MediaPublishOptions struct {
	IGUserID   string
	CreationID string
}

type MediaPublishResult struct {
	IGUserID    string         `json:"ig_user_id"`
	CreationID  string         `json:"creation_id"`
	MediaID     string         `json:"media_id"`
	RequestPath string         `json:"request_path"`
	Response    map[string]any `json:"response"`
}

type FeedPublishOptions struct {
	IGUserID   string
	MediaURL   string
	Caption    string
	MediaType  string
	StrictMode bool
}

type FeedPublishResult struct {
	Mode               string                  `json:"mode"`
	IGUserID           string                  `json:"ig_user_id"`
	MediaType          string                  `json:"media_type"`
	CreationID         string                  `json:"creation_id"`
	MediaID            string                  `json:"media_id"`
	Status             string                  `json:"status,omitempty"`
	StatusCode         string                  `json:"status_code"`
	CaptionValidation  CaptionValidationResult `json:"caption_validation"`
	UploadRequestPath  string                  `json:"upload_request_path"`
	PublishRequestPath string                  `json:"publish_request_path"`
	UploadResponse     map[string]any          `json:"upload_response"`
	StatusResponse     map[string]any          `json:"status_response"`
	PublishResponse    map[string]any          `json:"publish_response"`
}

type Service struct {
	Client *graph.Client
}

func New(client *graph.Client) *Service {
	if client == nil {
		client = graph.NewClient(nil, "")
	}
	return &Service{Client: client}
}

func (s *Service) Upload(ctx context.Context, version string, token string, appSecret string, options MediaUploadOptions) (*MediaUploadResult, error) {
	if s == nil || s.Client == nil {
		return nil, errors.New("instagram service client is required")
	}

	request, mediaType, err := BuildUploadRequest(version, token, appSecret, options)
	if err != nil {
		return nil, err
	}

	response, err := s.Client.Do(ctx, request)
	if err != nil {
		return nil, err
	}

	creationID, _ := response.Body["id"].(string)
	if strings.TrimSpace(creationID) == "" {
		return nil, errors.New("instagram media upload response did not include id")
	}

	return &MediaUploadResult{
		CreationID:  creationID,
		RequestPath: request.Path,
		MediaType:   mediaType,
		Response:    response.Body,
	}, nil
}

func (s *Service) Status(ctx context.Context, version string, token string, appSecret string, options MediaStatusOptions) (*MediaStatusResult, error) {
	if s == nil || s.Client == nil {
		return nil, errors.New("instagram service client is required")
	}

	request, err := BuildStatusRequest(version, token, appSecret, options)
	if err != nil {
		return nil, err
	}

	response, err := s.Client.Do(ctx, request)
	if err != nil {
		return nil, err
	}

	creationID := strings.TrimSpace(options.CreationID)
	if responseID, ok := response.Body["id"].(string); ok && strings.TrimSpace(responseID) != "" {
		creationID = responseID
	}
	if creationID == "" {
		return nil, errors.New("instagram media status response did not include id")
	}

	status, _ := response.Body["status"].(string)
	statusCode, _ := response.Body["status_code"].(string)
	return &MediaStatusResult{
		CreationID: creationID,
		Status:     strings.TrimSpace(status),
		StatusCode: strings.TrimSpace(statusCode),
		Response:   response.Body,
	}, nil
}

func (s *Service) Publish(ctx context.Context, version string, token string, appSecret string, options MediaPublishOptions) (*MediaPublishResult, error) {
	if s == nil || s.Client == nil {
		return nil, errors.New("instagram service client is required")
	}

	request, igUserID, creationID, err := BuildPublishRequest(version, token, appSecret, options)
	if err != nil {
		return nil, err
	}

	response, err := s.Client.Do(ctx, request)
	if err != nil {
		return nil, err
	}

	mediaID, _ := response.Body["id"].(string)
	if strings.TrimSpace(mediaID) == "" {
		return nil, errors.New("instagram media publish response did not include id")
	}

	return &MediaPublishResult{
		IGUserID:    igUserID,
		CreationID:  creationID,
		MediaID:     strings.TrimSpace(mediaID),
		RequestPath: request.Path,
		Response:    response.Body,
	}, nil
}

func (s *Service) PublishFeedImmediate(ctx context.Context, version string, token string, appSecret string, options FeedPublishOptions) (*FeedPublishResult, error) {
	if s == nil || s.Client == nil {
		return nil, errors.New("instagram service client is required")
	}

	captionValidation := ValidateCaption(options.Caption, options.StrictMode)
	if !captionValidation.Valid {
		return nil, errors.New(strings.Join(captionValidation.Errors, "; "))
	}

	uploadResult, err := s.Upload(ctx, version, token, appSecret, MediaUploadOptions{
		IGUserID:  options.IGUserID,
		MediaURL:  options.MediaURL,
		Caption:   options.Caption,
		MediaType: options.MediaType,
	})
	if err != nil {
		return nil, err
	}

	statusResult, err := s.Status(ctx, version, token, appSecret, MediaStatusOptions{
		CreationID: uploadResult.CreationID,
	})
	if err != nil {
		return nil, err
	}

	if err := ensureMediaReadyForPublish(statusResult); err != nil {
		return nil, err
	}

	publishResult, err := s.Publish(ctx, version, token, appSecret, MediaPublishOptions{
		IGUserID:   options.IGUserID,
		CreationID: uploadResult.CreationID,
	})
	if err != nil {
		return nil, err
	}

	return &FeedPublishResult{
		Mode:               "immediate",
		IGUserID:           publishResult.IGUserID,
		MediaType:          uploadResult.MediaType,
		CreationID:         uploadResult.CreationID,
		MediaID:            publishResult.MediaID,
		Status:             statusResult.Status,
		StatusCode:         statusResult.StatusCode,
		CaptionValidation:  captionValidation,
		UploadRequestPath:  uploadResult.RequestPath,
		PublishRequestPath: publishResult.RequestPath,
		UploadResponse:     uploadResult.Response,
		StatusResponse:     statusResult.Response,
		PublishResponse:    publishResult.Response,
	}, nil
}

func BuildUploadRequest(version string, token string, appSecret string, options MediaUploadOptions) (graph.Request, string, error) {
	path, form, mediaType, err := shapeUploadPayload(options)
	if err != nil {
		return graph.Request{}, "", err
	}
	return graph.Request{
		Method:      "POST",
		Path:        path,
		Version:     strings.TrimSpace(version),
		Form:        form,
		AccessToken: token,
		AppSecret:   appSecret,
	}, mediaType, nil
}

func BuildStatusRequest(version string, token string, appSecret string, options MediaStatusOptions) (graph.Request, error) {
	creationID, err := normalizeGraphID("creation id", options.CreationID)
	if err != nil {
		return graph.Request{}, err
	}
	return graph.Request{
		Method:  "GET",
		Path:    creationID,
		Version: strings.TrimSpace(version),
		Query: map[string]string{
			"fields": "id,status,status_code",
		},
		AccessToken: token,
		AppSecret:   appSecret,
	}, nil
}

func BuildPublishRequest(version string, token string, appSecret string, options MediaPublishOptions) (graph.Request, string, string, error) {
	igUserID, creationID, form, err := shapePublishPayload(options)
	if err != nil {
		return graph.Request{}, "", "", err
	}

	return graph.Request{
		Method:      "POST",
		Path:        fmt.Sprintf("%s/media_publish", igUserID),
		Version:     strings.TrimSpace(version),
		Form:        form,
		AccessToken: token,
		AppSecret:   appSecret,
	}, igUserID, creationID, nil
}

func shapeUploadPayload(options MediaUploadOptions) (string, map[string]string, string, error) {
	igUserID, err := normalizeGraphID("ig user id", options.IGUserID)
	if err != nil {
		return "", nil, "", err
	}

	mediaType, err := normalizeMediaType(options.MediaType)
	if err != nil {
		return "", nil, "", err
	}

	mediaURL := strings.TrimSpace(options.MediaURL)
	if mediaURL == "" {
		return "", nil, "", errors.New("media url is required")
	}

	form := map[string]string{}
	switch mediaType {
	case MediaTypeImage:
		form["image_url"] = mediaURL
	case MediaTypeVideo:
		form["video_url"] = mediaURL
	case MediaTypeReels:
		form["video_url"] = mediaURL
		form["media_type"] = MediaTypeReels
	default:
		return "", nil, "", fmt.Errorf("unsupported media type %q: expected IMAGE|VIDEO|REELS", options.MediaType)
	}

	if caption := strings.TrimSpace(options.Caption); caption != "" {
		form["caption"] = caption
	}
	if options.IsCarouselItem {
		form["is_carousel_item"] = "true"
	}

	return fmt.Sprintf("%s/media", igUserID), form, mediaType, nil
}

func shapePublishPayload(options MediaPublishOptions) (string, string, map[string]string, error) {
	igUserID, err := normalizeGraphID("ig user id", options.IGUserID)
	if err != nil {
		return "", "", nil, err
	}

	creationID, err := normalizeGraphID("creation id", options.CreationID)
	if err != nil {
		return "", "", nil, err
	}

	return igUserID, creationID, map[string]string{
		"creation_id": creationID,
	}, nil
}

func ensureMediaReadyForPublish(result *MediaStatusResult) error {
	if result == nil {
		return errors.New("instagram media status result is required")
	}

	statusCode := strings.ToUpper(strings.TrimSpace(result.StatusCode))
	if statusCode == "" {
		return errors.New("instagram media status response did not include status_code")
	}
	if statusCode != MediaStatusCodeFinished {
		return fmt.Errorf("instagram media container is not ready for publish: status_code=%s", statusCode)
	}
	return nil
}

func normalizeMediaType(mediaType string) (string, error) {
	normalized := strings.ToUpper(strings.TrimSpace(mediaType))
	switch normalized {
	case MediaTypeImage, MediaTypeVideo, MediaTypeReels:
		return normalized, nil
	case "":
		return "", errors.New("media type is required")
	default:
		return "", fmt.Errorf("unsupported media type %q: expected IMAGE|VIDEO|REELS", mediaType)
	}
}

func normalizeGraphID(label string, value string) (string, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return "", fmt.Errorf("%s is required", label)
	}
	if strings.Contains(trimmed, "/") {
		return "", fmt.Errorf("invalid %s %q: expected single graph id token", label, value)
	}
	return trimmed, nil
}
