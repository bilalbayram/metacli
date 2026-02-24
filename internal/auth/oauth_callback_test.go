package auth

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"testing"
	"time"
)

func TestOAuthCallbackListenerReturnsAuthorizationCode(t *testing.T) {
	t.Parallel()

	listener, err := NewOAuthCallbackListener("http://127.0.0.1:0/callback", "state-123")
	if err != nil {
		t.Fatalf("new callback listener: %v", err)
	}
	t.Cleanup(func() {
		_ = listener.Close(context.Background())
	})

	go func() {
		res, reqErr := http.Get(listener.RedirectURI() + "?code=auth-code&state=state-123")
		if reqErr != nil {
			return
		}
		defer res.Body.Close()
		_, _ = io.ReadAll(res.Body)
	}()

	code, err := listener.Wait(context.Background(), 2*time.Second)
	if err != nil {
		t.Fatalf("wait for callback: %v", err)
	}
	if code != "auth-code" {
		t.Fatalf("unexpected code: got=%s want=auth-code", code)
	}
}

func TestOAuthCallbackListenerFailsOnStateMismatch(t *testing.T) {
	t.Parallel()

	listener, err := NewOAuthCallbackListener("http://127.0.0.1:0/callback", "expected-state")
	if err != nil {
		t.Fatalf("new callback listener: %v", err)
	}
	t.Cleanup(func() {
		_ = listener.Close(context.Background())
	})

	go func() {
		res, reqErr := http.Get(listener.RedirectURI() + "?code=auth-code&state=wrong")
		if reqErr != nil {
			return
		}
		defer res.Body.Close()
		_, _ = io.ReadAll(res.Body)
	}()

	_, err = listener.Wait(context.Background(), 2*time.Second)
	if !errors.Is(err, ErrOAuthStateMismatch) {
		t.Fatalf("expected ErrOAuthStateMismatch, got: %v", err)
	}
}

func TestOAuthCallbackListenerFailsOnTimeout(t *testing.T) {
	t.Parallel()

	listener, err := NewOAuthCallbackListener("http://127.0.0.1:0/callback", "state-timeout")
	if err != nil {
		t.Fatalf("new callback listener: %v", err)
	}
	t.Cleanup(func() {
		_ = listener.Close(context.Background())
	})

	_, err = listener.Wait(context.Background(), 100*time.Millisecond)
	if !errors.Is(err, ErrOAuthCallbackTimeout) {
		t.Fatalf("expected ErrOAuthCallbackTimeout, got: %v", err)
	}
}

func TestOAuthCallbackListenerFailsOnProviderError(t *testing.T) {
	t.Parallel()

	listener, err := NewOAuthCallbackListener("http://127.0.0.1:0/callback", "state-123")
	if err != nil {
		t.Fatalf("new callback listener: %v", err)
	}
	t.Cleanup(func() {
		_ = listener.Close(context.Background())
	})

	go func() {
		res, reqErr := http.Get(listener.RedirectURI() + "?error=access_denied&error_description=user+denied")
		if reqErr != nil {
			return
		}
		defer res.Body.Close()
		_, _ = io.ReadAll(res.Body)
	}()

	_, err = listener.Wait(context.Background(), 2*time.Second)
	if !errors.Is(err, ErrOAuthCallbackProviderError) {
		t.Fatalf("expected ErrOAuthCallbackProviderError, got: %v", err)
	}
}

func TestOAuthCallbackListenerRejectsNonLocalhostRedirectURI(t *testing.T) {
	t.Parallel()

	_, err := NewOAuthCallbackListener("https://example.com/callback", "state-123")
	if !errors.Is(err, ErrOAuthCallbackInvalidRedirectURI) {
		t.Fatalf("expected ErrOAuthCallbackInvalidRedirectURI, got: %v", err)
	}
}

func TestListenForOAuthCodeHelper(t *testing.T) {
	t.Parallel()

	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen for free port: %v", err)
	}
	port := l.Addr().(*net.TCPAddr).Port
	if closeErr := l.Close(); closeErr != nil {
		t.Fatalf("close probe listener: %v", closeErr)
	}
	redirectURI := fmt.Sprintf("http://127.0.0.1:%d/callback", port)

	done := make(chan struct{})
	var gotCode string
	var gotErr error
	go func() {
		defer close(done)
		gotCode, gotErr = ListenForOAuthCode(context.Background(), OAuthCallbackInput{
			RedirectURI:   redirectURI,
			ExpectedState: "helper-state",
			Timeout:       2 * time.Second,
		})
	}()

	time.Sleep(150 * time.Millisecond)
	res, err := http.Get(redirectURI + "?code=helper-code&state=helper-state")
	if err != nil {
		t.Fatalf("request callback url: %v", err)
	}
	defer res.Body.Close()
	_, _ = io.ReadAll(res.Body)

	<-done
	if gotErr != nil {
		t.Fatalf("listen for oauth code: %v", gotErr)
	}
	if gotCode != "helper-code" {
		t.Fatalf("unexpected code: got=%s want=helper-code", gotCode)
	}
}
