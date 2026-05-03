package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/alnah/moth/internal/limits"
)

// ErrUnsupportedConfigField reports a config field outside the file schema.
var ErrUnsupportedConfigField = errors.New("unsupported config field")

// FileConfig contains non-secret settings loaded from an explicit JSON config file.
type FileConfig struct {
	Browser  BrowserConfig      `json:"browser"`
	Limits   limits.Options     `json:"limits"`
	Presence FileConfigPresence `json:"-"`
}

// BrowserConfig contains browser settings from the config file.
type BrowserConfig struct {
	Bin string `json:"bin"`
}

// FileConfigPresence reports fields explicitly present in the config file.
type FileConfigPresence struct {
	BrowserBin bool
	Limits     LimitsConfigPresence
}

// LimitsConfigPresence reports limit fields explicitly present in the config file.
type LimitsConfigPresence struct {
	Timeout    bool
	MaxResults bool
	MaxBytes   bool
	Retries    bool
	RetryBase  bool
	RetryMax   bool
}

// LoadFile loads non-secret settings from an explicit JSON config file path.
func LoadFile(path string) (FileConfig, error) {
	data, err := os.ReadFile(path) //nolint:gosec // Config path is an explicit user argument.
	if err != nil {
		return FileConfig{}, fmt.Errorf("read config file %q: %w", path, err)
	}

	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return FileConfig{}, fmt.Errorf("parse JSON config: %w", err)
	}
	if raw == nil {
		return FileConfig{}, fmt.Errorf("parse JSON config: root must be an object")
	}

	if err := rejectUnsupportedConfigFields(raw, map[string]struct{}{"browser": {}, "limits": {}}, ""); err != nil {
		return FileConfig{}, err
	}

	var fileConfig FileConfig
	if data, ok := raw["browser"]; ok {
		browser, present, err := parseFileBrowserOptions(data)
		if err != nil {
			return FileConfig{}, err
		}
		fileConfig.Browser = browser
		fileConfig.Presence.BrowserBin = present.BrowserBin
	}
	if data, ok := raw["limits"]; ok {
		limitOptions, present, err := parseFileLimits(data)
		if err != nil {
			return FileConfig{}, err
		}
		fileConfig.Limits = limitOptions
		fileConfig.Presence.Limits = present
	}

	return fileConfig, nil
}

func parseFileBrowserOptions(data json.RawMessage) (BrowserConfig, FileConfigPresence, error) {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return BrowserConfig{}, FileConfigPresence{}, fmt.Errorf("parse JSON config browser: %w", err)
	}
	if raw == nil {
		return BrowserConfig{}, FileConfigPresence{}, fmt.Errorf("parse JSON config browser: must be an object")
	}
	if err := rejectUnsupportedConfigFields(raw, map[string]struct{}{"bin": {}}, "browser"); err != nil {
		return BrowserConfig{}, FileConfigPresence{}, err
	}

	var browser BrowserConfig
	var present FileConfigPresence
	if data, ok := raw["bin"]; ok {
		present.BrowserBin = true
		if err := json.Unmarshal(data, &browser.Bin); err != nil {
			return BrowserConfig{}, FileConfigPresence{}, fmt.Errorf("parse JSON config browser.bin: %w", err)
		}
		if browser.Bin == "" {
			return BrowserConfig{}, FileConfigPresence{}, fmt.Errorf("validate config browser.bin: must not be empty")
		}
	}

	return browser, present, nil
}

func parseFileLimits(data json.RawMessage) (limits.Options, LimitsConfigPresence, error) {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return limits.Options{}, LimitsConfigPresence{}, fmt.Errorf("parse JSON config limits: %w", err)
	}
	if raw == nil {
		return limits.Options{}, LimitsConfigPresence{}, fmt.Errorf("parse JSON config limits: must be an object")
	}
	allowed := map[string]struct{}{
		"timeout":     {},
		"max_results": {},
		"max_bytes":   {},
		"retries":     {},
		"retry_base":  {},
		"retry_max":   {},
	}
	if err := rejectUnsupportedConfigFields(raw, allowed, "limits"); err != nil {
		return limits.Options{}, LimitsConfigPresence{}, err
	}

	return parseFileLimitValues(raw)
}

