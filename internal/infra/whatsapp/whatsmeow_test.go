package whatsapp

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"go.mau.fi/whatsmeow/proto/waE2E"
	"google.golang.org/protobuf/proto"
)

func TestExtractText(t *testing.T) {
	cases := []struct {
		name string
		msg  *waE2E.Message
		want string
	}{
		{
			name: "nil message",
			msg:  nil,
			want: "",
		},
		{
			name: "plain conversation",
			msg:  &waE2E.Message{Conversation: proto.String("vaga: dev go")},
			want: "vaga: dev go",
		},
		{
			name: "extended text message",
			msg: &waE2E.Message{
				ExtendedTextMessage: &waE2E.ExtendedTextMessage{Text: proto.String("vaga com link")},
			},
			want: "vaga com link",
		},
		{
			name: "neither present",
			msg:  &waE2E.Message{ImageMessage: &waE2E.ImageMessage{Caption: proto.String("legenda ignorada")}},
			want: "",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := extractText(tc.msg); got != tc.want {
				t.Errorf("extractText() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestBuildRecipientJID(t *testing.T) {
	cases := []struct {
		name string
		to   string
		want string
	}{
		{name: "already a JID", to: "5561992055310@s.whatsapp.net", want: "5561992055310@s.whatsapp.net"},
		{name: "formatted BR number without country code", to: "(61) 99205-5310", want: "5561992055310@s.whatsapp.net"},
		{name: "plain digits without country code", to: "61992055310", want: "5561992055310@s.whatsapp.net"},
		{name: "already has country code", to: "5561992055310", want: "5561992055310@s.whatsapp.net"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := buildRecipientJID(tc.to)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got.String() != tc.want {
				t.Errorf("buildRecipientJID(%q) = %q, want %q", tc.to, got.String(), tc.want)
			}
		})
	}
}

// Pedir para enviar por um número não pareado precisa falhar dizendo isso, em
// vez de fabricar um device em branco que nunca vai conseguir enviar nada — era
// o que produzia o enganoso "candidate whatsapp client is not authenticated".
func TestEnsureClientRefusesUnpairedPhoneWhenNotCreating(t *testing.T) {
	m, err := NewWhatsMeowManager(filepath.Join(t.TempDir(), "wa.db"))
	if err != nil {
		t.Fatalf("failed to create manager: %v", err)
	}
	ctx := context.Background()

	before, err := m.container.GetAllDevices(ctx)
	if err != nil {
		t.Fatalf("failed to list devices: %v", err)
	}

	_, err = m.ensureClient(ctx, "5561999999999", false)
	if err == nil {
		t.Fatal("esperava erro para número não pareado")
	}
	if !strings.Contains(err.Error(), "não está pareado") {
		t.Errorf("erro deve dizer que o número não está pareado, got: %v", err)
	}

	after, err := m.container.GetAllDevices(ctx)
	if err != nil {
		t.Fatalf("failed to list devices: %v", err)
	}
	if len(after) != len(before) {
		t.Errorf("não pode criar device para número não pareado: antes %d, depois %d", len(before), len(after))
	}

	m.clientsMu.Lock()
	tracked := len(m.clients)
	m.clientsMu.Unlock()
	if tracked != 0 {
		t.Errorf("não pode registrar client para número não pareado, got %d", tracked)
	}
}

// Um destino que já é JID explícito não pode disparar consulta ao servidor: ele
// já está resolvido, e no caso de um @lid a consulta por número nem faria
// sentido. O client nil aqui é justamente a prova de que nada é chamado nele.
func TestResolveRecipientSkipsLookupForExplicitJID(t *testing.T) {
	cases := []string{
		"5561992055310@s.whatsapp.net",
		"184095216803890@lid",
	}

	for _, to := range cases {
		t.Run(to, func(t *testing.T) {
			got, err := resolveRecipient(context.Background(), nil, to)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got.String() != to {
				t.Errorf("JID explícito deve passar intacto: want %q, got %q", to, got.String())
			}
		})
	}
}
