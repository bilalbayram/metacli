package ig

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/bilalbayram/metacli/internal/graph"
)

type stubHTTPClient struct {
	t *testing.T

	statusCode int
	response   string
	err        error

	calls      int
	lastMethod string
	lastURL    string
	lastBody   string
}

type sequenceStubResponse struct {
	statusCode int
	response   string
	err        error
}

type capturedHTTPCall struct {
	method string
	url    string
	body   string
}

type sequenceHTTPClient struct {
	t *testing.T

	responses []sequenceStubResponse
	calls     []capturedHTTPCall
}

func (c *stubHTTPClient) Do(req *http.Request) (*http.Response, error) {
	c.calls++
	c.lastMethod = req.Method
	c.lastURL = req.URL.String()
	if req.Body != nil {
		body, readErr := io.ReadAll(req.Body)
		if readErr != nil {
			c.t.Fatalf("read request body: %v", readErr)
		}
		c.lastBody = string(body)
	}

	if c.err != nil {
		return nil, c.err
	}
	return &http.Response{
		StatusCode: c.statusCode,
		Header:     http.Header{},
		Body:       io.NopCloser(strings.NewReader(c.response)),
	}, nil
}

func (c *sequenceHTTPClient) Do(req *http.Request) (*http.Response, error) {
	body := ""
	if req.Body != nil {
		rawBody, readErr := io.ReadAll(req.Body)
		if readErr != nil {
			c.t.Fatalf("read request body: %v", readErr)
		}
		body = string(rawBody)
	}
	c.calls = append(c.calls, capturedHTTPCall{
		method: req.Method,
		url:    req.URL.String(),
		body:   body,
	})

	if len(c.responses) == 0 {
		c.t.Fatal("unexpected graph request: no stubbed responses remaining")
	}
	response := c.responses[0]
	c.responses = c.responses[1:]

	if response.err != nil {
		return nil, response.err
	}
	return &http.Response{
		StatusCode: response.statusCode,
		Header:     http.Header{},
		Body:       io.NopCloser(strings.NewReader(response.response)),
	}, nil
}

func TestBuildUploadRequestShapesImagePayload(t *testing.T) {
	t.Parallel()

	request, mediaType, err := BuildUploadRequest("v25.0", "token-1", "secret-1", MediaUploadOptions{
		IGUserID:       "17841400008460056",
		MediaURL:       "https://cdn.example.com/image.jpg",
		Caption:        "hello instagram",
		MediaType:      "image",
		IsCarouselItem: true,
	})
	if err != nil {
		t.Fatalf("build upload request: %v", err)
	}

	if request.Method != "POST" {
		t.Fatalf("unexpected method %q", request.Method)
	}
	if request.Path != "17841400008460056/media" {
		t.Fatalf("unexpected path %q", request.Path)
	}
	if request.Version != "v25.0" {
		t.Fatalf("unexpected version %q", request.Version)
	}
	if request.Form["image_url"] != "https://cdn.example.com/image.jpg" {
		t.Fatalf("unexpected image_url %q", request.Form["image_url"])
	}
	if request.Form["caption"] != "hello instagram" {
		t.Fatalf("unexpected caption %q", request.Form["caption"])
	}
	if request.Form["is_carousel_item"] != "true" {
		t.Fatalf("unexpected is_carousel_item %q", request.Form["is_carousel_item"])
	}
	if mediaType != MediaTypeImage {
		t.Fatalf("unexpected media type %q", mediaType)
	}
}

