package service

import (
	"context"
	"fmt"
	"log"
	"math"
	"os"
	"strconv"
	"strings"
	"time"

	"api/internal/domain"
)

type orchestrator struct {
	processor        domain.PayloadProcessor
	geminiService    domain.GeminiService // matching service (Gemini or DeepSeek)
	embeddingService domain.GeminiService // embedding service (always Gemini)
	emailService     domain.EmailService
	whatsAppService  domain.WhatsAppService
}

// NewOrchestrator creates a new application use case orchestrator.
func NewOrchestrator(
	processor domain.PayloadProcessor,
	geminiService domain.GeminiService,
	embeddingService domain.GeminiService,
	emailService domain.EmailService,
	whatsAppService domain.WhatsAppService,
) domain.Orchestrator {
	return &orchestrator{
		processor:        processor,
		geminiService:    geminiService,
		embeddingService: embeddingService,
		emailService:     emailService,
		whatsAppService:  whatsAppService,
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
			
			// Find vacancy text from scores slice
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

func (o *orchestrator) RunMatchAndDispatch(ctx context.Context, req *domain.ProcessRequest) (*domain.ProcessResult, error) {
	start := time.Now()
	logFilename := fmt.Sprintf("log_%s.txt", start.Format("2006-01-02_15-04-05"))
	log.Printf("[Orchestrator] Starting match and dispatch process. Log file will be: %s", logFilename)

	// 1. Process the payload (Split vacancies and check resume metadata)
	res, err := o.processor.Process(ctx, req)
	if err != nil {
		log.Printf("[Orchestrator] Payload processing failed: %v", err)
		return nil, err
	}

	// 2. We can only perform matching if both the resume and the vacancies list are provided
	if req.FileBase64 == "" || len(res.Items) == 0 {
		res.DurationMs = time.Since(start).Milliseconds()

		reason := "Matching ignorado: currículo ou lista de vagas não fornecido."
		if req.FileBase64 == "" && len(res.Items) > 0 {
			reason = fmt.Sprintf("Matching ignorado: currículo não fornecido (vagas processadas: %d).", len(res.Items))
		} else if req.FileBase64 != "" && len(res.Items) == 0 {
			reason = "Matching ignorado: lista de vagas não fornecida (currículo carregado)."
		}

		log.Printf("[Orchestrator] Skipping matching. Reason: %s", reason)
		_ = writeExecutionLog(logFilename, start, res.ItemsProcessed, 0, nil, nil, reason)
		return res, nil
	}

	// Get file mime type
	mimeType := "application/pdf"
	if res.File != nil && res.File.MimeType != "" {
		mimeType = res.File.MimeType
	}

	// 3. Extract text from resume PDF/Document
	log.Printf("[Orchestrator] Extracting text from candidate's resume (MIME: %s)...", mimeType)
	resumeText, err := o.geminiService.ExtractText(ctx, req.FileBase64, mimeType)
	if err != nil {
		log.Printf("[Orchestrator] Resume text extraction failed: %v", err)
		_ = writeExecutionLog(logFilename, start, res.ItemsProcessed, 0, nil, nil, fmt.Sprintf("Erro ao extrair texto do currículo: %v", err))
		return nil, fmt.Errorf("failed to extract resume text: %w", err)
	}
	
	resumePreview := resumeText
	if len(resumePreview) > 200 {
		resumePreview = resumePreview[:200] + "..."
	}
	log.Printf("[Orchestrator] Resume text extracted successfully (%d characters). Preview:\n%s", len(resumeText), resumePreview)

	// 4. Generate embeddings for candidate resume
	log.Println("[Orchestrator] Requesting embedding for candidate's resume...")
	resumeEmbs, err := o.embeddingService.GetEmbeddings(ctx, []string{resumeText})
	if err != nil || len(resumeEmbs) == 0 {
		log.Printf("[Orchestrator] Failed to get resume embedding: %v", err)
		_ = writeExecutionLog(logFilename, start, res.ItemsProcessed, 0, nil, nil, fmt.Sprintf("Erro ao obter embedding do currículo: %v", err))
		return nil, fmt.Errorf("failed to get resume embedding: %w", err)
	}
	resumeEmbedding := resumeEmbs[0]
	log.Printf("[Orchestrator] Resume embedding computed successfully (dimensions: %d).", len(resumeEmbedding))

	// 5. Generate embeddings for vacancies in batches of 100
	log.Printf("[Orchestrator] Requesting embeddings for %d vacancies in batches...", len(res.Items))
	var vacancyEmbeddings [][]float32
	batchSize := 100
	for i := 0; i < len(res.Items); i += batchSize {
		end := i + batchSize
		if end > len(res.Items) {
			end = len(res.Items)
		}
		log.Printf("[Orchestrator] Fetching embeddings for vacancies index %d to %d...", i, end-1)
		embs, err := o.embeddingService.GetEmbeddings(ctx, res.Items[i:end])
		if err != nil {
			log.Printf("[Orchestrator] Failed to get embeddings for vacancies batch %d-%d: %v", i, end-1, err)
			_ = writeExecutionLog(logFilename, start, res.ItemsProcessed, 0, nil, nil, fmt.Sprintf("Erro ao obter embeddings das vagas: %v", err))
			return nil, fmt.Errorf("failed to get vacancy embeddings: %w", err)
		}
		vacancyEmbeddings = append(vacancyEmbeddings, embs...)
	}
	log.Printf("[Orchestrator] Successfully collected embeddings for all %d vacancies.", len(vacancyEmbeddings))

	// 6. Pre-filter vacancies using Cosine Similarity threshold in memory
	thresholdStr := os.Getenv("EMBEDDING_THRESHOLD")
	threshold := float32(0.35)
	if thresholdStr != "" {
		if val, err := strconv.ParseFloat(thresholdStr, 32); err == nil {
			threshold = float32(val)
		}
	}

	log.Printf("[Orchestrator] Starting Cosine Similarity pre-filtering (threshold >= %.2f)...", threshold)
	type filteredVacancy struct {
		originalIndex int
		text          string
		score         float32
	}
	var filtered []filteredVacancy
	var scores []vacancyScore

	for idx, vacEmb := range vacancyEmbeddings {
		score := cosineSimilarity(resumeEmbedding, vacEmb)
		passed := score >= threshold
		scores = append(scores, vacancyScore{
			Index:  idx,
			Score:  score,
			Passed: passed,
			Text:   res.Items[idx],
		})
		
		log.Printf("[Orchestrator] Similarity check - Vaga %d: Cosseno=%.4f (Passou=%t)", idx, score, passed)
		
		if passed {
			filtered = append(filtered, filteredVacancy{
				originalIndex: idx,
				text:          res.Items[idx],
				score:         score,
			})
		}
	}

	log.Printf("[Orchestrator] Similarity pre-filtering completed: keeping %d out of %d vacancies.", len(filtered), len(res.Items))

	// 7. If no vacancies met the threshold, write empty match report and return early
	if len(filtered) == 0 {
		reason := fmt.Sprintf("Nenhuma vaga compatível com o limite de similaridade semântica (threshold: %.2f).", threshold)
		log.Printf("[Orchestrator] Early return: %s", reason)
		_ = writeExecutionLog(logFilename, start, res.ItemsProcessed, 0, nil, scores, reason)
		res.DurationMs = time.Since(start).Milliseconds()
		res.Matches = []domain.Match{}
		return res, nil
	}

	// 8. Extract job texts to send for LLM validation
	filteredTexts := make([]string, len(filtered))
	for i, f := range filtered {
		filteredTexts[i] = f.text
	}

	// 9. Request match confirmation, contact extraction, and justifications from LLM
	log.Printf("[Orchestrator] Invoking LLM validator on the %d pre-filtered vacancies...", len(filtered))
	matchRes, err := o.geminiService.MatchResume(ctx, req.FileBase64, mimeType, filteredTexts)
	if err != nil {
		log.Printf("[Orchestrator] LLM validation matching failed: %v", err)
		_ = writeExecutionLog(logFilename, start, res.ItemsProcessed, 0, nil, scores, fmt.Sprintf("Erro no refino de matching LLM: %v", err))
		return nil, fmt.Errorf("gemini matching failed: %w", err)
	}

	// 10. Map indices returned by LLM back to original indexes in res.Items
	var validMatches []domain.Match
	for i := range matchRes.Matches {
		idx := matchRes.Matches[i].Index
		if idx >= 0 && idx < len(filtered) {
			origIdx := filtered[idx].originalIndex
			log.Printf("[Orchestrator] Match index mapping: LLM index %d -> original vacancy index %d", idx, origIdx)
			matchRes.Matches[i].Index = origIdx
			validMatches = append(validMatches, matchRes.Matches[i])
		} else {
			log.Printf("[Orchestrator] Warning: LLM returned out-of-bounds index %d in validator choice", idx)
		}
	}
	matchRes.Matches = validMatches
	log.Printf("[Orchestrator] LLM validated %d final matches.", len(matchRes.Matches))

	// 11. Dispatch applications based on match contacts
	var dispatchResults []dispatchResult
	for i, match := range matchRes.Matches {
		log.Printf("[Orchestrator] Processing match dispatch %d/%d (original index: %d, contact: %s, target: %s)", i+1, len(matchRes.Matches), match.Index, match.ContactType, match.ContactTarget)
		status := "Não disparado (sem contato válido)"
		switch match.ContactType {
		case "email":
			log.Printf("[Orchestrator] Dispatching Email -> to: %s...", match.ContactTarget)
			subject := "Candidatura - Processamento Automático"
			body := fmt.Sprintf("Olá,\n\nEstou me candidatando à vaga de emprego número %d.\n\nMotivo da compatibilidade:\n%s\n\nEm anexo, envio meu currículo para avaliação.\n\nAtenciosamente,\nCandidato", match.Index+1, match.Reason)

			err := o.emailService.SendEmail(ctx, match.ContactTarget, subject, body, req.FileBase64, "curriculo.pdf")
			if err != nil {
				log.Printf("[Orchestrator] Email dispatch to %s failed: %v", match.ContactTarget, err)
				status = fmt.Sprintf("Erro no envio do e-mail: %v", err)
			} else {
				log.Printf("[Orchestrator] Email dispatched successfully to %s", match.ContactTarget)
				status = "Sucesso (e-mail enviado)"
			}

		case "whatsapp":
			log.Printf("[Orchestrator] Dispatching WhatsApp -> to: %s...", match.ContactTarget)
			body := fmt.Sprintf("Olá!\n\nEstou me candidatando à sua vaga de emprego.\n\n*Motivo do Match:*\n%s\n\nEnviei meu currículo por e-mail ou no formato correspondente para análise.", match.Reason)

			err := o.whatsAppService.SendMessage(ctx, match.ContactTarget, body)
			if err != nil {
				log.Printf("[Orchestrator] WhatsApp dispatch to %s failed: %v", match.ContactTarget, err)
				status = fmt.Sprintf("Erro no envio do WhatsApp: %v", err)
			} else {
				log.Printf("[Orchestrator] WhatsApp dispatched successfully to %s", match.ContactTarget)
				status = "Sucesso (WhatsApp enviado)"
			}

		default:
			log.Printf("[Orchestrator] Unknown contact type '%s'. Skipping.", match.ContactType)
			status = fmt.Sprintf("Não disparado (tipo de contato desconhecido: '%s')", match.ContactType)
		}

		dispatchResults = append(dispatchResults, dispatchResult{
			match:  match,
			status: status,
		})
	}

	// 12. Write execution report to log file
	err = writeExecutionLog(logFilename, start, res.ItemsProcessed, len(matchRes.Matches), dispatchResults, scores, "")
	if err != nil {
		log.Printf("[Orchestrator] Error writing report to log file %s: %v", logFilename, err)
	} else {
		log.Printf("[Orchestrator] Detailed report successfully saved to log file: %s", logFilename)
	}

	// 13. Populate final match results and duration
	res.Matches = matchRes.Matches
	res.DurationMs = time.Since(start).Milliseconds()
	log.Printf("[Orchestrator] Total orchestrator process finished in %dms.", res.DurationMs)

	return res, nil
}
