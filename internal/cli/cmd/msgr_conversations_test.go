package cmd

import (
	"bytes"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/bilalbayram/metacli/internal/config"
	"github.com/bilalbayram/metacli/internal/graph"
)

func TestMSGRConversationsListSendsGetToPageConversations(t *testing.T) {
	stub := &stubHTTPClient{
		t:          t,
		statusCode: http.StatusOK,
		response:   `{"data":[{"id":"conv_1","updated_time":"2025-01-01T00:00:00+0000"}]}`,
	}
	useMSGRDependencies(t,
		func(profile string) (*ProfileCredentials, error) {
			return &ProfileCredentials{
				Name:      "prod",
				Profile:   config.Profile{GraphVersion: "v25.0", PageID: "page_123"},
				Token:     "test-token",
				AppSecret: "test-secret",
			}, nil
		},
		func() *graph.Client {
			client := graph.NewClient(stub, "https://graph.example.com")
			client.MaxRetries = 0
			return client
		},
	)

	output := &bytes.Buffer{}
	errOutput := &bytes.Buffer{}
	cmd := NewMSGRCommand(testRuntime("prod"))
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
	cmd.SetOut(output)
	cmd.SetErr(errOutput)
	cmd.SetArgs([]string{
		"conversations", "list",
		"--page-id", "page_123",
	})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute msgr conversations list: %v", err)
	}

	if stub.calls != 1 {
		t.Fatalf("expected one graph call, got %d", stub.calls)
	}
	if stub.lastMethod != http.MethodGet {
		t.Fatalf("unexpected method %q", stub.lastMethod)
	}

	parsedURL, err := url.Parse(stub.lastURL)
	if err != nil {
		t.Fatalf("parse request url: %v", err)
	}
	if parsedURL.Path != "/v25.0/page_123/conversations" {
		t.Fatalf("unexpected request path %q", parsedURL.Path)
	}

	envelope := decodeEnvelope(t, output.Bytes())
	assertEnvelopeBasics(t, envelope, "meta msgr conversations list")
	if errOutput.Len() != 0 {
		t.Fatalf("expected empty stderr, got %q", errOutput.String())
	}
}