func TestBuildUploadRequestRejectsInvalidInput(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		options  MediaUploadOptions
		errorMsg string
	}{
		{
			name: "missing ig user id",
			options: MediaUploadOptions{
				MediaURL:  "https://cdn.example.com/image.jpg",
				MediaType: MediaTypeImage,
			},
			errorMsg: "ig user id is required",
		},
		{
			name: "missing media type",
			options: MediaUploadOptions{
				IGUserID: "17841400008460056",
				MediaURL: "https://cdn.example.com/image.jpg",
			},
			errorMsg: "media type is required",
		},
		{
			name: "unsupported media type",
			options: MediaUploadOptions{
				IGUserID:  "17841400008460056",
				MediaURL:  "https://cdn.example.com/image.jpg",
				MediaType: "story",
			},
			errorMsg: "unsupported media type",
		},
		{
			name: "missing media url",
			options: MediaUploadOptions{
				IGUserID:  "17841400008460056",
				MediaType: MediaTypeImage,
			},
			errorMsg: "media url is required",
		},
		{
			name: "malformed graph id",
			options: MediaUploadOptions{
				IGUserID:  "17841400008460056/media",
				MediaURL:  "https://cdn.example.com/image.jpg",
				MediaType: MediaTypeImage,
			},
			errorMsg: "expected single graph id token",
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, _, err := BuildUploadRequest("v25.0", "token-1", "secret-1", tc.options)
			if err == nil {
				t.Fatal("expected error")
			}
			if !strings.Contains(err.Error(), tc.errorMsg) {
				t.Fatalf("expected error containing %q, got %v", tc.errorMsg, err)
			}
		})
	}
}

func TestBuildStatusRequestShapesFieldsQuery(t *testing.T) {
	t.Parallel()

	request, err := BuildStatusRequest("v25.0", "token-1", "secret-1", MediaStatusOptions{
		CreationID: "17900011122233344",
	})
	if err != nil {
		t.Fatalf("build status request: %v", err)
	}

	if request.Method != "GET" {
		t.Fatalf("unexpected method %q", request.Method)
	}
	if request.Path != "17900011122233344" {
		t.Fatalf("unexpected path %q", request.Path)
	}
	if request.Query["fields"] != "id,status,status_code" {
		t.Fatalf("unexpected fields query %q", request.Query["fields"])
	}
}

func TestBuildPublishRequestShapesPayload(t *testing.T) {
	t.Parallel()

	request, igUserID, creationID, err := BuildPublishRequest("v25.0", "token-1", "secret-1", MediaPublishOptions{
		IGUserID:   "17841400008460056",
		CreationID: "creation_1",
	})
	if err != nil {
		t.Fatalf("build publish request: %v", err)
	}

	if request.Method != "POST" {
		t.Fatalf("unexpected method %q", request.Method)
	}
	if request.Path != "17841400008460056/media_publish" {
		t.Fatalf("unexpected path %q", request.Path)
	}
	if request.Form["creation_id"] != "creation_1" {
		t.Fatalf("unexpected creation_id %q", request.Form["creation_id"])
	}
	if igUserID != "17841400008460056" {
		t.Fatalf("unexpected ig user id %q", igUserID)
	}
	if creationID != "creation_1" {
		t.Fatalf("unexpected creation id %q", creationID)
	}
}

func TestBuildPublishRequestRejectsInvalidInput(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		options  MediaPublishOptions
		errorMsg string
	}{
		{
			name: "missing ig user id",
			options: MediaPublishOptions{
				CreationID: "creation_1",
			},
			errorMsg: "ig user id is required",
		},
		{
			name: "missing creation id",
			options: MediaPublishOptions{
				IGUserID: "17841400008460056",
			},
			errorMsg: "creation id is required",
		},
		{
			name: "malformed creation id",
			options: MediaPublishOptions{
				IGUserID:   "17841400008460056",
				CreationID: "creation_1/media",
			},
			errorMsg: "expected single graph id token",
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, _, _, err := BuildPublishRequest("v25.0", "token-1", "secret-1", tc.options)
			if err == nil {
				t.Fatal("expected error")
			}
			if !strings.Contains(err.Error(), tc.errorMsg) {
				t.Fatalf("expected error containing %q, got %v", tc.errorMsg, err)
			}
		})
	}
}

