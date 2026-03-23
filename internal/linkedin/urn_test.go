package linkedin

import "testing"

func TestURNHelpers(t *testing.T) {
	urn, err := SponsoredAccountURN("123")
	if err != nil {
		t.Fatalf("build urn: %v", err)
	}
	if urn.String() != "urn:li:sponsoredAccount:123" {
		t.Fatalf("unexpected urn %q", urn.String())
	}

	parsed, kind, id, err := ParseURN(urn.String())
	if err != nil {
		t.Fatalf("parse urn: %v", err)
	}
	if parsed != urn || kind != "sponsoredAccount" || id != "123" {
		t.Fatalf("unexpected parse result %#v kind=%q id=%q", parsed, kind, id)
	}

	normalized, err := NormalizeSponsoredAccountURN("urn:li:sponsoredAccount:123")
	if err != nil {
		t.Fatalf("normalize urn: %v", err)
	}
	if normalized != urn {
		t.Fatalf("unexpected normalized urn %q", normalized)
	}

	if _, err := NormalizeSponsoredCampaignURN(""); err == nil {
		t.Fatal("expected empty campaign urn to fail")
	}
}
