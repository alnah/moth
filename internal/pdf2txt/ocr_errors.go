package pdf2txt

import (
	"errors"
	"fmt"
	"strings"

	"github.com/alnah/moth/internal/content"
	"github.com/alnah/moth/internal/tools"
)

func (extractor *Extractor) handleOCRError(item content.Item, err error) (content.Item, error) {
	if !errors.Is(err, tools.ErrToolMissing) {
		item.Warnings = append(item.Warnings, content.WarningOCRFailed)
		return item, fmt.Errorf("OCR PDF: %w", err)
	}

	item.Warnings = append(item.Warnings, content.WarningToolMissing)
	if strings.TrimSpace(item.Text) != "" {
		return item, nil
	}

	return item, fmt.Errorf("OCR PDF: %w", err)
}