func TestServiceUploadExecutesGraphRequest(t *testing.T) {
	t.Parallel()

	stub := &stubHTTPClient{
		t:          t,
		statusCode: http.StatusOK,
		response:   `{"id":"creation_1","status_code":"IN_PROGRESS"}`,
	}
	client := graph.NewClient(stub, "https://graph.example.com")
	client.MaxRetries = 0
	service := New(client)

	result, err := service.Upload(context.Background(), "v25.0", "token-1", "secret-1", MediaUploadOptions{
		IGUserID:       "17841400008460056",
		MediaURL:       "https://cdn.example.com/video.mp4",
		Caption:        "caption",
		MediaType:      MediaTypeVideo,
		IsCarouselItem: true,
	})
	if err != nil {
		t.Fatalf("upload media: %v", err)
	}

	if stub.calls != 1 {
		t.Fatalf("expected one graph call, got %d", stub.calls)
	}
	if stub.lastMethod != http.MethodPost {
		t.Fatalf("unexpected method %q", stub.lastMethod)
	}
	parsedURL, err := url.Parse(stub.lastURL)
	if err != nil {
		t.Fatalf("parse url: %v", err)
	}
	if parsedURL.Path != "/v25.0/17841400008460056/media" {
		t.Fatalf("unexpected path %q", parsedURL.Path)
	}

	form, err := url.ParseQuery(stub.lastBody)
	if err != nil {
		t.Fatalf("parse form body: %v", err)
	}
	if got := form.Get("video_url"); got != "https://cdn.example.com/video.mp4" {
		t.Fatalf("unexpected video_url %q", got)
	}
	if got := form.Get("caption"); got != "caption" {
		t.Fatalf("unexpected caption %q", got)
	}
	if got := form.Get("is_carousel_item"); got != "true" {
		t.Fatalf("unexpected is_carousel_item %q", got)
	}
	if got := form.Get("access_token"); got != "token-1" {
		t.Fatalf("unexpected access_token %q", got)
	}
	if got := form.Get("appsecret_proof"); got == "" {
		t.Fatal("expected appsecret_proof to be set")
	}

	if result.CreationID != "creation_1" {
		t.Fatalf("unexpected creation id %q", result.CreationID)
	}
	if result.MediaType != MediaTypeVideo {
		t.Fatalf("unexpected media type %q", result.MediaType)
	}
}

func TestServiceStatusExecutesGraphRequest(t *testing.T) {
	t.Parallel()

	stub := &stubHTTPClient{
		t:          t,
		statusCode: http.StatusOK,
		response:   `{"id":"creation_1","status":"IN_PROGRESS","status_code":"IN_PROGRESS"}`,
	}
	client := graph.NewClient(stub, "https://graph.example.com")
	client.MaxRetries = 0
	service := New(client)

	result, err := service.Status(context.Background(), "v25.0", "token-1", "secret-1", MediaStatusOptions{
		CreationID: "creation_1",
	})
	if err != nil {
		t.Fatalf("media status: %v", err)
	}

	if stub.calls != 1 {
		t.Fatalf("expected one graph call, got %d", stub.calls)
	}
	if stub.lastMethod != http.MethodGet {
		t.Fatalf("unexpected method %q", stub.lastMethod)
	}
	parsedURL, err := url.Parse(stub.lastURL)
	if err != nil {
		t.Fatalf("parse url: %v", err)
	}
	if parsedURL.Path != "/v25.0/creation_1" {
		t.Fatalf("unexpected path %q", parsedURL.Path)
	}
	query := parsedURL.Query()
	if got := query.Get("fields"); got != "id,status,status_code" {
		t.Fatalf("unexpected fields query %q", got)
	}
	if got := query.Get("access_token"); got != "token-1" {
		t.Fatalf("unexpected access_token %q", got)
	}
	if got := query.Get("appsecret_proof"); got == "" {
		t.Fatal("expected appsecret_proof query value")
	}

	if result.CreationID != "creation_1" {
		t.Fatalf("unexpected creation id %q", result.CreationID)
	}
	if result.Status != "IN_PROGRESS" {
		t.Fatalf("unexpected status %q", result.Status)
	}
}

