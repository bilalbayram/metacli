package ig

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/bilalbayram/metacli/internal/graph"
)

const (
	MediaTypeImage = "IMAGE"
	MediaTypeVideo = "VIDEO"
	MediaTypeReels = "REELS"
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
