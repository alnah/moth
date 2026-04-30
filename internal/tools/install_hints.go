package tools

import "strings"

func installHints(name ToolName, platform Platform, missingLanguages []string) []string {
	osName := strings.ToLower(platform.OS)
	switch name {
	case ToolYTDLP:
		return ytdlpHints(osName)
	case ToolFFmpeg, ToolFFprobe:
		return ffmpegHints(osName)
	case ToolPDFToText:
		return popplerHints(osName)
	case ToolOCRMyPDF:
		return ocrMyPDFHints(osName)
	case ToolTesseract:
		return tesseractHints(osName, missingLanguages)
	case ToolChromium:
		return chromiumHints(osName)
	}
	return []string{"install the missing tool with apt, dnf, snap, nix, brew, winget, scoop, choco, or --tools-dir"}
}

func ytdlpHints(osName string) []string {
	if osName == "darwin" {
		return []string{"install yt-dlp with brew, macports/port, nix, or place the official release binary in --tools-dir"}
	}
	if osName == "windows" {
		return []string{"install yt-dlp with winget, scoop, choco, or place yt-dlp.exe in --tools-dir"}
	}
	return []string{"install yt-dlp with apt, dnf, snap, nix, pipx, or place the official release binary in --tools-dir"}
}

func ffmpegHints(osName string) []string {
	if osName == "darwin" {
		return []string{"install ffmpeg with brew, macports/port, or nix"}
	}
	if osName == "windows" {
		return []string{"install ffmpeg with winget, scoop, or choco"}
	}
	return []string{"install ffmpeg with apt, dnf, snap, or nix"}
}

func popplerHints(osName string) []string {
	if osName == "darwin" {
		return []string{"install poppler for pdftotext with brew, macports/port, or nix"}
	}
	if osName == "windows" {
		return []string{"install poppler for pdftotext with winget, scoop, or choco"}
	}
	return []string{"install poppler-utils for pdftotext with apt, dnf, snap, or nix"}
}

func ocrMyPDFHints(osName string) []string {
	if osName == "darwin" {
		return []string{"install ocrmypdf with brew, macports/port, or nix"}
	}
	if osName == "windows" {
		return []string{
			"install OCRmyPDF dependencies with winget, scoop, or choco; " +
				"use WSL or Docker if native OCRmyPDF is unavailable",
		}
	}
	return []string{"install ocrmypdf with apt, dnf, snap, or nix"}
}

func tesseractHints(osName string, missingLanguages []string) []string {
	languageSuffix := ""
	if len(missingLanguages) > 0 {
		languageSuffix = " for languages " + strings.Join(missingLanguages, ",")
	}

	if osName == "darwin" {
		return []string{"install tesseract and tessdata" + languageSuffix + " with brew, macports/port, or nix"}
	}
	if osName == "windows" {
		return []string{
			"install tesseract and language data" + languageSuffix +
				" with winget, scoop, or choco; WSL or Docker can provide OCR fallback",
		}
	}
	return []string{"install tesseract OCR language packages" + languageSuffix + " with apt, dnf, snap, or nix"}
}

func chromiumHints(osName string) []string {
	if osName == "darwin" {
		return []string{"install Chromium or Chrome with brew, macports/port, or nix, or set ROD_BROWSER_BIN"}
	}
	if osName == "windows" {
		return []string{"install Chromium, Chrome, or Edge with winget, scoop, or choco, or set ROD_BROWSER_BIN"}
	}
	return []string{"install chromium, google-chrome, or edge with apt, dnf, snap, or nix, or set ROD_BROWSER_BIN"}
}
