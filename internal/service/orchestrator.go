package service

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"api/internal/domain"
	"api/internal/infra/email"
)

type orchestrator struct {
	geminiService    domain.GeminiService
	embeddingService domain.GeminiService
	vectorStore      domain.VectorStore
	credsRepo        domain.CredentialsRepository
	waManager        domain.WhatsAppManager
	newEmailService  func(creds *domain.EmailCredentials) domain.EmailService
	matchesMu        sync.RWMutex
	matches          map[string]*domain.MatchRecord
	dispatcher       *dispatcher
}

func NewOrchestrator(
	geminiService domain.GeminiService,
	embeddingService domain.GeminiService,
	vectorStore domain.VectorStore,
	credsRepo domain.CredentialsRepository,
	waManager domain.WhatsAppManager,
) domain.Orchestrator {
	o := &orchestrator{
		geminiService:    geminiService,
		embeddingService: embeddingService,
		vectorStore:      vectorStore,
		credsRepo:        credsRepo,
		waManager:        waManager,
		newEmailService: func(creds *domain.EmailCredentials) domain.EmailService {
			return email.NewOAuthEmailService(creds)
		},
		matches: make(map[string]*domain.MatchRecord),
	}
	o.dispatcher = newDispatcher(o.sendJob, nil)
	return o
}

// sendJob executa um job da fila. É o que o dispatcher chama depois de esperar o
// intervalo do canal.
func (o *orchestrator) sendJob(ctx context.Context, job dispatchJob) error {
	switch job.Channel {
	case channelEmail:
		if job.EmailService == nil {
			return fmt.Errorf("serviço de e-mail ausente no job")
		}
		return job.EmailService.SendEmail(ctx, job.Target, job.Subject, job.Body, job.FileB64, job.Filename)

	case channelWhatsApp:
		fileBytes, err := base64.StdEncoding.DecodeString(job.FileB64)
		if err != nil {
			return fmt.Errorf("falha ao decodificar currículo em base64: %w", err)
		}
		return o.waManager.SendDocument(ctx, job.SenderPhone, job.Target, job.Body, fileBytes, job.Filename)

	default:
		return fmt.Errorf("canal de envio desconhecido: %q", job.Channel)
	}
}

type dispatchResult struct {
	match  domain.Match
	status string
}

// Prefixos de tarefa do modelo de embedding. O nomic-embed-text é treinado com
// eles; sem os prefixos, a similaridade entre quaisquer dois textos do mesmo
// assunto fica artificialmente alta e o threshold deixa de separar qualquer
// coisa. Configuráveis porque outros modelos usam prefixos diferentes ou nenhum.
const (
	defaultDocPrefix   = "search_document: "
	defaultQueryPrefix = "search_query: "
)

// embeddingPrefix lê o prefixo de env, caindo no default quando a variável não
// está definida. Definida e vazia desliga o prefixo — daí a checagem por
// presença em vez de por valor vazio.
func embeddingPrefix(envVar, fallback string) string {
	if v, ok := os.LookupEnv(envVar); ok {
		return v
	}
	return fallback
}

// logMu serializa as escritas no arquivo de execução: os dois workers do
// dispatcher anexam o desfecho de cada envio, e um match novo pode estar
// escrevendo o cabeçalho ao mesmo tempo.
var logMu sync.Mutex

