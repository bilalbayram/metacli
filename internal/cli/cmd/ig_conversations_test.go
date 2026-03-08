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

func TestIGConversationsListSendsGetToIGUserConversations(t *testing.T) {
	stub := &stubHTTPClient{
		t:          t,
		statusCode: http.StatusOK,
		response:   `{"data":[{"id":"conv_ig_1","updated_time":"2025-01-01T00:00:00+0000"}]}`,
	}
	useIGDependencies(t,
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
	cmd := NewIGCommand(testRuntime("prod"))
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
	cmd.SetOut(output)
	cmd.SetErr(errOutput)
	cmd.SetArgs([]string{
		"conversations", "list",
		"--ig-user-id", "17841400008460056",
	})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute ig conversations list: %v", err)
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
	if parsedURL.Path != "/v25.0/17841400008460056/conversations" {
		t.Fatalf("unexpected request path %q", parsedURL.Path)
	}
	query := parsedURL.Query()
	if got := query.Get("platform"); got != "instagram" {
		t.Fatalf("unexpected platform query %q", got)
	}

	envelope := decodeEnvelope(t, output.Bytes())
	assertEnvelopeBasics(t, envelope, "meta ig conversations list")
	if errOutput.Len() != 0 {
		t.Fatalf("expected empty stderr, got %q", errOutput.String())
	}
}

func TestIGConversationsListFailsWithoutIGUserID(t *testing.T) {
	useIGDependencies(t,
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

	cmd := NewIGCommand(testRuntime("prod"))
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	cmd.SetArgs([]string{"conversations", "list"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for missing ig user id")
	}
	if !strings.Contains(err.Error(), "ig user id is required") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestIGConversationsReplySendsPostToIGUserMessages(t *testing.T) {
	stub := &stubHTTPClient{
		t:          t,
		statusCode: http.StatusOK,
		response:   `{"recipient_id":"ig_user_456","message_id":"mid.xyz"}`,
	}
	useIGDependencies(t,
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
	cmd := NewIGCommand(testRuntime("prod"))
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
	cmd.SetOut(output)
	cmd.SetErr(errOutput)
	cmd.SetArgs([]string{
		"conversations", "reply",
		"--ig-user-id", "17841400008460056",
		"--recipient-id", "ig_user_456",
		"--message", "Thanks for your message!",
	})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute ig conversations reply: %v", err)
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
	if parsedURL.Path != "/v25.0/17841400008460056/messages" {
		t.Fatalf("unexpected request path %q", parsedURL.Path)
	}

	envelope := decodeEnvelope(t, output.Bytes())
	assertEnvelopeBasics(t, envelope, "meta ig conversations reply")
	if errOutput.Len() != 0 {
		t.Fatalf("expected empty stderr, got %q", errOutput.String())
	}
}

func TestIGConversationsReplyFailsWithoutRecipientID(t *testing.T) {
	useIGDependencies(t,
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

	cmd := NewIGCommand(testRuntime("prod"))
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	cmd.SetArgs([]string{
		"conversations", "reply",
		"--ig-user-id", "17841400008460056",
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

func TestIGConversationsReplyFailsWithoutMessage(t *testing.T) {
	useIGDependencies(t,
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

	cmd := NewIGCommand(testRuntime("prod"))
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	cmd.SetArgs([]string{
		"conversations", "reply",
		"--ig-user-id", "17841400008460056",
		"--recipient-id", "ig_user_456",
	})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for missing message")
	}
	if !strings.Contains(err.Error(), "message is required") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestIGConversationsCommandFailsWithoutSubcommand(t *testing.T) {
	cmd := NewIGCommand(Runtime{})
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	cmd.SetArgs([]string{"conversations"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error when conversations is executed without subcommand")
	}
	if !strings.Contains(err.Error(), "ig conversations requires a subcommand") {
		t.Fatalf("unexpected error: %v", err)
	}
}