func parseFileLimitValues(raw map[string]json.RawMessage) (limits.Options, LimitsConfigPresence, error) {
	var options limits.Options
	var present LimitsConfigPresence
	if err := parseDurationLimitValues(raw, &options, &present); err != nil {
		return limits.Options{}, LimitsConfigPresence{}, err
	}
	if err := parseNumericLimitValues(raw, &options, &present); err != nil {
		return limits.Options{}, LimitsConfigPresence{}, err
	}
	return options, present, nil
}

func parseDurationLimitValues(
	raw map[string]json.RawMessage,
	options *limits.Options,
	present *LimitsConfigPresence,
) error {
	var err error
	if data, ok := raw["timeout"]; ok {
		present.Timeout = true
		options.Timeout, err = parseConfigDuration(data, "limits.timeout")
		if err != nil {
			return err
		}
	}
	if data, ok := raw["retry_base"]; ok {
		present.RetryBase = true
		options.RetryBase, err = parseConfigDuration(data, "limits.retry_base")
		if err != nil {
			return err
		}
	}
	if data, ok := raw["retry_max"]; ok {
		present.RetryMax = true
		options.RetryMax, err = parseConfigDuration(data, "limits.retry_max")
		if err != nil {
			return err
		}
	}
	return nil
}

func parseNumericLimitValues(
	raw map[string]json.RawMessage,
	options *limits.Options,
	present *LimitsConfigPresence,
) error {
	var err error
	if data, ok := raw["max_results"]; ok {
		present.MaxResults = true
		options.MaxResults, err = parseNonNegativeConfigInt(data, "limits.max_results")
		if err != nil {
			return err
		}
	}
	if data, ok := raw["max_bytes"]; ok {
		present.MaxBytes = true
		options.MaxBytes, err = parseNonNegativeConfigInt64(data, "limits.max_bytes")
		if err != nil {
			return err
		}
	}
	if data, ok := raw["retries"]; ok {
		present.Retries = true
		options.Retries, err = parseNonNegativeConfigInt(data, "limits.retries")
		if err != nil {
			return err
		}
	}
	return nil
}

func rejectUnsupportedConfigFields(raw map[string]json.RawMessage, allowed map[string]struct{}, prefix string) error {
	for field := range raw {
		if _, ok := allowed[field]; ok {
			continue
		}
		return fmt.Errorf("%w %q", ErrUnsupportedConfigField, qualifiedConfigField(prefix, field))
	}
	return nil
}

func qualifiedConfigField(prefix string, field string) string {
	if prefix == "" {
		return field
	}
	return prefix + "." + field
}

func parseConfigDuration(data json.RawMessage, field string) (time.Duration, error) {
	var text string
	if err := json.Unmarshal(data, &text); err != nil {
		return 0, fmt.Errorf("parse JSON config %s: duration must be a string", field)
	}
	duration, err := time.ParseDuration(text)
	if err != nil {
		return 0, fmt.Errorf("validate config %s: %w", field, err)
	}
	if duration < 0 {
		return 0, fmt.Errorf("validate config %s: must be non-negative", field)
	}
	return duration, nil
}

func parseNonNegativeConfigInt(data json.RawMessage, field string) (int, error) {
	var value int
	if err := json.Unmarshal(data, &value); err != nil {
		return 0, fmt.Errorf("parse JSON config %s: %w", field, err)
	}
	if value < 0 {
		return 0, fmt.Errorf("validate config %s: must be non-negative", field)
	}
	return value, nil
}

func parseNonNegativeConfigInt64(data json.RawMessage, field string) (int64, error) {
	var value int64
	if err := json.Unmarshal(data, &value); err != nil {
		return 0, fmt.Errorf("parse JSON config %s: %w", field, err)
	}
	if value < 0 {
		return 0, fmt.Errorf("validate config %s: must be non-negative", field)
	}
	return value, nil
}
