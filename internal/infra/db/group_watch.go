package db

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"api/internal/domain"
)

type groupWatchRepo struct {
	db *sql.DB
}

// NewGroupWatchRepo cria as tabelas de grupos observados e buffer de mensagens
// em db caso não existam. Retorna um domain.GroupWatchRepository, ou erro se a
// criação do schema falhar.
func NewGroupWatchRepo(db *sql.DB) (domain.GroupWatchRepository, error) {
	query := `
	CREATE TABLE IF NOT EXISTS watched_groups (
		phone       TEXT NOT NULL,
		group_jid   TEXT NOT NULL,
		group_name  TEXT NOT NULL DEFAULT '',
		created_at  DATETIME NOT NULL,
		PRIMARY KEY (phone, group_jid)
	);

	CREATE TABLE IF NOT EXISTS group_message_buffer (
		message_id   TEXT NOT NULL,
		phone        TEXT NOT NULL,
		group_jid    TEXT NOT NULL,
		sender_jid   TEXT NOT NULL,
		text         TEXT NOT NULL,
		received_at  DATETIME NOT NULL,
		processed_at DATETIME,
		PRIMARY KEY (phone, message_id)
	);
	CREATE INDEX IF NOT EXISTS idx_group_message_buffer_unprocessed
		ON group_message_buffer (processed_at);
	`
	if _, err := db.Exec(query); err != nil {
		return nil, fmt.Errorf("failed to initialize group watch schema: %w", err)
	}

	return &groupWatchRepo{db: db}, nil
}

func (r *groupWatchRepo) ListWatchedGroups(ctx context.Context, phone string) ([]domain.WatchedGroup, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT phone, group_jid, group_name, created_at FROM watched_groups WHERE phone = ?;`, phone)
	if err != nil {
		return nil, fmt.Errorf("failed to list watched groups: %w", err)
	}
	defer rows.Close()

	groups := make([]domain.WatchedGroup, 0)
	for rows.Next() {
		var g domain.WatchedGroup
		if err := rows.Scan(&g.Phone, &g.GroupJID, &g.GroupName, &g.CreatedAt); err != nil {
			return nil, fmt.Errorf("failed to scan watched group: %w", err)
		}
		groups = append(groups, g)
	}
	return groups, rows.Err()
}

func (r *groupWatchRepo) ListAllWatchedPhones(ctx context.Context) ([]string, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT DISTINCT phone FROM watched_groups;`)
	if err != nil {
		return nil, fmt.Errorf("failed to list watched phones: %w", err)
	}
	defer rows.Close()

	var phones []string
	for rows.Next() {
		var phone string
		if err := rows.Scan(&phone); err != nil {
			return nil, fmt.Errorf("failed to scan phone: %w", err)
		}
		phones = append(phones, phone)
	}
	return phones, rows.Err()
}

func (r *groupWatchRepo) SetWatchedGroups(ctx context.Context, phone string, groups []domain.GroupInfo) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, `DELETE FROM watched_groups WHERE phone = ?;`, phone); err != nil {
		return fmt.Errorf("failed to clear watched groups: %w", err)
	}

	for _, g := range groups {
		_, err := tx.ExecContext(ctx,
			`INSERT INTO watched_groups (phone, group_jid, group_name, created_at) VALUES (?, ?, ?, ?);`,
			phone, g.JID, g.Name, time.Now())
		if err != nil {
			return fmt.Errorf("failed to save watched group %s: %w", g.JID, err)
		}
	}

	return tx.Commit()
}

// ArchiveMessages grava em lote as mensagens já processadas. processed_at é
// preenchido na própria inserção: quando a linha nasce, a vaga já está no banco
// vetorial. Retorna erro se a transação falhar.
func (r *groupWatchRepo) ArchiveMessages(ctx context.Context, msgs []domain.BufferedMessage) error {
	if len(msgs) == 0 {
		return nil
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	stmt, err := tx.PrepareContext(ctx,
		`INSERT OR IGNORE INTO group_message_buffer
			(message_id, phone, group_jid, sender_jid, text, received_at, processed_at)
		VALUES (?, ?, ?, ?, ?, ?, ?);`)
	if err != nil {
		return fmt.Errorf("failed to prepare archive statement: %w", err)
	}
	defer stmt.Close()

	now := time.Now()
	for _, m := range msgs {
		if _, err := stmt.ExecContext(ctx, m.MessageID, m.Phone, m.GroupJID, m.SenderJID, m.Text, m.ReceivedAt, now); err != nil {
			return fmt.Errorf("failed to archive message %s: %w", m.MessageID, err)
		}
	}

	return tx.Commit()
}
