package service

import (
	"api/internal/domain"
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type mockGroupWatchRepo struct {
	mu       sync.Mutex
	archived []domain.BufferedMessage
	phones   []string
}

func (r *mockGroupWatchRepo) ListWatchedGroups(ctx context.Context, phone string) ([]domain.WatchedGroup, error) {
	return nil, nil
}

func (r *mockGroupWatchRepo) ListAllWatchedPhones(ctx context.Context) ([]string, error) {
	return r.phones, nil
}

func (r *mockGroupWatchRepo) SetWatchedGroups(ctx context.Context, phone string, groups []domain.GroupInfo) error {
	return nil
}

func (r *mockGroupWatchRepo) ArchiveMessages(ctx context.Context, msgs []domain.BufferedMessage) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.archived = append(r.archived, msgs...)
	return nil
}

func (r *mockGroupWatchRepo) archivedCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.archived)
}

type mockOrchestrator struct {
	calls       atomic.Int32
	clears      atomic.Int32
	populateErr error
}

func (m *mockOrchestrator) ClearVacancies(ctx context.Context) error {
	m.clears.Add(1)
	return nil
}

func (m *mockOrchestrator) PopulateVacancyTexts(ctx context.Context, texts []string) (int, error) {
	m.calls.Add(1)
	if m.populateErr != nil {
		return 0, m.populateErr
	}
	return len(texts), nil
}

func (m *mockOrchestrator) ListVacancies(ctx context.Context) ([]domain.Vacancy, error) {
	return nil, nil
}

func (m *mockOrchestrator) DeleteVacancy(ctx context.Context, id string) error {
	return nil
}

func (m *mockOrchestrator) MatchResume(ctx context.Context, fileB64, fileMime, candidateEmail, candidatePhone string) (*domain.ProcessResult, error) {
	return nil, nil
}

func (m *mockOrchestrator) ListMatches(ctx context.Context) ([]*domain.MatchRecord, error) {
	return nil, nil
}

func (m *mockOrchestrator) GetMatch(ctx context.Context, id string) (*domain.MatchRecord, error) {
	return nil, nil
}

func (m *mockOrchestrator) DeleteMatch(ctx context.Context, id string) error {
	return nil
}

func TestGroupWatcherDebounce(t *testing.T) {
	repo := &mockGroupWatchRepo{}
	orch := &mockOrchestrator{}
	gw := NewGroupWatcher(repo, nil, orch)
	gw.debounceDelay = 20 * time.Millisecond

	gw.onMessage("phone", domain.BufferedMessage{MessageID: "1", Phone: "p", Text: "hello", ReceivedAt: time.Now()})

	if orch.calls.Load() != 0 {
		t.Fatalf("expected no flush before debounce elapses, got %d calls", orch.calls.Load())
	}

	time.Sleep(60 * time.Millisecond)

	if orch.calls.Load() != 1 {
		t.Fatalf("expected exactly 1 flush after debounce elapses, got %d", orch.calls.Load())
	}
}

func TestGroupWatcherDebounceResetOnNewMessage(t *testing.T) {
	repo := &mockGroupWatchRepo{}
	orch := &mockOrchestrator{}
	gw := NewGroupWatcher(repo, nil, orch)
	gw.debounceDelay = 30 * time.Millisecond

	gw.onMessage("phone", domain.BufferedMessage{MessageID: "1", Phone: "p", Text: "a", ReceivedAt: time.Now()})
	time.Sleep(15 * time.Millisecond)
	gw.onMessage("phone", domain.BufferedMessage{MessageID: "2", Phone: "p", Text: "b", ReceivedAt: time.Now()})
	time.Sleep(20 * time.Millisecond)
	if orch.calls.Load() != 0 {
		t.Fatalf("expected no flush yet (timer should have reset), got %d calls", orch.calls.Load())
	}

	time.Sleep(30 * time.Millisecond)
	if orch.calls.Load() != 1 {
		t.Fatalf("expected exactly 1 flush after reset delay elapses, got %d", orch.calls.Load())
	}
}

func TestGroupWatcherListWatchedPhones(t *testing.T) {
	repo := &mockGroupWatchRepo{phones: []string{"5511999999999", "5511888888888"}}
	gw := NewGroupWatcher(repo, nil, &mockOrchestrator{})

	got, err := gw.ListWatchedPhones(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 phones, got: %d", len(got))
	}
}

func TestFlushNowClearVacanciesOncePerDay(t *testing.T) {
	repo := &mockGroupWatchRepo{}
	orch := &mockOrchestrator{}
	gw := NewGroupWatcher(repo, nil, orch)

	gw.onMessage("p", domain.BufferedMessage{MessageID: "1", Phone: "p", Text: "vaga A"})
	if _, err := gw.FlushNow(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if orch.clears.Load() != 1 {
		t.Fatalf("primeiro flush do dia deve limpar as vagas, got %d", orch.clears.Load())
	}

	gw.onMessage("p", domain.BufferedMessage{MessageID: "2", Phone: "p", Text: "vaga B"})
	if _, err := gw.FlushNow(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if orch.clears.Load() != 1 {
		t.Errorf("segundo flush no mesmo dia não pode limpar de novo, got %d", orch.clears.Load())
	}
}

// O buffer só existe em memória, então uma falha no populate não pode arquivar
// nada — senão a mensagem contaria como processada sem ter virado vaga, e como
// não há mais fila no disco, ninguém a reprocessaria.
func TestFlushNowKeepsMessagesInBufferWhenPopulateFails(t *testing.T) {
	repo := &mockGroupWatchRepo{}
	orch := &mockOrchestrator{populateErr: errors.New("ollama fora do ar")}
	gw := NewGroupWatcher(repo, nil, orch)

	gw.onMessage("p", domain.BufferedMessage{MessageID: "1", Phone: "p", Text: "vaga A"})

	if _, err := gw.FlushNow(context.Background()); err == nil {
		t.Fatal("esperava erro do populate")
	}
	if got := repo.archivedCount(); got != 0 {
		t.Errorf("não pode arquivar mensagem que não virou vaga, arquivou %d", got)
	}

	orch.populateErr = nil
	if _, err := gw.FlushNow(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := repo.archivedCount(); got != 1 {
		t.Fatalf("mensagem retida deve ser reprocessada, arquivou %d", got)
	}

	status, err := gw.Status(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if status.PendingCount != 0 {
		t.Errorf("buffer deve esvaziar após sucesso, sobrou %d", status.PendingCount)
	}
}

// Reentrega do mesmo evento pelo whatsmeow não pode duplicar a vaga nem adiar o
// flush — era o papel do INSERT OR IGNORE na PK (phone, message_id).
func TestOnMessageIgnoresDuplicateMessageID(t *testing.T) {
	repo := &mockGroupWatchRepo{}
	orch := &mockOrchestrator{}
	gw := NewGroupWatcher(repo, nil, orch)

	msg := domain.BufferedMessage{MessageID: "1", Phone: "p", Text: "vaga A"}
	gw.onMessage("p", msg)
	gw.onMessage("p", msg)

	status, err := gw.Status(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if status.PendingCount != 1 {
		t.Errorf("mensagem repetida não pode entrar duas vezes, buffer tem %d", status.PendingCount)
	}
}
