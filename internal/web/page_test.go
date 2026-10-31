package web

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"glucose-dashboard/internal/model"
	"glucose-dashboard/internal/store"
)

func TestPageEmptyAndWithReadings(t *testing.T) {
	loc, err := time.LoadLocation("America/Sao_Paulo")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 10, 31, 18, 0, 0, 0, time.UTC)

	emptyBody := render(t, loc, now, nil)
	if !strings.Contains(emptyBody, "No glucose readings yet") {
		t.Fatalf("empty page missing empty state: %s", emptyBody)
	}
	if strings.Contains(emptyBody, `role="img"`) {
		t.Fatal("empty page rendered a chart")
	}
	assertCommonPage(t, emptyBody)

	fresh := render(t, loc, now, []model.Reading{{
		TS: now.Add(-2 * time.Minute), Mgdl: 112, Source: "test",
	}})
	for _, want := range []string{
		"112",
		"14:58",
		"2 min ago",
		`class="value in-range"`,
		`role="img"`,
		"Latest 112 mg/dL",
		"prefers-color-scheme",
	} {
		if !strings.Contains(fresh, want) {
			t.Fatalf("fresh page missing %q", want)
		}
	}
	if strings.Contains(fresh, "Stale data") {
		t.Fatal("fresh reading was marked stale")
	}
	assertCommonPage(t, fresh)

	stale := render(t, loc, now, []model.Reading{{
		TS: now.Add(-20 * time.Minute), Mgdl: 54, Source: "test",
	}})
	if !strings.Contains(stale, "Stale data") || !strings.Contains(stale, `class="value low"`) {
		t.Fatalf("stale page = %s", stale)
	}
	assertCommonPage(t, stale)

	outside := render(t, loc, now, []model.Reading{{
		TS: now.Add(-13 * time.Hour), Mgdl: 190, Source: "test",
	}})
	if !strings.Contains(outside, "No readings in the last 12 hours") || !strings.Contains(outside, "Stale data") {
		t.Fatalf("outside window page = %s", outside)
	}
	if !strings.Contains(outside, `class="value high"`) {
		t.Fatal("latest value above range was not marked")
	}
}

func assertCommonPage(t *testing.T, body string) {
	t.Helper()
	if !strings.Contains(body, "Informational only. Not a medical device. Do not use for dosing decisions.") {
		t.Fatal("missing disclaimer")
	}
	lower := strings.ToLower(body)
	if strings.Contains(lower, "<script") || strings.Contains(lower, "http-equiv") {
		t.Fatal("page includes script or a refresh directive")
	}
}

func render(t *testing.T, loc *time.Location, now time.Time, readings []model.Reading) string {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "glucose.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	if len(readings) > 0 {
		if _, _, err := st.Upsert(context.Background(), readings); err != nil {
			t.Fatal(err)
		}
	}
	h, err := NewHandler(st, loc, 70, 180)
	if err != nil {
		t.Fatal(err)
	}
	h.Now = func() time.Time { return now }
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); !strings.Contains(ct, "text/html") {
		t.Fatalf("content type = %s", ct)
	}
	return rec.Body.String()
}
