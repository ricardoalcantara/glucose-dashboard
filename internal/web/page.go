package web

import (
	"html/template"
	"log/slog"
	"net/http"
	"time"

	"glucose-dashboard/internal/store"
	"glucose-dashboard/web"
)

// Handler renders the dashboard page.
type Handler struct {
	Store      *store.Store
	Loc        *time.Location
	TargetLow  int
	TargetHigh int
	Now        func() time.Time
	tmpl       *template.Template
	css        template.CSS
}

// NewHandler loads the embedded template and stylesheet.
func NewHandler(st *store.Store, loc *time.Location, targetLow, targetHigh int) (*Handler, error) {
	css, err := site.FS.ReadFile("static/style.css")
	if err != nil {
		return nil, err
	}
	tmpl, err := template.ParseFS(site.FS, "templates/index.html")
	if err != nil {
		return nil, err
	}
	if loc == nil {
		loc = time.UTC
	}
	return &Handler{
		Store:      st,
		Loc:        loc,
		TargetLow:  targetLow,
		TargetHigh: targetHigh,
		tmpl:       tmpl,
		css:        template.CSS(css),
	}, nil
}

func (h *Handler) now() time.Time {
	if h.Now != nil {
		return h.Now()
	}
	return time.Now()
}

type pageData struct {
	CSS        template.CSS
	Empty      bool
	LatestMgdl int
	LatestWhen string
	MinutesAgo int
	Stale      bool
	RangeClass string
	TrendArrow string
	TrendLabel string
	Chart      Chart
	Generated  string
}

// ServeHTTP renders the latest reading and the last 12 hours.
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	now := h.now().UTC()
	latest, ok, err := h.Store.Latest(r.Context())
	if err != nil {
		slog.Error("latest reading", "err", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	rows, err := h.Store.List(r.Context(), now.Add(-chartWindow), now)
	if err != nil {
		slog.Error("list readings", "err", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	data := pageData{
		CSS:       h.css,
		Empty:     !ok,
		Chart:     BuildChart(rows, now, h.Loc, h.TargetLow, h.TargetHigh),
		Generated: now.In(h.Loc).Format("2006-01-02 15:04:05 ") + h.Loc.String(),
	}
	if ok {
		data.LatestMgdl = latest.Mgdl
		data.LatestWhen = latest.TS.In(h.Loc).Format("2006-01-02 15:04")
		age := now.Sub(latest.TS)
		if age < 0 {
			age = 0
		}
		data.MinutesAgo = int(age / time.Minute)
		data.Stale = age > staleAfter
		data.RangeClass = rangeClass(latest.Mgdl, h.TargetLow, h.TargetHigh)
		data.TrendArrow, data.TrendLabel = trendView(latest.Trend)
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	if err := h.tmpl.ExecuteTemplate(w, "index.html", data); err != nil {
		slog.Error("render page", "err", err)
	}
}

func trendView(name string) (arrow, label string) {
	switch name {
	case "SingleDown":
		return "↓", "falling quickly"
	case "FortyFiveDown":
		return "↘", "falling"
	case "Flat":
		return "→", "steady"
	case "FortyFiveUp":
		return "↗", "rising"
	case "SingleUp":
		return "↑", "rising quickly"
	default:
		return "", ""
	}
}

func rangeClass(mgdl, low, high int) string {
	switch {
	case mgdl < low:
		return "low"
	case mgdl > high:
		return "high"
	default:
		return "in-range"
	}
}
