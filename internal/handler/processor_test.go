package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"api/internal/domain"
)

// mockOrchestrator implements domain.Orchestrator for testing handlers
type mockOrchestrator struct {
	runFn func(ctx context.Context, req *domain.ProcessRequest) (*domain.ProcessResult, error)
}

func (m *mockOrchestrator) RunMatchAndDispatch(ctx context.Context, req *domain.ProcessRequest) (*domain.ProcessResult, error) {
	return m.runFn(ctx, req)
}

func TestProcessorHandler_Process(t *testing.T) {
	t.Run("Successful processing with content only", func(t *testing.T) {
		mockOrch := &mockOrchestrator{
			runFn: func(ctx context.Context, req *domain.ProcessRequest) (*domain.ProcessResult, error) {
				return &domain.ProcessResult{
					BytesProcessed: 12,
					ItemsProcessed: 1,
					Items:          []string{"test content"},
					Status:         "success",
				}, nil
			},
		}

		h := NewProcessorHandler(mockOrch)
		jsonReq := `{"content": "test content", "delimiter": "\n"}`
		req := httptest.NewRequest("POST", "/api/v1/process", strings.NewReader(jsonReq))
		w := httptest.NewRecorder()

		h.Process(w, req)

		resp := w.Result()
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Errorf("expected status 200, got: %d", resp.StatusCode)
		}
	})

	t.Run("Successful processing with file only", func(t *testing.T) {
		mockOrch := &mockOrchestrator{
			runFn: func(ctx context.Context, req *domain.ProcessRequest) (*domain.ProcessResult, error) {
				return &domain.ProcessResult{
					BytesProcessed: 0,
					ItemsProcessed: 0,
					Items:          []string{},
					File: &domain.FileMetadata{
						SizeInBytes: 11,
						MimeType:    "text/plain",
						Status:      "decoded",
					},
					Status: "success",
				}, nil
			},
		}

		h := NewProcessorHandler(mockOrch)
		jsonReq := `{"file_base64": "aGVsbG8gd29ybGQ="}`
		req := httptest.NewRequest("POST", "/api/v1/process", strings.NewReader(jsonReq))
		w := httptest.NewRecorder()

		h.Process(w, req)

		resp := w.Result()
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Errorf("expected status 200, got: %d", resp.StatusCode)
		}

		var result domain.ProcessResult
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}

		if result.File == nil || result.File.MimeType != "text/plain" {
			t.Errorf("unexpected file metadata in response: %+v", result.File)
		}
	})

	t.Run("Validation fails - both fields empty", func(t *testing.T) {
		h := NewProcessorHandler(&mockOrchestrator{})
		jsonReq := `{"delimiter": "\n"}`
		req := httptest.NewRequest("POST", "/api/v1/process", strings.NewReader(jsonReq))
		w := httptest.NewRecorder()

		h.Process(w, req)

		resp := w.Result()
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("expected status 400, got: %d", resp.StatusCode)
		}

		var errResp map[string]string
		json.NewDecoder(resp.Body).Decode(&errResp)
		if !strings.Contains(errResp["error"], "Either 'content' or 'file_base64' must be provided") {
			t.Errorf("unexpected error message: %s", errResp["error"])
		}
	})

	t.Run("Validation fails - content sent but delimiter missing", func(t *testing.T) {
		h := NewProcessorHandler(&mockOrchestrator{})
		jsonReq := `{"content": "hello"}`
		req := httptest.NewRequest("POST", "/api/v1/process", strings.NewReader(jsonReq))
		w := httptest.NewRecorder()

		h.Process(w, req)

		resp := w.Result()
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("expected status 400, got: %d", resp.StatusCode)
		}

		var errResp map[string]string
		json.NewDecoder(resp.Body).Decode(&errResp)
		if !strings.Contains(errResp["error"], "Field 'delimiter' is required") {
			t.Errorf("unexpected error message: %s", errResp["error"])
		}
	})

	t.Run("Service returns bad base64 encoding error", func(t *testing.T) {
		mockOrch := &mockOrchestrator{
			runFn: func(ctx context.Context, req *domain.ProcessRequest) (*domain.ProcessResult, error) {
				return nil, errors.New("invalid base64 encoding")
			},
		}

		h := NewProcessorHandler(mockOrch)
		jsonReq := `{"file_base64": "invalid-base64!!"}`
		req := httptest.NewRequest("POST", "/api/v1/process", strings.NewReader(jsonReq))
		w := httptest.NewRecorder()

		h.Process(w, req)

		resp := w.Result()
		defer resp.Body.Close()

		// Handler is expected to catch "invalid base64 encoding" and return 400
		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("expected status 400, got: %d", resp.StatusCode)
		}
	})
}
