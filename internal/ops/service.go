package ops

import (
	"errors"
	"fmt"
	"time"
)

const (
	CheckStatusPass = "pass"
	CheckStatusFail = "fail"
)

const (
	checkNameChangelogOCCDelta = "changelog_occ_delta"
	checkNameSchemaPackDrift   = "schema_pack_drift"
)

func Initialize(statePath string) (InitResult, error) {
	state, err := InitBaseline(statePath)
	if err != nil {
		switch {
		case errors.Is(err, ErrStatePathRequired):
			return InitResult{}, WrapExit(ExitCodeInput, err)
		case errors.Is(err, ErrBaselineAlreadyExist):
			return InitResult{}, WrapExit(ExitCodeInput, err)
		default:
			return InitResult{}, WrapExit(ExitCodeState, err)
		}
	}
	return InitResult{
		StatePath: statePath,
		State:     state,
	}, nil
}

func Run(statePath string) (RunResult, error) {
	state, err := LoadBaseline(statePath)
	if err != nil {
		switch {
		case errors.Is(err, ErrStatePathRequired):
			return RunResult{}, WrapExit(ExitCodeInput, err)
		default:
			return RunResult{}, WrapExit(ExitCodeState, err)
		}
	}

	report := NewReportSkeleton(state)
	currentSnapshot, err := captureChangelogOCCSnapshot(time.Now().UTC())
	if err != nil {
		return RunResult{}, WrapExit(ExitCodeState, err)
	}
	report.Checks = append(report.Checks, evaluateChangelogOCCDelta(state.Snapshots.ChangelogOCC, currentSnapshot))

	currentSchemaPack, err := captureSchemaPackSnapshot()
	if err != nil {
		return RunResult{}, WrapExit(ExitCodeState, err)
	}
	report.Checks = append(report.Checks, evaluateSchemaPackDrift(state.Snapshots.SchemaPack, currentSchemaPack))
	report.Summary = summarizeChecks(report.Checks)

	return RunResult{
		StatePath: statePath,
		Report:    report,
	}, nil
}

func RunExitCode(report Report) int {
	if report.Summary.Blocking > 0 {
		return ExitCodePolicy
	}
	return ExitCodeSuccess
}

func summarizeChecks(checks []Check) Summary {
	summary := Summary{
		Total: len(checks),
	}
	for _, check := range checks {
		if check.Status == CheckStatusPass {
			summary.Passed++
			continue
		}
		summary.Failed++
		if check.Blocking {
			summary.Blocking++
		}
	}
	return summary
}

func evaluateChangelogOCCDelta(baseline ChangelogOCCSnapshot, current ChangelogOCCSnapshot) Check {
	check := Check{
		Name:   checkNameChangelogOCCDelta,
		Status: CheckStatusPass,
		Message: fmt.Sprintf(
			"snapshot unchanged: latest_version=%s occ_digest=%s",
			current.LatestVersion,
			current.OCCDigest,
		),
	}
	if baseline.LatestVersion != current.LatestVersion || baseline.OCCDigest != current.OCCDigest {
		check.Status = CheckStatusFail
		check.Blocking = true
		check.Message = fmt.Sprintf(
			"snapshot drift detected: baseline latest_version=%s occ_digest=%s current latest_version=%s occ_digest=%s",
			baseline.LatestVersion,
			baseline.OCCDigest,
			current.LatestVersion,
			current.OCCDigest,
		)
	}
	return check
}

func evaluateSchemaPackDrift(baseline SchemaPackSnapshot, current SchemaPackSnapshot) Check {
	check := Check{
		Name:   checkNameSchemaPackDrift,
		Status: CheckStatusPass,
		Message: fmt.Sprintf(
			"schema pack unchanged: domain=%s version=%s sha256=%s",
			current.Domain,
			current.Version,
			current.SHA256,
		),
	}
	if baseline.Domain != current.Domain || baseline.Version != current.Version || baseline.SHA256 != current.SHA256 {
		check.Status = CheckStatusFail
		check.Blocking = true
		check.Message = fmt.Sprintf(
			"schema pack drift detected: baseline domain=%s version=%s sha256=%s current domain=%s version=%s sha256=%s",
			baseline.Domain,
			baseline.Version,
			baseline.SHA256,
			current.Domain,
			current.Version,
			current.SHA256,
		)
	}
	return check
}
