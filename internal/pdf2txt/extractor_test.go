package pdf2txt

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/alnah/moth/internal/content"
	"github.com/alnah/moth/internal/tools"
)

func TestExtractUsesPDFToTextOnlyWhenTextIsStrong(t *testing.T) {
	inputPDF := writeTestPDF(t)
	runner := newFakePDFRunner([]string{"This PDF has enough searchable text to pass the quality gate."})
	extractor := New(Options{
		Runner:        runner,
		PDFToTextPath: "pdftotext",
		OCRMyPDFPath:  "ocrmypdf",
		TesseractPath: "tesseract",
		TempDir:       t.TempDir(),
		OCRAllowed:    true,
		OCRLanguage:   "fra+eng",
		OCRMode:       "skip",
	})

	got, err := extractor.Extract(context.Background(), inputPDF)
	if err != nil {
		t.Fatalf("Extract(strong searchable PDF) error = %v, want nil", err)
	}

	if got.Text != "This PDF has enough searchable text to pass the quality gate." {
		t.Fatalf("Extract(strong searchable PDF) text = %q, want pdftotext output", got.Text)
	}
	assertWarningsMissing(t, got.Warnings, content.WarningOCRUsed, content.WarningToolMissing)
	assertToolCalls(t, runner.calls, []tools.ToolName{tools.ToolPDFToText})
}

func TestExtractRunsOCRThenPDFToTextAgainWhenTextIsEmpty(t *testing.T) {
	inputPDF := writeTestPDF(t)
	runner := newFakePDFRunner([]string{"", "Text recovered after OCR."})
	extractor := New(defaultExtractorOptions(t, runner))

	got, err := extractor.Extract(context.Background(), inputPDF)
	if err != nil {
		t.Fatalf("Extract(scanned PDF) error = %v, want nil", err)
	}

	if got.Text != "Text recovered after OCR." {
		t.Fatalf("Extract(scanned PDF) text = %q, want second pdftotext output", got.Text)
	}
	assertWarningsContain(t, got.Warnings, content.WarningOCRUsed)
	assertToolCalls(t, runner.calls, []tools.ToolName{tools.ToolPDFToText, tools.ToolOCRMyPDF, tools.ToolPDFToText})
	assertOCRMyPDFUsesFastTemporaryPDFMode(t, runner.calls[1])
}

func TestExtractUsesDefaultOCRLanguageWhenOCRIsNeeded(t *testing.T) {
	inputPDF := writeTestPDF(t)
	runner := newFakePDFRunner([]string{"", "Text recovered with default OCR language."})
	extractor := New(Options{
		Runner:        runner,
		PDFToTextPath: "pdftotext",
		OCRMyPDFPath:  "ocrmypdf",
		TesseractPath: "tesseract",
		TempDir:       t.TempDir(),
		OCRAllowed:    true,
	})

	got, err := extractor.Extract(context.Background(), inputPDF)
	if err != nil {
		t.Fatalf("Extract(scanned PDF with default OCR language) error = %v, want nil", err)
	}

	if got.Text != "Text recovered with default OCR language." {
		t.Fatalf("Extract(scanned PDF with default OCR language) text = %q, want second pdftotext output", got.Text)
	}
	assertToolCalls(t, runner.calls, []tools.ToolName{tools.ToolPDFToText, tools.ToolOCRMyPDF, tools.ToolPDFToText})
	assertOCRMyPDFUsesFastTemporaryPDFMode(t, runner.calls[1])
}

func TestExtractRunsOCRWhenTextQualityIsWeak(t *testing.T) {
	inputPDF := writeTestPDF(t)
	runner := newFakePDFRunner([]string{"%%%% 0000 \uFFFD\uFFFD", "Readable text recovered by OCR."})
	extractor := New(defaultExtractorOptions(t, runner))

	got, err := extractor.Extract(context.Background(), inputPDF)
	if err != nil {
		t.Fatalf("Extract(weak text PDF) error = %v, want nil", err)
	}

	if got.Text != "Readable text recovered by OCR." {
		t.Fatalf("Extract(weak text PDF) text = %q, want OCR recovered text", got.Text)
	}
	assertWarningsContain(t, got.Warnings, content.WarningOCRUsed)
	assertToolCalls(t, runner.calls, []tools.ToolName{tools.ToolPDFToText, tools.ToolOCRMyPDF, tools.ToolPDFToText})
}

