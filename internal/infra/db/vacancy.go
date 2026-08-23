package db

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/binary"
	"fmt"
	"log"
	"math"
	"sort"
	"strings"
	"time"

	"api/internal/domain"
)

type vacancyStore struct {
	db *sql.DB
}

// NewVacancyStore cria a tabela de vagas em db caso não exista. Retorna um
// domain.VectorStore, ou erro se a criação do schema falhar.
//
// A busca por similaridade é força bruta: carrega todos os embeddings e calcula
// o cosseno contra cada um. É o mesmo algoritmo que o chromem-go usava, e serve
// de sobra para a escala deste projeto (algumas centenas de vagas, limpas
// diariamente). ponytail: acima de ~100 mil vagas isso precisaria de um índice
// vetorial de verdade.
func NewVacancyStore(db *sql.DB) (domain.VectorStore, error) {
	query := `
	CREATE TABLE IF NOT EXISTS vacancies (
		id         TEXT PRIMARY KEY,
		text       TEXT NOT NULL,
		embedding  BLOB NOT NULL,
		created_at DATETIME NOT NULL
	);`
	if _, err := db.Exec(query); err != nil {
		return nil, fmt.Errorf("failed to initialize vacancies schema: %w", err)
	}

	return &vacancyStore{db: db}, nil
}

// vacancyID devolve o identificador estável de uma vaga: o sha256 do texto sem
// espaços nas pontas. É o que deduplica a mesma vaga repostada no grupo.
func vacancyID(text string) string {
	return fmt.Sprintf("vac_%x", sha256.Sum256([]byte(strings.TrimSpace(text))))
}

func encodeEmbedding(v []float32) []byte {
	buf := make([]byte, len(v)*4)
	for i, f := range v {
		binary.LittleEndian.PutUint32(buf[i*4:], math.Float32bits(f))
	}
	return buf
}

func decodeEmbedding(b []byte) ([]float32, error) {
	if len(b)%4 != 0 {
		return nil, fmt.Errorf("embedding blob tem %d bytes, não múltiplo de 4", len(b))
	}
	v := make([]float32, len(b)/4)
	for i := range v {
		v[i] = math.Float32frombits(binary.LittleEndian.Uint32(b[i*4:]))
	}
	return v, nil
}

// cosineSimilarity devolve 0 para vetores de dimensões diferentes, o que faz a
// vaga ser ignorada em vez de derrubar a busca inteira — acontece se
// OLLAMA_MODEL mudar e sobrarem vetores da dimensão antiga no banco.
func cosineSimilarity(a, b []float32) float32 {
	if len(a) != len(b) || len(a) == 0 {
		return 0
	}
	var dot, normA, normB float64
	for i := range a {
		// Converte antes de multiplicar: fazer float64(a[i]*b[i]) acumularia o
		// erro do produto em float32 antes do alargamento.
		x, y := float64(a[i]), float64(b[i])
		dot += x * y
		normA += x * x
		normB += y * y
	}
	if normA == 0 || normB == 0 {
		return 0
	}
	return float32(dot / (math.Sqrt(normA) * math.Sqrt(normB)))
}

func (s *vacancyStore) Clear(ctx context.Context) error {
	log.Println("[VacancyStore] Limpando tabela de vagas...")
	if _, err := s.db.ExecContext(ctx, `DELETE FROM vacancies;`); err != nil {
		return fmt.Errorf("failed to clear vacancies: %w", err)
	}
	log.Println("[VacancyStore] Tabela de vagas limpa com sucesso")
	return nil
}

// idChunkSize limita quantos ids entram num IN (...) por consulta, para não
// esbarrar no teto de variáveis vinculadas do SQLite.
const idChunkSize = 500

func (s *vacancyStore) ExistingVacancyIDs(ctx context.Context, ids []string) (map[string]bool, error) {
	existing := make(map[string]bool, len(ids))
	if len(ids) == 0 {
		return existing, nil
	}

	for start := 0; start < len(ids); start += idChunkSize {
		end := start + idChunkSize
		if end > len(ids) {
			end = len(ids)
		}
		chunk := ids[start:end]

		args := make([]any, len(chunk))
		for i, id := range chunk {
			args[i] = id
		}
		query := `SELECT id FROM vacancies WHERE id IN (?` + strings.Repeat(`,?`, len(chunk)-1) + `);`

		rows, err := s.db.QueryContext(ctx, query, args...)
		if err != nil {
			return nil, fmt.Errorf("failed to query existing vacancy ids: %w", err)
		}
		for rows.Next() {
			var id string
			if err := rows.Scan(&id); err != nil {
				rows.Close()
				return nil, fmt.Errorf("failed to scan vacancy id: %w", err)
			}
			existing[id] = true
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return nil, fmt.Errorf("failed to iterate vacancy ids: %w", err)
		}
		rows.Close()
	}

	return existing, nil
}