func TestServicePublishExecutesGraphRequest(t *testing.T) {
	t.Parallel()

	stub := &stubHTTPClient{
		t:          t,
		statusCode: http.StatusOK,
		response:   `{"id":"media_1"}`,
	}
	client := graph.NewClient(stub, "https://graph.example.com")
	client.MaxRetries = 0
	service := New(client)

	result, err := service.Publish(context.Background(), "v25.0", "token-1", "secret-1", MediaPublishOptions{
		IGUserID:   "17841400008460056",
		CreationID: "creation_1",
	})
	if err != nil {
		t.Fatalf("publish media: %v", err)
	}

	if stub.calls != 1 {
		t.Fatalf("expected one graph call, got %d", stub.calls)
	}
	if stub.lastMethod != http.MethodPost {
		t.Fatalf("unexpected method %q", stub.lastMethod)
	}
	parsedURL, err := url.Parse(stub.lastURL)
	if err != nil {
		t.Fatalf("parse url: %v", err)
	}
	if parsedURL.Path != "/v25.0/17841400008460056/media_publish" {
		t.Fatalf("unexpected path %q", parsedURL.Path)
	}

	form, err := url.ParseQuery(stub.lastBody)
	if err != nil {
		t.Fatalf("parse form body: %v", err)
	}
	if got := form.Get("creation_id"); got != "creation_1" {
		t.Fatalf("unexpected creation_id %q", got)
	}
	if got := form.Get("access_token"); got != "token-1" {
		t.Fatalf("unexpected access_token %q", got)
	}
	if got := form.Get("appsecret_proof"); got == "" {
		t.Fatal("expected appsecret_proof to be set")
	}

	if result.MediaID != "media_1" {
		t.Fatalf("unexpected media id %q", result.MediaID)
	}
	if result.CreationID != "creation_1" {
		t.Fatalf("unexpected creation id %q", result.CreationID)
	}
}

func TestServicePublishFeedImmediateExecutesUploadStatusPublish(t *testing.T) {
	t.Parallel()

	stub := &sequenceHTTPClient{
		t: t,
		responses: []sequenceStubResponse{
			{
				statusCode: http.StatusOK,
				response:   `{"id":"creation_77","status_code":"IN_PROGRESS"}`,
			},
			{
				statusCode: http.StatusOK,
				response:   `{"id":"creation_77","status":"FINISHED","status_code":"FINISHED"}`,
			},
			{
				statusCode: http.StatusOK,
				response:   `{"id":"media_88"}`,
			},
		},
	}
	client := graph.NewClient(stub, "https://graph.example.com")
	client.MaxRetries = 0
	service := New(client)

	result, err := service.PublishFeedImmediate(context.Background(), "v25.0", "token-1", "secret-1", FeedPublishOptions{
		IGUserID:   "17841400008460056",
		MediaURL:   "https://cdn.example.com/image.jpg",
		Caption:    "hello #meta",
		MediaType:  MediaTypeImage,
		StrictMode: true,
	})
	if err != nil {
		t.Fatalf("publish feed immediate: %v", err)
	}

	if len(stub.calls) != 3 {
		t.Fatalf("expected three graph calls, got %d", len(stub.calls))
	}

	uploadURL, err := url.Parse(stub.calls[0].url)
	if err != nil {
		t.Fatalf("parse upload url: %v", err)
	}
	if uploadURL.Path != "/v25.0/17841400008460056/media" {
		t.Fatalf("unexpected upload path %q", uploadURL.Path)
	}
	uploadForm, err := url.ParseQuery(stub.calls[0].body)
	if err != nil {
		t.Fatalf("parse upload form: %v", err)
	}
	if got := uploadForm.Get("image_url"); got != "https://cdn.example.com/image.jpg" {
		t.Fatalf("unexpected image_url %q", got)
	}
	if got := uploadForm.Get("caption"); got != "hello #meta" {
		t.Fatalf("unexpected caption %q", got)
	}

	statusURL, err := url.Parse(stub.calls[1].url)
	if err != nil {
		t.Fatalf("parse status url: %v", err)
	}
	if statusURL.Path != "/v25.0/creation_77" {
		t.Fatalf("unexpected status path %q", statusURL.Path)
	}
	if got := statusURL.Query().Get("fields"); got != "id,status,status_code" {
		t.Fatalf("unexpected fields query %q", got)
	}

	publishURL, err := url.Parse(stub.calls[2].url)
	if err != nil {
		t.Fatalf("parse publish url: %v", err)
	}
	if publishURL.Path != "/v25.0/17841400008460056/media_publish" {
		t.Fatalf("unexpected publish path %q", publishURL.Path)
	}
	publishForm, err := url.ParseQuery(stub.calls[2].body)
	if err != nil {
		t.Fatalf("parse publish form: %v", err)
	}
	if got := publishForm.Get("creation_id"); got != "creation_77" {
		t.Fatalf("unexpected creation_id %q", got)
	}

	if result.Mode != "immediate" {
		t.Fatalf("unexpected mode %q", result.Mode)
	}
	if result.CreationID != "creation_77" {
		t.Fatalf("unexpected creation id %q", result.CreationID)
	}
	if result.MediaID != "media_88" {
		t.Fatalf("unexpected media id %q", result.MediaID)
	}
	if result.StatusCode != MediaStatusCodeFinished {
		t.Fatalf("unexpected status code %q", result.StatusCode)
	}
	if !result.CaptionValidation.Valid {
		t.Fatalf("expected caption validation to pass, got errors: %v", result.CaptionValidation.Errors)
	}
}

