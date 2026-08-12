package domain

import (
	"context"
	"time"
)

// GroupInfo é o subconjunto dos metadados de um grupo do WhatsApp que a app usa.
type GroupInfo struct {
	JID  string `json:"jid"`
	Name string `json:"name"`
}

// WatchedGroup é um grupo que um número está configurado para escutar.
type WatchedGroup struct {
	Phone     string    `json:"phone"`
	GroupJID  string    `json:"group_jid"`
	GroupName string    `json:"group_name"`
	CreatedAt time.Time `json:"created_at"`
}

// BufferedMessage é uma mensagem de texto de grupo capturada pelo listener
// ao vivo, aguardando o próximo flush (agendado ou manual) para o pipeline de vagas.
type BufferedMessage struct {
	MessageID  string
	Phone      string
	GroupJID   string
	SenderJID  string
	Text       string
	ReceivedAt time.Time
}

// FlushSchedule é o horário diário único e global em que o buffer é processado.
type FlushSchedule struct {
	Hour   int `json:"hour"`
	Minute int `json:"minute"`
}

// GroupWatchRepository persiste grupos observados, o buffer de mensagens
// pendentes e o agendamento global de flush.
type GroupWatchRepository interface {
	ListWatchedGroups(ctx context.Context, phone string) ([]WatchedGroup, error)
	ListAllWatchedPhones(ctx context.Context) ([]string, error)
	SetWatchedGroups(ctx context.Context, phone string, groups []GroupInfo) error

	BufferMessage(ctx context.Context, msg BufferedMessage) error
	PendingMessages(ctx context.Context) ([]BufferedMessage, error)
	MarkProcessed(ctx context.Context, phone string, messageIDs []string) error
	CountPending(ctx context.Context) (int, error)

	GetSchedule(ctx context.Context) (FlushSchedule, error)
	SetSchedule(ctx context.Context, sched FlushSchedule) error
}
