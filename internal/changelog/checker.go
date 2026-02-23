package changelog

import (
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
)

type VersionInfo struct {
	Version         string    `json:"version"`
	ReleaseDate     time.Time `json:"release_date"`
	DeprecationDate time.Time `json:"deprecation_date"`
}

type CheckResult struct {
	RequestedVersion   string `json:"requested_version"`
	LatestVersion      string `json:"latest_version"`
	IsLatest           bool   `json:"is_latest"`
	IsDeprecated       bool   `json:"is_deprecated"`
	DaysToDeprecation  int    `json:"days_to_deprecation"`
	NeedsUpgrade       bool   `json:"needs_upgrade"`
	Warning            string `json:"warning,omitempty"`
}

type Checker struct {
	versions map[string]VersionInfo
}

func NewChecker() *Checker {
	return &Checker{
		versions: map[string]VersionInfo{
			"v24.0": {
				Version:         "v24.0",
				ReleaseDate:     time.Date(2025, time.February, 1, 0, 0, 0, 0, time.UTC),
				DeprecationDate: time.Date(2026, time.July, 1, 0, 0, 0, 0, time.UTC),
			},
			"v25.0": {
				Version:         "v25.0",
				ReleaseDate:     time.Date(2025, time.May, 1, 0, 0, 0, 0, time.UTC),
				DeprecationDate: time.Date(2026, time.October, 1, 0, 0, 0, 0, time.UTC),
			},
		},
	}
}

func (c *Checker) Check(version string, now time.Time) (*CheckResult, error) {
	if c == nil {
		return nil, errors.New("checker is nil")
	}
	version = strings.TrimSpace(version)
	if version == "" {
		return nil, errors.New("version is required")
	}
	info, ok := c.versions[version]
	if !ok {
		return nil, fmt.Errorf("unknown version %q", version)
	}
	latest := c.latestVersion()
	days := int(info.DeprecationDate.Sub(now.UTC()).Hours() / 24)
	result := &CheckResult{
		RequestedVersion:  version,
		LatestVersion:     latest,
		IsLatest:          version == latest,
		IsDeprecated:      now.UTC().After(info.DeprecationDate),
		DaysToDeprecation: days,
		NeedsUpgrade:      version != latest || now.UTC().After(info.DeprecationDate),
	}
	if result.IsDeprecated {
		result.Warning = "requested version is already deprecated and must be upgraded immediately"
	} else if days <= 90 {
		result.Warning = "requested version is within 90 days of deprecation"
	}
	return result, nil
}

func (c *Checker) latestVersion() string {
	versions := make([]string, 0, len(c.versions))
	for version := range c.versions {
		versions = append(versions, version)
	}
	sort.Slice(versions, func(i, j int) bool {
		return compareVersion(versions[i], versions[j]) > 0
	})
	if len(versions) == 0 {
		return ""
	}
	return versions[0]
}

func compareVersion(left string, right string) int {
	leftMajor, leftMinor := parseVersion(left)
	rightMajor, rightMinor := parseVersion(right)
	if leftMajor > rightMajor {
		return 1
	}
	if leftMajor < rightMajor {
		return -1
	}
	if leftMinor > rightMinor {
		return 1
	}
	if leftMinor < rightMinor {
		return -1
	}
	return 0
}

func parseVersion(version string) (int, int) {
	version = strings.TrimPrefix(version, "v")
	parts := strings.Split(version, ".")
	if len(parts) != 2 {
		return 0, 0
	}
	major, _ := strconv.Atoi(parts[0])
	minor, _ := strconv.Atoi(parts[1])
	return major, minor
}
