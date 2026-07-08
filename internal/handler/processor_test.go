package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"api/internal/domain"
)

type mockOrchestrator struct {
	clearFn         func(ctx context.Context) error
	populateFn      func(ctx context.Context, content, delimiter string) (int, error)
	populateAsyncFn func(ctx context.Context, content, delimiter string) (string, error)
	taskStatusFn    func(ctx context.Context, taskID string) (*domain.TaskStatus, error)
	matchFn         func(ctx context.Context, fileB64, fileMime, candidateEmail, candidatePhone string) (*domain.ProcessResult, error)
}

func (m *mockOrchestrator) ClearVacancies(ctx context.Context) error {
	if m.clearFn != nil {
		return m.clearFn(ctx)
	}
	return nil
}

func (m *mockOrchestrator) PopulateVacancies(ctx context.Context, content, delimiter string) (int, error) {
	if m.populateFn != nil {
		return m.populateFn(ctx, content, delimiter)
	}
	return 0, nil
}

func (m *mockOrchestrator) PopulateVacanciesAsync(ctx context.Context, content, delimiter string) (string, error) {
	if m.populateAsyncFn != nil {
		return m.populateAsyncFn(ctx, content, delimiter)
	}
	return "task_123", nil
}

func (m *mockOrchestrator) GetTaskStatus(ctx context.Context, taskID string) (*domain.TaskStatus, error) {
	if m.taskStatusFn != nil {
		return m.taskStatusFn(ctx, taskID)
	}
	return &domain.TaskStatus{ID: taskID, Status: "completed"}, nil
}

func (m *mockOrchestrator) MatchResume(ctx context.Context, fileB64, fileMime, candidateEmail, candidatePhone string) (*domain.ProcessResult, error) {
	if m.matchFn != nil {
		return m.matchFn(ctx, fileB64, fileMime, candidateEmail, candidatePhone)
	}
	return &domain.ProcessResult{Status: "success"}, nil
}

func TestProcessorHandler_Clear(t *testing.T) {
	t.Run("Successful clear", func(t *testing.T) {
		clearCalled := false
		mockOrch := &mockOrchestrator{
			clearFn: func(ctx context.Context) error {
				clearCalled = true
				return nil
			},
		}

		h := NewProcessorHandler(mockOrch)
		req := httptest.NewRequest("POST", "/api/v1/vacancies/clear", nil)
		w := httptest.NewRecorder()

		h.Clear(w, req)

		resp := w.Result()
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Errorf("expected status 200, got: %d", resp.StatusCode)
		}
		if !clearCalled {
			t.Error("expected ClearVacancies to be called on orchestrator")
		}
	})
}

func TestProcessorHandler_Populate(t *testing.T) {
	t.Run("Successful populate (async)", func(t *testing.T) {
		populateCalled := false
		mockOrch := &mockOrchestrator{
			populateAsyncFn: func(ctx context.Context, content, delimiter string) (string, error) {
				populateCalled = true
				return "task_123", nil
			},
		}

		h := NewProcessorHandler(mockOrch)
		jsonReq := `{"content": "vaga1---vaga2", "delimiter": "---"}`
		req := httptest.NewRequest("POST", "/api/v1/vacancies", strings.NewReader(jsonReq))
		w := httptest.NewRecorder()

		h.Populate(w, req)

		resp := w.Result()
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusAccepted {
			t.Errorf("expected status 202, got: %d", resp.StatusCode)
		}
		if !populateCalled {
			t.Error("expected PopulateVacanciesAsync to be called on orchestrator")
		}

		var res map[string]interface{}
		json.NewDecoder(resp.Body).Decode(&res)
		if res["status"] != "accepted" || res["task_id"] != "task_123" {
			t.Errorf("unexpected response content: %v", res)
		}
	})
}

func TestProcessorHandler_GetTaskStatus(t *testing.T) {
	t.Run("Successful get task status", func(t *testing.T) {
		statusCalled := false
		mockOrch := &mockOrchestrator{
			taskStatusFn: func(ctx context.Context, taskID string) (*domain.TaskStatus, error) {
				statusCalled = true
				if taskID != "task_999" {
					t.Errorf("expected task_999, got %s", taskID)
				}
				return &domain.TaskStatus{
					ID:             "task_999",
					Status:         "completed",
					ItemsProcessed: 3,
				}, nil
			},
		}

		h := NewProcessorHandler(mockOrch)
		req := httptest.NewRequest("GET", "/api/v1/tasks/task_999", nil)
		req.SetPathValue("id", "task_999")
		w := httptest.NewRecorder()

		h.GetTaskStatus(w, req)

		resp := w.Result()
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Errorf("expected status 200, got: %d", resp.StatusCode)
		}
		if !statusCalled {
			t.Error("expected GetTaskStatus to be called on orchestrator")
		}

		var res domain.TaskStatus
		json.NewDecoder(resp.Body).Decode(&res)
		if res.ID != "task_999" || res.Status != "completed" || res.ItemsProcessed != 3 {
			t.Errorf("unexpected task response: %+v", res)
		}
	})
}

func TestProcessorHandler_Match(t *testing.T) {
	t.Run("Successful match", func(t *testing.T) {
		matchCalled := false
		mockOrch := &mockOrchestrator{
			matchFn: func(ctx context.Context, fileB64, fileMime, candidateEmail, candidatePhone string) (*domain.ProcessResult, error) {
				matchCalled = true
				return &domain.ProcessResult{Status: "success"}, nil
			},
		}

		h := NewProcessorHandler(mockOrch)
		jsonReq := `{"file_base64": "aGVsbG8="}`
		req := httptest.NewRequest("POST", "/api/v1/match", strings.NewReader(jsonReq))
		w := httptest.NewRecorder()

		h.Match(w, req)

		resp := w.Result()
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Errorf("expected status 200, got: %d", resp.StatusCode)
		}
		if !matchCalled {
			t.Error("expected MatchResume to be called on orchestrator")
		}
	})
}
