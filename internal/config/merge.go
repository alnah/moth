package config

import "github.com/alnah/moth/internal/limits"

// RuntimeConfig contains merged non-secret settings for one CLI execution.
type RuntimeConfig struct {
	BrowserBin string
	Limits     limits.Options
}

// FieldSet selects non-secret config fields for precedence decisions.
type FieldSet struct {
	BrowserBin bool
	Timeout    bool
	MaxResults bool
	MaxBytes   bool
	Retries    bool
	RetryBase  bool
	RetryMax   bool
}

// MergeFileConfig applies file values unless command flags already set them.
func MergeFileConfig(base RuntimeConfig, file FileConfig, flags FieldSet) (RuntimeConfig, FieldSet) {
	merged := base
	applied := FieldSet{}

	if file.Presence.BrowserBin && !flags.BrowserBin {
		merged.BrowserBin = file.Browser.Bin
		applied.BrowserBin = true
	}
	mergeLimitsConfig(&merged.Limits, file.Limits, file.Presence.Limits, flags, &applied)

	return merged, applied
}

func mergeLimitsConfig(
	base *limits.Options,
	file limits.Options,
	filePresence LimitsConfigPresence,
	flags FieldSet,
	applied *FieldSet,
) {
	if filePresence.Timeout && !flags.Timeout {
		base.Timeout = file.Timeout
		applied.Timeout = true
	}
	if filePresence.MaxResults && !flags.MaxResults {
		base.MaxResults = file.MaxResults
		applied.MaxResults = true
	}
	if filePresence.MaxBytes && !flags.MaxBytes {
		base.MaxBytes = file.MaxBytes
		applied.MaxBytes = true
	}
	if filePresence.Retries && !flags.Retries {
		base.Retries = file.Retries
		applied.Retries = true
	}
	if filePresence.RetryBase && !flags.RetryBase {
		base.RetryBase = file.RetryBase
		applied.RetryBase = true
	}
	if filePresence.RetryMax && !flags.RetryMax {
		base.RetryMax = file.RetryMax
		applied.RetryMax = true
	}
}
