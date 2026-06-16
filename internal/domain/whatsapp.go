package domain

import "context"

// WhatsAppService defines the domain contract for sending WhatsApp messages.
type WhatsAppService interface {
	SendMessage(ctx context.Context, to string, message string) error
}
