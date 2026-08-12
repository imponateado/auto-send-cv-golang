package service

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"api/internal/domain"
	"api/internal/infra/whatsapp"
)

// FlushStatus resume a última rodada de flush do buffer de mensagens.
type FlushStatus struct {
	LastRun      *time.Time `json:"last_run,omitempty"`
	LastCount    int        `json:"last_count"`
	LastError    string     `json:"last_error,omitempty"`
	PendingCount int        `json:"pending_count"`
}

// GroupWatcher liga o listener de grupos do WhatsApp ao pipeline de vagas:
// mensagens de grupos observados são bufferizadas e processadas em lote,
// num horário diário configurável ou sob demanda.
type GroupWatcher struct {
	repo         domain.GroupWatchRepository
	waManager    *whatsapp.WhatsMeowManager
	orchestrator domain.Orchestrator

	rescheduleCh chan struct{}

	statusMu  sync.RWMutex
	lastRun   time.Time
	lastCount int
	lastError string

	flushMu sync.Mutex
}

func NewGroupWatcher(repo domain.GroupWatchRepository, waManager *whatsapp.WhatsMeowManager, orchestrator domain.Orchestrator) *GroupWatcher {
	return &GroupWatcher{
		repo:         repo,
		waManager:    waManager,
		orchestrator: orchestrator,
		rescheduleCh: make(chan struct{}, 1),
	}
}

func (s *GroupWatcher) ListJoinedGroups(ctx context.Context, phone string) ([]domain.GroupInfo, error) {
	return s.waManager.ListJoinedGroups(ctx, phone)
}

func (s *GroupWatcher) GetWatchedGroups(ctx context.Context, phone string) ([]domain.WatchedGroup, error) {
	return s.repo.ListWatchedGroups(ctx, phone)
}

// SetWatchedGroups persiste o novo conjunto de grupos observados para o
// phone e reinstala o listener ao vivo com o conjunto atualizado.
func (s *GroupWatcher) SetWatchedGroups(ctx context.Context, phone string, groups []domain.GroupInfo) error {
	if err := s.repo.SetWatchedGroups(ctx, phone, groups); err != nil {
		return err
	}

	jids := make([]string, len(groups))
	for i, g := range groups {
		jids[i] = g.JID
	}

	return s.waManager.WatchGroups(ctx, phone, jids, s.onMessage)
}

// onMessage é o callback injetado no WhatsMeowManager, mantendo o pacote
// whatsapp livre de qualquer conhecimento sobre o buffer de persistência.
// Dispara a partir da goroutine de eventos do whatsmeow, fora de uma
// requisição HTTP, por isso usa context.Background().
func (s *GroupWatcher) onMessage(_ string, msg domain.BufferedMessage) {
	if err := s.repo.BufferMessage(context.Background(), msg); err != nil {
		log.Printf("[GroupWatcher] failed to buffer message: %v", err)
	}
}

func (s *GroupWatcher) GetSchedule(ctx context.Context) (domain.FlushSchedule, error) {
	return s.repo.GetSchedule(ctx)
}

// SetSchedule persiste o novo horário e acorda o scheduler para recalcular
// a próxima execução sem precisar reiniciar o servidor.
func (s *GroupWatcher) SetSchedule(ctx context.Context, sched domain.FlushSchedule) error {
	if err := s.repo.SetSchedule(ctx, sched); err != nil {
		return err
	}
	select {
	case s.rescheduleCh <- struct{}{}:
	default:
	}
	return nil
}

