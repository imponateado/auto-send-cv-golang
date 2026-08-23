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

type FlushStatus struct {
	LastRun      *time.Time `json:"last_run,omitempty"`
	LastCount    int        `json:"last_count"`
	LastError    string     `json:"last_error,omitempty"`
	PendingCount int        `json:"pending_count"`
}

var debounceDelay = 10 * time.Minute

type GroupWatcher struct {
	repo         domain.GroupWatchRepository
	waManager    *whatsapp.WhatsMeowManager
	orchestrator domain.Orchestrator

	debounceMu    sync.Mutex
	debounceTimer *time.Timer

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
	}
}

func (s *GroupWatcher) ListJoinedGroups(ctx context.Context, phone string) ([]domain.GroupInfo, error) {
	return s.waManager.ListJoinedGroups(ctx, phone)
}

func (s *GroupWatcher) GetWatchedGroups(ctx context.Context, phone string) ([]domain.WatchedGroup, error) {
	return s.repo.ListWatchedGroups(ctx, phone)
}

func (s *GroupWatcher) ListWatchedPhones(ctx context.Context) ([]string, error) {
	return s.repo.ListAllWatchedPhones(ctx)
}

// SetWatchedGroups persiste o conjunto de grupos observados para phone e reinstala
// o listener ao vivo com o conjunto atualizado. Retorna erro se a persistência ou
// a reinstalação do listener falhar.
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

// onMessage grave msg no buffer de persistência e reinicia o debounce do flush automático. Não retorna nada ao chamador; erros de persistência só são logados.
func (s *GroupWatcher) onMessage(_ string, msg domain.BufferedMessage) {
	if err := s.repo.BufferMessage(context.Background(), msg); err != nil {
		log.Printf("[GroupWatcher] failed to buffer message: %v", err)
	}
	s.resetDebounce()
}

// resetDebounce reinicia o timer de debounce do flush automático: se debounceDelay se passar sem uma nova mensagem, dispara FlushNow sozinho.
func (s *GroupWatcher) resetDebounce() {
	s.debounceMu.Lock()
	defer s.debounceMu.Unlock()
	if s.debounceTimer != nil {
		s.debounceTimer.Stop()
	}
	s.debounceTimer = time.AfterFunc(debounceDelay, func() {
		if _, err := s.FlushNow(context.Background()); err != nil {
			log.Printf("[GroupWatcher] debounced flush failed: %v", err)
		}
	})
}

// FlushNow drena as mensagens pendentes do buffer através do orchestrator, num
// único lote de embeddings. Retorna a quantidade de mensagens processadas e um
// erro se a leitura, o processamento ou a marcação como processada falhar.
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

// Status monta o FlushStatus atual (contagem pendente mais o resultado da
// última rodada de flush em memória). Retorna erro apenas se a contagem de
// pendências falhar.
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

// Bootstrap reinstala os listeners ao vivo para todo phone com pelo menos um
// grupo observado. Retorna erro apenas se listar os phones observados falhar;
// falhas por phone individual só são logadas.
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
