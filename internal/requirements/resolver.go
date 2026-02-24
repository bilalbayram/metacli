package requirements

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/bilalbayram/metacli/internal/schema"
)

const (
	SeverityError   = "error"
	SeverityWarning = "warning"
)

type ProfileContext struct {
	ProfileName  string   `json:"profile_name,omitempty"`
	Domain       string   `json:"domain,omitempty"`
	GraphVersion string   `json:"graph_version,omitempty"`
	TokenType    string   `json:"token_type,omitempty"`
	Scopes       []string `json:"scopes,omitempty"`
	BusinessID   string   `json:"business_id,omitempty"`
	AccountID    string   `json:"account_id,omitempty"`
	AppID        string   `json:"app_id,omitempty"`
}

type ResolveInput struct {
	Mutation string            `json:"mutation"`
	Payload  map[string]string `json:"payload"`
	Profile  ProfileContext    `json:"profile"`
}

type RequirementSet struct {
	Allowed        []string `json:"allowed"`
	Schema         []string `json:"schema"`
	RuntimeAdded   []string `json:"runtime_added"`
	RuntimeRemoved []string `json:"runtime_removed"`
	Final          []string `json:"final"`
}

type PayloadPlan struct {
	Input    map[string]string `json:"input"`
	Injected map[string]string `json:"injected"`
	Final    map[string]string `json:"final"`
}

type Violation struct {
	Code     string `json:"code"`
	Severity string `json:"severity"`
	Source   string `json:"source"`
	Field    string `json:"field,omitempty"`
	Message  string `json:"message"`
}

type Drift struct {
	Code     string `json:"code"`
	Severity string `json:"severity"`
	Source   string `json:"source"`
	Field    string `json:"field,omitempty"`
	Message  string `json:"message"`
}

type Resolution struct {
	Domain       string         `json:"domain"`
	Version      string         `json:"version"`
	Mutation     string         `json:"mutation"`
	Requirements RequirementSet `json:"requirements"`
	Payload      PayloadPlan    `json:"payload"`
	Profile      ProfileContext `json:"profile"`
	Violations   []Violation    `json:"violations"`
	Drift        []Drift        `json:"drift"`
	Blocking     bool           `json:"blocking"`
}

func (r Resolution) HasBlockingViolations() bool {
	if r.Blocking {
		return true
	}
	for _, violation := range r.Violations {
		if violation.Severity == SeverityError {
			return true
		}
	}
	for _, drift := range r.Drift {
		if drift.Severity == SeverityError {
			return true
		}
	}
	return false
}

func (r Resolution) ViolationSummary() string {
	if len(r.Violations) == 0 {
		return ""
	}
	messages := make([]string, 0, len(r.Violations))
	for _, violation := range r.Violations {
		messages = append(messages, violation.Message)
	}
	return strings.Join(messages, "; ")
}

type Resolver struct {
	schemaPack *schema.Pack
	rulePack   *RulePack
}

func NewResolver(schemaPack *schema.Pack, rulePack *RulePack) (*Resolver, error) {
	if schemaPack == nil {
		return nil, errors.New("schema pack is required")
	}
	if strings.TrimSpace(schemaPack.Domain) == "" || strings.TrimSpace(schemaPack.Version) == "" {
		return nil, errors.New("schema pack identity is required")
	}
	if rulePack == nil {
		return nil, errors.New("runtime rule pack is required")
	}
	if err := rulePack.NormalizeAndValidate(); err != nil {
		return nil, err
	}
	if schemaPack.Domain != rulePack.Domain || schemaPack.Version != rulePack.Version {
		return nil, fmt.Errorf(
			"resolver pack identity mismatch: schema=%s/%s rules=%s/%s",
			schemaPack.Domain,
			schemaPack.Version,
			rulePack.Domain,
			rulePack.Version,
		)
	}
	if schemaPack.EndpointRequiredParams == nil {
		schemaPack.EndpointRequiredParams = map[string][]string{}
	}
	return &Resolver{schemaPack: schemaPack, rulePack: rulePack}, nil
}

