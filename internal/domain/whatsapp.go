package domain

import "context"

type WhatsAppStatus struct {
	Phone  string `json:"phone"`
	Status string `json:"status"`
	JID    string `json:"jid,omitempty"`
}

type WhatsAppService interface {
	SendMessage(ctx context.Context, to string, message string) error
}

type WhatsAppManager interface {
	SendMessage(ctx context.Context, phoneSender string, to string, message string) error
	SendDocument(ctx context.Context, phoneSender string, to string, caption string, fileBytes []byte, filename string) error
}
