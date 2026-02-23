package lint

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/bilalbayram/metacli/internal/schema"
)

type RequestSpec struct {
	Method string            `json:"method"`
	Path   string            `json:"path"`
	Params map[string]string `json:"params,omitempty"`
	Fields []string          `json:"fields,omitempty"`
}

type Result struct {
	Errors   []string `json:"errors"`
	Warnings []string `json:"warnings"`
}

type Linter struct {
	pack *schema.Pack
}

func New(pack *schema.Pack) (*Linter, error) {
	if pack == nil {
		return nil, errors.New("schema pack is required")
	}
	return &Linter{pack: pack}, nil
}

func LoadRequestSpec(path string) (*RequestSpec, error) {
	if strings.TrimSpace(path) == "" {
		return nil, errors.New("request spec file path is required")
	}
	if filepath.Ext(path) != ".json" {
		return nil, fmt.Errorf("unsupported request spec format for %s: only .json is supported", path)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read request spec file %s: %w", path, err)
	}
	var spec RequestSpec
	if err := json.Unmarshal(data, &spec); err != nil {
		return nil, fmt.Errorf("decode request spec file %s: %w", path, err)
	}
	spec.Method = normalizeMethod(spec.Method)
	if strings.TrimSpace(spec.Path) == "" {
		return nil, errors.New("request spec path is required")
	}
	if spec.Params == nil {
		spec.Params = map[string]string{}
	}
	return &spec, nil
}

func (l *Linter) Lint(spec *RequestSpec, strict bool) Result {
	result := Result{
		Errors:   []string{},
		Warnings: []string{},
	}
	if spec == nil {
		result.Errors = append(result.Errors, "request spec is required")
		return result
	}

	method := normalizeMethod(spec.Method)
	endpoint := detectEndpoint(spec.Path, method)
	entity := detectEntity(spec.Path, endpoint)

	allowedParams := toSet(l.pack.EndpointParams[endpoint])
	deprecatedParams := toSet(l.pack.DeprecatedParams[endpoint])
	if strict && isMutationMethod(method) && endpoint != "generic" && len(allowedParams) == 0 && len(deprecatedParams) == 0 {
		result.Errors = append(result.Errors, fmt.Sprintf("schema pack has no mutation param definitions for endpoint %q", endpoint))
	}
	for key := range spec.Params {
		if _, deprecated := deprecatedParams[key]; deprecated {
			result.Errors = append(result.Errors, fmt.Sprintf("deprecated param %q is not allowed for endpoint %q", key, endpoint))
			continue
		}
		if len(allowedParams) > 0 {
			if _, ok := allowedParams[key]; !ok {
				message := fmt.Sprintf("unknown param %q for endpoint %q", key, endpoint)
				if strict {
					result.Errors = append(result.Errors, message)
				} else {
					result.Warnings = append(result.Warnings, message)
				}
			}
		}
	}

	allowedFields := toSet(l.pack.Entities[entity])
	for _, field := range spec.Fields {
		field = strings.TrimSpace(field)
		if field == "" {
			continue
		}
		if len(allowedFields) > 0 {
			if _, ok := allowedFields[field]; !ok {
				message := fmt.Sprintf("unknown field %q for entity %q", field, entity)
				if strict {
					result.Errors = append(result.Errors, message)
				} else {
					result.Warnings = append(result.Warnings, message)
				}
			}
		}
	}
	return result
}

func detectEndpoint(path string, method string) string {
	base := detectBaseEndpoint(path)
	if base == "generic" {
		return base
	}
	if isMutationMethod(method) {
		return fmt.Sprintf("%s.%s", base, strings.ToLower(method))
	}
	return base
}

func detectBaseEndpoint(path string) string {
	path = strings.ToLower(path)
	switch {
	case strings.Contains(path, "insights"):
		return "insights"
	case strings.Contains(path, "adcreative"), strings.Contains(path, "creative"):
		return "adcreatives"
	case strings.Contains(path, "customaudience"), strings.Contains(path, "audience"):
		return "customaudiences"
	case strings.Contains(path, "product_catalog"), strings.Contains(path, "catalog"):
		return "product_catalogs"
	case strings.Contains(path, "adset"):
		return "adsets"
	case strings.Contains(path, "ads"):
		return "ads"
	case strings.Contains(path, "campaign"):
		return "campaigns"
	default:
		return "generic"
	}
}

func detectEntity(path string, endpoint string) string {
	path = strings.ToLower(path)
	if endpoint == "insights" {
		return "insights"
	}
	switch {
	case strings.Contains(path, "campaign"):
		return "campaign"
	case strings.Contains(path, "adset"):
		return "adset"
	case strings.Contains(path, "adcreative"), strings.Contains(path, "creative"):
		return "creative"
	case strings.Contains(path, "customaudience"), strings.Contains(path, "audience"):
		return "audience"
	case strings.Contains(path, "product_catalog"), strings.Contains(path, "catalog"):
		return "catalog"
	case strings.Contains(path, "ads"):
		return "ad"
	default:
		return "generic"
	}
}

func normalizeMethod(method string) string {
	method = strings.ToUpper(strings.TrimSpace(method))
	if method == "" {
		return "GET"
	}
	return method
}

func isMutationMethod(method string) bool {
	switch normalizeMethod(method) {
	case "POST", "PUT", "PATCH", "DELETE":
		return true
	default:
		return false
	}
}

func toSet(values []string) map[string]struct{} {
	set := map[string]struct{}{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		set[value] = struct{}{}
	}
	return set
}
