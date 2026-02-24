package auth

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

var (
	ErrOAuthCallbackInvalidRedirectURI = errors.New("oauth callback redirect uri is invalid")
	ErrOAuthCallbackStateRequired      = errors.New("oauth callback expected state is required")
	ErrOAuthCallbackTimeout            = errors.New("oauth callback timed out")
	ErrOAuthStateMismatch              = errors.New("oauth callback state mismatch")
	ErrOAuthCallbackCodeMissing        = errors.New("oauth callback authorization code is missing")
	ErrOAuthCallbackProviderError      = errors.New("oauth callback returned provider error")
)

type OAuthCallbackInput struct {
	RedirectURI   string
	ExpectedState string
	Timeout       time.Duration
}

type OAuthCallbackListener struct {
	redirectURI   string
	expectedState string
	listener      net.Listener
	server        *http.Server
	result        chan oauthCallbackResult
	resultOnce    sync.Once
	closeOnce     sync.Once
}

type oauthCallbackResult struct {
	code string
	err  error
}

func NewOAuthCallbackListener(redirectURI string, expectedState string) (*OAuthCallbackListener, error) {
	parsed, err := parseOAuthCallbackRedirectURI(redirectURI)
	if err != nil {
		return nil, err
	}
	expectedState = strings.TrimSpace(expectedState)
	if expectedState == "" {
		return nil, ErrOAuthCallbackStateRequired
	}

	lis, err := net.Listen("tcp", parsed.Host)
	if err != nil {
		return nil, fmt.Errorf("listen for oauth callback %q: %w", parsed.Host, err)
	}

	callbackURI := *parsed
	callbackURI.Host = net.JoinHostPort(parsed.Hostname(), strconv.Itoa(lis.Addr().(*net.TCPAddr).Port))
	listener := &OAuthCallbackListener{
		redirectURI:   callbackURI.String(),
		expectedState: expectedState,
		listener:      lis,
		result:        make(chan oauthCallbackResult, 1),
	}

	mux := http.NewServeMux()
	mux.HandleFunc(callbackURI.Path, listener.handleCallback)
	listener.server = &http.Server{
		Handler: mux,
	}

	go listener.serve()
	return listener, nil
}

func (l *OAuthCallbackListener) RedirectURI() string {
	return l.redirectURI
}

func (l *OAuthCallbackListener) Wait(ctx context.Context, timeout time.Duration) (string, error) {
	if timeout <= 0 {
		return "", errors.New("oauth callback timeout must be greater than zero")
	}

	waitCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	select {
	case result := <-l.result:
		_ = l.Close(context.Background())
		if result.err != nil {
			return "", result.err
		}
		return result.code, nil
	case <-waitCtx.Done():
		_ = l.Close(context.Background())
		if errors.Is(waitCtx.Err(), context.DeadlineExceeded) {
			return "", fmt.Errorf("%w after %s", ErrOAuthCallbackTimeout, timeout)
		}
		return "", waitCtx.Err()
	}
}

func (l *OAuthCallbackListener) Close(ctx context.Context) error {
	var closeErr error
	l.closeOnce.Do(func() {
		closeErr = l.server.Shutdown(ctx)
	})
	return closeErr
}

func ListenForOAuthCode(ctx context.Context, input OAuthCallbackInput) (string, error) {
	listener, err := NewOAuthCallbackListener(input.RedirectURI, input.ExpectedState)
	if err != nil {
		return "", err
	}
	defer listener.Close(context.Background())

	return listener.Wait(ctx, input.Timeout)
}

func (l *OAuthCallbackListener) serve() {
	if err := l.server.Serve(l.listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
		l.resolve("", fmt.Errorf("oauth callback server failed: %w", err))
	}
}

func (l *OAuthCallbackListener) handleCallback(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	query := r.URL.Query()
	if providerError := strings.TrimSpace(query.Get("error")); providerError != "" {
		description := strings.TrimSpace(query.Get("error_description"))
		http.Error(w, "OAuth authorization failed. You can close this tab.", http.StatusBadRequest)
		l.resolve("", fmt.Errorf("%w: error=%s description=%s", ErrOAuthCallbackProviderError, providerError, description))
		return
	}

	actualState := strings.TrimSpace(query.Get("state"))
	if actualState != l.expectedState {
		http.Error(w, "OAuth state mismatch. You can close this tab.", http.StatusBadRequest)
		l.resolve("", fmt.Errorf("%w: expected=%q actual=%q", ErrOAuthStateMismatch, l.expectedState, actualState))
		return
	}

	code := strings.TrimSpace(query.Get("code"))
	if code == "" {
		http.Error(w, "OAuth code missing. You can close this tab.", http.StatusBadRequest)
		l.resolve("", ErrOAuthCallbackCodeMissing)
		return
	}

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("Authentication complete. You can close this tab."))
	l.resolve(code, nil)
}

func (l *OAuthCallbackListener) resolve(code string, err error) {
	l.resultOnce.Do(func() {
		l.result <- oauthCallbackResult{code: code, err: err}
	})
}

func parseOAuthCallbackRedirectURI(raw string) (*url.URL, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, fmt.Errorf("%w: redirect uri is required", ErrOAuthCallbackInvalidRedirectURI)
	}

	parsed, err := url.Parse(raw)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrOAuthCallbackInvalidRedirectURI, err)
	}
	if parsed.Scheme != "http" {
		return nil, fmt.Errorf("%w: redirect uri must use http", ErrOAuthCallbackInvalidRedirectURI)
	}
	hostname := parsed.Hostname()
	if hostname != "127.0.0.1" && hostname != "localhost" {
		return nil, fmt.Errorf("%w: redirect uri host must be localhost or 127.0.0.1", ErrOAuthCallbackInvalidRedirectURI)
	}
	if parsed.Port() == "" {
		return nil, fmt.Errorf("%w: redirect uri port is required", ErrOAuthCallbackInvalidRedirectURI)
	}
	if parsed.Path == "" {
		parsed.Path = "/"
	}
	return parsed, nil
}
