package ops

import (
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/bilalbayram/metacli/internal/lint"
	"github.com/bilalbayram/metacli/internal/schema"
)

type RuntimeResponseShapeSnapshot struct {
	Method         string   `json:"method"`
	Path           string   `json:"path"`
	ObservedFields []string `json:"observed_fields"`
}

func (s *RuntimeResponseShapeSnapshot) Validate() error {
	if s == nil {
		return errors.New("runtime response snapshot is required")
	}
	s.Method = normalizeHTTPMethod(s.Method)
	if strings.TrimSpace(s.Path) == "" {
		return errors.New("runtime response snapshot path is required")
	}
	fields := normalizeTokenList(s.ObservedFields)
	if len(fields) == 0 {
		return errors.New("runtime response snapshot observed_fields must include at least one field")
	}
	s.ObservedFields = fields
	return nil
}

func validateLintRequestSpec(spec *lint.RequestSpec) error {
	if spec == nil {
		return errors.New("lint request spec is required")
	}
	spec.Method = normalizeHTTPMethod(spec.Method)
	spec.Path = strings.TrimSpace(spec.Path)
	if spec.Path == "" {
		return errors.New("lint request spec path is required")
	}
	spec.Fields = normalizeTokenList(spec.Fields)
	if len(spec.Fields) == 0 {
		return errors.New("lint request spec fields are required for runtime drift linkage")
	}
	if spec.Params == nil {
		spec.Params = map[string]string{}
	}
	return nil
}

