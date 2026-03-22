package linkedin

import (
	"errors"
	"fmt"
	"net/url"
	"strings"
)

type URN string

func ParseURN(raw string) (URN, string, string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", "", "", errors.New("urn is required")
	}
	parts := strings.Split(raw, ":")
	if len(parts) < 4 || parts[0] != "urn" || parts[1] != "li" {
		return "", "", "", fmt.Errorf("invalid linkedin urn %q", raw)
	}
	return URN(raw), parts[2], strings.Join(parts[3:], ":"), nil
}

func URNOf(kind string, id string) (URN, error) {
	kind = strings.TrimSpace(kind)
	id = strings.TrimSpace(id)
	if kind == "" {
		return "", errors.New("urn kind is required")
	}
	if id == "" {
		return "", errors.New("urn id is required")
	}
	if strings.Contains(id, ":") {
		return "", fmt.Errorf("urn id must not contain colon: %q", id)
	}
	return URN(fmt.Sprintf("urn:li:%s:%s", kind, id)), nil
}

func SponsoredAccountURN(id string) (URN, error) {
	return URNOf("sponsoredAccount", id)
}

func SponsoredCampaignGroupURN(id string) (URN, error) {
	return URNOf("adCampaignGroup", id)
}

func SponsoredCampaignURN(id string) (URN, error) {
	return URNOf("adCampaign", id)
}

func SponsoredCreativeURN(id string) (URN, error) {
	return URNOf("creative", id)
}

func OrganizationURN(id string) (URN, error) {
	return URNOf("organization", id)
}

func PersonURN(id string) (URN, error) {
	return URNOf("person", id)
}

func NormalizeURN(raw string, kind string) (URN, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", errors.New("urn is required")
	}
	if strings.HasPrefix(raw, "urn:li:") {
		urn, parsedKind, _, err := ParseURN(raw)
		if err != nil {
			return "", err
		}
		if kind != "" && parsedKind != kind {
			return "", fmt.Errorf("unexpected urn kind %q, want %q", parsedKind, kind)
		}
		return urn, nil
	}
	return URNOf(kind, raw)
}

func NormalizeSponsoredAccountURN(raw string) (URN, error) {
	return NormalizeURN(raw, "sponsoredAccount")
}

func NormalizeSponsoredCampaignGroupURN(raw string) (URN, error) {
	return NormalizeURN(raw, "adCampaignGroup")
}

func NormalizeSponsoredCampaignURN(raw string) (URN, error) {
	return NormalizeURN(raw, "adCampaign")
}

func NormalizeSponsoredCreativeURN(raw string) (URN, error) {
	return NormalizeURN(raw, "creative")
}

func NormalizeOrganizationURN(raw string) (URN, error) {
	return NormalizeURN(raw, "organization")
}

func NormalizePersonURN(raw string) (URN, error) {
	return NormalizeURN(raw, "person")
}

func BuildURN(kind string, id string) (URN, error) {
	return URNOf(kind, id)
}

func ValidateURN(raw string, kind string) error {
	_, parsedKind, _, err := ParseURN(raw)
	if err != nil {
		return err
	}
	if strings.TrimSpace(kind) != "" && parsedKind != strings.TrimSpace(kind) {
		return fmt.Errorf("unexpected urn kind %q, want %q", parsedKind, kind)
	}
	return nil
}

func EncodeURN(raw string) (string, error) {
	if err := ValidateURN(raw, ""); err != nil {
		return "", err
	}
	return url.PathEscape(strings.TrimSpace(raw)), nil
}

func (u URN) String() string {
	return string(u)
}