func TestExtractDoesNotUseOCRMyPDFSidecarAsFinalText(t *testing.T) {
	inputPDF := writeTestPDF(t)
	runner := newFakePDFRunner([]string{"", "Full document text from OCR PDF, not sidecar."})
	runner.sidecarText = "sidecar-only text must not be returned"
	extractor := New(defaultExtractorOptions(t, runner))

	got, err := extractor.Extract(context.Background(), inputPDF)
	if err != nil {
		t.Fatalf("Extract(scanned PDF with sidecar trap) error = %v, want nil", err)
	}

	if got.Text != "Full document text from OCR PDF, not sidecar." {
		t.Fatalf("Extract(scanned PDF with sidecar trap) text = %q, want final pdftotext output", got.Text)
	}
	if strings.Contains(got.Text, "sidecar-only") {
		t.Fatalf("Extract(scanned PDF with sidecar trap) text = %q, want no OCRmyPDF sidecar text", got.Text)
	}
	assertToolCalls(t, runner.calls, []tools.ToolName{tools.ToolPDFToText, tools.ToolOCRMyPDF, tools.ToolPDFToText})
}

func TestExtractReturnsWeakTextAndToolMissingWarningWhenOCRMyPDFIsMissing(t *testing.T) {
	inputPDF := writeTestPDF(t)
	runner := newFakePDFRunner([]string{"%%%% 0000 \uFFFD\uFFFD"})
	runner.missingTools[tools.ToolOCRMyPDF] = true
	extractor := New(defaultExtractorOptions(t, runner))

	got, err := extractor.Extract(context.Background(), inputPDF)
	if err != nil {
		t.Fatalf("Extract(weak text without ocrmypdf) error = %v, want weak text with warning", err)
	}

	if got.Text != "%%%% 0000 \uFFFD\uFFFD" {
		t.Fatalf("Extract(weak text without ocrmypdf) text = %q, want weak pdftotext output", got.Text)
	}
	assertWarningsContain(t, got.Warnings, content.WarningToolMissing)
	assertToolCalls(t, runner.calls, []tools.ToolName{tools.ToolPDFToText, tools.ToolOCRMyPDF})
}

func TestExtractReturnsErrorWhenOCRMyPDFIsMissingAndTextIsEmpty(t *testing.T) {
	inputPDF := writeTestPDF(t)
	runner := newFakePDFRunner([]string{""})
	runner.missingTools[tools.ToolOCRMyPDF] = true
	extractor := New(defaultExtractorOptions(t, runner))

	got, err := extractor.Extract(context.Background(), inputPDF)
	if err == nil {
		t.Fatal("Extract(empty text without ocrmypdf) error = nil, want tool_missing error")
	}
	if !errors.Is(err, tools.ErrToolMissing) {
		t.Fatalf("Extract(empty text without ocrmypdf) error = %v, want ErrToolMissing", err)
	}
	assertWarningsContain(t, got.Warnings, content.WarningToolMissing)
	assertToolCalls(t, runner.calls, []tools.ToolName{tools.ToolPDFToText, tools.ToolOCRMyPDF})
}

func TestExtractReturnsErrorWhenTesseractIsMissingAndTextIsEmpty(t *testing.T) {
	inputPDF := writeTestPDF(t)
	runner := newFakePDFRunner([]string{""})
	runner.missingTools[tools.ToolTesseract] = true
	extractor := New(defaultExtractorOptions(t, runner))

	got, err := extractor.Extract(context.Background(), inputPDF)
	if err == nil {
		t.Fatal("Extract(empty text without tesseract) error = nil, want tool_missing error")
	}
	if !errors.Is(err, tools.ErrToolMissing) {
		t.Fatalf("Extract(empty text without tesseract) error = %v, want ErrToolMissing", err)
	}
	assertWarningsContain(t, got.Warnings, content.WarningToolMissing)
}

func TestExtractCleansTemporaryOCRFiles(t *testing.T) {
	inputPDF := writeTestPDF(t)
	tempRoot := t.TempDir()
	runner := newFakePDFRunner([]string{"", "Clean text after OCR."})
	extractor := New(Options{
		Runner:        runner,
		PDFToTextPath: "pdftotext",
		OCRMyPDFPath:  "ocrmypdf",
		TesseractPath: "tesseract",
		TempDir:       tempRoot,
		OCRAllowed:    true,
		OCRLanguage:   "fra+eng",
		OCRMode:       "skip",
	})

	_, err := extractor.Extract(context.Background(), inputPDF)
	if err != nil {
		t.Fatalf("Extract(scanned PDF cleanup) error = %v, want nil", err)
	}

	entries, err := os.ReadDir(tempRoot)
	if err != nil {
		t.Fatalf("read temp root after Extract: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("temp root entries after Extract = %v, want cleanup", entries)
	}
}

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

func defaultExtractorOptions(t *testing.T, runner *fakePDFRunner) Options {
	t.Helper()

	return Options{
		Runner:        runner,
		PDFToTextPath: "pdftotext",
		OCRMyPDFPath:  "ocrmypdf",
		TesseractPath: "tesseract",
		TempDir:       t.TempDir(),
		OCRAllowed:    true,
		OCRLanguage:   "fra+eng",
		OCRMode:       "skip",
	}
}

func writeTestPDF(t *testing.T) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "input.pdf")
	if err := os.WriteFile(path, []byte("%PDF-1.7\n"), 0o600); err != nil {
		t.Fatalf("write test PDF: %v", err)
	}
	return path
}

