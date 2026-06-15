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
		expectedBytes int64
		expectedItems int64
		expectedList  []string
		expectErr     bool
	}{
		{
			name:          "Empty input",
			content:       "",
			delimiter:     "\n",
			expectedBytes: 0,
			expectedItems: 0,
			expectedList:  []string{},
			expectErr:     false,
		},
		{
			name:          "Single line without trailing newline",
			content:       "hello world",
			delimiter:     "\n",
			expectedBytes: 11,
			expectedItems: 1,
			expectedList:  []string{"hello world"},
			expectErr:     false,
		},
		{
			name:          "Single line with trailing newline",
			content:       "hello world\n",
			delimiter:     "\n",
			expectedBytes: 12,
			expectedItems: 1,
			expectedList:  []string{"hello world"},
			expectErr:     false,
		},
		{
			name:          "Multiple lines",
			content:       "line 1\nline 2\nline 3",
			delimiter:     "\n",
			expectedBytes: 20,
			expectedItems: 3,
			expectedList:  []string{"line 1", "line 2", "line 3"},
			expectErr:     false,
		},
		{
			name:          "Multiple lines with trailing newline",
			content:       "line 1\nline 2\nline 3\n",
			delimiter:     "\n",
			expectedBytes: 21,
			expectedItems: 3,
			expectedList:  []string{"line 1", "line 2", "line 3"},
			expectErr:     false,
		},
		{
			name:          "Comma delimiter",
			content:       "apple,banana,cherry",
			delimiter:     ",",
			expectedBytes: 19,
			expectedItems: 3,
			expectedList:  []string{"apple", "banana", "cherry"},
			expectErr:     false,
		},
		{
			name:          "Comma delimiter with trailing comma",
			content:       "apple,banana,cherry,",
			delimiter:     ",",
			expectedBytes: 20,
			expectedItems: 3,
			expectedList:  []string{"apple", "banana", "cherry"},
			expectErr:     false,
		},
		{
			name:          "Empty delimiter returns error",
			content:       "some content",
			delimiter:     "",
			expectedBytes: 0,
			expectedItems: 0,
			expectedList:  nil,
			expectErr:     true,
		},
	}

	p := NewTextProcessor()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := &domain.ProcessRequest{
				Content:   tt.content,
				Delimiter: tt.delimiter,
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
				if res.Status != "success" {
					t.Errorf("expected status 'success', got: %s", res.Status)
				}
			}
		})
	}
}

func TestTextProcessor_ContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

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
