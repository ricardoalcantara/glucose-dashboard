package api

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"time"

	"glucose-dashboard/internal/store"
)

const (
	maxBodyBytes = 1 << 20
	maxBatch     = 2000
	maxFuture    = 5 * time.Minute
	maxRange     = 7 * 24 * time.Hour
	mgdlMin      = 20
	mgdlMax      = 600
)

// Server serves the ingestion API and health check.
type Server struct {
	Store  *store.Store
	APIKey string
	Now    func() time.Time
}

func (s *Server) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

// NewMux routes the API, health check, and dashboard page.
// page may be nil in tests that do not request GET /.
// A non-empty dashPassword protects GET / with HTTP Basic Auth.
func NewMux(s *Server, dashPassword string, page http.Handler) http.Handler {
	mux := http.NewServeMux()
	mux.Handle("POST /api/v1/readings", bearer(s.APIKey, http.HandlerFunc(s.postReadings)))
	mux.Handle("GET /api/v1/readings", bearer(s.APIKey, http.HandlerFunc(s.getReadings)))
	mux.HandleFunc("GET /healthz", health)
	if page != nil {
		if dashPassword != "" {
			mux.Handle("GET /{$}", basic(dashPassword, page))
		} else {
			mux.Handle("GET /{$}", page)
		}
	}
	return logRequests(mux)
}

func health(w http.ResponseWriter, _ *http.Request) {
	writeText(w, http.StatusOK, "ok")
}

func writeText(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(status)
	_, _ = io.WriteString(w, msg)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(status)
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		slog.Error("write json", "err", err)
	}
}
