package service

import (
	"context"
	"encoding/base64"
	"fmt"
	"log"
	"os"
	"strings"
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

const defaultDebounceDelay = 10 * time.Minute

// debounceDelayFromEnv lê GROUP_FLUSH_DEBOUNCE como uma duração ("2m", "10m",
// "30s"). Valor ausente ou inválido cai no default, logando o motivo — um typo
// na variável não pode virar um watcher que nunca dispara.
func debounceDelayFromEnv() time.Duration {
	raw := os.Getenv("GROUP_FLUSH_DEBOUNCE")
	if raw == "" {
		return defaultDebounceDelay
	}

	d, err := time.ParseDuration(raw)
	if err != nil || d <= 0 {
		log.Printf("[GroupWatcher] GROUP_FLUSH_DEBOUNCE inválido (%q), usando o default de %s", raw, defaultDebounceDelay)
		return defaultDebounceDelay
	}
	return d
}

type GroupWatcher struct {
	repo         domain.GroupWatchRepository
	waManager    *whatsapp.WhatsMeowManager
	orchestrator domain.Orchestrator
	llm          domain.GeminiService

	debounceDelay time.Duration
	debounceMu    sync.Mutex
	debounceTimer *time.Timer

	// ponytail: o buffer de mensagens vive só em memória entre o recebimento e o
	// flush. Um restart dentro da janela de debounce descarta a rajada inteira —
	// aceito deliberadamente em troca de não escrever no disco a cada mensagem (e
	// de tirar I/O do caminho de despacho de eventos do whatsmeow). Upgrade path:
	// voltar a persistir na chegada, com uma goroutine escritora separada para
	// não bloquear o handler.
	pendingMu sync.Mutex
	pending   []domain.BufferedMessage

	statusMu  sync.RWMutex
	lastRun   time.Time
	lastCount int
	lastError string

	flushMu sync.Mutex
	// ponytail: lastFlushDay só vive em memória, então um restart entre dois
	// flushes do mesmo dia faz o próximo flush limpar as vagas sem precisar.
	// Com uma rajada diária isso é inofensivo. Upgrade path: derivar de
	// MAX(processed_at) do group_message_buffer.
	lastFlushDay time.Time
}

func NewGroupWatcher(repo domain.GroupWatchRepository, waManager *whatsapp.WhatsMeowManager, orchestrator domain.Orchestrator, llm domain.GeminiService) *GroupWatcher {
	delay := debounceDelayFromEnv()
	log.Printf("[GroupWatcher] Flush automático após %s de silêncio no grupo.", delay)

	return &GroupWatcher{
		repo:          repo,
		waManager:     waManager,
		orchestrator:  orchestrator,
		llm:           llm,
		debounceDelay: delay,
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

// onMessage acumula msg no buffer em memória e reinicia o debounce do flush.
// Não persiste nada: o disco só é tocado no flush. Roda dentro do handler de
// eventos do whatsmeow, então precisa ser barato. Mensagem repetida é ignorada
// e não adia o flush.
func (s *GroupWatcher) onMessage(_ string, msg domain.BufferedMessage) {
	s.pendingMu.Lock()
	// ponytail: varredura linear no lugar do INSERT OR IGNORE na PK
	// (phone, message_id) que deduplicava reentrega do mesmo evento. É O(n) por
	// mensagem, com n limitado ao tamanho da rajada (~200). Upgrade path: um
	// map[string]struct{} limpo junto com o slice, se as rajadas crescerem
	// ordens de grandeza.
	duplicate := false
	for _, m := range s.pending {
		if m.Phone == msg.Phone && m.MessageID == msg.MessageID {
			duplicate = true
			break
		}
	}
	if !duplicate {
		s.pending = append(s.pending, msg)
	}
	s.pendingMu.Unlock()

	if duplicate {
		return
	}
	s.resetDebounce()
}

// resetDebounce reinicia o timer de debounce do flush automático: se
// s.debounceDelay se passar sem uma nova mensagem, dispara FlushNow sozinho.
func (s *GroupWatcher) resetDebounce() {
	s.debounceMu.Lock()
	defer s.debounceMu.Unlock()
	if s.debounceTimer != nil {
		s.debounceTimer.Stop()
	}
	s.debounceTimer = time.AfterFunc(s.debounceDelay, func() {
		if _, err := s.FlushNow(context.Background()); err != nil {
			log.Printf("[GroupWatcher] debounced flush failed: %v", err)
		}
	})
}

// FlushNow processa o buffer em memória: popula o banco vetorial e só então
// arquiva as mensagens no SQLite. Se o processamento falhar, as mensagens
// continuam no buffer para a próxima tentativa. Retorna quantas vagas foram
// salvas e o erro que interrompeu a rodada.
func (s *GroupWatcher) FlushNow(ctx context.Context) (int, error) {
	s.flushMu.Lock()
	defer s.flushMu.Unlock()

	s.pendingMu.Lock()
	msgs := make([]domain.BufferedMessage, len(s.pending))
	copy(msgs, s.pending)
	s.pendingMu.Unlock()

	if len(msgs) == 0 {
		s.recordRun(0, nil)
		return 0, nil
	}

	// OCR das imagens antes de indexar. Escreve de volta em msgs[i].Text para que
	// o arquivamento guarde exatamente o texto que virou vaga.
	s.ocrPending(ctx, msgs)

	texts := make([]string, len(msgs))
	for i, msg := range msgs {
		texts[i] = msg.Text
	}

	// Vagas valem por um dia: o primeiro flush de cada dia começa do zero.
	if now := time.Now(); !sameDay(s.lastFlushDay, now) {
		if err := s.orchestrator.ClearVacancies(ctx); err != nil {
			s.recordRun(0, err)
			return 0, fmt.Errorf("failed to clear previous day vacancies: %w", err)
		}
		s.lastFlushDay = now
	}

	// Popula primeiro: se falhar, as mensagens seguem no buffer para retry.
	count, err := s.orchestrator.PopulateVacancyTexts(ctx, texts)
	if err != nil {
		s.recordRun(count, err)
		return count, err
	}

	// Arquiva só o que já virou vaga. Falha aqui perde proveniência, não vaga —
	// então loga e segue, em vez de reprocessar tudo no próximo flush.
	if err := s.repo.ArchiveMessages(ctx, msgs); err != nil {
		log.Printf("[GroupWatcher] failed to archive %d processed messages: %v", len(msgs), err)
	}

	// Descarta exatamente o prefixo processado; mensagens que chegaram durante o
	// flush ficam para a próxima rodada.
	s.pendingMu.Lock()
	s.pending = s.pending[len(msgs):]
	s.pendingMu.Unlock()

	s.recordRun(count, nil)
	return count, nil
}

// ocrMinInterval espaça as chamadas de OCR. O free tier do Gemini são 10
// requisições por minuto; sem intervalo a rajada toma 429 e todo print falha.
// Este caminho já é assíncrono — só roda depois do debounce, sem ninguém
// esperando HTTP — então minutos aqui não custam nada. Em tier pago pode ir a 0.
const ocrMinInterval = 6 * time.Second

// ocrPending transcreve as imagens da rajada e concatena o resultado na legenda,
// escrevendo em msgs[i].Text. Falha de uma imagem não interrompe o flush: loga e
// segue com a legenda, e se não sobrar texto o PopulateVacancyTexts já descarta
// string vazia.
func (s *GroupWatcher) ocrPending(ctx context.Context, msgs []domain.BufferedMessage) {
	if s.llm == nil {
		return
	}

	first := true
	for i := range msgs {
		if len(msgs[i].Image) == 0 {
			continue
		}
		if !first {
			time.Sleep(ocrMinInterval)
		}
		first = false

		b64 := base64.StdEncoding.EncodeToString(msgs[i].Image)
		text, err := s.llm.ExtractText(ctx, b64, msgs[i].ImageMime)
		if err != nil {
			log.Printf("[GroupWatcher] OCR falhou na mensagem %s: %v", msgs[i].MessageID, err)
			continue
		}

		msgs[i].Text = strings.TrimSpace(msgs[i].Text + "\n" + text)
	}
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

// Status monta o FlushStatus atual: quantas mensagens estão no buffer em memória
// mais o resultado da última rodada de flush. O erro é sempre nil hoje, mantido
// na assinatura porque o handler já o trata.
func (s *GroupWatcher) Status(ctx context.Context) (FlushStatus, error) {
	s.pendingMu.Lock()
	pending := len(s.pending)
	s.pendingMu.Unlock()

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
// falhas por phone individual só são logadas. Não há buffer a recuperar: o que
// não tinha sido processado antes do restart morreu com o processo.
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

func sameDay(a, b time.Time) bool {
	ay, am, ad := a.Date()
	by, bm, bd := b.Date()
	return ay == by && am == bm && ad == bd
}
