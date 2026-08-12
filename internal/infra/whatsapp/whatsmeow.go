package whatsapp

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"api/internal/domain"
	"github.com/skip2/go-qrcode"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/store"
	"go.mau.fi/whatsmeow/store/sqlstore"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	waLog "go.mau.fi/whatsmeow/util/log"
	_ "modernc.org/sqlite"
	"google.golang.org/protobuf/proto"
)

type WhatsMeowManager struct {
	container       *sqlstore.Container
	clients         map[string]*whatsmeow.Client
	clientsMu       sync.Mutex
	qrCodes         map[string]string
	qrCodesMu       sync.RWMutex
	groupHandlers   map[string]uint32
	groupHandlersMu sync.Mutex
}

// GroupMessageCallback é invocado para cada mensagem de texto vista num
// grupo observado para o phone dado. O texto já vem extraído; mensagens
// vazias/não-texto são filtradas antes desta chamada.
type GroupMessageCallback func(phone string, msg domain.BufferedMessage)

func NewWhatsMeowManager(dbPath string) (*WhatsMeowManager, error) {
	// sqlstore.New takes context, dialect, dsn, and logger in this version
	container, err := sqlstore.New(context.Background(), "sqlite", "file:"+dbPath+"?_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)", waLog.Stdout("Database", "WARN", true))
	if err != nil {
		return nil, fmt.Errorf("failed to initialize whatsmeow sqlstore: %w", err)
	}

	return &WhatsMeowManager{
		container:     container,
		clients:       make(map[string]*whatsmeow.Client),
		qrCodes:       make(map[string]string),
		groupHandlers: make(map[string]uint32),
	}, nil
}

func (m *WhatsMeowManager) EnsureClient(ctx context.Context, phone string) (*whatsmeow.Client, error) {
	m.clientsMu.Lock()
	defer m.clientsMu.Unlock()

	if cli, exists := m.clients[phone]; exists {
		if cli.IsConnected() {
			return cli, nil
		}
		err := cli.Connect()
		if err != nil {
			return nil, fmt.Errorf("failed to connect existing whatsmeow client: %w", err)
		}
		return cli, nil
	}

	// GetAllDevices takes context
	devices, err := m.container.GetAllDevices(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list devices from db: %w", err)
	}

	var devStore *store.Device
	for _, dev := range devices {
		if dev.ID != nil && dev.ID.User == phone {
			devStore = dev
			break
		}
	}

	if devStore == nil {
		devStore = m.container.NewDevice()
	}

	cli := whatsmeow.NewClient(devStore, waLog.Stdout("Client", "DEBUG", true))
	m.clients[phone] = cli

	if cli.Store.ID != nil {
		err := cli.Connect()
		if err != nil {
			return nil, fmt.Errorf("failed to auto-connect whatsmeow client: %w", err)
		}
	}

	return cli, nil
}

