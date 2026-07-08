package db

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"api/internal/domain"
	_ "modernc.org/sqlite"
)

type sqliteRepo struct {
	db *sql.DB
}

// NewSQLRepo inicializa a conexão com o banco SQLite e garante que a tabela de credenciais exista.
func NewSQLRepo(dbPath string) (domain.CredentialsRepository, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open sqlite database: %w", err)
	}

	query := `
	CREATE TABLE IF NOT EXISTS candidate_credentials (
		email TEXT PRIMARY KEY,
		provider TEXT NOT NULL,
		refresh_token TEXT NOT NULL,
		client_id TEXT NOT NULL,
		client_secret TEXT NOT NULL,
		created_at DATETIME NOT NULL
	);`
	_, err = db.Exec(query)
	if err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to initialize schema: %w", err)
	}

	return &sqliteRepo{db: db}, nil
}

func (r *sqliteRepo) SaveEmailCredentials(ctx context.Context, creds *domain.EmailCredentials) error {
	query := `
	INSERT OR REPLACE INTO candidate_credentials (email, provider, refresh_token, client_id, client_secret, created_at)
	VALUES (?, ?, ?, ?, ?, ?);`
	_, err := r.db.ExecContext(ctx, query, creds.Email, creds.Provider, creds.RefreshToken, creds.ClientID, creds.ClientSecret, time.Now())
	if err != nil {
		return fmt.Errorf("failed to save email credentials: %w", err)
	}
	return nil
}

func (r *sqliteRepo) GetEmailCredentials(ctx context.Context, email string) (*domain.EmailCredentials, error) {
	query := `SELECT email, provider, refresh_token, client_id, client_secret FROM candidate_credentials WHERE email = ?;`
	row := r.db.QueryRowContext(ctx, query, email)

	var creds domain.EmailCredentials
	err := row.Scan(&creds.Email, &creds.Provider, &creds.RefreshToken, &creds.ClientID, &creds.ClientSecret)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("credentials not found for email: %s", email)
	} else if err != nil {
		return nil, fmt.Errorf("failed to query email credentials: %w", err)
	}

	return &creds, nil
}

func (r *sqliteRepo) DeleteEmailCredentials(ctx context.Context, email string) error {
	query := `DELETE FROM candidate_credentials WHERE email = ?;`
	_, err := r.db.ExecContext(ctx, query, email)
	if err != nil {
		return fmt.Errorf("failed to delete credentials: %w", err)
	}
	return nil
}
