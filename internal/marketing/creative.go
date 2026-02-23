package marketing

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/bilalbayram/metacli/internal/graph"
)

type CreativeService struct {
	Client *graph.Client
}

type CreativeUploadInput struct {
	AccountID string
	FilePath  string
	FileName  string
}

type CreativeCreateInput struct {
	AccountID string
	Params    map[string]string
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
