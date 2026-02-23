package ops

import (
	"path/filepath"
	"testing"

	"github.com/bilalbayram/metacli/internal/auth"
)

func TestEvaluatePermissionPolicyPreflightSkipsWhenDisabled(t *testing.T) {
	t.Parallel()

	check := evaluatePermissionPolicyPreflight(PermissionPreflightSnapshot{})
	if check.Name != checkNamePermissionPolicyPreflight {
		t.Fatalf("unexpected check name: %s", check.Name)
	}
	if check.Status != CheckStatusPass {
		t.Fatalf("unexpected status: %s", check.Status)
	}
	if check.Blocking {
		t.Fatal("expected non-blocking skip check")
	}
}

func TestEvaluatePermissionPolicyPreflightFailsOnLoadError(t *testing.T) {
	t.Parallel()

	check := evaluatePermissionPolicyPreflight(PermissionPreflightSnapshot{
		Enabled:     true,
		ProfileName: "prod",
		LoadError:   "config file missing",
	})
	if check.Status != CheckStatusFail {
		t.Fatalf("unexpected status: %s", check.Status)
	}
	if !check.Blocking {
		t.Fatal("expected blocking failure on load error")
	}
}

func TestEvaluatePermissionPolicyPreflightFailsOnMissingPolicyFields(t *testing.T) {
	t.Parallel()

	check := evaluatePermissionPolicyPreflight(PermissionPreflightSnapshot{
		Enabled:      true,
		ProfileName:  "prod",
		Domain:       "marketing",
		GraphVersion: "v25.0",
		TokenType:    auth.TokenTypeSystemUser,
		TokenRef:     "invalid",
	})
	if check.Status != CheckStatusFail {
		t.Fatalf("unexpected status: %s", check.Status)
	}
	if !check.Blocking {
		t.Fatal("expected blocking failure on missing fields")
	}
}

func TestEvaluatePermissionPolicyPreflightPassesForValidSystemUserProfile(t *testing.T) {
	t.Parallel()

	check := evaluatePermissionPolicyPreflight(PermissionPreflightSnapshot{
		Enabled:      true,
		ProfileName:  "prod",
		Domain:       "marketing",
		GraphVersion: "v25.0",
		TokenType:    auth.TokenTypeSystemUser,
		BusinessID:   "biz_123",
		AppID:        "app_123",
		TokenRef:     "keychain://meta-marketing-cli/prod/token",
	})
	if check.Status != CheckStatusPass {
		t.Fatalf("unexpected status: %s", check.Status)
	}
	if check.Blocking {
		t.Fatal("expected non-blocking pass check")
	}
}

func TestRunWithOptionsIncludesPreflightFailure(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "baseline-state.json")
	if _, err := InitBaseline(path); err != nil {
		t.Fatalf("init baseline: %v", err)
	}

	result, err := RunWithOptions(path, RunOptions{
		PermissionPreflight: &PermissionPreflightSnapshot{
			Enabled:      true,
			ProfileName:  "prod",
			Domain:       "marketing",
			GraphVersion: "v25.0",
			TokenType:    auth.TokenTypeSystemUser,
			TokenRef:     "keychain://meta-marketing-cli/prod/token",
		},
	})
	if err != nil {
		t.Fatalf("run with preflight options: %v", err)
	}
	if result.Report.Checks[3].Name != checkNamePermissionPolicyPreflight {
		t.Fatalf("unexpected check name: %s", result.Report.Checks[3].Name)
	}
	if result.Report.Checks[3].Status != CheckStatusFail {
		t.Fatalf("unexpected check status: %s", result.Report.Checks[3].Status)
	}
	if result.Report.Summary.Blocking != 1 {
		t.Fatalf("unexpected summary: %+v", result.Report.Summary)
	}
}
