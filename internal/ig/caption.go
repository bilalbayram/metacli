package ig

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

const (
	MaxCaptionCharacters     = 2200
	CaptionWarningCharacters = 2000
	MaxCaptionHashtags       = 30
	CaptionWarningHashtags   = 25
)

type CaptionValidationResult struct {
	Caption        string   `json:"caption"`
	CharacterCount int      `json:"character_count"`
	HashtagCount   int      `json:"hashtag_count"`
	Strict         bool     `json:"strict"`
	Valid          bool     `json:"valid"`
	Errors         []string `json:"errors"`
	Warnings       []string `json:"warnings"`
}

func ValidateCaption(caption string, strict bool) CaptionValidationResult {
	trimmed := strings.TrimSpace(caption)
	result := CaptionValidationResult{
		Caption:        caption,
		CharacterCount: utf8.RuneCountInString(caption),
		HashtagCount:   countPrefixedTokens(caption, "#"),
		Strict:         strict,
		Errors:         make([]string, 0, 4),
		Warnings:       make([]string, 0, 4),
	}

	if trimmed == "" {
		result.Errors = append(result.Errors, "caption is required")
	}
	if result.CharacterCount > MaxCaptionCharacters {
		result.Errors = append(result.Errors, fmt.Sprintf("caption exceeds %d characters (%d)", MaxCaptionCharacters, result.CharacterCount))
	}
	if result.HashtagCount > MaxCaptionHashtags {
		result.Errors = append(result.Errors, fmt.Sprintf("caption exceeds %d hashtags (%d)", MaxCaptionHashtags, result.HashtagCount))
	}

	if result.CharacterCount > CaptionWarningCharacters && result.CharacterCount <= MaxCaptionCharacters {
		result.Warnings = append(result.Warnings, fmt.Sprintf("caption is near limit (%d/%d)", result.CharacterCount, MaxCaptionCharacters))
	}
	if result.HashtagCount > CaptionWarningHashtags && result.HashtagCount <= MaxCaptionHashtags {
		result.Warnings = append(result.Warnings, fmt.Sprintf("caption uses many hashtags (%d/%d)", result.HashtagCount, MaxCaptionHashtags))
	}

	if strict && len(result.Warnings) > 0 {
		for _, warning := range result.Warnings {
			result.Errors = append(result.Errors, fmt.Sprintf("strict mode: %s", warning))
		}
		result.Warnings = []string{}
	}

	result.Valid = len(result.Errors) == 0
	return result
}

func countPrefixedTokens(value string, prefix string) int {
	total := 0
	for _, token := range strings.Fields(value) {
		normalized := strings.TrimSpace(token)
		if strings.HasPrefix(normalized, prefix) && len(normalized) > len(prefix) {
			total++
		}
	}
	return total
}