// appendToLog anexa text ao arquivo de execução.
func appendToLog(filename, text string) error {
	logMu.Lock()
	defer logMu.Unlock()

	file, err := os.OpenFile(filename, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	if err != nil {
		return err
	}
	defer file.Close()

	_, err = file.WriteString(text)
	return err
}

// appendDispatchOutcome registra no log de execução o desfecho de um envio. Sem
// isso o arquivo congelaria em "Enfileirado": o resultado real só existiria no
// stdout e no histórico em memória, que somem num restart.
func appendDispatchOutcome(filename, channel, target, vacancyID, status string) {
	if filename == "" {
		return
	}

	var b strings.Builder
	b.WriteString("--------------------------------------------------------------------------------\n")
	b.WriteString(fmt.Sprintf("ENVIO CONCLUÍDO: %s\n", time.Now().Format("2006-01-02 15:04:05")))
	b.WriteString(fmt.Sprintf("- ID da Vaga: %s\n", vacancyID))
	b.WriteString(fmt.Sprintf("- Canal: %s\n", channel))
	b.WriteString(fmt.Sprintf("- Destino: %s\n", target))
	b.WriteString(fmt.Sprintf("- Status Final: %s\n", status))

	if err := appendToLog(filename, b.String()); err != nil {
		log.Printf("[Dispatcher] Erro ao registrar desfecho no log %s: %v", filename, err)
	}
}

// summarizeVacancy devolve uma linha única identificando a vaga, para a
// listagem de pré-filtragem. Corta em 100 caracteres contando runas, não bytes,
// para não partir um acento ao meio.
func summarizeVacancy(text string) string {
	oneLine := strings.Join(strings.Fields(text), " ")
	runes := []rune(oneLine)
	if len(runes) > 100 {
		return string(runes[:100]) + "..."
	}
	return oneLine
}

func writeExecutionLog(filename string, startTime time.Time, processedCount int64, matchCount int, results []dispatchResult, vacancies []domain.Vacancy, skippedReason string) error {
	logMu.Lock()
	defer logMu.Unlock()

	file, err := os.OpenFile(filename, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	if err != nil {
		return err
	}
	defer file.Close()

	var builder strings.Builder
	timestamp := startTime.Format("2006-01-02 15:04:05")
	builder.WriteString(fmt.Sprintf("================================================================================\n"))
	builder.WriteString(fmt.Sprintf("EXECUÇÃO: %s\n", timestamp))
	builder.WriteString(fmt.Sprintf("--------------------------------------------------------------------------------\n"))

	if skippedReason != "" {
		builder.WriteString(fmt.Sprintf("Status: %s\n", skippedReason))
	} else {
		builder.WriteString(fmt.Sprintf("Vagas processadas no chat: %d\n", processedCount))
		builder.WriteString(fmt.Sprintf("Matches confirmados pela LLM: %d\n\n", matchCount))

		// Só chegam aqui as vagas que já passaram do threshold em SearchSimilarity,
		// então não existe caso "descartada" para relatar.
		builder.WriteString(fmt.Sprintf("================================================================================\n"))
		builder.WriteString(fmt.Sprintf("VAGAS APROVADAS NA PRÉ-FILTRAGEM SEMÂNTICA:\n"))
		builder.WriteString(fmt.Sprintf("--------------------------------------------------------------------------------\n"))
		// Só o resumo de cada vaga aqui: sem limite de vagas, despejar o texto
		// completo das centenas pré-filtradas fazia o log passar de 100KB por
		// execução. O texto integral segue na seção de matches, que é onde importa.
		for _, v := range vacancies {
			builder.WriteString(fmt.Sprintf("[%s] Score: %.4f | %s\n", v.ID, v.Score, summarizeVacancy(v.Text)))
		}

		builder.WriteString(fmt.Sprintf("================================================================================\n"))
		builder.WriteString(fmt.Sprintf("DISPAROS DE CANDIDATURA (Matches Confirmados):\n"))
		builder.WriteString(fmt.Sprintf("--------------------------------------------------------------------------------\n"))
		for i, r := range results {
			builder.WriteString(fmt.Sprintf("[MATCH %d]\n", i+1))
			builder.WriteString(fmt.Sprintf("- ID da Vaga: %s\n", r.match.VacancyID))
			builder.WriteString(fmt.Sprintf("- Tipo de Contato: %s\n", r.match.ContactType))
			builder.WriteString(fmt.Sprintf("- Destino de Envio: %s\n", r.match.ContactTarget))
			builder.WriteString(fmt.Sprintf("- Justificativa da LLM: %s\n", r.match.Reason))
			builder.WriteString(fmt.Sprintf("- Status de Envio: %s\n", r.status))

			var vacText string
			for _, v := range vacancies {
				if v.ID == r.match.VacancyID {
					vacText = v.Text
					break
				}
			}
			builder.WriteString(fmt.Sprintf("- Conteúdo da Vaga:\n%s\n", strings.TrimSpace(vacText)))
			builder.WriteString("\n")
		}
	}
	builder.WriteString(fmt.Sprintf("================================================================================\n"))

	_, err = file.WriteString(builder.String())
	return err
}

// ClearVacancies apaga todas as vagas da tabela local do banco vetorial. Retorna
// erro se a limpeza falhar.
func (o *orchestrator) ClearVacancies(ctx context.Context) error {
	return o.vectorStore.Clear(ctx)
}

func (o *orchestrator) PopulateVacancyTexts(ctx context.Context, texts []string) (int, error) {
	return o.populateItems(ctx, texts)
}

// ListVacancies retorna todas as vagas atualmente armazenadas. Retorna erro se a
// consulta ao banco falhar.
func (o *orchestrator) ListVacancies(ctx context.Context) ([]domain.Vacancy, error) {
	return o.vectorStore.ListVacancies(ctx)
}

func (o *orchestrator) DeleteVacancy(ctx context.Context, id string) error {
	return o.vectorStore.DeleteVacancy(ctx, id)
}

// recordMatch guarda result no histórico em memória, sob um ID novo. Histórico
// não é persistido em disco — some num restart do servidor (aceito
// deliberadamente, é só pra consulta durante a sessão).
func (o *orchestrator) recordMatch(result *domain.ProcessResult) {
	id := fmt.Sprintf("match_%d", time.Now().UnixNano())
	o.matchesMu.Lock()
	o.matches[id] = &domain.MatchRecord{ID: id, CreatedAt: time.Now(), Result: result}
	o.matchesMu.Unlock()
}

// snapshot copia um MatchRecord para leitura fora do lock. Necessário porque o
// dispatcher segue escrevendo Matches[i].Status depois que o registro foi
// devolvido, e o handler só serializa o JSON já sem o lock.
func snapshot(r *domain.MatchRecord) *domain.MatchRecord {
	copied := *r
	if r.Result != nil {
		result := *r.Result
		result.Matches = append([]domain.Match(nil), r.Result.Matches...)
		copied.Result = &result
	}
	return &copied
}

// ListMatches retorna o histórico de execuções de MatchResume desta sessão do
// servidor (em memória, perdido num restart).
func (o *orchestrator) ListMatches(ctx context.Context) ([]*domain.MatchRecord, error) {
	o.matchesMu.RLock()
	defer o.matchesMu.RUnlock()

	records := make([]*domain.MatchRecord, 0, len(o.matches))
	for _, r := range o.matches {
		records = append(records, snapshot(r))
	}
	return records, nil
}

// GetMatch busca um MatchRecord do histórico em memória pelo ID. Retorna erro
// se o ID não existir.
func (o *orchestrator) GetMatch(ctx context.Context, id string) (*domain.MatchRecord, error) {
	o.matchesMu.RLock()
	defer o.matchesMu.RUnlock()

	r, exists := o.matches[id]
	if !exists {
		return nil, fmt.Errorf("match %s not found", id)
	}
	return snapshot(r), nil
}

// DeleteMatch remove um MatchRecord do histórico em memória pelo ID. Retorna
// erro se o ID não existir.
func (o *orchestrator) DeleteMatch(ctx context.Context, id string) error {
	o.matchesMu.Lock()
	defer o.matchesMu.Unlock()

	if _, exists := o.matches[id]; !exists {
		return fmt.Errorf("match %s not found", id)
	}
	delete(o.matches, id)
	return nil
}

func (o *orchestrator) populateItems(ctx context.Context, items []string) (int, error) {
	if len(items) == 0 {
		return 0, nil
	}

	hashes := make([]string, 0, len(items))
	candidates := make([]string, 0, len(items))
	for _, item := range items {
		trimmed := strings.TrimSpace(item)
		if trimmed == "" {
			continue
		}
		hashes = append(hashes, fmt.Sprintf("vac_%x", sha256.Sum256([]byte(trimmed))))
		candidates = append(candidates, item)
	}

	existing, err := o.vectorStore.ExistingVacancyIDs(ctx, hashes)
	if err != nil {
		return 0, fmt.Errorf("failed to check existing vacancies: %w", err)
	}

	var newItems []string
	for i, item := range candidates {
		if !existing[hashes[i]] {
			newItems = append(newItems, item)
		}
	}

	if len(newItems) == 0 {
		log.Println("[Orchestrator] Todas as vagas fornecidas já existem no banco de dados local. Pulando geração de embeddings.")
		return 0, nil
	}

	batchSize := 1000
	if batchSizeStr := os.Getenv("OLLAMA_BATCH_SIZE"); batchSizeStr != "" {
		if val, err := strconv.Atoi(batchSizeStr); err == nil && val > 0 {
			batchSize = val
		}
	}

	log.Printf("[Orchestrator] Das %d vagas fornecidas, %d são novas. Gerando embeddings locais em lotes de %d...", len(items), len(newItems), batchSize)
	itemsSaved := 0

	for i := 0; i < len(newItems); i += batchSize {
		end := i + batchSize
		if end > len(newItems) {
			end = len(newItems)
		}

		batchItems := newItems[i:end]
		log.Printf("[Orchestrator] Gerando embeddings para o lote de vagas novas %d até %d...", i, end-1)

		// Só o texto mandado ao modelo leva o prefixo de tarefa; o que vai para o
		// banco e para a LLM continua sendo o texto original.
		prefixed := make([]string, len(batchItems))
		docPrefix := embeddingPrefix("EMBEDDING_DOC_PREFIX", defaultDocPrefix)
		for j, t := range batchItems {
			prefixed[j] = docPrefix + t
		}

		embs, err := o.embeddingService.GetEmbeddings(ctx, prefixed)
		if err != nil {
			return itemsSaved, fmt.Errorf("failed to generate embeddings for batch %d-%d: %w", i, end-1, err)
		}

		err = o.vectorStore.AddVacancies(ctx, batchItems, embs)
		if err != nil {
			return itemsSaved, fmt.Errorf("failed to add vacancies batch %d-%d to local database: %w", i, end-1, err)
		}

		itemsSaved += len(batchItems)
	}

	return itemsSaved, nil
}

// MatchResume extrai o texto do currículo, gera seu embedding, busca vagas
// similares no banco vetorial local e refina os matches via LLM, disparando a
// candidatura (e-mail ou WhatsApp) para cada match confirmado. Retorna o
// *domain.ProcessResult com os matches e um erro se qualquer etapa (extração,
// embedding, busca de similaridade ou matching via LLM) falhar.
func (o *orchestrator) MatchResume(ctx context.Context, fileB64, fileMime, candidateEmail, candidatePhone string) (*domain.ProcessResult, error) {
	start := time.Now()
	logFilename := fmt.Sprintf("log_%s.txt", start.Format("2006-01-02_15-04-05"))
	log.Printf("[Orchestrator] Iniciando matching do currículo. Log salvo em: %s", logFilename)

	if fileMime == "" {
		fileMime = "application/pdf"
	}

	log.Printf("[Orchestrator] Extraindo texto do currículo (MIME: %s)...", fileMime)
	resumeText, err := o.geminiService.ExtractText(ctx, fileB64, fileMime)
	if err != nil {
		log.Printf("[Orchestrator] Falha ao extrair texto do currículo: %v", err)
		_ = writeExecutionLog(logFilename, start, 0, 0, nil, nil, fmt.Sprintf("Erro ao extrair texto do currículo: %v", err))
		return nil, fmt.Errorf("failed to extract resume text: %w", err)
	}

	resumePreview := resumeText
	if len(resumePreview) > 200 {
		resumePreview = resumePreview[:200] + "..."
	}
	log.Printf("[Orchestrator] Texto do currículo extraído (%d caracteres). Preview:\n%s", len(resumeText), resumePreview)

	log.Println("[Orchestrator] Solicitando embedding para o currículo...")
	queryPrefix := embeddingPrefix("EMBEDDING_QUERY_PREFIX", defaultQueryPrefix)
	resumeEmbs, err := o.embeddingService.GetEmbeddings(ctx, []string{queryPrefix + resumeText})
	if err != nil || len(resumeEmbs) == 0 {
		log.Printf("[Orchestrator] Falha ao gerar embedding do currículo: %v", err)
		_ = writeExecutionLog(logFilename, start, 0, 0, nil, nil, fmt.Sprintf("Erro ao obter embedding do currículo: %v", err))
		return nil, fmt.Errorf("failed to get resume embedding: %w", err)
	}
	resumeEmbedding := resumeEmbs[0]
	log.Printf("[Orchestrator] Embedding do currículo gerado com sucesso (dimensões: %d).", len(resumeEmbedding))

	thresholdStr := os.Getenv("EMBEDDING_THRESHOLD")
	threshold := float32(0.35)
	if thresholdStr != "" {
		if val, err := strconv.ParseFloat(thresholdStr, 32); err == nil {
			threshold = float32(val)
		}
	}

	// 0 = sem limite. Quem filtra aptidão é a LLM adiante; cortar aqui em N
	// descartaria vagas com score praticamente idêntico ao das que passaram.
	vacancyLimit := 0
	if s := os.Getenv("MATCH_VACANCY_LIMIT"); s != "" {
		if v, err := strconv.Atoi(s); err == nil && v >= 0 {
			vacancyLimit = v
		} else {
			log.Printf("[Orchestrator] MATCH_VACANCY_LIMIT inválido (%q), usando 0 (sem limite)", s)
		}
	}

	matchedVacancies, err := o.vectorStore.SearchSimilarity(ctx, resumeEmbedding, vacancyLimit, threshold)
	if err != nil {
		log.Printf("[Orchestrator] Erro na busca vetorial local: %v", err)
		_ = writeExecutionLog(logFilename, start, 0, 0, nil, nil, fmt.Sprintf("Erro na busca de similaridade: %v", err))
		return nil, fmt.Errorf("failed to search similar vacancies: %w", err)
	}

	if len(matchedVacancies) == 0 {
		reason := fmt.Sprintf("Nenhuma vaga compatível com o limite de similaridade semântica (threshold: %.2f).", threshold)
		log.Printf("[Orchestrator] Fim de fluxo precoce: %s", reason)
		_ = writeExecutionLog(logFilename, start, 0, 0, nil, nil, reason)
		result := &domain.ProcessResult{
			Status:     "success",
			Matches:    []domain.Match{},
			DurationMs: time.Since(start).Milliseconds(),
		}
		o.recordMatch(result)
		return result, nil
	}

	filteredTexts := make([]string, len(matchedVacancies))
	for i, v := range matchedVacancies {
		filteredTexts[i] = v.Text
	}

	log.Printf("[Orchestrator] Chamando validador da LLM para as %d vagas pré-filtradas...", len(matchedVacancies))
	matchRes, err := o.geminiService.MatchResume(ctx, fileB64, fileMime, filteredTexts)
	if err != nil {
		log.Printf("[Orchestrator] LLM validation matching falhou: %v", err)
		_ = writeExecutionLog(logFilename, start, 0, 0, nil, matchedVacancies, fmt.Sprintf("Erro no refino de matching LLM: %v", err))
		return nil, fmt.Errorf("LLM matching failed: %w", err)
	}

	var validMatches []domain.Match
	for i := range matchRes.Matches {
		idx := matchRes.Matches[i].Index
		if idx < 0 || idx >= len(matchedVacancies) {
			log.Printf("[Orchestrator] Aviso: LLM retornou índice fora do escopo (%d)", idx)
			continue
		}
		matchRes.Matches[i].VacancyID = matchedVacancies[idx].ID
		log.Printf("[Orchestrator] LLM index %d -> vaga %s", idx, matchedVacancies[idx].ID)
		validMatches = append(validMatches, matchRes.Matches[i])
	}
	matchRes.Matches = validMatches
	log.Printf("[Orchestrator] LLM validou %d matches finais.", len(matchRes.Matches))

	// Credenciais e cliente de e-mail são os mesmos para todos os matches: buscar
	// dentro do loop faria uma leitura no banco e um refresh de token por disparo.
	var emailService domain.EmailService
	emailUnavailable := "Não disparado (campo 'candidate_email' ausente na requisição)"
	if candidateEmail != "" {
		log.Printf("[Orchestrator] Obtendo credenciais OAuth2 para o candidato %s...", candidateEmail)
		creds, err := o.credsRepo.GetEmailCredentials(ctx, candidateEmail)
		if err != nil {
			log.Printf("[Orchestrator] Erro ao carregar credenciais para %s: %v", candidateEmail, err)
			emailUnavailable = fmt.Sprintf("Não disparado (sem credenciais OAuth2 para %s — cadastre em POST /api/v1/credentials): %v", candidateEmail, err)
		} else {
			emailService = o.newEmailService(creds)
		}
	}

	// Sem canal de e-mail, todo match por e-mail morre em silêncio. Como a maioria
	// das vagas pede currículo por e-mail, isso costuma ser a maior parte delas.
	if emailService == nil {
		emailMatches := 0
		for _, m := range matchRes.Matches {
			if m.ContactType == channelEmail {
				emailMatches++
			}
		}
		if emailMatches > 0 {
			log.Printf("[Orchestrator] ATENÇÃO: %d de %d matches são por e-mail e não serão enviados. Motivo: %s",
				emailMatches, len(matchRes.Matches), emailUnavailable)
		}
	}

	// O resultado é registrado ANTES de enfileirar: os callbacks do dispatcher
	// escrevem status nele conforme os envios acontecem, e é esse mesmo ponteiro
	// que GET /api/v1/matches/{id} devolve.
	result := &domain.ProcessResult{
		Status:     "success",
		Matches:    matchRes.Matches,
		DurationMs: time.Since(start).Milliseconds(),
	}
	o.recordMatch(result)

	var dispatchResults []dispatchResult
	for i := range result.Matches {
		match := result.Matches[i]
		log.Printf("[Orchestrator] Enfileirando disparo %d/%d (vaga: %s, contato: %s, destino: %s)", i+1, len(result.Matches), match.VacancyID, match.ContactType, match.ContactTarget)

		job, status := o.buildDispatchJob(match, emailService, emailUnavailable, candidatePhone, fileB64)
		job.LogFile = logFilename
		queue := status == ""
		if queue {
			status = "Enfileirado"
		}

		// O status inicial precisa ser gravado ANTES do enqueue: o dispatcher pode
		// concluir o envio e escrever "Sucesso" antes desta linha rodar, e gravar
		// depois sobrescreveria o resultado real com "Enfileirado" para sempre.
		o.setMatchStatus(result, i, status)

		if queue {
			job.onResult = o.matchStatusSetter(result, i)
			if err := o.dispatcher.enqueue(job); err != nil {
				status = fmt.Sprintf("Erro ao enfileirar: %v", err)
				o.setMatchStatus(result, i, status)
			}
		}

		match.Status = status
		dispatchResults = append(dispatchResults, dispatchResult{match: match, status: status})
	}

	err = writeExecutionLog(logFilename, start, int64(len(matchedVacancies)), len(result.Matches), dispatchResults, matchedVacancies, "")
	if err != nil {
		log.Printf("[Orchestrator] Erro ao gravar arquivo de log %s: %v", logFilename, err)
	}

	// Devolve uma cópia: o result registrado segue sendo escrito pelo dispatcher, e
	// o handler serializa o JSON já fora do lock.
	o.matchesMu.RLock()
	defer o.matchesMu.RUnlock()
	returned := *result
	returned.Matches = append([]domain.Match(nil), result.Matches...)
	return &returned, nil
}

// messageTZ é o fuso que decide a saudação da mensagem. O default é Brasil
// porque o resto do fluxo já assume isso (o prompt exige DDI 55); o TZ do
// sistema não serve de default porque em container ele costuma ser UTC, e a
// saudação sairia três horas adiantada.
var messageTZ = loadMessageTZ()

func loadMessageTZ() *time.Location {
	name := os.Getenv("MESSAGE_TZ")
	if name == "" {
		name = "America/Sao_Paulo"
	}
	loc, err := time.LoadLocation(name)
	if err != nil {
		log.Printf("[Orchestrator] MESSAGE_TZ %q inválido (%v), usando o fuso do sistema", name, err)
		return time.Local
	}
	return loc
}

// greeting devolve a saudação da hora do dia.
func greeting(t time.Time) string {
	switch h := t.In(messageTZ).Hour(); {
	case h < 5, h >= 18:
		return "Boa noite"
	case h < 12:
		return "Bom dia"
	default:
		return "Boa tarde"
	}
}

// sanitizeRole normaliza o cargo vindo da LLM antes de ele virar assunto de
// e-mail e corpo de mensagem: uma linha só e curto o bastante para caber num
// assunto.
func sanitizeRole(role string) string {
	role = strings.Join(strings.Fields(role), " ")
	if r := []rune(role); len(r) > 80 {
		role = strings.TrimSpace(string(r[:80]))
	}
	return role
}

// applicationMessage é a mensagem que o recrutador recebe, a mesma nos dois
// canais. O motivo do match não entra: é raciocínio interno da LLM, escrito para
// o log, não para quem vai ler a candidatura.
func applicationMessage(role string, now time.Time) string {
	vaga := "à vaga divulgada"
	if role != "" {
		vaga = "à vaga de " + role
	}
	return fmt.Sprintf("%s,\n\nCandidato-me %s. Currículo em anexo para avaliação.", greeting(now), vaga)
}

func applicationSubject(role string) string {
	if role == "" {
		return "Candidatura"
	}
	return "Candidatura - " + role
}

// buildDispatchJob monta o job de envio de match. Devolve status não-vazio
// quando o disparo nem chega a ser enfileirado — falta de credencial, de
// telefone ou canal desconhecido.
func (o *orchestrator) buildDispatchJob(match domain.Match, emailService domain.EmailService, emailUnavailable, candidatePhone, fileB64 string) (dispatchJob, string) {
	role := sanitizeRole(match.Role)

	switch match.ContactType {
	case channelEmail:
		if emailService == nil {
			return dispatchJob{}, emailUnavailable
		}
		return dispatchJob{
			Channel:      channelEmail,
			Target:       match.ContactTarget,
			VacancyID:    match.VacancyID,
			EmailService: emailService,
			Subject:      applicationSubject(role),
			Body:         applicationMessage(role, time.Now()),
			FileB64:      fileB64,
			Filename:     "curriculo.pdf",
		}, ""

	case channelWhatsApp:
		if candidatePhone == "" {
			return dispatchJob{}, "Não disparado (telefone do candidato não fornecido)"
		}
		return dispatchJob{
			Channel:     channelWhatsApp,
			Target:      match.ContactTarget,
			VacancyID:   match.VacancyID,
			SenderPhone: candidatePhone,
			Body:        applicationMessage(role, time.Now()),
			FileB64:     fileB64,
			Filename:    "curriculo.pdf",
		}, ""

	default:
		return dispatchJob{}, fmt.Sprintf("Não disparado (tipo de contato desconhecido: '%s')", match.ContactType)
	}
}

// setMatchStatus grava o status do match de índice i sob o mesmo lock que serve
// GET /api/v1/matches/{id}. O ponteiro result já está no histórico, então
// goroutines do dispatcher e leitores HTTP disputam esses campos.
func (o *orchestrator) setMatchStatus(result *domain.ProcessResult, i int, status string) {
	o.matchesMu.Lock()
	result.Matches[i].Status = status
	o.matchesMu.Unlock()
}

// matchStatusSetter devolve o callback que o dispatcher usa para reportar o
// desfecho do envio do match de índice i.
func (o *orchestrator) matchStatusSetter(result *domain.ProcessResult, i int) func(string) {
	return func(status string) {
		o.setMatchStatus(result, i, status)
	}
}
