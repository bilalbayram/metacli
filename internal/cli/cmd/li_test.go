package cmd

import "testing"

func TestNewLICommandIncludesPublicNamespaces(t *testing.T) {
	cmd := NewLICommand(Runtime{})

	for _, name := range []string{
		"auth",
		"api",
		"account",
		"organization",
		"campaign-group",
		"campaign",
		"creative",
		"insights",
		"targeting",
		"lead-form",
		"lead",
	} {
		sub, _, err := cmd.Find([]string{name})
		if err != nil {
			t.Fatalf("find %s command: %v", name, err)
		}
		if sub == nil || sub.Name() != name {
			t.Fatalf("expected %s command, got %#v", name, sub)
		}
	}
}

func TestNewLIInsightsCommandIncludesNestedNamespaces(t *testing.T) {
	cmd := NewLICommand(Runtime{})

	for _, args := range [][]string{
		{"insights", "metrics", "list"},
		{"insights", "pivots", "list"},
		{"lead", "webhook", "list"},
	} {
		sub, _, err := cmd.Find(args)
		if err != nil {
			t.Fatalf("find %v: %v", args, err)
		}
		if sub == nil {
			t.Fatalf("expected command for %v", args)
		}
	}
}
