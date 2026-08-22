package service

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"log"
	"math"
	"os"
	"strconv"
	"strings"
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
}

func NewOrchestrator(
	geminiService domain.GeminiService,
	embeddingService domain.GeminiService,
	vectorStore domain.VectorStore,
	credsRepo domain.CredentialsRepository,
	waManager domain.WhatsAppManager,
) domain.Orchestrator {
	return &orchestrator{
		geminiService:    geminiService,
		embeddingService: embeddingService,
		vectorStore:      vectorStore,
		credsRepo:        credsRepo,
		waManager:        waManager,
		newEmailService: func(creds *domain.EmailCredentials) domain.EmailService {
			return email.NewOAuthEmailService(creds)
		},
	}
}

type dispatchResult struct {
	match  domain.Match
	status string
}

type vacancyScore struct {
	Index  int
	Score  float32
	Passed bool
	Text   string
}

func writeExecutionLog(filename string, startTime time.Time, processedCount int64, matchCount int, results []dispatchResult, scores []vacancyScore, skippedReason string) error {
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

		builder.WriteString(fmt.Sprintf("================================================================================\n"))
		builder.WriteString(fmt.Sprintf("PRÉ-FILTRAGEM DE SIMILARIDADE SEMÂNTICA (EM MEMÓRIA):\n"))
		builder.WriteString(fmt.Sprintf("--------------------------------------------------------------------------------\n"))
		for _, s := range scores {
			statusStr := "[DESCARTADA]"
			if s.Passed {
				statusStr = "[PASSOU]"
			}
			builder.WriteString(fmt.Sprintf("[Vaga %d] Score Cosseno: %.4f -> %s\n", s.Index, s.Score, statusStr))
			builder.WriteString(fmt.Sprintf("- Conteúdo: %s\n", strings.ReplaceAll(strings.TrimSpace(s.Text), "\n", " / ")))
			builder.WriteString("\n")
		}

		builder.WriteString(fmt.Sprintf("================================================================================\n"))
		builder.WriteString(fmt.Sprintf("DISPAROS DE CANDIDATURA (Matches Confirmados):\n"))
		builder.WriteString(fmt.Sprintf("--------------------------------------------------------------------------------\n"))
		for i, r := range results {
			builder.WriteString(fmt.Sprintf("[MATCH %d]\n", i+1))
			builder.WriteString(fmt.Sprintf("- Índice Original da Vaga: %d\n", r.match.Index))
			builder.WriteString(fmt.Sprintf("- Tipo de Contato: %s\n", r.match.ContactType))
			builder.WriteString(fmt.Sprintf("- Destino de Envio: %s\n", r.match.ContactTarget))
			builder.WriteString(fmt.Sprintf("- Justificativa da LLM: %s\n", r.match.Reason))
			builder.WriteString(fmt.Sprintf("- Status de Envio: %s\n", r.status))

			var vacText string
			for _, s := range scores {
				if s.Index == r.match.Index {
					vacText = s.Text
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

func cosineSimilarity(a, b []float32) float32 {
	if len(a) != len(b) || len(a) == 0 {
		return 0
	}
	var dotProduct, normA, normB float64
	for i := range a {
		dotProduct += float64(a[i] * b[i])
		normA += float64(a[i] * a[i])
		normB += float64(b[i] * b[i])
	}
	if normA == 0 || normB == 0 {
		return 0
	}
	return float32(dotProduct / (math.Sqrt(normA) * math.Sqrt(normB)))
}

// ClearVacancies apaga todas as vagas da tabela local do banco vetorial. Retorna
// erro se a limpeza falhar.
func (o *orchestrator) ClearVacancies(ctx context.Context) error {
	return o.vectorStore.Clear(ctx)
}

func (o *orchestrator) PopulateVacancyTexts(ctx context.Context, texts []string) (int, error) {
	return o.populateItems(ctx, texts)
}

// ListVacancies retorna todas as vagas atualmente armazenadas no banco vetorial
// local. Retorna erro se a geração do embedding de referência ou a consulta ao
// banco vetorial falhar.
func (o *orchestrator) ListVacancies(ctx context.Context) ([]domain.Vacancy, error) {
	embs, err := o.embeddingService.GetEmbeddings(ctx, []string{"."})
	if err != nil || len(embs) == 0 {
		return nil, fmt.Errorf("failed to get placeholder embedding: %w", err)
	}
	return o.vectorStore.ListVacancies(ctx, embs[0])
}

func (o *orchestrator) populateItems(ctx context.Context, items []string) (int, error) {
	if len(items) == 0 {
		return 0, nil
	}

	var newItems []string
	for _, item := range items {
		trimmed := strings.TrimSpace(item)
		if trimmed == "" {
			continue
		}
		hash := sha256.Sum256([]byte(trimmed))
		hashStr := fmt.Sprintf("vac_%x", hash)

		exists, err := o.vectorStore.HasVacancy(ctx, hashStr)
		if err != nil {
			log.Printf("[Orchestrator] Erro ao verificar existência da vaga: %v", err)
			newItems = append(newItems, item)
			continue
		}

		if !exists {
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

		embs, err := o.embeddingService.GetEmbeddings(ctx, batchItems)
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
	resumeEmbs, err := o.embeddingService.GetEmbeddings(ctx, []string{resumeText})
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

	matchedVacancies, err := o.vectorStore.SearchSimilarity(ctx, resumeEmbedding, 10, threshold)
	if err != nil {
		log.Printf("[Orchestrator] Erro na busca vetorial local: %v", err)
		_ = writeExecutionLog(logFilename, start, 0, 0, nil, nil, fmt.Sprintf("Erro na busca de similaridade: %v", err))
		return nil, fmt.Errorf("failed to search similar vacancies: %w", err)
	}

	if len(matchedVacancies) == 0 {
		reason := fmt.Sprintf("Nenhuma vaga compatível com o limite de similaridade semântica (threshold: %.2f).", threshold)
		log.Printf("[Orchestrator] Fim de fluxo precoce: %s", reason)
		_ = writeExecutionLog(logFilename, start, 0, 0, nil, nil, reason)
		return &domain.ProcessResult{
			Status:     "success",
			Matches:    []domain.Match{},
			DurationMs: time.Since(start).Milliseconds(),
		}, nil
	}

	filteredTexts := make([]string, len(matchedVacancies))
	for i, v := range matchedVacancies {
		filteredTexts[i] = v.Text
	}

	log.Printf("[Orchestrator] Chamando validador da LLM para as %d vagas pré-filtradas...", len(matchedVacancies))
	matchRes, err := o.geminiService.MatchResume(ctx, fileB64, fileMime, filteredTexts)
	if err != nil {
		log.Printf("[Orchestrator] LLM validation matching falhou: %v", err)
		var dummyScores []vacancyScore
		for _, v := range matchedVacancies {
			dummyScores = append(dummyScores, vacancyScore{Index: v.Index, Text: v.Text, Passed: true})
		}
		_ = writeExecutionLog(logFilename, start, 0, 0, nil, dummyScores, fmt.Sprintf("Erro no refino de matching LLM: %v", err))
		return nil, fmt.Errorf("LLM matching failed: %w", err)
	}

	var validMatches []domain.Match
	for i := range matchRes.Matches {
		idx := matchRes.Matches[i].Index
		if idx >= 0 && idx < len(matchedVacancies) {
			origIdx := matchedVacancies[idx].Index
			log.Printf("[Orchestrator] Mapeamento de índice: LLM index %d -> original index %d", idx, origIdx)
			matchRes.Matches[i].Index = origIdx
			validMatches = append(validMatches, matchRes.Matches[i])
		} else {
			log.Printf("[Orchestrator] Aviso: LLM retornou índice fora do escopo (%d)", idx)
		}
	}
	matchRes.Matches = validMatches
	log.Printf("[Orchestrator] LLM validou %d matches finais.", len(matchRes.Matches))

	var dispatchResults []dispatchResult
	var scores []vacancyScore
	for _, v := range matchedVacancies {
		scores = append(scores, vacancyScore{
			Index:  v.Index,
			Score:  1.0,
			Passed: true,
			Text:   v.Text,
		})
	}

	for i, match := range matchRes.Matches {
		log.Printf("[Orchestrator] Processando disparo %d/%d (original index: %d, contato: %s, destino: %s)", i+1, len(matchRes.Matches), match.Index, match.ContactType, match.ContactTarget)
		status := "Não disparado (sem contato válido)"
		switch match.ContactType {
		case "email":
			if candidateEmail == "" {
				log.Println("[Orchestrator] E-mail do candidato não fornecido na requisição. Pulando disparo.")
				status = "Não disparado (e-mail do candidato não fornecido)"
				break
			}

			log.Printf("[Orchestrator] Obtendo credenciais OAuth2 para o candidato %s...", candidateEmail)
			creds, err := o.credsRepo.GetEmailCredentials(ctx, candidateEmail)
			if err != nil {
				log.Printf("[Orchestrator] Erro ao carregar credenciais para %s: %v", candidateEmail, err)
				status = fmt.Sprintf("Erro ao carregar credenciais de e-mail: %v", err)
				break
			}

			log.Printf("[Orchestrator] Enviando E-mail -> para: %s...", match.ContactTarget)
			subject := "Candidatura - Processamento Automático"
			body := fmt.Sprintf("Olá,\n\nEstou me candidatando à vaga de emprego número %d.\n\nMotivo da compatibilidade:\n%s\n\nEm anexo, envio meu currículo para avaliação.\n\nAtenciosamente,\nCandidato", match.Index+1, match.Reason)

			emailService := o.newEmailService(creds)
			err = emailService.SendEmail(ctx, match.ContactTarget, subject, body, fileB64, "curriculo.pdf")
			if err != nil {
				log.Printf("[Orchestrator] Falha no e-mail para %s: %v", match.ContactTarget, err)
				status = fmt.Sprintf("Erro no envio do e-mail: %v", err)
			} else {
				log.Printf("[Orchestrator] E-mail enviado com sucesso para %s", match.ContactTarget)
				status = "Sucesso (e-mail enviado)"
			}

		case "whatsapp":
			if candidatePhone == "" {
				log.Println("[Orchestrator] Telefone do candidato não fornecido na requisição. Pulando disparo.")
				status = "Não disparado (telefone do candidato não fornecido)"
				break
			}

			fileBytes, err := base64.StdEncoding.DecodeString(fileB64)
			if err != nil {
				log.Printf("[Orchestrator] Falha ao decodificar currículo em base64: %v", err)
				status = fmt.Sprintf("Erro ao decodificar currículo: %v", err)
				break
			}

			log.Printf("[Orchestrator] Enviando WhatsApp via WhatsMeow -> para: %s...", match.ContactTarget)
			caption := fmt.Sprintf("Olá!\n\nEstou me candidatando à sua vaga de emprego.\n\n*Motivo do Match:*\n%s\n\nEm anexo, envio meu currículo para avaliação.", match.Reason)

			err = o.waManager.SendDocument(ctx, candidatePhone, match.ContactTarget, caption, fileBytes, "curriculo.pdf")
			if err != nil {
				log.Printf("[Orchestrator] Falha no WhatsApp para %s: %v", match.ContactTarget, err)
				status = fmt.Sprintf("Erro no envio do WhatsApp: %v", err)
			} else {
				log.Printf("[Orchestrator] WhatsApp enviado com sucesso para %s", match.ContactTarget)
				status = "Sucesso (WhatsApp enviado)"
			}

		default:
			log.Printf("[Orchestrator] Tipo de contato desconhecido '%s'. Pulando.", match.ContactType)
			status = fmt.Sprintf("Não disparado (tipo de contato desconhecido: '%s')", match.ContactType)
		}

		dispatchResults = append(dispatchResults, dispatchResult{
			match:  match,
			status: status,
		})
	}

	err = writeExecutionLog(logFilename, start, int64(len(scores)), len(matchRes.Matches), dispatchResults, scores, "")
	if err != nil {
		log.Printf("[Orchestrator] Erro ao gravar arquivo de log %s: %v", logFilename, err)
	}

	return &domain.ProcessResult{
		Status:     "success",
		Matches:    matchRes.Matches,
		DurationMs: time.Since(start).Milliseconds(),
	}, nil
}

