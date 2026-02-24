package smoke

import (
	"errors"
	"fmt"
	"strings"
)

const (
	OptionalPolicyStrict = "strict"
	OptionalPolicySkip   = "skip"
)

var ErrInvalidOptionalPolicy = errors.New("invalid optional policy")

func NormalizeOptionalPolicy(policy string) string {
	switch strings.ToLower(strings.TrimSpace(policy)) {
	case OptionalPolicyStrict:
		return OptionalPolicyStrict
	case OptionalPolicySkip:
		return OptionalPolicySkip
	default:
		return ""
	}
}

func ValidateOptionalPolicy(policy string) error {
	if NormalizeOptionalPolicy(policy) == "" {
		return fmt.Errorf("%w: optional policy must be one of [%s %s], got %q", ErrInvalidOptionalPolicy, OptionalPolicyStrict, OptionalPolicySkip, policy)
	}
	return nil
}
