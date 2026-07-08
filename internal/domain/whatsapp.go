package domain

import "context"

type WhatsAppStatus struct {
	Phone  string `json:"phone"`
	Status string `json:"status"`
	JID    string `json:"jid,omitempty"`
}

// WhatsAppService defines the domain contract for sending WhatsApp messages.
type WhatsAppService interface {
	SendMessage(ctx context.Context, to string, message string) error
}

// WhatsAppManager defines the domain contract for managing multiple WhatsApp client connections.
type WhatsAppManager interface {
	SendMessage(ctx context.Context, phoneSender string, to string, message string) error
}
