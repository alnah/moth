package config

import (
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"

	"github.com/alnah/moth/internal/limits"
)

func TestMergeFileConfigAppliesPresentFieldsUnlessFlagSet(t *testing.T) {
	base := RuntimeConfig{
		BrowserBin: "/usr/bin/base-chrome",
		Limits: limits.Options{
			Timeout:    3 * time.Second,
			MaxResults: 3,
			MaxBytes:   3_000,
			Retries:    3,
			RetryBase:  300 * time.Millisecond,
			RetryMax:   3 * time.Second,
		},
	}
	file := FileConfig{
		Browser: BrowserConfig{Bin: "/opt/file-chrome"},
		Limits: limits.Options{
			Timeout:    7 * time.Second,
			MaxResults: 7,
			MaxBytes:   7_000,
			Retries:    7,
			RetryBase:  700 * time.Millisecond,
			RetryMax:   7 * time.Second,
		},
		Presence: allConfigFieldsPresent(),
	}

	tests := []struct {
		name        string
		file        FileConfig
		flags       FieldSet
		wantRuntime RuntimeConfig
		wantApplied FieldSet
	}{
		{
			name:        "applies every present file field without flags",
			file:        file,
			wantRuntime: RuntimeConfig{BrowserBin: file.Browser.Bin, Limits: file.Limits},
			wantApplied: allMergedFieldsApplied(),
		},
		{
			name:        "flags preserve base values",
			file:        file,
			flags:       allMergedFieldsApplied(),
			wantRuntime: base,
			wantApplied: FieldSet{},
		},
		{
			name: "explicit zero limit values are applied",
			file: FileConfig{
				Limits:   limits.Options{},
				Presence: FileConfigPresence{Limits: allLimitFieldsPresent()},
			},
			wantRuntime: RuntimeConfig{BrowserBin: base.BrowserBin, Limits: limits.Options{}},
			wantApplied: FieldSet{
				Timeout:    true,
				MaxResults: true,
				MaxBytes:   true,
				Retries:    true,
				RetryBase:  true,
				RetryMax:   true,
			},
		},
		{
			name: "marks exactly applied fields",
			file: FileConfig{
				Browser: BrowserConfig{Bin: "/opt/partial-chrome"},
				Limits: limits.Options{
					MaxBytes: 42,
					RetryMax: 4 * time.Second,
				},
				Presence: FileConfigPresence{
					BrowserBin: true,
					Limits: LimitsConfigPresence{
						MaxBytes: true,
						RetryMax: true,
					},
				},
			},
			wantRuntime: RuntimeConfig{
				BrowserBin: "/opt/partial-chrome",
				Limits: limits.Options{
					Timeout:    base.Limits.Timeout,
					MaxResults: base.Limits.MaxResults,
					MaxBytes:   42,
					Retries:    base.Limits.Retries,
					RetryBase:  base.Limits.RetryBase,
					RetryMax:   4 * time.Second,
				},
			},
			wantApplied: FieldSet{BrowserBin: true, MaxBytes: true, RetryMax: true},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotRuntime, gotApplied := MergeFileConfig(base, tt.file, tt.flags)
			if diff := cmp.Diff(tt.wantRuntime, gotRuntime); diff != "" {
				t.Fatalf("MergeFileConfig() runtime mismatch (-want +got):\n%s", diff)
			}
			if diff := cmp.Diff(tt.wantApplied, gotApplied); diff != "" {
				t.Fatalf("MergeFileConfig() applied fields mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func allMergedFieldsApplied() FieldSet {
	return FieldSet{
		BrowserBin: true,
		Timeout:    true,
		MaxResults: true,
		MaxBytes:   true,
		Retries:    true,
		RetryBase:  true,
		RetryMax:   true,
	}
}
