package ops

import (
	"errors"
	"fmt"
	"time"

	"github.com/bilalbayram/metacli/internal/lint"
)

const (
	CheckStatusPass = "pass"
	CheckStatusFail = "fail"
)

const (
	checkNameChangelogOCCDelta         = "changelog_occ_delta"
	checkNameSchemaPackDrift           = "schema_pack_drift"
	checkNameRateLimitThreshold        = "rate_limit_threshold"
	checkNamePermissionPolicyPreflight = "permission_policy_preflight"
	checkNameRuntimeResponseShapeDrift = "runtime_response_shape_drift"
)

const (
	reportSectionMonitor   = "monitor"
	reportSectionDrift     = "drift"
	reportSectionRateLimit = "rate_limit"
	reportSectionPreflight = "preflight"
	reportSectionOther     = "other"
)

const (
	RunOutcomeClean    = "clean"
	RunOutcomeWarning  = "warning"
	RunOutcomeBlocking = "blocking"
	RunOutcomeError    = "error"
)

const (
	DefaultRateLimitWarningThreshold = 60
	DefaultRateLimitThreshold        = 75
)

type reportSectionDefinition struct {
	Name      string
	CheckName []string
}

var reportSectionDefinitions = []reportSectionDefinition{
	{Name: reportSectionMonitor, CheckName: []string{checkNameChangelogOCCDelta}},
	{Name: reportSectionDrift, CheckName: []string{checkNameSchemaPackDrift, checkNameRuntimeResponseShapeDrift}},
	{Name: reportSectionRateLimit, CheckName: []string{checkNameRateLimitThreshold}},
	{Name: reportSectionPreflight, CheckName: []string{checkNamePermissionPolicyPreflight}},
}

type RunOptions struct {
	RateLimitTelemetry  *RateLimitTelemetrySnapshot
	PermissionPreflight *PermissionPreflightSnapshot
	RuntimeResponse     *RuntimeResponseShapeSnapshot
	LintRequestSpec     *lint.RequestSpec
	LintRequestSpecFile string
}

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
	return RunWithOptions(statePath, RunOptions{})
}

func RunWithOptions(statePath string, options RunOptions) (RunResult, error) {
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
		return RunResult{}, WrapExit(ExitCodeRuntime, err)
	}
	report.Checks = append(report.Checks, evaluateChangelogOCCDelta(state.Snapshots.ChangelogOCC, currentSnapshot))

	currentSchemaPack, err := captureSchemaPackSnapshot()
	if err != nil {
		return RunResult{}, WrapExit(ExitCodeRuntime, err)
	}
	report.Checks = append(report.Checks, evaluateSchemaPackDrift(state.Snapshots.SchemaPack, currentSchemaPack))

	rateTelemetry := state.Snapshots.RateLimit
	if options.RateLimitTelemetry != nil {
		if err := options.RateLimitTelemetry.Validate(); err != nil {
			return RunResult{}, WrapExit(ExitCodeInput, err)
		}
		rateTelemetry = *options.RateLimitTelemetry
	}
	report.Checks = append(report.Checks, evaluateRateLimitThreshold(rateTelemetry, DefaultRateLimitWarningThreshold, DefaultRateLimitThreshold))

	preflightSnapshot := PermissionPreflightSnapshot{}
	if options.PermissionPreflight != nil {
		preflightSnapshot = *options.PermissionPreflight
	}
	report.Checks = append(report.Checks, evaluatePermissionPolicyPreflight(preflightSnapshot))

	if options.RuntimeResponse != nil {
		if err := options.RuntimeResponse.Validate(); err != nil {
			return RunResult{}, WrapExit(ExitCodeInput, err)
		}
	}
	if options.LintRequestSpec != nil {
		if err := validateLintRequestSpec(options.LintRequestSpec); err != nil {
			return RunResult{}, WrapExit(ExitCodeInput, err)
		}
	}
	if options.LintRequestSpec != nil && options.RuntimeResponse == nil {
		return RunResult{}, WrapExit(ExitCodeInput, errors.New("runtime response snapshot is required when lint request spec is provided"))
	}

	runtimeDriftCheck, err := evaluateRuntimeResponseShapeDrift(
		state.Snapshots.SchemaPack,
		options.RuntimeResponse,
		options.LintRequestSpec,
		options.LintRequestSpecFile,
	)
	if err != nil {
		return RunResult{}, WrapExit(ExitCodeRuntime, err)
	}
	report.Checks = append(report.Checks, runtimeDriftCheck)
	report.Summary = summarizeChecks(report.Checks)
	report.Outcome = summarizeOutcome(report.Summary)
	report.Sections = composeReportSections(report.Checks)

	return RunResult{
		StatePath: statePath,
		Report:    report,
	}, nil
}

