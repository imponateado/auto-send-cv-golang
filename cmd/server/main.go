package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"api/internal/config"
	"api/internal/domain"
	"api/internal/handler"
	"api/internal/infra/deepseek"
	"api/internal/infra/email"
	"api/internal/infra/gemini"
	"api/internal/infra/multillm"
	"api/internal/infra/whatsapp"
	"api/internal/service"
)

// @title API Go de Processamento de Texto e Documentos
// @version 1.0
// @description API REST em Go com Clean Architecture para processamento de textos, análise de currículos com Gemini e disparos automáticos.
// @host localhost:8080
// @BasePath /
func main() {
	log.Println("Starting API server initialization...")

	cfg := config.Load()

	smtpHost := os.Getenv("SMTP_HOST")
	smtpPort := os.Getenv("SMTP_PORT")
	smtpUser := os.Getenv("SMTP_USERNAME")
	smtpPass := os.Getenv("SMTP_PASSWORD")
	smtpSender := os.Getenv("SMTP_SENDER")
	emailService := email.NewSMTPSender(smtpHost, smtpPort, smtpUser, smtpPass, smtpSender)

	zapiInstanceID := os.Getenv("ZAPI_INSTANCE_ID")
	zapiToken := os.Getenv("ZAPI_TOKEN")
	zapiClientToken := os.Getenv("ZAPI_CLIENT_TOKEN")
	whatsappService := whatsapp.NewWhatsAppClient(zapiInstanceID, zapiToken, zapiClientToken)

	geminiKey := os.Getenv("GEMINI_API_KEY")
	geminiModel := os.Getenv("GEMINI_MODEL")
	geminiService := gemini.NewGeminiClient(geminiKey, geminiModel)

	deepseekKey := os.Getenv("DEEPSEEK_API_KEY")
	deepseekModel := os.Getenv("DEEPSEEK_MODEL")
	deepseekService := deepseek.NewDeepSeekClient(deepseekKey, deepseekModel)

	var activeLLM domain.GeminiService
	provider := os.Getenv("LLM_PROVIDER")
	if provider == "" {
		provider = "fallback"
	}

	switch provider {
	case "gemini":
		activeLLM = geminiService
	case "deepseek":
		activeLLM = deepseekService
	default: // fallback
		activeLLM = multillm.NewFallbackLLMService(geminiService, deepseekService)
	}

	procService := service.NewTextProcessor()
	matchingOrchestrator := service.NewOrchestrator(
		procService,
		activeLLM,
		geminiService, // Always use Gemini for embeddings
		emailService,
		whatsappService,
	)

	procHandler := handler.NewProcessorHandler(matchingOrchestrator)

	mux := http.NewServeMux()
	router := handler.RegisterRoutes(mux, procHandler)

	server := &http.Server{
		Addr:    ":" + cfg.Port,
		Handler: router,
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	serverErrors := make(chan error, 1)

	go func() {
		log.Printf("Server listening on port %s (%s mode)...", cfg.Port, cfg.Env)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErrors <- err
		}
	}()

	shutdownSignal := make(chan os.Signal, 1)
	signal.Notify(shutdownSignal, os.Interrupt, syscall.SIGTERM)

	select {
	case err := <-serverErrors:
		log.Fatalf("Critical server error: %v", err)

	case sig := <-shutdownSignal:
		log.Printf("Received shutdown signal: %v. Initiating graceful shutdown...", sig)

		// Create a context with a timeout for the graceful shutdown
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()

		// Attempt to gracefully shutdown the server
		if err := server.Shutdown(ctx); err != nil {
			log.Printf("Could not gracefully stop server: %v. Forcing close...", err)
			if err := server.Close(); err != nil {
				log.Fatalf("Error forcing server close: %v", err)
			}
		}
		log.Println("Server stopped gracefully.")
	}
}
