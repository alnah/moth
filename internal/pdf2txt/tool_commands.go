package pdf2txt

import (
	"context"
	"fmt"

	"github.com/alnah/moth/internal/tools"
)

func (extractor *Extractor) runPDFToText(ctx context.Context, inputPDF string, outputPath string) (string, error) {
	path, err := extractor.toolPath(ctx, tools.ToolPDFToText, extractor.pdfToTextPath, "PDFTOTEXT_PATH")
	if err != nil {
		return "", err
	}
	_, err = extractor.runner.Run(ctx, extractor.pdfToTextCommand(path, inputPDF, outputPath))
	if err != nil {
		return "", fmt.Errorf("run pdftotext: %w", err)
	}

	return readTextFile(outputPath, extractor.maxTextBytes)
}

func (extractor *Extractor) runOCRMyPDF(ctx context.Context, inputPDF string, outputPDF string) error {
	_, err := extractor.toolPath(ctx, tools.ToolTesseract, extractor.tesseractPath, "TESSERACT_PATH")
	if err != nil {
		return err
	}

	path, err := extractor.toolPath(ctx, tools.ToolOCRMyPDF, extractor.ocrMyPDFPath, "OCRMYPDF_PATH")
	if err != nil {
		return err
	}
	_, err = extractor.runner.Run(ctx, extractor.ocrMyPDFCommand(path, inputPDF, outputPDF))
	if err != nil {
		return fmt.Errorf("run ocrmypdf: %w", err)
	}

	return nil
}

func (extractor *Extractor) pdfToTextCommand(path string, inputPDF string, outputPath string) tools.Command {
	return tools.Command{
		Tool:             tools.ToolPDFToText,
		Path:             path,
		Args:             []string{"-layout", inputPDF, outputPath},
		StdoutLimitBytes: extractor.maxTextBytes,
		StderrLimitBytes: extractor.maxTextBytes,
	}
}

func (extractor *Extractor) ocrMyPDFCommand(path string, inputPDF string, outputPDF string) tools.Command {
	return tools.Command{
		Tool: tools.ToolOCRMyPDF,
		Path: path,
		Args: []string{
			"-l", extractor.ocrLanguage,
			"--mode", extractor.ocrMode,
			"--output-type", "pdf",
			"--optimize", "0",
			inputPDF,
			outputPDF,
		},
		StdoutLimitBytes: extractor.maxTextBytes,
		StderrLimitBytes: extractor.maxTextBytes,
	}
}

func (extractor *Extractor) toolPath(
	ctx context.Context,
	name tools.ToolName,
	explicitPath string,
	envVar string,
) (string, error) {
	if explicitPath != "" {
		return explicitPath, nil
	}
	resolved, err := tools.Resolve(ctx, tools.ResolveOptions{
		Name:     name,
		EnvVar:   envVar,
		ToolsDir: extractor.toolsDir,
	})
	if err != nil {
		return "", err
	}

	return resolved.Path, nil
}
