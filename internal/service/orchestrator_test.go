package service

import (
	"context"
	"crypto/sha256"
	"fmt"
	"testing"

	"api/internal/domain"
)

type mockGemini struct {
	matchFn       func(ctx context.Context, fileB64 string, fileMime string, vacancies []string) (*domain.MatchResult, error)
	embedFn       func(ctx context.Context, texts []string) ([][]float32, error)
	extractTextFn func(ctx context.Context, fileB64 string, fileMime string) (string, error)
}

func (m *mockGemini) MatchResume(ctx context.Context, fileB64 string, fileMime string, vacancies []string) (*domain.MatchResult, error) {
	return m.matchFn(ctx, fileB64, fileMime, vacancies)
}

func (m *mockGemini) GetEmbeddings(ctx context.Context, texts []string) ([][]float32, error) {
	if m.embedFn != nil {
		return m.embedFn(ctx, texts)
	}
	res := make([][]float32, len(texts))
	for i := range res {
		res[i] = []float32{1.0}
	}
	return res, nil
}

func (m *mockGemini) ExtractText(ctx context.Context, fileB64 string, fileMime string) (string, error) {
	if m.extractTextFn != nil {
		return m.extractTextFn(ctx, fileB64, fileMime)
	}
	return "currículo texto", nil
}

type mockVectorStore struct {
	clearFn            func(ctx context.Context) error
	hasVacancyFn       func(ctx context.Context, id string) (bool, error)
	addVacanciesFn     func(ctx context.Context, vacancies []string, embeddings [][]float32) error
	searchSimilarityFn func(ctx context.Context, queryEmbedding []float32, limit int, threshold float32) ([]domain.Vacancy, error)
}

func (m *mockVectorStore) Clear(ctx context.Context) error {
	if m.clearFn != nil {
		return m.clearFn(ctx)
	}
	return nil
}

func (m *mockVectorStore) HasVacancy(ctx context.Context, id string) (bool, error) {
	if m.hasVacancyFn != nil {
		return m.hasVacancyFn(ctx, id)
	}
	return false, nil
}

func (m *mockVectorStore) AddVacancies(ctx context.Context, vacancies []string, embeddings [][]float32) error {
	if m.addVacanciesFn != nil {
		return m.addVacanciesFn(ctx, vacancies, embeddings)
	}
	return nil
}

func (m *mockVectorStore) SearchSimilarity(ctx context.Context, queryEmbedding []float32, limit int, threshold float32) ([]domain.Vacancy, error) {
	if m.searchSimilarityFn != nil {
		return m.searchSimilarityFn(ctx, queryEmbedding, limit, threshold)
	}
	return []domain.Vacancy{
		{Index: 0, Text: "vaga 1"},
		{Index: 1, Text: "vaga 2"},
	}, nil
}

type mockEmail struct {
	sendFn func(ctx context.Context, to string, subject string, body string, attachmentB64 string, attachmentName string) error
}

func (m *mockEmail) SendEmail(ctx context.Context, to string, subject string, body string, attachmentB64 string, attachmentName string) error {
	return m.sendFn(ctx, to, subject, body, attachmentB64, attachmentName)
}

type mockWhatsApp struct {
	sendFn func(ctx context.Context, to string, message string) error
}

func (m *mockWhatsApp) SendMessage(ctx context.Context, to string, message string) error {
	return m.sendFn(ctx, to, message)
}

