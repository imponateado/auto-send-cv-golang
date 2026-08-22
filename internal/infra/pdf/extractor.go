package pdf

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"strings"

	"github.com/ledongthuc/pdf"
)

// ExtractTextFromBase64 decodes base64Str and, if mimeType is a PDF, parses its
// pages into plain text; for text/* or octet-stream it returns the decoded bytes
// as-is. Returns the extracted text, or an error if decoding or PDF parsing
// fails.
func ExtractTextFromBase64(base64Str string, mimeType string) (string, error) {
	if idx := strings.Index(base64Str, ","); idx != -1 {
		base64Str = base64Str[idx+1:]
	}
	base64Str = strings.Join(strings.Fields(base64Str), "")

	decodedBytes, err := base64.StdEncoding.DecodeString(base64Str)
	if err != nil {
		return "", fmt.Errorf("failed to decode base64: %w", err)
	}

	if strings.Contains(mimeType, "text/") || mimeType == "application/octet-stream" {
		return string(decodedBytes), nil
	}

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
			continue
		}
		textBuilder.WriteString(text)
		textBuilder.WriteString("\n")
	}

	return textBuilder.String(), nil
}
