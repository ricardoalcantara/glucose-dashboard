package api

import (
	"encoding/json"
	"io"
	"log/slog"
	"math"
	"net/http"
	"strings"
	"time"

	"glucose-dashboard/internal/model"
)

type ingestRequest struct {
	Source   string       `json:"source"`
	Readings []rawReading `json:"readings"`
}

type rawReading struct {
	TS    string          `json:"ts"`
	Mgdl  json.RawMessage `json:"mgdl"`
	Trend json.RawMessage `json:"trend"`
}

type ingestResponse struct {
	Received int      `json:"received"`
	Inserted int      `json:"inserted"`
	Updated  int      `json:"updated"`
	Rejected []reject `json:"rejected"`
}

type reject struct {
	Index  int    `json:"index"`
	TS     string `json:"ts"`
	Reason string `json:"reason"`
}

func (s *Server) postReadings(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(io.LimitReader(r.Body, maxBodyBytes+1))
	if err != nil || len(body) > maxBodyBytes {
		writeText(w, http.StatusBadRequest, "invalid request")
		return
	}

	var req ingestRequest
	if err := json.Unmarshal(body, &req); err != nil {
		writeText(w, http.StatusBadRequest, "invalid request")
		return
	}
	source := strings.TrimSpace(req.Source)
	if source == "" || len(req.Readings) > maxBatch {
		writeText(w, http.StatusBadRequest, "invalid request")
		return
	}

	now := s.now()
	rejected := make([]reject, 0)
	accepted := make([]model.Reading, 0, len(req.Readings))
	for i, item := range req.Readings {
		reading, reason := validateReading(item, now)
		if reason != "" {
			rejected = append(rejected, reject{Index: i, TS: item.TS, Reason: reason})
			continue
		}
		reading.Source = source
		accepted = append(accepted, reading)
	}

	inserted, updated := 0, 0
	if len(accepted) > 0 {
		inserted, updated, err = s.Store.Upsert(r.Context(), accepted)
		if err != nil {
			slog.Error("upsert readings", "err", err)
			writeText(w, http.StatusInternalServerError, "internal error")
			return
		}
	}

	writeJSON(w, http.StatusOK, ingestResponse{
		Received: len(req.Readings),
		Inserted: inserted,
		Updated:  updated,
		Rejected: rejected,
	})
}

func validateReading(item rawReading, now time.Time) (model.Reading, string) {
	if strings.TrimSpace(item.TS) == "" {
		return model.Reading{}, "timestamp_invalid"
	}
	ts, err := parseTime(item.TS)
	if err != nil {
		return model.Reading{}, "timestamp_invalid"
	}
	if ts.After(now.Add(maxFuture)) {
		return model.Reading{}, "timestamp_in_future"
	}
	mgdl, reason := parseMgdl(item.Mgdl)
	if reason != "" {
		return model.Reading{}, reason
	}
	trend, reason := parseTrend(item.Trend)
	if reason != "" {
		return model.Reading{}, reason
	}
	return model.Reading{
		TS:    ts.UTC().Truncate(time.Minute),
		Mgdl:  mgdl,
		Trend: trend,
	}, ""
}

func parseTrend(raw json.RawMessage) (string, string) {
	trimmed := bytesTrim(raw)
	if len(trimmed) == 0 || string(trimmed) == "null" {
		return "", ""
	}
	var value string
	if err := json.Unmarshal(trimmed, &value); err != nil {
		return "", "trend_invalid"
	}
	value = strings.TrimSpace(value)
	if value == "" {
		return "", ""
	}
	switch value {
	case "SingleDown", "FortyFiveDown", "Flat", "FortyFiveUp", "SingleUp":
		return value, ""
	default:
		return "", "trend_invalid"
	}
}

func parseMgdl(raw json.RawMessage) (int, string) {
	if len(bytesTrim(raw)) == 0 || string(raw) == "null" {
		return 0, "mgdl_not_integer"
	}
	var num float64
	if err := json.Unmarshal(raw, &num); err != nil || math.IsNaN(num) || math.IsInf(num, 0) || num != math.Trunc(num) {
		return 0, "mgdl_not_integer"
	}
	if num < mgdlMin || num > mgdlMax {
		return 0, "mgdl_out_of_range"
	}
	return int(num), ""
}

func bytesTrim(raw json.RawMessage) []byte {
	return []byte(strings.TrimSpace(string(raw)))
}

func parseTime(value string) (time.Time, error) {
	if ts, err := time.Parse(time.RFC3339, value); err == nil {
		return ts, nil
	}
	return time.Parse(time.RFC3339Nano, value)
}
