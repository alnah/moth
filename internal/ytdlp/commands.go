package ytdlp

import "strings"

func metadataArgs(request MetadataRequest) []string {
	return []string{"-J", "--skip-download", request.URL}
}

func subtitleArgs(request SubtitleRequest) []string {
	args := []string{"--skip-download", "--write-subs"}
	if request.IncludeAutomatic {
		args = append(args, "--write-auto-subs")
	}
	if len(request.Languages) > 0 {
		args = append(args, "--sub-langs", strings.Join(request.Languages, ","))
	}
	if request.Format != "" {
		args = append(args, "--sub-format", request.Format)
	}

	return append(args,
		"--paths", request.OutputDir,
		"--output", "subtitle:%(id)s.%(language)s.%(ext)s",
		request.URL,
	)
}

func audioArgs(request AudioRequest) []string {
	args := []string{"--extract-audio"}
	if request.Format != "" {
		args = append(args, "--audio-format", request.Format)
	}
	if request.Section.Start > 0 || request.Section.End > 0 {
		args = append(args, "--download-sections", formatDownloadSection(request.Section))
	}

	return append(args,
		"--paths", request.OutputDir,
		"--output", "%(id)s.%(ext)s",
		"--print", "after_move:filepath",
		request.URL,
	)
}
