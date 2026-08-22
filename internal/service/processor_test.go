package service

import (
	"context"
	"errors"
	"testing"

	"api/internal/domain"
)

func equalSlices(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestTextProcessor_Process(t *testing.T) {
	tests := []struct {
		name          string
		content       string
		delimiter     string
		fileBase64    string
		expectedBytes int64
		expectedItems int64
		expectedList  []string
		expectedFile  *domain.FileMetadata
		expectErr     bool
	}{
		{
			name:          "Empty input (neither content nor file)",
			content:       "",
			delimiter:     "\n",
			fileBase64:    "",
			expectedBytes: 0,
			expectedItems: 0,
			expectedList:  []string{},
			expectedFile:  nil,
			expectErr:     false,
		},
		{
			name:          "Content only",
			content:       "hello\nworld",
			delimiter:     "\n",
			fileBase64:    "",
			expectedBytes: 11,
			expectedItems: 2,
			expectedList:  []string{"hello", "world"},
			expectedFile:  nil,
			expectErr:     false,
		},
		{
			name:          "File only (Text File in Base64)",
			content:       "",
			delimiter:     "",
			fileBase64:    "aGVsbG8gd29ybGQ=",
			expectedBytes: 0,
			expectedItems: 0,
			expectedList:  []string{},
			expectedFile: &domain.FileMetadata{
				SizeInBytes: 11,
				MimeType:    "text/plain; charset=utf-8",
				Status:      "decoded",
			},
			expectErr: false,
		},
		{
			name:          "File only (PDF-like bytes in Base64)",
			content:       "",
			delimiter:     "",
			fileBase64:    "JVBERi0xLjQK",
			expectedBytes: 0,
			expectedItems: 0,
			expectedList:  []string{},
			expectedFile: &domain.FileMetadata{
				SizeInBytes: 9,
				MimeType:    "application/pdf",
				Status:      "decoded",
			},
			expectErr: false,
		},
		{
			name:          "File only with Data URL prefix",
			content:       "",
			delimiter:     "",
			fileBase64:    "data:text/plain;base64,aGVsbG8=",
			expectedBytes: 0,
			expectedItems: 0,
			expectedList:  []string{},
			expectedFile: &domain.FileMetadata{
				SizeInBytes: 5,
				MimeType:    "text/plain; charset=utf-8",
				Status:      "decoded",
			},
			expectErr: false,
		},
		{
			name:          "Both content and file",
			content:       "apple,banana",
			delimiter:     ",",
			fileBase64:    "aGVsbG8=",
			expectedBytes: 12,
			expectedItems: 2,
			expectedList:  []string{"apple", "banana"},
			expectedFile: &domain.FileMetadata{
				SizeInBytes: 5,
				MimeType:    "text/plain; charset=utf-8",
				Status:      "decoded",
			},
			expectErr: false,
		},
		{
			name:          "Delimiter missing when content provided",
			content:       "hello",
			delimiter:     "",
			fileBase64:    "",
			expectedBytes: 0,
			expectedItems: 0,
			expectedList:  nil,
			expectedFile:  nil,
			expectErr:     true,
		},
		{
			name:          "Invalid base64 encoding",
			content:       "",
			delimiter:     "",
			fileBase64:    "invalid-base64!!",
			expectedBytes: 0,
			expectedItems: 0,
			expectedList:  nil,
			expectedFile:  nil,
			expectErr:     true,
		},
	}

	p := NewTextProcessor()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := &domain.ProcessRequest{
				Content:    tt.content,
				Delimiter:  tt.delimiter,
				FileBase64: tt.fileBase64,
			}
			res, err := p.Process(context.Background(), req)

			if (err != nil) != tt.expectErr {
				t.Fatalf("expected error: %v, got: %v", tt.expectErr, err)
			}

			if !tt.expectErr {
				if res.BytesProcessed != tt.expectedBytes {
					t.Errorf("expected bytes: %d, got: %d", tt.expectedBytes, res.BytesProcessed)
				}
				if res.ItemsProcessed != tt.expectedItems {
					t.Errorf("expected items: %d, got: %d", tt.expectedItems, res.ItemsProcessed)
				}
				if !equalSlices(res.Items, tt.expectedList) {
					t.Errorf("expected list: %v, got: %v", tt.expectedList, res.Items)
				}
				if tt.expectedFile != nil {
					if res.File == nil {
						t.Fatal("expected file metadata, got nil")
					}
					if res.File.SizeInBytes != tt.expectedFile.SizeInBytes {
						t.Errorf("expected file size: %d, got: %d", tt.expectedFile.SizeInBytes, res.File.SizeInBytes)
					}
					if res.File.MimeType != tt.expectedFile.MimeType {
						t.Errorf("expected file mime: %s, got: %s", tt.expectedFile.MimeType, res.File.MimeType)
					}
					if res.File.Status != tt.expectedFile.Status {
						t.Errorf("expected file status: %s, got: %s", tt.expectedFile.Status, res.File.Status)
					}
				} else if res.File != nil {
					t.Errorf("expected file metadata to be nil, got: %+v", res.File)
				}
				if res.Status != "success" {
					t.Errorf("expected status 'success', got: %s", res.Status)
				}
			}
		})
	}
}

func TestTextProcessor_ContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	p := NewTextProcessor()
	req := &domain.ProcessRequest{
		Content:   "hello",
		Delimiter: "\n",
	}

	_, err := p.Process(ctx, req)
	if err == nil {
		t.Fatal("expected error due to context cancellation, got nil")
	}

	if !errors.Is(err, context.Canceled) {
		t.Errorf("expected context.Canceled error, got: %v", err)
	}
}
