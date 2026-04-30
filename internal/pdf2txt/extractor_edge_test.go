package pdf2txt

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alnah/moth/internal/content"
	"github.com/alnah/moth/internal/tools"
)

func TestExtractReturnsWeakTextWhenOCRIsDisabled(t *testing.T) {
	inputPDF := writeTestPDF(t)
	runner := newFakePDFRunner([]string{"%%%% 0000 ��"})
	extractor := New(Options{
		Runner:        runner,
		PDFToTextPath: "pdftotext",
		TempDir:       t.TempDir(),
		OCRAllowed:    false,
	})

	got, err := extractor.Extract(context.Background(), inputPDF)
	if err != nil {
		t.Fatalf("Extract(weak text with OCR disabled) error = %v, want nil", err)
	}
	if got.Text != "%%%% 0000 ��" {
		t.Fatalf("Extract(weak text with OCR disabled) text = %q, want weak pdftotext output", got.Text)
	}
	assertToolCalls(t, runner.calls, []tools.ToolName{tools.ToolPDFToText})
}

func TestExtractResolvesDefaultToolPaths(t *testing.T) {
	inputPDF := writeTestPDF(t)
	toolsDir := t.TempDir()
	pdfToTextPath := writeFakeExecutable(t, toolsDir, tools.ToolPDFToText)
	ocrMyPDFPath := writeFakeExecutable(t, toolsDir, tools.ToolOCRMyPDF)
	writeFakeExecutable(t, toolsDir, tools.ToolTesseract)
	runner := newFakePDFRunner([]string{"", "Text recovered through resolved tools."})
	extractor := New(Options{Runner: runner, TempDir: "", OCRAllowed: true})
	t.Setenv("PATH", toolsDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	got, err := extractor.Extract(context.Background(), inputPDF)
	if err != nil {
		t.Fatalf("Extract(default tool resolution) error = %v, want nil", err)
	}
	if got.Text != "Text recovered through resolved tools." {
		t.Fatalf("Extract(default tool resolution) text = %q, want OCR text", got.Text)
	}
	if runner.calls[0].Path != pdfToTextPath || runner.calls[1].Path != ocrMyPDFPath {
		t.Fatalf("tool paths = %q, %q; want %q, %q", runner.calls[0].Path, runner.calls[1].Path, pdfToTextPath, ocrMyPDFPath)
	}
}

func TestExtractReturnsToolMissingWhenPDFToTextCannotResolve(t *testing.T) {
	inputPDF := writeTestPDF(t)
	extractor := New(Options{Runner: newFakePDFRunner([]string{}), TempDir: t.TempDir()})
	t.Setenv("PATH", t.TempDir())

	_, err := extractor.Extract(context.Background(), inputPDF)
	if err == nil {
		t.Fatal("Extract(missing pdftotext) error = nil, want tool_missing")
	}
	if !errors.Is(err, tools.ErrToolMissing) {
		t.Fatalf("Extract(missing pdftotext) error = %v, want ErrToolMissing", err)
	}
}

func TestExtractReturnsPDFToTextRunnerError(t *testing.T) {
	inputPDF := writeTestPDF(t)
	runner := failingToolRunner{tool: tools.ToolPDFToText, err: errors.New("pdftotext crashed")}
	extractor := New(Options{
		Runner:        runner,
		PDFToTextPath: "pdftotext",
		TempDir:       t.TempDir(),
	})

	_, err := extractor.Extract(context.Background(), inputPDF)
	if err == nil {
		t.Fatal("Extract(pdftotext failure) error = nil, want error")
	}
	if !strings.Contains(err.Error(), "pdftotext crashed") {
		t.Fatalf("Extract(pdftotext failure) error = %v, want runner context", err)
	}
}

func TestExtractReturnsToolMissingWhenTesseractCannotResolve(t *testing.T) {
	inputPDF := writeTestPDF(t)
	toolsDir := t.TempDir()
	writeFakeExecutable(t, toolsDir, tools.ToolPDFToText)
	writeFakeExecutable(t, toolsDir, tools.ToolOCRMyPDF)
	runner := newFakePDFRunner([]string{""})
	extractor := New(Options{
		Runner:     runner,
		TempDir:    t.TempDir(),
		OCRAllowed: true,
	})
	t.Setenv("PATH", toolsDir)

	got, err := extractor.Extract(context.Background(), inputPDF)
	if err == nil {
		t.Fatal("Extract(missing resolved tesseract) error = nil, want tool_missing")
	}
	if !errors.Is(err, tools.ErrToolMissing) {
		t.Fatalf("Extract(missing resolved tesseract) error = %v, want ErrToolMissing", err)
	}
	assertWarningsContain(t, got.Warnings, content.WarningToolMissing)
	assertToolCalls(t, runner.calls, []tools.ToolName{tools.ToolPDFToText})
}

func TestExtractReturnsToolMissingWhenOCRMyPDFCannotResolve(t *testing.T) {
	inputPDF := writeTestPDF(t)
	runner := newFakePDFRunner([]string{""})
	extractor := New(Options{
		Runner:        runner,
		PDFToTextPath: "pdftotext",
		TesseractPath: "tesseract",
		TempDir:       t.TempDir(),
		OCRAllowed:    true,
	})
	t.Setenv("PATH", t.TempDir())

	got, err := extractor.Extract(context.Background(), inputPDF)
	if err == nil {
		t.Fatal("Extract(missing resolved ocrmypdf) error = nil, want tool_missing")
	}
	if !errors.Is(err, tools.ErrToolMissing) {
		t.Fatalf("Extract(missing resolved ocrmypdf) error = %v, want ErrToolMissing", err)
	}
	assertWarningsContain(t, got.Warnings, content.WarningToolMissing)
}

func TestExtractReturnsOCRFailedErrorWhenOCRToolFails(t *testing.T) {
	inputPDF := writeTestPDF(t)
	runner := newFailingOCRRunner([]string{""}, errors.New("ocr crashed"))
	extractor := New(Options{
		Runner:        runner,
		PDFToTextPath: "pdftotext",
		OCRMyPDFPath:  "ocrmypdf",
		TesseractPath: "tesseract",
		TempDir:       t.TempDir(),
		OCRAllowed:    true,
	})

	got, err := extractor.Extract(context.Background(), inputPDF)
	if err == nil {
		t.Fatal("Extract(OCR failure) error = nil, want error")
	}
	assertWarningsContain(t, got.Warnings, content.WarningOCRFailed)
}

func TestExtractRejectsOversizedTextOutput(t *testing.T) {
	inputPDF := writeTestPDF(t)
	runner := newFakePDFRunner([]string{"text output over limit"})
	extractor := New(Options{
		Runner:        runner,
		PDFToTextPath: "pdftotext",
		TempDir:       t.TempDir(),
		OCRAllowed:    false,
		MaxTextBytes:  4,
	})

	_, err := extractor.Extract(context.Background(), inputPDF)
	if err == nil {
		t.Fatal("Extract(oversized text output) error = nil, want size error")
	}
	if !strings.Contains(err.Error(), "over 4 bytes") {
		t.Fatalf("Extract(oversized text output) error = %v, want size context", err)
	}
}

type failingToolRunner struct {
	tool tools.ToolName
	err  error
}

func (runner failingToolRunner) Run(_ context.Context, command tools.Command) (tools.Result, error) {
	if command.Tool == runner.tool {
		return tools.Result{}, runner.err
	}

	return tools.Result{}, errors.New("unexpected tool")
}

type failingOCRRunner struct {
	fakePDFRunner
	err error
}

func newFailingOCRRunner(texts []string, err error) *failingOCRRunner {
	return &failingOCRRunner{fakePDFRunner: *newFakePDFRunner(texts), err: err}
}

func (runner *failingOCRRunner) Run(ctx context.Context, command tools.Command) (tools.Result, error) {
	if command.Tool == tools.ToolOCRMyPDF {
		runner.calls = append(runner.calls, command)
		return tools.Result{}, runner.err
	}

	return runner.fakePDFRunner.Run(ctx, command)
}

func writeFakeExecutable(t *testing.T, dir string, name tools.ToolName) string {
	t.Helper()

	path := filepath.Join(dir, string(name))
	//nolint:gosec // Test fixture must be executable for PATH resolution.
	if err := os.WriteFile(path, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatalf("write fake executable: %v", err)
	}

	return path
}
