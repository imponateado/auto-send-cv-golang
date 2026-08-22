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
	populateTextsFn func(ctx context.Context, texts []string) (int, error)
	listFn          func(ctx context.Context) ([]domain.Vacancy, error)
	matchFn         func(ctx context.Context, fileB64, fileMime, candidateEmail, candidatePhone string) (*domain.ProcessResult, error)
}

func (m *mockOrchestrator) ClearVacancies(ctx context.Context) error {
	if m.clearFn != nil {
		return m.clearFn(ctx)
	}
	return nil
}

func (m *mockOrchestrator) PopulateVacancyTexts(ctx context.Context, texts []string) (int, error) {
	if m.populateTextsFn != nil {
		return m.populateTextsFn(ctx, texts)
	}
	return 0, nil
}

func (m *mockOrchestrator) ListVacancies(ctx context.Context) ([]domain.Vacancy, error) {
	if m.listFn != nil {
		return m.listFn(ctx)
	}
	return nil, nil
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

func TestProcessorHandler_List(t *testing.T) {
	t.Run("Successful list", func(t *testing.T) {
		listCalled := false
		mockOrch := &mockOrchestrator{
			listFn: func(ctx context.Context) ([]domain.Vacancy, error) {
				listCalled = true
				return []domain.Vacancy{{Index: 0, Text: "vaga 1"}}, nil
			},
		}

		h := NewProcessorHandler(mockOrch)
		req := httptest.NewRequest("GET", "/api/v1/vacancies", nil)
		w := httptest.NewRecorder()

		h.List(w, req)

		resp := w.Result()
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Errorf("expected status 200, got: %d", resp.StatusCode)
		}
		if !listCalled {
			t.Error("expected ListVacancies to be called on orchestrator")
		}

		var res []domain.Vacancy
		json.NewDecoder(resp.Body).Decode(&res)
		if len(res) != 1 || res[0].Text != "vaga 1" {
			t.Errorf("unexpected list response: %+v", res)
		}
	})

	t.Run("Failure returns 500", func(t *testing.T) {
		mockOrch := &mockOrchestrator{
			listFn: func(ctx context.Context) ([]domain.Vacancy, error) {
				return nil, context.DeadlineExceeded
			},
		}

		h := NewProcessorHandler(mockOrch)
		req := httptest.NewRequest("GET", "/api/v1/vacancies", nil)
		w := httptest.NewRecorder()

		h.List(w, req)

		resp := w.Result()
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusInternalServerError {
			t.Errorf("expected status 500, got: %d", resp.StatusCode)
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
