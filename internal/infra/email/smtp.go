package email

import (
	"bytes"
	"context"
	"fmt"
	"net/smtp"
	"strings"

	"api/internal/domain"
)

type smtpSender struct {
	host     string
	port     string
	username string
	password string
	sender   string
}

// NewSMTPSender creates a new instance of domain.EmailService using SMTP.
func NewSMTPSender(host, port, username, password, sender string) domain.EmailService {
	return &smtpSender{
		host:     host,
		port:     port,
		username: username,
		password: password,
		sender:   sender,
	}
}

// SendEmail sends an email using Go's native net/smtp package. It supports sending attachments in Base64 format.
func (s *smtpSender) SendEmail(ctx context.Context, to string, subject string, body string, attachmentB64 string, attachmentName string) error {
	// Respect context cancellation
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	if s.host == "" || s.port == "" {
		return fmt.Errorf("smtp sender is misconfigured: host and port are required")
	}

	auth := smtp.PlainAuth("", s.username, s.password, s.host)
	addr := fmt.Sprintf("%s:%s", s.host, s.port)

	var msg []byte

	if attachmentB64 != "" {
		boundary := "my-multipart-boundary-12345"

		// Clean Data URI prefix if present
		base64Data := attachmentB64
		if idx := strings.Index(base64Data, ","); idx != -1 {
			base64Data = base64Data[idx+1:]
		}

		// Strip any whitespace/newlines
		base64Data = strings.Join(strings.Fields(base64Data), "")

		if attachmentName == "" {
			attachmentName = "curriculo.pdf"
		}

		header := fmt.Sprintf("To: %s\r\n"+
			"Subject: %s\r\n"+
			"MIME-Version: 1.0\r\n"+
			"Content-Type: multipart/mixed; boundary=%s\r\n"+
			"\r\n"+
			"--%s\r\n"+
			"Content-Type: text/plain; charset=UTF-8\r\n"+
			"Content-Transfer-Encoding: 7bit\r\n"+
			"\r\n"+
			"%s\r\n"+
			"\r\n"+
			"--%s\r\n"+
			"Content-Type: application/octet-stream; name=\"%s\"\r\n"+
			"Content-Transfer-Encoding: base64\r\n"+
			"Content-Disposition: attachment; filename=\"%s\"\r\n"+
			"\r\n", to, subject, boundary, boundary, body, boundary, attachmentName, attachmentName)

		var buf bytes.Buffer
		buf.WriteString(header)

		// Split base64 into lines of 76 characters for RFC compliant transport
		for i := 0; i < len(base64Data); i += 76 {
			end := i + 76
			if end > len(base64Data) {
				end = len(base64Data)
			}
			buf.WriteString(base64Data[i:end])
			buf.WriteString("\r\n")
		}

		buf.WriteString(fmt.Sprintf("--%s--\r\n", boundary))
		msg = buf.Bytes()
	} else {
		// Simple text email
		msg = []byte(fmt.Sprintf("To: %s\r\n"+
			"Subject: %s\r\n"+
			"Content-Type: text/plain; charset=UTF-8\r\n"+
			"\r\n"+
			"%s\r\n", to, subject, body))
	}

	err := smtp.SendMail(addr, auth, s.sender, []string{to}, msg)
	if err != nil {
		return fmt.Errorf("smtp send mail failed: %w", err)
	}

	return nil
}
