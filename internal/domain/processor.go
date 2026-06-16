package domain

import (
	"context"
)

// ProcessRequest represents the input payload for processing.
type ProcessRequest struct {
	Content    string `json:"content"`
	Delimiter  string `json:"delimiter"`
	FileBase64 string `json:"file_base64"`
}

// FileMetadata holds the parsed metadata of the uploaded file.
type FileMetadata struct {
	SizeInBytes int64  `json:"size_in_bytes"`
	MimeType    string `json:"mime_type"`
	Status      string `json:"status"`
}

// ProcessResult represents the statistics of the processed text stream.
type ProcessResult struct {
	BytesProcessed int64         `json:"bytes_processed"`
	ItemsProcessed int64         `json:"items_processed"`
	Items          []string      `json:"items"`
	File           *FileMetadata `json:"file,omitempty"`
	Matches        []Match       `json:"matches,omitempty"` // matches detected by Gemini
	DurationMs     int64         `json:"duration_ms"`
	Status         string        `json:"status"`
}

// PayloadProcessor defines the domain contract for processing raw text payloads.
type PayloadProcessor interface {
	Process(ctx context.Context, req *ProcessRequest) (*ProcessResult, error)
}