func TestMSGRConversationsListUsesProfilePageID(t *testing.T) {
	stub := &stubHTTPClient{
		t:          t,
		statusCode: http.StatusOK,
		response:   `{"data":[]}`,
	}
	useMSGRDependencies(t,
		func(string) (*ProfileCredentials, error) {
			return &ProfileCredentials{
				Name:      "prod",
				Profile:   config.Profile{GraphVersion: "v25.0", PageID: "profile_page_456"},
				Token:     "test-token",
				AppSecret: "test-secret",
			}, nil
		},
		func() *graph.Client {
			client := graph.NewClient(stub, "https://graph.example.com")
			client.MaxRetries = 0
			return client
		},
	)

	output := &bytes.Buffer{}
	cmd := NewMSGRCommand(testRuntime("prod"))
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
	cmd.SetOut(output)
	cmd.SetErr(io.Discard)
	cmd.SetArgs([]string{"conversations", "list"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute msgr conversations list: %v", err)
	}

	parsedURL, err := url.Parse(stub.lastURL)
	if err != nil {
		t.Fatalf("parse request url: %v", err)
	}
	if parsedURL.Path != "/v25.0/profile_page_456/conversations" {
		t.Fatalf("unexpected request path %q", parsedURL.Path)
	}
}

func TestMSGRConversationsListFailsWithoutPageID(t *testing.T) {
	useMSGRDependencies(t,
		func(string) (*ProfileCredentials, error) {
			return &ProfileCredentials{
				Name:    "prod",
				Profile: config.Profile{GraphVersion: "v25.0"},
				Token:   "test-token",
			}, nil
		},
		func() *graph.Client {
			return graph.NewClient(nil, "")
		},
	)

	cmd := NewMSGRCommand(testRuntime("prod"))
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	cmd.SetArgs([]string{"conversations", "list"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for missing page id")
	}
	if !strings.Contains(err.Error(), "page id is required") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestMSGRConversationsReplySendsPostToMeMessages(t *testing.T) {
	stub := &stubHTTPClient{
		t:          t,
		statusCode: http.StatusOK,
		response:   `{"recipient_id":"psid_123","message_id":"mid.abc"}`,
	}
	useMSGRDependencies(t,
		func(string) (*ProfileCredentials, error) {
			return &ProfileCredentials{
				Name:      "prod",
				Profile:   config.Profile{GraphVersion: "v25.0"},
				Token:     "test-token",
				AppSecret: "test-secret",
			}, nil
		},
		func() *graph.Client {
			client := graph.NewClient(stub, "https://graph.example.com")
			client.MaxRetries = 0
			return client
		},
	)

	output := &bytes.Buffer{}
	errOutput := &bytes.Buffer{}
	cmd := NewMSGRCommand(testRuntime("prod"))
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
	cmd.SetOut(output)
	cmd.SetErr(errOutput)
	cmd.SetArgs([]string{
		"conversations", "reply",
		"--recipient-id", "psid_123",
		"--message", "Hello from CLI",
	})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute msgr conversations reply: %v", err)
	}

	if stub.calls != 1 {
		t.Fatalf("expected one graph call, got %d", stub.calls)
	}
	if stub.lastMethod != http.MethodPost {
		t.Fatalf("unexpected method %q", stub.lastMethod)
	}

	parsedURL, err := url.Parse(stub.lastURL)
	if err != nil {
		t.Fatalf("parse request url: %v", err)
	}
	if parsedURL.Path != "/v25.0/me/messages" {
		t.Fatalf("unexpected request path %q", parsedURL.Path)
	}

	envelope := decodeEnvelope(t, output.Bytes())
	assertEnvelopeBasics(t, envelope, "meta msgr conversations reply")
	if errOutput.Len() != 0 {
		t.Fatalf("expected empty stderr, got %q", errOutput.String())
	}
}

func TestMSGRConversationsReplyFailsWithoutRecipientID(t *testing.T) {
	useMSGRDependencies(t,
		func(string) (*ProfileCredentials, error) {
			return &ProfileCredentials{
				Name:    "prod",
				Profile: config.Profile{GraphVersion: "v25.0"},
				Token:   "test-token",
			}, nil
		},
		func() *graph.Client {
			return graph.NewClient(nil, "")
		},
	)

	cmd := NewMSGRCommand(testRuntime("prod"))
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	cmd.SetArgs([]string{
		"conversations", "reply",
		"--message", "hello",
	})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for missing recipient id")
	}
	if !strings.Contains(err.Error(), "recipient id is required") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestMSGRConversationsReplyFailsWithoutMessage(t *testing.T) {
	useMSGRDependencies(t,
		func(string) (*ProfileCredentials, error) {
			return &ProfileCredentials{
				Name:    "prod",
				Profile: config.Profile{GraphVersion: "v25.0"},
				Token:   "test-token",
			}, nil
		},
		func() *graph.Client {
			return graph.NewClient(nil, "")
		},
	)

	cmd := NewMSGRCommand(testRuntime("prod"))
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	cmd.SetArgs([]string{
		"conversations", "reply",
		"--recipient-id", "psid_123",
	})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for missing message")
	}
	if !strings.Contains(err.Error(), "message is required") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestMSGRAutoReplySetSendsPostToMessengerProfile(t *testing.T) {
	stub := &stubHTTPClient{
		t:          t,
		statusCode: http.StatusOK,
		response:   `{"result":"success"}`,
	}
	useMSGRDependencies(t,
		func(string) (*ProfileCredentials, error) {
			return &ProfileCredentials{
				Name:      "prod",
				Profile:   config.Profile{GraphVersion: "v25.0", PageID: "page_123"},
				Token:     "test-token",
				AppSecret: "test-secret",
			}, nil
		},
		func() *graph.Client {
			client := graph.NewClient(stub, "https://graph.example.com")
			client.MaxRetries = 0
			return client
		},
	)

	output := &bytes.Buffer{}
	errOutput := &bytes.Buffer{}
	cmd := NewMSGRCommand(testRuntime("prod"))
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
	cmd.SetOut(output)
	cmd.SetErr(errOutput)
	cmd.SetArgs([]string{
		"auto-reply", "set",
		"--page-id", "page_123",
		"--message", "Thanks for reaching out!",
	})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute msgr auto-reply set: %v", err)
	}

	if stub.calls != 1 {
		t.Fatalf("expected one graph call, got %d", stub.calls)
	}
	if stub.lastMethod != http.MethodPost {
		t.Fatalf("unexpected method %q", stub.lastMethod)
	}

	parsedURL, err := url.Parse(stub.lastURL)
	if err != nil {
		t.Fatalf("parse request url: %v", err)
	}
	if parsedURL.Path != "/v25.0/page_123/messenger_profile" {
		t.Fatalf("unexpected request path %q", parsedURL.Path)
	}

	envelope := decodeEnvelope(t, output.Bytes())
	assertEnvelopeBasics(t, envelope, "meta msgr auto-reply set")
	if errOutput.Len() != 0 {
		t.Fatalf("expected empty stderr, got %q", errOutput.String())
	}
}

func TestMSGRAutoReplySetFailsWithoutMessage(t *testing.T) {
	useMSGRDependencies(t,
		func(string) (*ProfileCredentials, error) {
			return &ProfileCredentials{
				Name:    "prod",
				Profile: config.Profile{GraphVersion: "v25.0", PageID: "page_123"},
				Token:   "test-token",
			}, nil
		},
		func() *graph.Client {
			return graph.NewClient(nil, "")
		},
	)

	cmd := NewMSGRCommand(testRuntime("prod"))
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	cmd.SetArgs([]string{
		"auto-reply", "set",
		"--page-id", "page_123",
	})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for missing message")
	}
	if !strings.Contains(err.Error(), "message is required") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestMSGRConversationsCommandFailsWithoutSubcommand(t *testing.T) {
	cmd := NewMSGRCommand(Runtime{})
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	cmd.SetArgs([]string{"conversations"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error when conversations is executed without subcommand")
	}
	if !strings.Contains(err.Error(), "msgr conversations requires a subcommand") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestMSGRAutoReplyCommandFailsWithoutSubcommand(t *testing.T) {
	cmd := NewMSGRCommand(Runtime{})
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	cmd.SetArgs([]string{"auto-reply"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error when auto-reply is executed without subcommand")
	}
	if !strings.Contains(err.Error(), "msgr auto-reply requires a subcommand") {
		t.Fatalf("unexpected error: %v", err)
	}
}