func (m *WhatsMeowManager) GetQR(ctx context.Context, phone string) ([]byte, error) {
	cli, err := m.EnsureClient(ctx, phone)
	if err != nil {
		return nil, err
	}

	if cli.IsLoggedIn() {
		return nil, errors.New("already authenticated")
	}

	// whatsmeow requires GetQRChannel to be called before Connect. EnsureClient
	// auto-connects any pre-existing, not-yet-logged-in client (e.g. left over
	// from a previous QR attempt that expired), so a retry here can find a
	// client that's already connecting/connected. Reset it before asking for
	// a fresh QR channel.
	if cli.IsConnected() {
		cli.Disconnect()
	}

	// The QR channel and the connection it drives must outlive this HTTP
	// request: whatsmeow disconnects the client as soon as the context it
	// was given is cancelled, which happens the instant this handler
	// returns if we used the request's context here.
	pairCtx := context.Background()

	qrChan, err := cli.GetQRChannel(pairCtx)
	if err != nil {
		m.qrCodesMu.RLock()
		qrStr, exists := m.qrCodes[phone]
		m.qrCodesMu.RUnlock()
		if exists && qrStr != "" {
			return qrcode.Encode(qrStr, qrcode.Medium, -8)
		}
		return nil, fmt.Errorf("failed to get qr channel: %w", err)
	}

	if !cli.IsConnected() {
		go func() {
			_ = cli.Connect()
		}()
	}

	// WhatsApp rotates the QR code roughly every 20s (60s for the last one)
	// and disconnects the client once 8 unread codes pile up in the channel,
	// so this must keep draining qrChan for the whole pairing attempt rather
	// than reading a single code and abandoning the rest.
	firstCode := make(chan string, 1)
	go func() {
		gotFirst := false
		for item := range qrChan {
			if item.Event != "code" {
				if item.Error != nil {
					fmt.Printf("whatsapp pairing for %s ended with event %q: %v\n", phone, item.Event, item.Error)
				} else {
					fmt.Printf("whatsapp pairing for %s ended with event %q\n", phone, item.Event)
				}
				break
			}
			m.qrCodesMu.Lock()
			m.qrCodes[phone] = item.Code
			m.qrCodesMu.Unlock()
			if !gotFirst {
				gotFirst = true
				firstCode <- item.Code
			}
		}
		m.qrCodesMu.Lock()
		delete(m.qrCodes, phone)
		m.qrCodesMu.Unlock()
	}()

	select {
	case code := <-firstCode:
		return qrcode.Encode(code, qrcode.Medium, -8)
	case <-time.After(10 * time.Second):
		return nil, errors.New("timeout waiting for QR code generation")
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (m *WhatsMeowManager) GetStatus(ctx context.Context, phone string) (domain.WhatsAppStatus, error) {
	cli, err := m.EnsureClient(ctx, phone)
	if err != nil {
		return domain.WhatsAppStatus{Phone: phone, Status: "unauthenticated"}, nil
	}

	status := "unauthenticated"
	if cli.IsLoggedIn() {
		status = "authenticated"
	} else if cli.IsConnected() {
		status = "connecting"
	}

	jidStr := ""
	if cli.Store.ID != nil {
		jidStr = cli.Store.ID.String()
	}

	return domain.WhatsAppStatus{
		Phone:  phone,
		Status: status,
		JID:    jidStr,
	}, nil
}

// ListConnected returns one entry per phone number that has ever completed
// WhatsApp pairing (i.e. has a device persisted in the store), without the
// side effect of connecting clients that aren't already running.
func (m *WhatsMeowManager) ListConnected(ctx context.Context) ([]domain.WhatsAppStatus, error) {
	devices, err := m.container.GetAllDevices(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list devices from db: %w", err)
	}

	statuses := make([]domain.WhatsAppStatus, 0, len(devices))
	for _, dev := range devices {
		if dev.ID == nil {
			continue
		}
		phone := dev.ID.User

		m.clientsMu.Lock()
		cli, tracked := m.clients[phone]
		m.clientsMu.Unlock()

		status := "authenticated"
		if tracked {
			if cli.IsLoggedIn() {
				status = "authenticated"
			} else if cli.IsConnected() {
				status = "connecting"
			} else {
				status = "unauthenticated"
			}
		}

		statuses = append(statuses, domain.WhatsAppStatus{
			Phone:  phone,
			Status: status,
			JID:    dev.ID.String(),
		})
	}

	return statuses, nil
}

// ListJoinedGroups retorna os grupos dos quais a conta do phone dado é membro.
func (m *WhatsMeowManager) ListJoinedGroups(ctx context.Context, phone string) ([]domain.GroupInfo, error) {
	cli, err := m.EnsureClient(ctx, phone)
	if err != nil {
		return nil, err
	}

	groups, err := cli.GetJoinedGroups(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list joined groups: %w", err)
	}

	infos := make([]domain.GroupInfo, 0, len(groups))
	for _, g := range groups {
		infos = append(infos, domain.GroupInfo{JID: g.JID.String(), Name: g.Name})
	}
	return infos, nil
}

// WatchGroups (re)instala o handler de mensagens de grupo para o phone dado,
// encaminhando mensagens de texto de qualquer JID em watchedJIDs para
// onMessage. Chamar de novo sempre que o conjunto observado mudar — remove
// o handler anterior antes de instalar o novo.
func (m *WhatsMeowManager) WatchGroups(ctx context.Context, phone string, watchedJIDs []string, onMessage GroupMessageCallback) error {
	cli, err := m.EnsureClient(ctx, phone)
	if err != nil {
		return err
	}

	watchedSet := make(map[string]bool, len(watchedJIDs))
	for _, jid := range watchedJIDs {
		watchedSet[jid] = true
	}

	m.groupHandlersMu.Lock()
	defer m.groupHandlersMu.Unlock()

	if oldID, exists := m.groupHandlers[phone]; exists {
		go cli.RemoveEventHandler(oldID)
	}

	id := cli.AddEventHandler(func(evt any) {
		v, ok := evt.(*events.Message)
		if !ok || !v.Info.IsGroup || v.Info.IsFromMe {
			return
		}
		if !watchedSet[v.Info.Chat.String()] {
			return
		}
		text := extractText(v.Message)
		if text == "" {
			return
		}
		onMessage(phone, domain.BufferedMessage{
			MessageID:  v.Info.ID,
			Phone:      phone,
			GroupJID:   v.Info.Chat.String(),
			SenderJID:  v.Info.Sender.String(),
			Text:       text,
			ReceivedAt: v.Info.Timestamp,
		})
	})
	m.groupHandlers[phone] = id

	return nil
}

// extractText extrai o texto puro de uma mensagem do whatsmeow, cobrindo
// apenas conversas simples e respostas/links formatados (ExtendedTextMessage).
// Legendas de mídia (imagem/documento/vídeo) ficam fora do escopo por ora.
func extractText(msg *waE2E.Message) string {
	if msg == nil {
		return ""
	}
	if c := msg.GetConversation(); c != "" {
		return c
	}
	return msg.GetExtendedTextMessage().GetText()
}

func (m *WhatsMeowManager) Disconnect(ctx context.Context, phone string) error {
	m.clientsMu.Lock()
	defer m.clientsMu.Unlock()

	cli, exists := m.clients[phone]
	if !exists {
		return nil
	}

	if cli.IsConnected() {
		cli.Disconnect()
	}

	if cli.Store.ID != nil {
		_ = cli.Logout(ctx)
	}

	delete(m.clients, phone)
	m.qrCodesMu.Lock()
	delete(m.qrCodes, phone)
	m.qrCodesMu.Unlock()

	return nil
}

func (m *WhatsMeowManager) SendMessage(ctx context.Context, phoneSender string, to string, message string) error {
	cli, err := m.EnsureClient(ctx, phoneSender)
	if err != nil {
		return fmt.Errorf("failed to initialize client for sender: %w", err)
	}

	if !cli.IsLoggedIn() {
		return errors.New("candidate whatsapp client is not authenticated")
	}

	for i := 0; i < 20; i++ {
		if cli.IsConnected() {
			break
		}
		time.Sleep(200 * time.Millisecond)
	}

	targetJID, err := types.ParseJID(to)
	if err != nil {
		if !strings.Contains(to, "@") {
			targetJID, err = types.ParseJID(to + "@s.whatsapp.net")
		}
		if err != nil {
			return fmt.Errorf("failed to parse recipient JID: %w", err)
		}
	}

	payload := &waE2E.Message{
		Conversation: proto.String(message),
	}

	_, err = cli.SendMessage(ctx, targetJID, payload)
	if err != nil {
		return fmt.Errorf("failed to send message via whatsmeow: %w", err)
	}

	return nil
}