func evaluateRuntimeResponseShapeDrift(
	schemaSnapshot SchemaPackSnapshot,
	runtimeSnapshot *RuntimeResponseShapeSnapshot,
	lintRequestSpec *lint.RequestSpec,
	lintRequestSpecFile string,
) (Check, error) {
	check := Check{
		Name:   checkNameRuntimeResponseShapeDrift,
		Status: CheckStatusPass,
	}
	if runtimeSnapshot == nil {
		check.Message = "runtime response drift check skipped: runtime response snapshot not provided"
		return check, nil
	}

	schemaSourcePath, err := resolveSchemaPackSnapshotSource(schemaSnapshot.Domain, schemaSnapshot.Version)
	if err != nil {
		return Check{}, fmt.Errorf("resolve schema pack for runtime drift check: %w", err)
	}
	schemaRoot := filepath.Dir(filepath.Dir(schemaSourcePath))
	pack, err := schema.NewProvider(schemaRoot, "", "").GetPack(schemaSnapshot.Domain, schemaSnapshot.Version)
	if err != nil {
		return Check{}, fmt.Errorf("load schema pack for runtime drift check: %w", err)
	}
	linter, err := lint.New(pack)
	if err != nil {
		return Check{}, fmt.Errorf("initialize linter for runtime drift check: %w", err)
	}

	runtimeEndpoint, runtimeEntity := lint.ResolveRequestTarget(runtimeSnapshot.Path, runtimeSnapshot.Method)
	if runtimeEndpoint == "generic" || runtimeEntity == "generic" {
		check.Status = CheckStatusFail
		check.Blocking = true
		check.Message = fmt.Sprintf(
			"runtime drift detected: unable to resolve schema target for method=%s path=%s",
			runtimeSnapshot.Method,
			runtimeSnapshot.Path,
		)
		return check, nil
	}
	expectedFields := normalizeTokenList(pack.Entities[runtimeEntity])
	if len(expectedFields) == 0 {
		check.Status = CheckStatusFail
		check.Blocking = true
		check.Message = fmt.Sprintf(
			"runtime drift detected: schema pack has no field expectations for entity=%s (endpoint=%s)",
			runtimeEntity,
			runtimeEndpoint,
		)
		return check, nil
	}

	observedSpec := &lint.RequestSpec{
		Method: runtimeSnapshot.Method,
		Path:   runtimeSnapshot.Path,
		Fields: append([]string(nil), runtimeSnapshot.ObservedFields...),
	}
	observedLint := linter.Lint(observedSpec, true)
	if len(observedLint.Errors) > 0 {
		check.Status = CheckStatusFail
		check.Blocking = true
		check.Message = fmt.Sprintf(
			"runtime drift detected: observed response shape is outside schema expectations for endpoint=%s entity=%s: %s",
			runtimeEndpoint,
			runtimeEntity,
			strings.Join(observedLint.Errors, "; "),
		)
		return check, nil
	}

	observedSet := toFieldSet(runtimeSnapshot.ObservedFields)
	if lintRequestSpec != nil {
		lintEndpoint, lintEntity := lint.ResolveRequestTarget(lintRequestSpec.Path, lintRequestSpec.Method)
		if lintEndpoint != runtimeEndpoint || lintEntity != runtimeEntity {
			check.Status = CheckStatusFail
			check.Blocking = true
			check.Message = fmt.Sprintf(
				"runtime drift detected: runtime target endpoint=%s entity=%s does not match lint request target endpoint=%s entity=%s (%s)",
				runtimeEndpoint,
				runtimeEntity,
				lintEndpoint,
				lintEntity,
				describeLintReference(lintRequestSpec, lintRequestSpecFile),
			)
			return check, nil
		}

		lintResult := linter.Lint(lintRequestSpec, true)
		if len(lintResult.Errors) > 0 {
			check.Status = CheckStatusFail
			check.Blocking = true
			check.Message = fmt.Sprintf(
				"runtime drift detected: linked lint request spec is invalid (%s): %s",
				describeLintReference(lintRequestSpec, lintRequestSpecFile),
				strings.Join(lintResult.Errors, "; "),
			)
			return check, nil
		}

		missingFields := missingRequestedFields(lintRequestSpec.Fields, observedSet)
		if len(missingFields) > 0 {
			check.Status = CheckStatusFail
			check.Blocking = true
			check.Message = fmt.Sprintf(
				"runtime drift detected: observed response is missing requested fields %s (%s)",
				strings.Join(missingFields, ","),
				describeLintReference(lintRequestSpec, lintRequestSpecFile),
			)
			return check, nil
		}

		check.Message = fmt.Sprintf(
			"runtime response shape matches schema expectations: endpoint=%s entity=%s observed_fields=%d (%s)",
			runtimeEndpoint,
			runtimeEntity,
			len(observedSet),
			describeLintReference(lintRequestSpec, lintRequestSpecFile),
		)
		return check, nil
	}

	check.Message = fmt.Sprintf(
		"runtime response shape matches schema expectations: endpoint=%s entity=%s observed_fields=%d",
		runtimeEndpoint,
		runtimeEntity,
		len(observedSet),
	)
	return check, nil
}

func normalizeHTTPMethod(method string) string {
	method = strings.ToUpper(strings.TrimSpace(method))
	if method == "" {
		return "GET"
	}
	return method
}

func normalizeTokenList(values []string) []string {
	normalized := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		token := strings.TrimSpace(value)
		if token == "" {
			continue
		}
		if _, exists := seen[token]; exists {
			continue
		}
		seen[token] = struct{}{}
		normalized = append(normalized, token)
	}
	sort.Strings(normalized)
	return normalized
}

func toFieldSet(values []string) map[string]struct{} {
	set := make(map[string]struct{}, len(values))
	for _, value := range values {
		set[value] = struct{}{}
	}
	return set
}

func missingRequestedFields(requested []string, observed map[string]struct{}) []string {
	missing := make([]string, 0)
	for _, field := range requested {
		if _, exists := observed[field]; exists {
			continue
		}
		missing = append(missing, field)
	}
	sort.Strings(missing)
	return missing
}

func describeLintReference(spec *lint.RequestSpec, sourcePath string) string {
	location := "inline"
	if strings.TrimSpace(sourcePath) != "" {
		location = strings.TrimSpace(sourcePath)
	}
	return fmt.Sprintf("lint_request_spec=%s method=%s path=%s", location, spec.Method, spec.Path)
}
