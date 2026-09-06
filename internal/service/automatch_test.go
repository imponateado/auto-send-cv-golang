package service

import (
	"context"
	"sync"
	"testing"

	"api/internal/domain"
)

// mockCandidateRepo é o repositório de perfis em memória. AppliedVacancyIDs e
// MarkApplied compartilham o mesmo mapa, então o teste vê exatamente o que a
// próxima rodada veria.
type mockCandidateRepo struct {
	mu       sync.Mutex
	profiles []domain.CandidateProfile
	applied  map[string]map[string]bool
	listedFn func()
}

func newMockCandidateRepo(profiles ...domain.CandidateProfile) *mockCandidateRepo {
	return &mockCandidateRepo{profiles: profiles, applied: map[string]map[string]bool{}}
}

func (m *mockCandidateRepo) SaveProfile(ctx context.Context, p *domain.CandidateProfile) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.profiles = append(m.profiles, *p)
	return nil
}

func (m *mockCandidateRepo) ListProfiles(ctx context.Context) ([]domain.CandidateProfile, error) {
	if m.listedFn != nil {
		m.listedFn()
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]domain.CandidateProfile(nil), m.profiles...), nil
}

func (m *mockCandidateRepo) DeleteProfile(ctx context.Context, id string) error { return nil }

func (m *mockCandidateRepo) AppliedVacancyIDs(ctx context.Context, candidateID string) (map[string]bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := map[string]bool{}
	for id := range m.applied[candidateID] {
		out[id] = true
	}
	return out, nil
}

func (m *mockCandidateRepo) MarkApplied(ctx context.Context, candidateID string, vacancyIDs []string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.applied[candidateID] == nil {
		m.applied[candidateID] = map[string]bool{}
	}
	for _, id := range vacancyIDs {
		m.applied[candidateID][id] = true
	}
	return nil
}

func TestMatchResume_SkipsAlreadyAppliedVacancies(t *testing.T) {
	const (
		oldID = "vac_ja_candidatada"
		newID = "vac_nova"
	)

	mStore := &mockVectorStore{
		searchSimilarityFn: func(ctx context.Context, q []float32, limit int, threshold float32) ([]domain.Vacancy, error) {
			return []domain.Vacancy{
				{Index: 0, ID: oldID, Text: "vaga antiga", Score: 0.9},
				{Index: 1, ID: newID, Text: "vaga nova", Score: 0.8},
			}, nil
		},
	}

	var seenByLLM []string
	mGemini := &mockGemini{
		matchFn: func(ctx context.Context, fileB64, fileMime string, vacancies []string) (*domain.MatchResult, error) {
			seenByLLM = vacancies
			matches := make([]domain.Match, len(vacancies))
			for i := range vacancies {
				matches[i] = domain.Match{Index: i, ContactType: "whatsapp", ContactTarget: "5511999999999"}
			}
			return &domain.MatchResult{Matches: matches}, nil
		},
	}

	repo := newMockCandidateRepo()
	if err := repo.MarkApplied(context.Background(), "eu@example.com", []string{oldID}); err != nil {
		t.Fatalf("setup falhou: %v", err)
	}

	// O dispatcher drena a fila numa goroutine própria e chega a chamar o envio:
	// sem docFn o worker estoura depois que o teste já passou.
	mWhatsApp := &mockWhatsApp{docFn: func(ctx context.Context, phoneSender, to, caption string, fileBytes []byte, filename string) error {
		return nil
	}}

	orch := NewOrchestrator(mGemini, &mockGemini{}, mStore, &mockCredsRepo{}, repo, mWhatsApp)
	res, err := orch.MatchResume(context.Background(), "Zm9v", "application/pdf", "eu@example.com", "5511999999999")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(seenByLLM) != 1 || seenByLLM[0] != "vaga nova" {
		t.Errorf("a vaga já candidatada devia ser filtrada antes da LLM, mas ela recebeu: %v", seenByLLM)
	}
	if len(res.Matches) != 1 || res.Matches[0].VacancyID != newID {
		t.Errorf("esperava só a vaga nova nos matches, got: %+v", res.Matches)
	}

	applied, _ := repo.AppliedVacancyIDs(context.Background(), "eu@example.com")
	if !applied[newID] {
		t.Error("a vaga nova enfileirada devia ter sido registrada como candidatada")
	}
}