func TestOrchestrator_Methods(t *testing.T) {
	t.Run("ClearVacancies delegates to store", func(t *testing.T) {
		clearCalled := false
		mStore := &mockVectorStore{
			clearFn: func(ctx context.Context) error {
				clearCalled = true
				return nil
			},
		}

		orch := NewOrchestrator(&mockGemini{}, &mockGemini{}, mStore, &mockEmail{}, &mockWhatsApp{})
		err := orch.ClearVacancies(context.Background())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !clearCalled {
			t.Error("expected Clear to be called on store")
		}
	})

	t.Run("PopulateVacancies splits, embeds and saves", func(t *testing.T) {
		addCalled := false
		mStore := &mockVectorStore{
			addVacanciesFn: func(ctx context.Context, vacancies []string, embeddings [][]float32) error {
				addCalled = true
				if len(vacancies) != 2 || vacancies[0] != "vaga 1" || vacancies[1] != "vaga 2" {
					t.Errorf("unexpected vacancies passed to store: %v", vacancies)
				}
				return nil
			},
		}

		orch := NewOrchestrator(&mockGemini{}, &mockGemini{}, mStore, &mockEmail{}, &mockWhatsApp{})
		count, err := orch.PopulateVacancies(context.Background(), "vaga 1---vaga 2", "---")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if count != 2 {
			t.Errorf("expected count 2, got: %d", count)
		}
		if !addCalled {
			t.Error("expected AddVacancies to be called on store")
		}
	})

	t.Run("PopulateVacancies skips existing vacancies", func(t *testing.T) {
		addCalled := false
		mStore := &mockVectorStore{
			hasVacancyFn: func(ctx context.Context, id string) (bool, error) {
				expectedHash := sha256.Sum256([]byte("vaga 1"))
				expectedID := fmt.Sprintf("vac_%x", expectedHash)
				if id == expectedID {
					return true, nil
				}
				return false, nil
			},
			addVacanciesFn: func(ctx context.Context, vacancies []string, embeddings [][]float32) error {
				addCalled = true
				if len(vacancies) != 1 || vacancies[0] != "vaga 2" {
					t.Errorf("expected only 'vaga 2' to be added, got: %v", vacancies)
				}
				return nil
			},
		}

		orch := NewOrchestrator(&mockGemini{}, &mockGemini{}, mStore, &mockEmail{}, &mockWhatsApp{})
		count, err := orch.PopulateVacancies(context.Background(), "vaga 1---vaga 2", "---")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if count != 1 {
			t.Errorf("expected count 1, got: %d", count)
		}
		if !addCalled {
			t.Error("expected AddVacancies to be called on store")
		}
	})

	t.Run("MatchResume extracts text, embeds query, queries similarity, matches LLM and dispatches", func(t *testing.T) {
		mGemini := &mockGemini{
			matchFn: func(ctx context.Context, fileB64 string, fileMime string, vacancies []string) (*domain.MatchResult, error) {
				return &domain.MatchResult{
					Matches: []domain.Match{
						{Index: 0, Reason: "perfeito", ContactType: "email", ContactTarget: "rh@empresa.com"},
						{Index: 1, Reason: "bom", ContactType: "whatsapp", ContactTarget: "5511999999999"},
					},
				}, nil
			},
		}

		emailCalls := 0
		mEmail := &mockEmail{
			sendFn: func(ctx context.Context, to string, subject string, body string, attachmentB64 string, attachmentName string) error {
				emailCalls++
				if to != "rh@empresa.com" || attachmentB64 != "aGVsbG8=" {
					t.Errorf("unexpected email call parameters: to=%s, b64=%s", to, attachmentB64)
				}
				return nil
			},
		}

		waCalls := 0
		mWhatsApp := &mockWhatsApp{
			sendFn: func(ctx context.Context, to string, message string) error {
				waCalls++
				if to != "5511999999999" {
					t.Errorf("unexpected whatsapp call parameters: to=%s", to)
				}
				return nil
			},
		}

		orch := NewOrchestrator(mGemini, mGemini, &mockVectorStore{}, mEmail, mWhatsApp)
		res, err := orch.MatchResume(context.Background(), "aGVsbG8=", "application/pdf")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(res.Matches) != 2 {
			t.Errorf("expected 2 matches, got: %d", len(res.Matches))
		}
		if emailCalls != 1 {
			t.Errorf("expected 1 email call, got: %d", emailCalls)
		}
		if waCalls != 1 {
			t.Errorf("expected 1 whatsapp call, got: %d", waCalls)
		}
	})
}
