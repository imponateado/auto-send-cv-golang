package handler

import (
	"log"
	"net/http"
	"runtime/debug"
	"time"

	_ "api/docs"
	httpSwagger "github.com/swaggo/http-swagger"
)

// responseWriter wraps http.ResponseWriter to capture the status code of responses.
type responseWriter struct {
	http.ResponseWriter
	statusCode int
}

func newResponseWriter(w http.ResponseWriter) *responseWriter {
	return &responseWriter{w, http.StatusOK}
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

// RegisterRoutes sets up the endpoints and decorates them with logging and recovery middleware.
func RegisterRoutes(mux *http.ServeMux, procHandler *ProcessorHandler, credsHandler *CredentialsHandler, groupHandler *GroupWatchHandler) http.Handler {
	// Register endpoints using Go 1.22+ ServeMux method-matching syntax
	mux.HandleFunc("POST /api/v1/vacancies/clear", procHandler.Clear)
	mux.HandleFunc("POST /api/v1/vacancies", procHandler.Populate)
	mux.HandleFunc("POST /api/v1/match", procHandler.Match)
	mux.HandleFunc("GET /api/v1/tasks", procHandler.ListTasks)
	mux.HandleFunc("GET /api/v1/tasks/{id}", procHandler.GetTaskStatus)

	// Credentials & WhatsApp routes
	mux.HandleFunc("POST /api/v1/credentials", credsHandler.RegisterCredentials)
	mux.HandleFunc("DELETE /api/v1/credentials/{email}", credsHandler.DeleteCredentials)
	mux.HandleFunc("GET /api/v1/whatsapp/connections", credsHandler.ListWhatsAppConnections)
	mux.HandleFunc("GET /api/v1/whatsapp/qr", credsHandler.GetWhatsAppQR)
	mux.HandleFunc("GET /api/v1/whatsapp/status", credsHandler.GetWhatsAppStatus)
	mux.HandleFunc("POST /api/v1/whatsapp/disconnect", credsHandler.DisconnectWhatsApp)

	// Group-watch routes (fonte de vagas via grupos do WhatsApp)
	mux.HandleFunc("GET /api/v1/whatsapp/groups", groupHandler.ListGroups)
	mux.HandleFunc("GET /api/v1/whatsapp/groups/watched", groupHandler.GetWatchedGroups)
	mux.HandleFunc("PUT /api/v1/whatsapp/groups/watched", groupHandler.SetWatchedGroups)
	mux.HandleFunc("GET /api/v1/whatsapp/groups/schedule", groupHandler.GetSchedule)
	mux.HandleFunc("PUT /api/v1/whatsapp/groups/schedule", groupHandler.SetSchedule)
	mux.HandleFunc("POST /api/v1/whatsapp/groups/flush", groupHandler.FlushNow)
	mux.HandleFunc("GET /api/v1/whatsapp/groups/status", groupHandler.Status)

	// Register Swagger UI handler
	mux.Handle("GET /swagger/", httpSwagger.WrapHandler)

	var handler http.Handler = mux
	handler = loggingMiddleware(handler)
	handler = recoveryMiddleware(handler)
	handler = corsMiddleware(handler)

	return handler
}

// corsMiddleware allows the (separately hosted) Flutter frontend to call this API cross-origin.
func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rw := newResponseWriter(w)

		next.ServeHTTP(rw, r)

		log.Printf("[HTTP] %s %s - Status: %d - Duration: %v - IP: %s",
			r.Method, r.URL.Path, rw.statusCode, time.Since(start), r.RemoteAddr)
	})
}

func recoveryMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if err := recover(); err != nil {
				log.Printf("[PANIC RECOVERED] error: %v\nstacktrace:\n%s", err, debug.Stack())
				respondWithError(w, http.StatusInternalServerError, "Internal Server Error")
			}
		}()

		next.ServeHTTP(w, r)
	})
}
