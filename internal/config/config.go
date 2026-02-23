package config

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

const (
	SchemaVersion       = 1
	DefaultGraphVersion = "v25.0"
	DefaultDomain       = "marketing"
)

type Profile struct {
	Domain        string `yaml:"domain"`
	GraphVersion  string `yaml:"graph_version"`
	TokenType     string `yaml:"token_type"`
	BusinessID    string `yaml:"business_id,omitempty"`
	AppID         string `yaml:"app_id,omitempty"`
	PageID        string `yaml:"page_id,omitempty"`
	SourceProfile string `yaml:"source_profile,omitempty"`
	TokenRef      string `yaml:"token_ref"`
	AppSecretRef  string `yaml:"app_secret_ref,omitempty"`
}

type Config struct {
	SchemaVersion  int                `yaml:"schema_version"`
	DefaultProfile string             `yaml:"default_profile,omitempty"`
	Profiles       map[string]Profile `yaml:"profiles"`
}

func DefaultPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve user home directory: %w", err)
	}
	return filepath.Join(home, ".meta", "config.yaml"), nil
}

func New() *Config {
	return &Config{
		SchemaVersion: SchemaVersion,
		Profiles:      map[string]Profile{},
	}
}

func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("%w: config file does not exist at %s", os.ErrNotExist, path)
		}
		return nil, fmt.Errorf("read config file %s: %w", path, err)
	}

	cfg := &Config{}
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)
	if err := decoder.Decode(cfg); err != nil {
		return nil, fmt.Errorf("decode config file %s: %w", path, err)
	}
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return cfg, nil
}

func LoadOrCreate(path string) (*Config, error) {
	cfg, err := Load(path)
	if err == nil {
		return cfg, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}

	cfg = New()
	if err := Save(path, cfg); err != nil {
		return nil, err
	}
	return cfg, nil
}

func Save(path string, cfg *Config) error {
	if cfg == nil {
		return errors.New("config is nil")
	}
	if err := cfg.Validate(); err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create config directory for %s: %w", path, err)
	}

	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}

	tmpFile, err := os.CreateTemp(filepath.Dir(path), ".config-*.yaml")
	if err != nil {
		return fmt.Errorf("create temp config file: %w", err)
	}
	defer os.Remove(tmpFile.Name())

	if _, err := tmpFile.Write(data); err != nil {
		tmpFile.Close()
		return fmt.Errorf("write temp config file: %w", err)
	}
	if err := tmpFile.Chmod(0o600); err != nil {
		tmpFile.Close()
		return fmt.Errorf("chmod temp config file: %w", err)
	}
	if err := tmpFile.Close(); err != nil {
		return fmt.Errorf("close temp config file: %w", err)
	}
	if err := os.Rename(tmpFile.Name(), path); err != nil {
		return fmt.Errorf("replace config file %s: %w", path, err)
	}
	return nil
}

func (c *Config) Validate() error {
	if c == nil {
		return errors.New("config is nil")
	}
	if c.SchemaVersion != SchemaVersion {
		return fmt.Errorf("unsupported config schema_version=%d (expected %d)", c.SchemaVersion, SchemaVersion)
	}
	if c.Profiles == nil {
		return errors.New("config profiles map is required")
	}
	for name, profile := range c.Profiles {
		if err := validateProfile(name, profile); err != nil {
			return err
		}
	}
	if c.DefaultProfile != "" {
		if _, ok := c.Profiles[c.DefaultProfile]; !ok {
			return fmt.Errorf("default_profile %q does not exist", c.DefaultProfile)
		}
	}
	return nil
}

func (c *Config) ResolveProfile(name string) (string, Profile, error) {
	if c == nil {
		return "", Profile{}, errors.New("config is nil")
	}
	if name == "" {
		name = c.DefaultProfile
	}
	if name == "" {
		return "", Profile{}, errors.New("profile is required and default_profile is not configured")
	}
	profile, ok := c.Profiles[name]
	if !ok {
		return "", Profile{}, fmt.Errorf("profile %q does not exist", name)
	}
	return name, profile, nil
}

func (c *Config) UpsertProfile(name string, profile Profile) error {
	if c == nil {
		return errors.New("config is nil")
	}
	if c.Profiles == nil {
		c.Profiles = map[string]Profile{}
	}
	profile = applyProfileDefaults(profile)
	if err := validateProfile(name, profile); err != nil {
		return err
	}

	c.Profiles[name] = profile
	if c.DefaultProfile == "" {
		c.DefaultProfile = name
	}
	return nil
}

func applyProfileDefaults(profile Profile) Profile {
	if profile.Domain == "" {
		profile.Domain = DefaultDomain
	}
	if profile.GraphVersion == "" {
		profile.GraphVersion = DefaultGraphVersion
	}
	return profile
}

func validateProfile(name string, profile Profile) error {
	if name == "" {
		return errors.New("profile name cannot be empty")
	}
	if profile.Domain == "" {
		return fmt.Errorf("profile %q domain is required", name)
	}
	if profile.GraphVersion == "" {
		return fmt.Errorf("profile %q graph_version is required", name)
	}
	if profile.TokenType == "" {
		return fmt.Errorf("profile %q token_type is required", name)
	}
	if profile.TokenRef == "" {
		return fmt.Errorf("profile %q token_ref is required", name)
	}
	return nil
}
