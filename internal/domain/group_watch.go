package domain

import (
	"context"
	"time"
)

type GroupInfo struct {
	JID  string `json:"jid"`
	Name string `json:"name"`
}

type WatchedGroup struct {
	Phone     string    `json:"phone"`
	GroupJID  string    `json:"group_jid"`
	GroupName string    `json:"group_name"`
	CreatedAt time.Time `json:"created_at"`
}

type BufferedMessage struct {
	MessageID  string
	Phone      string
	GroupJID   string
	SenderJID  string
	Text       string
	ReceivedAt time.Time
}

type GroupWatchRepository interface {
	ListWatchedGroups(ctx context.Context, phone string) ([]WatchedGroup, error)
	ListAllWatchedPhones(ctx context.Context) ([]string, error)
	SetWatchedGroups(ctx context.Context, phone string, groups []GroupInfo) error

	// ArchiveMessages grava as mensagens que já viraram vaga. É registro
	// histórico de proveniência (quem postou, em qual grupo, quando), não fila de
	// trabalho — o buffer de trabalho vive em memória no GroupWatcher.
	ArchiveMessages(ctx context.Context, msgs []BufferedMessage) error
}