func RunExitCode(report Report) int {
	switch RunOutcomeForReport(report) {
	case RunOutcomeBlocking:
		return ExitCodePolicy
	case RunOutcomeWarning:
		return ExitCodeWarning
	case RunOutcomeClean:
		return ExitCodeSuccess
	default:
		return ExitCodeUnknown
	}
}

func RunOutcomeForReport(report Report) string {
	if report.Outcome != "" {
		return report.Outcome
	}
	return summarizeOutcome(report.Summary)
}

func summarizeOutcome(summary Summary) string {
	if summary.Blocking > 0 {
		return RunOutcomeBlocking
	}
	if summary.Warnings > 0 {
		return RunOutcomeWarning
	}
	return RunOutcomeClean
}

func composeReportSections(checks []Check) []ReportSection {
	checkByName := make(map[string]Check, len(checks))
	for _, check := range checks {
		if _, exists := checkByName[check.Name]; exists {
			continue
		}
		checkByName[check.Name] = check
	}

	sections := make([]ReportSection, 0, len(reportSectionDefinitions)+1)
	assigned := make(map[string]struct{}, len(checkByName))

	for _, definition := range reportSectionDefinitions {
		sectionChecks := make([]Check, 0, len(definition.CheckName))
		for _, checkName := range definition.CheckName {
			check, ok := checkByName[checkName]
			if !ok {
				continue
			}
			sectionChecks = append(sectionChecks, check)
			assigned[checkName] = struct{}{}
		}

		summary := summarizeChecks(sectionChecks)
		sections = append(sections, ReportSection{
			Name:    definition.Name,
			Summary: summary,
			Outcome: summarizeOutcome(summary),
			Checks:  sectionChecks,
		})
	}

	uncategorized := make([]Check, 0)
	for _, check := range checks {
		if _, ok := assigned[check.Name]; ok {
			continue
		}
		uncategorized = append(uncategorized, check)
	}
	if len(uncategorized) > 0 {
		summary := summarizeChecks(uncategorized)
		sections = append(sections, ReportSection{
			Name:    reportSectionOther,
			Summary: summary,
			Outcome: summarizeOutcome(summary),
			Checks:  uncategorized,
		})
	}

	return sections
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
			continue
		}
		summary.Warnings++
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

func evaluateRateLimitThreshold(snapshot RateLimitTelemetrySnapshot, warningThreshold int, threshold int) Check {
	check := Check{
		Name:   checkNameRateLimitThreshold,
		Status: CheckStatusPass,
	}
	metric, value := highestRateLimitMetric(snapshot)
	check.Message = fmt.Sprintf(
		"rate limit within thresholds: max_metric=%s value=%d warning_threshold=%d threshold=%d",
		metric,
		value,
		warningThreshold,
		threshold,
	)
	if value >= threshold {
		check.Status = CheckStatusFail
		check.Blocking = true
		check.Message = fmt.Sprintf(
			"rate limit threshold exceeded: metric=%s value=%d threshold=%d",
			metric,
			value,
			threshold,
		)
		return check
	}
	if value >= warningThreshold {
		check.Status = CheckStatusFail
		check.Message = fmt.Sprintf(
			"rate limit warning threshold reached: metric=%s value=%d warning_threshold=%d threshold=%d",
			metric,
			value,
			warningThreshold,
			threshold,
		)
	}
	return check
}

func highestRateLimitMetric(snapshot RateLimitTelemetrySnapshot) (string, int) {
	metrics := []struct {
		name  string
		value int
	}{
		{name: "app_call_count", value: snapshot.AppCallCount},
		{name: "app_total_cputime", value: snapshot.AppTotalCPUTime},
		{name: "app_total_time", value: snapshot.AppTotalTime},
		{name: "page_call_count", value: snapshot.PageCallCount},
		{name: "page_total_cputime", value: snapshot.PageTotalCPUTime},
		{name: "page_total_time", value: snapshot.PageTotalTime},
		{name: "ad_account_util_pct", value: snapshot.AdAccountUtilPct},
	}

	maxMetric := metrics[0]
	for _, metric := range metrics[1:] {
		if metric.value > maxMetric.value {
			maxMetric = metric
		}
	}
	return maxMetric.name, maxMetric.value
}
