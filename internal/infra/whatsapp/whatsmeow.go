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
	waLog "go.mau.fi/whatsmeow/util/log"
	_ "modernc.org/sqlite"
	"google.golang.org/protobuf/proto"
)

type WhatsMeowManager struct {
	container *sqlstore.Container
	clients   map[string]*whatsmeow.Client
	clientsMu sync.Mutex
	qrCodes   map[string]string
	qrCodesMu sync.RWMutex
}

func NewWhatsMeowManager(dbPath string) (*WhatsMeowManager, error) {
	// sqlstore.New takes context, dialect, dsn, and logger in this version
	container, err := sqlstore.New(context.Background(), "sqlite", "file:"+dbPath+"?_foreign_keys=on", waLog.Noop)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize whatsmeow sqlstore: %w", err)
	}

	return &WhatsMeowManager{
		container: container,
		clients:   make(map[string]*whatsmeow.Client),
		qrCodes:   make(map[string]string),
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

	cli := whatsmeow.NewClient(devStore, waLog.Noop)
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

	qrChan, err := cli.GetQRChannel(ctx)
	if err != nil {
		m.qrCodesMu.RLock()
		qrStr, exists := m.qrCodes[phone]
		m.qrCodesMu.RUnlock()
		if exists && qrStr != "" {
			return qrcode.Encode(qrStr, qrcode.Medium, 256)
		}
		return nil, fmt.Errorf("failed to get qr channel: %w", err)
	}

	if !cli.IsConnected() {
		go func() {
			_ = cli.Connect()
		}()
	}

	select {
	case item := <-qrChan:
		if item.Event == "code" {
			m.qrCodesMu.Lock()
			m.qrCodes[phone] = item.Code
			m.qrCodesMu.Unlock()

			return qrcode.Encode(item.Code, qrcode.Medium, 256)
		}
		return nil, fmt.Errorf("received unexpected QR channel event: %s", item.Event)
	case <-time.After(10 * time.Second):
		m.qrCodesMu.RLock()
		qrStr, exists := m.qrCodes[phone]
		m.qrCodesMu.RUnlock()
		if exists && qrStr != "" {
			return qrcode.Encode(qrStr, qrcode.Medium, 256)
		}
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