func TestServicePublishFeedImmediateFailsWhenContainerNotReady(t *testing.T) {
	t.Parallel()

	stub := &sequenceHTTPClient{
		t: t,
		responses: []sequenceStubResponse{
			{
				statusCode: http.StatusOK,
				response:   `{"id":"creation_77","status_code":"IN_PROGRESS"}`,
			},
			{
				statusCode: http.StatusOK,
				response:   `{"id":"creation_77","status":"IN_PROGRESS","status_code":"IN_PROGRESS"}`,
			},
		},
	}
	client := graph.NewClient(stub, "https://graph.example.com")
	client.MaxRetries = 0
	service := New(client)

	_, err := service.PublishFeedImmediate(context.Background(), "v25.0", "token-1", "secret-1", FeedPublishOptions{
		IGUserID:   "17841400008460056",
		MediaURL:   "https://cdn.example.com/image.jpg",
		Caption:    "hello #meta",
		MediaType:  MediaTypeImage,
		StrictMode: true,
	})
	if err == nil {
		t.Fatal("expected publish failure")
	}
	if !strings.Contains(err.Error(), "not ready for publish") {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(stub.calls) != 2 {
		t.Fatalf("expected upload+status only, got %d calls", len(stub.calls))
	}
}

func TestServicePublishFeedImmediateFailsCaptionValidationBeforeUpload(t *testing.T) {
	t.Parallel()

	stub := &sequenceHTTPClient{
		t:         t,
		responses: []sequenceStubResponse{},
	}
	client := graph.NewClient(stub, "https://graph.example.com")
	client.MaxRetries = 0
	service := New(client)

	_, err := service.PublishFeedImmediate(context.Background(), "v25.0", "token-1", "secret-1", FeedPublishOptions{
		IGUserID:   "17841400008460056",
		MediaURL:   "https://cdn.example.com/image.jpg",
		Caption:    strings.Repeat("a", CaptionWarningCharacters+5),
		MediaType:  MediaTypeImage,
		StrictMode: true,
	})
	if err == nil {
		t.Fatal("expected publish validation error")
	}
	if !strings.Contains(err.Error(), "strict mode:") {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(stub.calls) != 0 {
		t.Fatalf("expected no graph calls, got %d", len(stub.calls))
	}
}

func TestServiceUploadFailsWhenResponseMissingCreationID(t *testing.T) {
	t.Parallel()

	stub := &stubHTTPClient{
		t:          t,
		statusCode: http.StatusOK,
		response:   `{"status_code":"IN_PROGRESS"}`,
	}
	client := graph.NewClient(stub, "https://graph.example.com")
	client.MaxRetries = 0
	service := New(client)

	_, err := service.Upload(context.Background(), "v25.0", "token-1", "secret-1", MediaUploadOptions{
		IGUserID:  "17841400008460056",
		MediaURL:  "https://cdn.example.com/image.jpg",
		MediaType: MediaTypeImage,
	})
	if err == nil {
		t.Fatal("expected upload error")
	}
	if !strings.Contains(err.Error(), "did not include id") {
		t.Fatalf("unexpected error: %v", err)
	}
}
