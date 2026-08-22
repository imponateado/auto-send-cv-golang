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

// NewGroupWatchRepo cria as tabelas de grupos observados, buffer de mensagens e
// agendamento de flush em db caso não existam. Retorna um
// domain.GroupWatchRepository, ou erro se a criação do schema falhar.
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

	CREATE TABLE IF NOT EXISTS group_flush_schedule (
		id         INTEGER PRIMARY KEY CHECK (id = 1),
		hour       INTEGER NOT NULL DEFAULT 11,
		minute     INTEGER NOT NULL DEFAULT 0,
		updated_at DATETIME NOT NULL
	);
	INSERT OR IGNORE INTO group_flush_schedule (id, hour, minute, updated_at)
		VALUES (1, 11, 0, CURRENT_TIMESTAMP);
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

func (r *groupWatchRepo) BufferMessage(ctx context.Context, msg domain.BufferedMessage) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT OR IGNORE INTO group_message_buffer (message_id, phone, group_jid, sender_jid, text, received_at)
		VALUES (?, ?, ?, ?, ?, ?);`,
		msg.MessageID, msg.Phone, msg.GroupJID, msg.SenderJID, msg.Text, msg.ReceivedAt)
	if err != nil {
		return fmt.Errorf("failed to buffer message: %w", err)
	}
	return nil
}

func (r *groupWatchRepo) PendingMessages(ctx context.Context) ([]domain.BufferedMessage, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT message_id, phone, group_jid, sender_jid, text, received_at
		FROM group_message_buffer WHERE processed_at IS NULL;`)
	if err != nil {
		return nil, fmt.Errorf("failed to list pending messages: %w", err)
	}
	defer rows.Close()

	var msgs []domain.BufferedMessage
	for rows.Next() {
		var m domain.BufferedMessage
		if err := rows.Scan(&m.MessageID, &m.Phone, &m.GroupJID, &m.SenderJID, &m.Text, &m.ReceivedAt); err != nil {
			return nil, fmt.Errorf("failed to scan buffered message: %w", err)
		}
		msgs = append(msgs, m)
	}
	return msgs, rows.Err()
}

func (r *groupWatchRepo) MarkProcessed(ctx context.Context, phone string, messageIDs []string) error {
	if len(messageIDs) == 0 {
		return nil
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	stmt, err := tx.PrepareContext(ctx,
		`UPDATE group_message_buffer SET processed_at = ? WHERE phone = ? AND message_id = ?;`)
	if err != nil {
		return fmt.Errorf("failed to prepare mark-processed statement: %w", err)
	}
	defer stmt.Close()

	now := time.Now()
	for _, id := range messageIDs {
		if _, err := stmt.ExecContext(ctx, now, phone, id); err != nil {
			return fmt.Errorf("failed to mark message %s processed: %w", id, err)
		}
	}

	return tx.Commit()
}

func (r *groupWatchRepo) CountPending(ctx context.Context) (int, error) {
	var count int
	err := r.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM group_message_buffer WHERE processed_at IS NULL;`).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count pending messages: %w", err)
	}
	return count, nil
}

func (r *groupWatchRepo) GetSchedule(ctx context.Context) (domain.FlushSchedule, error) {
	var sched domain.FlushSchedule
	err := r.db.QueryRowContext(ctx, `SELECT hour, minute FROM group_flush_schedule WHERE id = 1;`).
		Scan(&sched.Hour, &sched.Minute)
	if err != nil {
		return domain.FlushSchedule{}, fmt.Errorf("failed to get flush schedule: %w", err)
	}
	return sched, nil
}

func (r *groupWatchRepo) SetSchedule(ctx context.Context, sched domain.FlushSchedule) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE group_flush_schedule SET hour = ?, minute = ?, updated_at = ? WHERE id = 1;`,
		sched.Hour, sched.Minute, time.Now())
	if err != nil {
		return fmt.Errorf("failed to set flush schedule: %w", err)
	}
	return nil
}
