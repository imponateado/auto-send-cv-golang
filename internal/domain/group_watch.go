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

	BufferMessage(ctx context.Context, msg BufferedMessage) error
	PendingMessages(ctx context.Context) ([]BufferedMessage, error)
	MarkProcessed(ctx context.Context, phone string, messageIDs []string) error
	CountPending(ctx context.Context) (int, error)
}