func (r *Resolver) Resolve(input ResolveInput) (Resolution, error) {
	if r == nil {
		return Resolution{}, errors.New("resolver is required")
	}
	mutation := strings.TrimSpace(input.Mutation)
	if mutation == "" {
		return Resolution{}, errors.New("mutation is required")
	}

	payload, err := normalizePayload(input.Payload)
	if err != nil {
		return Resolution{}, err
	}

	rule, exists := r.rulePack.Mutations[mutation]
	if !exists {
		return Resolution{}, fmt.Errorf("runtime rule pack has no mutation rule for %q", mutation)
	}

	allowed := normalizeTokens(r.schemaPack.EndpointParams[mutation])
	if len(allowed) == 0 {
		return Resolution{}, fmt.Errorf("schema pack has no endpoint params for mutation %q", mutation)
	}
	schemaRequired := normalizeTokens(r.schemaPack.EndpointRequiredParams[mutation])

	finalRequired := computeFinalRequired(schemaRequired, rule.AddRequired, rule.RemoveRequired)

	inputProfile := normalizeProfileContext(input.Profile)
	inputProfile.Domain = r.schemaPack.Domain
	inputProfile.GraphVersion = r.schemaPack.Version

	injected, finalPayload := applyPayloadDefaults(payload, rule.InjectDefaults)
	violations := make([]Violation, 0)
	deprecated := normalizeTokens(r.schemaPack.DeprecatedParams[mutation])
	drift := detectRuleSchemaDrift(allowed, schemaRequired, deprecated, rule)

	allowedSet := tokenSet(allowed)
	finalPayloadKeys := sortedMapKeys(finalPayload)
	for _, key := range finalPayloadKeys {
		if _, exists := allowedSet[key]; exists {
			continue
		}
		violations = append(violations, Violation{
			Code:     "unknown_param",
			Severity: SeverityError,
			Source:   "schema",
			Field:    key,
			Message:  fmt.Sprintf("unknown param %q for mutation %q", key, mutation),
		})
	}

	forbiddenSet := tokenSet(rule.Forbidden)
	for _, key := range finalPayloadKeys {
		if _, exists := forbiddenSet[key]; !exists {
			continue
		}
		violations = append(violations, Violation{
			Code:     "forbidden_param",
			Severity: SeverityError,
			Source:   "runtime_rule",
			Field:    key,
			Message:  fmt.Sprintf("runtime rules forbid param %q for mutation %q", key, mutation),
		})
	}

	for _, required := range finalRequired {
		value, exists := finalPayload[required]
		if exists && strings.TrimSpace(value) != "" {
			continue
		}
		violations = append(violations, Violation{
			Code:     "missing_required_param",
			Severity: SeverityError,
			Source:   "resolved_requirements",
			Field:    required,
			Message:  fmt.Sprintf("missing required param %q for mutation %q", required, mutation),
		})
	}

	profileScopes := tokenSet(inputProfile.Scopes)
	for _, scope := range rule.RequiredScopes {
		if _, exists := profileScopes[scope]; exists {
			continue
		}
		violations = append(violations, Violation{
			Code:     "missing_required_scope",
			Severity: SeverityError,
			Source:   "profile_context",
			Field:    scope,
			Message:  fmt.Sprintf("profile is missing required scope %q for mutation %q", scope, mutation),
		})
	}

	requiredContextFields := requiredContextForTokenType(rule.RequiredContext, inputProfile.TokenType)
	for _, field := range requiredContextFields {
		if strings.TrimSpace(contextFieldValue(inputProfile, field)) != "" {
			continue
		}
		violations = append(violations, Violation{
			Code:     "missing_required_context",
			Severity: SeverityError,
			Source:   "profile_context",
			Field:    field,
			Message:  fmt.Sprintf("profile context is missing required field %q for mutation %q", field, mutation),
		})
	}

	for _, driftItem := range drift {
		if driftItem.Severity != SeverityError {
			continue
		}
		violations = append(violations, Violation{
			Code:     "runtime_schema_drift",
			Severity: SeverityError,
			Source:   "runtime_rule",
			Field:    driftItem.Field,
			Message:  driftItem.Message,
		})
	}

	sortViolations(violations)
	sortDrift(drift)

	resolution := Resolution{
		Domain:   r.schemaPack.Domain,
		Version:  r.schemaPack.Version,
		Mutation: mutation,
		Requirements: RequirementSet{
			Allowed:        append([]string(nil), allowed...),
			Schema:         append([]string(nil), schemaRequired...),
			RuntimeAdded:   append([]string(nil), rule.AddRequired...),
			RuntimeRemoved: append([]string(nil), rule.RemoveRequired...),
			Final:          finalRequired,
		},
		Payload: PayloadPlan{
			Input:    payload,
			Injected: injected,
			Final:    finalPayload,
		},
		Profile:    inputProfile,
		Violations: violations,
		Drift:      drift,
	}
	resolution.Blocking = resolution.HasBlockingViolations()
	return resolution, nil
}

func normalizePayload(payload map[string]string) (map[string]string, error) {
	normalized := map[string]string{}
	for key, value := range payload {
		trimmedKey := strings.TrimSpace(key)
		if trimmedKey == "" {
			return nil, errors.New("payload param key cannot be empty")
		}
		normalized[trimmedKey] = strings.TrimSpace(value)
	}
	return normalized, nil
}

func normalizeProfileContext(profile ProfileContext) ProfileContext {
	profile.ProfileName = strings.TrimSpace(profile.ProfileName)
	profile.Domain = strings.TrimSpace(profile.Domain)
	profile.GraphVersion = strings.TrimSpace(profile.GraphVersion)
	profile.TokenType = strings.TrimSpace(profile.TokenType)
	profile.Scopes = normalizeTokens(profile.Scopes)
	profile.BusinessID = strings.TrimSpace(profile.BusinessID)
	profile.AccountID = strings.TrimSpace(profile.AccountID)
	profile.AppID = strings.TrimSpace(profile.AppID)
	return profile
}

