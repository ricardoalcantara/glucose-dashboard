package api

import (
	"log/slog"
	"net/http"
	"time"
)

type readingJSON struct {
	TS     string `json:"ts"`
	Mgdl   int    `json:"mgdl"`
	Source string `json:"source"`
	Trend  string `json:"trend,omitempty"`
}

func (s *Server) getReadings(w http.ResponseWriter, r *http.Request) {
	fromRaw := r.URL.Query().Get("from")
	toRaw := r.URL.Query().Get("to")
	from, errFrom := parseTime(fromRaw)
	to, errTo := parseTime(toRaw)
	if errFrom != nil || errTo != nil || to.Before(from) || to.Sub(from) > maxRange {
		writeText(w, http.StatusBadRequest, "invalid request")
		return
	}

	rows, err := s.Store.List(r.Context(), from, to)
	if err != nil {
		slog.Error("list readings", "err", err)
		writeText(w, http.StatusInternalServerError, "internal error")
		return
	}
	out := make([]readingJSON, 0, len(rows))
	for _, row := range rows {
		out = append(out, readingJSON{
			TS:     row.TS.UTC().Format(time.RFC3339),
			Mgdl:   row.Mgdl,
			Source: row.Source,
			Trend:  row.Trend,
		})
	}
	writeJSON(w, http.StatusOK, out)
}