func (s *vacancyStore) AddVacancies(ctx context.Context, vacancies []string, embeddings [][]float32) error {
	if len(vacancies) != len(embeddings) {
		return fmt.Errorf("size mismatch: got %d vacancies and %d embeddings", len(vacancies), len(embeddings))
	}
	if len(vacancies) == 0 {
		return nil
	}

	log.Printf("[VacancyStore] Adicionando %d vagas ao banco...", len(vacancies))

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	stmt, err := tx.PrepareContext(ctx,
		`INSERT OR REPLACE INTO vacancies (id, text, embedding, created_at) VALUES (?, ?, ?, ?);`)
	if err != nil {
		return fmt.Errorf("failed to prepare insert statement: %w", err)
	}
	defer stmt.Close()

	now := time.Now()
	for i, vacancy := range vacancies {
		if _, err := stmt.ExecContext(ctx, vacancyID(vacancy), vacancy, encodeEmbedding(embeddings[i]), now); err != nil {
			return fmt.Errorf("failed to insert vacancy %d: %w", i, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit vacancies: %w", err)
	}

	log.Printf("[VacancyStore] %d vagas persistidas com sucesso", len(vacancies))
	return nil
}

func (s *vacancyStore) SearchSimilarity(ctx context.Context, queryEmbedding []float32, limit int, threshold float32) ([]domain.Vacancy, error) {
	log.Printf("[VacancyStore] Buscando as %d vagas mais similares (threshold=%.2f)...", limit, threshold)

	if limit <= 0 {
		limit = 10
	}

	rows, err := s.db.QueryContext(ctx, `SELECT id, text, embedding FROM vacancies;`)
	if err != nil {
		return nil, fmt.Errorf("failed to query vacancies: %w", err)
	}
	defer rows.Close()

	var scanned int
	var candidates []domain.Vacancy
	for rows.Next() {
		var id, text string
		var blob []byte
		if err := rows.Scan(&id, &text, &blob); err != nil {
			return nil, fmt.Errorf("failed to scan vacancy: %w", err)
		}
		scanned++

		embedding, err := decodeEmbedding(blob)
		if err != nil {
			log.Printf("[VacancyStore] Vaga ID=%s tem embedding corrompido, ignorando: %v", id, err)
			continue
		}

		score := cosineSimilarity(queryEmbedding, embedding)
		log.Printf("[VacancyStore] Vaga ID=%s - Similaridade de Cosseno: %.4f", id, score)
		if score >= threshold {
			candidates = append(candidates, domain.Vacancy{Text: text, ID: id, Score: score})
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate vacancies: %w", err)
	}

	sort.SliceStable(candidates, func(i, j int) bool {
		return candidates[i].Score > candidates[j].Score
	})
	if len(candidates) > limit {
		candidates = candidates[:limit]
	}
	for i := range candidates {
		candidates[i].Index = i
	}

	log.Printf("[VacancyStore] Busca concluída: %d de %d vagas bateram com o threshold", len(candidates), scanned)
	return candidates, nil
}

// ListVacancies devolve todas as vagas armazenadas. Não lê o embedding: não há
// consulta contra a qual pontuar, então Score fica zerado.
func (s *vacancyStore) ListVacancies(ctx context.Context) ([]domain.Vacancy, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, text FROM vacancies ORDER BY created_at, id;`)
	if err != nil {
		return nil, fmt.Errorf("failed to list vacancies: %w", err)
	}
	defer rows.Close()

	vacancies := make([]domain.Vacancy, 0)
	for rows.Next() {
		var v domain.Vacancy
		if err := rows.Scan(&v.ID, &v.Text); err != nil {
			return nil, fmt.Errorf("failed to scan vacancy: %w", err)
		}
		v.Index = len(vacancies)
		vacancies = append(vacancies, v)
	}
	return vacancies, rows.Err()
}

// DeleteVacancy remove uma única vaga pelo seu ID (o mesmo devolvido em
// Vacancy.ID por SearchSimilarity/ListVacancies).
func (s *vacancyStore) DeleteVacancy(ctx context.Context, id string) error {
	if _, err := s.db.ExecContext(ctx, `DELETE FROM vacancies WHERE id = ?;`, id); err != nil {
		return fmt.Errorf("failed to delete vacancy %s: %w", id, err)
	}
	return nil
}
