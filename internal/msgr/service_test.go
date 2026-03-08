package msgr

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/bilalbayram/metacli/internal/graph"
)

func TestBuildListConversationsRequestRequiresPageID(t *testing.T) {
	t.Parallel()

	_, _, err := BuildListConversationsRequest("v25.0", "token", "", ListConversationsOptions{})
	if err == nil {
		t.Fatal("expected error for missing page id")
	}
	if err.Error() != "page id is required" {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestBuildListConversationsRequestShapesPath(t *testing.T) {
	t.Parallel()

	req, pageID, err := BuildListConversationsRequest("v25.0", "test-token", "test-secret", ListConversationsOptions{
		PageID: "123456",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pageID != "123456" {
		t.Fatalf("unexpected page id %q", pageID)
	}
	if req.Path != "123456/conversations" {
		t.Fatalf("unexpected path %q", req.Path)
	}
	if req.Method != "GET" {
		t.Fatalf("unexpected method %q", req.Method)
	}
	if req.Query["fields"] != "id,updated_time,participants,messages{message,from,created_time}" {
		t.Fatalf("unexpected fields %q", req.Query["fields"])
	}
	if req.AccessToken != "test-token" {
		t.Fatalf("unexpected access token %q", req.AccessToken)
	}
}

func TestBuildReplyRequestRequiresRecipientID(t *testing.T) {
	t.Parallel()

	_, err := BuildReplyRequest("v25.0", "token", "", ReplyOptions{
		Message: "hello",
	})
	if err == nil {
		t.Fatal("expected error for missing recipient id")
	}
	if err.Error() != "recipient id is required" {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestBuildReplyRequestRequiresMessage(t *testing.T) {
	t.Parallel()

	_, err := BuildReplyRequest("v25.0", "token", "", ReplyOptions{
		RecipientID: "psid_123",
	})
	if err == nil {
		t.Fatal("expected error for missing message")
	}
	if err.Error() != "message is required" {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestBuildReplyRequestShapesPostToMeMessages(t *testing.T) {
	t.Parallel()

	req, err := BuildReplyRequest("v25.0", "test-token", "test-secret", ReplyOptions{
		RecipientID: "psid_123",
		Message:     "hello there",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if req.Method != "POST" {
		t.Fatalf("unexpected method %q", req.Method)
	}
	if req.Path != "me/messages" {
		t.Fatalf("unexpected path %q", req.Path)
	}
	if req.Form["messaging_type"] != "RESPONSE" {
		t.Fatalf("unexpected messaging_type %q", req.Form["messaging_type"])
	}
}

func TestBuildReplyRequestEscapesJSONPayloads(t *testing.T) {
	t.Parallel()

	req, err := BuildReplyRequest("v25.0", "test-token", "test-secret", ReplyOptions{
		RecipientID: "psid_123",
		Message:     "He said \"hi\"\nand sent a slash \\",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var recipient struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal([]byte(req.Form["recipient"]), &recipient); err != nil {
		t.Fatalf("decode recipient payload: %v", err)
	}
	if recipient.ID != "psid_123" {
		t.Fatalf("unexpected recipient id %q", recipient.ID)
	}

	var message struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal([]byte(req.Form["message"]), &message); err != nil {
		t.Fatalf("decode message payload: %v", err)
	}
	if message.Text != "He said \"hi\"\nand sent a slash \\" {
		t.Fatalf("unexpected message text %q", message.Text)
	}
}

func TestBuildSetGreetingRequestRequiresPageID(t *testing.T) {
	t.Parallel()

	_, _, err := BuildSetGreetingRequest("v25.0", "token", "", SetGreetingOptions{
		Message: "hello",
	})
	if err == nil {
		t.Fatal("expected error for missing page id")
	}
	if err.Error() != "page id is required" {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestBuildSetGreetingRequestRequiresMessage(t *testing.T) {
	t.Parallel()

	_, _, err := BuildSetGreetingRequest("v25.0", "token", "", SetGreetingOptions{
		PageID: "123456",
	})
	if err == nil {
		t.Fatal("expected error for missing message")
	}
	if err.Error() != "message is required" {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestBuildSetGreetingRequestShapesPost(t *testing.T) {
	t.Parallel()

	req, pageID, err := BuildSetGreetingRequest("v25.0", "test-token", "test-secret", SetGreetingOptions{
		PageID:  "123456",
		Message: "Welcome!",
		Locale:  "en_US",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pageID != "123456" {
		t.Fatalf("unexpected page id %q", pageID)
	}
	if req.Method != "POST" {
		t.Fatalf("unexpected method %q", req.Method)
	}
	if req.Path != "123456/messenger_profile" {
		t.Fatalf("unexpected path %q", req.Path)
	}
	if req.Form["greeting"] == "" {
		t.Fatal("expected greeting form field")
	}
}

func TestBuildSetGreetingRequestEscapesGreetingPayload(t *testing.T) {
	t.Parallel()

	req, _, err := BuildSetGreetingRequest("v25.0", "test-token", "test-secret", SetGreetingOptions{
		PageID:  "123456",
		Message: "Welcome \"friend\"\nThanks for reaching out",
		Locale:  "en_US",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var greeting []struct {
		Locale string `json:"locale"`
		Text   string `json:"text"`
	}
	if err := json.Unmarshal([]byte(req.Form["greeting"]), &greeting); err != nil {
		t.Fatalf("decode greeting payload: %v", err)
	}
	if len(greeting) != 1 {
		t.Fatalf("expected one greeting entry, got %d", len(greeting))
	}
	if greeting[0].Locale != "en_US" {
		t.Fatalf("unexpected locale %q", greeting[0].Locale)
	}
	if greeting[0].Text != "Welcome \"friend\"\nThanks for reaching out" {
		t.Fatalf("unexpected greeting text %q", greeting[0].Text)
	}
}

func TestBuildSetGreetingRequestDefaultsLocale(t *testing.T) {
	t.Parallel()

	req, _, err := BuildSetGreetingRequest("v25.0", "test-token", "", SetGreetingOptions{
		PageID:  "123456",
		Message: "Welcome!",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if req.Form["greeting"] == "" {
		t.Fatal("expected greeting form field")
	}
	if !contains(req.Form["greeting"], "default") {
		t.Fatalf("expected default locale in greeting payload, got %q", req.Form["greeting"])
	}
}

func TestServiceListConversationsFollowsNextPages(t *testing.T) {
	t.Parallel()

	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v25.0/page_123/conversations" {
			t.Fatalf("unexpected path %q", r.URL.Path)
		}
		if r.URL.Query().Get("after") == "" {
			if err := json.NewEncoder(w).Encode(map[string]any{
				"data": []map[string]any{
					{"id": "conv_1"},
					{"id": "conv_2"},
				},
				"paging": map[string]any{
					"next": server.URL + "/v25.0/page_123/conversations?after=cursor_1",
				},
			}); err != nil {
				t.Fatalf("encode first page: %v", err)
			}
			return
		}

		if got := r.URL.Query().Get("after"); got != "cursor_1" {
			t.Fatalf("unexpected after cursor %q", got)
		}
		if err := json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]any{
				{"id": "conv_3"},
			},
		}); err != nil {
			t.Fatalf("encode second page: %v", err)
		}
	}))
	defer server.Close()

	client := graph.NewClient(server.Client(), server.URL)
	client.MaxRetries = 0
	service := New(client)

	result, err := service.ListConversations(context.Background(), "v25.0", "token-1", "secret-1", ListConversationsOptions{
		PageID: "page_123",
		Limit:  3,
	})
	if err != nil {
		t.Fatalf("list conversations: %v", err)
	}

	if len(result.Conversations) != 3 {
		t.Fatalf("expected 3 conversations, got %d", len(result.Conversations))
	}
	if result.Pagination == nil {
		t.Fatal("expected pagination metadata")
	}
	if result.Pagination.PagesFetched != 2 {
		t.Fatalf("expected 2 pages fetched, got %d", result.Pagination.PagesFetched)
	}
	if result.Pagination.ItemsFetched != 3 {
		t.Fatalf("expected 3 items fetched, got %d", result.Pagination.ItemsFetched)
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && searchSubstring(s, substr)
}

func searchSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
