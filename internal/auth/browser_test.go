package auth

import (
	"errors"
	"testing"
)

type recordingCommandRunner struct {
	name string
	args []string
	err  error
}

func (r *recordingCommandRunner) Run(name string, args ...string) error {
	r.name = name
	r.args = append([]string{}, args...)
	return r.err
}

func TestBrowserOpenerDarwinUsesOpen(t *testing.T) {
	t.Parallel()

	runner := &recordingCommandRunner{}
	opener := NewBrowserOpenerForOS("darwin", runner)

	if err := opener.Open("https://example.com"); err != nil {
		t.Fatalf("open browser: %v", err)
	}
	if runner.name != "open" {
		t.Fatalf("unexpected command: got=%s want=open", runner.name)
	}
	if len(runner.args) != 1 || runner.args[0] != "https://example.com" {
		t.Fatalf("unexpected command args: %#v", runner.args)
	}
}

func TestBrowserOpenerLinuxUsesXDGOpen(t *testing.T) {
	t.Parallel()

	runner := &recordingCommandRunner{}
	opener := NewBrowserOpenerForOS("linux", runner)

	if err := opener.Open("https://example.com"); err != nil {
		t.Fatalf("open browser: %v", err)
	}
	if runner.name != "xdg-open" {
		t.Fatalf("unexpected command: got=%s want=xdg-open", runner.name)
	}
	if len(runner.args) != 1 || runner.args[0] != "https://example.com" {
		t.Fatalf("unexpected command args: %#v", runner.args)
	}
}

func TestBrowserOpenerRejectsUnsupportedOS(t *testing.T) {
	t.Parallel()

	runner := &recordingCommandRunner{}
	opener := NewBrowserOpenerForOS("windows", runner)

	err := opener.Open("https://example.com")
	if !errors.Is(err, ErrBrowserUnsupportedOS) {
		t.Fatalf("expected ErrBrowserUnsupportedOS, got: %v", err)
	}
}

func TestBrowserOpenerRejectsEmptyURL(t *testing.T) {
	t.Parallel()

	opener := NewBrowserOpenerForOS("darwin", &recordingCommandRunner{})
	err := opener.Open("")
	if !errors.Is(err, ErrBrowserURLRequired) {
		t.Fatalf("expected ErrBrowserURLRequired, got: %v", err)
	}
}

func TestBrowserOpenerRejectsInvalidURL(t *testing.T) {
	t.Parallel()

	opener := NewBrowserOpenerForOS("darwin", &recordingCommandRunner{})
	err := opener.Open("://bad")
	if !errors.Is(err, ErrBrowserInvalidURL) {
		t.Fatalf("expected ErrBrowserInvalidURL, got: %v", err)
	}
}

func TestBrowserOpenerCommandFailureIsDeterministic(t *testing.T) {
	t.Parallel()

	runner := &recordingCommandRunner{err: errors.New("boom")}
	opener := NewBrowserOpenerForOS("darwin", runner)

	err := opener.Open("https://example.com")
	if !errors.Is(err, ErrBrowserOpenFailed) {
		t.Fatalf("expected ErrBrowserOpenFailed, got: %v", err)
	}
}
