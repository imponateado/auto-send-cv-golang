package db

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"api/internal/domain"
)

type candidateRepo struct {
	db *sql.DB
}

// NewCandidateRepo cria as tabelas de perfil e de candidaturas já feitas em db
// caso não existam. Retorna um domain.CandidateRepository, ou erro se a criação
// do schema falhar.
//
// applied_vacancies não é limpa junto com as vagas: como o id da vaga é o sha256
// do texto, guardar o histórico para sempre é o que impede uma vaga repostada
// amanhã de gerar uma segunda candidatura.
func NewCandidateRepo(db *sql.DB) (domain.CandidateRepository, error) {
	query := `
	CREATE TABLE IF NOT EXISTS candidate_profiles (
		id          TEXT PRIMARY KEY,
		email       TEXT NOT NULL DEFAULT '',
		phone       TEXT NOT NULL DEFAULT '',
		resume_b64  TEXT NOT NULL,
		resume_mime TEXT NOT NULL,
		active      INTEGER NOT NULL DEFAULT 1,
		updated_at  DATETIME NOT NULL
	);

	CREATE TABLE IF NOT EXISTS applied_vacancies (
		candidate_id TEXT NOT NULL,
		vacancy_id   TEXT NOT NULL,
		applied_at   DATETIME NOT NULL,
		PRIMARY KEY (candidate_id, vacancy_id)
	);`
	if _, err := db.Exec(query); err != nil {
		return nil, fmt.Errorf("failed to initialize candidate schema: %w", err)
	}

	return &candidateRepo{db: db}, nil
}

func (r *candidateRepo) SaveProfile(ctx context.Context, p *domain.CandidateProfile) error {
	query := `
	INSERT OR REPLACE INTO candidate_profiles (id, email, phone, resume_b64, resume_mime, active, updated_at)
	VALUES (?, ?, ?, ?, ?, ?, ?);`
	p.UpdatedAt = time.Now()
	_, err := r.db.ExecContext(ctx, query, p.ID, p.Email, p.Phone, p.ResumeB64, p.ResumeMime, p.Active, p.UpdatedAt)
	if err != nil {
		return fmt.Errorf("failed to save candidate profile: %w", err)
	}
	return nil
}

func (r *candidateRepo) ListProfiles(ctx context.Context) ([]domain.CandidateProfile, error) {
	query := `SELECT id, email, phone, resume_b64, resume_mime, active, updated_at FROM candidate_profiles ORDER BY id;`
	rows, err := r.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to list candidate profiles: %w", err)
	}
	defer rows.Close()

	profiles := make([]domain.CandidateProfile, 0)
	for rows.Next() {
		var p domain.CandidateProfile
		if err := rows.Scan(&p.ID, &p.Email, &p.Phone, &p.ResumeB64, &p.ResumeMime, &p.Active, &p.UpdatedAt); err != nil {
			return nil, fmt.Errorf("failed to scan candidate profile: %w", err)
		}
		profiles = append(profiles, p)
	}
	return profiles, rows.Err()
}

func (r *candidateRepo) DeleteProfile(ctx context.Context, id string) error {
	if _, err := r.db.ExecContext(ctx, `DELETE FROM candidate_profiles WHERE id = ?;`, id); err != nil {
		return fmt.Errorf("failed to delete candidate profile %s: %w", id, err)
	}
	return nil
}

func (r *candidateRepo) AppliedVacancyIDs(ctx context.Context, candidateID string) (map[string]bool, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT vacancy_id FROM applied_vacancies WHERE candidate_id = ?;`, candidateID)
	if err != nil {
		return nil, fmt.Errorf("failed to query applied vacancies: %w", err)
	}
	defer rows.Close()

	applied := make(map[string]bool)
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("failed to scan applied vacancy id: %w", err)
		}
		applied[id] = true
	}
	return applied, rows.Err()
}

// MarkApplied registra as vagas já disparadas para o candidato. OR IGNORE porque
// o mesmo par pode reaparecer numa corrida entre o match manual e o automático,
// e nesse caso a primeira gravação é a que vale.
func (r *candidateRepo) MarkApplied(ctx context.Context, candidateID string, vacancyIDs []string) error {
	if candidateID == "" || len(vacancyIDs) == 0 {
		return nil
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	stmt, err := tx.PrepareContext(ctx,
		`INSERT OR IGNORE INTO applied_vacancies (candidate_id, vacancy_id, applied_at) VALUES (?, ?, ?);`)
	if err != nil {
		return fmt.Errorf("failed to prepare applied insert: %w", err)
	}
	defer stmt.Close()

	now := time.Now()
	for _, id := range vacancyIDs {
		if strings.TrimSpace(id) == "" {
			continue
		}
		if _, err := stmt.ExecContext(ctx, candidateID, id, now); err != nil {
			return fmt.Errorf("failed to mark vacancy %s as applied: %w", id, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit applied vacancies: %w", err)
	}
	return nil
}
