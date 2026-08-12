package whatsapp

import (
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
