package email

import (
	"encoding/base64"
	"io"
	"mime"
	"mime/multipart"
	"net/mail"
	"strings"
	"testing"
)

func TestBuildRawRFC822PreservesAccents(t *testing.T) {
	body := "Motivo: experiência sólida em manutenção e automação."
	raw, err := buildRawRFC822("de@x.com", "para@y.com", "Candidatura - Processamento Automático", body, "aGVsbG8=", "curriculo.pdf")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	msg, err := mail.ReadMessage(strings.NewReader(string(raw)))
	if err != nil {
		t.Fatalf("mensagem RFC822 inválida: %v", err)
	}

	subject, err := new(mime.WordDecoder).DecodeHeader(msg.Header.Get("Subject"))
	if err != nil {
		t.Fatalf("subject não decodifica como RFC 2047: %v", err)
	}
	if !strings.Contains(subject, "Automático") {
		t.Errorf("acento perdido no subject: %q", subject)
	}

	_, params, err := mime.ParseMediaType(msg.Header.Get("Content-Type"))
	if err != nil {
		t.Fatalf("Content-Type inválido: %v", err)
	}

	mr := multipart.NewReader(msg.Body, params["boundary"])

	textPart, err := mr.NextPart()
	if err != nil {
		t.Fatalf("parte do texto ausente: %v", err)
	}
	if got := textPart.Header.Get("Content-Transfer-Encoding"); got != "base64" {
		t.Fatalf("corpo UTF-8 precisa de CTE base64, veio %q", got)
	}
	// multipart só decodifica quoted-printable de forma transparente; base64 vem
	// cru, então decodificamos aqui — o que também prova que a quebra em 76
	// colunas do writeWrapped é base64 válido.
	gotBody, err := io.ReadAll(base64.NewDecoder(base64.StdEncoding, textPart))
	if err != nil {
		t.Fatalf("erro lendo o corpo: %v", err)
	}
	if string(gotBody) != body {
		t.Errorf("corpo alterado no transporte:\nwant %q\ngot %q", body, gotBody)
	}

	attachPart, err := mr.NextPart()
	if err != nil {
		t.Fatalf("parte do anexo ausente: %v", err)
	}
	if got := attachPart.FileName(); got != "curriculo.pdf" {
		t.Errorf("filename do anexo: want curriculo.pdf, got %q", got)
	}
	gotAttach, err := io.ReadAll(base64.NewDecoder(base64.StdEncoding, attachPart))
	if err != nil {
		t.Fatalf("erro lendo anexo: %v", err)
	}
	if string(gotAttach) != "hello" {
		t.Errorf("anexo corrompido: want %q, got %q", "hello", gotAttach)
	}
}
