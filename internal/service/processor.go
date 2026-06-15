package service

import (
	"context"
	"encoding/base64"
	"errors"
	"net/http"
	"strings"
	"time"

	"api/internal/domain"
)

type textProcessor struct{}

// NewTextProcessor creates a new instance of the text payload processor.
func NewTextProcessor() domain.PayloadProcessor {
	return &textProcessor{}
}

// Process processes the text content and optional base64 file.
func (p *textProcessor) Process(ctx context.Context, req *domain.ProcessRequest) (*domain.ProcessResult, error) {
	start := time.Now()

	if req == nil {
		return nil, errors.New("request cannot be nil")
	}

	// Respect context cancellation
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	var bytesProcessed int64
	var itemsProcessed int64
	var items []string

	// Process Content if present
	if req.Content != "" {
		if req.Delimiter == "" {
			return nil, errors.New("delimiter cannot be empty when content is provided")
		}

		bytesProcessed = int64(len(req.Content))
		items = strings.Split(req.Content, req.Delimiter)

		// Trim trailing empty element caused by delimiter
		if len(items) > 0 && items[len(items)-1] == "" {
			items = items[:len(items)-1]
		}
		itemsProcessed = int64(len(items))
	} else {
		// Initialize items to empty slice instead of nil
		items = []string{}
	}

	// Process FileBase64 if present
	var fileMetadata *domain.FileMetadata
	if req.FileBase64 != "" {
		// Clean Data URI prefix if present (e.g. "data:application/pdf;base64,")
		base64Data := req.FileBase64
		if idx := strings.Index(base64Data, ","); idx != -1 {
			base64Data = base64Data[idx+1:]
		}

		// Strip any whitespace/newlines
		base64Data = strings.Join(strings.Fields(base64Data), "")

		decodedBytes, err := base64.StdEncoding.DecodeString(base64Data)
		if err != nil {
			return nil, errors.New("invalid base64 encoding")
		}

		size := int64(len(decodedBytes))

		// Detect Content-Type (MIME Type)
		mimeType := http.DetectContentType(decodedBytes)

		fileMetadata = &domain.FileMetadata{
			SizeInBytes: size,
			MimeType:    mimeType,
			Status:      "decoded",
		}
	}

	return &domain.ProcessResult{
		BytesProcessed: bytesProcessed,
		ItemsProcessed: itemsProcessed,
		Items:          items,
		File:           fileMetadata,
		DurationMs:     time.Since(start).Milliseconds(),
		Status:         "success",
	}, nil
}
