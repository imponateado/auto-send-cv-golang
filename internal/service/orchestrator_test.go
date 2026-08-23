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
	existingIDsFn      func(ctx context.Context, ids []string) (map[string]bool, error)
	addVacanciesFn     func(ctx context.Context, vacancies []string, embeddings [][]float32) error
	searchSimilarityFn func(ctx context.Context, queryEmbedding []float32, limit int, threshold float32) ([]domain.Vacancy, error)
	listVacanciesFn    func(ctx context.Context) ([]domain.Vacancy, error)
	deleteVacancyFn    func(ctx context.Context, id string) error
}

func (m *mockVectorStore) Clear(ctx context.Context) error {
	if m.clearFn != nil {
		return m.clearFn(ctx)
	}
	return nil
}

func (m *mockVectorStore) ExistingVacancyIDs(ctx context.Context, ids []string) (map[string]bool, error) {
	if m.existingIDsFn != nil {
		return m.existingIDsFn(ctx, ids)
	}
	return map[string]bool{}, nil
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
		{Index: 0, Text: "vaga 1", ID: "vac_aaa", Score: 0.91},
		{Index: 1, Text: "vaga 2", ID: "vac_bbb", Score: 0.72},
	}, nil
}

func (m *mockVectorStore) ListVacancies(ctx context.Context) ([]domain.Vacancy, error) {
	if m.listVacanciesFn != nil {
		return m.listVacanciesFn(ctx)
	}
	return nil, nil
}

func (m *mockVectorStore) DeleteVacancy(ctx context.Context, id string) error {
	if m.deleteVacancyFn != nil {
		return m.deleteVacancyFn(ctx, id)
	}
	return nil
}

type mockEmail struct {
	sendFn func(ctx context.Context, to string, subject string, body string, attachmentB64 string, attachmentName string) error
}

func (m *mockEmail) SendEmail(ctx context.Context, to string, subject string, body string, attachmentB64 string, attachmentName string) error {
	return m.sendFn(ctx, to, subject, body, attachmentB64, attachmentName)
}

type mockWhatsApp struct {
	sendFn func(ctx context.Context, phoneSender string, to string, message string) error
	docFn  func(ctx context.Context, phoneSender string, to string, caption string, fileBytes []byte, filename string) error
}

func (m *mockWhatsApp) SendMessage(ctx context.Context, phoneSender string, to string, message string) error {
	return m.sendFn(ctx, phoneSender, to, message)
}

func (m *mockWhatsApp) SendDocument(ctx context.Context, phoneSender string, to string, caption string, fileBytes []byte, filename string) error {
	return m.docFn(ctx, phoneSender, to, caption, fileBytes, filename)
}

type mockCredsRepo struct {
	getFn func(ctx context.Context, email string) (*domain.EmailCredentials, error)
}

func (m *mockCredsRepo) SaveEmailCredentials(ctx context.Context, creds *domain.EmailCredentials) error {
	return nil
}

func (m *mockCredsRepo) GetEmailCredentials(ctx context.Context, email string) (*domain.EmailCredentials, error) {
	if m.getFn != nil {
		return m.getFn(ctx, email)
	}
	return &domain.EmailCredentials{
		Email:        email,
		Provider:     "google",
		RefreshToken: "ref",
		ClientID:     "id",
		ClientSecret: "secret",
	}, nil
}

func (m *mockCredsRepo) DeleteEmailCredentials(ctx context.Context, email string) error {
	return nil
}

