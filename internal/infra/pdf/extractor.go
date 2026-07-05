package pdf

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"strings"

	"github.com/ledongthuc/pdf"
)

// ExtractTextFromBase64 decodes a base64 string and extracts its text contents.
// If the MIME type is PDF, it parses it to extract pages. Otherwise it attempts to return it as a string.
func ExtractTextFromBase64(base64Str string, mimeType string) (string, error) {
	// Clean base64 string
	if idx := strings.Index(base64Str, ","); idx != -1 {
		base64Str = base64Str[idx+1:]
	}
	base64Str = strings.Join(strings.Fields(base64Str), "")

	decodedBytes, err := base64.StdEncoding.DecodeString(base64Str)
	if err != nil {
		return "", fmt.Errorf("failed to decode base64: %w", err)
	}

	// For plain text, decode directly
	if strings.Contains(mimeType, "text/") || mimeType == "application/octet-stream" {
		return string(decodedBytes), nil
	}

	// Default to PDF parsing
	readerAt := bytes.NewReader(decodedBytes)
	size := int64(len(decodedBytes))

	pdfReader, err := pdf.NewReader(readerAt, size)
	if err != nil {
		return "", fmt.Errorf("failed to parse PDF document: %w", err)
	}

	var textBuilder strings.Builder
	numPages := pdfReader.NumPage()
	for i := 1; i <= numPages; i++ {
		page := pdfReader.Page(i)
		if page.V.IsNull() {
			continue
		}
		text, err := page.GetPlainText(nil)
		if err != nil {
			// Some pages might fail or be empty, continue to extract other pages
			continue
		}
		textBuilder.WriteString(text)
		textBuilder.WriteString("\n")
	}

	return textBuilder.String(), nil
}
