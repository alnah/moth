//go:build integration

package pdf2txt

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/alnah/moth/internal/content"
)

func TestIntegrationExtractSearchablePDFWithPDFToText(t *testing.T) {
	pdfToTextPath := requireIntegrationExecutable(t, "pdftotext", "-v")
	inputPDF := writeSearchableIntegrationPDF(t)
	extractor := New(Options{
		PDFToTextPath: pdfToTextPath,
		TempDir:       t.TempDir(),
		OCRAllowed:    false,
	})

	got, err := extractor.Extract(context.Background(), inputPDF)
	if err != nil {
		t.Fatalf("Extract(searchable PDF with real pdftotext) error = %v, want nil", err)
	}
	if !strings.Contains(got.Text, "Searchable PDF text for Moth integration test") {
		t.Fatalf("Extract(searchable PDF with real pdftotext) text = %q, want fixture text", got.Text)
	}
	assertWarningsMissing(t, got.Warnings, content.WarningOCRUsed, content.WarningToolMissing)
}

func TestIntegrationExtractImagePDFWithOCRMyPDF(t *testing.T) {
	pdfToTextPath := requireIntegrationExecutable(t, "pdftotext", "-v")
	ocrMyPDFPath := requireIntegrationExecutable(t, "ocrmypdf", "--version")
	tesseractPath := requireIntegrationExecutable(t, "tesseract", "--version")
	requireTesseractLanguage(t, tesseractPath, "eng")
	inputPDF := writeImageOnlyIntegrationPDF(t)
	extractor := New(Options{
		PDFToTextPath: pdfToTextPath,
		OCRMyPDFPath:  ocrMyPDFPath,
		TesseractPath: tesseractPath,
		TempDir:       t.TempDir(),
		OCRAllowed:    true,
		OCRLanguage:   "eng",
		OCRMode:       "skip",
	})

	got, err := extractor.Extract(context.Background(), inputPDF)
	if err != nil {
		t.Fatalf("Extract(image PDF with real ocrmypdf) error = %v, want nil", err)
	}
	assertWarningsContain(t, got.Warnings, content.WarningOCRUsed)
}

func requireIntegrationExecutable(t *testing.T, name string, versionArgs ...string) string {
	t.Helper()

	path, err := exec.LookPath(name)
	if err != nil {
		t.Skipf("%s not found in PATH; skipping integration test", name)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	//nolint:gosec // Integration test intentionally executes the discovered local tool.
	cmd := exec.CommandContext(ctx, path, versionArgs...)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Skipf("%s version check failed: %v; output: %s", name, err, output)
	}

	return path
}

func requireTesseractLanguage(t *testing.T, tesseractPath string, language string) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	//nolint:gosec // Integration test intentionally queries the discovered local tesseract tool.
	cmd := exec.CommandContext(ctx, tesseractPath, "--list-langs")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Skipf("tesseract language check failed: %v; output: %s", err, output)
	}
	for _, line := range strings.Split(string(output), "\n") {
		if strings.TrimSpace(line) == language {
			return
		}
	}
	t.Skipf("tesseract language %q not installed; skipping integration test", language)
}

func writeSearchableIntegrationPDF(t *testing.T) string {
	t.Helper()

	textStream := []byte("BT /F1 24 Tf 72 720 Td (Searchable PDF text for Moth integration test.) Tj ET")
	return writeIntegrationPDF(t, []integrationPDFObject{
		{body: []byte("<< /Type /Catalog /Pages 2 0 R >>")},
		{body: []byte("<< /Type /Pages /Kids [3 0 R] /Count 1 >>")},
		{body: []byte("<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] " +
			"/Resources << /Font << /F1 4 0 R >> >> /Contents 5 0 R >>")},
		{body: []byte("<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>")},
		{body: streamObjectBody(textStream)},
	})
}

func writeImageOnlyIntegrationPDF(t *testing.T) string {
	t.Helper()

	const imageWidth = 200
	const imageHeight = 60
	imageData := bytes.Repeat([]byte{0xff}, imageWidth*imageHeight)
	for y := 20; y < 40; y++ {
		for x := 20; x < 180; x++ {
			if x%18 < 10 {
				imageData[y*imageWidth+x] = 0x00
			}
		}
	}
	imageStreamPrefix := fmt.Sprintf(
		"<< /Type /XObject /Subtype /Image /Width %d /Height %d /ColorSpace /DeviceGray "+
			"/BitsPerComponent 8 /Length %d >>\nstream\n",
		imageWidth,
		imageHeight,
		len(imageData),
	)
	imageBody := append([]byte(imageStreamPrefix), imageData...)
	imageBody = append(imageBody, []byte("\nendstream")...)
	pageStream := []byte("q 200 0 0 60 72 650 cm /Im1 Do Q")

	return writeIntegrationPDF(t, []integrationPDFObject{
		{body: []byte("<< /Type /Catalog /Pages 2 0 R >>")},
		{body: []byte("<< /Type /Pages /Kids [3 0 R] /Count 1 >>")},
		{body: []byte("<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] " +
			"/Resources << /XObject << /Im1 5 0 R >> >> /Contents 4 0 R >>")},
		{body: streamObjectBody(pageStream)},
		{body: imageBody},
	})
}

type integrationPDFObject struct {
	body []byte
}

func writeIntegrationPDF(t *testing.T, objects []integrationPDFObject) string {
	t.Helper()

	var data bytes.Buffer
	writePDFBytes(t, &data, []byte("%PDF-1.4\n"))
	offsets := make([]int, 0, len(objects))
	for index, object := range objects {
		offsets = append(offsets, data.Len())
		writePDFBytes(t, &data, []byte(fmt.Sprintf("%d 0 obj\n", index+1)))
		writePDFBytes(t, &data, object.body)
		writePDFBytes(t, &data, []byte("\nendobj\n"))
	}
	xrefOffset := data.Len()
	writePDFBytes(t, &data, []byte(fmt.Sprintf("xref\n0 %d\n", len(objects)+1)))
	writePDFBytes(t, &data, []byte("0000000000 65535 f \n"))
	for _, offset := range offsets {
		writePDFBytes(t, &data, []byte(fmt.Sprintf("%010d 00000 n \n", offset)))
	}
	writePDFBytes(t, &data, []byte(fmt.Sprintf(
		"trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n",
		len(objects)+1,
		xrefOffset,
	)))

	path := filepath.Join(t.TempDir(), "integration.pdf")
	if err := os.WriteFile(path, data.Bytes(), 0o600); err != nil {
		t.Fatalf("write integration PDF: %v", err)
	}

	return path
}

func streamObjectBody(stream []byte) []byte {
	body := []byte(fmt.Sprintf("<< /Length %d >>\nstream\n", len(stream)))
	body = append(body, stream...)
	body = append(body, []byte("\nendstream")...)

	return body
}

func writePDFBytes(t *testing.T, data *bytes.Buffer, value []byte) {
	t.Helper()

	if _, err := data.Write(value); err != nil {
		t.Fatalf("write PDF buffer: %v", err)
	}
}