func TestMatchResume_NoNewVacanciesSkipsLLM(t *testing.T) {
	mStore := &mockVectorStore{
		searchSimilarityFn: func(ctx context.Context, q []float32, limit int, threshold float32) ([]domain.Vacancy, error) {
			return []domain.Vacancy{{Index: 0, ID: "vac_x", Text: "vaga", Score: 0.9}}, nil
		},
	}
	mGemini := &mockGemini{
		matchFn: func(ctx context.Context, fileB64, fileMime string, vacancies []string) (*domain.MatchResult, error) {
			t.Fatal("a LLM não devia ser chamada quando toda vaga já foi candidatada")
			return nil, nil
		},
	}

	repo := newMockCandidateRepo()
	_ = repo.MarkApplied(context.Background(), "eu@example.com", []string{"vac_x"})

	orch := NewOrchestrator(mGemini, &mockGemini{}, mStore, &mockCredsRepo{}, repo, &mockWhatsApp{})
	res, err := orch.MatchResume(context.Background(), "Zm9v", "application/pdf", "eu@example.com", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(res.Matches) != 0 {
		t.Errorf("esperava nenhum match, got: %+v", res.Matches)
	}
}

func TestMatchStoredProfiles_RunsOnlyActiveProfiles(t *testing.T) {
	var matched []string
	mStore := &mockVectorStore{
		searchSimilarityFn: func(ctx context.Context, q []float32, limit int, threshold float32) ([]domain.Vacancy, error) {
			return nil, nil // sem vagas: o fluxo retorna cedo, sem tocar na LLM
		},
	}
	mGemini := &mockGemini{
		extractTextFn: func(ctx context.Context, fileB64, fileMime string) (string, error) {
			matched = append(matched, fileB64)
			return "currículo", nil
		},
	}

	repo := newMockCandidateRepo(
		domain.CandidateProfile{ID: "ativo@example.com", Email: "ativo@example.com", ResumeB64: "cv-ativo", Active: true},
		domain.CandidateProfile{ID: "pausado@example.com", Email: "pausado@example.com", ResumeB64: "cv-pausado", Active: false},
	)

	orch := NewOrchestrator(mGemini, mGemini, mStore, &mockCredsRepo{}, repo, &mockWhatsApp{})
	ran, err := orch.MatchStoredProfiles(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ran != 1 {
		t.Errorf("esperava 1 perfil processado, got: %d", ran)
	}
	if len(matched) != 1 || matched[0] != "cv-ativo" {
		t.Errorf("esperava só o currículo do perfil ativo, got: %v", matched)
	}
}

func TestMatchStoredProfiles_ConcurrentRunIsSkipped(t *testing.T) {
	// Segura a rodada dentro do ListProfiles para garantir que a segunda chamada
	// encontra a primeira em andamento.
	release := make(chan struct{})
	entered := make(chan struct{})

	repo := newMockCandidateRepo(
		domain.CandidateProfile{ID: "eu@example.com", Email: "eu@example.com", ResumeB64: "cv", Active: true},
	)
	var once sync.Once
	repo.listedFn = func() {
		once.Do(func() {
			close(entered)
			<-release
		})
	}

	mStore := &mockVectorStore{
		searchSimilarityFn: func(ctx context.Context, q []float32, limit int, threshold float32) ([]domain.Vacancy, error) {
			return nil, nil
		},
	}
	orch := NewOrchestrator(&mockGemini{}, &mockGemini{}, mStore, &mockCredsRepo{}, repo, &mockWhatsApp{})

	go func() {
		_, _ = orch.MatchStoredProfiles(context.Background())
	}()
	<-entered

	ran, err := orch.MatchStoredProfiles(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ran != 0 {
		t.Errorf("rodada concorrente devia ser pulada, mas processou %d perfis", ran)
	}
	close(release)
}

// O parser de PDF entra em pânico com arquivo malformado, e no match automático
// não há middleware de recuperação: sem este guard, um currículo ruim salvo no
// banco derruba o servidor num flush de madrugada.
func TestMatchStoredProfiles_SurvivesPanicInOneProfile(t *testing.T) {
	mGemini := &mockGemini{
		extractTextFn: func(ctx context.Context, fileB64, fileMime string) (string, error) {
			if fileB64 == "cv-corrompido" {
				panic("unexpected delimiter ')'")
			}
			return "currículo", nil
		},
	}
	mStore := &mockVectorStore{
		searchSimilarityFn: func(ctx context.Context, q []float32, limit int, threshold float32) ([]domain.Vacancy, error) {
			return nil, nil
		},
	}

	repo := newMockCandidateRepo(
		domain.CandidateProfile{ID: "ruim@example.com", Email: "ruim@example.com", ResumeB64: "cv-corrompido", Active: true},
		domain.CandidateProfile{ID: "bom@example.com", Email: "bom@example.com", ResumeB64: "cv-ok", Active: true},
	)

	orch := NewOrchestrator(mGemini, mGemini, mStore, &mockCredsRepo{}, repo, &mockWhatsApp{})
	ran, err := orch.MatchStoredProfiles(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// O perfil bom precisa rodar mesmo depois do pânico do anterior.
	if ran != 1 {
		t.Errorf("esperava o perfil seguinte processado apesar do pânico, got: %d", ran)
	}
}
