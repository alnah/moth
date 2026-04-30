// Package pdf2txt extracts text from PDFs with optional OCR fallback.
package pdf2txt

import (
	"context"
	"path/filepath"

	"github.com/alnah/moth/internal/content"
	"github.com/alnah/moth/internal/limits"
	"github.com/alnah/moth/internal/tools"
)

const (
	defaultOCRLanguage = "fra+eng"
	defaultOCRMode     = "skip"
)

// Options configures PDF text extraction.
type Options struct {
	Runner        tools.Runner
	PDFToTextPath string
	OCRMyPDFPath  string
	TesseractPath string
	ToolsDir      string
	TempDir       string
	OCRAllowed    bool
	OCRLanguage   string
	OCRMode       string
	MaxTextBytes  int64
}

// Extractor runs pdftotext first, then OCRmyPDF when extracted text is weak or empty.
type Extractor struct {
	runner        tools.Runner
	pdfToTextPath string
	ocrMyPDFPath  string
	tesseractPath string
	toolsDir      string
	tempDir       string
	ocrAllowed    bool
	ocrLanguage   string
	ocrMode       string
	maxTextBytes  int64
}

// New creates an Extractor with conservative defaults.
func New(options Options) *Extractor {
	runner := options.Runner
	if runner == nil {
		runner = tools.LocalRunner{}
	}
	ocrLanguage := options.OCRLanguage
	if ocrLanguage == "" {
		ocrLanguage = defaultOCRLanguage
	}
	ocrMode := options.OCRMode
	if ocrMode == "" {
		ocrMode = defaultOCRMode
	}
	maxTextBytes := options.MaxTextBytes
	if maxTextBytes <= 0 {
		maxTextBytes = limits.DefaultMaxBytes
	}

	return &Extractor{
		runner:        runner,
		pdfToTextPath: options.PDFToTextPath,
		ocrMyPDFPath:  options.OCRMyPDFPath,
		tesseractPath: options.TesseractPath,
		toolsDir:      options.ToolsDir,
		tempDir:       options.TempDir,
		ocrAllowed:    options.OCRAllowed,
		ocrLanguage:   ocrLanguage,
		ocrMode:       ocrMode,
		maxTextBytes:  maxTextBytes,
	}
}

// Extract returns PDF text. It preserves weak non-empty text when OCR tools are missing.
func (extractor *Extractor) Extract(ctx context.Context, inputPDF string) (content.Item, error) {
	item := content.Item{Kind: content.KindPDF, Warnings: []content.Warning{}}
	workDir, err := extractor.createWorkDir()
	if err != nil {
		return item, err
	}
	defer workDir.close()

	firstTextPath := filepath.Join(workDir.path, "first.txt")
	text, err := extractor.runPDFToText(ctx, inputPDF, firstTextPath)
	if err != nil {
		return item, err
	}
	item.Text = text
	if isStrongText(text) || !extractor.ocrAllowed {
		return item, nil
	}

	ocrPDFPath := filepath.Join(workDir.path, "ocr.pdf")
	err = extractor.runOCRMyPDF(ctx, inputPDF, ocrPDFPath)
	if err != nil {
		return extractor.handleOCRError(item, err)
	}

	secondTextPath := filepath.Join(workDir.path, "ocr.txt")
	text, err = extractor.runPDFToText(ctx, ocrPDFPath, secondTextPath)
	if err != nil {
		return item, err
	}
	item.Text = text
	item.Warnings = append(item.Warnings, content.WarningOCRUsed)

	return item, nil
}
