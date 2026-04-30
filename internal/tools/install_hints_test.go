package tools_test

import (
	"context"
	"strings"
	"testing"

	"github.com/alnah/moth/internal/tools"
)

func TestDoctorInstallHintsAreSpecificToPlatformAndTool(t *testing.T) {
	cases := []struct {
		name                 string
		platform             tools.Platform
		managerTokens        []string
		ocrFallbackTokens    []string
		browserFallbackToken []string
	}{
		{
			name:              "linux managers",
			platform:          tools.Platform{OS: "linux"},
			managerTokens:     []string{"apt", "dnf", "snap", "nix"},
			ocrFallbackTokens: []string{"apt", "dnf", "snap", "nix"},
		},
		{
			name:              "macos managers",
			platform:          tools.Platform{OS: "darwin"},
			managerTokens:     []string{"brew", "macports", "port", "nix"},
			ocrFallbackTokens: []string{"brew", "macports", "port", "nix"},
		},
		{
			name:                 "windows managers and OCR fallback",
			platform:             tools.Platform{OS: "windows"},
			managerTokens:        []string{"winget", "scoop", "choco"},
			ocrFallbackTokens:    []string{"wsl", "docker", "winget", "scoop", "choco"},
			browserFallbackToken: []string{"winget", "scoop", "choco"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			toolsDir := t.TempDir()
			fakeExecutablePath(t, toolsDir, "tesseract")

			t.Setenv("PATH", isolatedPATH(t))
			t.Setenv("ROD_BROWSER_BIN", "")

			report, err := tools.Doctor(context.Background(), tools.DoctorOptions{
				ToolsDir:                   toolsDir,
				RequiredTesseractLanguages: []string{"eng", "fra"},
				Platform:                   tc.platform,
				Runner:                     newFakeDoctorRunner("eng"),
			})
			if err != nil {
				t.Fatalf("run doctor for %s: %v", tc.platform.OS, err)
			}

			ytdlpStatus := findToolStatus(t, report, tools.ToolYTDLP)
			if ytdlpStatus.Status != tools.StatusMissing {
				t.Fatalf("yt-dlp status = %q, want missing", ytdlpStatus.Status)
			}
			assertHintsMentionAny(t, ytdlpStatus.InstallHints, tc.managerTokens)

			ocrStatus := findToolStatus(t, report, tools.ToolOCRMyPDF)
			if ocrStatus.Status != tools.StatusMissing {
				t.Fatalf("ocrmypdf status = %q, want missing", ocrStatus.Status)
			}
			assertHintsMentionAny(t, ocrStatus.InstallHints, tc.ocrFallbackTokens)

			tesseractStatus := findToolStatus(t, report, tools.ToolTesseract)
			if tesseractStatus.Status != tools.StatusWarning {
				t.Fatalf("tesseract status = %q, want warning for missing fra language", tesseractStatus.Status)
			}
			assertHintsMentionAny(t, tesseractStatus.InstallHints, tc.ocrFallbackTokens)

			browserStatus := findToolStatus(t, report, tools.ToolChromium)
			if browserStatus.Status != tools.StatusMissing {
				t.Fatalf("chromium status = %q, want missing", browserStatus.Status)
			}
			browserTokens := tc.managerTokens
			if len(tc.browserFallbackToken) > 0 {
				browserTokens = tc.browserFallbackToken
			}
			assertHintsMentionAny(t, browserStatus.InstallHints, browserTokens)
		})
	}
}

func assertHintsMentionAny(t *testing.T, hints []string, tokens []string) {
	t.Helper()

	if len(hints) == 0 {
		t.Fatalf("install hints are empty, want one of %v", tokens)
	}

	joinedHints := strings.ToLower(strings.Join(hints, "\n"))
	for _, token := range tokens {
		if strings.Contains(joinedHints, strings.ToLower(token)) {
			return
		}
	}

	t.Fatalf("install hints %q do not mention any of %v", joinedHints, tokens)
}