func computeFinalRequired(schemaRequired []string, add []string, remove []string) []string {
	set := tokenSet(schemaRequired)
	for _, field := range add {
		set[field] = struct{}{}
	}
	for _, field := range remove {
		delete(set, field)
	}
	out := make([]string, 0, len(set))
	for field := range set {
		out = append(out, field)
	}
	sort.Strings(out)
	return out
}

func applyPayloadDefaults(payload map[string]string, defaults map[string]string) (map[string]string, map[string]string) {
	finalPayload := map[string]string{}
	for key, value := range payload {
		finalPayload[key] = value
	}
	injected := map[string]string{}
	for _, key := range sortedMapKeys(defaults) {
		if value, exists := finalPayload[key]; exists && strings.TrimSpace(value) != "" {
			continue
		}
		finalPayload[key] = defaults[key]
		injected[key] = defaults[key]
	}
	return injected, finalPayload
}

func detectRuleSchemaDrift(allowed []string, schemaRequired []string, deprecated []string, rule MutationRule) []Drift {
	allowedSet := tokenSet(allowed)
	schemaRequiredSet := tokenSet(schemaRequired)
	deprecatedSet := tokenSet(deprecated)
	forbiddenSet := tokenSet(rule.Forbidden)
	removeSet := tokenSet(rule.RemoveRequired)
	uniqueFields := map[string]struct{}{}
	for _, field := range rule.AddRequired {
		uniqueFields[field] = struct{}{}
	}
	for _, field := range rule.RemoveRequired {
		uniqueFields[field] = struct{}{}
	}
	for _, field := range rule.Forbidden {
		uniqueFields[field] = struct{}{}
	}
	for field := range rule.InjectDefaults {
		uniqueFields[field] = struct{}{}
	}

	severity := SeverityError
	if rule.DriftPolicy == DriftPolicyWarning {
		severity = SeverityWarning
	}

	drift := make([]Drift, 0)
	fields := make([]string, 0, len(uniqueFields))
	for field := range uniqueFields {
		fields = append(fields, field)
	}
	sort.Strings(fields)

	for _, field := range fields {
		if _, exists := allowedSet[field]; exists {
			if _, removing := removeSet[field]; removing {
				if _, required := schemaRequiredSet[field]; !required {
					drift = append(drift, Drift{
						Code:     "runtime_remove_missing_schema_requirement",
						Severity: SeverityWarning,
						Source:   "runtime_rule",
						Field:    field,
						Message:  fmt.Sprintf("runtime rule removes %q but schema has no static required entry", field),
					})
				}
			}
			continue
		}
		if _, forbidden := forbiddenSet[field]; forbidden {
			if _, deprecated := deprecatedSet[field]; deprecated {
				continue
			}
		}
		drift = append(drift, Drift{
			Code:     "runtime_param_not_in_schema",
			Severity: severity,
			Source:   "runtime_rule",
			Field:    field,
			Message:  fmt.Sprintf("runtime rule references param %q that is not present in schema endpoint params", field),
		})
	}

	return drift
}

func requiredContextForTokenType(required map[string][]string, tokenType string) []string {
	all := make([]string, 0)
	if global, exists := required["*"]; exists {
		all = append(all, global...)
	}
	if specific, exists := required[tokenType]; exists {
		all = append(all, specific...)
	}
	return normalizeTokens(all)
}

func contextFieldValue(profile ProfileContext, field string) string {
	switch strings.ToLower(strings.TrimSpace(field)) {
	case "profile_name":
		return profile.ProfileName
	case "domain":
		return profile.Domain
	case "graph_version":
		return profile.GraphVersion
	case "token_type":
		return profile.TokenType
	case "business_id":
		return profile.BusinessID
	case "account_id":
		return profile.AccountID
	case "app_id":
		return profile.AppID
	default:
		return ""
	}
}

func tokenSet(values []string) map[string]struct{} {
	set := make(map[string]struct{}, len(values))
	for _, value := range values {
		set[value] = struct{}{}
	}
	return set
}

func sortedMapKeys[T any](values map[string]T) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func sortViolations(violations []Violation) {
	sort.Slice(violations, func(i, j int) bool {
		if violations[i].Code != violations[j].Code {
			return violations[i].Code < violations[j].Code
		}
		if violations[i].Field != violations[j].Field {
			return violations[i].Field < violations[j].Field
		}
		if violations[i].Severity != violations[j].Severity {
			return violations[i].Severity < violations[j].Severity
		}
		return violations[i].Message < violations[j].Message
	})
}

func sortDrift(drift []Drift) {
	sort.Slice(drift, func(i, j int) bool {
		if drift[i].Code != drift[j].Code {
			return drift[i].Code < drift[j].Code
		}
		if drift[i].Field != drift[j].Field {
			return drift[i].Field < drift[j].Field
		}
		if drift[i].Severity != drift[j].Severity {
			return drift[i].Severity < drift[j].Severity
		}
		return drift[i].Message < drift[j].Message
	})
}