func (m *mockCredsRepo) ListEmailCredentials(ctx context.Context) ([]domain.EmailCredentials, error) {
	return nil, nil
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

		orch := NewOrchestrator(&mockGemini{}, &mockGemini{}, mStore, &mockCredsRepo{}, &mockWhatsApp{})
		err := orch.ClearVacancies(context.Background())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !clearCalled {
			t.Error("expected Clear to be called on store")
		}
	})

	t.Run("PopulateVacancyTexts embeds and saves new vacancies", func(t *testing.T) {
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

		orch := NewOrchestrator(&mockGemini{}, &mockGemini{}, mStore, &mockCredsRepo{}, &mockWhatsApp{})
		count, err := orch.PopulateVacancyTexts(context.Background(), []string{"vaga 1", "vaga 2"})
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

	t.Run("PopulateVacancyTexts skips existing vacancies", func(t *testing.T) {
		addCalled := false
		mStore := &mockVectorStore{
			existingIDsFn: func(ctx context.Context, ids []string) (map[string]bool, error) {
				if len(ids) != 2 {
					t.Errorf("esperava os 2 hashes numa consulta só, got: %v", ids)
				}
				existingID := fmt.Sprintf("vac_%x", sha256.Sum256([]byte("vaga 1")))
				return map[string]bool{existingID: true}, nil
			},
			addVacanciesFn: func(ctx context.Context, vacancies []string, embeddings [][]float32) error {
				addCalled = true
				if len(vacancies) != 1 || vacancies[0] != "vaga 2" {
					t.Errorf("expected only 'vaga 2' to be added, got: %v", vacancies)
				}
				return nil
			},
		}

		orch := NewOrchestrator(&mockGemini{}, &mockGemini{}, mStore, &mockCredsRepo{}, &mockWhatsApp{})
		count, err := orch.PopulateVacancyTexts(context.Background(), []string{"vaga 1", "vaga 2"})
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

	// Listar vagas não pode depender do serviço de embeddings: com o SQLite é um
	// SELECT, e GET /api/v1/vacancies precisa responder com o Ollama fora do ar.
	t.Run("ListVacancies delegates to the store without touching the embedding service", func(t *testing.T) {
		wantVacancies := []domain.Vacancy{{Index: 0, Text: "vaga 1"}, {Index: 1, Text: "vaga 2"}}
		mGemini := &mockGemini{
			embedFn: func(ctx context.Context, texts []string) ([][]float32, error) {
				t.Error("ListVacancies não pode chamar o serviço de embeddings")
				return nil, fmt.Errorf("embedding service unavailable")
			},
		}
		mStore := &mockVectorStore{
			listVacanciesFn: func(ctx context.Context) ([]domain.Vacancy, error) {
				return wantVacancies, nil
			},
		}

		orch := NewOrchestrator(&mockGemini{}, mGemini, mStore, &mockCredsRepo{}, &mockWhatsApp{})
		got, err := orch.ListVacancies(context.Background())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(got) != len(wantVacancies) {
			t.Errorf("expected %d vacancies, got: %d", len(wantVacancies), len(got))
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
			docFn: func(ctx context.Context, phoneSender string, to string, caption string, fileBytes []byte, filename string) error {
				waCalls++
				if phoneSender != "5511888888888" || to != "5511999999999" {
					t.Errorf("unexpected whatsapp call parameters: sender=%s, to=%s", phoneSender, to)
				}
				if string(fileBytes) != "hello" {
					t.Errorf("unexpected whatsapp document bytes: %q", fileBytes)
				}
				return nil
			},
		}

		orch := NewOrchestrator(mGemini, mGemini, &mockVectorStore{}, &mockCredsRepo{}, mWhatsApp).(*orchestrator)
		orch.newEmailService = func(creds *domain.EmailCredentials) domain.EmailService {
			return mEmail
		}

		res, err := orch.MatchResume(context.Background(), "aGVsbG8=", "application/pdf", "test@gmail.com", "5511888888888")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(res.Matches) != 2 {
			t.Fatalf("expected 2 matches, got: %d", len(res.Matches))
		}
		// O índice que a LLM devolve é posicional na lista pré-filtrada; o que
		// identifica a vaga de verdade é o ID carregado a partir dele.
		if res.Matches[0].VacancyID != "vac_aaa" || res.Matches[1].VacancyID != "vac_bbb" {
			t.Errorf("match deve carregar o ID da vaga pré-filtrada, got %q e %q",
				res.Matches[0].VacancyID, res.Matches[1].VacancyID)
		}
		if emailCalls != 1 {
			t.Errorf("expected 1 email call, got: %d", emailCalls)
		}
		if waCalls != 1 {
			t.Errorf("expected 1 whatsapp call, got: %d", waCalls)
		}
	})

	t.Run("DeleteVacancy delegates to store", func(t *testing.T) {
		var gotID string
		mStore := &mockVectorStore{
			deleteVacancyFn: func(ctx context.Context, id string) error {
				gotID = id
				return nil
			},
		}

		orch := NewOrchestrator(&mockGemini{}, &mockGemini{}, mStore, &mockCredsRepo{}, &mockWhatsApp{})
		if err := orch.DeleteVacancy(context.Background(), "vac_123"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if gotID != "vac_123" {
			t.Errorf("expected store to receive id 'vac_123', got: %q", gotID)
		}
	})

	t.Run("Match history: MatchResume records a match, ListMatches/GetMatch/DeleteMatch operate on it", func(t *testing.T) {
		mGemini := &mockGemini{
			matchFn: func(ctx context.Context, fileB64 string, fileMime string, vacancies []string) (*domain.MatchResult, error) {
				return &domain.MatchResult{Matches: []domain.Match{}}, nil
			},
		}

		orch := NewOrchestrator(mGemini, mGemini, &mockVectorStore{}, &mockCredsRepo{}, &mockWhatsApp{})

		res, err := orch.MatchResume(context.Background(), "aGVsbG8=", "application/pdf", "", "")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		records, err := orch.ListMatches(context.Background())
		if err != nil {
			t.Fatalf("unexpected error listing matches: %v", err)
		}
		if len(records) != 1 {
			t.Fatalf("expected 1 recorded match, got: %d", len(records))
		}
		id := records[0].ID

		got, err := orch.GetMatch(context.Background(), id)
		if err != nil {
			t.Fatalf("unexpected error getting match: %v", err)
		}
		if got.Result != res {
			t.Errorf("expected GetMatch to return the same result recorded by MatchResume")
		}

		if err := orch.DeleteMatch(context.Background(), id); err != nil {
			t.Fatalf("unexpected error deleting match: %v", err)
		}

		if _, err := orch.GetMatch(context.Background(), id); err == nil {
			t.Error("expected error getting a deleted match")
		}

		if err := orch.DeleteMatch(context.Background(), "does-not-exist"); err == nil {
			t.Error("expected error deleting a non-existent match")
		}
	})

}
