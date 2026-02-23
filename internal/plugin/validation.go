package plugin

import (
	"fmt"
	"regexp"
	"strings"
)

var nameTokenPattern = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)

func validateNameToken(label string, value string) error {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return fmt.Errorf("%s is required", label)
	}
	if !nameTokenPattern.MatchString(trimmed) {
		return fmt.Errorf("invalid %s %q: expected lowercase token matching %s", label, value, nameTokenPattern.String())
	}
	return nil
}
