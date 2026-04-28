package buildinfo

import (
	"os"
	"strings"
)

const (
	defaultCommitValue            = "unknown"
	defaultVersionValue           = "unknown"
	defaultPHPMyAdminVersionValue = "unknown"
	maxShortCommitLength          = 12
)

type Info struct {
	AppVersion        string `json:"appVersion"`
	AppDisplayVersion string `json:"appDisplayVersion"`
	AppCommit         string `json:"appCommit"`
	AppShortCommit    string `json:"appShortCommit"`
	PHPMyAdminVersion string `json:"phpMyAdminVersion"`
}

func Current() Info {
	appVersion := normalizeVersion(firstNonEmptyEnv("BUILD_VERSION", "PMA_GATEWAY_BUILD_VERSION"))
	appCommit := normalizeCommit(firstNonEmptyEnv("BUILD_COMMIT", "PMA_GATEWAY_BUILD_COMMIT"))
	phpMyAdminVersion := normalizeVersion(os.Getenv("PMA_GATEWAY_PHPMYADMIN_VERSION"))

	return Info{
		AppVersion:        appVersion,
		AppDisplayVersion: displayVersion(appVersion, appCommit),
		AppCommit:         appCommit,
		AppShortCommit:    shortCommit(appCommit),
		PHPMyAdminVersion: phpMyAdminVersion,
	}
}

func firstNonEmptyEnv(keys ...string) string {
	for _, key := range keys {
		if value := os.Getenv(key); strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func normalizeVersion(value string) string {
	trimmed := strings.TrimSpace(value)
	trimmed = strings.TrimPrefix(trimmed, "v")
	if trimmed == "" {
		return defaultVersionValue
	}
	return trimmed
}

func normalizeCommit(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return defaultCommitValue
	}
	return trimmed
}

func shortCommit(value string) string {
	if value == "" || value == defaultCommitValue {
		return ""
	}
	if len(value) <= maxShortCommitLength {
		return value
	}
	return value[:maxShortCommitLength]
}

func displayVersion(version string, commit string) string {
	if version == "" || version == defaultVersionValue {
		return defaultVersionValue
	}
	display := "v" + version
	if short := shortCommit(commit); short != "" {
		display += "+" + short
	}
	return display
}
