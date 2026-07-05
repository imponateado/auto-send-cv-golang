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
	
	ids := make([]string, len(vacancies))
	metadatas := make([]map[string]string, len(vacancies))
	for i, vacancy := range vacancies {
		trimmed := strings.TrimSpace(vacancy)
		hash := sha256.Sum256([]byte(trimmed))
		ids[i] = fmt.Sprintf("vac_%x", hash)
		metadatas[i] = map[string]string{
			"index": fmt.Sprintf("%d", i),
		}
	}

	err := s.collection.Add(ctx, ids, embeddings, metadatas, vacancies)
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

	// In chromem-go v0.7.0, QueryEmbedding expects: ctx, queryEmbedding, limit, metadatas, documentContents
	results, err := s.collection.QueryEmbedding(ctx, queryEmbedding, limit, nil, nil)
	if err != nil {
		return nil, fmt.Errorf("chromem query failed: %w", err)
	}

	var matchedVacancies []domain.Vacancy
	for _, res := range results {
		log.Printf("[ChromeStore] Vaga ID=%s - Similaridade de Cosseno: %.4f", res.ID, res.Similarity)
		if res.Similarity >= threshold {
			var origIdx int
			if _, err := fmt.Sscanf(res.Metadata["index"], "%d", &origIdx); err != nil {
				origIdx = 0
			}
			matchedVacancies = append(matchedVacancies, domain.Vacancy{
				Index: origIdx,
				Text:  res.Content,
			})
		}
	}

	log.Printf("[ChromeStore] Busca vetorial concluída: %d de %d vagas bateram com o limite (threshold)", len(matchedVacancies), len(results))
	return matchedVacancies, nil
}
