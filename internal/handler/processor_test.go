package handler

import (
	"context"
	"encoding/json"
	"fmt"
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
	deleteVacancyFn func(ctx context.Context, id string) error
	matchFn         func(ctx context.Context, fileB64, fileMime, candidateEmail, candidatePhone string) (*domain.ProcessResult, error)
	listMatchesFn   func(ctx context.Context) ([]*domain.MatchRecord, error)
	getMatchFn      func(ctx context.Context, id string) (*domain.MatchRecord, error)
	deleteMatchFn   func(ctx context.Context, id string) error
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

func (m *mockOrchestrator) DeleteVacancy(ctx context.Context, id string) error {
	if m.deleteVacancyFn != nil {
		return m.deleteVacancyFn(ctx, id)
	}
	return nil
}

func (m *mockOrchestrator) MatchResume(ctx context.Context, fileB64, fileMime, candidateEmail, candidatePhone string) (*domain.ProcessResult, error) {
	if m.matchFn != nil {
		return m.matchFn(ctx, fileB64, fileMime, candidateEmail, candidatePhone)
	}
	return &domain.ProcessResult{Status: "success"}, nil
}

func (m *mockOrchestrator) ListMatches(ctx context.Context) ([]*domain.MatchRecord, error) {
	if m.listMatchesFn != nil {
		return m.listMatchesFn(ctx)
	}
	return nil, nil
}

func (m *mockOrchestrator) GetMatch(ctx context.Context, id string) (*domain.MatchRecord, error) {
	if m.getMatchFn != nil {
		return m.getMatchFn(ctx, id)
	}
	return nil, nil
}

func (m *mockOrchestrator) DeleteMatch(ctx context.Context, id string) error {
	if m.deleteMatchFn != nil {
		return m.deleteMatchFn(ctx, id)
	}
	return nil
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

func TestProcessorHandler_DeleteVacancy(t *testing.T) {
	t.Run("Successful delete", func(t *testing.T) {
		var gotID string
		mockOrch := &mockOrchestrator{
			deleteVacancyFn: func(ctx context.Context, id string) error {
				gotID = id
				return nil
			},
		}

		h := NewProcessorHandler(mockOrch)
		req := httptest.NewRequest("DELETE", "/api/v1/vacancies/vac_abc123", nil)
		req.SetPathValue("id", "vac_abc123")
		w := httptest.NewRecorder()

		h.DeleteVacancy(w, req)

		resp := w.Result()
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Errorf("expected status 200, got: %d", resp.StatusCode)
		}
		if gotID != "vac_abc123" {
			t.Errorf("expected orchestrator to receive id 'vac_abc123', got: %q", gotID)
		}
	})

	t.Run("Missing id returns 400", func(t *testing.T) {
		h := NewProcessorHandler(&mockOrchestrator{})
		req := httptest.NewRequest("DELETE", "/api/v1/vacancies/", nil)
		req.SetPathValue("id", "")
		w := httptest.NewRecorder()

		h.DeleteVacancy(w, req)

		resp := w.Result()
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("expected status 400, got: %d", resp.StatusCode)
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

func TestProcessorHandler_Matches(t *testing.T) {
	t.Run("ListMatches success", func(t *testing.T) {
		mockOrch := &mockOrchestrator{
			listMatchesFn: func(ctx context.Context) ([]*domain.MatchRecord, error) {
				return []*domain.MatchRecord{{ID: "match_1", Result: &domain.ProcessResult{Status: "success"}}}, nil
			},
		}

		h := NewProcessorHandler(mockOrch)
		req := httptest.NewRequest("GET", "/api/v1/matches", nil)
		w := httptest.NewRecorder()

		h.ListMatches(w, req)

		resp := w.Result()
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Errorf("expected status 200, got: %d", resp.StatusCode)
		}

		var res []domain.MatchRecord
		json.NewDecoder(resp.Body).Decode(&res)
		if len(res) != 1 || res[0].ID != "match_1" {
			t.Errorf("unexpected list response: %+v", res)
		}
	})

	t.Run("GetMatchRecord not found returns 404", func(t *testing.T) {
		mockOrch := &mockOrchestrator{
			getMatchFn: func(ctx context.Context, id string) (*domain.MatchRecord, error) {
				return nil, fmt.Errorf("match %s not found", id)
			},
		}

		h := NewProcessorHandler(mockOrch)
		req := httptest.NewRequest("GET", "/api/v1/matches/missing", nil)
		req.SetPathValue("id", "missing")
		w := httptest.NewRecorder()

		h.GetMatchRecord(w, req)

		resp := w.Result()
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusNotFound {
			t.Errorf("expected status 404, got: %d", resp.StatusCode)
		}
	})

	t.Run("DeleteMatch success", func(t *testing.T) {
		var gotID string
		mockOrch := &mockOrchestrator{
			deleteMatchFn: func(ctx context.Context, id string) error {
				gotID = id
				return nil
			},
		}

		h := NewProcessorHandler(mockOrch)
		req := httptest.NewRequest("DELETE", "/api/v1/matches/match_1", nil)
		req.SetPathValue("id", "match_1")
		w := httptest.NewRecorder()

		h.DeleteMatch(w, req)

		resp := w.Result()
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Errorf("expected status 200, got: %d", resp.StatusCode)
		}
		if gotID != "match_1" {
			t.Errorf("expected orchestrator to receive id 'match_1', got: %q", gotID)
		}
	})
}
