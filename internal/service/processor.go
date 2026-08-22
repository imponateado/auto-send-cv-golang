package service

import (
	"context"
	"encoding/base64"
	"errors"
	"log"
	"net/http"
	"strings"
	"time"

	"api/internal/domain"
)

type textProcessor struct{}

// NewTextProcessor returns a domain.PayloadProcessor backed by a *textProcessor.
func NewTextProcessor() domain.PayloadProcessor {
	return &textProcessor{}
}

// Process splits req.Content on req.Delimiter into items and, if req.FileBase64 is
// set, decodes it and fills in file metadata. Returns the populated
// *domain.ProcessResult, or an error if req is nil, the delimiter is missing
// while content is present, the base64 payload is invalid, or ctx is cancelled.
func (p *textProcessor) Process(ctx context.Context, req *domain.ProcessRequest) (*domain.ProcessResult, error) {
	start := time.Now()
	if req == nil {
		log.Println("[Processor] Error: request is nil")
		return nil, errors.New("request cannot be nil")
	}

	log.Printf("[Processor] Received request: content_len=%d, file_base64_len=%d", len(req.Content), len(req.FileBase64))

	select {
	case <-ctx.Done():
		log.Println("[Processor] Request cancelled via context")
		return nil, ctx.Err()
	default:
	}

	var bytesProcessed int64
	var itemsProcessed int64
	var items []string

	if req.Content != "" {
		if req.Delimiter == "" {
			log.Println("[Processor] Error: Content provided but delimiter is empty")
			return nil, errors.New("delimiter cannot be empty when content is provided")
		}

		bytesProcessed = int64(len(req.Content))
		log.Printf("[Processor] Splitting content of size %d bytes using delimiter %q...", bytesProcessed, req.Delimiter)
		items = strings.Split(req.Content, req.Delimiter)

		if len(items) > 0 && items[len(items)-1] == "" {
			items = items[:len(items)-1]
		}
		itemsProcessed = int64(len(items))
		log.Printf("[Processor] Successfully extracted %d vacancies from text content", itemsProcessed)
	} else {
		items = []string{}
	}

	var fileMetadata *domain.FileMetadata
	if req.FileBase64 != "" {
		log.Println("[Processor] Cleaning and decoding base64 resume document...")
		base64Data := req.FileBase64
		if idx := strings.Index(base64Data, ","); idx != -1 {
			base64Data = base64Data[idx+1:]
		}

		base64Data = strings.Join(strings.Fields(base64Data), "")
		decodedBytes, err := base64.StdEncoding.DecodeString(base64Data)
		if err != nil {
			log.Printf("[Processor] Error decoding base64: %v", err)
			return nil, errors.New("invalid base64 encoding")
		}

		size := int64(len(decodedBytes))
		mimeType := http.DetectContentType(decodedBytes)
		log.Printf("[Processor] Decoded document: size=%d bytes, detected MIME type=%s", size, mimeType)

		fileMetadata = &domain.FileMetadata{
			SizeInBytes: size,
			MimeType:    mimeType,
			Status:      "decoded",
		}
	}

	duration := time.Since(start)
	log.Printf("[Processor] Payload processing finished in %s. Status: success", duration)

	return &domain.ProcessResult{
		BytesProcessed: bytesProcessed,
		ItemsProcessed: itemsProcessed,
		Items:          items,
		File:           fileMetadata,
		DurationMs:     duration.Milliseconds(),
		Status:         "success",
	}, nil
}
