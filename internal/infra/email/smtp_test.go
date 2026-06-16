package email

import (
	"context"
	"testing"
)

func TestSMTPSender_SendMessage_Misconfiguration(t *testing.T) {
	sender := NewSMTPSender("", "", "", "", "")
	err := sender.SendEmail(context.Background(), "test@example.com", "hello", "world", "", "")
	if err == nil {
		t.Fatal("expected error on misconfiguration, got nil")
	}
}
