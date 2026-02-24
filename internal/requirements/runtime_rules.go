package requirements

import (
	"bytes"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const (
	DriftPolicyError   = "error"
	DriftPolicyWarning = "warning"
)

//go:embed rulepacks/*/*.json
var embeddedRulePacks embed.FS

type RulePack struct {
	Domain    string                  `json:"domain"`
	Version   string                  `json:"version"`
	Mutations map[string]MutationRule `json:"mutations"`
}

type MutationRule struct {
	AddRequired     []string            `json:"add_required,omitempty"`
	RemoveRequired  []string            `json:"remove_required,omitempty"`
	Forbidden       []string            `json:"forbidden_params,omitempty"`
	InjectDefaults  map[string]string   `json:"inject_defaults,omitempty"`
	RequiredScopes  []string            `json:"required_scopes,omitempty"`
	RequiredContext map[string][]string `json:"required_context,omitempty"`
	DriftPolicy     string              `json:"drift_policy,omitempty"`
}

func LoadRulePack(domain string, version string, rulesDir string) (*RulePack, error) {
	domain = strings.TrimSpace(domain)
	version = strings.TrimSpace(version)
	if domain == "" {
		return nil, errors.New("runtime rule pack domain is required")
	}
	if version == "" {
		return nil, errors.New("runtime rule pack version is required")
	}

	var (
		body []byte
		err  error
	)
	if strings.TrimSpace(rulesDir) != "" {
		path := filepath.Join(rulesDir, domain, version+".json")
		body, err = os.ReadFile(path)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				return nil, fmt.Errorf("runtime rule pack not found for domain=%s version=%s at %s", domain, version, path)
			}
			return nil, fmt.Errorf("read runtime rule pack %s: %w", path, err)
		}
	} else {
		embeddedPath := filepath.ToSlash(filepath.Join("rulepacks", domain, version+".json"))
		body, err = embeddedRulePacks.ReadFile(embeddedPath)
		if err != nil {
			return nil, fmt.Errorf("runtime rule pack not found for domain=%s version=%s in embedded rulepacks", domain, version)
		}
	}

	pack, err := decodeRulePack(body)
	if err != nil {
		return nil, err
	}
	if pack.Domain != domain || pack.Version != version {
		return nil, fmt.Errorf(
			"runtime rule pack identity mismatch: expected %s/%s got %s/%s",
			domain,
			version,
			pack.Domain,
			pack.Version,
		)
	}
	if err := pack.NormalizeAndValidate(); err != nil {
		return nil, err
	}
	return pack, nil
}

func decodeRulePack(body []byte) (*RulePack, error) {
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()

	var pack RulePack
	if err := decoder.Decode(&pack); err != nil {
		return nil, fmt.Errorf("decode runtime rule pack: %w", err)
	}
	var trailing struct{}
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return nil, errors.New("decode runtime rule pack: multiple JSON values")
		}
		return nil, fmt.Errorf("decode runtime rule pack: %w", err)
	}
	return &pack, nil
}

func (p *RulePack) NormalizeAndValidate() error {
	if p == nil {
		return errors.New("runtime rule pack is required")
	}
	p.Domain = strings.TrimSpace(p.Domain)
	p.Version = strings.TrimSpace(p.Version)
	if p.Domain == "" {
		return errors.New("runtime rule pack domain is required")
	}
	if p.Version == "" {
		return errors.New("runtime rule pack version is required")
	}
	if len(p.Mutations) == 0 {
		return errors.New("runtime rule pack mutations are required")
	}

	normalizedMutations := make(map[string]MutationRule, len(p.Mutations))
	for mutation, rule := range p.Mutations {
		trimmedMutation := strings.TrimSpace(mutation)
		if trimmedMutation == "" {
			return errors.New("runtime rule pack mutations contains blank mutation key")
		}
		rule.normalize()
		if err := rule.validate(trimmedMutation); err != nil {
			return err
		}
		normalizedMutations[trimmedMutation] = rule
	}
	p.Mutations = normalizedMutations
	return nil
}

func (r *MutationRule) normalize() {
	if r == nil {
		return
	}
	r.AddRequired = normalizeTokens(r.AddRequired)
	r.RemoveRequired = normalizeTokens(r.RemoveRequired)
	r.Forbidden = normalizeTokens(r.Forbidden)
	r.RequiredScopes = normalizeTokens(r.RequiredScopes)
	r.DriftPolicy = strings.ToLower(strings.TrimSpace(r.DriftPolicy))
	if r.DriftPolicy == "" {
		r.DriftPolicy = DriftPolicyError
	}

	if r.InjectDefaults == nil {
		r.InjectDefaults = map[string]string{}
	}
	normalizedInject := make(map[string]string, len(r.InjectDefaults))
	for key, value := range r.InjectDefaults {
		trimmedKey := strings.TrimSpace(key)
		if trimmedKey == "" {
			continue
		}
		normalizedInject[trimmedKey] = strings.TrimSpace(value)
	}
	r.InjectDefaults = normalizedInject

	if r.RequiredContext == nil {
		r.RequiredContext = map[string][]string{}
	}
	normalizedContext := make(map[string][]string, len(r.RequiredContext))
	for tokenType, fields := range r.RequiredContext {
		trimmedTokenType := strings.TrimSpace(tokenType)
		if trimmedTokenType == "" {
			continue
		}
		normalizedContext[trimmedTokenType] = normalizeTokens(fields)
	}
	r.RequiredContext = normalizedContext
}

func (r MutationRule) validate(mutation string) error {
	if strings.TrimSpace(mutation) == "" {
		return errors.New("runtime rule mutation key is required")
	}
	switch r.DriftPolicy {
	case DriftPolicyError, DriftPolicyWarning:
	default:
		return fmt.Errorf("runtime rule for mutation %q has invalid drift_policy %q", mutation, r.DriftPolicy)
	}
	for tokenType, fields := range r.RequiredContext {
		if strings.TrimSpace(tokenType) == "" {
			return fmt.Errorf("runtime rule for mutation %q has blank required_context token_type key", mutation)
		}
		if len(fields) == 0 {
			return fmt.Errorf("runtime rule for mutation %q has empty required_context field set for token_type %q", mutation, tokenType)
		}
	}
	return nil
}

func normalizeTokens(values []string) []string {
	if len(values) == 0 {
		return []string{}
	}
	set := make(map[string]struct{}, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		set[trimmed] = struct{}{}
	}
	normalized := make([]string, 0, len(set))
	for token := range set {
		normalized = append(normalized, token)
	}
	sort.Strings(normalized)
	return normalized
}
