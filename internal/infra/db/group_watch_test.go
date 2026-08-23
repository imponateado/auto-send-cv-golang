package db

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"api/internal/domain"
)

func TestArchiveMessages(t *testing.T) {
	sqlDB, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("failed to open db: %v", err)
	}
	defer sqlDB.Close()

	repo, err := NewGroupWatchRepo(sqlDB)
	if err != nil {
		t.Fatalf("failed to init repo: %v", err)
	}
	ctx := context.Background()

	if err := repo.ArchiveMessages(ctx, nil); err != nil {
		t.Fatalf("arquivar lote vazio não pode falhar: %v", err)
	}

	received := time.Now().Add(-time.Hour).Truncate(time.Second)
	msgs := []domain.BufferedMessage{
		{MessageID: "m1", Phone: "5511999999999", GroupJID: "g@g.us", SenderJID: "s@s.net", Text: "vaga A", ReceivedAt: received},
		{MessageID: "m2", Phone: "5511999999999", GroupJID: "g@g.us", SenderJID: "s@s.net", Text: "vaga B", ReceivedAt: received},
	}
	if err := repo.ArchiveMessages(ctx, msgs); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Arquivar de novo é inofensivo: a PK (phone, message_id) absorve a repetição.
	if err := repo.ArchiveMessages(ctx, msgs); err != nil {
		t.Fatalf("unexpected error on re-archive: %v", err)
	}

	var total, unprocessed int
	if err := sqlDB.QueryRowContext(ctx, `SELECT COUNT(*) FROM group_message_buffer;`).Scan(&total); err != nil {
		t.Fatalf("failed to count rows: %v", err)
	}
	if total != 2 {
		t.Errorf("esperava 2 linhas após arquivar o mesmo lote duas vezes, got %d", total)
	}

	// A linha nasce processada — é arquivo, não fila de trabalho.
	if err := sqlDB.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM group_message_buffer WHERE processed_at IS NULL;`).Scan(&unprocessed); err != nil {
		t.Fatalf("failed to count unprocessed rows: %v", err)
	}
	if unprocessed != 0 {
		t.Errorf("linha arquivada não pode ficar com processed_at NULL, got %d", unprocessed)
	}

	var text string
	var gotReceived time.Time
	if err := sqlDB.QueryRowContext(ctx,
		`SELECT text, received_at FROM group_message_buffer WHERE message_id = 'm1';`).Scan(&text, &gotReceived); err != nil {
		t.Fatalf("failed to read archived row: %v", err)
	}
	if text != "vaga A" {
		t.Errorf("texto arquivado: want %q, got %q", "vaga A", text)
	}
	if !gotReceived.UTC().Equal(received.UTC()) {
		t.Errorf("received_at deve preservar o horário original da mensagem: want %v, got %v", received, gotReceived)
	}
}
