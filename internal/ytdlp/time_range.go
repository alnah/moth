package ytdlp

import (
	"fmt"
	"time"
)

func validateTimeRange(timeRange TimeRange) error {
	if timeRange.End > 0 && timeRange.Start > timeRange.End {
		return fmt.Errorf("yt-dlp duration range is invalid")
	}

	return nil
}

func formatDownloadSection(timeRange TimeRange) string {
	return "*" + formatSectionTime(timeRange.Start) + "-" + formatSectionTime(timeRange.End)
}

func formatSectionTime(duration time.Duration) string {
	totalSeconds := int(duration.Round(time.Second) / time.Second)
	hours := totalSeconds / 3600
	minutes := totalSeconds % 3600 / 60
	seconds := totalSeconds % 60

	return fmt.Sprintf("%02d:%02d:%02d", hours, minutes, seconds)
}
