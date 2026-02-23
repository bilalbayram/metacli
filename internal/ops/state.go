package ops

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

const (
	StateSchemaVersion = 1
	BaselineVersion    = 1
)

const baselineStatusInitialized = "initialized"

var (
	ErrStatePathRequired    = errors.New("state path is required")
	ErrBaselineAlreadyExist = errors.New("baseline state already exists")
)

type BaselineState struct {
	SchemaVersion   int    `json:"schema_version"`
	BaselineVersion int    `json:"baseline_version"`
	Status          string `json:"status"`
}

func NewBaselineState() BaselineState {
	return BaselineState{
		SchemaVersion:   StateSchemaVersion,
		BaselineVersion: BaselineVersion,
		Status:          baselineStatusInitialized,
	}
}

func DefaultStatePath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve user home directory: %w", err)
	}
	return filepath.Join(home, ".meta", "ops", "baseline-state.json"), nil
}

func InitBaseline(path string) (BaselineState, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return BaselineState{}, ErrStatePathRequired
	}

	if _, err := os.Stat(path); err == nil {
		return BaselineState{}, fmt.Errorf("%w at %s", ErrBaselineAlreadyExist, path)
	} else if !errors.Is(err, os.ErrNotExist) {
		return BaselineState{}, fmt.Errorf("stat baseline state %s: %w", path, err)
	}

	state := NewBaselineState()
	if err := SaveBaseline(path, state); err != nil {
		return BaselineState{}, err
	}
	return state, nil
}

func SaveBaseline(path string, state BaselineState) error {
	path = strings.TrimSpace(path)
	if path == "" {
		return ErrStatePathRequired
	}
	if err := state.Validate(); err != nil {
		return err
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create baseline directory for %s: %w", path, err)
	}

	payload, err := marshalState(state)
	if err != nil {
		return err
	}

	tmpFile, err := os.CreateTemp(dir, ".baseline-*.json")
	if err != nil {
		return fmt.Errorf("create temp baseline file: %w", err)
	}
	defer os.Remove(tmpFile.Name())

	if _, err := tmpFile.Write(payload); err != nil {
		tmpFile.Close()
		return fmt.Errorf("write temp baseline file: %w", err)
	}
	if err := tmpFile.Chmod(0o600); err != nil {
		tmpFile.Close()
		return fmt.Errorf("chmod temp baseline file: %w", err)
	}
	if err := tmpFile.Close(); err != nil {
		return fmt.Errorf("close temp baseline file: %w", err)
	}
	if err := os.Rename(tmpFile.Name(), path); err != nil {
		return fmt.Errorf("replace baseline state %s: %w", path, err)
	}
	return nil
}

func LoadBaseline(path string) (BaselineState, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return BaselineState{}, ErrStatePathRequired
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return BaselineState{}, fmt.Errorf("%w: baseline state does not exist at %s", os.ErrNotExist, path)
		}
		return BaselineState{}, fmt.Errorf("read baseline state %s: %w", path, err)
	}

	var state BaselineState
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&state); err != nil {
		return BaselineState{}, fmt.Errorf("decode baseline state %s: %w", path, err)
	}
	var trailing struct{}
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return BaselineState{}, fmt.Errorf("decode baseline state %s: multiple JSON values", path)
		}
		return BaselineState{}, fmt.Errorf("decode baseline state %s: %w", path, err)
	}
	if err := state.Validate(); err != nil {
		return BaselineState{}, err
	}
	return state, nil
}

func (s BaselineState) Validate() error {
	if s.SchemaVersion != StateSchemaVersion {
		return fmt.Errorf("unsupported baseline schema_version=%d (expected %d)", s.SchemaVersion, StateSchemaVersion)
	}
	if s.BaselineVersion != BaselineVersion {
		return fmt.Errorf("unsupported baseline baseline_version=%d (expected %d)", s.BaselineVersion, BaselineVersion)
	}
	if s.Status != baselineStatusInitialized {
		return fmt.Errorf("baseline status must be %q", baselineStatusInitialized)
	}
	return nil
}

func marshalState(state BaselineState) ([]byte, error) {
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal baseline state: %w", err)
	}
	return append(data, '\n'), nil
}
