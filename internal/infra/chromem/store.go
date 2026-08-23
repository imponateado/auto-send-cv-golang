package chromem

import (
	"context"
	"crypto/sha256"
	"fmt"
	"log"
	"strings"

	"api/internal/domain"

	"github.com/philippgille/chromem-go"
)

type chromemStore struct {
	db         *chromem.DB
	collection *chromem.Collection
	dummyFunc  chromem.EmbeddingFunc
}

func NewChromemStore(dbPath string) (domain.VectorStore, error) {
	db, err := chromem.NewPersistentDB(dbPath, false)
	if err != nil {
		return nil, fmt.Errorf("failed to create persistent chromem db at %s: %w", dbPath, err)
	}

	dummyFunc := func(ctx context.Context, text string) ([]float32, error) {
		return nil, fmt.Errorf("automatic embedding is disabled, please supply embeddings manually")
	}

	collection, err := db.GetOrCreateCollection("vacancies", nil, dummyFunc)
	if err != nil {
		return nil, fmt.Errorf("failed to get or create vacancies collection: %w", err)
	}

	return &chromemStore{
		db:         db,
		collection: collection,
		dummyFunc:  dummyFunc,
	}, nil
}

func (s *chromemStore) Clear(ctx context.Context) error {
	log.Println("[ChromeStore] Limpando coleção de vagas...")

	err := s.db.DeleteCollection("vacancies")
	if err != nil {
		log.Printf("[ChromeStore] Aviso ao deletar coleção (pode não existir ainda): %v", err)
	}

	collection, err := s.db.GetOrCreateCollection("vacancies", nil, s.dummyFunc)
	if err != nil {
		return fmt.Errorf("failed to recreate vacancies collection after clear: %w", err)
	}
	s.collection = collection
	log.Printf("[ChromeStore] Coleção de vagas limpa com sucesso")
	return nil
}

func (s *chromemStore) HasVacancy(ctx context.Context, id string) (bool, error) {
	_, err := s.collection.GetByID(ctx, id)
	if err != nil {
		return false, nil
	}
	return true, nil
}

func (s *chromemStore) AddVacancies(ctx context.Context, vacancies []string, embeddings [][]float32) error {
	if len(vacancies) != len(embeddings) {
		return fmt.Errorf("size mismatch: got %d vacancies and %d embeddings", len(vacancies), len(embeddings))
	}

	log.Printf("[ChromeStore] Adicionando %d vagas ao banco...", len(vacancies))

	// O ID (hash do texto) é a única identidade que a vaga precisa: gravar um
	// índice na metadata guardaria a posição dentro *deste* lote, que recomeça do
	// zero a cada chamada e colidiria entre lotes.
	ids := make([]string, len(vacancies))
	for i, vacancy := range vacancies {
		hash := sha256.Sum256([]byte(strings.TrimSpace(vacancy)))
		ids[i] = fmt.Sprintf("vac_%x", hash)
	}

	err := s.collection.Add(ctx, ids, embeddings, nil, vacancies)
	if err != nil {
		return fmt.Errorf("failed to add documents to chromem collection: %w", err)
	}

	log.Printf("[ChromeStore] %d vagas persistidas em disco com sucesso", len(vacancies))
	return nil
}

func (s *chromemStore) SearchSimilarity(ctx context.Context, queryEmbedding []float32, limit int, threshold float32) ([]domain.Vacancy, error) {
	log.Printf("[ChromeStore] Buscando as %d vagas mais similares (threshold=%.2f) em memória...", limit, threshold)

	if limit <= 0 {
		limit = 10
	}

	results, err := s.collection.QueryEmbedding(ctx, queryEmbedding, limit, nil, nil)
	if err != nil {
		return nil, fmt.Errorf("chromem query failed: %w", err)
	}

	var matchedVacancies []domain.Vacancy
	for _, res := range results {
		log.Printf("[ChromeStore] Vaga ID=%s - Similaridade de Cosseno: %.4f", res.ID, res.Similarity)
		if res.Similarity >= threshold {
			matchedVacancies = append(matchedVacancies, domain.Vacancy{
				Index: len(matchedVacancies),
				Text:  res.Content,
				ID:    res.ID,
				Score: res.Similarity,
			})
		}
	}

	log.Printf("[ChromeStore] Busca vetorial concluída: %d de %d vagas bateram com o limite (threshold)", len(matchedVacancies), len(results))
	return matchedVacancies, nil
}

// ponytail: chromem-go v0.7.0 não tem API nativa de "listar tudo" (sem
// ListIDs/Iterate/GetAll). Pedimos exatamente Count() resultados via
// QueryEmbedding, que devolve todos os documentos independente da ordenação
// por similaridade — o vetor de consulta só serve pra bater a dimensão
// esperada pela lib. Upgrade path: trocar por uma API nativa de listagem se a
// lib um dia adicionar uma.
func (s *chromemStore) ListVacancies(ctx context.Context, queryEmbedding []float32) ([]domain.Vacancy, error) {
	count := s.collection.Count()
	if count == 0 {
		return []domain.Vacancy{}, nil
	}

	results, err := s.collection.QueryEmbedding(ctx, queryEmbedding, count, nil, nil)
	if err != nil {
		return nil, fmt.Errorf("chromem list query failed: %w", err)
	}

	vacancies := make([]domain.Vacancy, 0, len(results))
	for i, res := range results {
		vacancies = append(vacancies, domain.Vacancy{
			Index: i,
			Text:  res.Content,
			ID:    res.ID,
			Score: res.Similarity,
		})
	}
	return vacancies, nil
}

// DeleteVacancy remove uma única vaga do banco vetorial pelo seu ID (o mesmo
// devolvido em Vacancy.ID por SearchSimilarity/ListVacancies).
func (s *chromemStore) DeleteVacancy(ctx context.Context, id string) error {
	if err := s.collection.Delete(ctx, nil, nil, id); err != nil {
		return fmt.Errorf("failed to delete vacancy %s: %w", id, err)
	}
	return nil
}