// FlushNow drena todas as mensagens pendentes do buffer através do
// orchestrator, num único lote de embeddings.
func (s *GroupWatcher) FlushNow(ctx context.Context) (int, error) {
	s.flushMu.Lock()
	defer s.flushMu.Unlock()

	msgs, err := s.repo.PendingMessages(ctx)
	if err != nil {
		s.recordRun(0, err)
		return 0, err
	}

	if len(msgs) == 0 {
		s.recordRun(0, nil)
		return 0, nil
	}

	texts := make([]string, len(msgs))
	idsByPhone := make(map[string][]string)
	for i, msg := range msgs {
		texts[i] = msg.Text
		idsByPhone[msg.Phone] = append(idsByPhone[msg.Phone], msg.MessageID)
	}

	count, err := s.orchestrator.PopulateVacancyTexts(ctx, texts)
	if err != nil {
		s.recordRun(count, err)
		return count, err
	}

	for phone, ids := range idsByPhone {
		if err := s.repo.MarkProcessed(ctx, phone, ids); err != nil {
			s.recordRun(count, err)
			return count, err
		}
	}

	s.recordRun(count, nil)
	return count, nil
}

func (s *GroupWatcher) recordRun(count int, err error) {
	s.statusMu.Lock()
	defer s.statusMu.Unlock()
	s.lastRun = time.Now()
	s.lastCount = count
	if err != nil {
		s.lastError = err.Error()
	} else {
		s.lastError = ""
	}
}

// Status é uma simplificação deliberada: last_run/last_count/last_error
// vivem só em memória e resetam num restart do servidor. Upgrade path:
// persistir numa coluna da tabela group_flush_schedule se isso importar.
func (s *GroupWatcher) Status(ctx context.Context) (FlushStatus, error) {
	pending, err := s.repo.CountPending(ctx)
	if err != nil {
		return FlushStatus{}, err
	}

	s.statusMu.RLock()
	defer s.statusMu.RUnlock()

	status := FlushStatus{LastCount: s.lastCount, LastError: s.lastError, PendingCount: pending}
	if !s.lastRun.IsZero() {
		lastRun := s.lastRun
		status.LastRun = &lastRun
	}
	return status, nil
}

// Bootstrap reinstala os listeners ao vivo para todo phone com pelo menos
// um grupo observado — necessário depois de um restart do servidor.
func (s *GroupWatcher) Bootstrap(ctx context.Context) error {
	phones, err := s.repo.ListAllWatchedPhones(ctx)
	if err != nil {
		return fmt.Errorf("failed to list watched phones: %w", err)
	}

	for _, phone := range phones {
		groups, err := s.repo.ListWatchedGroups(ctx, phone)
		if err != nil {
			log.Printf("[GroupWatcher] failed to load watched groups for %s: %v", phone, err)
			continue
		}

		jids := make([]string, len(groups))
		for i, g := range groups {
			jids[i] = g.GroupJID
		}

		if err := s.waManager.WatchGroups(ctx, phone, jids, s.onMessage); err != nil {
			log.Printf("[GroupWatcher] failed to watch groups for %s: %v", phone, err)
		}
	}

	return nil
}

func nextOccurrence(now time.Time, hour, minute int) time.Duration {
	next := time.Date(now.Year(), now.Month(), now.Day(), hour, minute, 0, 0, now.Location())
	if !next.After(now) {
		next = next.Add(24 * time.Hour)
	}
	return next.Sub(now)
}

// RunScheduler bloqueia, acordando no horário configurado (HH:MM) a cada
// dia — ou imediatamente ao recalcular via SetSchedule — para disparar
// FlushNow. Encerra quando ctx é cancelado (shutdown do servidor).
func (s *GroupWatcher) RunScheduler(ctx context.Context) {
	for {
		sched, err := s.repo.GetSchedule(context.Background())
		wait := 24 * time.Hour
		if err == nil {
			wait = nextOccurrence(time.Now(), sched.Hour, sched.Minute)
		}

		select {
		case <-time.After(wait):
			if _, err := s.FlushNow(context.Background()); err != nil {
				log.Printf("[GroupWatcher] scheduled flush failed: %v", err)
			}
		case <-s.rescheduleCh:
			continue
		case <-ctx.Done():
			return
		}
	}
}
