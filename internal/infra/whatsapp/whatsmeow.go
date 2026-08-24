package whatsapp

import (
	"context"
	"errors"
	"fmt"
	"log"
	"math/rand/v2"
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
	"google.golang.org/protobuf/proto"
	_ "modernc.org/sqlite"
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

type GroupMessageCallback func(phone string, msg domain.BufferedMessage)

func NewWhatsMeowManager(dbPath string) (*WhatsMeowManager, error) {
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

// EnsureClient devolve o client de phone, criando um device novo em branco se
// ele ainda não estiver pareado. Só serve ao fluxo de pareamento e à consulta de
// status — para qualquer outra coisa use ensureClient com createIfMissing false,
// que falha em vez de fabricar um device que nunca vai conseguir enviar nada.
func (m *WhatsMeowManager) EnsureClient(ctx context.Context, phone string) (*whatsmeow.Client, error) {
	return m.ensureClient(ctx, phone, true)
}

// ensureClient devolve o client de phone. createIfMissing decide o que fazer
// quando não há device pareado: o pareamento precisa de um device em branco;
// enviar, listar grupos ou observá-los precisa de um erro claro.
func (m *WhatsMeowManager) ensureClient(ctx context.Context, phone string, createIfMissing bool) (*whatsmeow.Client, error) {
	m.clientsMu.Lock()
	defer m.clientsMu.Unlock()

	if cli, exists := m.clients[phone]; exists {
		if cli.IsConnected() {
			return cli, nil
		}
		err := cli.Connect()
		if err != nil {
			if errors.Is(err, store.ErrDeviceDeleted) {
				delete(m.clients, phone)
			} else {
				return nil, fmt.Errorf("failed to connect existing whatsmeow client: %w", err)
			}
		} else {
			return cli, nil
		}
	}

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
		if !createIfMissing {
			return nil, fmt.Errorf("número %s não está pareado — escaneie o QR em GET /api/v1/whatsapp/qr?phone=%s", phone, phone)
		}
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

	if cli.IsConnected() {
		cli.Disconnect()
	}

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

// ListConnected lê todos os devices persistidos no store, sem conectar nenhum
// client que não esteja já rodando. Retorna um domain.WhatsAppStatus por
// número já pareado, ou erro se a leitura do store falhar.
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

// ListJoinedGroups consulta os grupos do WhatsApp de que a conta de phone é
// membro. Retorna a lista de domain.GroupInfo, ou erro se a conexão ou a
// consulta falhar.
func (m *WhatsMeowManager) ListJoinedGroups(ctx context.Context, phone string) ([]domain.GroupInfo, error) {
	cli, err := m.ensureClient(ctx, phone, false)
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

// WatchGroups substitui o handler de mensagens de grupo de phone por um novo,
// que encaminha para onMessage qualquer mensagem de texto vinda de um JID em
// watchedJIDs. Retorna erro apenas se obter/conectar o client falhar.
func (m *WhatsMeowManager) WatchGroups(ctx context.Context, phone string, watchedJIDs []string, onMessage GroupMessageCallback) error {
	cli, err := m.ensureClient(ctx, phone, false)
	if err != nil {
		return err
	}

	watchedSet := make(map[string]bool, len(watchedJIDs))
	for _, jid := range watchedJIDs {
		watchedSet[jid] = true
	}

	m.groupHandlersMu.Lock()
	defer m.groupHandlersMu.Unlock()

	// Síncrono de propósito: assíncrono deixaria uma janela com o handler antigo e
	// o novo ativos ao mesmo tempo. RemoveEventHandler pega o lock de escrita que
	// dispatchEvent segura para leitura, então só travaria se WatchGroups fosse
	// chamado de dentro de um handler — e não é (só vem do HTTP e do Bootstrap).
	if oldID, exists := m.groupHandlers[phone]; exists {
		cli.RemoveEventHandler(oldID)
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

// extractText extrai o texto de uma mensagem do whatsmeow, cobrindo conversas
// simples e respostas/links formatados. Retorna string vazia se msg for nil ou
// não tiver texto extraível.
func extractText(msg *waE2E.Message) string {
	if msg == nil {
		return ""
	}
	if c := msg.GetConversation(); c != "" {
		return c
	}
	return msg.GetExtendedTextMessage().GetText()
}

// buildRecipientJID parses to as a JID directly if it already contains "@"
// (types.ParseJID never errors on a bare string, it just misassigns it to the
// Server field, so that check can't be inferred from ParseJID's return error).
// Otherwise it treats to as a phone number: strips non-digit characters and,
// ponytail: assumes Brazil (prefixes "55") when the result looks like a local
// number without a country code (<=11 digits) — upgrade path: make the default
// country code configurable if targets outside Brazil are ever needed.
func buildRecipientJID(to string) (types.JID, error) {
	if strings.Contains(to, "@") {
		return types.ParseJID(to)
	}

	digits := strings.Map(func(r rune) rune {
		if r >= '0' && r <= '9' {
			return r
		}
		return -1
	}, to)

	if len(digits) <= 11 {
		digits = "55" + digits
	}

	return types.ParseJID(digits + "@s.whatsapp.net")
}

// resolveRecipient converte to no JID canônico com que o WhatsApp realmente
// endereça o destinatário, consultando o servidor.
//
// Não dá para mandar direto para "<numero>@s.whatsapp.net": desde a migração
// para LID, o whatsmeow precisa traduzir o número para um LID antes de enviar, e
// ele só consegue fazer isso sozinho se o número já estiver no cache local — o
// que nunca acontece aqui, porque os recrutadores são números lidos do texto de
// uma vaga, com quem o candidato nunca conversou. Sem esta consulta o envio
// morre com "no LID found for ... from server".
//
// De quebra resolve o nono dígito: celular brasileiro é divulgado como
// (61) 99596-8079, mas muitas contas estão registradas na forma antiga, de oito
// dígitos. O JID devolvido aqui é o que o WhatsApp usa de fato.
func resolveRecipient(ctx context.Context, cli *whatsmeow.Client, to string) (types.JID, error) {
	jid, err := buildRecipientJID(to)
	if err != nil {
		return types.EmptyJID, fmt.Errorf("failed to parse recipient JID: %w", err)
	}

	// Um JID já explícito (inclusive @lid) vem pronto do chamador.
	if strings.Contains(to, "@") {
		return jid, nil
	}

	results, err := cli.IsOnWhatsApp(ctx, []string{jid.User})
	if err != nil {
		return types.EmptyJID, fmt.Errorf("failed to check if %s is on whatsapp: %w", jid.User, err)
	}
	if len(results) == 0 {
		return types.EmptyJID, fmt.Errorf("número %s não retornou resultado na consulta ao WhatsApp", jid.User)
	}

	res := results[0]
	if !res.IsIn {
		return types.EmptyJID, fmt.Errorf("número %s não tem WhatsApp", jid.User)
	}
	if res.JID.IsEmpty() {
		return types.EmptyJID, fmt.Errorf("WhatsApp não devolveu JID canônico para %s", jid.User)
	}

	if res.JID.User != jid.User {
		log.Printf("[WhatsMeow] Número %s resolvido para %s pelo servidor", jid.User, res.JID)
	}
	return res.JID, nil
}

func (m *WhatsMeowManager) Disconnect(ctx context.Context, phone string) error {
	m.clientsMu.Lock()
	defer m.clientsMu.Unlock()

	cli, exists := m.clients[phone]
	if !exists {
		devices, err := m.container.GetAllDevices(ctx)
		if err != nil {
			return fmt.Errorf("failed to list devices from db: %w", err)
		}

		var devStore *store.Device
		for _, dev := range devices {
			if dev.ID != nil && dev.ID.User == phone {
				devStore = dev
				break
			}
		}
		if devStore == nil {
			return nil
		}

		cli = whatsmeow.NewClient(devStore, waLog.Stdout("Client", "DEBUG", true))
	}

	if cli.Store.ID != nil {
		if err := cli.Logout(ctx); err != nil {
			cli.Disconnect()
			if delErr := cli.Store.Delete(ctx); delErr != nil {
				return fmt.Errorf("failed to force-delete device after logout error (%v): %w", err, delErr)
			}
		}
	} else if cli.IsConnected() {
		cli.Disconnect()
	}

	delete(m.clients, phone)
	m.qrCodesMu.Lock()
	delete(m.qrCodes, phone)
	m.qrCodesMu.Unlock()

	return nil
}

func (m *WhatsMeowManager) SendMessage(ctx context.Context, phoneSender string, to string, message string) error {
	cli, err := m.ensureClient(ctx, phoneSender, false)
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

	targetJID, err := resolveRecipient(ctx, cli, to)
	if err != nil {
		return err
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

// sendTypingIndicator mostra "digitando" para o destinatário antes do envio, de
// modo que a candidatura não chegue com cara de disparo automático — mesma
// intenção do espaçamento entre envios no dispatcher.
//
// Falha aqui é puramente cosmética, então o erro só é logado: perder o
// "digitando" não pode impedir a candidatura de sair. Note que o WhatsApp só
// entrega o chatstate se a conta estiver marcada como online, o que a torna
// visível como online para todos os contatos.
func sendTypingIndicator(ctx context.Context, cli *whatsmeow.Client, to types.JID) {
	if err := cli.SendChatPresence(ctx, to, types.ChatPresenceComposing, types.ChatPresenceMediaText); err != nil {
		log.Printf("[WhatsMeow] não foi possível enviar presença 'digitando' para %s: %v", to, err)
		return
	}

	select {
	case <-time.After(typingDuration()):
	case <-ctx.Done():
		return
	}

	if err := cli.SendChatPresence(ctx, to, types.ChatPresencePaused, types.ChatPresenceMediaText); err != nil {
		log.Printf("[WhatsMeow] não foi possível encerrar a presença 'digitando' para %s: %v", to, err)
	}
}

// typingDuration devolve por quanto tempo o "digitando" fica visível: 3 a 7
// segundos, tempo plausível para alguém escrever uma mensagem curta.
func typingDuration() time.Duration {
	return 3*time.Second + rand.N(4001*time.Millisecond)
}

func (m *WhatsMeowManager) SendDocument(ctx context.Context, phoneSender string, to string, caption string, fileBytes []byte, filename string) error {
	cli, err := m.ensureClient(ctx, phoneSender, false)
	if err != nil {
		return fmt.Errorf("failed to initialise client for sender: %w", err)
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

	targetJID, err := resolveRecipient(ctx, cli, to)
	if err != nil {
		return err
	}

	uploaded, err := cli.Upload(ctx, fileBytes, whatsmeow.MediaDocument)
	if err != nil {
		return fmt.Errorf("failed to upload document: %w", err)
	}

	sendTypingIndicator(ctx, cli, targetJID)

	payload := &waE2E.Message{
		DocumentMessage: &waE2E.DocumentMessage{
			URL:           proto.String(uploaded.URL),
			DirectPath:    proto.String(uploaded.DirectPath),
			MediaKey:      uploaded.MediaKey,
			FileEncSHA256: uploaded.FileEncSHA256,
			FileSHA256:    uploaded.FileSHA256,
			FileLength:    proto.Uint64(uploaded.FileLength),
			// ponytail: o único chamador (orchestrator) sempre envia currículo em
			// PDF. Upgrade path: receber o mime como parâmetro se entrar outro
			// formato.
			Mimetype: proto.String("application/pdf"),
			FileName: proto.String(filename),
			Caption:  proto.String(caption),
		},
	}

	_, err = cli.SendMessage(ctx, targetJID, payload)
	if err != nil {
		return fmt.Errorf("failed to send document via whatsmeow: %w", err)
	}

	return nil
}
