package auth

import (
	"errors"
	"fmt"
	"net/url"
	"os/exec"
	"runtime"
	"strings"
)

var (
	ErrBrowserURLRequired   = errors.New("browser url is required")
	ErrBrowserInvalidURL    = errors.New("browser url is invalid")
	ErrBrowserUnsupportedOS = errors.New("browser opener is unsupported on this operating system")
	ErrBrowserOpenFailed    = errors.New("browser open command failed")
)

type BrowserOpener interface {
	Open(targetURL string) error
}

type browserCommandRunner interface {
	Run(name string, args ...string) error
}

type execBrowserCommandRunner struct{}

func (execBrowserCommandRunner) Run(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	return cmd.Run()
}

type ShellBrowserOpener struct {
	goos   string
	runner browserCommandRunner
}

func NewBrowserOpener() *ShellBrowserOpener {
	return NewBrowserOpenerForOS(runtime.GOOS, execBrowserCommandRunner{})
}

func NewBrowserOpenerForOS(goos string, runner browserCommandRunner) *ShellBrowserOpener {
	if runner == nil {
		runner = execBrowserCommandRunner{}
	}
	return &ShellBrowserOpener{
		goos:   strings.TrimSpace(goos),
		runner: runner,
	}
}

func OpenBrowser(targetURL string) error {
	return NewBrowserOpener().Open(targetURL)
}

func (o *ShellBrowserOpener) Open(targetURL string) error {
	targetURL = strings.TrimSpace(targetURL)
	if targetURL == "" {
		return ErrBrowserURLRequired
	}

	parsed, err := url.Parse(targetURL)
	if err != nil || parsed == nil || parsed.Scheme == "" || parsed.Host == "" {
		return fmt.Errorf("%w: %q", ErrBrowserInvalidURL, targetURL)
	}

	command, err := browserCommandForOS(o.goos)
	if err != nil {
		return err
	}

	if err := o.runner.Run(command, targetURL); err != nil {
		return fmt.Errorf("%w: %s %q: %v", ErrBrowserOpenFailed, command, targetURL, err)
	}
	return nil
}

func browserCommandForOS(goos string) (string, error) {
	switch goos {
	case "darwin":
		return "open", nil
	case "linux":
		return "xdg-open", nil
	default:
		return "", fmt.Errorf("%w: %q", ErrBrowserUnsupportedOS, goos)
	}
}
