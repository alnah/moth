package pdf2txt

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

const (
	minStrongTextRunes = 20
	minStrongRuneRatio = 0.5
	maxWeakRuneRatio   = 0.1
)

// isStrongText accepts enough searchable text to skip OCR: at least 20 runes,
// at least 50% letters or digits, and no more than 10% invalid/control runes.
func isStrongText(text string) bool {
	trimmed := strings.TrimSpace(text)
	if utf8.RuneCountInString(trimmed) < minStrongTextRunes {
		return false
	}

	total := 0
	strong := 0
	weak := 0
	for _, value := range trimmed {
		if unicode.IsSpace(value) {
			continue
		}
		total++
		if unicode.IsLetter(value) || unicode.IsDigit(value) {
			strong++
		}
		if value == utf8.RuneError || unicode.IsControl(value) {
			weak++
		}
	}
	if total == 0 {
		return false
	}

	strongRatio := float64(strong) / float64(total)
	weakRatio := float64(weak) / float64(total)

	return strongRatio >= minStrongRuneRatio && weakRatio <= maxWeakRuneRatio
}