type fakePDFRunner struct {
	texts        []string
	textIndex    int
	calls        []tools.Command
	missingTools map[tools.ToolName]bool
	sidecarText  string
}

func newFakePDFRunner(texts []string) *fakePDFRunner {
	return &fakePDFRunner{
		texts:        texts,
		missingTools: map[tools.ToolName]bool{},
	}
}

func (runner *fakePDFRunner) Run(_ context.Context, command tools.Command) (tools.Result, error) {
	runner.calls = append(runner.calls, command)
	if runner.missingTools[command.Tool] {
		return tools.Result{}, fmt.Errorf("run %s: %w", command.Tool, tools.ErrToolMissing)
	}

	switch command.Tool {
	case tools.ToolPDFToText:
		if runner.textIndex >= len(runner.texts) {
			return tools.Result{}, errors.New("fake pdftotext text output exhausted")
		}
		outputPath := lastArg(command.Args)
		if err := os.WriteFile(outputPath, []byte(runner.texts[runner.textIndex]), 0o600); err != nil {
			return tools.Result{}, err
		}
		runner.textIndex++
		return tools.Result{ExitCode: 0}, nil
	case tools.ToolOCRMyPDF:
		if runner.missingTools[tools.ToolTesseract] {
			return tools.Result{}, fmt.Errorf("run ocrmypdf: %w", tools.ErrToolMissing)
		}
		if runner.sidecarText != "" {
			for index, arg := range command.Args {
				if arg == "--sidecar" && index+1 < len(command.Args) {
					if err := os.WriteFile(command.Args[index+1], []byte(runner.sidecarText), 0o600); err != nil {
						return tools.Result{}, err
					}
				}
			}
		}
		outputPath := lastArg(command.Args)
		if err := os.WriteFile(outputPath, []byte("%PDF-1.7\nocr layer\n"), 0o600); err != nil {
			return tools.Result{}, err
		}
		return tools.Result{ExitCode: 0}, nil
	case tools.ToolTesseract:
		return tools.Result{ExitCode: 0}, nil
	default:
		return tools.Result{}, fmt.Errorf("unexpected tool %s", command.Tool)
	}
}

func assertToolCalls(t *testing.T, calls []tools.Command, want []tools.ToolName) {
	t.Helper()

	got := make([]tools.ToolName, 0, len(calls))
	for _, call := range calls {
		got = append(got, call.Tool)
	}
	if !slices.Equal(got, want) {
		t.Fatalf("tool calls = %v, want %v", got, want)
	}
}

func assertOCRMyPDFUsesFastTemporaryPDFMode(t *testing.T, call tools.Command) {
	t.Helper()

	if call.Tool != tools.ToolOCRMyPDF {
		t.Fatalf("OCR call tool = %s, want ocrmypdf", call.Tool)
	}
	assertArgsContainAdjacent(t, call.Args, "-l", "fra+eng")
	assertArgsContainAdjacent(t, call.Args, "--mode", "skip")
	assertArgsContainAdjacent(t, call.Args, "--output-type", "pdf")
	assertArgsContainAdjacent(t, call.Args, "--optimize", "0")
}

func assertArgsContainAdjacent(t *testing.T, args []string, key string, value string) {
	t.Helper()

	for index := 0; index+1 < len(args); index++ {
		if args[index] == key && args[index+1] == value {
			return
		}
	}
	t.Fatalf("args = %v, want adjacent %q %q", args, key, value)
}

func assertWarningsContain(t *testing.T, warnings []content.Warning, want content.Warning) {
	t.Helper()

	if slices.Contains(warnings, want) {
		return
	}
	t.Fatalf("warnings = %v, want %q", warnings, want)
}

func assertWarningsMissing(t *testing.T, warnings []content.Warning, unwanted ...content.Warning) {
	t.Helper()

	for _, warning := range unwanted {
		if slices.Contains(warnings, warning) {
			t.Fatalf("warnings = %v, want no %q", warnings, warning)
		}
	}
}

func lastArg(args []string) string {
	if len(args) == 0 {
		return ""
	}
	return args[len(args)-1]
}
