package ops

import (
	"fmt"
	"strings"
)

const (
	OptionalModulePolicyStrict = "strict"
	OptionalModulePolicySkip   = "skip"
)

func NormalizeOptionalModulePolicy(policy string) string {
	switch strings.ToLower(strings.TrimSpace(policy)) {
	case "", OptionalModulePolicySkip:
		return OptionalModulePolicySkip
	case OptionalModulePolicyStrict:
		return OptionalModulePolicyStrict
	default:
		return ""
	}
}

func ValidateOptionalModulePolicy(policy string) error {
	if NormalizeOptionalModulePolicy(policy) == "" {
		return fmt.Errorf("optional module policy must be one of [strict skip], got %q", policy)
	}
	return nil
}
