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

// mockProcessor implements domain.PayloadProcessor for testing handlers
type mockProcessor struct {
	processFn func(ctx context.Context, req *domain.ProcessRequest) (*domain.ProcessResult, error)
}

func (m *mockProcessor) Process(ctx context.Context, req *domain.ProcessRequest) (*domain.ProcessResult, error) {
	return m.processFn(ctx, req)
}

func TestProcessorHandler_Process(t *testing.T) {
	t.Run("Successful processing", func(t *testing.T) {
		mockProc := &mockProcessor{
			processFn: func(ctx context.Context, req *domain.ProcessRequest) (*domain.ProcessResult, error) {
				if req.Content != "test content" || req.Delimiter != "\n" {
					return nil, errors.New("unexpected request data")
				}
				return &domain.ProcessResult{
					BytesProcessed: 12,
					ItemsProcessed: 1,
					Items:          []string{"test content"},
					DurationMs:     5,
					Status:         "success",
				}, nil
			},
		}

		h := NewProcessorHandler(mockProc)

		jsonReq := `{"content": "test content", "delimiter": "\n"}`
		req := httptest.NewRequest("POST", "/api/v1/process", strings.NewReader(jsonReq))
		w := httptest.NewRecorder()

		h.Process(w, req)

		resp := w.Result()
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Errorf("expected status 200, got: %d", resp.StatusCode)
		}

		if cType := resp.Header.Get("Content-Type"); cType != "application/json" {
			t.Errorf("expected Content-Type application/json, got: %s", cType)
		}

		var result domain.ProcessResult
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}

		if result.BytesProcessed != 12 || result.ItemsProcessed != 1 || result.Status != "success" {
			t.Errorf("unexpected response content: %+v", result)
		}

		if len(result.Items) != 1 || result.Items[0] != "test content" {
			t.Errorf("unexpected items slice content: %v", result.Items)
		}
	})

	t.Run("Invalid JSON payload", func(t *testing.T) {
		h := NewProcessorHandler(&mockProcessor{})

		invalidReq := `{invalid json}`
		req := httptest.NewRequest("POST", "/api/v1/process", strings.NewReader(invalidReq))
		w := httptest.NewRecorder()

		h.Process(w, req)

		resp := w.Result()
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("expected status 400, got: %d", resp.StatusCode)
		}

		var errResp map[string]string
		if err := json.NewDecoder(resp.Body).Decode(&errResp); err != nil {
			t.Fatalf("failed to decode error response: %v", err)
		}

		if errResp["status"] != "error" || !strings.Contains(errResp["error"], "Invalid JSON payload") {
			t.Errorf("unexpected error response content: %+v", errResp)
		}
	})

	t.Run("Missing delimiter field", func(t *testing.T) {
		h := NewProcessorHandler(&mockProcessor{})

		missingDelimiter := `{"content": "some content"}`
		req := httptest.NewRequest("POST", "/api/v1/process", strings.NewReader(missingDelimiter))
		w := httptest.NewRecorder()

		h.Process(w, req)

		resp := w.Result()
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("expected status 400, got: %d", resp.StatusCode)
		}

		var errResp map[string]string
		if err := json.NewDecoder(resp.Body).Decode(&errResp); err != nil {
			t.Fatalf("failed to decode error response: %v", err)
		}

		if errResp["status"] != "error" || !strings.Contains(errResp["error"], "delimiter' is required") {
			t.Errorf("unexpected error response content: %+v", errResp)
		}
	})

	t.Run("Service returns error", func(t *testing.T) {
		mockProc := &mockProcessor{
			processFn: func(ctx context.Context, req *domain.ProcessRequest) (*domain.ProcessResult, error) {
				return nil, errors.New("internal logic failure")
			},
		}

		h := NewProcessorHandler(mockProc)

		jsonReq := `{"content": "test content", "delimiter": "\n"}`
		req := httptest.NewRequest("POST", "/api/v1/process", strings.NewReader(jsonReq))
		w := httptest.NewRecorder()

		h.Process(w, req)

		resp := w.Result()
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusInternalServerError {
			t.Errorf("expected status 500, got: %d", resp.StatusCode)
		}

		var errResp map[string]string
		if err := json.NewDecoder(resp.Body).Decode(&errResp); err != nil {
			t.Fatalf("failed to decode error response: %v", err)
		}

		if errResp["status"] != "error" || !strings.Contains(errResp["error"], "internal logic failure") {
			t.Errorf("unexpected error response content: %+v", errResp)
		}
	})
}
