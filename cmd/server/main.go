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
	"api/internal/handler"
	"api/internal/service"
)

func main() {
	log.Println("Starting API server initialization...")

	// Load configuration
	cfg := config.Load()

	// Initialize layers (Dependency Injection)
	procService := service.NewTextProcessor()
	procHandler := handler.NewProcessorHandler(procService)

	// Set up router
	mux := http.NewServeMux()
	router := handler.RegisterRoutes(mux, procHandler)

	// Configure server
	server := &http.Server{
		Addr:    ":" + cfg.Port,
		Handler: router,
		// Timeout configurations:
		ReadHeaderTimeout: 5 * time.Second,  // Protects against Slowloris attacks
		IdleTimeout:       120 * time.Second, // Max time to keep idle connections open
		// Note: We avoid setting http.Server.ReadTimeout to a low value
		// to allow clients to stream large/infinite payloads slowly.
	}

	// Channel to listen for errors from the server listener
	serverErrors := make(chan error, 1)

	// Start the server in a goroutine
	go func() {
		log.Printf("Server listening on port %s (%s mode)...", cfg.Port, cfg.Env)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErrors <- err
		}
	}()

	// Channel to listen for OS signals (termination signals)
	shutdownSignal := make(chan os.Signal, 1)
	signal.Notify(shutdownSignal, os.Interrupt, syscall.SIGTERM)

	// Wait for server error or termination signal
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
