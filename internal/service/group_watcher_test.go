package service

import (
	"api/internal/domain"
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type mockGroupWatchRepo struct {
	mu     sync.Mutex
	msgs   []domain.BufferedMessage
	phones []string
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

func (r *mockGroupWatchRepo) BufferMessage(ctx context.Context, msg domain.BufferedMessage) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.msgs = append(r.msgs, msg)
	return nil
}

func (r *mockGroupWatchRepo) PendingMessages(ctx context.Context) ([]domain.BufferedMessage, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	pending := make([]domain.BufferedMessage, len(r.msgs))
	copy(pending, r.msgs)
	return pending, nil
}

func (r *mockGroupWatchRepo) MarkProcessed(ctx context.Context, phone string, messageIDs []string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	ids := make(map[string]bool, len(messageIDs))
	for _, id := range messageIDs {
		ids[id] = true
	}
	var remaining []domain.BufferedMessage
	for _, m := range r.msgs {
		if !ids[m.MessageID] {
			remaining = append(remaining, m)
		}
	}
	r.msgs = remaining
	return nil
}

func (r *mockGroupWatchRepo) CountPending(ctx context.Context) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.msgs), nil
}

type mockOrchestrator struct {
	calls atomic.Int32
}

func (m *mockOrchestrator) ClearVacancies(ctx context.Context) error { return nil }

func (m *mockOrchestrator) PopulateVacancyTexts(ctx context.Context, texts []string) (int, error) {
	m.calls.Add(1)
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
	orig := debounceDelay
	debounceDelay = 20 * time.Millisecond
	defer func() { debounceDelay = orig }()

	repo := &mockGroupWatchRepo{}
	orch := &mockOrchestrator{}
	gw := NewGroupWatcher(repo, nil, orch)

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
	orig := debounceDelay
	debounceDelay = 30 * time.Millisecond
	defer func() { debounceDelay = orig }()

	repo := &mockGroupWatchRepo{}
	orch := &mockOrchestrator{}
	gw := NewGroupWatcher(repo, nil, orch)

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
